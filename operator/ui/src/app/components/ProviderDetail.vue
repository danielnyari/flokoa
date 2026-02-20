<script setup lang="ts">
import type { ModelProvider, Condition } from '~/types'

const props = defineProps<{
  provider: ModelProvider
}>()

const open = defineModel<boolean>('open', { default: false })

const conditions = computed(() => props.provider.status?.conditions ?? [])

function conditionColor(c: Condition): 'success' | 'error' | 'warning' {
  if (c.status === 'True') return 'success'
  if (c.status === 'False') return 'error'
  return 'warning'
}

function getProviderType(): string {
  const p = props.provider
  if (p.status?.provider) return p.status.provider
  if (p.spec.openai) return 'openai'
  if (p.spec.anthropic) return 'anthropic'
  if (p.spec.google) return 'google'
  if (p.spec.bedrock) return 'bedrock'
  return 'unknown'
}

function getProviderIcon(type: string): string {
  const icons: Record<string, string> = {
    openai: 'i-simple-icons-openai',
    anthropic: 'i-simple-icons-anthropic',
    google: 'i-simple-icons-google',
    bedrock: 'i-simple-icons-amazonaws'
  }
  return icons[type] ?? 'i-lucide-cloud'
}

const configEntries = computed(() => {
  const spec = props.provider.spec
  const entries: { label: string, value: string }[] = []

  if (spec.openai?.baseURL) entries.push({ label: 'Base URL', value: spec.openai.baseURL })
  if (spec.anthropic?.baseURL) entries.push({ label: 'Base URL', value: spec.anthropic.baseURL })
  if (spec.google?.project) entries.push({ label: 'Project', value: spec.google.project })
  if (spec.google?.location) entries.push({ label: 'Location', value: spec.google.location })
  if (spec.bedrock?.region) entries.push({ label: 'Region', value: spec.bedrock.region })

  return entries
})
</script>

<template>
  <USlideover v-model:open="open" title="Provider Details" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <div class="flex flex-col gap-6">
        <!-- Header -->
        <div class="flex items-center gap-3">
          <div class="flex items-center justify-center size-10 rounded-lg bg-primary/10">
            <UIcon :name="getProviderIcon(getProviderType())" class="size-5 text-primary" />
          </div>
          <div>
            <h3 class="text-base font-semibold text-highlighted">
              {{ provider.metadata.name }}
            </h3>
            <p class="text-sm text-muted">
              {{ provider.metadata.namespace }}
            </p>
          </div>
          <UBadge
            v-if="provider.status?.ready !== undefined"
            :color="provider.status.ready ? 'success' : 'error'"
            variant="subtle"
            class="ml-auto"
          >
            {{ provider.status.ready ? 'Ready' : 'Not Ready' }}
          </UBadge>
        </div>

        <!-- Provider Type -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Provider Type
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon :name="getProviderIcon(getProviderType())" class="size-4" />
            <UBadge variant="subtle" color="neutral" class="capitalize">
              {{ getProviderType() }}
            </UBadge>
          </div>
        </section>

        <!-- Configuration -->
        <section v-if="configEntries.length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Configuration
          </h4>
          <div class="space-y-2">
            <div
              v-for="entry in configEntries"
              :key="entry.label"
              class="p-3 rounded-lg border border-default bg-elevated/50"
            >
              <p class="text-xs text-muted">
                {{ entry.label }}
              </p>
              <p class="text-sm font-mono text-highlighted break-all">
                {{ entry.value }}
              </p>
            </div>
          </div>
        </section>

        <!-- API Key -->
        <section v-if="provider.spec.apiKeySecretRef">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            API Key Secret
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50 flex items-center gap-2">
            <UIcon name="i-lucide-key-round" class="size-4 text-muted shrink-0" />
            <span class="text-sm font-mono">{{ provider.spec.apiKeySecretRef.name }}</span>
            <span v-if="provider.spec.apiKeySecretRef.key" class="text-xs text-muted">
              (key: {{ provider.spec.apiKeySecretRef.key }})
            </span>
          </div>
        </section>

        <!-- TLS -->
        <section>
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            TLS
          </h4>
          <div class="p-3 rounded-lg border border-default bg-elevated/50">
            <div v-if="!provider.spec.tls" class="text-sm text-muted">
              Default (system CAs)
            </div>
            <div v-else class="space-y-2">
              <div class="flex items-center gap-2">
                <UIcon
                  :name="provider.spec.tls.insecureSkipVerify ? 'i-lucide-shield-off' : 'i-lucide-shield-check'"
                  :class="provider.spec.tls.insecureSkipVerify ? 'text-warning' : 'text-success'"
                  class="size-4"
                />
                <span class="text-sm">
                  {{ provider.spec.tls.insecureSkipVerify ? 'TLS verification disabled' : 'TLS verification enabled' }}
                </span>
              </div>
              <div v-if="provider.spec.tls.useSystemCAs !== undefined" class="flex items-center gap-2">
                <UIcon name="i-lucide-lock" class="size-4 text-muted" />
                <span class="text-sm text-muted">
                  System CAs: {{ provider.spec.tls.useSystemCAs ? 'Yes' : 'No' }}
                </span>
              </div>
            </div>
          </div>
        </section>

        <!-- Default Headers -->
        <section v-if="provider.spec.defaultHeaders && Object.keys(provider.spec.defaultHeaders).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Default Headers
          </h4>
          <div class="space-y-1">
            <div
              v-for="(value, key) in provider.spec.defaultHeaders"
              :key="key"
              class="p-2 rounded border border-default bg-elevated/50 flex items-center gap-2"
            >
              <span class="text-xs font-mono font-medium text-highlighted">{{ key }}:</span>
              <span class="text-xs font-mono text-muted truncate">{{ value }}</span>
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
        <section v-if="provider.metadata.labels && Object.keys(provider.metadata.labels).length > 0">
          <h4 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
            Labels
          </h4>
          <div class="flex flex-wrap gap-1.5">
            <UBadge
              v-for="(value, key) in provider.metadata.labels"
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
