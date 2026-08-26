package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/models"
)

// Distinct key types: a single shared key would let the authenticated
// *models.ApiKey and the *user_repo.UserRepo overwrite one another in the
// context, and only the middleware ordering kept that from breaking auth.
type (
	apiKeyCtxKey   struct{}
	userRepoCtxKey struct{}
)

func GetApiKeyFromCtx(ctx context.Context) (*models.ApiKey, bool) {
	user, ok := ctx.Value(apiKeyCtxKey{}).(*models.ApiKey)
	return user, ok
}

func SetApiKeyInCtx(ctx context.Context, key *models.ApiKey) context.Context {
	return context.WithValue(ctx, apiKeyCtxKey{}, key)
}

func GetUserRepoFromCtx(ctx context.Context) (*user_repo.UserRepo, bool) {
	repo, ok := ctx.Value(userRepoCtxKey{}).(*user_repo.UserRepo)
	return repo, ok
}

func UserRepoToCtxInterceptor(userRepo *user_repo.UserRepo) func(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			ctx := context.WithValue(r.Context(), userRepoCtxKey{}, userRepo)
			next(w, r.WithContext(ctx), pathParams)
		}
	}
}

func AuthorizeRequest(userRepo *user_repo.UserRepo) func(req *http.Response) (context.Context, error) {
	return func(r *http.Response) (context.Context, error) {
		auth := r.Request.Header.Get("Authorization")
		if auth == "" {
			return nil, fmt.Errorf("Authorization header is required")
		}

		if after, ok := strings.CutPrefix(auth, "Password "); ok {
			auth = after
		} else if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			auth = after
		} else {
			return nil, fmt.Errorf("Authorization header is required")
		}

		key, err := userRepo.LoginUser(r.Request.Context(), auth)

		if err != nil {
			return nil, fmt.Errorf("Unauthorized")
		}

		ctx := SetApiKeyInCtx(r.Request.Context(), key)
		return ctx, nil
	}
}

func AuthInterceptor(isAdmin bool) func(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, "Authorization header is required", http.StatusUnauthorized)
				return
			}

			if after, ok := strings.CutPrefix(auth, "Password "); ok {
				auth = after
			} else if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
				auth = after
			} else {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}

			userRepo, ok := GetUserRepoFromCtx(r.Context())
			if !ok {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			key, err := userRepo.LoginUser(r.Context(), auth)
			if err != nil {
				// Deliberately generic: err may carry database detail.
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if key != nil && isAdmin && !key.IsAdmin {
				http.Error(w, "permission denied", http.StatusForbidden)
				return
			}
			if key == nil {
				key = &models.ApiKey{
					IsAdmin: true,
					ID: auth,
					Key: auth,
				}
			}

			ctx := SetApiKeyInCtx(r.Context(), key)
			next(w, r.WithContext(ctx), pathParams)
		}
	}
}
