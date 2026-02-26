<script setup lang="ts">
import type { Agent } from '~/types'

definePageMeta({ layout: 'default' })

const { current: namespace } = useNamespace()
const { listAgents } = useFlokoa()
const { data: agentList } = listAgents()
const { messages, status, error, send, clear, stop } = useAgChat()

const selectedAgent = ref<string | undefined>(undefined)
const input = ref('')

const agents = computed(() => agentList.value?.items ?? [])
const currentAgent = computed<Agent | undefined>(() =>
  agents.value.find(a => a.metadata.name === selectedAgent.value)
)

const agentItems = computed(() =>
  agents.value
    .filter(a => a.status?.phase === 'Running')
    .map(a => ({
      label: a.metadata.name,
      value: a.metadata.name,
      description: a.metadata.namespace
    }))
)

// Map composable status to UChatMessages expected status prop.
const chatStatus = computed(() => {
  if (status.value === 'streaming') return 'streaming'
  if (status.value === 'error') return 'error'
  return 'ready'
})

async function handleSend() {
  if (!input.value.trim() || !selectedAgent.value || !namespace.value || status.value === 'streaming') return
  const msg = input.value
  input.value = ''
  await send(namespace.value, selectedAgent.value, msg)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

// Clear conversation when agent changes.
watch(selectedAgent, () => {
  clear()
})
</script>

<template>
  <UDashboardPanel>
    <div class="flex flex-col h-[calc(100vh-theme(spacing.16))]">
      <!-- Top: Agent selector -->
      <div class="flex items-center gap-3 p-4 border-b border-default shrink-0">
        <USelectMenu
          v-model="selectedAgent"
          :items="agentItems"
          value-key="value"
          placeholder="Select an agent..."
          class="w-64"
        />

        <FrameworkBadge
          v-if="currentAgent?.spec?.framework"
          :framework="currentAgent.spec.framework"
        />

        <UBadge
          v-if="currentAgent?.status?.url"
          variant="subtle"
          color="success"
          size="sm"
        >
          <span class="truncate max-w-48">{{ currentAgent.status.url }}</span>
        </UBadge>

        <div class="flex-1" />

        <UButton
          v-if="status === 'streaming'"
          icon="i-lucide-square"
          variant="ghost"
          color="error"
          size="sm"
          @click="stop"
        >
          Stop
        </UButton>

        <UButton
          v-if="messages.length > 0"
          icon="i-lucide-trash-2"
          variant="ghost"
          color="neutral"
          size="sm"
          @click="clear"
        />
      </div>

      <!-- Middle: Chat messages -->
      <div class="flex-1 overflow-y-auto">
        <template v-if="!selectedAgent">
          <div class="flex flex-col items-center justify-center h-full text-center text-muted gap-4">
            <UIcon name="i-lucide-flask-conical" class="size-12 text-dimmed" />
            <div>
              <h3 class="text-lg font-medium text-highlighted">
                Agent Playground
              </h3>
              <p class="text-sm mt-1">
                Select a running agent to start testing
              </p>
            </div>
          </div>
        </template>

        <template v-else-if="messages.length === 0">
          <div class="flex flex-col items-center justify-center h-full text-center text-muted gap-4">
            <UIcon name="i-lucide-message-square" class="size-12 text-dimmed" />
            <div>
              <h3 class="text-lg font-medium text-highlighted">
                Chat with {{ currentAgent?.metadata.name }}
              </h3>
              <p class="text-sm mt-1">
                Send a message to start the conversation
              </p>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="max-w-3xl mx-auto p-4 space-y-4">
            <UChatMessage
              v-for="msg in messages"
              :key="msg.id"
              :id="msg.id"
              :role="msg.role"
              :parts="msg.parts"
              :variant="msg.role === 'user' ? 'soft' : 'naked'"
              :side="msg.role === 'user' ? 'right' : 'left'"
              :icon="msg.role === 'assistant' ? 'i-lucide-bot' : undefined"
            />

            <div
              v-if="status === 'streaming' && messages.length > 0 && messages[messages.length - 1]?.parts[0]?.text === ''"
              class="flex items-center gap-2 text-sm text-muted"
            >
              <UIcon name="i-lucide-loader" class="size-4 animate-spin" />
              <span>Agent is thinking...</span>
            </div>
          </div>
        </template>
      </div>

      <!-- Bottom: Input area -->
      <div v-if="selectedAgent" class="p-4 border-t border-default shrink-0">
        <form class="max-w-3xl mx-auto flex gap-2" @submit.prevent="handleSend">
          <UTextarea
            v-model="input"
            :disabled="status === 'streaming'"
            placeholder="Type a message..."
            autoresize
            :rows="1"
            :maxrows="6"
            class="flex-1"
            @keydown="handleKeydown"
          />
          <UButton
            type="submit"
            icon="i-lucide-send"
            :loading="status === 'streaming'"
            :disabled="!input.trim() || status === 'streaming'"
            class="self-end"
          />
        </form>
        <p v-if="error" class="max-w-3xl mx-auto text-sm text-error mt-2">
          {{ error }}
        </p>
      </div>
    </div>
  </UDashboardPanel>
</template>
