import { record, text } from '../utils'

export interface LiveProxy {
  id: string
  name: string
  url: string
  cloudflareAddress: string
  warpInfo: WarpInfo
}

export interface WarpInfo {
  ip: string
  httpType: string
  serverCity: string
  serverLocation: string
  tls: string
}

export interface Endpoint{
  addr: string
  rtt: string
}

export function parseLiveProxy(value: unknown): LiveProxy {
  const item = record(value)
  const warpInfo = record(item.warp_info ?? item.warpInfo)
  return {
    id: text(item.id),
    name: text(item.name),
    url: text(item.url),
    cloudflareAddress: text(item.cloudflareAddr ?? item.cloudflare_addr),
    warpInfo: {
      ip: text(warpInfo.ip),
      httpType: text(warpInfo.http_type ?? warpInfo.httpType),
      serverCity: text(warpInfo.server_city ?? warpInfo.serverCity),
      serverLocation: text(warpInfo.server_location ?? warpInfo.serverLocation),
      tls: text(warpInfo.tls),
    },
  }
}
export function parseEndpoint(value: unknown): Endpoint {
  const item = record(value)
  return {
    addr: text(item.addr),
    rtt: text(item.rtt),
  }
}
