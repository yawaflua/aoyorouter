package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/yawaflua/aoyorouter/frontend"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	"github.com/yawaflua/aoyorouter/internal/app/provider"
	"github.com/yawaflua/aoyorouter/internal/crons"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"golang.org/x/sync/errgroup"
)

const pushEventRetention = 7 * 24 * time.Hour

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

	// Must be set before anything below can trigger a restart, which reads it.
	a.provider.SetAppCtx(ctx)

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
		return a.provider.RunCPAPI(ctx)
	})

	return group.Wait()
}

func (a *App) GracefullyStop(ctx context.Context) error {
	go func() {
		a.provider.Closer().CloseAll()
	}()

	if err := a.provider.Closer().WaitTimeout(30 * time.Second); err != nil {
		a.provider.Logger().Error("graceful shutdown incomplete", "error", err)
	}

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
			Interval: "*/1 * * * *",
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
				subscribed, err := a.provider.QuotaWatcher(ctx).SubscribedProviderIDs(ctx)
				if err != nil {
					a.provider.Logger().Error("Failed to get push subscriptions", "error", err)
					subscribed = nil
				}
				for _, provider := range providers_db {
					a.provider.Logger().Debug("Working on provider", "id", provider.ID)
					if provider.Type != aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM && !provider.Disabled {
						if cfg, err := a.provider.ProviderVendor(ctx).ProviderOAuthConfig(provider.Type); err != nil {
							a.provider.Logger().Error("Failed to get provider oauth config", "id", provider.ID, "error", err)
						} else {
							a.provider.Logger().Debug("Provider oauth config", "id", provider.ID, "config", cfg)
							quota := cfg.LoadQuota(ctx, provider.Credentials, provider.UseProxy, provider.Proxy)
							if quota != nil {
								a.provider.Logger().Debug("Provider quota", "id", provider.ID, "quota", quota)
								a.provider.Cache().SaveQuota(provider.ID, quota)
								if _, ok := subscribed[provider.ID]; ok {
									a.provider.QuotaWatcher(ctx).Observe(ctx, provider.ID, provider.Name, quota)
								}
							}
						}
					}
				}
				return nil
			},
		},
		{
			Name:     "provider_remover",
			Interval: "*/5 * * * *",
			Handler: func() error {
				providers, err := a.provider.ProviderRepo(ctx).GetProviders(ctx)
				if err != nil {
					return err
				}
				for _, provider := range providers {
					if provider.ClientSecret == "oauth:pending" && provider.UpdatedAt.Before(time.Now().Add(-10*time.Minute)) {
						if err := a.provider.ProviderRepo(ctx).DeleteProvider(ctx, provider.ID); err != nil {
							return err
						}
					}
				}
				return nil
			},
			Closer: a.provider.Closer(),
			Logger: a.provider.Logger(),
		},
		{
			Name:     "push_events_pruner",
			Interval: "0 4 * * *",
			Closer:   a.provider.Closer(),
			Logger:   a.provider.Logger(),
			Handler: func() error {
				if err := a.provider.PushRepo(ctx).PruneEvents(ctx, time.Now().Add(-pushEventRetention)); err != nil {
					a.provider.Logger().Error("Failed to prune push events", "error", err)
					return err
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
			middlewares.AuthInterceptor(false),
			middlewares.LoggerToCtxInterceptor(a.provider.Logger()),
			middlewares.RequestIDInterceptor,
			middlewares.LoggerInterceptor,
		),
	)

	if err := aoyorouter.RegisterAoyoRouterServiceHandlerServer(context.Background(), restServer, server); err != nil {
		panic(err)
	}

	target, err := url.Parse(fmt.Sprintf("http://%s:%d", a.provider.Config().HTTP.Host, a.provider.Config().HTTP.Port+1))
	if err != nil {
		return fmt.Errorf("url.Parse error: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		for _, h := range []string{
			"Access-Control-Allow-Origin",
			"Access-Control-Allow-Credentials",
			"Access-Control-Allow-Methods",
			"Access-Control-Allow-Headers",
			"Access-Control-Expose-Headers",
			"Access-Control-Max-Age",
		} {
			resp.Header.Del(h)
		}

		if strings.HasSuffix(resp.Request.URL.Path, "/v1/models") {
			a.provider.Logger().Info("authorizing request", "path", resp.Request.URL.Path)
			ctx, err := middlewares.AuthorizeRequest(a.provider.UserRepo(resp.Request.Context()))(resp)
			if err != nil {
				a.provider.Logger().Error("Failed to authorize request", "error", err)
				rewriteResponse(resp, http.StatusUnauthorized, `{"error":{"message":"Unauthorized","type":"authentication_error"}}`)
				return nil
			}
			resp.Request = resp.Request.WithContext(ctx)
			a.provider.Logger().Info("authorized request", "path", resp.Request.URL.Path)

			if a.provider.Config().EnableEffortPresets {
				err = cliproxyapi.ModifyEffortModelsResponseWithLogger(a.provider.Logger())(resp)
				if err != nil {
					a.provider.Logger().Error("Failed to modify effort models response", "error", err)
					return err
				}
				a.provider.Logger().Info("modified effort models response", "path", resp.Request.URL.Path)
			}
			a.provider.Logger().Info("access provider middleware", "path", resp.Request.URL.Path)
			err = cliproxyapi.AccessProviderMiddleware(a.provider.Logger(), a.provider.ProviderRepo(resp.Request.Context()), a.provider.ProviderVendor(resp.Request.Context()))(resp)
			if err != nil {
				a.provider.Logger().Error("Failed to access provider middleware", "error", err)
				return err
			}
			a.provider.Logger().Info("access provider middleware succeeded", "path", resp.Request.URL.Path)
		} else {

		}
		return nil
	}

	var v1Handler http.Handler = proxy

	rootMux := http.NewServeMux()

	rootMux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if a.provider.Config().Env != "prod" {
			origin := r.Header.Get("Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		restServer.ServeHTTP(w, r)
	})

	if a.provider.Config().Env != "prod" {
		v1Handler = allowAllCORS(v1Handler)

	}
	if a.provider.Config().EnableEffortPresets {
		v1Handler = cliproxyapi.EffortPresetMiddleware(a.provider.Logger(), v1Handler)
	}
	rootMux.Handle("/v1/{path...}", v1Handler)
	rootMux.Handle("/v1beta/{path...}", v1Handler)
	if a.provider.Config().Env != "prod" {
		rootMux.Handle("/v0/{path...}", v1Handler)
	}
	if a.provider.Config().Env == "prod" {
		rootMux.Handle("/", frontend.Handler())
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.provider.Config().HTTP.Host, a.provider.Config().HTTP.Port),
		Handler: rootMux,

		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
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

func rewriteResponse(resp *http.Response, status int, body string) {
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Body = io.NopCloser(strings.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	resp.Header.Del("Content-Encoding")
}

func allowAllCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Expose-Headers", "*")

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if h := r.Header.Get("Access-Control-Request-Headers"); h != "" {
				w.Header().Set("Access-Control-Allow-Headers", h)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "*")
			}
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *App) runHTTPServer(context.Context) error {
	a.provider.Logger().Info("http server started", slog.String("address", a.httpServer.Addr))

	if err := a.httpServer.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}
