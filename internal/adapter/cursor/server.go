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

// SetProxy registers an outbound proxy for a token on the bridge. It is a
// no-op when the bridge has not been created yet — previously this
// dereferenced a nil s.server.
func (s *CursorServer) SetProxy(token, proxyURL string) {
	if s.server == nil {
		s.logger.Warn("cursor: SetProxy called before the bridge was created")
		return
	}
	s.server.SetProxy(token, proxyURL)
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
