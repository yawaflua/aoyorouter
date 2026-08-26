package warp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"net/netip"

	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/internal/config"
	clwarp "github.com/yawaflua/cloudflare-warp/sdk"
)

type Warp struct {
	proxies   map[string]map[string]*clwarp.Proxy
	endpoints map[clwarp.Endpoint]bool
	logger    *slog.Logger
	closer    *closer.C
	mu        sync.Mutex
}

func New(ctx context.Context, logger *slog.Logger, closer *closer.C, config *config.C) *Warp {

	endpointList := make(map[clwarp.Endpoint]bool)
	if config.NotUseCloudflare {
		logger.Warn("aoyorouter will not use cloudflare. endpoints will not be scanned.")
	} else {
		// A failed endpoint scan used to panic, taking the whole process down
		// over what is an optional feature and a network-dependent one at that.
		// Degrade instead: with no endpoints CreateProxy returns nil, which
		// callers already handle.
		identity, err := clwarp.GenerateIdentity("proxy/startup")
		if err != nil {
			logger.Error("warp: failed to generate startup identity, proxies disabled", slog.Any("err", err))
			return newWarp(endpointList, logger, closer)
		}
		endpoints, err := identity.Scan(ctx, clwarp.ScanOptions{
			IPv4:    true,
			IPv6:    true,
			Limit:   config.WarpLimit,
			Timeout: time.Second * 10,
			MaxRTT:  time.Second * 10,
		})
		if err != nil {
			logger.Error("warp: endpoint scan failed, proxies disabled", slog.Any("err", err))
			return newWarp(endpointList, logger, closer)
		}
		for _, endpoint := range endpoints {
			endpointList[endpoint] = false
		}
	}

	return newWarp(endpointList, logger, closer)
}

func newWarp(endpoints map[clwarp.Endpoint]bool, logger *slog.Logger, closer *closer.C) *Warp {
	return &Warp{
		proxies:   make(map[string]map[string]*clwarp.Proxy),
		endpoints: endpoints,
		logger:    logger,
		closer:    closer,
	}
}

// Proxies returns a snapshot of the registered proxies. Handing out the live
// map would let callers read it after the lock is released, which is exactly
// the concurrent map access the mutex exists to prevent.
func (p *Warp) Proxies() map[string]map[string]*clwarp.Proxy {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make(map[string]map[string]*clwarp.Proxy, len(p.proxies))
	for endpoint, byName := range p.proxies {
		inner := make(map[string]*clwarp.Proxy, len(byName))
		for name, proxy := range byName {
			inner[name] = proxy
		}
		out[endpoint] = inner
	}
	return out
}

// Endpoints returns a snapshot, for the same reason as Proxies.
func (p *Warp) Endpoints() map[clwarp.Endpoint]bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make(map[clwarp.Endpoint]bool, len(p.endpoints))
	for endpoint, used := range p.endpoints {
		out[endpoint] = used
	}
	return out
}

func (p *Warp) CreateProxy(ctx context.Context, name string) *clwarp.Proxy {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logger.Info("creating new warp", slog.String("name", name))

	var willUseEndpoint *clwarp.Endpoint
	for endpoint, used := range p.endpoints {
		if used {
			continue
		}
		if _, ok := p.proxies[endpoint.AddrPort.String()]; !ok {
			willUseEndpoint = &endpoint
			break
		}
	}
	if willUseEndpoint == nil {
		p.logger.Error("amount of warps already maxed out. Try set WARP_LIMIT more")
		return nil
	}
	proxy := p.createProxy(ctx, name, 0, willUseEndpoint)
	if proxy == nil {
		return nil
	}

	return proxy
}

func (p *Warp) createProxy(ctx context.Context, name string, port uint16, endpoint *clwarp.Endpoint) *clwarp.Proxy {
	identity, err := clwarp.GenerateIdentity(fmt.Sprintf("proxy/%s/", name))
	if err != nil {
		p.logger.Error("Provider.CreateProxy error when generating identity", slog.Any("err", err))
		return nil
	}

	proxy, err := identity.NewProxy(ctx, clwarp.ProxyConfig{
		Port:         port,
		EndpointIP:   endpoint.AddrPort.Addr(),
		EndpointPort: endpoint.AddrPort.Port(),
		DNS:          netip.AddrFrom4([4]byte{1, 1, 1, 1}),
		Protocol:     clwarp.HTTP,
	})
	if err != nil {
		p.logger.Error("Provider.CreateProxy error when creating proxy", slog.Any("err", err))
		return nil
	}

	// Start before registering. The previous order recorded the proxy and
	// marked the endpoint consumed first, so a failed Start left a dead entry
	// in the map and permanently burned the endpoint.
	if err = proxy.Start(); err != nil {
		p.logger.Error("Provider.CreateProxy error when starting proxy", slog.Any("err", err))
		return nil
	}

	key := endpoint.AddrPort.String()
	if _, ok := p.proxies[key]; !ok {
		p.proxies[key] = make(map[string]*clwarp.Proxy)
	}
	p.proxies[key][name] = proxy
	p.endpoints[*endpoint] = true

	p.closer.Add(func() error {
		if proxy == nil {
			return nil
		}
		proxy.Stop()
		p.logger.Info("proxy done", slog.String("name", name))
		return proxy.WaitContext(ctx)
	})
	return proxy
}
