import { record, text } from '../utils'

export interface LiveProxy {
  id: string
  name: string
  url: string
  cloudflareAddress: string
}

export function parseLiveProxy(value: unknown): LiveProxy {
  const item = record(value)
  return {
    id: text(item.id),
    name: text(item.name),
    url: text(item.url),
    cloudflareAddress: text(item.cloudflareAddr ?? item.cloudflare_addr),
  }
}
