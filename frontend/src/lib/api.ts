import { ApiError, errorMessage } from './models/apierror'
import type { ApiKey, ApiKeyUsage, CreatedApiKey } from './models/apikey'
import {
  parseAuthorizationStatus,
  type CodexAuthorization,
  type ProviderAuthorization,
  type ProviderAuthorizationStatus,
} from './models/authorization'
import { parseLogEntry, type LogEntry } from './models/logentry'
import { parseProvider, type Provider, type ProviderType } from './models/providers'
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

  async signIn(): Promise<void> {
    await this.request('/api/aoyo/v1/signin', {
      method: 'POST',
      body: JSON.stringify({ password: this.password }),
    })
  }

  async getApiKeys(): Promise<ApiKey[]> {
    const response = await this.request('/api/aoyo/v1/api-keys')
    const items = response.apiKeys ?? response.api_keys
    return Array.isArray(items)
      ? items.map((value) => {
          const item = record(value)
          return {
            id: text(item.id),
            name: text(item.name),
            isAdmin: text(item.isAdmin ?? item.is_admin),
          }
        })
      : []
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

  async getProviders(): Promise<Provider[]> {
    const response = await this.request('/api/aoyo/v1/providers')
    return Array.isArray(response.providers) ? response.providers.map(parseProvider) : []
  }

  async createProvider(input: {
    name: string
    type: ProviderType
    customUrl: string
    authorizationData: string
  }): Promise<string> {
    const response = await this.request('/api/aoyo/v1/providers', {
      method: 'POST',
      body: JSON.stringify({
        name: input.name,
        type: input.type,
        clientId: input.customUrl,
        clientSecret: input.authorizationData,
      }),
    })
    return text(response.providerId ?? response.provider_id)
  }

  async createCodexAuthorization(name: string, customUrl: string): Promise<CodexAuthorization> {
    const response = await this.request('/api/aoyo/v1/providers/codex/authorize', {
      method: 'POST',
      body: JSON.stringify({ name, customUrl }),
    })
    return {
      authorizationUrl: text(response.authorizationUrl ?? response.authorization_url),
      state: text(response.state),
      providerId: text(response.providerId ?? response.provider_id),
    }
  }

  async completeCodexAuthorization(input: {
    state: string
    callbackUrl: string
  }): Promise<string> {
    const response = await this.request('/api/aoyo/v1/providers/codex/complete', {
      method: 'POST',
      body: JSON.stringify(input),
    })
    return text(response.providerId ?? response.provider_id)
  }

  async createProviderAuthorization(input: {
    name: string
    type: Exclude<ProviderType, 'PROVIDER_TYPE_CUSTOM' | 'PROVIDER_TYPE_OPENAI'>
    customUrl: string
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

  async completeProviderAuthorization(state: string, callbackUrl: string): Promise<ProviderAuthorizationStatus> {
    const response = await this.request('/api/aoyo/v1/providers/authorize/complete', {
      method: 'POST',
      body: JSON.stringify({ state, callbackUrl }),
    })
    return parseAuthorizationStatus(response)
  }

  async getProviderAuthorizationStatus(state: string): Promise<ProviderAuthorizationStatus> {
    const response = await this.request(`/api/aoyo/v1/providers/authorize/${encodeURIComponent(state)}`)
    return parseAuthorizationStatus(response)
  }

  async deleteProvider(id: string): Promise<void> {
    await this.request(`/api/aoyo/v1/providers/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  async getUsageLogs(limit = 100, offset = 0): Promise<LogEntry[]> {
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    const response = await this.request(`/api/aoyo/v1/usage/logs?${query}`)
    return Array.isArray(response.logs) ? response.logs.map(parseLogEntry) : []
  }

  async getProviderLogsByKeyID(apiKeyId: string): Promise<ApiKeyUsage> {
    const response = await this.request(`/api/aoyo/v1/api-keys/${encodeURIComponent(apiKeyId)}/logs`)
    return {
      logs: Array.isArray(response.logs) ? response.logs.map(parseLogEntry).slice(0, 10) : [],
      totalTokens: Number(response.totalTokens ?? response.total_tokens ?? 0),
    }
  }
}
