package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/usage_entry_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	"github.com/yawaflua/aoyorouter/internal/cache"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

const cpapiConfigPath = "config.yaml"

type Dependencies struct {
	UserRepo       *user_repo.UserRepo
	ProviderRepo   *provider_repo.ProviderRepo
	ApiKeyRepo     *apikey_repo.ApiKeyRepo
	UsageEntryRepo *usage_entry_repo.UsageEntryRepo
	CPAPIConfig    *config.Config
	Warp           *warp.Warp
	Management     *cliproxyapi.Management
	Logger         *slog.Logger
	Cache          *cache.Cache
	ProviderVendor *providers.ProviderVendor
	CpapiRestarter func(ctx context.Context) error
}

type AoyoRouterService struct {
	UserRepo       *user_repo.UserRepo
	ProviderRepo   *provider_repo.ProviderRepo
	ApiKeyRepo     *apikey_repo.ApiKeyRepo
	UsageEntryRepo *usage_entry_repo.UsageEntryRepo
	CPAPIConfig    *config.Config
	Management     *cliproxyapi.Management
	configMu       sync.Mutex
	warp           *warp.Warp
	logger         *slog.Logger
	cache          *cache.Cache
	providerVendor *providers.ProviderVendor
	cpapiRestarter func(ctx context.Context) error
	aoyorouter.UnimplementedAoyoRouterServiceServer
}


// mustEmbedUnimplementedAoyoRouterServiceServer implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) mustEmbedUnimplementedAoyoRouterServiceServer() {
	panic("unimplemented")
}

// SignIn implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) SignIn(_ context.Context, req *aoyorouter.SignInRequest) (*aoyorouter.SignInResponse, error) {
	return &aoyorouter.SignInResponse{Status: "ok", AuthToken: req.GetPassword()}, nil
}

func (a *AoyoRouterService) HealthCheck(ctx context.Context, _ *emptypb.Empty) (*aoyorouter.HealthCheckResponse, error) {
	issues := make([]string, 0)

	if _, err := a.ProviderRepo.GetProviders(ctx); err != nil {
		issues = append(issues, "database unreachable: "+err.Error())
	}

	for _, names := range a.warp.Proxies() {
		for name, proxy := range names {
			if _, err := proxy.GetWARPInfo(); err != nil {
				issues = append(issues, fmt.Sprintf("proxy %s unhealthy: %v", name, err))
			}
		}
	}

	if err := a.checkCPAPIAlive(); err != nil {
		issues = append(issues, "cliproxyapi unreachable: "+err.Error())
	} else if providers, err := a.ProviderRepo.GetProviders(ctx); err == nil {
		for _, provider := range providers {
			if provider.Disabled || provider.ClientSecret == "" {
				continue
			}
			if provider.ClientSecret == "oauth:pending" {
				issues = append(issues, fmt.Sprintf("provider %s (%s) authorization is not completed", provider.Name, provider.ID))
			}
		}
	}

	statusText := "ok"
	if len(issues) > 0 {
		statusText = "unhealthy"
	}
	return &aoyorouter.HealthCheckResponse{Status: statusText, Issues: issues}, nil
}

func (a *AoyoRouterService) checkCPAPIAlive() error {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("http://%s:%d/", a.CPAPIConfig.Host, a.CPAPIConfig.Port+1)
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func NewAoyoRouterService(deps Dependencies) *AoyoRouterService {
	if deps.CPAPIConfig == nil {
		panic("server.NewAoyoRouterService: CPAPIConfig is nil")
	}

	return &AoyoRouterService{
		UserRepo: deps.UserRepo, ProviderRepo: deps.ProviderRepo, ApiKeyRepo: deps.ApiKeyRepo, UsageEntryRepo: deps.UsageEntryRepo,
		CPAPIConfig: deps.CPAPIConfig, Management: deps.Management, warp: deps.Warp, logger: deps.Logger, cache: deps.Cache,
		providerVendor: deps.ProviderVendor, cpapiRestarter: deps.CpapiRestarter,
	}
}
