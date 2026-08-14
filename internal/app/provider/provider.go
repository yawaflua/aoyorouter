package provider

import (
	"context"
	"log/slog"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/internal/config"
	"github.com/yawaflua/aoyorouter/internal/driver/server"
	"github.com/yawaflua/aoyorouter/pkg/logger"

	cpapi_config "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

type P struct {
	config *config.C
	closer *closer.C
	server *server.AoyoRouterService
	logger *slog.Logger
	db     *postgres.DB

	userRepo     *user_repo.UserRepo
	providerRepo *provider_repo.ProviderRepo
	apiKeyRepo   *apikey_repo.ApiKeyRepo

	cliproxy        *cliproxy.Service
	cliproxy_config *cpapi_config.Config
}

func New() *P {
	return &P{}
}

func (p *P) Config() *config.C {
	if p.config == nil {
		p.config = config.MustLoad()
	}

	return p.config
}

func (p *P) Closer() *closer.C {
	if p.closer == nil {
		p.closer = closer.New(p.Logger())
	}

	return p.closer
}

func (p *P) Server(ctx context.Context) *server.AoyoRouterService {
	if p.server == nil {
		p.server = server.NewAoyoRouterService(server.Dependencies{
			UserRepo:     p.UserRepo(ctx),
			ProviderRepo: p.ProviderRepo(ctx),
			ApiKeyRepo:   p.ApiKeyRepo(ctx),
			CPAPIConfig:  p.CLIProxyAPIConfig(),
		})
	}

	return p.server
}

func (p *P) Logger() *slog.Logger {
	if p.logger == nil {
		p.logger = logger.InitLogger(p.Config().Env)
	}

	return p.logger
}

func (p *P) DB(ctx context.Context) *postgres.DB {
	if p.db == nil {
		db, err := postgres.New(ctx, &p.Config().Postgres)
		if err != nil {
			panic(err)
		}

		p.Closer().Add(func() error {
			db.Close()

			p.Logger().Info("database connection closed")
			return nil
		})

		p.db = db
	}

	return p.db
}
