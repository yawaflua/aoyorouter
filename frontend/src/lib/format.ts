import type { LogEntry } from './models/logentry'
import type { QuotaWindow } from './models/quota';

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

export function quotaLabel(quota: QuotaWindow | null): string {
  if (quota === null) return 'Quota unavailable'

  return `${Math.max(0, Math.round(100 - quota.usedPercent))}% left`
}

export function quotaReset(quota: QuotaWindow | null): string {
  if (quota === null) return ''

  const reset = quota?.resetsAt
  if (!reset) return ''

  const date = parseLocalDate(reset)
  return date ? `Resets ${date.toLocaleString()}` : ''
}

export function quotaResetLabel(quota: QuotaWindow | null, nowMs: number = Date.now()): string {
  if (quota === null) return ''

  const reset = quota?.resetsAt
  if (!reset) return ''
  const date = parseLocalDate(reset)
  if (!date) return ''

  const now = new Date(nowMs)
  const diffMs = date.getTime() - now.getTime()
  if (diffMs <= 0) return 'now'

  const isToday =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (isToday) {
    const totalMinutes = Math.ceil(diffMs / 60_000)
    const hours = Math.floor(totalMinutes / 60)
    const minutes = totalMinutes % 60
    if (hours === 0) return `${minutes}m`
    if (minutes === 0) return `${hours}h`
    return `${hours}h ${minutes}m`
  }

  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfReset = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const days = Math.round((startOfReset.getTime() - startOfToday.getTime()) / 86_400_000)

  return `${days}d`
}
