package server

import (
	"context"
	"log/slog"
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
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const cpapiConfigPath = "config.yaml"

type Dependencies struct {
	UserRepo                *user_repo.UserRepo
	ProviderRepo            *provider_repo.ProviderRepo
	ApiKeyRepo              *apikey_repo.ApiKeyRepo
	UsageEntryRepo          *usage_entry_repo.UsageEntryRepo
	CPAPIConfig             *config.Config
	CodexOAuth              *codexOAuthStore
	Warp                    *warp.Warp
	CPAPIManagementURL      string
	CPAPIManagementPassword string
	Logger                  *slog.Logger
}

type AoyoRouterService struct {
	UserRepo                *user_repo.UserRepo
	ProviderRepo            *provider_repo.ProviderRepo
	ApiKeyRepo              *apikey_repo.ApiKeyRepo
	UsageEntryRepo          *usage_entry_repo.UsageEntryRepo
	CPAPIConfig             *config.Config
	CodexOAuth              *codexOAuthStore
	ProviderOAuth           *providerOAuthStore
	CPAPIManagementURL      string
	CPAPIManagementPassword string
	configMu                sync.Mutex
	warp                    *warp.Warp
	logger                  *slog.Logger
	aoyorouter.UnimplementedAoyoRouterServiceServer
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

func quotaPeriod(strategy aoyorouter.QuotaResetStrategy) (models.QuotaPeriod, time.Duration, error) {
	switch strategy {
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES:
		return models.QuotaPeriodMinute, time.Minute, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY:
		return models.QuotaPeriodHour, time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY:
		return models.QuotaPeriodDay, 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY:
		return models.QuotaPeriodWeek, 7 * 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY:
		return models.QuotaPeriodMonth, 30 * 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER:
		return models.QuotaPeriodForever, 0, nil
	default:
		return "", 0, status.Error(codes.InvalidArgument, "unsupported quota_reset_strategy")
	}
}

// GetProxies implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProxies(context.Context, *aoyorouter.GetProxiesRequest) (*aoyorouter.GetProxiesResponse, error) {
	proxies := a.warp.Proxies()
	resp := aoyorouter.GetProxiesResponse{}
	for addr, names := range proxies {
		for name, proxy := range names {
			resp.Proxies = append(resp.Proxies, &aoyorouter.ProxyProxy{
				Id:             name,
				Name:           name,
				Url:            proxy.Addr().String(),
				CloudflareAddr: addr,
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

	provider, err := a.ProviderRepo.CreateProvider(ctx, req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret(), req.GetUseProxy(), req.GetProxy())

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

	if err := a.ProviderRepo.DeleteProvider(ctx, req.GetProviderId()); err != nil {
		return nil, err
	}

	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return &aoyorouter.DeleteProviderResponse{Status: "ok"}, nil
	}

	if err := a.removeProvider(provider); err != nil {
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
		})
	}

	return &aoyorouter.GetApiKeyListResponse{ApiKeys: result}, nil
}

func quotaResetStrategy(period models.QuotaPeriod) aoyorouter.QuotaResetStrategy {
	switch period {
	case models.QuotaPeriodMinute:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES
	case models.QuotaPeriodHour:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY
	case models.QuotaPeriodDay:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY
	case models.QuotaPeriodWeek:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY
	case models.QuotaPeriodMonth:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY
	default:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER
	}
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
			switch aoyorouter.ProviderType(provider.Type) {
			case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
				item.Quota = loadCodexQuota(ctx, provider.Credentials)
			case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
				item.Quota = loadKimiQuota(ctx, provider.Credentials)
			case aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
				item.Quota = loadAntigravityQuota(ctx, provider.Credentials)
			}
		}
		result = append(result, item)
	}

	return &aoyorouter.GetProvidersListResponse{Providers: result}, nil
}

func providerOAuthReady(clientSecret string, credentials map[string]any) bool {
	if !strings.HasPrefix(clientSecret, "oauth:") {
		return false
	}
	if clientSecret != "oauth:pending" {
		return true
	}
	return providerCredentialsCompleted(credentials)
}

func (a *AoyoRouterService) HealthCheck(context.Context, *emptypb.Empty) (*aoyorouter.HealthCheckResponse, error) {
	return &aoyorouter.HealthCheckResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) UpdateProvider(ctx context.Context, req *aoyorouter.UpdateProviderRequest) (*aoyorouter.UpdateProviderResponse, error) {
	if err := validateProvider(req.GetName(), req.GetType(), req.GetClientSecret()); err != nil {
		return nil, err
	}

	oldProvider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}

	provider, err := a.ProviderRepo.UpdateProvider(ctx, req.GetProviderId(), req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret(), req.GetUseProxy(), req.GetProxy())
	if err != nil {
		return nil, err
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

func validateProvider(name string, providerType aoyorouter.ProviderType, secret string) error {
	if name == "" || secret == "" {
		return status.Error(codes.InvalidArgument, "name and client_secret are required")
	}

	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_UNSPECIFIED:
		return status.Error(codes.InvalidArgument, "unsupported provider type")

	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM, aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI, aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC, aoyorouter.ProviderType_PROVIDER_TYPE_KIMI, aoyorouter.ProviderType_PROVIDER_TYPE_GROK, aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return nil

	default:
		return status.Error(codes.InvalidArgument, "unsupported provider type")
	}
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func NewAoyoRouterService(deps Dependencies) *AoyoRouterService {
	if deps.CPAPIConfig == nil {
		panic("server.NewAoyoRouterService: CPAPIConfig is nil")
	}

	if deps.CodexOAuth == nil {
		deps.CodexOAuth = newCodexOAuthStore()
	}
	if deps.CPAPIManagementURL == "" || deps.CPAPIManagementPassword == "" {
		panic("server.NewAoyoRouterService: CLIProxyAPI management connection is not configured")
	}

	return &AoyoRouterService{
		UserRepo: deps.UserRepo, ProviderRepo: deps.ProviderRepo, ApiKeyRepo: deps.ApiKeyRepo, UsageEntryRepo: deps.UsageEntryRepo,
		CPAPIConfig: deps.CPAPIConfig, CodexOAuth: deps.CodexOAuth,
		ProviderOAuth: newProviderOAuthStore(), CPAPIManagementURL: deps.CPAPIManagementURL,
		CPAPIManagementPassword: deps.CPAPIManagementPassword, warp: deps.Warp, logger: deps.Logger,
	}
}
