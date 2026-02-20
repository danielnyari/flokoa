<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'

interface RootNodeData {
  label: string
}

const props = defineProps<NodeProps<RootNodeData>>()
</script>

<template>
  <div class="flex items-center justify-center size-[60px] rounded-full border-2 border-default bg-elevated shadow-sm">
    <span class="text-xs font-medium text-muted text-center leading-tight px-1 truncate">
      {{ props.data.label }}
    </span>
    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>
