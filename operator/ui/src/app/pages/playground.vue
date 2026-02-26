<script setup lang="ts">
import type { Agent } from '~/types'
import { isAgentPhase } from '~/utils/enums'

definePageMeta({ layout: 'default' })

const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()
const { items: agents } = useListWatch<Agent>({
  listUrl: () => namespacedPath('agents'),
  watchUrl: () => buildWatchUrl('agents')
})
const { messages, status, error, send, clear, stop } = useAgChat()

const selectedAgent = ref<string | undefined>(undefined)
const input = ref('')

const currentAgent = computed<Agent | undefined>(() =>
  agents.value.find(a => a.metadata.name === selectedAgent.value)
)

const agentItems = computed(() =>
  agents.value
    .filter(a => isAgentPhase(a.status?.phase, 'Running'))
    .map(a => ({
      label: a.metadata.name,
      value: a.metadata.name,
      description: a.metadata.namespace
    }))
)

// Whether the last assistant message has any visible content yet.
const lastMessageHasContent = computed(() => {
  const lastMsg = messages.value[messages.value.length - 1]
  if (!lastMsg) return false
  return lastMsg.parts.some(p =>
    (p.type === 'text' && p.text) || p.type === 'data-artifact'
  )
})

async function handleSend() {
  const ns = currentAgent.value?.metadata.namespace
  if (!input.value.trim() || !selectedAgent.value || !ns || status.value === 'streaming') return
  const msg = input.value
  input.value = ''
  await send(ns, selectedAgent.value, msg)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function formatData(data: unknown): string {
  return typeof data === 'string' ? data : JSON.stringify(data, null, 2)
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
            >
              <template #content>
                <template v-for="(part, index) in msg.parts" :key="`${msg.id}-${part.type}-${index}`">
                  <p v-if="part.type === 'text' && part.text" class="whitespace-pre-wrap">
                    {{ part.text }}
                  </p>
                  <div
                    v-else-if="part.type === 'data-artifact'"
                    class="rounded-lg border border-default bg-elevated/50 p-3 my-2"
                  >
                    <div class="flex items-center gap-2 text-xs text-muted mb-2">
                      <UIcon name="i-lucide-box" class="size-3.5" />
                      <span class="font-medium">{{ part.name || 'Data' }}</span>
                    </div>
                    <pre class="text-xs overflow-x-auto whitespace-pre-wrap break-all">{{ formatData(part.data) }}</pre>
                  </div>
                </template>
              </template>
            </UChatMessage>

            <div
              v-if="status === 'streaming' && messages.length > 0 && !lastMessageHasContent"
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
