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

const (
	OpencodeZenBaseURL = "https://opencode.ai/zen/v1"
	OpencodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
)

type OpencodeZenProvider struct {
	logger *slog.Logger
}

type OpencodeGoProvider struct {
	logger *slog.Logger
}

func NewOpencodeZenProvider(logger *slog.Logger) *OpencodeZenProvider {
	return &OpencodeZenProvider{
		logger: logger,
	}
}

func NewOpencodeGoProvider(logger *slog.Logger) *OpencodeGoProvider {
	return &OpencodeGoProvider{
		logger: logger,
	}
}

// RemoveProviderConfig implements [ProviderConfig].
func (p *OpencodeZenProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	removeOpenAICompatibilityConfig(cfg, provider)
}

// RemoveProviderConfig implements [ProviderConfig].
func (p *OpencodeGoProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	removeOpenAICompatibilityConfig(cfg, provider)
}

func removeOpenAICompatibilityConfig(cfg *config.Config, provider *models.Provider) {
	for index, configured := range cfg.OpenAICompatibility {
		if configured.Name == provider.ID && configured.BaseURL == provider.BaseUrl && configured.APIKeyEntries[0].APIKey == provider.ClientSecret {
			cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [ProviderConfig].
func (p *OpencodeZenProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	addOpencodeProviderConfig(slog.Default(), cfg, provider, OpencodeZenBaseURL)
}

// AddProviderConfig implements [ProviderConfig].
func (p *OpencodeGoProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	addOpencodeProviderConfig(slog.Default(), cfg, provider, OpencodeGoBaseURL)
}

func addOpencodeProviderConfig(logger *slog.Logger, cfg *config.Config, provider *models.Provider, defaultBaseURL string) {
	baseURL := provider.BaseUrl
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	modelsURL, err := url.Parse(fmt.Sprintf("%s/models", baseURL))
	if err != nil {
		fmt.Println("Error when parsing opencode provider", slog.Any("err", err), slog.String("url", baseURL))
		return
	}
	request := &http.Request{
		Method: "GET",
		URL:    modelsURL,
		Header: map[string][]string{
			"Authorization": {"Bearer " + provider.ClientSecret},
		},
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Println("Error when fetching opencode provider models", slog.Any("err", err), slog.String("url", modelsURL.String()))
		return
	}
	var openaiModels []config.OpenAICompatibilityModel
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Error reading response", slog.Any("err", err))
			return
		}
		var response struct {
			Data []struct {
				ID               string   `json:"id"`
				Root             string   `json:"root"`
				Name             string   `json:"name"`
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			logger.Error("Error when unmarshalling opencode provider response", slog.Any("err", err), slog.String("url", modelsURL.String()))
			return
		}
		for _, model := range response.Data {
			openaiModels = append(openaiModels, config.OpenAICompatibilityModel{
				Name:             model.ID,
				Alias:            model.Root,
				InputModalities:  model.InputModalities,
				OutputModalities: model.OutputModalities,
				DisplayName:      model.Name,
			})
		}
	} else {
		logger.Error("Error when unmarshalling opencode provider response", slog.Any("err", err), slog.String("url", modelsURL.String()))
	}

	cfg.OpenAICompatibility = append(
		cfg.OpenAICompatibility,
		config.OpenAICompatibility{
			Name:    provider.ID,
			BaseURL: baseURL,
			Prefix:  "opencode",
			APIKeyEntries: []config.OpenAICompatibilityAPIKey{
				{APIKey: provider.ClientSecret},
			},
			Models: openaiModels,
		},
	)
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *OpencodeZenProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return nil
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *OpencodeGoProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return nil
}

// LoadQuota implements [ProviderConfig].
func (p *OpencodeZenProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return &aoyorouter.ProviderQuota{
		Error: "not supported",
	}
}

// LoadQuota implements [ProviderConfig].
func (p *OpencodeGoProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	return &aoyorouter.ProviderQuota{
		Error: "not supported",
	}
}
