import { record, text } from '../utils'
import { parseQuota, type ProviderQuota } from './quota'

export type ProviderType =
  | 'PROVIDER_TYPE_CUSTOM'
  | 'PROVIDER_TYPE_OPENAI'
  | 'PROVIDER_TYPE_ANTHROPIC'
  | 'PROVIDER_TYPE_KIMI'
  | 'PROVIDER_TYPE_GROK'
  | 'PROVIDER_TYPE_ANTIGRAVITY'

export interface Provider {
  id: string
  name: string
  type: ProviderType
  customUrl: string
  quota: ProviderQuota | null
}

export function parseProvider(value: unknown): Provider {
  const item = record(value)
  return {
    id: text(item.id),
    name: text(item.name),
    type: providerType(item.type),
    customUrl: text(item.clientId ?? item.client_id),
    quota: parseQuota(item.quota),
  }
}

export function providerType(value: unknown): ProviderType {
  if (value === 2 || value === 'PROVIDER_TYPE_OPENAI') return 'PROVIDER_TYPE_OPENAI'
  if (value === 3 || value === 'PROVIDER_TYPE_ANTHROPIC') return 'PROVIDER_TYPE_ANTHROPIC'
  if (value === 4 || value === 'PROVIDER_TYPE_KIMI') return 'PROVIDER_TYPE_KIMI'
  if (value === 5 || value === 'PROVIDER_TYPE_GROK') return 'PROVIDER_TYPE_GROK'
  if (value === 6 || value === 'PROVIDER_TYPE_ANTIGRAVITY') return 'PROVIDER_TYPE_ANTIGRAVITY'
  return 'PROVIDER_TYPE_CUSTOM'
}
