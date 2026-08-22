package provider

import (
	"context"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
)

func (p *P) startSelectorKeeper(manager *coreauth.Manager, selector *cliproxyapi.RestrictedSelector) {
	ctx, cancel := context.WithCancel(context.Background())
	p.Closer().Add(func() error {
		cancel()
		return nil
	})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if manager.Selector() != coreauth.Selector(selector) {
					manager.SetSelector(selector)
				}
			}
		}
	}()
}
