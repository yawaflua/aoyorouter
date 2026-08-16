export type JsonRecord = Record<string, unknown>

export function record(value: unknown): JsonRecord {
  return value && typeof value === 'object' ? (value as JsonRecord) : {}
}

export function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}
