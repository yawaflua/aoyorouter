import { record, text } from '../utils'

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
export function parseQuota(value: unknown): ProviderQuota | null {
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
