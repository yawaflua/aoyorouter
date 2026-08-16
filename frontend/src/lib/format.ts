import type { LogEntry } from './models/logentry'
import type { Provider } from './models/providers'

export function formatTokens(tokens: number): string {
  return `${(tokens / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 3 })}M tokens`
}

export function formatLogTime(entry: LogEntry): string {
  const value = entry.requestTime || entry.createdAt
  if (!value) return 'Unknown time'

  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

export function quotaLabel(provider: Provider): string {
  const quota = provider.quota?.primary
  const error = provider.quota?.error

  if (quota) return `${Math.max(0, Math.round(100 - quota.usedPercent))}% left`
  if (error?.includes('active subscription')) return 'Subscription required'
  return error ? 'Quota unavailable' : ''
}

export function quotaReset(provider: Provider): string {
  if (provider.quota?.error) return provider.quota.error

  const reset = provider.quota?.primary?.resetsAt
  if (!reset) return ''

  const date = new Date(reset)
  return Number.isNaN(date.getTime()) ? '' : `Resets ${date.toLocaleString()}`
}
