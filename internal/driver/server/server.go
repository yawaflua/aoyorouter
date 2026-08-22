package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/usage_entry_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	"github.com/yawaflua/aoyorouter/internal/cache"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	aoyorouter.UnimplementedAoyoRouterServiceServer
}

// UpdateProxy implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) UpdateProxy(ctx context.Context, req *aoyorouter.UpdateProxyRequest) (*aoyorouter.UpdateProxyResponse, error) {
	return nil, status.Error(codes.Code(418), "method is obsolete")
}

// EditApiKey implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) EditApiKey(ctx context.Context, req *aoyorouter.EditApiKeyRequest) (*aoyorouter.EditApiKeyResponse, error) {
	input := req.GetApiKey()
	if input == nil || strings.TrimSpace(input.GetName()) == "" {
		return nil, status.Error(codes.InvalidArgument, "api_key.name is required")
	}

	key, err := a.ApiKeyRepo.GetApiKeyByID(ctx, req.GetApiKeyId())
	if err != nil {
		return nil, err
	}
	period, duration, err := quotaPeriod(input.GetQuotaResetStrategy())
	if err != nil {
		return nil, err
	}
	if input.GetQuotaSetted() && input.GetReservedTokens() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "reserved_tokens must be greater than zero")
	}

	key.Name = strings.TrimSpace(input.GetName())
	key.IsActive = input.GetIsActive()
	key.QuotaSetted = input.GetQuotaSetted()
	key.ReservedTokens = input.GetReservedTokens()
	key.RestrictedProviders = normalizedUniqueStrings(input.GetRestrictedProviders())
	key.RestrictedModels = normalizedUniqueStrings(input.GetRestrictedModels())
	key.QuotaPeriod = period
	if !key.QuotaSetted {
		key.ReservedTokens = 0
		key.QuotaPeriod = models.QuotaPeriodForever
		key.QuotaResetAt = time.Now().UTC()
	} else if period == models.QuotaPeriodForever {
		key.QuotaResetAt = time.Now().UTC()
	} else if input.GetQuotaResetAt() != nil && input.GetQuotaResetAt().IsValid() && input.GetQuotaResetAt().AsTime().After(time.Now()) {
		key.QuotaResetAt = input.GetQuotaResetAt().AsTime().UTC()
	} else {
		key.QuotaResetAt = time.Now().UTC().Add(duration)
	}

	if err := a.ApiKeyRepo.UpdateApiKey(ctx, key); err != nil {
		return nil, err
	}
	return &aoyorouter.EditApiKeyResponse{Status: "ok"}, nil
}

// GetProxies implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProxies(context.Context, *aoyorouter.GetProxiesRequest) (*aoyorouter.GetProxiesResponse, error) {
	proxies := a.warp.Proxies()
	resp := aoyorouter.GetProxiesResponse{}
	resp.AvailableEndpoints = make([]*aoyorouter.ProxyEndpoint, 0, len(a.warp.Endpoints()))
	for endpoint := range a.warp.Endpoints() {
		resp.AvailableEndpoints = append(resp.AvailableEndpoints, &aoyorouter.ProxyEndpoint{
			Addr: endpoint.AddrPort.String(),
			Rtt:  strconv.FormatFloat(endpoint.RTT.Seconds(), 'f', 2, 64),
		})
	}

	for addr, names := range proxies {
		for name, proxy := range names {
			warpInfo, err := proxy.GetWARPInfo()
			if err != nil {
				return nil, err
			}

			protoWarpInfo := aoyorouter.WARPInfo{
				Ip:             warpInfo.IP.String(),
				HttpType:       warpInfo.HTTP,
				ServerCity:     warpInfo.ServerPlace,
				ServerLocation: warpInfo.ServerPlace,
				Tls:            warpInfo.TLS,
			}
			resp.Proxies = append(resp.Proxies, &aoyorouter.Proxy{
				Id:             name,
				Name:           name,
				Url:            proxy.Addr().String(),
				CloudflareAddr: addr,
				WarpInfo:       &protoWarpInfo,
			})
		}
	}
	return &resp, nil
}

// GetProviderLogsByKeyID implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProviderLogsByKeyID(ctx context.Context, req *aoyorouter.GetProviderLogsByKeyIDRequest) (*aoyorouter.GetProviderLogsByKeyIDResponse, error) {
	usage, err := a.UsageEntryRepo.GetUsageEntryByApiKeyID(ctx, uuid.MustParse(req.GetApiKeyId()))
	if err != nil {
		return nil, err
	}
	resp := aoyorouter.GetProviderLogsByKeyIDResponse{}
	for _, v := range usage {
		resp.Logs = append(resp.Logs, &aoyorouter.LogEntry{
			Provider:        v.Provider,
			ApiKeyId:        v.ApiTokenID.String(),
			Latency:         int64(v.Latency),
			InputTokens:     v.InputTokens,
			OutputTokens:    v.OutputTokens,
			TotalTokens:     v.TotalTokens,
			CachedTokens:    v.CachedTokens,
			Model:           v.Model,
			ReasoningEffort: v.Reasoning,
			Failed:          v.Failed,
			Error:           v.Error,
			RequestTime:     timestamppb.New(v.RequestedAt),
			CreatedAt:       timestamppb.New(v.CreatedAt),
		})
		resp.TotalTokens += v.TotalTokens
	}

	return &resp, nil
}

// GetUsageLogs implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetUsageLogs(ctx context.Context, req *aoyorouter.GetUsageLogsRequest) (*aoyorouter.GetUsageLogsResponse, error) {
	usage, err := a.UsageEntryRepo.GetAllUsageEntries(ctx, uint64(req.GetLimit()), uint64(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	resp := aoyorouter.GetUsageLogsResponse{}
	for _, v := range usage {
		resp.Logs = append(resp.Logs, &aoyorouter.LogEntry{
			Provider:        v.Provider,
			ApiKeyId:        v.ApiTokenID.String(),
			Latency:         int64(v.Latency),
			InputTokens:     v.InputTokens,
			OutputTokens:    v.OutputTokens,
			TotalTokens:     v.TotalTokens,
			CachedTokens:    v.CachedTokens,
			Model:           v.Model,
			ReasoningEffort: v.Reasoning,
			Failed:          v.Failed,
			Error:           v.Error,
			RequestTime:     timestamppb.New(v.RequestedAt),
			CreatedAt:       timestamppb.New(v.CreatedAt),
		})
	}

	return &resp, nil
}

// SignIn implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) SignIn(_ context.Context, req *aoyorouter.SignInRequest) (*aoyorouter.SignInResponse, error) {
	return &aoyorouter.SignInResponse{Status: "ok", AuthToken: req.GetPassword()}, nil
}

func (a *AoyoRouterService) CreateApiKey(ctx context.Context, req *aoyorouter.CreateApiKeyRequest) (*aoyorouter.CreateApiKeyResponse, error) {
	key, err := a.ApiKeyRepo.CreateApiKey(ctx, req.GetName())
	if err != nil {
		return nil, err
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.CPAPIConfig.SDKConfig.APIKeys = append(a.CPAPIConfig.SDKConfig.APIKeys, key.Key)
	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}

	return &aoyorouter.CreateApiKeyResponse{Status: "ok", ApiKey: key.Key, ApiKeyId: key.ID}, nil
}

func (a *AoyoRouterService) CreateProvider(ctx context.Context, req *aoyorouter.CreateProviderRequest) (*aoyorouter.CreateProviderResponse, error) {
	switch req.GetType() {
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI, aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC, aoyorouter.ProviderType_PROVIDER_TYPE_KIMI, aoyorouter.ProviderType_PROVIDER_TYPE_GROK, aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return nil, status.Error(codes.InvalidArgument, "use the provider authorization endpoint")
	}

	if err := validateProvider(req.GetName(), req.GetType(), req.GetClientSecret()); err != nil {
		return nil, err
	}

	if req.GetClientId() == "" {
		req.ClientId = "https://api.openai.com/v1"
	}

	provider, err := a.ProviderRepo.CreateProvider(ctx, req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret(), req.GetUseProxy(), req.GetProxy(), req.GetProxy() == "", req.GetPriority())

	if err != nil {
		return nil, err
	}

	if err := a.addProvider(provider, ctx); err != nil {
		return nil, err
	}

	return &aoyorouter.CreateProviderResponse{Status: "ok", ProviderId: provider.ID}, nil
}

func (a *AoyoRouterService) DeleteApiKey(ctx context.Context, req *aoyorouter.DeleteApiKeyRequest) (*aoyorouter.DeleteApiKeyResponse, error) {
	keys, err := a.ApiKeyRepo.GetApiKeys(ctx)
	if err != nil {
		return nil, err
	}

	deletedKey := ""
	for _, key := range keys {
		if key.ID == req.GetApiKeyId() {
			deletedKey = key.Key
			break
		}
	}
	if deletedKey == "" {
		return nil, status.Error(codes.NotFound, "api key not found")
	}

	if err := a.ApiKeyRepo.DeleteApiKey(ctx, req.GetApiKeyId()); err != nil {
		return nil, err
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.CPAPIConfig.SDKConfig.APIKeys = removeString(a.CPAPIConfig.SDKConfig.APIKeys, deletedKey)
	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}

	return &aoyorouter.DeleteApiKeyResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) DeleteProvider(ctx context.Context, req *aoyorouter.DeleteProviderRequest) (*aoyorouter.DeleteProviderResponse, error) {
	provider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return &aoyorouter.DeleteProviderResponse{Status: "ok"}, nil
	}

	if err := a.removeProvider(provider); err != nil {
		return nil, err
	}

	if err := a.ProviderRepo.DeleteProvider(ctx, req.GetProviderId()); err != nil {
		return nil, err
	}

	return &aoyorouter.DeleteProviderResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) GetApiKeyList(ctx context.Context, _ *aoyorouter.GetApiKeyListRequest) (*aoyorouter.GetApiKeyListResponse, error) {
	keys, err := a.ApiKeyRepo.GetApiKeys(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*aoyorouter.ApiKey, 0, len(keys))
	for _, key := range keys {
		result = append(result, &aoyorouter.ApiKey{
			Id: key.ID, Name: key.Name, IsActive: key.IsActive, QuotaSetted: key.QuotaSetted,
			ReservedTokens: key.ReservedTokens, QuotaUsed: key.QuotaTokens,
			QuotaResetAt: timestamppb.New(key.QuotaResetAt), QuotaResetStrategy: quotaResetStrategy(key.QuotaPeriod),
			RestrictedProviders: append([]string(nil), key.RestrictedProviders...),
			RestrictedModels:    append([]string(nil), key.RestrictedModels...),
		})
	}

	return &aoyorouter.GetApiKeyListResponse{ApiKeys: result}, nil
}

func (a *AoyoRouterService) GetProvider(ctx context.Context, req *aoyorouter.GetProviderRequest) (*aoyorouter.GetProviderResponse, error) {
	provider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())

	if err != nil {
		return nil, err
	}

	return &aoyorouter.GetProviderResponse{Provider: providerToProto(provider)}, nil
}

func (a *AoyoRouterService) GetProvidersList(ctx context.Context, _ *aoyorouter.GetProvidersListRequest) (*aoyorouter.GetProvidersListResponse, error) {
	providers, err := a.ProviderRepo.GetProviders(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*aoyorouter.Provider, 0, len(providers))
	for _, provider := range providers {
		item := providerToProto(provider)
		if providerOAuthReady(provider.ClientSecret, provider.Credentials) {
			if quota, err := a.loadQuota(provider, ctx); err == nil {
				item.Quota = quota
			}
		}
		result = append(result, item)
	}

	return &aoyorouter.GetProvidersListResponse{Providers: result}, nil
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

func (a *AoyoRouterService) UpdateProvider(ctx context.Context, req *aoyorouter.UpdateProviderRequest) (*aoyorouter.UpdateProviderResponse, error) {
	if err := validateProvider(req.GetName(), req.GetType(), req.GetClientSecret()); err != nil {
		return nil, err
	}

	oldProvider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}

	provider, err := a.ProviderRepo.UpdateProvider(ctx, req.GetProviderId(), req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret(), req.GetUseProxy(), req.GetProxy(), req.GetIsCloudflare(), req.GetPriority(), req.GetDisabled())
	if err != nil {
		return nil, err
	}
	if len(provider.Credentials) > 0 && a.Management != nil {
		if err := a.Management.ManagementJSON(ctx, "PATCH", "/v0/management/auth-files/status", nil, map[string]any{
			"name":     provider.ID,
			"disabled": provider.Disabled,
		}, nil); err != nil {
			return nil, err
		}
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	removeProviderConfig(a.CPAPIConfig, oldProvider)
	a.addProviderConfig(ctx, a.CPAPIConfig, provider)

	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}

	return &aoyorouter.UpdateProviderResponse{Status: "ok"}, nil
}

func NewAoyoRouterService(deps Dependencies) *AoyoRouterService {
	if deps.CPAPIConfig == nil {
		panic("server.NewAoyoRouterService: CPAPIConfig is nil")
	}

	return &AoyoRouterService{
		UserRepo: deps.UserRepo, ProviderRepo: deps.ProviderRepo, ApiKeyRepo: deps.ApiKeyRepo, UsageEntryRepo: deps.UsageEntryRepo,
		CPAPIConfig: deps.CPAPIConfig, Management: deps.Management, warp: deps.Warp, logger: deps.Logger, cache: deps.Cache,
	}
}
