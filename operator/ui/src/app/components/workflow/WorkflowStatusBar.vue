<script setup lang="ts">
import type { WorkflowRun } from '~/types'
import { runPhaseLabel, runPhaseColor } from '~/utils/enums'

const props = defineProps<{
  run: WorkflowRun
}>()

const duration = computed(() => {
  if (!props.run.startedAt) return '\u2014'
  const start = new Date(props.run.startedAt)
  const end = props.run.finishedAt ? new Date(props.run.finishedAt) : new Date()
  const seconds = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remaining = seconds % 60
  return `${minutes}m ${remaining}s`
})

const progressPercent = computed(() => {
  if (!props.run.progress) return 0
  const parts = props.run.progress.split('/')
  if (parts.length !== 2) return 0
  const done = parseInt(parts[0] ?? '0')
  const total = parseInt(parts[1] ?? '0')
  if (total === 0) return 0
  return Math.round((done / total) * 100)
})
</script>

<template>
  <div class="flex items-center gap-4 px-4 py-2 border-b border-default bg-elevated/50">
    <UBadge :color="runPhaseColor(run.phase)" variant="subtle">
      {{ runPhaseLabel(run.phase) }}
    </UBadge>

    <span class="text-sm font-mono text-muted">{{ run.metadata.name }}</span>

    <span class="text-sm text-muted">{{ duration }}</span>

    <div v-if="run.progress" class="flex items-center gap-2 ml-auto">
      <span class="text-xs text-muted">{{ run.progress }}</span>
      <UProgress :value="progressPercent" size="sm" class="w-24" />
    </div>

    <span v-if="run.message" class="text-xs text-muted ml-auto truncate max-w-64">
      {{ run.message }}
    </span>
  </div>
</template>
