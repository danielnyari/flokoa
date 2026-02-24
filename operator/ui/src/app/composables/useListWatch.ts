import type { ObjectMeta } from '~/types'
import { normaliseTimestamp } from '~/utils/enums'

/**
 * WatchEvent matches the SSE event format from the server.
 */
export interface WatchEvent<T> {
  type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'BOOKMARK' | 'ERROR'
  object: T
}

interface Resource {
  metadata: ObjectMeta
}

interface ListResponse<T> {
  items: T[]
}

export interface UseListWatchOptions<T extends Resource> {
  /** Function that returns the list API URL */
  listUrl: () => string
  /** The watch SSE endpoint URL */
  watchUrl: () => string
  /** Optional: sort function (default: newest first) */
  sorter?: (a: T, b: T) => number
}

/**
 * useListWatch - Vue composable implementing the Argo Workflows list-watch pattern.
 *
 * 1. Initial list() call gets all items
 * 2. Opens an SSE watch connection
 * 3. Merges ADDED/MODIFIED/DELETED events into the reactive list
 * 4. Automatic reconnection with exponential backoff
 *
 * Returns a reactive `items` ref that stays in sync with the cluster.
 */
export function useListWatch<T extends Resource>(options: UseListWatchOptions<T>) {
  const auth = useAuth()
  const items = ref<T[]>([]) as Ref<T[]>
  const status = ref<'idle' | 'pending' | 'success' | 'error'>('idle')
  const error = ref<Error | null>(null)
  let eventSource: EventSource | null = null
  let reconnectMs = 3000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  const defaultSorter = (a: Resource, b: Resource) => {
    const ta = normaliseTimestamp(a.metadata.creationTimestamp) ?? ''
    const tb = normaliseTimestamp(b.metadata.creationTimestamp) ?? ''
    return tb.localeCompare(ta) // newest first
  }

  const sorter = options.sorter ?? defaultSorter

  function authHeaders(): Record<string, string> {
    const token = auth.getAccessToken()
    if (token) {
      return { Authorization: `Bearer ${token}` }
    }
    return {}
  }

  /** Merge a single watch event into the items array */
  function mergeEvent(event: WatchEvent<T>) {
    const idx = items.value.findIndex(
      x => x.metadata.namespace === event.object.metadata.namespace
        && x.metadata.name === event.object.metadata.name
    )

    if (event.type === 'DELETED') {
      if (idx > -1) {
        items.value.splice(idx, 1)
        items.value = [...items.value]
      }
    } else if (event.type === 'ADDED' || event.type === 'MODIFIED') {
      if (idx > -1) {
        items.value[idx] = event.object
      } else {
        items.value.push(event.object)
      }
      items.value = [...items.value].sort(sorter)
    }
  }

  function buildWatchUrl(): string {
    const base = options.watchUrl()
    const token = auth.getAccessToken()
    if (!token) return base
    const separator = base.includes('?') ? '&' : '?'
    return `${base}${separator}access_token=${encodeURIComponent(token)}`
  }

  function startWatch() {
    stopWatch()

    const url = buildWatchUrl()
    eventSource = new EventSource(url)

    eventSource.onopen = () => {
      reconnectMs = 3000 // reset backoff
      error.value = null
    }

    eventSource.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as WatchEvent<T>
        if (event.type && event.object) {
          mergeEvent(event)
        }
      } catch {
        // Ignore unparseable messages (heartbeats)
      }
    }

    eventSource.onerror = () => {
      stopWatch()
      scheduleReconnect()
    }
  }

  function stopWatch() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function scheduleReconnect() {
    reconnectTimer = setTimeout(() => {
      // Re-list then re-watch to ensure consistency
      start()
    }, reconnectMs)
    reconnectMs = Math.min(reconnectMs * 1.5, 60000)
  }

  /** Perform an initial list, then start watching */
  async function start() {
    stopWatch()
    status.value = 'pending'
    error.value = null

    try {
      const data = await $fetch<ListResponse<T>>(options.listUrl(), {
        headers: authHeaders()
      })
      items.value = (data.items ?? []).sort(sorter)
      status.value = 'success'

      // Start watching after successful list
      startWatch()
    } catch (e) {
      error.value = e instanceof Error ? e : new Error(String(e))
      status.value = 'error'
      scheduleReconnect()
    }
  }

  function stop() {
    stopWatch()
    status.value = 'idle'
  }

  /** Manually refresh the full list (useful for the refresh button) */
  async function refresh() {
    stopWatch()
    await start()
  }

  // Auto-start on mount
  onMounted(() => {
    start()
  })

  // Auto-cleanup on scope dispose
  onScopeDispose(() => {
    stop()
  })

  return {
    items: readonly(items) as Readonly<Ref<T[]>>,
    status: readonly(status),
    error: readonly(error),
    refresh,
    start,
    stop
  }
}
