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
import { record, text, type JsonRecord } from './utils'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

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
    return body ? record(JSON.parse(body)) : {}
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
      body: JSON.stringify({ name, isAdmin: String(isAdmin) }),
    })
    return {
      id: text(response.apiKeyId ?? response.api_key_id),
      value: text(response.apiKey ?? response.api_key),
    }
  }

  async deleteApiKey(id: string): Promise<void> {
    await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(id)}`, { method: 'DELETE' })
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
}
