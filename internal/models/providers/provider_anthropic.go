package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type AnthropicProvider struct {
	logger *slog.Logger
}

func NewAnthropicProvider(logger *slog.Logger) *AnthropicProvider {
	return &AnthropicProvider{
		logger: logger,
	}
}

const anthropicUsageURL = "https://api.anthropic.com/api/oauth/usage"

type anthropicUsageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type anthropicUsageResponse struct {
	FiveHour *anthropicUsageWindow `json:"five_hour"`
	SevenDay *anthropicUsageWindow `json:"seven_day"`
	PlanType string                `json:"plan_type"`
}

// RemoveProviderConfig implements [ProviderConfig].
func (a *AnthropicProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	apiKey := provider.ClientSecret
	if strings.HasPrefix(apiKey, "oauth:") {
		apiKey = provider.Credentials["access_token"].(string)
	}
	for index, key := range cfg.ClaudeKey {
		if key.APIKey == apiKey && key.BaseURL == provider.BaseUrl {
			cfg.ClaudeKey = append(cfg.ClaudeKey[:index], cfg.ClaudeKey[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [providers.ProviderConfig].
func (a *AnthropicProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}

	cfg.ClaudeKey = append(cfg.ClaudeKey, config.ClaudeKey{
		APIKey:   provider.ClientSecret,
		BaseURL:  provider.BaseUrl,
		ProxyURL: provider.Proxy,
		Prefix:   "anthropic",
	})
}

// GetOAuthDefinition implements [providers.ProviderConfig].
func (a *AnthropicProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "anthropic",
		CredentialProvider: "claude",
		Endpoint:           "/v0/management/anthropic-auth-url",
		Callback:           true,
		DefaultURL:         "https://anthropic.com",
	}
}

// LoadQuota implements [providers.ProviderConfig].
func (a *AnthropicProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	accessToken, _ := credentials["access_token"].(string)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		
		return &aoyorouter.ProviderQuota{Error: "Anthropic credentials are incomplete"}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, anthropicUsageURL, nil)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Anthropic-Beta", "oauth-2025-04-20")
	request.Header.Set("User-Agent", "claude-code")

	client, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	response, err := client.Do(request)
	if err != nil {
		a.logger.Error("Quota request failed", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		a.logger.Error("Quota request failed", "status", response.StatusCode)
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", response.StatusCode)}
	}

	var usage anthropicUsageResponse
	if err := json.NewDecoder(response.Body).Decode(&usage); err != nil {
		a.logger.Error("Invalid quota response", "error", err)
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}

	primary := anthropicQuotaWindow(usage.FiveHour, 5*60)
	secondary := anthropicQuotaWindow(usage.SevenDay, 7*24*60)
	if primary == nil {
		primary, secondary = secondary, nil
	}
	if primary == nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	planType := strings.TrimSpace(usage.PlanType)
	if planType == "" {
		planType = anthropicPlanType(credentials)
	}
	return &aoyorouter.ProviderQuota{
		Quotas:   []*aoyorouter.ProviderQuotaWindow{primary, secondary},
		PlanType: planType,
	}
}

func anthropicQuotaWindow(window *anthropicUsageWindow, windowMinutes int32) *aoyorouter.ProviderQuotaWindow {
	if window == nil || window.Utilization == nil {
		return nil
	}
	usedPercent := *window.Utilization
	if usedPercent < 0 {
		usedPercent = 0
	}
	if usedPercent > 100 {
		usedPercent = 100
	}
	return &aoyorouter.ProviderQuotaWindow{
		UsedPercent:   usedPercent,
		ResetsAt:      window.ResetsAt,
		WindowMinutes: windowMinutes,
	}
}

func anthropicPlanType(credentials map[string]any) string {
	for _, key := range []string{"subscription_type", "plan_type"} {
		if value, _ := credentials[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "Claude"
}
