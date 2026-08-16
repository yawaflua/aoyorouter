import { record, text } from '../utils'

export interface LogEntry {
  provider: string
  apiKeyId: string
  latency: number
  inputTokens: number
  outputTokens: number
  totalTokens: number
  cachedTokens: number
  model: string
  reasoningEffort: string
  failed: boolean
  error: string
  requestTime: string
  createdAt: string
}
export function parseLogEntry(value: unknown): LogEntry {
  const item = record(value)
  return {
    provider: text(item.provider),
    apiKeyId: text(item.apiKeyId ?? item.api_key_id),
    latency: Number(item.latency ?? 0),
    inputTokens: Number(item.inputTokens ?? item.input_tokens ?? 0),
    outputTokens: Number(item.outputTokens ?? item.output_tokens ?? 0),
    totalTokens: Number(item.totalTokens ?? item.total_tokens ?? 0),
    cachedTokens: Number(item.cachedTokens ?? item.cached_tokens ?? 0),
    model: text(item.model),
    reasoningEffort: text(item.reasoningEffort ?? item.reasoning_effort),
    failed: item.failed === true,
    error: text(item.error),
    requestTime: text(item.requestTime ?? item.request_time),
    createdAt: text(item.createdAt ?? item.created_at),
  }
}
