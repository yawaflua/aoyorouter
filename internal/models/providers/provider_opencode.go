package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

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
	removeOpenAICompatibilityConfig(cfg, provider, p.GetOAuthDefinition().Provider)
}

// RemoveProviderConfig implements [ProviderConfig].
func (p *OpencodeGoProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	removeOpenAICompatibilityConfig(cfg, provider, p.GetOAuthDefinition().Provider)
}

func removeOpenAICompatibilityConfig(cfg *config.Config, provider *models.Provider, providerName string) {
	for _, configured := range cfg.OpenAICompatibility {
		if configured.Name == providerName {
			accessToken := provider.ClientSecret
			configured.APIKeyEntries = slices.DeleteFunc(configured.APIKeyEntries, func(entry config.OpenAICompatibilityAPIKey) bool {
				return entry.APIKey == accessToken
			})
			break
		}
	}
}

// AddProviderConfig implements [ProviderConfig].
func (p *OpencodeZenProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	addOpencodeProviderConfig(slog.Default(), cfg, provider, p.GetOAuthDefinition())
}

// AddProviderConfig implements [ProviderConfig].
func (p *OpencodeGoProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	addOpencodeProviderConfig(slog.Default(), cfg, provider, p.GetOAuthDefinition())
}

func addOpencodeProviderConfig(logger *slog.Logger, cfg *config.Config, provider *models.Provider, oauthDef *ProviderOAuthDefinition) {
	baseURL := provider.BaseUrl
	if baseURL == "" {
		baseURL = oauthDef.DefaultURL
	}

	if i := slices.IndexFunc(cfg.OpenAICompatibility, func(provider config.OpenAICompatibility) bool {
		return provider.Name == oauthDef.Provider
	}); i != -1 {
		cfg.OpenAICompatibility[i].APIKeyEntries = append(cfg.OpenAICompatibility[i].APIKeyEntries, config.OpenAICompatibilityAPIKey{
			APIKey:   provider.ClientSecret,
			ProxyURL: provider.Proxy,
		})
	} else {
		modelsURL, err := url.Parse(fmt.Sprintf("%s/models", baseURL))
		if err != nil {
			logger.Error("Error when parsing opencode provider", slog.Any("err", err), slog.String("url", baseURL))
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
			logger.Error("Error when fetching opencode provider models", slog.Any("err", err), slog.String("url", modelsURL.String()))
			return
		}
		var openaiModels []config.OpenAICompatibilityModel
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error("Error reading response", slog.Any("err", err))

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
			if len(openaiModels) == 0 {
				logger.Error("No models found", slog.String("url", modelsURL.String()))
			}
		} else {
			logger.Error("Error when unmarshalling opencode provider response", slog.Any("err", err), slog.String("url", modelsURL.String()))
		}

		cfg.OpenAICompatibility = append(
			cfg.OpenAICompatibility,
			config.OpenAICompatibility{
				Name:    oauthDef.Provider,
				BaseURL: baseURL,
				Prefix:  "opencode",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: provider.ClientSecret},
				},
				Models:   openaiModels,
				Disabled: provider.Disabled,
			},
		)
	}
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *OpencodeZenProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "opencode-zen",
		CredentialProvider: "opencode",
		Endpoint:           "",
		DefaultURL:         OpencodeZenBaseURL,
		Callback:           false,
	}
}

// GetOAuthDefinition implements [ProviderConfig].
func (p *OpencodeGoProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "opencode-go",
		CredentialProvider: "opencode",
		Endpoint:           "",
		DefaultURL:         OpencodeGoBaseURL,
		Callback:           false,
	}
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
