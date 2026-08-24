package provider

import (
	"context"

	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
)

func (p *P) UserRepo(ctx context.Context) *user_repo.UserRepo {
	if p.userRepo == nil {
		p.userRepo = user_repo.NewUserRepo(p.DB(ctx), p.Config(), p.ApiKeyRepo(ctx))
	}
	return p.userRepo
}
func (p *P) ProviderRepo(ctx context.Context) *provider_repo.ProviderRepo {
	if p.providerRepo == nil {
		p.providerRepo = provider_repo.NewProviderRepo(p.DB(ctx))
	}
	return p.providerRepo
}

func (p *P) ApiKeyRepo(ctx context.Context) *apikey_repo.ApiKeyRepo {
	if p.apiKeyRepo == nil {
		p.apiKeyRepo = apikey_repo.NewApiKeyRepo(p.DB(ctx), p.Logger())
	}
	return p.apiKeyRepo
}
