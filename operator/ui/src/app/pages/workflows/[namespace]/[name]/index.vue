<script setup lang="ts">
import type { RunPhase, WorkflowRun } from '~/types'

const route = useRoute()
const toast = useToast()

const ns = route.params.namespace as string
const name = route.params.name as string

const { getAgentWorkflow, submitWorkflowRun, watchWorkflowRunsUrl } = useFlokoa()
const { data: workflow, status: wfStatus, refresh: refreshWorkflow } = await getAgentWorkflow(ns, name)

// Use list-watch for real-time run updates (replaces polling)
const { items: runs, status: runStatus, refresh: refreshRuns } = useListWatch<WorkflowRun>({
  listUrl: () => `/api/v1alpha1/namespaces/${ns}/agentworkflows/${name}/runs`,
  watchUrl: () => watchWorkflowRunsUrl(ns, name)
})

const submitting = ref(false)
async function handleSubmitRun() {
  submitting.value = true
  try {
    await submitWorkflowRun(ns, name)
    toast.add({ title: 'Run submitted', description: 'A new workflow run has been created.' })
    // No need to manually refresh — the SSE watch will pick up the new run
  } catch {
    toast.add({ title: 'Failed', description: 'Could not submit workflow run.', color: 'error' })
  } finally {
    submitting.value = false
  }
}

function runPhaseColor(phase?: RunPhase): 'success' | 'error' | 'warning' | 'neutral' | 'info' {
  switch (phase) {
    case 'RUN_PHASE_RUNNING': return 'info'
    case 'RUN_PHASE_SUCCEEDED': return 'success'
    case 'RUN_PHASE_FAILED': return 'error'
    case 'RUN_PHASE_ERROR': return 'error'
    case 'RUN_PHASE_PENDING': return 'warning'
    default: return 'neutral'
  }
}

function runPhaseLabel(phase?: RunPhase): string {
  switch (phase) {
    case 'RUN_PHASE_RUNNING': return 'Running'
    case 'RUN_PHASE_SUCCEEDED': return 'Succeeded'
    case 'RUN_PHASE_FAILED': return 'Failed'
    case 'RUN_PHASE_ERROR': return 'Error'
    case 'RUN_PHASE_PENDING': return 'Pending'
    default: return 'Unknown'
  }
}

function formatDuration(run: WorkflowRun): string {
  if (!run.startedAt) return '\u2014'
  const start = new Date(run.startedAt)
  const end = run.finishedAt ? new Date(run.finishedAt) : new Date()
  const seconds = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remaining = seconds % 60
  return `${minutes}m ${remaining}s`
}

function refreshAll() {
  refreshWorkflow()
  refreshRuns()
}
</script>

<template>
  <UDashboardPanel id="workflow-detail">
    <template #header>
      <UDashboardNavbar :title="name" icon="i-lucide-git-branch">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #trailing>
          <UButton
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="ghost"
            :loading="wfStatus === 'pending' || runStatus === 'pending'"
            @click="refreshAll()"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <!-- Breadcrumb -->
      <UBreadcrumb
        :items="[
          { label: 'Workflows', to: '/workflows', icon: 'i-lucide-git-branch' },
          { label: name }
        ]"
        class="mb-4"
      />

      <!-- Workflow metadata -->
      <div v-if="workflow" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <div class="p-3 rounded-lg border border-default bg-elevated/50">
          <p class="text-xs text-muted">
            Status
          </p>
          <UBadge
            :color="workflow.status?.ready ? 'success' : 'error'"
            variant="subtle"
            class="mt-1"
          >
            {{ workflow.status?.ready ? 'Ready' : 'Not Ready' }}
          </UBadge>
        </div>
        <div class="p-3 rounded-lg border border-default bg-elevated/50">
          <p class="text-xs text-muted">
            Namespace
          </p>
          <p class="text-sm font-medium text-highlighted">
            {{ ns }}
          </p>
        </div>
        <div class="p-3 rounded-lg border border-default bg-elevated/50">
          <p class="text-xs text-muted">
            Tasks
          </p>
          <p class="text-sm font-medium text-highlighted">
            {{ workflow.spec.tasks?.length ?? 0 }}
          </p>
        </div>
        <div class="p-3 rounded-lg border border-default bg-elevated/50">
          <p class="text-xs text-muted">
            Template
          </p>
          <p class="text-sm font-mono text-highlighted truncate">
            {{ workflow.status?.workflowTemplateName ?? '\u2014' }}
          </p>
        </div>
      </div>

      <div v-if="workflow?.spec.description" class="mb-6 p-3 rounded-lg border border-default bg-elevated/50">
        <p class="text-xs text-muted mb-1">
          Description
        </p>
        <p class="text-sm text-highlighted">
          {{ workflow.spec.description }}
        </p>
      </div>

      <!-- Runs section -->
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-highlighted">
          Runs ({{ runs.length }})
        </h3>
        <UButton
          label="Submit Run"
          icon="i-lucide-play"
          size="sm"
          :loading="submitting"
          :disabled="!workflow?.status?.ready"
          @click="handleSubmitRun"
        />
      </div>

      <div v-if="runs.length === 0 && runStatus !== 'pending'" class="text-sm text-muted p-8 border border-default rounded-lg text-center">
        <UIcon name="i-lucide-play-circle" class="size-8 text-muted mb-2 mx-auto block" />
        <p class="font-medium text-highlighted mb-1">
          No runs yet
        </p>
        <p>Submit a run to execute this workflow.</p>
      </div>

      <div v-else class="space-y-2">
        <NuxtLink
          v-for="run in runs"
          :key="run.metadata.uid ?? run.metadata.name"
          :to="`/workflows/${ns}/${name}/runs/${run.metadata.name}`"
          class="flex items-center justify-between p-3 rounded-lg border border-default bg-elevated/50 hover:bg-elevated transition-colors"
        >
          <div class="flex items-center gap-3">
            <div class="flex items-center justify-center size-8 rounded-md" :class="run.phase === 'RUN_PHASE_RUNNING' ? 'bg-info/10' : 'bg-primary/10'">
              <UIcon
                :name="run.phase === 'RUN_PHASE_RUNNING' ? 'i-lucide-loader' : 'i-lucide-play'"
                class="size-4"
                :class="[
                  run.phase === 'RUN_PHASE_RUNNING' ? 'text-info animate-spin' : 'text-primary'
                ]"
              />
            </div>
            <div>
              <p class="text-sm font-medium text-highlighted font-mono">
                {{ run.metadata.name }}
              </p>
              <p v-if="run.progress" class="text-xs text-muted">
                Progress: {{ run.progress }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-xs text-muted">{{ formatDuration(run) }}</span>
            <UBadge :color="runPhaseColor(run.phase)" variant="subtle">
              {{ runPhaseLabel(run.phase) }}
            </UBadge>
          </div>
        </NuxtLink>
      </div>
    </template>
  </UDashboardPanel>
</template>
