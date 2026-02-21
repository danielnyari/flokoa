<script setup lang="ts">
import type { WorkflowRun } from '~/types'
import type { WatchEvent } from '~/composables/useListWatch'

const route = useRoute()

const ns = route.params.namespace as string
const workflowName = route.params.name as string
const runName = route.params.run as string

const { getWorkflowRun, getAgentWorkflow, watchWorkflowRunsUrl } = useFlokoa()
const { data: run, status: runStatus, refresh: refreshRun } = await getWorkflowRun(ns, workflowName, runName)
const { data: workflow } = await getAgentWorkflow(ns, workflowName)

const selectedNodeId = ref<string | null>(null)

const tasks = computed(() => {
  return (workflow.value?.spec.tasks ?? []).map(t => ({
    name: t.name,
    type: t.type
  }))
})

// Real-time updates via SSE watch - replaces polling.
// Watches all runs for this workflow and filters for our specific run.
const { connect } = useEventSource(watchWorkflowRunsUrl(ns, workflowName), {
  onEvent: (data) => {
    const event = data as WatchEvent<WorkflowRun>
    if (event.object?.metadata?.name === runName) {
      run.value = event.object
    }
  }
})

onMounted(() => {
  connect()
})
</script>

<template>
  <UDashboardPanel id="run-detail">
    <template #header>
      <UDashboardNavbar :title="runName" icon="i-lucide-play">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #trailing>
          <UButton
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="ghost"
            :loading="runStatus === 'pending'"
            @click="refreshRun()"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <!-- Breadcrumb -->
      <UBreadcrumb
        :items="[
          { label: 'Workflows', to: '/workflows', icon: 'i-lucide-git-branch' },
          { label: workflowName, to: `/workflows/${ns}/${workflowName}` },
          { label: runName }
        ]"
        class="mb-2"
      />

      <div v-if="!run && runStatus !== 'pending'" class="text-sm text-muted p-8 text-center">
        Run not found.
      </div>

      <template v-else-if="run">
        <WorkflowStatusBar :run="run" />

        <div class="flex flex-1 overflow-hidden" style="height: calc(100vh - 220px)">
          <!-- DAG canvas -->
          <div class="flex-1 min-w-0">
            <WorkflowDAG
              v-if="run.nodes && run.nodes.length > 0"
              v-model:selected-node="selectedNodeId"
              :run="run"
              :tasks="tasks"
            />
            <div v-else class="flex items-center justify-center h-full text-muted text-sm">
              <div class="text-center">
                <UIcon name="i-lucide-git-branch" class="size-8 mb-2 mx-auto block" />
                <p>No nodes available yet.</p>
              </div>
            </div>
          </div>

          <!-- Detail panel -->
          <div class="w-80 shrink-0">
            <WorkflowDetailPanel
              :run="run"
              :selected-node-id="selectedNodeId"
            />
          </div>
        </div>
      </template>
    </template>
  </UDashboardPanel>
</template>
