package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	config "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	_ "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator/builtin"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

func (p *P) InitCPAPI(ctx context.Context, pbHandler http.Handler) error {
	if p.cliproxy != nil {
		return nil
	}

	if pbHandler == nil {
		panic("pbHandler is nil")
	}

	accessPolicies := cliproxyapi.NewAccessPolicyStore(p.ProviderRepo(ctx))
	access.RegisterProvider("psql", cliproxyapi.NewAccessProvider(p.ApiKeyRepo(ctx), p.logger, p.UserRepo(ctx), accessPolicies))
	err := p.registerAllProviders(ctx)
	if err != nil {
		return fmt.Errorf("registerAllProviders error: %w", err)
	}

	err = p.registerAllKeys(ctx)
	if err != nil {
		return fmt.Errorf("registerAllKeys error: %w", err)
	}

	target, err := url.Parse(fmt.Sprintf("http://%s:%d", p.Config().HTTP.Host, p.Config().HTTP.Port+1))
	if err != nil {
		return fmt.Errorf("url.Parse error: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}

	credentialStore := cliproxyapi.NewProviderCredentialStore(p.ProviderRepo(ctx), p.ProviderVendor(ctx))
	auth.RegisterTokenStore(credentialStore)

	coreAuthManager := coreauth.NewManager(credentialStore, nil, nil)
	restrictedSelector := cliproxyapi.NewRestrictedSelector(accessPolicies, nil)
	coreAuthManager.SetSelector(restrictedSelector)
	
	p.startSelectorKeeper(coreAuthManager, restrictedSelector)
	if err := os.Setenv("MANAGEMENT_PASSWORD", p.Config().InitialPassword); err != nil {
		return fmt.Errorf("set CLIProxyAPI management password: %w", err)
	}
	cliproxy, err := cliproxy.NewBuilder().
		WithConfig(p.CLIProxyAPIConfig()).
		WithCoreAuthManager(coreAuthManager).
		WithAuthManager(auth.NewManager(credentialStore, auth.NewAntigravityAuthenticator(), auth.NewClaudeAuthenticator(), auth.NewCodexAuthenticator(), auth.NewKimiAuthenticator(), auth.NewXAIAuthenticator())).
		WithPostAuthHook(func(ctx context.Context, record *coreauth.Auth) error {
			info := coreauth.GetRequestInfo(ctx)
			if info == nil {
				return nil
			}
			providerID := strings.TrimSpace(info.Query.Get("provider_id"))
			if providerID == "" {
				return nil
			}
			record.ID = providerID
			record.FileName = ""
			if record.Attributes == nil {
				record.Attributes = make(map[string]string)
			}
			record.Attributes[coreauth.AttributeSourceBackend] = coreauth.AuthSourcePostgres
			return nil
		}).
		WithAPIKeyClientProvider(cliproxy.NewAPIKeyClientProvider()).
		WithLocalManagementPassword(p.Config().InitialPassword).
		WithServerOptions(api.WithEngineConfigurator(func(e *gin.Engine) {
			e.Any("/dashboard", func(c *gin.Context) {
				proxy.ServeHTTP(c.Writer, c.Request)
			})
			e.Any("/assets/*path", func(c *gin.Context) {
				proxy.ServeHTTP(c.Writer, c.Request)
			})
			e.Any("/favicon.svg", func(c *gin.Context) {
				proxy.ServeHTTP(c.Writer, c.Request)
			})
			e.Any("/api/aoyo/v1/*path", func(c *gin.Context) {
				origin := c.GetHeader("Origin")
				if origin != "" {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Vary", "Origin")
				}
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key")

				if c.Request.Method == http.MethodOptions {
					c.Status(http.StatusNoContent)
					return
				}

				proxy.ServeHTTP(c.Writer, c.Request)
			})
		})).
		WithConfigPath("config.yaml").
		Build()
	p.cliproxy.RegisterUsagePlugin(p.UsagePlugin(ctx))
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
			UsageStatisticsEnabled: true,
		}

		p.cliproxy_config.Payload.Override = append(p.cliproxy_config.Payload.Override, config.PayloadRule{
			Models: []config.PayloadModelRule{
				{
					Name:     "gpt-*",
					Protocol: "codex",
				},
			},
			Params: map[string]any{
				"store":  false,
				"stream": true,
			},
		})
	}
	return p.cliproxy_config
}

func (p *P) registerAllProviders(ctx context.Context) error {
	cfg := p.CLIProxyAPIConfig()

	cfg.CodexKey = nil
	cfg.XAIKey = nil
	cfg.ClaudeKey = nil
	cfg.OpenAICompatibility = nil

	providers, err := p.ProviderRepo(ctx).GetProviders(ctx)
	if err != nil {
		return err
	}

	for _, provider := range providers {
		if provider.Disabled {
			continue
		}
		if provider.UseProxy && provider.IsCloudflare {
			proxy := p.Warp(ctx).CreateProxy(ctx, provider.ID)
			if proxy != nil {
				provider.Proxy = "http://" + proxy.Addr().String()
			}
			p.ProviderRepo(ctx).UpdateProxy(ctx, provider.ID, provider.Proxy, true, true)
		}
		if conf, err := p.ProviderVendor(ctx).ProviderOAuthConfig(aoyorouter.ProviderType(provider.Type)); err == nil {
			conf.AddProviderConfig(ctx, cfg, provider)
		}
	}
	return nil

}

func (p *P) registerAllKeys(ctx context.Context) error {
	cfg := p.CLIProxyAPIConfig()

	cfg.APIKeys = nil

	keys, err := p.ApiKeyRepo(ctx).GetApiKeys(ctx)
	if err != nil {
		return err
	}

	for _, key := range keys {
		cfg.APIKeys = append(cfg.APIKeys, key.Key)
	}
	return nil

}
