package cliproxyapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
)

type AccessProvider struct {
	apikey_repo *apikey_repo.ApiKeyRepo
}

func NewAccessProvider(apikey_repo *apikey_repo.ApiKeyRepo) *AccessProvider {
	return &AccessProvider{apikey_repo: apikey_repo}
}

// Authenticate implements [access.Provider].
func (a AccessProvider) Authenticate(ctx context.Context, r *http.Request) (*access.Result, *access.AuthError) {
	keys, err := a.apikey_repo.GetApiKeys(ctx)
	if err != nil {
		return nil, &access.AuthError{Message: err.Error()}
	}
	token := strings.TrimSpace(r.Header.Get("x-api-key"))
	
	if token == "" {
	    authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	    token = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}
	
	for _, key := range keys {
		if key.Key == token {
			return &access.Result{
				Provider:  "aoyorouter.AccessProvider",
				Principal: key.ID,
			}, nil
		}
	}
	
	return nil, &access.AuthError{Message: "invalid token"}
}

// Identifier implements [access.Provider].
func (a AccessProvider) Identifier() string {
	return "AccessProvider"
}
