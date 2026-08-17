import type { LogEntry } from './logentry'
import { record, text } from '../utils'

export type QuotaResetStrategy =
  | 'QUOTA_RESET_STRATEGY_MINUTES'
  | 'QUOTA_RESET_STRATEGY_HOURLY'
  | 'QUOTA_RESET_STRATEGY_DAILY'
  | 'QUOTA_RESET_STRATEGY_WEEKLY'
  | 'QUOTA_RESET_STRATEGY_MONTHLY'
  | 'QUOTA_RESET_STRATEGY_FOREVER'

export interface ApiKey {
  id: string
  name: string
  isAdmin: string
  isActive: boolean
  quotaSet: boolean
  reservedTokens: number
  quotaUsed: number
  quotaResetAt: string
  quotaResetStrategy: QuotaResetStrategy
}

export interface UpdateApiKeyInput {
  id: string
  name: string
  isAdmin: string
  isActive: boolean
  quotaSet: boolean
  reservedTokens: number
  quotaResetAt: string
  quotaResetStrategy: QuotaResetStrategy
}

export interface ApiKeyUsage {
  logs: LogEntry[]
  totalTokens: number
}

export interface CreatedApiKey {
  id: string
  value: string
}

export const quotaResetLabels: Record<QuotaResetStrategy, string> = {
  QUOTA_RESET_STRATEGY_MINUTES: 'Every minute',
  QUOTA_RESET_STRATEGY_HOURLY: 'Every hour',
  QUOTA_RESET_STRATEGY_DAILY: 'Every day',
  QUOTA_RESET_STRATEGY_WEEKLY: 'Every week',
  QUOTA_RESET_STRATEGY_MONTHLY: 'Every month',
  QUOTA_RESET_STRATEGY_FOREVER: 'Never',
}

export const quotaResetOptions = Object.entries(quotaResetLabels).map(([value, label]) => ({
  value: value as QuotaResetStrategy,
  label,
}))

export function parseApiKey(value: unknown): ApiKey {
  const item = record(value)
  return {
    id: text(item.id),
    name: text(item.name),
    isAdmin: text(item.isAdmin ?? item.is_admin),
    isActive: booleanValue(item.isActive ?? item.is_active, true),
    quotaSet: booleanValue(item.quotaSetted ?? item.quota_setted, false),
    reservedTokens: Number(item.reservedTokens ?? item.reserved_tokens ?? item.reserverTokens ?? item.reserver_tokens ?? 0),
    quotaUsed: Number(item.quotaUsed ?? item.quota_used ?? 0),
    quotaResetAt: text(item.quotaResetAt ?? item.quota_reset_at),
    quotaResetStrategy: quotaResetStrategy(item.quotaResetStrategy ?? item.quota_reset_strategy),
  }
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  if (value === undefined || value === null || value === '') return fallback
  return value === true || value === 'true' || value === 1
}

function quotaResetStrategy(value: unknown): QuotaResetStrategy {
  if (value === 1 || value === 'QUOTA_RESET_STRATEGY_MINUTES') return 'QUOTA_RESET_STRATEGY_MINUTES'
  if (value === 2 || value === 'QUOTA_RESET_STRATEGY_HOURLY') return 'QUOTA_RESET_STRATEGY_HOURLY'
  if (value === 3 || value === 'QUOTA_RESET_STRATEGY_DAILY') return 'QUOTA_RESET_STRATEGY_DAILY'
  if (value === 4 || value === 'QUOTA_RESET_STRATEGY_WEEKLY') return 'QUOTA_RESET_STRATEGY_WEEKLY'
  if (value === 5 || value === 'QUOTA_RESET_STRATEGY_MONTHLY') return 'QUOTA_RESET_STRATEGY_MONTHLY'
  return 'QUOTA_RESET_STRATEGY_FOREVER'
}
