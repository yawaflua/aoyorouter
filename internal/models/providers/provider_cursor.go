package providers

import (
	"context"
	"fmt"
	"log/slog"
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
	// if provider.ClientSecret == "oauth:database" {
	// 	return
	// }
	if len(provider.Credentials) == 0 {
		p.logger.Error("cursor provider: no credentials")
		return
	}

	var server *cursor.Server
	var err error
	if provider.UseProxy && provider.IsCloudflare {
		server, err = p.cursor.CreateServer(ctx, cursor.Config{
			CursorClientVersion: "3.17.8",
		}, true)
		if err != nil {
			p.logger.Error("cursor provider: failed to start bridge", "error", err)
			return
		}
	} else {
		server, err = p.cursor.CreateServer(ctx, cursor.Config{
			CursorClientVersion: "3.17.8",
		}, false)
		if err != nil {
			p.logger.Error("cursor provider: failed to start bridge", "error", err)
			return
		}
	}
	provider.BaseUrl = server.BaseURL()

	models, err := server.HandleModels(ctx, provider.Credentials["access_token"].(string), "")
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
	for index, configured := range cfg.OpenAICompatibility {
		if configured.Name == provider.ID {
			cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
			return
		}
	}
}
