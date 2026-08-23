package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

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
	var server *cursor.Server
	var err error
	if provider.UseProxy && provider.IsCloudflare {
		server, err = p.cursor.CreateServer(ctx, cursor.Config{}, true)
		if err != nil {
			p.logger.Error("cursor provider: failed to start bridge", "error", err)
			return
		}
	} else {
		server, err = p.cursor.CreateServer(ctx, cursor.Config{}, false)
		if err != nil {
			p.logger.Error("cursor provider: failed to start bridge", "error", err)
			return
		}
	}

	modelUrl, err := url.Parse(fmt.Sprintf("%s/v1/models", provider.BaseUrl))
	if err != nil {
		p.logger.Error("Error when parsing custom provider", slog.Any("err", err), slog.String("url", provider.BaseUrl))
		return
	}
	request := &http.Request{
		Method: "GET",
		URL:    modelUrl,
		Header: map[string][]string{
			"Authorization": {"Bearer " + provider.Credentials["access_token"].(string)},
		},
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		p.logger.Error("Error when fetching custom provider models", slog.Any("err", err), slog.String("url", modelUrl.String()))
		return
	}

	var models []config.OpenAICompatibilityModel
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			p.logger.Error("Error reading response", slog.Any("err", err))
			return
		}
		var modelsCustom struct {
			Data []struct {
				ID               string   `json:"id"`
				Root             string   `json:"root"`
				Name             string   `json:"name"`
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &modelsCustom); err != nil {
			p.logger.Error("Error when unmarshalling custom provider response", slog.Any("err", err), slog.String("url", modelUrl.String()))
			return
		}

		for _, customModel := range modelsCustom.Data {
			models = append(models, config.OpenAICompatibilityModel{
				Name:             customModel.ID,
				Alias:            customModel.Root,
				InputModalities:  customModel.InputModalities,
				OutputModalities: customModel.OutputModalities,
				DisplayName:      customModel.Name,
			})
		}
	}

	cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{
		Name:    provider.Name,
		BaseURL: server.BaseURL() + "/v1",
		Prefix:  "cursor",
		APIKeyEntries: []config.OpenAICompatibilityAPIKey{
			{APIKey: provider.ClientSecret},
		},
		Models: models,
	})
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *ProviderCursor) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "cursor",
		CredentialProvider: "cursor",
		DefaultURL:         "https://api2.cursor.sh/",
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
		if configured.Name == provider.Name && len(configured.APIKeyEntries) > 0 && configured.APIKeyEntries[0].APIKey == provider.ClientSecret {
			cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
			return
		}
	}
}
