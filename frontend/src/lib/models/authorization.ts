import { record, text } from '../utils'

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

export function parseAuthorizationStatus(value: unknown): ProviderAuthorizationStatus {
  const response = record(value)
  const rawStatus = text(response.status)
  return {
    status: rawStatus === 'ok' || rawStatus === 'error' ? rawStatus : 'pending',
    providerId: text(response.providerId ?? response.provider_id),
    error: text(response.error),
  }
}
