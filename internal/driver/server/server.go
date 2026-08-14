package server

import (
	"context"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const cpapiConfigPath = "config.yaml"

type Dependencies struct {
	UserRepo     *user_repo.UserRepo
	ProviderRepo *provider_repo.ProviderRepo
	ApiKeyRepo   *apikey_repo.ApiKeyRepo
	CPAPIConfig  *config.Config
}

type AoyoRouterService struct {
	UserRepo     *user_repo.UserRepo
	ProviderRepo *provider_repo.ProviderRepo
	ApiKeyRepo   *apikey_repo.ApiKeyRepo
	CPAPIConfig  *config.Config
	configMu     sync.Mutex
	aoyorouter.UnimplementedAoyoRouterServiceServer
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
	if err := validateProvider(req.GetName(), req.GetType(), req.GetClientSecret()); err != nil {
		return nil, err
	}

	provider, err := a.ProviderRepo.CreateProvider(ctx, req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret())

	if err != nil {
		return nil, err
	}

	if err := a.addProvider(provider); err != nil {
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
		result = append(result, &aoyorouter.ApiKey{Id: key.ID, Name: key.Name})
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

func (a *AoyoRouterService) GetProviderLogs(context.Context, *aoyorouter.GetProviderLogsRequest) (*aoyorouter.GetProviderLogsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "provider log storage is not configured")
}

func (a *AoyoRouterService) GetProvidersList(ctx context.Context, _ *aoyorouter.GetProvidersListRequest) (*aoyorouter.GetProvidersListResponse, error) {
	providers, err := a.ProviderRepo.GetProviders(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*aoyorouter.Provider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, providerToProto(provider))
	}

	return &aoyorouter.GetProvidersListResponse{Providers: result}, nil
}

func (a *AoyoRouterService) HealthCheck(context.Context, *emptypb.Empty) (*aoyorouter.HealthCheckResponse, error) {
	return &aoyorouter.HealthCheckResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) SignIn(_ context.Context, req *aoyorouter.SignInRequest) (*aoyorouter.SignInResponse, error) {
	return &aoyorouter.SignInResponse{Status: "ok", AuthToken: req.GetPassword()}, nil
}

func (a *AoyoRouterService) UpdateProvider(ctx context.Context, req *aoyorouter.UpdateProviderRequest) (*aoyorouter.UpdateProviderResponse, error) {
	if err := validateProvider(req.GetName(), req.GetType(), req.GetClientSecret()); err != nil {
		return nil, err
	}

	oldProvider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}

	provider, err := a.ProviderRepo.UpdateProvider(ctx, req.GetProviderId(), req.GetName(), int32(req.GetType()), req.GetClientId(), req.GetClientSecret())
	if err != nil {
		return nil, err
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	removeProviderConfig(a.CPAPIConfig, oldProvider)
	addProviderConfig(a.CPAPIConfig, provider)

	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}

	return &aoyorouter.UpdateProviderResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) addProvider(provider *models.Provider) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	addProviderConfig(a.CPAPIConfig, provider)

	return config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
}

func (a *AoyoRouterService) removeProvider(provider *models.Provider) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	removeProviderConfig(a.CPAPIConfig, provider)

	return config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig)
}

func addProviderConfig(cfg *config.Config, provider *models.Provider) {
	switch aoyorouter.ProviderType(provider.Type) {

	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		cfg.CodexKey = append(cfg.CodexKey, config.CodexKey{APIKey: provider.ClientSecret, BaseURL: provider.ClientID})

	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		cfg.ClaudeKey = append(cfg.ClaudeKey, config.ClaudeKey{APIKey: provider.ClientSecret, BaseURL: provider.ClientID})

	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
		cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{Name: provider.Name, BaseURL: provider.ClientID, APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: provider.ClientSecret}}})
	}
}

func removeProviderConfig(cfg *config.Config, provider *models.Provider) {
	switch aoyorouter.ProviderType(provider.Type) {

	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		for index, key := range cfg.CodexKey {
			if key.APIKey == provider.ClientSecret && key.BaseURL == provider.ClientID {
				cfg.CodexKey = append(cfg.CodexKey[:index], cfg.CodexKey[index+1:]...)
				return
			}
		}

	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		for index, key := range cfg.ClaudeKey {
			if key.APIKey == provider.ClientSecret && key.BaseURL == provider.ClientID {
				cfg.ClaudeKey = append(cfg.ClaudeKey[:index], cfg.ClaudeKey[index+1:]...)
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

func validateProvider(name string, providerType aoyorouter.ProviderType, secret string) error {
	if name == "" || secret == "" {
		return status.Error(codes.InvalidArgument, "name and client_secret are required")
	}

	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_UNSPECIFIED:
		return status.Error(codes.InvalidArgument, "unsupported provider type")

	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM, aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI, aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return nil

	default:
		return status.Error(codes.InvalidArgument, "unsupported provider type")
	}
}

func providerToProto(provider *models.Provider) *aoyorouter.Provider {
	return &aoyorouter.Provider{Id: provider.ID, Name: provider.Name, Type: aoyorouter.ProviderType(provider.Type), ClientId: provider.ClientID, ClientSecret: provider.ClientSecret}
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
	return &AoyoRouterService{UserRepo: deps.UserRepo, ProviderRepo: deps.ProviderRepo, ApiKeyRepo: deps.ApiKeyRepo, CPAPIConfig: deps.CPAPIConfig}
}
