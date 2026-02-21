<script setup lang="ts">
import type { Agent, Model, ModelProvider, AgentTool, AgentWorkflow } from '~/types'

const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()

const { items: agents, status: agentStatus, refresh: refreshAgents } = useListWatch<Agent>({
  listUrl: () => namespacedPath('agents'),
  watchUrl: () => buildWatchUrl('agents')
})

const { items: models, status: modelStatus, refresh: refreshModels } = useListWatch<Model>({
  listUrl: () => namespacedPath('models'),
  watchUrl: () => buildWatchUrl('models')
})

const { items: providers, status: providerStatus, refresh: refreshProviders } = useListWatch<ModelProvider>({
  listUrl: () => namespacedPath('modelproviders'),
  watchUrl: () => buildWatchUrl('modelproviders')
})

const { items: tools, status: toolStatus, refresh: refreshTools } = useListWatch<AgentTool>({
  listUrl: () => namespacedPath('agenttools'),
  watchUrl: () => buildWatchUrl('agenttools')
})

const { items: workflows, status: workflowStatus, refresh: refreshWorkflows } = useListWatch<AgentWorkflow>({
  listUrl: () => namespacedPath('agentworkflows'),
  watchUrl: () => buildWatchUrl('agentworkflows')
})

const loading = computed(() =>
  agentStatus.value === 'pending'
  || modelStatus.value === 'pending'
  || providerStatus.value === 'pending'
  || toolStatus.value === 'pending'
  || workflowStatus.value === 'pending'
)

function refreshAll() {
  refreshAgents()
  refreshModels()
  refreshProviders()
  refreshTools()
  refreshWorkflows()
}

const runningAgents = computed(() => agents.value.filter(a => a.status?.phase === 'Running').length)
const pendingAgents = computed(() => agents.value.filter(a => a.status?.phase === 'Pending').length)
const failedAgents = computed(() => agents.value.filter(a => a.status?.phase === 'Failed').length)
const readyModels = computed(() => models.value.filter(m => m.status?.ready).length)
const readyProviders = computed(() => providers.value.filter(p => p.status?.ready).length)
const readyWorkflows = computed(() => workflows.value.filter(w => w.status?.ready).length)

const stats = computed(() => [
  {
    title: 'Agents',
    icon: 'i-lucide-bot',
    value: agents.value.length,
    description: `${runningAgents.value} running, ${failedAgents.value} failed`,
    to: '/agents'
  },
  {
    title: 'Models',
    icon: 'i-lucide-brain',
    value: models.value.length,
    description: `${readyModels.value} ready`,
    to: '/models'
  },
  {
    title: 'Providers',
    icon: 'i-lucide-cloud',
    value: providers.value.length,
    description: `${readyProviders.value} ready`,
    to: '/providers'
  },
  {
    title: 'Tools',
    icon: 'i-lucide-wrench',
    value: tools.value.length,
    description: `${tools.value.length} configured`,
    to: '/tools'
  },
  {
    title: 'Workflows',
    icon: 'i-lucide-git-branch',
    value: workflows.value.length,
    description: `${readyWorkflows.value} ready`,
    to: '/workflows'
  }
])

const recentAgents = computed(() =>
  [...agents.value]
    .sort((a, b) => {
      const ta = a.metadata.creationTimestamp ?? ''
      const tb = b.metadata.creationTimestamp ?? ''
      return tb.localeCompare(ta)
    })
    .slice(0, 5)
)

// Aggregate unique namespaces from all resources
const namespaces = computed(() => {
  const ns = new Set<string>()
  agents.value.forEach(a => ns.add(a.metadata.namespace))
  models.value.forEach(m => ns.add(m.metadata.namespace))
  providers.value.forEach(p => ns.add(p.metadata.namespace))
  tools.value.forEach(t => ns.add(t.metadata.namespace))
  return [...ns].sort()
})

// Provider type breakdown
const providerBreakdown = computed(() => {
  const counts: Record<string, number> = {}
  providers.value.forEach((p) => {
    let type = p.status?.provider ?? 'unknown'
    if (type === 'unknown') {
      if (p.spec.openai) type = 'openai'
      else if (p.spec.anthropic) type = 'anthropic'
      else if (p.spec.google) type = 'google'
      else if (p.spec.bedrock) type = 'bedrock'
    }
    counts[type] = (counts[type] || 0) + 1
  })
  return counts
})
</script>

<template>
  <UDashboardPanel id="home">
    <template #header>
      <UDashboardNavbar title="Overview" icon="i-lucide-layout-dashboard">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #trailing>
          <UButton
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="ghost"
            :loading="loading"
            @click="refreshAll()"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <!-- Stats cards -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <NuxtLink
          v-for="stat in stats"
          :key="stat.title"
          :to="stat.to"
          class="flex items-center gap-4 p-4 rounded-lg border border-default bg-elevated/50 hover:bg-elevated transition-colors"
        >
          <div class="flex items-center justify-center size-10 rounded-lg bg-primary/10">
            <UIcon :name="stat.icon" class="size-5 text-primary" />
          </div>
          <div>
            <div class="flex items-baseline gap-2">
              <span class="text-2xl font-semibold text-highlighted">
                <template v-if="loading">
                  ...
                </template>
                <template v-else>
                  {{ stat.value }}
                </template>
              </span>
              <span class="text-sm font-medium text-muted">{{ stat.title }}</span>
            </div>
            <p class="text-xs text-muted">
              {{ stat.description }}
            </p>
          </div>
        </NuxtLink>
      </div>

      <!-- Health + Info panels -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-6">
        <!-- Agent Health -->
        <div class="p-5 rounded-lg border border-default bg-elevated/50">
          <h3 class="text-sm font-semibold text-highlighted mb-4">
            Agent Health
          </h3>
          <div v-if="loading" class="text-sm text-muted">
            Loading...
          </div>
          <div v-else-if="agents.length === 0" class="text-sm text-muted text-center py-6">
            No agents deployed yet.
          </div>
          <HealthDonut
            v-else
            :running="runningAgents"
            :pending="pendingAgents"
            :failed="failedAgents"
          />
        </div>

        <!-- Platform Info -->
        <div class="p-5 rounded-lg border border-default bg-elevated/50">
          <h3 class="text-sm font-semibold text-highlighted mb-4">
            Platform Info
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted">Namespaces</span>
              <div class="flex flex-wrap gap-1 justify-end">
                <UBadge
                  v-for="ns in namespaces"
                  :key="ns"
                  variant="subtle"
                  color="neutral"
                  size="xs"
                >
                  {{ ns }}
                </UBadge>
                <span v-if="namespaces.length === 0" class="text-muted">—</span>
              </div>
            </div>
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted">Provider Types</span>
              <div class="flex flex-wrap gap-1 justify-end">
                <UBadge
                  v-for="(count, type) in providerBreakdown"
                  :key="type"
                  variant="subtle"
                  color="neutral"
                  size="xs"
                  class="capitalize"
                >
                  {{ type }} ({{ count }})
                </UBadge>
                <span v-if="Object.keys(providerBreakdown).length === 0" class="text-muted">—</span>
              </div>
            </div>
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted">Models Ready</span>
              <span class="font-medium text-highlighted">{{ readyModels }}/{{ models.length }}</span>
            </div>
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted">Providers Ready</span>
              <span class="font-medium text-highlighted">{{ readyProviders }}/{{ providers.length }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Agents -->
      <div class="mt-6">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-highlighted">
            Recent Agents
          </h3>
          <UButton
            label="View all"
            variant="ghost"
            color="neutral"
            size="sm"
            trailing-icon="i-lucide-arrow-right"
            to="/agents"
          />
        </div>

        <div v-if="loading" class="text-sm text-muted">
          Loading...
        </div>
        <div v-else-if="recentAgents.length === 0" class="text-sm text-muted p-8 border border-default rounded-lg text-center">
          <UIcon name="i-lucide-bot" class="size-8 text-muted mb-2 mx-auto block" />
          <p class="font-medium text-highlighted mb-1">
            No agents found
          </p>
          <p>Deploy your first agent to get started.</p>
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="agent in recentAgents"
            :key="agent.metadata.uid ?? agent.metadata.name"
            class="flex items-center justify-between p-3 rounded-lg border border-default bg-elevated/50"
          >
            <div class="flex items-center gap-3">
              <div class="flex items-center justify-center size-8 rounded-md bg-primary/10">
                <UIcon name="i-lucide-bot" class="size-4 text-primary" />
              </div>
              <div>
                <p class="text-sm font-medium text-highlighted">
                  {{ agent.metadata.name }}
                </p>
                <p class="text-xs text-muted">
                  {{ agent.metadata.namespace }}
                  <template v-if="agent.spec.framework || agent.status?.detectedFramework">
                    &middot; {{ agent.spec.framework ?? agent.status?.detectedFramework }}
                  </template>
                </p>
              </div>
            </div>
            <div class="flex items-center gap-3">
              <UBadge
                :color="
                  agent.status?.phase === 'Running' ? 'success'
                  : agent.status?.phase === 'Failed' ? 'error'
                    : 'warning'
                "
                variant="subtle"
                class="capitalize"
              >
                {{ agent.status?.phase ?? 'Unknown' }}
              </UBadge>
              <span v-if="agent.metadata.creationTimestamp" class="text-xs text-muted">
                {{ useTimeAgo(agent.metadata.creationTimestamp).value }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>
