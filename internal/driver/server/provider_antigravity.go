package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

const antigravityQuotaURL = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota"

var antigravityQuotaHTTPClient = &http.Client{Timeout: 8 * time.Second}

type antigravityQuotaResponse struct {
	Buckets []antigravityQuotaBucket `json:"buckets"`
}

type antigravityQuotaBucket struct {
	ModelID           string  `json:"modelId"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"`
}

func antigravityOAuthDefinition() providerOAuthDefinition {
	return providerOAuthDefinition{
		Provider:           "antigravity",
		CredentialProvider: "antigravity",
		Endpoint:           "/v0/management/antigravity-auth-url",
		DefaultURL:         "https://daily-cloudcode-pa.googleapis.com",
		Callback:           true,
	}
}

func loadAntigravityQuota(ctx context.Context, credentials map[string]any) *aoyorouter.ProviderQuota {
	accessToken, _ := credentials["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		return &aoyorouter.ProviderQuota{Error: "Antigravity credentials are incomplete"}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityQuotaURL, strings.NewReader("{}"))
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "antigravity")

	response, err := antigravityQuotaHTTPClient.Do(request)
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", response.StatusCode)}
	}

	var usage antigravityQuotaResponse
	if err := json.Unmarshal(data, &usage); err != nil {
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}
	primary, secondary := antigravityQuotaWindows(usage.Buckets)
	if primary == nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	return &aoyorouter.ProviderQuota{
		Primary:   primary,
		Secondary: secondary,
		PlanType:  antigravityPlanType(credentials),
	}
}

func antigravityQuotaWindows(buckets []antigravityQuotaBucket) (*aoyorouter.ProviderQuotaWindow, *aoyorouter.ProviderQuotaWindow) {
	var primary *antigravityQuotaBucket
	var secondary *antigravityQuotaBucket
	for index := range buckets {
		bucket := &buckets[index]
		if strings.TrimSpace(bucket.ResetTime) == "" {
			continue
		}
		if primary == nil || bucket.RemainingFraction < primary.RemainingFraction {
			secondary = primary
			primary = bucket
			continue
		}
		if secondary == nil || bucket.RemainingFraction < secondary.RemainingFraction {
			secondary = bucket
		}
	}
	return antigravityQuotaWindow(primary), antigravityQuotaWindow(secondary)
}

func antigravityQuotaWindow(bucket *antigravityQuotaBucket) *aoyorouter.ProviderQuotaWindow {
	if bucket == nil {
		return nil
	}
	remaining := bucket.RemainingFraction
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 1 {
		remaining = 1
	}
	return &aoyorouter.ProviderQuotaWindow{
		UsedPercent: 100 * (1 - remaining),
		ResetsAt:    bucket.ResetTime,
	}
}

func antigravityPlanType(credentials map[string]any) string {
	if value, _ := credentials["plan_type"].(string); strings.TrimSpace(value) != "" {
		return value
	}
	return "Google Antigravity"
}
