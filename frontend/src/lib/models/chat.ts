const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export interface ContentBlock {
  type: 'text' | 'image'
  text?: string
  source?: { type: 'base64'; media_type: string; data: string }
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  thinking?: string
  attachments?: string[]
}

export interface ChatStreamHandlers {
  onDelta: (text: string) => void
  onThinking: (text: string) => void
  onDone: () => void
}

export const MAX_TOKENS = 65536

export async function readFile(file: File): Promise<ContentBlock & { name: string }> {
  if (file.type.startsWith('image/')) {
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(new ChatError(`Could not read ${file.name}.`))
      reader.readAsDataURL(file)
    })
    const base64 = dataUrl.slice(dataUrl.indexOf(',') + 1)
    return {
      name: file.name,
      type: 'image',
      source: { type: 'base64', media_type: file.type || 'image/png', data: base64 },
    }
  }
  const text = await file.text()
  return {
    name: file.name,
    type: 'text',
    text: `<file name="${file.name}">\n${text}\n</file>`,
  }
}

export class ChatError extends Error {}

export async function streamChat(
  password: string,
  model: string,
  messages: ChatMessage[],
  attachments: (ContentBlock & { name: string })[] | undefined,
  handlers: ChatStreamHandlers,
  signal: AbortSignal,
): Promise<void> {
  const lastUser = messages[messages.length - 1]?.content || ''
  const userContent: string | ContentBlock[] =
    attachments && attachments.length > 0
      ? [{ type: 'text', text: lastUser }, ...attachments.map(({ name, ...block }) => block)]
      : lastUser
  const apiMessages = [
    ...messages.slice(0, -1).map(({ role, content }) => ({ role, content })),
    { role: 'user' as const, content: userContent },
  ]

  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}/v1/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
        Authorization: `Password ${password}`,
      },
      body: JSON.stringify({ model, max_tokens: MAX_TOKENS, stream: true, messages: apiMessages }),
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
