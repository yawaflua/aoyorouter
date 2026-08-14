package user_repo

import (
	"errors"

	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/config"
)

type UserRepo struct {
	DB     *postgres.DB
	Config *config.C
}

func NewUserRepo(db *postgres.DB, config *config.C) *UserRepo {
	return &UserRepo{DB: db, Config: config}
}

func (r *UserRepo) LoginUser(password string) error {
	if password != r.Config.InitialPassword {
		return errors.New("invalid password")
	}
	return nil
}
