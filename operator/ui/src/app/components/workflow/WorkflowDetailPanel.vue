<script setup lang="ts">
import type { WorkflowRun, WorkflowRunNode, RunPhase } from '~/types'

const props = defineProps<{
  run: WorkflowRun
  selectedNodeId?: string | null
}>()

const selectedNode = computed<WorkflowRunNode | undefined>(() => {
  if (!props.selectedNodeId || !props.run.nodes) return undefined
  return props.run.nodes.find(n => n.id === props.selectedNodeId)
})

const activeTab = ref('overview')

function phaseColor(phase?: RunPhase): 'success' | 'error' | 'warning' | 'info' | 'neutral' {
  switch (phase) {
    case 'RUN_PHASE_RUNNING': return 'info'
    case 'RUN_PHASE_SUCCEEDED': return 'success'
    case 'RUN_PHASE_FAILED': return 'error'
    case 'RUN_PHASE_ERROR': return 'error'
    case 'RUN_PHASE_PENDING': return 'warning'
    default: return 'neutral'
  }
}

function phaseLabel(phase?: RunPhase): string {
  switch (phase) {
    case 'RUN_PHASE_RUNNING': return 'Running'
    case 'RUN_PHASE_SUCCEEDED': return 'Succeeded'
    case 'RUN_PHASE_FAILED': return 'Failed'
    case 'RUN_PHASE_ERROR': return 'Error'
    case 'RUN_PHASE_PENDING': return 'Pending'
    default: return 'Unknown'
  }
}

function nodeTypeLabel(type?: string): string {
  switch (type) {
    case 'NODE_TYPE_POD': return 'Pod'
    case 'NODE_TYPE_DAG': return 'DAG'
    case 'NODE_TYPE_PLUGIN': return 'Plugin'
    case 'NODE_TYPE_STEPS': return 'Steps'
    case 'NODE_TYPE_SUSPEND': return 'Suspend'
    default: return 'Unknown'
  }
}

function formatTime(ts?: string): string {
  if (!ts) return '\u2014'
  return new Date(ts).toLocaleString()
}

function nodeDuration(node: WorkflowRunNode): string {
  if (!node.startedAt) return '\u2014'
  const start = new Date(node.startedAt)
  const end = node.finishedAt ? new Date(node.finishedAt) : new Date()
  const seconds = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remaining = seconds % 60
  return `${minutes}m ${remaining}s`
}

const tabItems = [
  { label: 'Overview', value: 'overview' },
  { label: 'YAML', value: 'yaml' }
]
</script>

<template>
  <div class="flex flex-col h-full border-l border-default bg-elevated/25">
    <UTabs
      v-model="activeTab"
      :items="tabItems"
      variant="pill"
      size="sm"
      class="px-3 pt-3"
    />

    <div class="flex-1 overflow-y-auto px-3 py-3">
      <!-- Overview tab -->
      <div v-if="activeTab === 'overview'">
        <!-- Node selected -->
        <div v-if="selectedNode" class="space-y-4">
          <div>
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Node
            </h4>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-highlighted">{{ selectedNode.displayName || selectedNode.name }}</span>
                <UBadge :color="phaseColor(selectedNode.phase)" variant="subtle" size="xs">
                  {{ phaseLabel(selectedNode.phase) }}
                </UBadge>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Type
                  </p>
                  <p class="text-sm text-highlighted">
                    {{ nodeTypeLabel(selectedNode.type) }}
                  </p>
                </div>
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Duration
                  </p>
                  <p class="text-sm text-highlighted">
                    {{ nodeDuration(selectedNode) }}
                  </p>
                </div>
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Started
                  </p>
                  <p class="text-xs text-highlighted">
                    {{ formatTime(selectedNode.startedAt) }}
                  </p>
                </div>
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Finished
                  </p>
                  <p class="text-xs text-highlighted">
                    {{ formatTime(selectedNode.finishedAt) }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <div v-if="selectedNode.message">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Message
            </h4>
            <p class="text-sm text-muted whitespace-pre-wrap break-words p-2 rounded border border-default bg-elevated/50">
              {{ selectedNode.message }}
            </p>
          </div>

          <div v-if="selectedNode.inputs && Object.keys(selectedNode.inputs).length > 0">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Inputs
            </h4>
            <div class="space-y-1">
              <div
                v-for="(value, key) in selectedNode.inputs"
                :key="key"
                class="p-2 rounded border border-default bg-elevated/50"
              >
                <p class="text-xs font-mono text-muted">
                  {{ key }}
                </p>
                <p class="text-sm text-highlighted break-all">
                  {{ value }}
                </p>
              </div>
            </div>
          </div>

          <div v-if="selectedNode.outputs && Object.keys(selectedNode.outputs).length > 0">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Outputs
            </h4>
            <div class="space-y-1">
              <div
                v-for="(value, key) in selectedNode.outputs"
                :key="key"
                class="p-2 rounded border border-default bg-elevated/50"
              >
                <p class="text-xs font-mono text-muted">
                  {{ key }}
                </p>
                <p class="text-sm text-highlighted break-all">
                  {{ value }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- No node selected: show run overview -->
        <div v-else class="space-y-4">
          <div>
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Run
            </h4>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-mono text-highlighted">{{ run.metadata.name }}</span>
                <UBadge :color="phaseColor(run.phase)" variant="subtle" size="xs">
                  {{ phaseLabel(run.phase) }}
                </UBadge>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Started
                  </p>
                  <p class="text-xs text-highlighted">
                    {{ formatTime(run.startedAt) }}
                  </p>
                </div>
                <div class="p-2 rounded border border-default bg-elevated/50">
                  <p class="text-xs text-muted">
                    Finished
                  </p>
                  <p class="text-xs text-highlighted">
                    {{ formatTime(run.finishedAt) }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <div v-if="run.progress">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Progress
            </h4>
            <p class="text-sm text-highlighted">
              {{ run.progress }}
            </p>
          </div>

          <div v-if="run.parameters && Object.keys(run.parameters).length > 0">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Parameters
            </h4>
            <div class="space-y-1">
              <div
                v-for="(value, key) in run.parameters"
                :key="key"
                class="p-2 rounded border border-default bg-elevated/50"
              >
                <p class="text-xs font-mono text-muted">
                  {{ key }}
                </p>
                <p class="text-sm text-highlighted break-all">
                  {{ value }}
                </p>
              </div>
            </div>
          </div>

          <div v-if="run.message">
            <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
              Message
            </h4>
            <p class="text-sm text-muted whitespace-pre-wrap break-words p-2 rounded border border-default bg-elevated/50">
              {{ run.message }}
            </p>
          </div>

          <p class="text-xs text-muted mt-4">
            Click a node in the graph to view its details.
          </p>
        </div>
      </div>

      <!-- YAML tab -->
      <div v-else-if="activeTab === 'yaml'">
        <pre class="font-mono text-xs whitespace-pre-wrap break-words p-3 rounded border border-default bg-elevated/50 overflow-auto max-h-[calc(100vh-200px)]">{{ JSON.stringify(selectedNode ?? run, null, 2) }}</pre>
      </div>
    </div>
  </div>
</template>
