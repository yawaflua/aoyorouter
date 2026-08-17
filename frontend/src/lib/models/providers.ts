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
  clientSecret: string
  useProxy: boolean
  proxy: string
  quota: ProviderQuota | null
}

export interface ProviderConnectionInput {
  name: string
  type: ProviderType
  customUrl: string
  authorizationData: string
  useProxy: boolean
  proxy: string
}

export interface UpdateProviderInput extends ProviderConnectionInput {
  id: string
}

export function parseProvider(value: unknown): Provider {
  const item = record(value)
  return {
    id: text(item.id),
    name: text(item.name),
    type: providerType(item.type),
    customUrl: text(item.clientId ?? item.client_id),
    clientSecret: text(item.clientSecret ?? item.client_secret),
    useProxy: booleanValue(item.useProxy ?? item.use_proxy),
    proxy: text(item.proxy),
    quota: parseQuota(item.quota),
  }
}

function booleanValue(value: unknown): boolean {
  return value === true || value === 'true' || value === 1
}

export function providerType(value: unknown): ProviderType {
  if (value === 2 || value === 'PROVIDER_TYPE_OPENAI') return 'PROVIDER_TYPE_OPENAI'
  if (value === 3 || value === 'PROVIDER_TYPE_ANTHROPIC') return 'PROVIDER_TYPE_ANTHROPIC'
  if (value === 4 || value === 'PROVIDER_TYPE_KIMI') return 'PROVIDER_TYPE_KIMI'
  if (value === 5 || value === 'PROVIDER_TYPE_GROK') return 'PROVIDER_TYPE_GROK'
  if (value === 6 || value === 'PROVIDER_TYPE_ANTIGRAVITY') return 'PROVIDER_TYPE_ANTIGRAVITY'
  return 'PROVIDER_TYPE_CUSTOM'
}

export function providerTypeAsCLIPROXY(value: ProviderType): string {
  if (value === 'PROVIDER_TYPE_OPENAI') return 'openai'
  if (value === 'PROVIDER_TYPE_ANTHROPIC') return 'anthropic'
  if (value === 'PROVIDER_TYPE_KIMI') return 'moonshot'
  if (value === 'PROVIDER_TYPE_GROK') return 'grok'
  if (value === 'PROVIDER_TYPE_ANTIGRAVITY') return 'antigravity'
  return 'custom'
}

export interface ProviderModel {
  created: number | null
  id: string
  object: string
  owned_by: string
}

export function parseProviderModel(value: unknown): ProviderModel {
  const item = record(value)
  return {
    created: Number(item.created) ?? null,
    id: text(item.id),
    object: text(item.object),
    owned_by: text(item.owned_by),
  }
}
