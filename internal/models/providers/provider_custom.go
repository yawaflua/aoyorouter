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
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type CustomProvider struct {
	logger *slog.Logger
}

// RemoveProviderConfig implements [ProviderConfig].
func (a *CustomProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	for index, configured := range cfg.XAIKey {
		if configured.APIKey == provider.ClientSecret && configured.BaseURL == provider.ClientID {
			cfg.XAIKey = append(cfg.XAIKey[:index], cfg.XAIKey[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [ProviderConfig].
func (a *CustomProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	modelUrl, err := url.Parse(fmt.Sprintf("%s/v1/models", provider.ClientID))
	if err != nil {
		a.logger.Error("Error when parsing custom provider", slog.Any("err", err), slog.String("url", provider.ClientID))
		return
	}
	request := &http.Request{
		Method: "GET",
		URL:    modelUrl,
		Header: map[string][]string{
			"Authorization": {"Bearer " + provider.ClientSecret},
		},
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		a.logger.Error("Error when fetching custom provider models", slog.Any("err", err), slog.String("url", modelUrl.String()))
		return
	}
	var models []config.OpenAICompatibilityModel
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			a.logger.Error("Error reading response", slog.Any("err", err))
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
			a.logger.Error("Error when unmarshalling custom provider response", slog.Any("err", err), slog.String("url", modelUrl.String()))
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

	cfg.OpenAICompatibility = append(
		cfg.OpenAICompatibility,
		config.OpenAICompatibility{
			Name:    provider.ID,
			BaseURL: provider.ClientID,
			APIKeyEntries: []config.OpenAICompatibilityAPIKey{
				{APIKey: provider.ClientSecret},
			},
			Models: models,
		},
	)
}

// GetOAuthDefinition implements [ProviderConfig].
func (c *CustomProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return nil
}

// LoadQuota implements [ProviderConfig].
func (c *CustomProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return &aoyorouter.ProviderQuota{
		Error: "not supported",
	}
}
