package user_repo

import (
	"context"
	"errors"

	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/config"
)

type UserRepo struct {
	DB         *postgres.DB
	Config     *config.C
	ApiKeyRepo *apikey_repo.ApiKeyRepo
}

func NewUserRepo(db *postgres.DB, config *config.C, apiKeyRepo *apikey_repo.ApiKeyRepo) *UserRepo {
	return &UserRepo{DB: db, Config: config, ApiKeyRepo: apiKeyRepo}
}

func (r *UserRepo) LoginUser(ctx context.Context, password string) error {
	if password != r.Config.InitialPassword {
		if key, err := r.ApiKeyRepo.GetApiKeyByKey(ctx, password); err == nil && key.IsActive && !key.IsDeleted && key.IsAdmin {
			return nil
		}
		return errors.New("invalid password")
	}
	return nil
}
