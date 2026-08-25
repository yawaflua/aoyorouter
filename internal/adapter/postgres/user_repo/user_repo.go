package user_repo

import (
	"context"
	"errors"

	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/config"
	"github.com/yawaflua/aoyorouter/internal/models"
)

type UserRepo struct {
	DB         *postgres.DB
	Config     *config.C
	ApiKeyRepo *apikey_repo.ApiKeyRepo
}

func NewUserRepo(db *postgres.DB, config *config.C, apiKeyRepo *apikey_repo.ApiKeyRepo) *UserRepo {
	return &UserRepo{DB: db, Config: config, ApiKeyRepo: apiKeyRepo}
}

func (r *UserRepo) LoginUser(ctx context.Context, password string) (*models.ApiKey, error) {
	if password != r.Config.InitialPassword {
		if key, err := r.ApiKeyRepo.GetApiKeyByKey(ctx, password); err == nil && key.IsActive && !key.IsDeleted {
			return key, nil
		}
		return nil, errors.New("invalid password")
	}
	return nil, nil
}
