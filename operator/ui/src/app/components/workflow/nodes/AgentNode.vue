<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'
import type { RunPhase } from '~/types'

interface AgentNodeData {
  label: string
  phase: RunPhase
  duration?: string
  selected?: boolean
}

const props = defineProps<NodeProps<AgentNodeData>>()

function phaseColor(phase?: RunPhase) {
  switch (phase) {
    case 'RUN_PHASE_RUNNING': return 'border-info'
    case 'RUN_PHASE_SUCCEEDED': return 'border-success'
    case 'RUN_PHASE_FAILED': return 'border-error'
    case 'RUN_PHASE_ERROR': return 'border-error'
    case 'RUN_PHASE_PENDING': return 'border-default'
    default: return 'border-default'
  }
}

function phaseBadgeColor(phase?: RunPhase): 'success' | 'error' | 'warning' | 'info' | 'neutral' {
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
</script>

<template>
  <div
    class="rounded-lg border-2 bg-elevated px-4 py-3 w-[200px] shadow-sm transition-all"
    :class="[
      phaseColor(props.data.phase),
      props.data.selected ? 'ring-2 ring-primary' : ''
    ]"
  >
    <Handle type="target" :position="Position.Top" />

    <div class="flex items-center gap-2 mb-1.5">
      <span
        v-if="props.data.phase === 'RUN_PHASE_RUNNING'"
        class="size-2 rounded-full bg-info animate-pulse shrink-0"
      />
      <UIcon v-else name="i-lucide-bot" class="size-3.5 text-muted shrink-0" />
      <span class="text-sm font-medium text-highlighted truncate">{{ props.data.label }}</span>
    </div>

    <div class="flex items-center justify-between">
      <UBadge :color="phaseBadgeColor(props.data.phase)" variant="subtle" size="xs">
        {{ phaseLabel(props.data.phase) }}
      </UBadge>
      <span v-if="props.data.duration" class="text-xs text-muted">{{ props.data.duration }}</span>
    </div>

    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>
