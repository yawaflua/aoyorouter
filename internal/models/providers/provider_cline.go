package providers

import (
	"context"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type ProviderCline struct {
}

// AddProviderConfig implements [ProviderConfig].
func (p *ProviderCline) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}
	var models []config.OpenAICompatibilityModel = []config.OpenAICompatibilityModel{
		{
			Name:        "cline-pass/glm-5.3",
			DisplayName: "GLM-5.3",
		},
		{
			Name:        "cline-pass/glm-5.2",
			DisplayName: "GLM-5.2",
		},
		{
			Name:        "cline-pass/kimi-k3",
			DisplayName: "Kimi-K3",
		},
		{
			Name:        "cline-pass/kimi-k2.7-code",
			DisplayName: "Kimi-K2.7-Code",
		},
		{
			Name:        "cline-pass/kimi-k2.6",
			DisplayName: "Kimi-K2.6",
		},
		{
			Name:        "cline-pass/deepseek-v4-pro",
			DisplayName: "DeepSeek-V4-Pro",
		},
		{
			Name:        "cline-pass/deepseek-v4-flash",
			DisplayName: "DeepSeek-V4-Flash",
		},
		{
			Name:        "cline-pass/mimo-v2.5",
			DisplayName: "MIMO-V2.5",
		},
		{
			Name:        "cline-pass/mimo-v2.5-pro",
			DisplayName: "MIMO-V2.5-Pro",
		},
		{
			Name:        "cline-pass/minimax-m3",
			DisplayName: "Minimax-M3",
		},
		{
			Name:        "cline-pass/qwen3.8-max",
			DisplayName: "Qwen3.8-Max",
		},
		{
			Name:        "cline-pass/qwen3.7-max",
			DisplayName: "Qwen3.7-Max",
		},
		{
			Name:        "cline-pass/qwen3.7-plus",
			DisplayName: "Qwen3.7-Plus",
		},
		{
			Name:        "minimax/minimax-m2.5",
			DisplayName: "Cline/Minimax-M2.5",
		},
	}
	cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{Name: "cline", Models: models, BaseURL: provider.BaseUrl, APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: provider.ClientSecret}}})

}

// GetOAuthDefinition implements [ProviderConfig].
func (p *ProviderCline) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "cline",
		CredentialProvider: "cline",
		DefaultURL:         "https://api.cline.bot/api/v1/",
		Callback:           false,
	}
}

// LoadQuota implements [ProviderConfig].
func (p *ProviderCline) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return nil
}

// RemoveProviderConfig implements [ProviderConfig].
func (p *ProviderCline) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	for index, configured := range cfg.OpenAICompatibility {
		if configured.Name == provider.Name && configured.BaseURL == provider.BaseUrl && configured.APIKeyEntries[0].APIKey == provider.ClientSecret {
			cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
			return
		}
	}
}
