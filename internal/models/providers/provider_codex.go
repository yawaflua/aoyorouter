package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type CodexProvider struct {
	logger *slog.Logger
}

func NewCodexProvider(logger *slog.Logger) *CodexProvider {
	return &CodexProvider{
		logger: logger,
	}
}


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

func (c *CodexProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	apiKey := provider.ClientSecret
	if strings.HasPrefix(apiKey, "oauth:") {
		apiKey = provider.Credentials["access_token"].(string)
	}
	for index, key := range cfg.CodexKey {
		if key.APIKey == apiKey && key.BaseURL == provider.BaseUrl {
			cfg.CodexKey = append(cfg.CodexKey[:index], cfg.CodexKey[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [providers.ProviderConfig].
func (c *CodexProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}

	cfg.CodexKey = append(cfg.CodexKey, config.CodexKey{
		APIKey:   provider.ClientSecret,
		BaseURL:  provider.BaseUrl,
		ProxyURL: provider.Proxy,
		Prefix:   "codex",
	})
}

// GetOAuthDefinition implements [providers.ProviderConfig].
func (c *CodexProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Endpoint:           "/v0/management/codex-auth-url",
		Callback:           true,
		Provider:           "codex",
		CredentialProvider: "codex",
		DefaultURL:         "https://api.openai.com",
	}
}

// LoadQuota implements [providers.ProviderConfig].
func (c *CodexProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return c.loadCodexQuota(ctx, credentials, useProxy, proxyURL)
}

func (a *CodexProvider) loadCodexQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	if len(credentials) == 0 {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	accessToken, _ := credentials["access_token"].(string)
	accountID, _ := credentials["account_id"].(string)
	if accessToken == "" || accountID == "" {
		return &aoyorouter.ProviderQuota{Error: "Codex credentials are incomplete"}
	}
	client, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota request failed: %v", err)}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota request failed: %v", err)}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Chatgpt-Account-Id", accountID)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Originator", "codex_cli_rs")
	response, err := client.Do(request)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota request failed: %v", err)}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		a.logger.Error("Quota request failed", "status", response.StatusCode)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", response.StatusCode)}
	}
	var usage codexUsageResponse
	if err := json.NewDecoder(response.Body).Decode(&usage); err != nil {
		a.logger.Error("Quota request failed", "error", err)
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
		Quotas:   []*aoyorouter.ProviderQuotaWindow{quotaWindowToProto(primary), quotaWindowToProto(secondary)},
		PlanType: usage.PlanType,
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
