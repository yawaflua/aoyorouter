package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

func (a *AoyoRouterService) addProvider(provider *models.Provider, ctx context.Context) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.addProviderConfig(ctx, a.CPAPIConfig, provider)
	return config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
}

func (a *AoyoRouterService) removeProvider(provider *models.Provider) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	removeProviderConfig(a.CPAPIConfig, provider)
	return config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
}

func (a *AoyoRouterService) addProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if provider.UseProxy && provider.Proxy == "" {
		proxy := a.warp.CreateProxy(ctx, fmt.Sprintf("%s-%s", provider.ID, provider.Name))
		provider.Proxy = fmt.Sprintf("http://%s", proxy.Addr().String())
	}

	switch aoyorouter.ProviderType(provider.Type) {
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		cfg.CodexKey = append(cfg.CodexKey, config.CodexKey{APIKey: provider.ClientSecret, BaseURL: provider.ClientID, Prefix: "cx"})
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		cfg.ClaudeKey = append(cfg.ClaudeKey, config.ClaudeKey{APIKey: provider.ClientSecret, BaseURL: provider.ClientID})
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		cfg.XAIKey = append(cfg.XAIKey, config.XAIKey{APIKey: provider.ClientSecret, BaseURL: provider.ClientID, Prefix: "grok"})
	case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{Name: "kimi", BaseURL: provider.ClientID, APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: provider.ClientSecret}}})
	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
		modelUrl, err := url.Parse(fmt.Sprintf("%s/v1/models", provider.ClientID))
		if err != nil {
			fmt.Printf("Error when parsing custom provider: %v\n", err)
			break
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
			fmt.Printf("Error when fetching custom provider models: %v\n", err)
			break
		}
		defer resp.Body.Close()
		var body []byte
		if _, err := resp.Body.Read(body); err != nil {
			fmt.Printf("Error when dumping custom provider response: %v\n", err)
			break
		}
		fmt.Printf("Custom provider response: %s\n", string(body))
		var models struct {
			Data []config.OpenAICompatibilityModel `json:"data"`
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
		if err := json.Unmarshal(body, &models); err != nil {
			fmt.Printf("Error when unmarshalling custom provider response: %v\n", err)
			break
		}
		if err := json.Unmarshal(body, &modelsCustom); err != nil {
			fmt.Printf("Error when unmarshalling custom provider response: %v\n", err)
			break
		}

		for _, customModel := range modelsCustom.Data {
			for _, model := range models.Data {
				if customModel.ID == model.Name {
					model.Alias = customModel.Root
					model.DisplayName = customModel.Name
				}
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
				Models: models.Data,
			},
		)
	}
}

func removeProviderConfig(cfg *config.Config, provider *models.Provider) {
	switch aoyorouter.ProviderType(provider.Type) {
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		for index, key := range cfg.CodexKey {
			if key.APIKey == provider.ClientSecret && key.BaseURL == provider.ClientID {
				cfg.CodexKey = append(cfg.CodexKey[:index], cfg.CodexKey[index+1:]...)
				return
			}
		}
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		if strings.HasPrefix(provider.ClientSecret, "oauth:") {
			return
		}
		for index, key := range cfg.ClaudeKey {
			if key.APIKey == provider.ClientSecret && key.BaseURL == provider.ClientID {
				cfg.ClaudeKey = append(cfg.ClaudeKey[:index], cfg.ClaudeKey[index+1:]...)
				return
			}
		}
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		for index, key := range cfg.XAIKey {
			if key.APIKey == provider.ClientSecret && key.BaseURL == provider.ClientID {
				cfg.XAIKey = append(cfg.XAIKey[:index], cfg.XAIKey[index+1:]...)
				return
			}
		}
	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
		for index, configured := range cfg.OpenAICompatibility {
			if configured.Name == provider.Name && configured.BaseURL == provider.ClientID {
				cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
				return
			}
		}
	}
}

func providerToProto(provider *models.Provider) *aoyorouter.Provider {
	return &aoyorouter.Provider{
		Id:           provider.ID,
		Name:         provider.Name,
		Type:         aoyorouter.ProviderType(provider.Type),
		ClientId:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
		UseProxy:     provider.UseProxy,
		Proxy:        provider.Proxy,
	}
}
