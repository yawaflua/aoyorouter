package provider

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	config "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/app/provider/cliproxyapi"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

func (p *P) InitCPAPI(ctx context.Context, pbHandler http.Handler) error {
	if p.cliproxy != nil {
		return nil
	}

	if pbHandler == nil {
		panic("pbHandler is nil")
	}

	access.RegisterProvider("psql", cliproxyapi.NewAccessProvider(p.ApiKeyRepo(ctx)))
	err := p.registerAllProviders(ctx)
	if err != nil {
		return err
	}

	cliproxy, err := cliproxy.NewBuilder().
		WithConfig(p.CLIProxyAPIConfig()).
		WithAuthManager(auth.NewManager(auth.NewFileTokenStore(), auth.NewAntigravityAuthenticator(), auth.NewClaudeAuthenticator(), auth.NewCodexAuthenticator())).
		WithAPIKeyClientProvider(cliproxy.NewAPIKeyClientProvider()).
		WithServerOptions(api.WithEngineConfigurator(func(e *gin.Engine) {
			e.Any("/api/aoyo/*path", gin.WrapH(pbHandler))
		})).
		WithConfigPath("config.yaml").
		Build()
	if err != nil {
		p.logger.Error("failed to create CLIProxyAPI", "error", err)
		panic("failed to create CLIProxyAPI")
	}
	p.cliproxy = cliproxy

	p.Closer().Add(func() error {
		p.Logger().Info("cpapi shutdown")
		return p.CLIProxyAPI(ctx).Shutdown(ctx)
	})
	return nil
}

func (p *P) CLIProxyAPI(ctx context.Context) *cliproxy.Service {
	if p.cliproxy == nil {
		p.Logger().Error("provider.CLIProxyAPI method was called before provider.InitCPAPI")
		panic("provider.CLIProxyAPI method was called before provider.InitCPAPI")
	}
	return p.cliproxy
}

func (p *P) CLIProxyAPIConfig() *config.Config {
	if p.cliproxy_config == nil {
		p.cliproxy_config = &config.Config{
			AuthDir: "auth",
			Debug:   p.Config().Env == "dev",
			Host:    p.Config().HTTP.Host,
			Port:    p.Config().HTTP.Port,
			SDKConfig: config.SDKConfig{
				APIKeys: make([]string, 0),
			},
		}
	}
	return p.cliproxy_config
}

func (p *P) registerAllProviders(ctx context.Context) error {
	cfg := p.CLIProxyAPIConfig()

	cfg.CodexKey = nil
	cfg.ClaudeKey = nil
	cfg.OpenAICompatibility = nil

	providers, err := p.ProviderRepo(ctx).GetProviders(ctx)
	if err != nil {
		return err
	}

	for _, provider := range providers {
		switch aoyorouter.ProviderType(provider.Type) {
		case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
			cfg.CodexKey = append(cfg.CodexKey, config.CodexKey{
				APIKey:  provider.ClientSecret,
				BaseURL: provider.ClientID,
			})

		case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
			cfg.ClaudeKey = append(cfg.ClaudeKey, config.ClaudeKey{
				APIKey:  provider.ClientSecret,
				BaseURL: provider.ClientID,
			})

		case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
			cfg.OpenAICompatibility = append(
				cfg.OpenAICompatibility,
				config.OpenAICompatibility{
					Name:    provider.Name,
					BaseURL: provider.ClientID,
					APIKeyEntries: []config.OpenAICompatibilityAPIKey{
						{APIKey: provider.ClientSecret},
					},
				},
			)
		}
	}
	return nil;

}
