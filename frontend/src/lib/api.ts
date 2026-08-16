export type ProviderType =
  | 'PROVIDER_TYPE_CUSTOM'
  | 'PROVIDER_TYPE_OPENAI'
  | 'PROVIDER_TYPE_ANTHROPIC'
  | 'PROVIDER_TYPE_KIMI'
  | 'PROVIDER_TYPE_GROK'
  | 'PROVIDER_TYPE_ANTIGRAVITY'

export interface QuotaWindow {
  usedPercent: number
  resetsAt: string
  windowMinutes: number
}

export interface ProviderQuota {
  primary: QuotaWindow | null
  secondary: QuotaWindow | null
  planType: string
  error: string
}

export interface ApiKey {
  id: string
  name: string
  isAdmin: string
}

export interface Provider {
  id: string
  name: string
  type: ProviderType
  customUrl: string
  quota: ProviderQuota | null
}

export interface LogEntry {
  provider: string
  apiKeyId: string
  latency: number
  inputTokens: number
  outputTokens: number
  totalTokens: number
  cachedTokens: number
  model: string
  reasoningEffort: string
  failed: boolean
  error: string
  requestTime: string
  createdAt: string
}

export interface ApiKeyUsage {
  logs: LogEntry[]
  totalTokens: number
}

export interface CreatedApiKey {
  id: string
  value: string
}

export interface CodexAuthorization {
  authorizationUrl: string
  state: string
  providerId: string
}

export interface ProviderAuthorization {
  authorizationUrl: string
  state: string
  providerId: string
  flow: 'device' | 'callback'
  userCode: string
  expiresIn: number
}

export interface ProviderAuthorizationStatus {
  status: 'pending' | 'ok' | 'error'
  providerId: string
  error: string
}

type JsonRecord = Record<string, unknown>

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function record(value: unknown): JsonRecord {
  return value && typeof value === 'object' ? (value as JsonRecord) : {}
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function providerType(value: unknown): ProviderType {
  if (value === 2 || value === 'PROVIDER_TYPE_OPENAI') return 'PROVIDER_TYPE_OPENAI'
  if (value === 3 || value === 'PROVIDER_TYPE_ANTHROPIC') return 'PROVIDER_TYPE_ANTHROPIC'
  if (value === 4 || value === 'PROVIDER_TYPE_KIMI') return 'PROVIDER_TYPE_KIMI'
  if (value === 5 || value === 'PROVIDER_TYPE_GROK') return 'PROVIDER_TYPE_GROK'
  if (value === 6 || value === 'PROVIDER_TYPE_ANTIGRAVITY') return 'PROVIDER_TYPE_ANTIGRAVITY'
  return 'PROVIDER_TYPE_CUSTOM'
}

function parseProvider(value: unknown): Provider {
  const item = record(value)
  return {
    id: text(item.id),
    name: text(item.name),
    type: providerType(item.type),
    customUrl: text(item.clientId ?? item.client_id),
    quota: parseQuota(item.quota),
  }
}

function parseLogEntry(value: unknown): LogEntry {
  const item = record(value)
  return {
    provider: text(item.provider),
    apiKeyId: text(item.apiKeyId ?? item.api_key_id),
    latency: Number(item.latency ?? 0),
    inputTokens: Number(item.inputTokens ?? item.input_tokens ?? 0),
    outputTokens: Number(item.outputTokens ?? item.output_tokens ?? 0),
    totalTokens: Number(item.totalTokens ?? item.total_tokens ?? 0),
    cachedTokens: Number(item.cachedTokens ?? item.cached_tokens ?? 0),
    model: text(item.model),
    reasoningEffort: text(item.reasoningEffort ?? item.reasoning_effort),
    failed: item.failed === true,
    error: text(item.error),
    requestTime: text(item.requestTime ?? item.request_time),
    createdAt: text(item.createdAt ?? item.created_at),
  }
}

function parseQuota(value: unknown): ProviderQuota | null {
  const item = record(value)
  if (!Object.keys(item).length) return null
  const parseWindow = (windowValue: unknown): QuotaWindow | null => {
    const window = record(windowValue)
    if (!Object.keys(window).length) return null
    return {
      usedPercent: Number(window.usedPercent ?? window.used_percent ?? 0),
      resetsAt: text(window.resetsAt ?? window.resets_at),
      windowMinutes: Number(window.windowMinutes ?? window.window_minutes ?? 0),
    }
  }
  return {
    primary: parseWindow(item.primary),
    secondary: parseWindow(item.secondary),
    planType: text(item.planType ?? item.plan_type),
    error: text(item.error),
  }
}

async function errorMessage(response: Response): Promise<string> {
  const fallback = response.status === 401 ? 'The password is incorrect.' : `Server returned status ${response.status}.`
  const body = await response.text()
  if (!body) return fallback

  try {
    const parsed = record(JSON.parse(body))
    return text(parsed.message) || text(parsed.error) || fallback
  } catch {
    return body.length < 240 ? body : fallback
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

function parseAuthorizationStatus(value: unknown): ProviderAuthorizationStatus {
  const response = record(value)
  const rawStatus = text(response.status)
  return {
    status: rawStatus === 'ok' || rawStatus === 'error' ? rawStatus : 'pending',
    providerId: text(response.providerId ?? response.provider_id),
    error: text(response.error),
  }
}
