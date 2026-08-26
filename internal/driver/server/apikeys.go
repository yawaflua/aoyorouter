package server

import (
	"context"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EditApiKey implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) EditApiKey(ctx context.Context, req *aoyorouter.EditApiKeyRequest) (*aoyorouter.EditApiKeyResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}
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

	// The admin flag was read from the request but never assigned, so it could
	// never actually be changed. Assign it — but refuse to let a caller strip
	// admin from their own key, which is a one-way ticket out of the panel.
	if requesterKey != nil && requesterKey.ID == key.ID && key.IsAdmin && !input.GetIsAdmin() {
		return nil, status.Error(codes.InvalidArgument, "cannot remove the admin flag from your own key")
	}
	key.IsAdmin = input.GetIsAdmin()

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

func (a *AoyoRouterService) CreateApiKey(ctx context.Context, req *aoyorouter.CreateApiKeyRequest) (*aoyorouter.CreateApiKeyResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}

	key, err := a.ApiKeyRepo.CreateApiKey(ctx, req.GetName(), req.GetIsAdmin())
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

func (a *AoyoRouterService) DeleteApiKey(ctx context.Context, req *aoyorouter.DeleteApiKeyRequest) (*aoyorouter.DeleteApiKeyResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}

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

func (a *AoyoRouterService) GetApiKeyList(ctx context.Context, _ *aoyorouter.GetApiKeyListRequest) (*aoyorouter.GetApiKeyListResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	result := make([]*aoyorouter.ApiKey, 0)
	if requesterKey == nil || requesterKey.IsAdmin {
		keys, err := a.ApiKeyRepo.GetApiKeys(ctx)
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			result = append(result, &aoyorouter.ApiKey{
				Id: key.ID, Name: key.Name, IsActive: key.IsActive, QuotaSetted: key.QuotaSetted,
				ReservedTokens: key.ReservedTokens, QuotaUsed: key.QuotaTokens,
				QuotaResetAt: timestamppb.New(key.QuotaResetAt), QuotaResetStrategy: quotaResetStrategy(key.QuotaPeriod),
				RestrictedProviders: append([]string(nil), key.RestrictedProviders...),
				RestrictedModels:    append([]string(nil), key.RestrictedModels...),
				IsAdmin:             key.IsAdmin,
			})
		}
	} else {
		keys, err := a.ApiKeyRepo.GetApiKeys(ctx)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if key.ID != requesterKey.ID {
				continue
			}
			result = append(result, &aoyorouter.ApiKey{
				Id: key.ID, Name: key.Name, IsActive: key.IsActive, QuotaSetted: key.QuotaSetted,
				ReservedTokens: key.ReservedTokens, QuotaUsed: key.QuotaTokens,
				QuotaResetAt: timestamppb.New(key.QuotaResetAt), QuotaResetStrategy: quotaResetStrategy(key.QuotaPeriod),
				RestrictedProviders: append([]string(nil), key.RestrictedProviders...),
				RestrictedModels:    append([]string(nil), key.RestrictedModels...),
				IsAdmin:             key.IsAdmin,
			})
		}
	}

	return &aoyorouter.GetApiKeyListResponse{ApiKeys: result}, nil
}

// RecreateApiKey implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) RecreateApiKey(ctx context.Context, req *aoyorouter.RecreateApiKeyRequest) (*aoyorouter.RecreateApiKeyResponse, error) {
	if req.GetApiKeyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "key id should to be provided")
	}
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		if req.GetApiKeyId() != requesterKey.ID {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
	}

	// Read the key being replaced before it is rotated. The previous code
	// removed requesterKey.Key from the config, so an admin rotating someone
	// else's key revoked their own and left the stale key working.
	oldKey, err := a.ApiKeyRepo.GetApiKeyByID(ctx, req.GetApiKeyId())
	if err != nil {
		return nil, err
	}

	apiKey, err := a.ApiKeyRepo.RecreateApiKey(ctx, req.GetApiKeyId())
	if err != nil {
		return nil, err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.CPAPIConfig.SDKConfig.APIKeys = removeString(a.CPAPIConfig.SDKConfig.APIKeys, oldKey.Key)
	a.CPAPIConfig.SDKConfig.APIKeys = append(a.CPAPIConfig.SDKConfig.APIKeys, apiKey)
	if err := config.SaveConfigPreserveComments(cpapiConfigPath, a.CPAPIConfig); err != nil {
		return nil, err
	}
	return &aoyorouter.RecreateApiKeyResponse{
		Status:   "ok",
		ApiKey:   apiKey,
		ApiKeyId: req.GetApiKeyId(),
	}, nil
}
