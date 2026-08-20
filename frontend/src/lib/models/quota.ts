import { record, text } from '../utils'

export interface QuotaWindow {
  name: string
  usedPercent: number
  resetsAt: string
  windowMinutes: number
}

export interface ProviderQuota {
  quotas: QuotaWindow[] | null
  planType: string
  error: string
}
export function parseQuota(value: unknown): ProviderQuota | null {
  const item = record(value)
  if (!Object.keys(item).length) return null
  const parseWindow = (windowValue: unknown): QuotaWindow => {
    const window = record(windowValue)
    
    return {
      name: text(window.name),
      usedPercent: Number(window.usedPercent ?? window.used_percent ?? 0),
      resetsAt: text(window.resetsAt ?? window.resets_at),
      windowMinutes: Number(window.windowMinutes ?? window.window_minutes ?? 0),
    }
  }
  return {
    quotas: Array.isArray(item.quotas) ? item.quotas.map(parseWindow) : null,
    planType: text(item.planType ?? item.plan_type),
    error: text(item.error),
  }
}
