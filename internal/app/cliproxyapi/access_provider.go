package cliproxyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	"github.com/yawaflua/aoyorouter/internal/models"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
)

type AccessProvider struct {
	apikey_repo *apikey_repo.ApiKeyRepo
	logger      *slog.Logger
	policies    *AccessPolicyStore
	userRepo    *user_repo.UserRepo
}

func NewAccessProvider(apikey_repo *apikey_repo.ApiKeyRepo, logger *slog.Logger, userRepo *user_repo.UserRepo, policies ...*AccessPolicyStore) *AccessProvider {
	provider := &AccessProvider{apikey_repo: apikey_repo, logger: logger, userRepo: userRepo}
	if len(policies) > 0 {
		provider.policies = policies[0]
	}
	return provider
}

// Authenticate implements [access.Provider].
func (a AccessProvider) Authenticate(ctx context.Context, r *http.Request) (*access.Result, *access.AuthError) {

	key, ok := middlewares.GetApiKeyFromCtx(ctx)
	var token string
	if ok {
		token = strings.TrimSpace(key.Key)
	} else {
		token = strings.TrimSpace(r.Header.Get("x-api-key"))
		if token == "" {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.Contains(authorization, "Password") || strings.Contains(authorization, "Bearer") {
				token = strings.TrimSpace(strings.TrimPrefix(authorization, "Password "))
				if strings.Contains(authorization, "Bearer") {
					token = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
				}
			}
		}
	}

	if key, err := a.userRepo.LoginUser(ctx, token); err != nil {
		return nil, &access.AuthError{Message: err.Error()}
	} else {
		if key == nil {
			return &access.Result{
				Principal: "admin",
			}, nil
		} else if !key.IsActive || key.IsDeleted {
			return nil, &access.AuthError{Message: "invalid token"}
		} else {
			if key.QuotaSetted {
				switch key.QuotaPeriod {

				case models.QuotaPeriodForever:
					if key.QuotaTokens > key.ReservedTokens {
						return nil, &access.AuthError{Message: "quota exceeded"}
					}

				default:
					if key.QuotaTokens >= key.ReservedTokens && key.QuotaResetAt.After(time.Now()) {
						return nil, &access.AuthError{Message: "quota exceeded"}

					} else if key.QuotaTokens >= key.ReservedTokens && key.QuotaResetAt.Before(time.Now()) {

						a.logger.Error("cron job did not update key", slog.String("key", key.ID), slog.Time("quota_reset_at", key.QuotaResetAt))
						a.apikey_repo.UpdateApiKeyQuota(ctx, key)
					}
				}
			}

			if a.policies != nil {
				a.policies.Set(ctx, key.ID, key.RestrictedProviders, key.RestrictedModels)
			}


			return &access.Result{
				Principal: key.ID,
			}, nil
		}
	}
}

// Identifier implements [access.Provider].
func (a AccessProvider) Identifier() string {
	return "AccessProvider"
}

func AccessProviderMiddleware(logger *slog.Logger, providerRepo *provider_repo.ProviderRepo, oauthVendor *providers.ProviderVendor) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp == nil || resp.Request == nil {
			logger.Error("Response or request not found")
			return nil
		}
		key, ok := middlewares.GetApiKeyFromCtx(resp.Request.Context())
		if !ok {
			logger.Error("Key not found in ctx")
			return nil
		}
		if key == nil || key.IsAdmin {
			return nil
		}
		if len(key.RestrictedModels) == 0 && len(key.RestrictedProviders) == 0 {
			return nil
		}

		filtered, err := ExcludeRestrictedModels(resp, logger, key, providerRepo, oauthVendor)
		logger.Debug("excluded restricted models", "filtered", filtered, "error", err)
		if err != nil {
			logger.Error("Failed to exclude restricted models", "error", err)
			return nil
		}
		resp.Body = io.NopCloser(bytes.NewReader(filtered))
		return nil
	}
}

func ExcludeRestrictedModels(resp *http.Response, logger *slog.Logger, key *models.ApiKey, providerRepo *provider_repo.ProviderRepo, oauthVendor *providers.ProviderVendor) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		return nil, err
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		logger.Error("Failed to unmarshal response", "error", err)
		return nil, nil
	}
	if response["object"] != "list" {
		logger.Error("Unexpected response object", "object", response["object"])
		return nil, nil
	}

	data, ok := response["data"].([]any)
	if !ok || len(data) == 0 {
		logger.Error("No data in response")
		return nil, nil
	}
	providersDb, err := providerRepo.GetProviders(resp.Request.Context())
	if err != nil {
		logger.Error("Failed to get providers", "error", err)
		return nil, err
	}
	var expanded []map[string]any
	for _, rawItem := range data {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}

		id, _ := item["id"].(string)
		owned, _ := item["owned_by"].(string)

		for _, provider := range providersDb {
			if cfg, err := oauthVendor.ProviderOAuthConfig(provider.Type); err == nil {
				if owned == cfg.GetOAuthDefinition().Provider {
					if len(key.RestrictedModels) != 0 && !slices.Contains(key.RestrictedModels, id) {
						expanded = append(expanded, copyMap(item))
					}
					if len(key.RestrictedProviders) != 0 && !slices.Contains(key.RestrictedProviders, provider.ID) {
						expanded = append(expanded, copyMap(item))
					}
				}
			}
		}

	}

	result := copyMap(response)
	result["data"] = expanded
	return json.Marshal(result)
}
