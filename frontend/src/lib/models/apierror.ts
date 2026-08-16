import { record, text } from '../utils'

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}
export async function errorMessage(response: Response): Promise<string> {
  const fallback = response.status === 401 ? 'The password is incorrect.' : `Server returned status ${response.status}.`
  const body = await response.text()
  if (!body) return fallback

  try {
    const parsed = record(JSON.parse(body))
    return text(parsed.message) || text(parsed.error) || fallback
  } catch {
    return body.length < 240 ? body : fallback
  }
}
