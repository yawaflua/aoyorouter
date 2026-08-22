const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  thinking?: string
}

export interface ChatStreamHandlers {
  onDelta: (text: string) => void
  onThinking: (text: string) => void
  onDone: () => void
}

export class ChatError extends Error {}

export async function streamChat(
  password: string,
  model: string,
  messages: ChatMessage[],
  handlers: ChatStreamHandlers,
  signal: AbortSignal,
): Promise<void> {
  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}/v1/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
        Authorization: `Password ${password}`,
      },
      body: JSON.stringify({ model, max_tokens: 8192, stream: true, messages }),
      signal,
    })
  } catch (error) {
    if (signal.aborted) return
    throw new ChatError('Could not reach the router. Check your connection.')
  }

  if (!response.ok || !response.body) {
    let message = `Request failed with status ${response.status}.`
    try {
      const body = await response.json()
      if (body?.error?.message) message = body.error.message
    } catch {
      // keep default message
    }
    throw new ChatError(message)
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      let boundary = buffer.indexOf('\n')
      while (boundary !== -1) {
        const line = buffer.slice(0, boundary).trim()
        buffer = buffer.slice(boundary + 1)
        boundary = buffer.indexOf('\n')
        if (!line.startsWith('data:')) continue
        const payload = line.slice(5).trim()
        if (!payload || payload === '[DONE]') continue

        let event: any
        try {
          event = JSON.parse(payload)
        } catch {
          continue
        }

        if (event.type === 'content_block_delta') {
          if (event.delta?.text) handlers.onDelta(event.delta.text)
          else if (event.delta?.thinking) handlers.onThinking(event.delta.thinking)
        } else if (event.type === 'error') {
          throw new ChatError(event.error?.message ?? 'The stream failed.')
        }
      }
    }
    handlers.onDone()
  } finally {
    reader.releaseLock()
  }
}
