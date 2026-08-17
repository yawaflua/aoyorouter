package warp

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"math/rand"
	"net/netip"

	clwarp "github.com/shahradelahi/cloudflare-warp/sdk"
	"github.com/yawaflua/aoyorouter/internal/closer"
	"github.com/yawaflua/aoyorouter/internal/config"
)

type Warp struct {
	proxies   map[string]map[string]*clwarp.Proxy
	endpoints map[clwarp.Endpoint]bool
	logger    *slog.Logger
	closer    *closer.C
}

func New(ctx context.Context, logger *slog.Logger, closer *closer.C, config *config.C) *Warp {
	identity, err := clwarp.GenerateIdentity("proxy/startup")
	if err != nil {
		panic(err)
	}
	endpoints, err := identity.Scan(ctx, clwarp.ScanOptions{
		IPv4:    true,
		IPv6:    true,
		Limit:   config.WarpLimit,
		Timeout: time.Second * 10,
		MaxRTT:  time.Second * 10,
	})
	if err != nil {
		panic(err)
	}
	endpointList := make(map[clwarp.Endpoint]bool)
	for _, endpoint := range endpoints {
		endpointList[endpoint] = false
	}

	return &Warp{
		proxies:   make(map[string]map[string]*clwarp.Proxy),
		endpoints: endpointList,
		logger:    logger,
		closer:    closer,
	}
}

func (p *Warp) Proxies() map[string]map[string]*clwarp.Proxy {
	fmt.Println(len(p.proxies))
	return p.proxies
}

func (p *Warp) CreateProxy(ctx context.Context, name string) *clwarp.Proxy {
	p.logger.Info("creating new warp", slog.String("name", name))
	identity, err := clwarp.GenerateIdentity(fmt.Sprintf("proxy/%s/", name))
	if err != nil {
		p.logger.Error("Provider.CreateProxy error when generating identity", slog.Any("err", err))
		return nil
	}
	var willUseEndpoint clwarp.Endpoint
	for endpoint, used := range p.endpoints {
		if used {
			continue
		}
		if _, ok := p.proxies[endpoint.AddrPort.String()]; !ok {
			willUseEndpoint = endpoint
			break
		}
	}
	if willUseEndpoint.AddrPort.String() == "" {
		p.logger.Error("amount of warps already maxed out. Try set WARP_LIMIT more")
		return nil
	}
	proxy, err := identity.NewProxy(ctx, clwarp.ProxyConfig{
		Port:         uint16(rand.Intn(65535)),
		EndpointIP:   willUseEndpoint.AddrPort.Addr(),
		EndpointPort: willUseEndpoint.AddrPort.Port(),
		DNS:          netip.AddrFrom4([4]byte{1, 1, 1, 1}),
		Protocol:     clwarp.HTTP,
	})

	if err != nil {
		p.logger.Error("Provider.CreateProxy error when creating proxy", slog.Any("err", err))
		return nil
	}

	if _, ok := p.proxies[willUseEndpoint.AddrPort.String()]; !ok {
		p.proxies[willUseEndpoint.AddrPort.String()] = make(map[string]*clwarp.Proxy)
	}
	p.proxies[willUseEndpoint.AddrPort.String()][name] = proxy

	p.closer.Add(func() error {
		proxy.Stop()
		fmt.Println("stopping proxy", name)
		return proxy.WaitContext(ctx)
	})

	go func() {
		<-proxy.Done()
		fmt.Println("proxy done", name)
		delete(p.proxies[willUseEndpoint.AddrPort.String()], name)
	}()
	return proxy
}
