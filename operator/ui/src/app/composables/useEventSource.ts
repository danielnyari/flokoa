/**
 * useEventSource - Low-level composable for consuming Server-Sent Events (SSE).
 *
 * Uses the native EventSource API with support for:
 * - Auth via query param (EventSource can't set custom headers)
 * - Automatic reconnection with exponential backoff
 * - Typed event parsing
 */
export interface EventSourceOptions {
  /** Called for each parsed SSE event */
  onEvent: (event: unknown) => void
  /** Called when the connection is established */
  onOpen?: () => void
  /** Called when an error occurs */
  onError?: (error: Event) => void
}

export function useEventSource(url: MaybeRef<string>, options: EventSourceOptions) {
  const auth = useAuth()
  let eventSource: EventSource | null = null
  let reconnectMs = 3000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  const connected = ref(false)
  const error = ref<Event | null>(null)

  function buildUrl(): string {
    const base = toValue(url)
    const token = auth.getAccessToken()
    if (!token) return base
    const separator = base.includes('?') ? '&' : '?'
    return `${base}${separator}access_token=${encodeURIComponent(token)}`
  }

  function connect() {
    disconnect()
    error.value = null

    const fullUrl = buildUrl()
    eventSource = new EventSource(fullUrl)

    eventSource.onopen = () => {
      connected.value = true
      reconnectMs = 3000 // reset backoff on successful connection
      options.onOpen?.()
    }

    eventSource.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data)
        options.onEvent(data)
      } catch {
        // Ignore unparseable messages (e.g. heartbeats)
      }
    }

    eventSource.onerror = (e) => {
      connected.value = false
      error.value = e
      options.onError?.(e)
      disconnect()
      scheduleReconnect()
    }
  }

  function disconnect() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function scheduleReconnect() {
    reconnectTimer = setTimeout(() => {
      connect()
    }, reconnectMs)
    reconnectMs = Math.min(reconnectMs * 1.5, 60000)
  }

  // Auto-cleanup on scope dispose
  onScopeDispose(() => {
    disconnect()
  })

  return {
    connected: readonly(connected),
    error: readonly(error),
    connect,
    disconnect
  }
}
