package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

func (a *AoyoRouterService) loadQuota(provider *models.Provider, ctx context.Context) (*aoyorouter.ProviderQuota, error) {
	if provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM || provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_ZEN || provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_GO {
		return nil, nil
	}
	if provider.Disabled {
		return nil, nil
	}

	if quota, ok := a.cache.GetQuota(provider.ID); ok {
		return quota, nil
	}
	cfg, err := a.providerVendor.ProviderOAuthConfig(aoyorouter.ProviderType(provider.Type))
	if err != nil {
		return nil, err
	}
	if quota := cfg.LoadQuota(ctx, provider.Credentials, provider.UseProxy, provider.Proxy); quota != nil {
		a.cache.SaveQuota(provider.ID, quota)
		return quota, nil
	}
	return nil, nil
}

func (a *AoyoRouterService) addProvider(provider *models.Provider, ctx context.Context) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.addProviderConfig(ctx, a.CPAPIConfig, provider)
	err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
	if err != nil {
		return err
	}
	return a.cpapiRestarter(ctx)
}

func (a *AoyoRouterService) removeProvider(provider *models.Provider) error {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return nil
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.removeProviderConfig(a.CPAPIConfig, provider)
	return config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
}

func (a *AoyoRouterService) addProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if provider.Disabled {
		return
	}
	if provider.UseProxy && provider.Proxy == "" {
		proxy := a.warp.CreateProxy(ctx, fmt.Sprintf("%s-%s", provider.ID, provider.Name))
		provider.Proxy = fmt.Sprintf("http://%s", proxy.Addr().String())
	}
	if conf, err := a.providerVendor.ProviderOAuthConfig(aoyorouter.ProviderType(provider.Type)); err == nil {
		definition := conf.GetOAuthDefinition()
		if definition == nil || !definition.Callback {
			conf.AddProviderConfig(ctx, cfg, provider)
		}
	}
}

func (a *AoyoRouterService) removeProviderConfig(cfg *config.Config, provider *models.Provider) {
	if conf, err := a.providerVendor.ProviderOAuthConfig(aoyorouter.ProviderType(provider.Type)); err == nil {
		definition := conf.GetOAuthDefinition()
		if definition == nil || !definition.Callback {
			conf.RemoveProviderConfig(cfg, provider)
		}
	}
}

func providerToProto(provider *models.Provider) *aoyorouter.Provider {
	return &aoyorouter.Provider{
		Id:           provider.ID,
		Name:         provider.Name,
		Type:         aoyorouter.ProviderType(provider.Type),
		ClientId:     provider.BaseUrl,
		ClientSecret: provider.ClientSecret,
		UseProxy:     provider.UseProxy,
		Proxy:        provider.Proxy,
		IsCloudflare: provider.IsCloudflare,
		Priority:     int32(provider.Priority),
		Disabled:     provider.Disabled,
	}
}
