<script setup lang="ts">
import type { Agent, A2AAgentCard } from '~/types'

const props = defineProps<{
  agent: Agent
}>()

const open = defineModel<boolean>('open', { default: false })
const toast = useToast()

const cardUrl = computed(() => {
  const base = props.agent.status?.url
  if (!base) return null
  const normalized = base.endsWith('/') ? base.slice(0, -1) : base
  return `${normalized}/.well-known/agent.json`
})

const card = ref<A2AAgentCard | null>(null)
const fetchError = ref<string | null>(null)
const fetching = ref(false)

async function fetchCard() {
  if (!cardUrl.value) return
  card.value = null
  fetchError.value = null
  fetching.value = true
  try {
    const data = await $fetch<A2AAgentCard>(cardUrl.value)
    card.value = data
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    fetchError.value = `Failed to fetch ${cardUrl.value}: ${msg}`
  } finally {
    fetching.value = false
  }
}

watch(open, (isOpen) => {
  if (isOpen) {
    fetchCard()
  }
})

const capabilities = computed(() => {
  const caps = card.value?.capabilities
  if (!caps) return []
  return [
    { label: 'Streaming', enabled: !!caps.streaming },
    { label: 'Push Notifications', enabled: !!caps.pushNotifications },
    { label: 'State History', enabled: !!caps.stateTransitionHistory }
  ]
})

const skillItems = computed(() => {
  return (card.value?.skills ?? []).map(skill => ({
    label: skill.name,
    icon: 'i-lucide-zap' as const,
    value: skill.id,
    content: skill.description || 'No description',
    slot: skill.id as string
  }))
})

const initials = computed(() => {
  if (!card.value?.name) return '?'
  return card.value.name
    .split(' ')
    .map(w => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
})

const { copy, copied } = useClipboard()

function copyUrl() {
  if (card.value?.url) {
    copy(card.value.url)
    toast.add({ title: 'Copied', description: 'Agent URL copied to clipboard' })
  }
}
</script>

<template>
  <UModal
    v-model:open="open"
    title="Agent Card"
    :ui="{
      content: 'sm:max-w-lg'
    }"
  >
    <!-- Hidden trigger (opened programmatically) -->
    <template #default />

    <template #content>
      <!-- Loading state -->
      <template v-if="fetching">
        <UCard :ui="{ body: 'flex flex-col items-center justify-center py-12 gap-4' }">
          <UIcon name="i-lucide-loader-circle" class="size-8 text-primary animate-spin" />
          <UBadge variant="subtle" color="neutral" size="lg">
            Loading agent card...
          </UBadge>
        </UCard>
      </template>

      <!-- Error state -->
      <template v-else-if="fetchError">
        <UCard>
          <template #header>
            <UBadge color="error" variant="subtle" icon="i-lucide-alert-circle" size="lg">
              Failed to load agent card
            </UBadge>
          </template>
          <UAlert
            color="error"
            variant="subtle"
            icon="i-lucide-wifi-off"
            title="Could not reach agent"
            :description="fetchError"
          />
          <template #footer>
            <UButton label="Retry" icon="i-lucide-refresh-cw" color="neutral" variant="outline" @click="fetchCard()" />
            <UButton label="Close" color="neutral" variant="ghost" @click="open = false" />
          </template>
        </UCard>
      </template>

      <!-- Card loaded -->
      <template v-else-if="card">
        <UCard
          :ui="{
            header: 'pb-0',
            body: 'space-y-5',
            footer: 'pt-0'
          }"
        >
          <!-- Header: Avatar + Name + Version + Provider + Close -->
          <template #header>
            <UButtonGroup class="w-full items-start justify-between">
              <UButton variant="ghost" class="flex items-center gap-3 pointer-events-none p-0 hover:bg-transparent">
                <UAvatar :text="initials" color="primary" size="lg" />
              </UButton>
              <UButton variant="ghost" class="flex flex-col items-start gap-0.5 pointer-events-none flex-1 p-0 hover:bg-transparent">
                <UButton variant="ghost" class="text-base font-semibold text-highlighted p-0 hover:bg-transparent pointer-events-none">
                  {{ card.name }}
                </UButton>
                <UButtonGroup class="gap-1.5 items-center">
                  <UBadge v-if="card.version" variant="subtle" color="primary" size="xs">
                    v{{ card.version }}
                  </UBadge>
                  <UButton v-if="card.provider" variant="ghost" size="xs" class="text-muted p-0 hover:bg-transparent pointer-events-none">
                    {{ card.provider.organization }}
                  </UButton>
                </UButtonGroup>
              </UButton>
              <UButton
                icon="i-lucide-x"
                color="neutral"
                variant="ghost"
                size="sm"
                @click="open = false"
              />
            </UButtonGroup>
          </template>

          <!-- Body -->
          <template #default>
            <!-- Description -->
            <UButton variant="ghost" class="text-sm text-muted leading-relaxed whitespace-normal text-left font-normal p-0 pointer-events-none hover:bg-transparent w-full justify-start">
              {{ card.description }}
            </UButton>

            <USeparator />

            <!-- Endpoint -->
            <USeparator label="Endpoint" icon="i-lucide-link" />
            <UButton
              :label="card.url"
              :icon="copied ? 'i-lucide-check' : 'i-lucide-copy'"
              :color="copied ? 'success' : 'neutral'"
              variant="subtle"
              class="w-full justify-between font-mono text-xs truncate"
              trailing
              @click="copyUrl"
            />

            <!-- Capabilities -->
            <USeparator label="Capabilities" icon="i-lucide-zap" />
            <UButtonGroup class="flex-wrap gap-1.5">
              <UBadge
                v-for="cap in capabilities"
                :key="cap.label"
                :color="cap.enabled ? 'success' : 'neutral'"
                :variant="cap.enabled ? 'subtle' : 'outline'"
                :icon="cap.enabled ? 'i-lucide-check' : 'i-lucide-x'"
              >
                {{ cap.label }}
              </UBadge>
            </UButtonGroup>

            <!-- Authentication -->
            <template v-if="card.authentication?.schemes?.length">
              <USeparator label="Authentication" icon="i-lucide-shield" />
              <UButtonGroup class="flex-wrap gap-1.5">
                <UBadge
                  v-for="scheme in card.authentication.schemes"
                  :key="scheme"
                  variant="outline"
                  color="neutral"
                  class="font-mono"
                >
                  {{ scheme }}
                </UBadge>
              </UButtonGroup>
            </template>

            <!-- I/O Modes -->
            <template v-if="card.defaultInputModes?.length || card.defaultOutputModes?.length">
              <USeparator />
              <UButtonGroup class="w-full gap-4">
                <UButton v-if="card.defaultInputModes?.length" variant="ghost" class="flex-1 flex-col items-start gap-1.5 p-0 pointer-events-none hover:bg-transparent">
                  <UBadge variant="outline" color="neutral" size="xs" class="text-muted uppercase tracking-wide font-semibold text-[10px]">
                    Input Modes
                  </UBadge>
                  <UButtonGroup class="flex-wrap gap-1">
                    <UBadge
                      v-for="mode in card.defaultInputModes"
                      :key="mode"
                      variant="subtle"
                      color="warning"
                      size="xs"
                      class="font-mono"
                    >
                      {{ mode.split('/')[1] || mode }}
                    </UBadge>
                  </UButtonGroup>
                </UButton>
                <UButton v-if="card.defaultOutputModes?.length" variant="ghost" class="flex-1 flex-col items-start gap-1.5 p-0 pointer-events-none hover:bg-transparent">
                  <UBadge variant="outline" color="neutral" size="xs" class="text-muted uppercase tracking-wide font-semibold text-[10px]">
                    Output Modes
                  </UBadge>
                  <UButtonGroup class="flex-wrap gap-1">
                    <UBadge
                      v-for="mode in card.defaultOutputModes"
                      :key="mode"
                      variant="subtle"
                      color="warning"
                      size="xs"
                      class="font-mono"
                    >
                      {{ mode.split('/')[1] || mode }}
                    </UBadge>
                  </UButtonGroup>
                </UButton>
              </UButtonGroup>
            </template>

            <!-- Skills -->
            <template v-if="skillItems.length">
              <USeparator :label="`Skills \u00B7 ${skillItems.length}`" icon="i-lucide-sparkles" />
              <UAccordion :items="skillItems" type="multiple">
                <template v-for="skill in card.skills" :key="skill.id" #[skill.id]>
                  <UCard variant="subtle" :ui="{ root: 'shadow-none', body: 'space-y-3 py-2 px-3' }">
                    <template #default>
                      <UButton v-if="skill.description" variant="ghost" class="text-xs text-muted whitespace-normal text-left font-normal p-0 pointer-events-none hover:bg-transparent w-full justify-start">
                        {{ skill.description }}
                      </UButton>

                      <!-- Tags -->
                      <UButtonGroup v-if="skill.tags?.length" class="flex-wrap gap-1">
                        <UBadge
                          v-for="tag in skill.tags"
                          :key="tag"
                          variant="subtle"
                          color="neutral"
                          size="xs"
                          class="font-mono"
                        >
                          {{ tag }}
                        </UBadge>
                      </UButtonGroup>

                      <!-- Example -->
                      <UAlert
                        v-if="skill.examples?.length"
                        variant="subtle"
                        color="primary"
                        icon="i-lucide-message-circle"
                        title="Example"
                        :description="`&quot;${skill.examples[0]}&quot;`"
                        :ui="{ root: 'py-2 px-3' }"
                      />
                    </template>
                  </UCard>
                </template>
              </UAccordion>
            </template>

            <!-- Documentation URL -->
            <template v-if="card.documentationUrl">
              <USeparator />
              <UButton
                :label="card.documentationUrl"
                icon="i-lucide-external-link"
                color="neutral"
                variant="link"
                :to="card.documentationUrl"
                target="_blank"
                class="text-xs font-mono"
              />
            </template>
          </template>
        </UCard>
      </template>
    </template>
  </UModal>
</template>
