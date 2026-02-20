<script setup lang="ts">
import type { Agent, Condition } from '~/types'

const props = defineProps<{
  agent: Agent
}>()

const open = defineModel<boolean>('open', { default: false })

const conditions = computed(() => props.agent.status?.conditions ?? [])

function conditionColor(c: Condition): 'success' | 'error' | 'warning' | 'neutral' {
  if (c.status === 'True') return 'success'
  if (c.status === 'False') return 'error'
  return 'warning'
}

const toolRefs = computed(() => {
  return (props.agent.spec.tools ?? []).map((t) => {
    if (t.toolRef) return t.toolRef.name
    if (t.name) return t.name
    return 'inline'
  })
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
            :color="
              agent.status?.phase === 'Running' ? 'success'
                : agent.status?.phase === 'Failed' ? 'error'
                  : 'warning'
            "
            variant="subtle"
            class="capitalize ml-auto"
          >
            {{ agent.status?.phase ?? 'Unknown' }}
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
                Framework
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.spec.framework ?? agent.status?.detectedFramework ?? '—' }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Runtime
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.spec.runtime?.type ?? '—' }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Replicas
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.status?.availableReplicas ?? 0 }}/{{ agent.status?.replicas ?? agent.spec.runtime?.standard?.replicas ?? 0 }}
              </p>
            </div>
            <div class="p-3 rounded-lg border border-default bg-elevated/50">
              <p class="text-xs text-muted">
                Age
              </p>
              <p class="text-sm font-medium text-highlighted">
                {{ agent.metadata.creationTimestamp ? useTimeAgo(agent.metadata.creationTimestamp).value : '—' }}
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
            <span class="text-sm font-mono truncate">{{ agent.status.url }}</span>
          </div>
        </section>

        <!-- Model Reference -->
        <section v-if="agent.spec.model">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Model
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon name="i-lucide-brain" class="size-4 text-primary shrink-0" />
            <span class="text-sm font-medium text-highlighted">{{ agent.spec.model.name }}</span>
            <span v-if="agent.spec.model.namespace" class="text-xs text-muted">
              ({{ agent.spec.model.namespace }})
            </span>
          </div>
        </section>

        <!-- Instruction -->
        <section v-if="agent.spec.instruction">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Instruction
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <div v-if="agent.spec.instruction.instructionRef" class="flex items-center gap-2">
              <UIcon name="i-lucide-file-text" class="size-4 text-muted shrink-0" />
              <span class="text-sm font-medium">{{ agent.spec.instruction.instructionRef.name }}</span>
            </div>
            <p v-else-if="agent.spec.instruction.template" class="text-sm whitespace-pre-wrap break-words">
              {{ agent.spec.instruction.template }}
            </p>
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
                <UBadge v-for="tag in skill.tags" :key="tag" variant="subtle" color="neutral" size="xs">
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
                {{ useTimeAgo(condition.lastTransitionTime).value }}
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
