<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'
import type { RunPhase } from '~/types'
import { runPhaseLabel, runPhaseColor, runPhaseBorderColor, isRunPhase } from '~/utils/enums'

interface AgentNodeData {
  label: string
  phase: RunPhase
  duration?: string
  selected?: boolean
}

const props = defineProps<NodeProps<AgentNodeData>>()
</script>

<template>
  <div
    class="rounded-lg border-2 bg-elevated px-4 py-3 w-[200px] shadow-sm transition-all"
    :class="[
      runPhaseBorderColor(props.data.phase),
      props.data.selected ? 'ring-2 ring-primary' : ''
    ]"
  >
    <Handle type="target" :position="Position.Top" />

    <div class="flex items-center gap-2 mb-1.5">
      <span
        v-if="isRunPhase(props.data.phase, 'Running')"
        class="size-2 rounded-full bg-info animate-pulse shrink-0"
      />
      <UIcon v-else name="i-lucide-bot" class="size-3.5 text-muted shrink-0" />
      <span class="text-sm font-medium text-highlighted truncate">{{ props.data.label }}</span>
    </div>

    <div class="flex items-center justify-between">
      <UBadge :color="runPhaseColor(props.data.phase)" variant="subtle" size="xs">
        {{ runPhaseLabel(props.data.phase) }}
      </UBadge>
      <span v-if="props.data.duration" class="text-xs text-muted">{{ props.data.duration }}</span>
    </div>

    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>
