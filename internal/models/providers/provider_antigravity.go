package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

const antigravityQuotaURL = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota"

type AntigravityProvider struct {
	logger *slog.Logger
}

func NewAntigravityProvider(logger *slog.Logger) *AntigravityProvider {
	return &AntigravityProvider{
		logger: logger,
	}
}

// RemoveProviderConfig implements [ProviderConfig].
func (a *AntigravityProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	apiKey := provider.ClientSecret
	if strings.HasPrefix(apiKey, "oauth:") {
		apiKey = provider.Credentials["access_token"].(string)
	}
	for index, key := range cfg.GeminiKey {
		if key.APIKey == apiKey && key.BaseURL == provider.BaseUrl {
			cfg.GeminiKey = append(cfg.GeminiKey[:index], cfg.GeminiKey[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [providers.ProviderConfig].
func (a *AntigravityProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}

	cfg.GeminiKey = append(cfg.GeminiKey, config.GeminiKey{
		APIKey:   provider.ClientSecret,
		BaseURL:  provider.BaseUrl,
		ProxyURL: provider.Proxy,
		Prefix:   "antigravity",
	})
}

// GetOAuthDefinition implements [providers.ProviderConfig].
func (a *AntigravityProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "antigravity",
		CredentialProvider: "antigravity",
		Endpoint:           "/v0/management/antigravity-auth-url",
		DefaultURL:         "https://daily-cloudcode-pa.googleapis.com",
		Callback:           true,
	}
}

// LoadQuota implements [providers.ProviderConfig].
func (a *AntigravityProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return a.loadAntigravityQuota(ctx, credentials, useProxy, proxyURL)
}

type antigravityQuotaResponse struct {
	Buckets []antigravityQuotaBucket `json:"buckets"`
}

type antigravityQuotaBucket struct {
	ModelID           string  `json:"modelId"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"`
}

func (a *AntigravityProvider) loadAntigravityQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	accessToken, _ := credentials["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		a.logger.Error("Antigravity credentials are incomplete")
		return &aoyorouter.ProviderQuota{Error: "Antigravity credentials are incomplete"}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityQuotaURL, strings.NewReader("{}"))
	if err != nil {
		a.logger.Error("Failed to create quota request", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "antigravity")

	client, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		a.logger.Error("Failed to create proxy HTTP client", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	response, err := client.Do(request)
	if err != nil {
		a.logger.Error("Failed to send quota request", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		a.logger.Error("Failed to read quota response", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		a.logger.Error("Quota returned status", "status", response.StatusCode)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", response.StatusCode)}
	}

	var usage antigravityQuotaResponse
	if err := json.Unmarshal(data, &usage); err != nil {
		a.logger.Error("Invalid quota response", "error", err)
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}
	result := antigravityQuotaWindows(usage.Buckets)
	if len(result) == 0 {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	return &aoyorouter.ProviderQuota{
		Quotas:   result,
		PlanType: antigravityPlanType(credentials),
	}
}

func antigravityQuotaWindows(buckets []antigravityQuotaBucket) []*aoyorouter.ProviderQuotaWindow {
	var result []*aoyorouter.ProviderQuotaWindow
	for index := range buckets {
		bucket := &buckets[index]
		result = append(result, antigravityQuotaWindow(bucket))
	}
	return result
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
		Name:        bucket.ModelID,
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
