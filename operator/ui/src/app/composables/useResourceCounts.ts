import { createSharedComposable } from '@vueuse/core'
import type { Agent, Model, ModelProvider, AgentTool, AgentWorkflow } from '~/types'

/**
 * Shared composable that provides live resource counts for sidebar badges.
 * Uses the same list-watch pattern as individual pages so counts stay in
 * sync with the cluster via SSE.
 */
const _useResourceCounts = () => {
  const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()

  const { items: agents } = useListWatch<Agent>({
    listUrl: () => namespacedPath('agents'),
    watchUrl: () => buildWatchUrl('agents')
  })

  const { items: models } = useListWatch<Model>({
    listUrl: () => namespacedPath('models'),
    watchUrl: () => buildWatchUrl('models')
  })

  const { items: providers } = useListWatch<ModelProvider>({
    listUrl: () => namespacedPath('modelproviders'),
    watchUrl: () => buildWatchUrl('modelproviders')
  })

  const { items: tools } = useListWatch<AgentTool>({
    listUrl: () => namespacedPath('agenttools'),
    watchUrl: () => buildWatchUrl('agenttools')
  })

  const { items: workflows } = useListWatch<AgentWorkflow>({
    listUrl: () => namespacedPath('agentworkflows'),
    watchUrl: () => buildWatchUrl('agentworkflows')
  })

  return {
    agentCount: computed(() => agents.value.length),
    modelCount: computed(() => models.value.length),
    providerCount: computed(() => providers.value.length),
    toolCount: computed(() => tools.value.length),
    workflowCount: computed(() => workflows.value.length)
  }
}

export const useResourceCounts = createSharedComposable(_useResourceCounts)
