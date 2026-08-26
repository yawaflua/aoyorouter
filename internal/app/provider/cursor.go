package provider

import (
	"context"

	cursor_adapter "github.com/yawaflua/aoyorouter/internal/adapter/cursor"
)

func (p *P) Cursor(ctx context.Context) *cursor_adapter.CursorServer {
	p.cursorOnce.Do(func() {
		p.cursor = cursor_adapter.NewCursorServer(p.Logger(), p.Warp(ctx), p.Closer())
	})
	return p.cursor
}
