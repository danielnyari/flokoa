/**
 * AG-UI event types emitted by the playground SSE endpoint.
 */
type AGUIEventType = 'RUN_STARTED' | 'TEXT_MESSAGE_START' | 'TEXT_MESSAGE_CONTENT' | 'TEXT_MESSAGE_END' | 'RUN_FINISHED' | 'RUN_ERROR' | 'DATA_PART'

interface AGUIEvent {
  type: AGUIEventType
  runId?: string
  threadId?: string
  messageId?: string
  role?: string
  delta?: string
  message?: string
  dataId?: string
  data?: unknown
  name?: string
  timestamp: number
}

/**
 * AI SDK v5 compatible part types for UChatMessage.
 */
export type AgTextPart = { type: 'text'; text: string }
export type AgDataPart = { type: 'data-artifact'; id: string; data: unknown; name?: string }
export type AgMessagePart = AgTextPart | AgDataPart

/**
 * Message format compatible with Nuxt UI's UChatMessage `parts` prop.
 */
export interface AgMessage {
  id: string
  role: 'user' | 'assistant'
  parts: AgMessagePart[]
}

type ChatStatus = 'idle' | 'streaming' | 'error'

/**
 * useAgChat manages a stateless playground chat session.
 *
 * Sends messages to the Flokoa server's playground endpoint which bridges
 * to A2A agents, streaming back AG-UI events over SSE. Messages are kept
 * only in the browser's reactive state — no persistence.
 */
export function useAgChat() {
  const auth = useAuth()
  const messages = ref<AgMessage[]>([])
  const status = ref<ChatStatus>('idle')
  const error = ref<string | null>(null)

  let abortController: AbortController | null = null

  function authHeaders(): Record<string, string> {
    const token = auth.getAccessToken()
    if (token) return { Authorization: `Bearer ${token}` }
    return {}
  }

  /**
   * Send a message to the selected agent and stream the response.
   */
  async function send(namespace: string, agentName: string, message: string) {
    if (!message.trim()) return

    // Add the user message to the conversation.
    const userMsgId = crypto.randomUUID()
    messages.value = [...messages.value, {
      id: userMsgId,
      role: 'user',
      parts: [{ type: 'text' as const, text: message }]
    }]

    // Build history from previous messages (excluding the one we just added).
    const history = messages.value
      .filter(m => m.id !== userMsgId)
      .map(m => ({
        role: m.role,
        content: m.parts.filter((p): p is AgTextPart => p.type === 'text').map(p => p.text).join('')
      }))

    status.value = 'streaming'
    error.value = null
    abortController = new AbortController()

    try {
      const stream = await $fetch<ReadableStream>(
        `/api/v1alpha1/namespaces/${encodeURIComponent(namespace)}/agents/${encodeURIComponent(agentName)}/playground`,
        {
          method: 'POST',
          headers: authHeaders(),
          body: { message, history },
          responseType: 'stream',
          signal: abortController.signal
        }
      )

      const reader = stream.pipeThrough(new TextDecoderStream()).getReader()
      await readSSEStream(reader)
    } catch (err) {
      if ((err as Error).name === 'AbortError') {
        status.value = 'idle'
        return
      }
      error.value = err instanceof Error ? err.message : String(err)
      status.value = 'error'
    } finally {
      abortController = null
    }
  }

  /**
   * Read and process the SSE stream from the response body.
   */
  async function readSSEStream(reader: ReadableStreamDefaultReader<string>) {
    let buffer = ''

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += value
        const { events, remaining } = parseSSEEvents(buffer)
        buffer = remaining

        for (const event of events) {
          handleEvent(event)
        }
      }

      // Process any remaining buffered data.
      if (buffer.trim()) {
        const { events } = parseSSEEvents(buffer + '\n\n')
        for (const event of events) {
          handleEvent(event)
        }
      }

      // If stream ended without RUN_FINISHED or RUN_ERROR, set idle.
      if (status.value === 'streaming') {
        status.value = 'idle'
      }
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        error.value = err instanceof Error ? err.message : String(err)
        status.value = 'error'
      }
    }
  }

  /**
   * Handle a single AG-UI event.
   */
  function handleEvent(event: AGUIEvent) {
    switch (event.type) {
      case 'TEXT_MESSAGE_START': {
        const msgId = event.messageId || crypto.randomUUID()
        messages.value = [...messages.value, {
          id: msgId,
          role: 'assistant',
          parts: [{ type: 'text' as const, text: '' }]
        }]
        break
      }

      case 'TEXT_MESSAGE_CONTENT': {
        if (!event.messageId || event.delta == null) break
        const idx = messages.value.findIndex(m => m.id === event.messageId)
        if (idx === -1) break

        // Clone to trigger reactivity.
        const updated: AgMessage[] = messages.value.map(m => ({ ...m, parts: [...m.parts] }))
        const msg = updated[idx]
        if (!msg) break
        const textPart = msg.parts.find((p): p is AgTextPart => p.type === 'text')
        if (textPart) {
          textPart.text += event.delta
        }
        messages.value = updated
        break
      }

      case 'DATA_PART': {
        if (!event.messageId || !event.data) break
        const idx = messages.value.findIndex(m => m.id === event.messageId)
        if (idx === -1) break

        // Clone to trigger reactivity.
        const updated: AgMessage[] = messages.value.map(m => ({ ...m, parts: [...m.parts] }))
        const msg = updated[idx]
        if (!msg) break
        msg.parts.push({
          type: 'data-artifact',
          id: event.dataId || crypto.randomUUID(),
          data: event.data,
          name: event.name
        })
        messages.value = updated
        break
      }

      case 'TEXT_MESSAGE_END':
        // No action needed — message is already complete.
        break

      case 'RUN_FINISHED':
        status.value = 'idle'
        break

      case 'RUN_ERROR':
        error.value = event.message || 'An error occurred'
        status.value = 'error'
        break

      case 'RUN_STARTED':
        // No action needed.
        break
    }
  }

  /**
   * Clear the conversation.
   */
  function clear() {
    messages.value = []
    error.value = null
    status.value = 'idle'
  }

  /**
   * Abort the current streaming request.
   */
  function stop() {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    status.value = 'idle'
  }

  return {
    messages: readonly(messages) as Readonly<Ref<AgMessage[]>>,
    status: readonly(status) as Readonly<Ref<ChatStatus>>,
    error: readonly(error) as Readonly<Ref<string | null>>,
    send,
    clear,
    stop
  }
}

/**
 * Parse SSE events from a text buffer.
 * SSE format: "event: <type>\ndata: <json>\n\n"
 */
function parseSSEEvents(text: string): { events: AGUIEvent[]; remaining: string } {
  const parts = text.split('\n\n')
  const remaining = parts.pop() ?? ''
  const events: AGUIEvent[] = []

  for (const part of parts) {
    if (!part.trim()) continue
    // Skip heartbeat comments.
    if (part.trim().startsWith(':')) continue

    const lines = part.split('\n')
    let data = ''

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        data = line.slice(6)
      } else if (line.startsWith('data:')) {
        data = line.slice(5)
      }
    }

    if (data) {
      try {
        events.push(JSON.parse(data) as AGUIEvent)
      } catch {
        // Ignore unparseable events.
      }
    }
  }

  return { events, remaining }
}
