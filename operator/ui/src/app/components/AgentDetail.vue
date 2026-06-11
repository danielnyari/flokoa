<script setup lang="ts">
import type { Agent, Condition } from '~/types'
import { agentPhaseLabel, agentPhaseColor, normaliseTimestamp } from '~/utils/enums'

const props = defineProps<{
  agent: Agent
}>()

const open = defineModel<boolean>('open', { default: false })

const conditions = computed(() => props.agent.status?.conditions ?? [])

function conditionColor(c: Condition): 'success' | 'error' | 'warning' {
  if (c.status === 'True') return 'success'
  if (c.status === 'False') return 'error'
  return 'warning'
}

const toolRefs = computed(() => {
  return (props.agent.spec.tools ?? []).map(t => t.name)
})

const skills = computed(() => props.agent.spec.card?.skills ?? [])
</script>

<template>
  <USlideover v-model:open="open" title="Agent Details" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <div class="flex flex-col gap-6">
        <!-- Header -->
        <div class="flex items-center gap-3">
          <div class="flex items-center justify-center size-10 rounded-lg bg-primary/10">
            <UIcon name="i-lucide-bot" class="size-5 text-primary" />
          </div>
          <div>
            <h3 class="text-base font-semibold text-highlighted">
              {{ agent.metadata.name }}
            </h3>
            <p class="text-sm text-muted">
              {{ agent.metadata.namespace }}
            </p>
          </div>
          <UBadge
            :color="agentPhaseColor(agent.status?.phase)"
            variant="subtle"
            class="ml-auto"
          >
            {{ agentPhaseLabel(agent.status?.phase) }}
          </UBadge>
        </div>

        <!-- Overview -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Overview
          </h4>
          <div class="grid grid-cols-2 gap-3">
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Spec Hash
              </p>
              <p class="text-sm font-mono font-medium text-highlighted truncate">
                {{ agent.status?.specHash ?? '—' }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Runner
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.spec.runtime?.image ?? (agent.status?.runnerVersion ? `flokoa-runner ${agent.status.runnerVersion}` : '—') }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Replicas
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.status?.availableReplicas ?? 0 }}/{{ agent.status?.replicas ?? agent.spec.runtime?.replicas ?? 0 }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Age
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ normaliseTimestamp(agent.metadata.creationTimestamp) ? useTimeAgo(normaliseTimestamp(agent.metadata.creationTimestamp)!).value : '—' }}
              </p>
            </div>
          </div>
        </section>

        <!-- URL -->
        <section v-if="agent.status?.url">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Endpoint
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon name="i-lucide-globe" class="size-4 text-muted shrink-0" />
            <a
              :href="agent.status.url"
              target="_blank"
              rel="noopener noreferrer"
              class="text-sm font-mono truncate text-primary hover:underline"
            >{{ agent.status.url }}</a>
          </div>
        </section>

        <!-- Model Reference -->
        <section v-if="agent.spec.modelRef">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Model
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon name="i-lucide-brain" class="size-4 text-primary shrink-0" />
            <span class="text-sm font-medium text-highlighted">{{ agent.spec.modelRef.name }}</span>
            <span v-if="agent.spec.modelRef.namespace" class="text-xs text-muted">
              ({{ agent.spec.modelRef.namespace }})
            </span>
          </div>
        </section>

        <!-- Instructions -->
        <section v-if="(agent.spec.instructionRefs ?? []).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Instructions ({{ (agent.spec.instructionRefs ?? []).length }})
          </h4>
          <div class="flex flex-wrap gap-2">
            <UBadge
              v-for="(ref, i) in agent.spec.instructionRefs"
              :key="i"
              variant="outline"
              color="neutral"
            >
              <UIcon name="i-lucide-file-text" class="size-3 mr-1" />
              {{ ref.name }}
            </UBadge>
          </div>
        </section>

        <!-- Tools -->
        <section v-if="toolRefs.length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Tools ({{ toolRefs.length }})
          </h4>
          <div class="flex flex-wrap gap-2">
            <UBadge
              v-for="(tool, i) in toolRefs"
              :key="i"
              variant="outline"
              color="neutral"
            >
              <UIcon name="i-lucide-wrench" class="size-3 mr-1" />
              {{ tool }}
            </UBadge>
          </div>
        </section>

        <!-- A2A Card / Skills -->
        <section v-if="skills.length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            A2A Skills ({{ skills.length }})
          </h4>
          <div class="space-y-2">
            <div
              v-for="skill in skills"
              :key="skill.id"
              class="p-3 rounded-lg border border-default bg-elevated/50"
            >
              <div class="flex items-center gap-2 mb-1">
                <UIcon name="i-lucide-zap" class="size-3.5 text-primary" />
                <span class="text-sm font-medium text-highlighted">{{ skill.name }}</span>
                <span class="text-xs text-muted font-mono">{{ skill.id }}</span>
              </div>
              <p v-if="skill.description" class="text-xs text-muted">
                {{ skill.description }}
              </p>
              <div v-if="skill.tags?.length" class="flex flex-wrap gap-1 mt-1.5">
                <UBadge
                  v-for="tag in skill.tags"
                  :key="tag"
                  variant="subtle"
                  color="neutral"
                  size="xs"
                >
                  {{ tag }}
                </UBadge>
              </div>
            </div>
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
                {{ normaliseTimestamp(condition.lastTransitionTime) ? useTimeAgo(normaliseTimestamp(condition.lastTransitionTime)!).value : '' }}
              </p>
            </div>
          </div>
        </section>

        <!-- Labels & Annotations -->
        <section v-if="agent.metadata.labels && Object.keys(agent.metadata.labels).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Labels
          </h4>
          <div class="flex flex-wrap gap-1.5">
            <UBadge
              v-for="(value, key) in agent.metadata.labels"
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
