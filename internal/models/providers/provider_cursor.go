package providers

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	cursor_adapter "github.com/yawaflua/aoyorouter/internal/adapter/cursor"
	"github.com/yawaflua/aoyorouter/internal/models"
	"github.com/yawaflua/aoyorouter/pkg/cursor"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type ProviderCursor struct {
	cursor *cursor_adapter.CursorServer
	logger *slog.Logger
}

func NewCursorProvider(logger *slog.Logger, cursor *cursor_adapter.CursorServer) *ProviderCursor {
	return &ProviderCursor{
		logger: logger,
		cursor: cursor,
	}
}

// AddProviderConfig implements [ProviderConfig].
func (p *ProviderCursor) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if len(provider.Credentials) == 0 {
		p.logger.Error("cursor provider: no credentials")
		return
	}
	if i := slices.IndexFunc(cfg.OpenAICompatibility, func(model config.OpenAICompatibility) bool {
		return model.Name == p.GetOAuthDefinition().Provider
	}); i != -1 {
		proxyUrl := ""
		if provider.UseProxy {
			if provider.Proxy != "" {
				proxyUrl = provider.Proxy
			}
		}

		accessToken, _ := provider.Credentials["access_token"].(string)
		if strings.TrimSpace(accessToken) == "" {
			p.logger.Error("cursor provider: missing access token")
			return
		}
		if proxyUrl != "" {
			p.cursor.SetProxy(accessToken, proxyUrl)
		}
		cfg.OpenAICompatibility[i].APIKeyEntries = append(cfg.OpenAICompatibility[i].APIKeyEntries, config.OpenAICompatibilityAPIKey{
			APIKey: provider.ClientSecret,
		})
	} else {

		server, err := p.cursor.GetOrCreateServer(ctx, cursor.Config{
			CursorClientVersion: "3.17.8",
		})
		if err != nil {
			p.logger.Error("cursor provider: failed to start bridge", "error", err)
			return
		}

		proxyUrl := ""
		if provider.UseProxy {
			if provider.Proxy != "" {
				proxyUrl = provider.Proxy
			}
		}

		provider.BaseUrl = server.BaseURL()

		models, err := server.HandleModels(ctx, provider.Credentials["access_token"].(string), "", proxyUrl)
		if err != nil {
			p.logger.Error("Error when fetching custom provider models", slog.Any("err", err))
			return
		}

		var customModels []config.OpenAICompatibilityModel
		for _, model := range models {
			customModels = append(customModels, config.OpenAICompatibilityModel{
				Name: model.Id,
			})
		}

		accessToken, _ := provider.Credentials["access_token"].(string)
		if strings.TrimSpace(accessToken) == "" {
			p.logger.Error("cursor provider: missing access token")
			return
		}

		if proxyUrl != "" {
			server.SetProxy(accessToken, proxyUrl)
		}
		cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{
			Name:     provider.ID,
			Disabled: provider.Disabled,
			BaseURL:  server.BaseURL(),
			Prefix:   "cursor",
			APIKeyEntries: []config.OpenAICompatibilityAPIKey{
				{APIKey: accessToken},
			},
			Models: customModels,
		})
	}
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *ProviderCursor) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "cursor",
		CredentialProvider: "cursor",
		DefaultURL:         "https://api2.cursor.sh",
		Callback:           false,
	}
}

// LoadQuota implements [ProviderConfig]
func (p *ProviderCursor) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return nil
}

// RemoveProviderConfig implements [ProviderConfig].
func (p *ProviderCursor) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	for i, configured := range cfg.OpenAICompatibility {
		if configured.Name == p.GetOAuthDefinition().Provider {
			accessToken, _ := provider.Credentials["access_token"].(string)
			cfg.OpenAICompatibility[i].APIKeyEntries = slices.DeleteFunc(configured.APIKeyEntries, func(entry config.OpenAICompatibilityAPIKey) bool {
				return entry.APIKey == accessToken
			})
			break
		}
	}
}
