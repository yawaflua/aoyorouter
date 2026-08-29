import { ApiError, errorMessage } from './models/apierror'
import { parseApiKey, type ApiKey, type ApiKeyUsage, type CreatedApiKey, type UpdateApiKeyInput } from './models/apikey'
import {
  parseAuthorizationStatus,
  type ProviderAuthorization,
  type ProviderAuthorizationStatus,
} from './models/authorization'
import { parseLogEntry, type LogEntry } from './models/logentry'
import { parseErrorLog, type ErrorLog } from './models/errorlog'
import { parseEndpoint, parseLiveProxy, type Endpoint, type LiveProxy } from './models/liveproxy'
import {
  parseProvider,
  parseProviderModel,
  type Provider,
  type ProviderConnectionInput,
  type ProviderModel,
  type ProviderType,
  type UpdateProviderInput,
} from './models/providers'
import { quotaSubject, type PushSubscriptionPayload } from './push'
import { record, text, type JsonRecord } from './utils'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export interface NotificationEvent {
  id: number
  subject: string
  title: string
  body: string
  tag: string
  providerId: string
  url: string
  createdAt: string
}

export interface NotificationEventPage {
  events: NotificationEvent[]
  lastId: number
}

function parseNotificationEvent(value: unknown): NotificationEvent {
  const item = record(value)
  const id = Number(item.id ?? 0)
  return {
    id: Number.isFinite(id) ? id : 0,
    subject: text(item.subject),
    title: text(item.title),
    body: text(item.body),
    tag: text(item.tag),
    providerId: text(item.providerId ?? item.provider_id),
    url: text(item.url),
    createdAt: text(item.createdAt ?? item.created_at),
  }
}

export class ApiClient {
  constructor(private readonly password: string) {}

  private async request(path: string, init: RequestInit = {}): Promise<JsonRecord> {
    let response: Response
    try {
      response = await fetch(`${API_BASE_URL}${path}`, {
        ...init,
        headers: {
          Accept: 'application/json',
          Authorization: `Password ${this.password}`,
          ...(init.body ? { 'Content-Type': 'application/json' } : {}),
          ...init.headers,
        },
      })
    } catch {
      throw new ApiError('Could not reach the server. Check the address and your connection.', 0)
    }

    if (!response.ok) throw new ApiError(await errorMessage(response), response.status)
    if (response.status === 204) return {}

    const body = await response.text()
    if (!body) return {}
    try {
      return record(JSON.parse(body))
    } catch {
      // A 200 with a non-JSON body (an HTML error page from a proxy in front
      // of us, say) used to surface as a raw SyntaxError the UI cannot render.
      throw new ApiError('The server returned a malformed response.', response.status)
    }
  }

  async getModels(): Promise<ProviderModel[]> {
    const response = await this.request('/v1/models')
    const items = response.data
    return Array.isArray(items) ? items.map(parseProviderModel) : []
  }

  async signIn(): Promise<void> {
    await this.request('/api/aoyo/v1/signin', {
      method: 'POST',
      body: JSON.stringify({ password: this.password }),
    })
  }

  async getApiKeys(): Promise<ApiKey[]> {
    const response = await this.request('/api/aoyo/v1/api-keys')
    const items = response.apiKeys ?? response.api_keys
    return Array.isArray(items) ? items.map(parseApiKey) : []
  }

  async createApiKey(name: string, isAdmin: boolean): Promise<CreatedApiKey> {
    const response = await this.request('/api/aoyo/v1/api-keys', {
      method: 'POST',
      body: JSON.stringify({ name, isAdmin: isAdmin }),
    })
    return {
      id: text(response.apiKeyId ?? response.api_key_id),
      value: text(response.apiKey ?? response.api_key),
    }
  }

  async deleteApiKey(id: string): Promise<void> {
    await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  async recreateApiKey(id: string): Promise<CreatedApiKey> {
    const response = await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(id)}/recreate`, {
      method: 'POST',
      body: JSON.stringify({}),
    })
    return {
      id: text(response.apiKeyId ?? response.api_key_id),
      value: text(response.apiKey ?? response.api_key),
    }
  }

  async updateApiKey(input: UpdateApiKeyInput): Promise<void> {
    await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(input.id)}`, {
      method: 'PATCH',
      body: JSON.stringify({
        apiKey: {
          id: input.id,
          name: input.name,
          isAdmin: input.isAdmin,
          isActive: input.isActive,
          quotaSetted: input.quotaSet,
          reservedTokens: String(input.reservedTokens),
          quotaResetAt: input.quotaResetAt || undefined,
          quotaResetStrategy: input.quotaResetStrategy,
          restrictedProviders: input.restrictedProviders,
          restrictedModels: input.restrictedModels,
        },
      }),
    })
  }

  async getProviders(): Promise<Provider[]> {
    const response = await this.request('/api/aoyo/v1/providers')
    return Array.isArray(response.providers) ? response.providers.map(parseProvider) : []
  }

  async reloadProviders(): Promise<void> {
    await this.request('/api/aoyo/v1/providers/reload', { method: 'POST', body: JSON.stringify({}) })
  }

  async getProxies(): Promise<{ resp_proxies: LiveProxy[], availableEndpoints: Endpoint[] }> {
    const response = await this.request('/api/aoyo/v1/proxies')
    let resp_proxies: LiveProxy[] = []
    let availableEndpoints: Endpoint[] = []
    if (Array.isArray(response.proxies)) {
      resp_proxies = response.proxies.map(parseLiveProxy)
    }
    if (Array.isArray(response.availableEndpoints)) {
      availableEndpoints = response.availableEndpoints.map(parseEndpoint)
    }
    return { resp_proxies, availableEndpoints }
  }

  async createProvider(input: ProviderConnectionInput): Promise<string> {
    const response = await this.request('/api/aoyo/v1/providers', {
      method: 'POST',
      body: JSON.stringify({
        name: input.name,
        type: input.type,
        clientId: input.customUrl,
        clientSecret: input.authorizationData,
        useProxy: input.useProxy,
        proxy: input.proxy,
        priority: input.priority,
      }),
    })
    return text(response.providerId ?? response.provider_id)
  }

  async updateProxy(input: { id: string, cloudflareEndpoint: string, newEndpoint: string }): Promise<LiveProxy> {
    const response = await this.request(`/api/aoyo/v1/proxies/${encodeURIComponent(input.id)}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
    })
    return parseLiveProxy(response)
  }

  async createProviderAuthorization(input: {
    name: string
    type: Exclude<ProviderType, 'PROVIDER_TYPE_CUSTOM' | 'PROVIDER_TYPE_OPENCODE_ZEN' | 'PROVIDER_TYPE_OPENCODE_GO'>
    customUrl: string
    useProxy: boolean
    proxy: string
  }): Promise<ProviderAuthorization> {
    const response = await this.request('/api/aoyo/v1/providers/authorize', {
      method: 'POST',
      body: JSON.stringify(input),
    })
    return {
      authorizationUrl: text(response.authorizationUrl ?? response.authorization_url),
      state: text(response.state),
      providerId: text(response.providerId ?? response.provider_id),
      flow: text(response.flow) === 'device' ? 'device' : 'callback',
      userCode: text(response.userCode ?? response.user_code),
      expiresIn: Number(response.expiresIn ?? response.expires_in ?? 0),
    }
  }

  async completeProviderAuthorization(
    state: string,
    callbackUrl: string,
    useProxy: boolean,
    proxy: string,
    type: ProviderType,
  ): Promise<ProviderAuthorizationStatus> {
    const response = await this.request('/api/aoyo/v1/providers/authorize/complete', {
      method: 'POST',
      body: JSON.stringify({ state, callbackUrl, useProxy, proxy, type }),
    })
    return parseAuthorizationStatus(response)
  }

  async getProviderAuthorizationStatus(state: string, useProxy: boolean, proxy: string): Promise<ProviderAuthorizationStatus> {
    const query = new URLSearchParams({ useProxy: String(useProxy), proxy })
    const response = await this.request(`/api/aoyo/v1/providers/authorize/${encodeURIComponent(state)}?${query}`)
    return parseAuthorizationStatus(response)
  }

  async updateProvider(input: UpdateProviderInput): Promise<void> {
    await this.request(`/api/aoyo/v1/providers/${encodeURIComponent(input.id)}`, {
      method: 'PATCH',
      body: JSON.stringify({
        name: input.name,
        type: input.type,
        clientId: input.customUrl,
        clientSecret: input.authorizationData,
        useProxy: input.useProxy,
        proxy: input.proxy,
        isCloudflare: input.isCloudflare,
        priority: input.priority,
        disabled: input.disabled,
      }),
    })
  }

  async deleteProvider(id: string): Promise<void> {
    await this.request(`/api/aoyo/v1/providers/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  async getUsageLogs(limit = 100, offset = 0): Promise<LogEntry[]> {
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    const response = await this.request(`/api/aoyo/v1/logs?${query}`)
    return Array.isArray(response.logs) ? response.logs.map(parseLogEntry) : []
  }

  async getErrors(): Promise<ErrorLog[]> {
    const response = await this.request('/api/aoyo/v1/logs/errors')
    return Array.isArray(response.errors) ? response.errors.map(parseErrorLog) : []
  }

  async getProviderLogsByKeyID(apiKeyId: string): Promise<ApiKeyUsage> {
    const response = await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(apiKeyId)}/logs`)
    return {
      logs: Array.isArray(response.logs) ? response.logs.map(parseLogEntry).slice(0, 10) : [],
      totalTokens: Number(response.totalTokens ?? response.total_tokens ?? 0),
    }
  }

  async getPushConfig(): Promise<{ vapidPublicKey: string, enabled: boolean }> {
    const response = await this.request('/api/aoyo/v1/notifications/config')
    return {
      vapidPublicKey: text(response.vapidPublicKey ?? response.vapid_public_key),
      enabled: Boolean(response.enabled),
    }
  }

  async getPushSubscriptions(endpoint: string): Promise<string[]> {
    const query = new URLSearchParams({ endpoint })
    const response = await this.request(`/api/aoyo/v1/notifications/subscriptions?${query}`)
    return Array.isArray(response.subjects) ? response.subjects.map(text).filter(Boolean) : []
  }

  async subscribeToProvider(providerId: string, subscription: PushSubscriptionPayload, userAgent: string): Promise<void> {
    await this.request('/api/aoyo/v1/notifications/subscribe', {
      method: 'POST',
      body: JSON.stringify({
        subject: quotaSubject(providerId),
        subscription,
        userAgent,
        labels: { providerId },
      }),
    })
  }

  async listNotificationEvents(endpoint: string, afterId: number): Promise<NotificationEventPage> {
    const query = new URLSearchParams({ endpoint, afterId: String(afterId) })
    const response = await this.request(`/api/aoyo/v1/notifications/events?${query}`)
    const events = Array.isArray(response.events) ? response.events.map(parseNotificationEvent) : []
    const reported = Number(response.lastId ?? response.last_id ?? 0)
    const lastId = Number.isFinite(reported) && reported > 0
      ? reported
      : events.reduce((max, event) => (event.id > max ? event.id : max), afterId)
    return { events, lastId }
  }

  async unsubscribeFromProvider(providerId: string, endpoint: string): Promise<void> {
    await this.request('/api/aoyo/v1/notifications/unsubscribe', {
      method: 'POST',
      body: JSON.stringify({ subject: quotaSubject(providerId), endpoint }),
    })
  }
}
