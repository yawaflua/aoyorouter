package server

import (
	"context"
	"strconv"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetProxies implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProxies(context.Context, *aoyorouter.GetProxiesRequest) (*aoyorouter.GetProxiesResponse, error) {
	proxies := a.warp.Proxies()
	resp := aoyorouter.GetProxiesResponse{}
	resp.AvailableEndpoints = make([]*aoyorouter.ProxyEndpoint, 0, len(a.warp.Endpoints()))
	for endpoint := range a.warp.Endpoints() {
		resp.AvailableEndpoints = append(resp.AvailableEndpoints, &aoyorouter.ProxyEndpoint{
			Addr: endpoint.AddrPort.String(),
			Rtt:  strconv.FormatFloat(endpoint.RTT.Seconds(), 'f', 2, 64),
		})
	}

	for addr, names := range proxies {
		for name, proxy := range names {
			warpInfo, err := proxy.GetWARPInfo()
			if err != nil {
				return nil, err
			}

			protoWarpInfo := aoyorouter.WARPInfo{
				Ip:             warpInfo.IP.String(),
				HttpType:       warpInfo.HTTP,
				ServerCity:     warpInfo.ServerPlace,
				ServerLocation: warpInfo.ServerPlace,
				Tls:            warpInfo.TLS,
			}
			resp.Proxies = append(resp.Proxies, &aoyorouter.Proxy{
				Id:             name,
				Name:           name,
				Url:            proxy.Addr().String(),
				CloudflareAddr: addr,
				WarpInfo:       &protoWarpInfo,
			})
		}
	}
	return &resp, nil
}

// UpdateProxy implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) UpdateProxy(ctx context.Context, req *aoyorouter.UpdateProxyRequest) (*aoyorouter.UpdateProxyResponse, error) {
	return nil, status.Error(codes.Code(418), "method is obsolete")
}
