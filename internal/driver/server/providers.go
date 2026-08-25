package server

import (
	"context"
	"slices"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (a *AoyoRouterService) CreateProvider(ctx context.Context, req *aoyorouter.CreateProviderRequest) (*aoyorouter.CreateProviderResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}

	cfg, err := a.providerVendor.ProviderOAuthConfig(req.GetType())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "provider not supported")
	}
	if cfg.GetOAuthDefinition().Callback {
		return nil, status.Error(codes.InvalidArgument, "use the provider authorization endpoint")
	}

	if req.GetClientId() == "" {
		req.ClientId = cfg.GetOAuthDefinition().DefaultURL
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

func (a *AoyoRouterService) DeleteProvider(ctx context.Context, req *aoyorouter.DeleteProviderRequest) (*aoyorouter.DeleteProviderResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}

	provider, err := a.ProviderRepo.GetProvider(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}

	if err := a.removeProvider(provider); err != nil {
		return nil, err
	}

	if err := a.ProviderRepo.DeleteProvider(ctx, req.GetProviderId()); err != nil {
		return nil, err
	}

	return &aoyorouter.DeleteProviderResponse{Status: "ok"}, nil
}

func (a *AoyoRouterService) GetProvider(ctx context.Context, req *aoyorouter.GetProviderRequest) (*aoyorouter.GetProviderResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		if slices.Contains(requesterKey.RestrictedProviders, req.GetProviderId()) {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
	}
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
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}


	result := make([]*aoyorouter.Provider, 0, len(providers))
	for _, provider := range providers {
		item := providerToProto(provider)
		if requesterKey != nil && !requesterKey.IsAdmin {
			if slices.Contains(requesterKey.RestrictedProviders, provider.ID) {
				continue
			}
		}
		if providerOAuthReady(provider.ClientSecret, provider.Credentials) {
			if quota, err := a.loadQuota(provider, ctx); err == nil {
				item.Quota = quota
			}
		}
		result = append(result, item)
	}

	return &aoyorouter.GetProvidersListResponse{Providers: result}, nil
}

func (a *AoyoRouterService) ReloadProviders(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied, ask admin to reload providers")
	}
	if a.cpapiRestarter == nil {
		return nil, status.Error(codes.Unimplemented, "provider reload is not configured")
	}
	if err := a.cpapiRestarter(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "reload providers: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (a *AoyoRouterService) UpdateProvider(ctx context.Context, req *aoyorouter.UpdateProviderRequest) (*aoyorouter.UpdateProviderResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied, ask admin to update provider")
	}
	
	cfg, err := a.providerVendor.ProviderOAuthConfig(req.GetType())
	if err != nil {
		return nil, err
	}

	if req.GetClientId() == "" && req.GetType() != aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM {
		req.ClientId = cfg.GetOAuthDefinition().DefaultURL
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

	a.removeProviderConfig(a.CPAPIConfig, oldProvider)
	a.addProviderConfig(ctx, a.CPAPIConfig, provider)

	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}

	return &aoyorouter.UpdateProviderResponse{Status: "ok"}, nil
}
