import { createSharedComposable } from '@vueuse/core'

/**
 * Shared composable that provides resource counts for sidebar badges.
 * Uses a single deferred fetch (after mount) so it never blocks app
 * initialisation or the NuxtLoadingIndicator.
 */
const _useResourceCounts = () => {
  const { namespacedPath } = useFlokoa()

  const agentCount = ref(0)
  const modelCount = ref(0)
  const providerCount = ref(0)
  const toolCount = ref(0)
  const workflowCount = ref(0)

  async function refresh() {
    const auth = useAuth()
    const headers: Record<string, string> = {}
    const token = auth.getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`

    const results = await Promise.allSettled([
      $fetch<{ items?: unknown[] }>(namespacedPath('agents'), { headers }),
      $fetch<{ items?: unknown[] }>(namespacedPath('models'), { headers }),
      $fetch<{ items?: unknown[] }>(namespacedPath('modelproviders'), { headers }),
      $fetch<{ items?: unknown[] }>(namespacedPath('agenttools'), { headers }),
      $fetch<{ items?: unknown[] }>(namespacedPath('agentworkflows'), { headers })
    ])

    const counts = results.map(r => r.status === 'fulfilled' ? (r.value.items?.length ?? 0) : 0)
    agentCount.value = counts[0]
    modelCount.value = counts[1]
    providerCount.value = counts[2]
    toolCount.value = counts[3]
    workflowCount.value = counts[4]
  }

  // Defer fetch until after the app has fully mounted
  onMounted(() => refresh())

  return { agentCount, modelCount, providerCount, toolCount, workflowCount, refresh }
}

export const useResourceCounts = createSharedComposable(_useResourceCounts)
