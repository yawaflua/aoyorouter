import { record, text } from '../utils'

export interface ErrorLog {
  id: string
  url: string
  method: string
  timestamp: string
  headers: string[]
  body: string
  responseBody: string
  statusCode: number
}

export function parseErrorLog(value: unknown): ErrorLog {
  const item = record(value)
  const method = record(item.method)
  return {
    id: text(item.id),
    url: text(item.url),
    method: text(method.name),
    timestamp: text(item.timestamp),
    headers: Array.isArray(item.headers) ? item.headers.map(text) : [],
    body: text(item.body),
    responseBody: text(item.responseBody ?? item.response_body),
    statusCode: Number(item.statusCode ?? item.status_code ?? 0),
  }
}
