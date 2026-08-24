import type { LogEntry } from './models/logentry'
import type { Provider } from './models/providers'

export function formatTokens(tokens: number): string {
  return `${(tokens / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 3 })}M tokens`
}

function parseLocalDate(value: string): Date | null {
  const normalized = value.replace(/(\.\d{3})\d+(Z|[+-]\d{2}:\d{2})?$/, '$1$2')
  const date = new Date(normalized)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatDateTime(value: string): string {
  if (!value) return 'Not scheduled'
  const date = parseLocalDate(value)
  if (!date) return value
  return date.toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  })
}

export function formatLogTime(entry: LogEntry): string {
  const value = entry.requestTime || entry.createdAt
  if (!value) return 'Unknown time'

  const date = parseLocalDate(value)
  return date ? date.toLocaleString() : value
}

export function quotaLabel(provider: Provider): string {
  const quota = provider.quota?.quotas?.[0]
  const error = provider.quota?.error

  if (quota) return `${Math.max(0, Math.round(100 - quota.usedPercent))}% left`
  if (error?.includes('active subscription')) return 'Subscription required'
  return error ? 'Quota unavailable' : ''
}

export function quotaReset(provider: Provider): string {
  if (provider.quota?.error) return provider.quota.error

  const reset = provider.quota?.quotas?.[0]?.resetsAt
  if (!reset) return ''

  const date = parseLocalDate(reset)
  return date ? `Resets ${date.toLocaleString()}` : ''
}
