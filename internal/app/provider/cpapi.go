package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	config "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	_ "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator/builtin"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

// cpapiConfigPath is the on-disk location of the CLIProxyAPI config. It must
// match the path handed to WithConfigPath below and the one the API server
// writes under configMu.
const cpapiConfigPath = "config.yaml"

func (p *P) InitCPAPI(ctx context.Context, pbHandler http.Handler) error {
	if pbHandler == nil {
		panic("pbHandler is nil")
	}

	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()

	p.pbHandler = pbHandler
	if p.cliproxy != nil {
		return nil
	}
	return p.buildCPAPILocked(ctx)
}

// buildCPAPILocked assembles the CLIProxyAPI service. Callers must hold cpapiMu.
func (p *P) buildCPAPILocked(ctx context.Context) error {
	if p.pbHandler == nil {
		panic("pbHandler is nil")
	}

	p.logger.Info("building CLIProxyAPI from current providers and config")

	accessPolicies := cliproxyapi.NewAccessPolicyStore(p.ProviderRepo(ctx))
	access.RegisterProvider("psql", cliproxyapi.NewAccessProvider(p.ApiKeyRepo(ctx), p.logger, p.UserRepo(ctx), accessPolicies))
	if err := p.registerAllProvidersLocked(ctx); err != nil {
		return fmt.Errorf("registerAllProviders error: %w", err)
	}
	if err := p.registerAllKeysLocked(ctx); err != nil {
		return fmt.Errorf("registerAllKeys error: %w", err)
	}

	credentialStore := cliproxyapi.NewProviderCredentialStore(p.ProviderRepo(ctx), p.ProviderVendor(ctx), p.Logger())
	auth.RegisterTokenStore(credentialStore)

	coreAuthManager := coreauth.NewManager(credentialStore, nil, nil)
	restrictedSelector := cliproxyapi.NewRestrictedSelector(accessPolicies, nil)
	coreAuthManager.SetSelector(restrictedSelector)

	if err := os.Setenv("MANAGEMENT_PASSWORD", p.Config().InitialPassword); err != nil {
		return fmt.Errorf("set CLIProxyAPI management password: %w", err)
	}

	svc, err := cliproxy.NewBuilder().
		WithConfig(p.cliProxyAPIConfigLocked()).
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
		WithConfigPath(cpapiConfigPath).
		Build()
	if err != nil {
		p.logger.Error("failed to create CLIProxyAPI", "error", err)
		return fmt.Errorf("failed to create CLIProxyAPI: %w", err)
	}

	svc.RegisterUsagePlugin(p.UsagePlugin(ctx))
	p.cliproxy = svc

	if !p.cpapiCloserSet {
		p.cpapiCloserSet = true
		p.Closer().Add(func() error {
			p.cancelSelector()
			p.Logger().Info("cpapi shutdown")
			return p.CLIProxyAPI(ctx).Shutdown(ctx)
		})
	}
	return nil
}

// cancelSelector cancels the currently running CLIProxyAPI context, if any.
func (p *P) cancelSelector() {
	p.cpapiMu.Lock()
	cancel := p.selectorCancel
	p.selectorCancel = nil
	p.cpapiMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (p *P) CLIProxyAPI(ctx context.Context) *cliproxy.Service {
	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()

	if p.cliproxy == nil {
		p.Logger().Error("provider.CLIProxyAPI method was called before provider.InitCPAPI")
		panic("provider.CLIProxyAPI method was called before provider.InitCPAPI")
	}
	return p.cliproxy
}

func (p *P) RestartCPAPI(ctx context.Context) error {
	p.Logger().Info("restarting CLIProxyAPI")

	appCtx := p.AppCtx()

	p.cpapiMu.Lock()

	if p.selectorCancel != nil {
		p.selectorCancel()
		p.selectorCancel = nil
	}

	if p.cliproxy != nil {
		if err := p.cliproxy.Shutdown(ctx); err != nil {
			p.Logger().Error("failed to shutdown CLIProxyAPI during restart, forcing restart", "error", err)
		}
		p.cliproxy = nil
	}

	p.cliproxy_config = nil

	if err := p.buildCPAPILocked(ctx); err != nil {
		p.cpapiMu.Unlock()
		return err
	}
	p.cpapiMu.Unlock()

	go func() {
		if err := p.RunCPAPI(appCtx); err != nil {
			p.Logger().Error("cliproxyapi restart failed", "error", err)
		}
	}()
	return nil
}

func (p *P) CLIProxyAPIConfig() *config.Config {
	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()

	return p.cliProxyAPIConfigLocked()
}

func (p *P) cliProxyAPIConfigLocked() *config.Config {
	if p.cliproxy_config == nil {
		p.cliproxy_config = &config.Config{
			AuthDir: "auth",
			Debug:   p.Config().Env == "dev",
			Host:    p.Config().HTTP.Host,
			Port:    p.Config().HTTP.Port + 1,
			SDKConfig: config.SDKConfig{
				APIKeys: make([]string, 0),
			},
			UsageStatisticsEnabled: true,
			RequestRetry:     3,
			MaxRetryInterval: 30,
		}
		p.cliproxy_config.QuotaExceeded.SwitchProject = true
		p.cliproxy_config.QuotaExceeded.SwitchPreviewModel = true
		p.cliproxy_config.QuotaExceeded.AntigravityCredits = true
		p.cliproxy_config.Payload.Override = append(p.cliproxy_config.Payload.Override, config.PayloadRule{
			Params: map[string]any{
				"store":  false,
				"stream": true,
			},
		})
	}
	return p.cliproxy_config
}

// saveCLIProxyAPIConfigLocked persists config.yaml. Callers must hold cpapiMu.
func (p *P) saveCLIProxyAPIConfigLocked() {
	if p.cliproxy_config == nil {
		return
	}
	if err := config.SaveConfigPreserveComments(cpapiConfigPath, p.cliproxy_config); err != nil {
		p.Logger().Error("failed to persist cliproxyapi config", "path", cpapiConfigPath, "error", err)
	}
}

func (p *P) registerAllProvidersLocked(ctx context.Context) error {
	cfg := p.cliProxyAPIConfigLocked()

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
		if provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_CURSOR {
			p.ProviderRepo(ctx).UpdateProvider(ctx, provider.ID, provider.Name, int32(provider.Type), provider.BaseUrl, provider.ClientSecret, provider.UseProxy, provider.Proxy, provider.IsCloudflare, int32(provider.Priority), provider.Disabled)
		}
	}

	p.saveCLIProxyAPIConfigLocked()
	return nil

}

func (p *P) registerAllKeysLocked(ctx context.Context) error {
	cfg := p.cliProxyAPIConfigLocked()

	cfg.APIKeys = nil

	keys, err := p.ApiKeyRepo(ctx).GetApiKeys(ctx)
	if err != nil {
		return err
	}

	for _, key := range keys {
		cfg.APIKeys = append(cfg.APIKeys, key.Key)
	}

	p.saveCLIProxyAPIConfigLocked()
	return nil

}
