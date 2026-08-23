package provider

import (
	"context"
	"log/slog"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	"github.com/yawaflua/aoyorouter/internal/adapter/cursor"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/apikey_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/usage_entry_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/user_repo"
	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	"github.com/yawaflua/aoyorouter/internal/cache"
	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/internal/config"
	"github.com/yawaflua/aoyorouter/internal/driver/server"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
	"github.com/yawaflua/aoyorouter/pkg/logger"

	cpapi_config "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

type P struct {
	config *config.C
	closer *closer.C
	server *server.AoyoRouterService
	logger *slog.Logger
	db     *postgres.DB

	userRepo       *user_repo.UserRepo
	providerRepo   *provider_repo.ProviderRepo
	apiKeyRepo     *apikey_repo.ApiKeyRepo
	usageEntryRepo *usage_entry_repo.UsageEntryRepo

	cliproxy        *cliproxy.Service
	cliproxy_config *cpapi_config.Config
	usagePlugin     *cliproxyapi.UsagePlugin
	management      *cliproxyapi.Management
	cache           *cache.Cache
	cursor          *cursor.CursorServer
	providerVendor  *providers.ProviderVendor

	warp *warp.Warp
}

func New() *P {
	return &P{}
}

func (p *P) Cache() *cache.Cache {
	if p.cache == nil {
		p.cache = cache.NewCache(p.Logger())
	}

	return p.cache
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
			UserRepo:       p.UserRepo(ctx),
			ProviderRepo:   p.ProviderRepo(ctx),
			ApiKeyRepo:     p.ApiKeyRepo(ctx),
			UsageEntryRepo: p.UsageEntryRepo(ctx),
			CPAPIConfig:    p.CLIProxyAPIConfig(),
			Management:     p.Management(ctx),
			Warp:           p.Warp(ctx),
			Logger:         p.Logger(),
			Cache:          p.Cache(),
			ProviderVendor: p.ProviderVendor(ctx),
			CpapiRestarter: p.RestartCPAPI,
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

func (p *P) UsageEntryRepo(ctx context.Context) *usage_entry_repo.UsageEntryRepo {
	if p.usageEntryRepo == nil {
		p.usageEntryRepo = usage_entry_repo.NewUsageEntryRepo(p.DB(ctx), p.ApiKeyRepo(ctx), p.Logger())
	}

	return p.usageEntryRepo
}

func (p *P) UsagePlugin(ctx context.Context) *cliproxyapi.UsagePlugin {
	if p.usagePlugin == nil {
		p.usagePlugin = cliproxyapi.NewUsagePlugin(p.ApiKeyRepo(ctx), p.UsageEntryRepo(ctx), p.Logger())
	}

	return p.usagePlugin
}

func (p *P) Warp(ctx context.Context) *warp.Warp {
	if p.warp == nil {
		p.warp = warp.New(ctx, p.Logger(), p.Closer(), p.Config())
	}

	return p.warp
}

func (p *P) Management(ctx context.Context) *cliproxyapi.Management {
	if p.management == nil {
		p.management = cliproxyapi.NewManagement(p.Config(), p.Logger())
	}

	return p.management
}

func (p *P) ProviderVendor(ctx context.Context) *providers.ProviderVendor {
	if p.providerVendor == nil {
		p.providerVendor = providers.NewProviderVendor(p.Logger(), p.Warp(ctx), p.Cursor(ctx))
	}

	return p.providerVendor
}
