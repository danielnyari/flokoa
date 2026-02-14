<script setup lang="ts">
const { listAgents, listModels, listModelProviders, listAgentTools } = useFlokoa()

const { data: agentList, status: agentStatus } = await listAgents()
const { data: modelList, status: modelStatus } = await listModels()
const { data: providerList, status: providerStatus } = await listModelProviders()
const { data: toolList, status: toolStatus } = await listAgentTools()

const agents = computed(() => agentList.value?.items ?? [])
const models = computed(() => modelList.value?.items ?? [])
const providers = computed(() => providerList.value?.items ?? [])
const tools = computed(() => toolList.value?.items ?? [])

const loading = computed(() =>
  agentStatus.value === 'pending'
  || modelStatus.value === 'pending'
  || providerStatus.value === 'pending'
  || toolStatus.value === 'pending'
)

const runningAgents = computed(() => agents.value.filter(a => a.status?.phase === 'Running').length)
const failedAgents = computed(() => agents.value.filter(a => a.status?.phase === 'Failed').length)
const readyModels = computed(() => models.value.filter(m => m.status?.ready).length)
const readyProviders = computed(() => providers.value.filter(p => p.status?.ready).length)

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
</script>

<template>
  <UDashboardPanel id="home">
    <template #header>
      <UDashboardNavbar title="Overview" icon="i-lucide-layout-dashboard">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <!-- Stats cards -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
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
        <div v-else-if="recentAgents.length === 0" class="text-sm text-muted p-4 border border-default rounded-lg text-center">
          No agents found. Deploy your first agent to get started.
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
