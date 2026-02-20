<script setup lang="ts">
import { VisSingleContainer, VisDonut } from '@unovis/vue'

interface Segment {
  label: string
  value: number
  color: string
}

const props = defineProps<{
  running: number
  pending: number
  failed: number
}>()

const total = computed(() => props.running + props.pending + props.failed)

const data = computed<Segment[]>(() => {
  if (total.value === 0) return [{ label: 'No agents', value: 1, color: 'var(--color-zinc-300)' }]
  const items: Segment[] = []
  if (props.running > 0) items.push({ label: 'Running', value: props.running, color: 'var(--color-green-500)' })
  if (props.pending > 0) items.push({ label: 'Pending', value: props.pending, color: 'var(--color-yellow-500)' })
  if (props.failed > 0) items.push({ label: 'Failed', value: props.failed, color: 'var(--color-red-500)' })
  return items
})

const value = (d: Segment) => d.value
const color = (d: Segment) => d.color
</script>

<template>
  <div class="flex items-center gap-6">
    <div class="relative size-28 shrink-0">
      <VisSingleContainer :data="data" :height="112">
        <VisDonut
          :value="value"
          :color="color"
          :arc-width="12"
          :pad-angle="0.02"
          :corner-radius="4"
        />
      </VisSingleContainer>
      <div class="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
        <span class="text-2xl font-bold text-highlighted">{{ total }}</span>
        <span class="text-xs text-muted">{{ total === 1 ? 'agent' : 'agents' }}</span>
      </div>
    </div>
    <div class="flex flex-col gap-2">
      <div class="flex items-center gap-2">
        <span class="size-2.5 rounded-full bg-green-500" />
        <span class="text-sm text-muted">Running</span>
        <span class="text-sm font-medium text-highlighted ml-auto tabular-nums">{{ running }}</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="size-2.5 rounded-full bg-yellow-500" />
        <span class="text-sm text-muted">Pending</span>
        <span class="text-sm font-medium text-highlighted ml-auto tabular-nums">{{ pending }}</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="size-2.5 rounded-full bg-red-500" />
        <span class="text-sm text-muted">Failed</span>
        <span class="text-sm font-medium text-highlighted ml-auto tabular-nums">{{ failed }}</span>
      </div>
    </div>
  </div>
</template>
