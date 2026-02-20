<script setup lang="ts">
import { ALL_NAMESPACES } from '~/composables/useNamespace'

defineProps<{
  collapsed?: boolean
}>()

const { selected, current } = useNamespace()

// Fetch all agents once to extract namespaces (using explicit empty namespace = all)
const { data: agentList } = await useFetch<{ items: Array<{ metadata: { namespace: string } }> }>('/api/v1alpha1/agents', {
  lazy: true
})

const namespaceItems = computed(() => {
  const items = agentList.value?.items ?? []
  const ns = new Set<string>()
  items.forEach(i => ns.add(i.metadata.namespace))
  return [
    { label: 'All namespaces', value: ALL_NAMESPACES },
    ...[...ns].sort().map(n => ({ label: n, value: n }))
  ]
})

const displayLabel = computed(() => current.value || 'All namespaces')
</script>

<template>
  <div v-if="!collapsed" class="px-2 mb-2">
    <USelect
      v-model="selected"
      :items="namespaceItems"
      :ui="{ trailingIcon: 'group-data-[state=open]:rotate-180 transition-transform duration-200' }"
      class="w-full"
      size="sm"
      icon="i-lucide-layers"
    />
  </div>
  <div v-else class="flex justify-center mb-2">
    <UTooltip :text="displayLabel">
      <UPopover>
        <UButton
          icon="i-lucide-layers"
          color="neutral"
          variant="ghost"
          square
          size="sm"
        />
        <template #panel>
          <div class="p-2 w-48">
            <USelect
              v-model="selected"
              :items="namespaceItems"
              size="sm"
              icon="i-lucide-layers"
              class="w-full"
            />
          </div>
        </template>
      </UPopover>
    </UTooltip>
  </div>
</template>
