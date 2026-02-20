<script setup lang="ts">
import type { AgentTool, Condition } from '~/types'

const props = defineProps<{
  tool: AgentTool
}>()

const open = defineModel<boolean>('open', { default: false })

const conditions = computed(() => props.tool.status?.conditions ?? [])

function conditionColor(c: Condition): 'success' | 'error' | 'warning' {
  if (c.status === 'True') return 'success'
  if (c.status === 'False') return 'error'
  return 'warning'
}

const sourceInfo = computed(() => {
  const api = props.tool.spec.openApi
  if (!api) return null

  if (api.url) return { type: 'URL', value: api.url }
  if (api.serviceRef) {
    const ref = api.serviceRef
    const port = ref.port ?? ref.portName ?? ''
    return {
      type: 'Service',
      value: `${ref.name}${ref.namespace ? '.' + ref.namespace : ''}${port ? ':' + port : ''}`
    }
  }
  return null
})

const schemaInfo = computed(() => {
  const schema = props.tool.spec.openApi?.openApiSchema
  if (!schema) return null

  if (schema.endpointPath) return { type: 'Endpoint', value: schema.endpointPath }
  if (schema.valueFrom) return { type: 'ConfigMap', value: `${schema.valueFrom.name}:${schema.valueFrom.key}` }
  if (schema.value) return { type: 'Inline', value: JSON.stringify(schema.value, null, 2) }
  return null
})

const headers = computed(() => props.tool.spec.openApi?.headers ?? {})
</script>

<template>
  <USlideover v-model:open="open" title="Tool Details" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <div class="flex flex-col gap-6">
        <!-- Header -->
        <div class="flex items-center gap-3">
          <div class="flex items-center justify-center size-10 rounded-lg bg-primary/10">
            <UIcon name="i-lucide-wrench" class="size-5 text-primary" />
          </div>
          <div>
            <h3 class="text-base font-semibold text-highlighted">
              {{ tool.metadata.name }}
            </h3>
            <p class="text-sm text-muted">
              {{ tool.metadata.namespace }}
            </p>
          </div>
          <UBadge variant="outline" color="neutral" class="ml-auto uppercase text-xs">
            {{ tool.spec.type }}
          </UBadge>
        </div>

        <!-- Description -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Description
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <p class="text-sm">
              {{ tool.spec.description }}
            </p>
          </div>
        </section>

        <!-- Source -->
        <section v-if="sourceInfo">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Source
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <p class="text-xs text-muted mb-1">
              {{ sourceInfo.type }}
            </p>
            <div class="flex items-center gap-2">
              <UIcon
                :name="sourceInfo.type === 'URL' ? 'i-lucide-globe' : 'i-lucide-server'"
                class="size-4 text-muted shrink-0"
              />
              <span class="text-sm font-mono text-highlighted break-all">{{ sourceInfo.value }}</span>
            </div>
          </div>
        </section>

        <!-- Timeout -->
        <section v-if="tool.spec.openApi?.timeoutSeconds">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Timeout
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon name="i-lucide-clock" class="size-4 text-muted" />
            <span class="text-sm font-medium">{{ tool.spec.openApi.timeoutSeconds }}s</span>
          </div>
        </section>

        <!-- Headers -->
        <section v-if="Object.keys(headers).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Headers
          </h4>
          <div class="space-y-1">
            <div
              v-for="(value, key) in headers"
              :key="key"
              class="p-2 rounded border border-default bg-elevated/50 flex items-center gap-2"
            >
              <span class="text-xs font-mono font-medium text-highlighted">{{ key }}:</span>
              <span class="text-xs font-mono text-muted truncate">{{ value }}</span>
            </div>
          </div>
        </section>

        <!-- OpenAPI Schema -->
        <section v-if="schemaInfo">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            OpenAPI Schema
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <p class="text-xs text-muted mb-1">
              {{ schemaInfo.type }}
            </p>
            <pre
              v-if="schemaInfo.type === 'Inline'"
              class="text-xs font-mono text-muted whitespace-pre-wrap break-words max-h-64 overflow-auto"
            >{{ schemaInfo.value }}</pre>
            <div v-else class="flex items-center gap-2">
              <UIcon
                :name="schemaInfo.type === 'Endpoint' ? 'i-lucide-globe' : 'i-lucide-file-text'"
                class="size-4 text-muted shrink-0"
              />
              <span class="text-sm font-mono text-highlighted">{{ schemaInfo.value }}</span>
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

        <!-- Labels -->
        <section v-if="tool.metadata.labels && Object.keys(tool.metadata.labels).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Labels
          </h4>
          <div class="flex flex-wrap gap-1.5">
            <UBadge
              v-for="(value, key) in tool.metadata.labels"
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
