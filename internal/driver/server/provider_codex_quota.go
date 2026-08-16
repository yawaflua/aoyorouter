package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type codexUsageWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	ResetAt       any     `json:"reset_at"`
	WindowMinutes int32   `json:"window_minutes"`
	WindowSeconds int32   `json:"limit_window_seconds"`
}

type codexUsageResponse struct {
	PlanType  string            `json:"plan_type"`
	Primary   *codexUsageWindow `json:"primary"`
	Secondary *codexUsageWindow `json:"secondary"`
	RateLimit *struct {
		Primary         *codexUsageWindow `json:"primary"`
		Secondary       *codexUsageWindow `json:"secondary"`
		PrimaryWindow   *codexUsageWindow `json:"primary_window"`
		SecondaryWindow *codexUsageWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

func loadCodexQuota(ctx context.Context, credentials map[string]any) *aoyorouter.ProviderQuota {
	if len(credentials) == 0 {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	accessToken, _ := credentials["access_token"].(string)
	accountID, _ := credentials["account_id"].(string)
	if accessToken == "" || accountID == "" {
		return &aoyorouter.ProviderQuota{Error: "Codex credentials are incomplete"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Chatgpt-Account-Id", accountID)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Originator", "codex_cli_rs")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", response.StatusCode)}
	}
	var usage codexUsageResponse
	if err := json.NewDecoder(response.Body).Decode(&usage); err != nil {
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}
	primary := usage.Primary
	secondary := usage.Secondary
	if usage.RateLimit != nil {
		if primary == nil {
			primary = usage.RateLimit.PrimaryWindow
			if primary == nil {
				primary = usage.RateLimit.Primary
			}
		}
		if secondary == nil {
			secondary = usage.RateLimit.SecondaryWindow
			if secondary == nil {
				secondary = usage.RateLimit.Secondary
			}
		}
	}
	return &aoyorouter.ProviderQuota{
		Primary:   quotaWindowToProto(primary),
		Secondary: quotaWindowToProto(secondary),
		PlanType:  usage.PlanType,
	}
}

func quotaWindowToProto(window *codexUsageWindow) *aoyorouter.ProviderQuotaWindow {
	if window == nil {
		return nil
	}
	windowMinutes := window.WindowMinutes
	if windowMinutes == 0 && window.WindowSeconds > 0 {
		windowMinutes = window.WindowSeconds / 60
	}
	return &aoyorouter.ProviderQuotaWindow{
		UsedPercent:   window.UsedPercent,
		ResetsAt:      quotaResetString(window.ResetAt),
		WindowMinutes: windowMinutes,
	}
}

func quotaResetString(value any) string {
	switch reset := value.(type) {
	case string:
		return reset
	case float64:
		return time.Unix(int64(reset), 0).UTC().Format(time.RFC3339)
	default:
		return ""
	}
}
