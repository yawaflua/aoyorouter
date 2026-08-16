import type { LogEntry } from './logentry'

export interface ApiKey {
  id: string
  name: string
  isAdmin: string
}

export interface ApiKeyUsage {
  logs: LogEntry[]
  totalTokens: number
}

export interface CreatedApiKey {
  id: string
  value: string
}
