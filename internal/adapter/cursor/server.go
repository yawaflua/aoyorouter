package cursor

import (
	"context"
	"log/slog"

	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/pkg/cursor"
)

type CursorServer struct {
	server *cursor.Server
	logger *slog.Logger
	warp   *warp.Warp
	closer *closer.C
}

func NewCursorServer(logger *slog.Logger, warp *warp.Warp, closer *closer.C) *CursorServer {
	return &CursorServer{
		logger: logger,
		warp:   warp,
		closer: closer,
	}
}

func (s *CursorServer) GetOrCreateServer(ctx context.Context, cfg cursor.Config) (*cursor.Server, error) {
	if s.server != nil {
		return s.server, nil
	}
	return s.CreateServer(ctx, cfg)
}

func (s *CursorServer) GetProxiesFromServer() *map[string]string {
	return s.server.Proxies()
}

func (s *CursorServer) CreateServer(ctx context.Context, cfg cursor.Config) (*cursor.Server, error) {
	cfg.Port = 0
	server, err := cursor.NewServer(cfg, s.logger)
	if err != nil {
		return nil, err
	}
	s.logger.Info("cursor server created", slog.Int("port", server.Port()))
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			s.logger.Error("cursor server error", slog.Any("error", err))
		}
	}()
	s.closer.Add(func() error {
		return server.Shutdown(ctx)
	})

	s.server = server
	return server, nil
}

func (s *CursorServer) GetServer() *cursor.Server {
	return s.server
}
