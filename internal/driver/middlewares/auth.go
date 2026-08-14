package middlewares

import (
	"context"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
)
type userRepoKey struct{}

func UserRepoToCtxInterceptor(userRepo *user_repo.UserRepo) func(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			ctx := context.WithValue(r.Context(), userRepoKey{}, userRepo)
			next(w, r.WithContext(ctx), pathParams)
		}
	}
}

func AuthInterceptor(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {

		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(auth, "Password ") {
			auth = strings.TrimPrefix(auth, "Password ")
		} else {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}
		

		user_repo := r.Context().Value(userRepoKey{}).(*user_repo.UserRepo)
		err := user_repo.LoginUser(auth)
		
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		next(w, r.WithContext(r.Context()), pathParams)
	}
}