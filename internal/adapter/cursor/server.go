package cursor

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"

	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/pkg/cursor"
)

type CursorServer struct {
	servers []*cursor.Server
	logger  *slog.Logger
	warp    *warp.Warp
	closer  *closer.C
}

func NewCursorServer(logger *slog.Logger, warp *warp.Warp, closer *closer.C) *CursorServer {
	return &CursorServer{
		logger:  logger,
		servers: make([]*cursor.Server, 0),
		warp:    warp,
		closer:  closer,
	}
}

func (s *CursorServer) CreateServer(ctx context.Context, cfg cursor.Config, useProxy bool) (*cursor.Server, error) {
	cfg.Port = rand.Intn(65535)
	if useProxy {
		cfg.ProxyURL = "http://" + s.warp.CreateProxy(ctx, fmt.Sprintf("cursor-%d", cfg.Port)).Addr().String()
	}
	s.logger.Info("creating cursor server", slog.Int("port", cfg.Port))

	server, err := cursor.NewServer(cfg, s.logger)
	if err != nil {
		return nil, err
	}
	s.logger.Info("cursor server created", slog.Int("port", cfg.Port))
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			s.logger.Error("cursor server error", err)
		}
	}()
	s.closer.Add(func() error {
		return server.Shutdown(ctx)
	})

	s.servers = append(s.servers, server)
	return server, nil
}

func (s *CursorServer) GetServers() []*cursor.Server {
	return s.servers
}
