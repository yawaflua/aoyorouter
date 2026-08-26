package provider

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

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
	pbHandler       http.Handler
	selectorCancel  context.CancelFunc
	cpapiCloserSet  bool
	appCtx          context.Context

	warp *warp.Warp

	// Each lazily-initialised dependency gets its own sync.Once. A single
	// shared mutex would deadlock, because the getters call one another
	// (Server -> UserRepo -> DB -> Closer -> Logger -> Config).
	cacheOnce          sync.Once
	configOnce         sync.Once
	closerOnce         sync.Once
	serverOnce         sync.Once
	loggerOnce         sync.Once
	dbOnce             sync.Once
	userRepoOnce       sync.Once
	providerRepoOnce   sync.Once
	apiKeyRepoOnce     sync.Once
	usageEntryRepoOnce sync.Once
	usagePluginOnce    sync.Once
	managementOnce     sync.Once
	cursorOnce         sync.Once
	providerVendorOnce sync.Once
	warpOnce           sync.Once

	// cpapiMu guards the CLIProxyAPI lifecycle fields (cliproxy,
	// cliproxy_config, selectorCancel, cpapiCloserSet, appCtx). Unlike the
	// fields above these are re-assigned at runtime by RestartCPAPI, which
	// runs on an HTTP handler goroutine while request handlers read them.
	cpapiMu sync.Mutex
}

func New() *P {
	return &P{}
}

func (p *P) Cache() *cache.Cache {
	p.cacheOnce.Do(func() {
		p.cache = cache.NewCache(p.Logger())
	})

	return p.cache
}

func (p *P) Config() *config.C {
	p.configOnce.Do(func() {
		p.config = config.MustLoad()
	})

	return p.config
}

func (p *P) Closer() *closer.C {
	p.closerOnce.Do(func() {
		p.closer = closer.New(p.Logger())
	})

	return p.closer
}

func (p *P) Server(ctx context.Context) *server.AoyoRouterService {
	p.serverOnce.Do(func() {
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
	})

	return p.server
}

func (p *P) Logger() *slog.Logger {
	p.loggerOnce.Do(func() {
		p.logger = logger.InitLogger(p.Config().Env)
	})

	return p.logger
}

func (p *P) DB(ctx context.Context) *postgres.DB {
	p.dbOnce.Do(func() {
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
	})

	return p.db
}

func (p *P) UsageEntryRepo(ctx context.Context) *usage_entry_repo.UsageEntryRepo {
	p.usageEntryRepoOnce.Do(func() {
		p.usageEntryRepo = usage_entry_repo.NewUsageEntryRepo(p.DB(ctx), p.ApiKeyRepo(ctx), p.Logger())
	})

	return p.usageEntryRepo
}

func (p *P) UsagePlugin(ctx context.Context) *cliproxyapi.UsagePlugin {
	p.usagePluginOnce.Do(func() {
		p.usagePlugin = cliproxyapi.NewUsagePlugin(p.ApiKeyRepo(ctx), p.UsageEntryRepo(ctx), p.Logger())
	})

	return p.usagePlugin
}

func (p *P) Warp(ctx context.Context) *warp.Warp {
	p.warpOnce.Do(func() {
		p.warp = warp.New(ctx, p.Logger(), p.Closer(), p.Config())
	})

	return p.warp
}

func (p *P) Management(ctx context.Context) *cliproxyapi.Management {
	p.managementOnce.Do(func() {
		p.management = cliproxyapi.NewManagement(p.Config(), p.Logger())
	})

	return p.management
}

func (p *P) ProviderVendor(ctx context.Context) *providers.ProviderVendor {
	p.providerVendorOnce.Do(func() {
		p.providerVendor = providers.NewProviderVendor(p.Logger(), p.Warp(ctx), p.Cursor(ctx))
	})

	return p.providerVendor
}

func (p *P) SetSelectorCancel(cncl context.CancelFunc) {
	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()
	p.selectorCancel = cncl
}

func (p *P) SetAppCtx(ctx context.Context) {
	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()
	p.appCtx = ctx
}

// AppCtx returns the process-wide context. It falls back to context.Background
// so that a restart triggered before Start finished wiring things up does not
// hand a nil context to context.WithCancel.
func (p *P) AppCtx() context.Context {
	p.cpapiMu.Lock()
	defer p.cpapiMu.Unlock()
	if p.appCtx == nil {
		return context.Background()
	}
	return p.appCtx
}

// RunCPAPI starts the embedded CLIProxyAPI service and blocks until it stops.
//
// It deliberately does not surface the Run error to its caller's errgroup:
// RestartCPAPI cancels this context on purpose, so a context.Canceled here is
// routine and must not bring the whole process down.
func (p *P) RunCPAPI(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	p.SetSelectorCancel(cancel)

	if err := p.CLIProxyAPI(runCtx).Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
		p.Logger().Error("cliproxyapi stopped with error", "error", err)
		return err
	}

	p.Logger().Info("cliproxyapi stopped")
	return nil
}
