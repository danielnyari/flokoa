<script setup lang="ts">
const open = ref(false)

const shortcuts = [
  { keys: ['g', 'h'], description: 'Go to Home' },
  { keys: ['g', 'a'], description: 'Go to Agents' },
  { keys: ['g', 'm'], description: 'Go to Models' },
  { keys: ['g', 'p'], description: 'Go to Providers' },
  { keys: ['g', 't'], description: 'Go to Tools' },
  { keys: ['g', 's'], description: 'Go to Settings' },
  { keys: ['?'], description: 'Show keyboard shortcuts' }
]

defineShortcuts({
  '?': () => {
    open.value = !open.value
  }
})
</script>

<template>
  <UModal v-model:open="open" title="Keyboard Shortcuts" :ui="{ content: 'max-w-sm' }">
    <template #body>
      <div class="space-y-2">
        <div
          v-for="shortcut in shortcuts"
          :key="shortcut.description"
          class="flex items-center justify-between py-1.5"
        >
          <span class="text-sm text-muted">{{ shortcut.description }}</span>
          <div class="flex items-center gap-1">
            <kbd
              v-for="key in shortcut.keys"
              :key="key"
              class="inline-flex items-center justify-center min-w-6 h-6 px-1.5 rounded border border-default bg-elevated text-xs font-mono font-medium text-highlighted"
            >
              {{ key }}
            </kbd>
          </div>
        </div>
      </div>
    </template>
  </UModal>
</template>
