package cliproxyapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
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

	token := strings.TrimSpace(r.Header.Get("x-api-key"))
	
	if token == "" {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.Contains(authorization, "Password") {
			token = strings.TrimSpace(strings.TrimPrefix(authorization, "Password "))
			if err := a.userRepo.LoginUser(token); err != nil {
				return nil, &access.AuthError{Message: err.Error()}
			} else {
				return &access.Result{
					Principal: "admin",
				}, nil
			}

		}
		token = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}

	key, err := a.apikey_repo.GetApiKeyByKey(ctx, token)
	if err != nil {
		return nil, &access.AuthError{Message: err.Error()}
	}
	if key == nil || !key.IsActive {
		return nil, &access.AuthError{Message: "invalid token"}
	}
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

// Identifier implements [access.Provider].
func (a AccessProvider) Identifier() string {
	return "AccessProvider"
}
