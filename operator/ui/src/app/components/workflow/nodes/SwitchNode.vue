<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'
import type { RunPhase } from '~/types'

interface SwitchNodeData {
  label: string
  phase: RunPhase
}

const props = defineProps<NodeProps<SwitchNodeData>>()
</script>

<template>
  <div class="relative w-[80px] h-[80px]">
    <Handle type="target" :position="Position.Top" />

    <svg viewBox="0 0 80 80" class="absolute inset-0">
      <polygon
        points="40,4 76,40 40,76 4,40"
        class="fill-elevated stroke-default"
        stroke-width="2"
      />
    </svg>

    <div class="absolute inset-0 flex items-center justify-center">
      <span class="text-xs font-medium text-muted text-center leading-tight px-2 truncate">
        {{ props.data.label }}
      </span>
    </div>

    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>
