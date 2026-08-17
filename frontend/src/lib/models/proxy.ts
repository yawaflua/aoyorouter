export interface ProxySettings {
  useProxy: boolean
  proxy: string
}

const SUPPORTED_PROXY_SCHEMES = new Set(['http:', 'https:', 'socks5:', 'socks5h:'])

export function normalizeProxyUrl(value: string): string {
  const proxy = value.trim()
  if (!proxy) return ''
  return proxy.replace(/^socks:\/\//i, 'socks5://')
}

export function validateProxy(settings: ProxySettings): ProxySettings {
  if (!settings.useProxy) return { useProxy: false, proxy: '' }

  const proxy = normalizeProxyUrl(settings.proxy)
  if (!proxy) return { useProxy: true, proxy: '' }

  let parsed: URL
  try {
    parsed = new URL(proxy)
  } catch {
    throw new Error('Enter a valid proxy URL, including its scheme.')
  }

  if (!parsed.hostname || !SUPPORTED_PROXY_SCHEMES.has(parsed.protocol)) {
    throw new Error('Proxy must use http://, https://, socks://, socks5://, or socks5h://.')
  }

  return { useProxy: true, proxy }
}
