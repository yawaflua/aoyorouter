package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/yawaflua/aoyorouter/frontend"
	"github.com/yawaflua/aoyorouter/internal/app/provider"
	"github.com/yawaflua/aoyorouter/internal/crons"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"golang.org/x/sync/errgroup"
)

type App struct {
	httpServer *http.Server

	provider *provider.P
}

func New(ctx context.Context) (*App, error) {
	a := App{}

	err := a.initDeps(ctx)
	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (a *App) Start(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return a.runHTTPServer(ctx)
	})

	group.Go(func() error {
		a.provider.ApiKeyRepo(ctx).ProcessQueue(ctx)
		return nil
	})

	group.Go(func() error {
		a.provider.UsageEntryRepo(ctx).ProcessQueue(ctx)
		return nil
	})

	group.Go(func() error {
		return a.provider.CLIProxyAPI(ctx).Run(ctx)
	})

	return nil
}

func (a *App) GracefullyStop(ctx context.Context) error {
	go func() {
		a.provider.Closer().CloseAll()
	}()

	a.provider.Closer().Wait()

	return nil
}

func (a *App) initDeps(ctx context.Context) error {
	deps := []func(context.Context) error{
		a.initProvider,
		a.initHttpServer,
		a.initCPAPI,
		a.initCrons,
	}

	for _, dep := range deps {
		if err := dep(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) initCrons(ctx context.Context) error {
	crons := []*crons.Crons{
		{
			Name:     "quota_resetter",
			Interval: "*/5 * * * *",
			Closer:   a.provider.Closer(),
			Logger:   a.provider.Logger(),
			Handler: func() error {
				apiKeys, err := a.provider.ApiKeyRepo(ctx).GetApiKeys(ctx)
				if err != nil {
					a.provider.Logger().Error("Failed to get api keys", "error", err)
					return err
				}
				for _, key := range apiKeys {
					a.provider.Logger().Debug("Working on api key", "id", key.ID)
					if key.QuotaResetAt.Before(time.Now()) {
						if err := a.provider.ApiKeyRepo(ctx).UpdateApiKeyQuota(ctx, key); err != nil {
							a.provider.Logger().Error("Failed to update api key quota", "id", key.ID, "error", err)
							return err
						}
					}
				}
				return nil
			},
		},
		{
			Name:     "quota_loader",
			Interval: "*/5 * * * *",
			Closer:   a.provider.Closer(),
			Logger:   a.provider.Logger(),
			Handler: func() error {
				providers_db, err := a.provider.ProviderRepo(ctx).GetProviders(ctx)
				if err != nil {
					a.provider.Logger().Error("Failed to get providers", "error", err)
					return err
				}
				for _, provider := range providers_db {
					a.provider.Logger().Debug("Working on provider", "id", provider.ID)
					if provider.Type != aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM {
						if cfg, err := providers.ProviderOAuthConfig(provider.Type); err != nil {
							a.provider.Logger().Error("Failed to get provider oauth config", "id", provider.ID, "error", err)
							return err
						} else {
							a.provider.Logger().Debug("Provider oauth config", "id", provider.ID, "config", cfg)
							quota := cfg.LoadQuota(ctx, provider.Credentials, provider.UseProxy, provider.Proxy)
							if quota != nil {
								a.provider.Logger().Debug("Provider quota", "id", provider.ID, "quota", quota)
								a.provider.Cache().SaveQuota(provider.ID, quota)
							}
						}
					}
				}
				return nil
			},
		},
	}

	for _, cron := range crons {
		if err := cron.Run(); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) initCPAPI(ctx context.Context) error {
	err := a.provider.InitCPAPI(ctx, a.httpServer.Handler)
	if err != nil {
		return err
	}

	if a.provider.CLIProxyAPI(ctx) == nil {
		panic("It should to be panic into CLIProxyAPI func not here")
	}

	return nil
}

func (a *App) initProvider(ctx context.Context) error {
	a.provider = provider.New()
	if a.provider.Config().InitialPassword == "" {
		a.provider.Logger().Warn("no initial password set. Set INITIAL_PASSWORD environment variable to set a custom password")
		token := make([]byte, 20)
		if _, err := rand.Read(token); err != nil {
			panic(err)
		}
		a.provider.Config().InitialPassword = hex.EncodeToString(token)
		a.provider.Logger().Warn("one-time initial password set", "password", a.provider.Config().InitialPassword)
		os.Setenv("INITIAL_PASSWORD", a.provider.Config().InitialPassword)
	}

	return nil
}

func (a *App) initHttpServer(ctx context.Context) error {
	server := a.provider.Server(ctx)

	restServer := runtime.NewServeMux(
		runtime.WithMiddlewares(
			middlewares.UserRepoToCtxInterceptor(a.provider.UserRepo(ctx)),
			middlewares.AuthInterceptor,
			middlewares.LoggerToCtxInterceptor(a.provider.Logger()),
			middlewares.RequestIDInterceptor,
			middlewares.LoggerInterceptor,
		),
	)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", restServer)
	if a.provider.Config().Env != "dev" {
		rootMux.Handle("/", frontend.Handler())
	}
	if err := aoyorouter.RegisterAoyoRouterServiceHandlerServer(context.Background(), restServer, server); err != nil {
		panic(err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.provider.Config().HTTP.Host, a.provider.Config().HTTP.Port+1),
		Handler: rootMux,
	}

	a.httpServer = httpServer

	a.provider.Closer().Add(func() error {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			return err
		}

		a.provider.Logger().Info("http server stopped")

		return nil
	})

	return nil
}

func (a *App) runHTTPServer(ctx context.Context) error {
	a.provider.Logger().Info("http server started", slog.String("address", a.httpServer.Addr))

	if err := a.httpServer.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}
