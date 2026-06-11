<script setup lang="ts">
import type { Model, Condition } from '~/types'

const props = defineProps<{
  model: Model
}>()

const open = defineModel<boolean>('open', { default: false })

const conditions = computed(() => props.model.status?.conditions ?? [])
const params = computed(() => props.model.spec.settings)

function conditionColor(c: Condition): 'success' | 'error' | 'warning' {
  if (c.status === 'True') return 'success'
  if (c.status === 'False') return 'error'
  return 'warning'
}

const paramEntries = computed(() => {
  if (!params.value) return []
  const entries: { label: string, value: string }[] = []
  if (params.value.temperature !== undefined) entries.push({ label: 'Temperature', value: String(params.value.temperature) })
  if (params.value.maxTokens !== undefined) entries.push({ label: 'Max Tokens', value: params.value.maxTokens.toLocaleString() })
  if (params.value.topP !== undefined) entries.push({ label: 'Top P', value: String(params.value.topP) })
  if (params.value.topK !== undefined) entries.push({ label: 'Top K', value: String(params.value.topK) })
  if (params.value.presencePenalty !== undefined) entries.push({ label: 'Presence Penalty', value: String(params.value.presencePenalty) })
  if (params.value.frequencyPenalty !== undefined) entries.push({ label: 'Frequency Penalty', value: String(params.value.frequencyPenalty) })
  if (params.value.timeoutSeconds !== undefined) entries.push({ label: 'Timeout', value: `${params.value.timeoutSeconds}s` })
  if (params.value.parallelToolCalls !== undefined) entries.push({ label: 'Parallel Tool Calls', value: params.value.parallelToolCalls ? 'Yes' : 'No' })
  if (params.value.seed !== undefined) entries.push({ label: 'Seed', value: String(params.value.seed) })
  if (params.value.stopSequences?.length) entries.push({ label: 'Stop Sequences', value: params.value.stopSequences.join(', ') })
  return entries
})
</script>

<template>
  <USlideover v-model:open="open" title="Model Details" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <div class="flex flex-col gap-6">
        <!-- Header -->
        <div class="flex items-center gap-3">
          <div class="flex items-center justify-center size-10 rounded-lg bg-primary/10">
            <UIcon name="i-lucide-brain" class="size-5 text-primary" />
          </div>
          <div>
            <h3 class="text-base font-semibold text-highlighted">
              {{ model.metadata.name }}
            </h3>
            <p class="text-sm text-muted">
              {{ model.metadata.namespace }}
            </p>
          </div>
          <UBadge
            v-if="model.status?.ready !== undefined"
            :color="model.status.ready ? 'success' : 'error'"
            variant="subtle"
            class="ml-auto"
          >
            {{ model.status.ready ? 'Ready' : 'Not Ready' }}
          </UBadge>
        </div>

        <!-- Model ID -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Model ID
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <span class="text-sm font-mono text-highlighted">{{ model.spec.model }}</span>
          </div>
        </section>

        <!-- Provider Reference -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Provider
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <div class="flex items-center gap-2">
              <UIcon name="i-lucide-cloud" class="size-4 text-primary shrink-0" />
              <span class="text-sm font-medium text-highlighted">{{ model.spec.providerRef?.name ?? '—' }}</span>
              <span v-if="model.spec.providerRef?.namespace" class="text-xs text-muted">
                ({{ model.spec.providerRef.namespace }})
              </span>
            </div>
            <div v-if="model.status?.resolvedProvider" class="mt-2 flex items-center gap-2">
              <UBadge variant="subtle" color="neutral" class="capitalize">
                {{ model.status.resolvedProvider.provider }}
              </UBadge>
              <span class="text-xs text-muted">
                {{ model.status.resolvedProvider.namespace }}/{{ model.status.resolvedProvider.name }}
              </span>
            </div>
          </div>
        </section>

        <!-- Parameters -->
        <section v-if="paramEntries.length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Parameters
          </h4>
          <div class="grid grid-cols-2 gap-3">
            <div
              v-for="entry in paramEntries"
              :key="entry.label"
              class="p-3 rounded-lg border border-default bg-elevated/50"
            >
              <p class="text-xs text-muted">
                {{ entry.label }}
              </p>
              <p class="text-sm font-medium text-highlighted font-mono">
                {{ entry.value }}
              </p>
            </div>
          </div>
        </section>

        <!-- Provider-Specific Overrides -->
        <section v-if="params?.openai || params?.anthropic || params?.google || params?.bedrock">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Provider Overrides
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <pre class="text-xs font-mono text-muted whitespace-pre-wrap break-words">{{ JSON.stringify(params?.openai ?? params?.anthropic ?? params?.google ?? params?.bedrock, null, 2) }}</pre>
          </div>
        </section>

        <!-- Conditions -->
        <section v-if="conditions.length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Conditions
          </h4>
          <div class="space-y-2">
            <div
              v-for="condition in conditions"
              :key="condition.type"
              class="p-3 rounded-lg border border-default bg-elevated/50"
            >
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-highlighted">{{ condition.type }}</span>
                <UBadge :color="conditionColor(condition)" variant="subtle" size="xs">
                  {{ condition.status }}
                </UBadge>
              </div>
              <p v-if="condition.reason" class="text-xs font-mono text-muted">
                {{ condition.reason }}
              </p>
              <p v-if="condition.message" class="text-xs text-muted mt-0.5">
                {{ condition.message }}
              </p>
              <p v-if="condition.lastTransitionTime" class="text-xs text-muted mt-1">
                {{ useTimeAgo(condition.lastTransitionTime).value }}
              </p>
            </div>
          </div>
        </section>

        <!-- Labels -->
        <section v-if="model.metadata.labels && Object.keys(model.metadata.labels).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Labels
          </h4>
          <div class="flex flex-wrap gap-1.5">
            <UBadge
              v-for="(value, key) in model.metadata.labels"
              :key="key"
              variant="subtle"
              color="neutral"
              size="xs"
            >
              {{ key }}={{ value }}
            </UBadge>
          </div>
        </section>
      </div>
    </template>
  </USlideover>
</template>
