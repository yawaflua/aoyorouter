package server

import (
	"context"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
