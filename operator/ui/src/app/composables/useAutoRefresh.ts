import { createSharedComposable } from '@vueuse/core'

const _useAutoRefresh = () => {
  const enabled = ref(false)
  const intervalMs = ref(30_000) // 30 seconds default
  const callbacks = new Set<() => void>()
  let timer: ReturnType<typeof setInterval> | null = null

  function start() {
    stop()
    timer = setInterval(() => {
      callbacks.forEach(cb => cb())
    }, intervalMs.value)
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  watch(enabled, (val) => {
    if (val) start()
    else stop()
  })

  watch(intervalMs, () => {
    if (enabled.value) start()
  })

  function register(cb: () => void) {
    callbacks.add(cb)
    return () => callbacks.delete(cb)
  }

  // Clean up on unmount of the root component
  onScopeDispose(() => {
    stop()
    callbacks.clear()
  })

  return {
    enabled,
    intervalMs,
    register
  }
}

export const useAutoRefresh = createSharedComposable(_useAutoRefresh)
