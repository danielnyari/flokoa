<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

const route = useRoute()
const { agentCount, modelCount, providerCount, toolCount, workflowCount } = useResourceCounts()

const open = ref(false)

// Keep navigation items as stable objects so UNavigationMenu doesn't
// re-mount NuxtLinks when badge counts change (which swallows clicks).
const agentLink = reactive<NavigationMenuItem>({ label: 'Agents', icon: 'i-lucide-bot', to: '/agents', onSelect: () => { open.value = false } })
const modelLink = reactive<NavigationMenuItem>({ label: 'Models', icon: 'i-lucide-brain', to: '/models', onSelect: () => { open.value = false } })
const providerLink = reactive<NavigationMenuItem>({ label: 'Providers', icon: 'i-lucide-cloud', to: '/providers', onSelect: () => { open.value = false } })
const toolLink = reactive<NavigationMenuItem>({ label: 'Tools', icon: 'i-lucide-wrench', to: '/tools', onSelect: () => { open.value = false } })
const workflowLink = reactive<NavigationMenuItem>({ label: 'Workflows', icon: 'i-lucide-git-branch', to: '/workflows', onSelect: () => { open.value = false } })
const playgroundLink = reactive<NavigationMenuItem>({ label: 'Playground', icon: 'i-lucide-flask-conical', to: '/playground', onSelect: () => { open.value = false } })

watchEffect(() => {
  agentLink.badge = agentCount.value > 0 ? String(agentCount.value) : undefined
  modelLink.badge = modelCount.value > 0 ? String(modelCount.value) : undefined
  providerLink.badge = providerCount.value > 0 ? String(providerCount.value) : undefined
  toolLink.badge = toolCount.value > 0 ? String(toolCount.value) : undefined
  workflowLink.badge = workflowCount.value > 0 ? String(workflowCount.value) : undefined
})

const links = [[{
  label: 'Home',
  icon: 'i-lucide-house',
  to: '/',
  onSelect: () => {
    open.value = false
  }
},
  agentLink,
  modelLink,
  providerLink,
  toolLink,
  workflowLink,
  playgroundLink,
{
  label: 'Settings',
  to: '/settings',
  icon: 'i-lucide-settings',
  defaultOpen: true,
  type: 'trigger',
  children: [{
    label: 'General',
    to: '/settings',
    exact: true,
    onSelect: () => {
      open.value = false
    }
  }, {
    label: 'Members',
    to: '/settings/members',
    onSelect: () => {
      open.value = false
    }
  }, {
    label: 'Notifications',
    to: '/settings/notifications',
    onSelect: () => {
      open.value = false
    }
  }, {
    label: 'Security',
    to: '/settings/security',
    onSelect: () => {
      open.value = false
    }
  }]
}], [{
  label: 'Documentation',
  icon: 'i-lucide-book-open',
  to: 'https://flokoa.ai',
  target: '_blank'
}, {
  label: 'GitHub',
  icon: 'i-simple-icons-github',
  to: 'https://github.com/danielnyari/flokoa',
  target: '_blank'
}]] as NavigationMenuItem[][]

const groups = computed(() => [{
  id: 'links',
  label: 'Go to',
  items: links.flat()
}, {
  id: 'code',
  label: 'Code',
  items: [{
    id: 'source',
    label: 'View page source',
    icon: 'i-simple-icons-github',
    to: `https://github.com/danielnyari/flokoa/blob/main/ui/app/pages${route.path === '/' ? '/index' : route.path}.vue`,
    target: '_blank'
  }]
}])
</script>

<template>
  <UDashboardGroup unit="rem">
    <UDashboardSidebar
      id="default"
      v-model:open="open"
      collapsible
      resizable
      class="bg-elevated/25"
      :ui="{ footer: 'lg:border-t lg:border-default' }"
    >
      <template #header="{ collapsed }">
        <div :class="['flex items-center gap-2 px-2 py-1', collapsed && 'justify-center']">
          <UIcon name="i-lucide-hexagon" class="size-6 text-primary shrink-0" />
          <span v-if="!collapsed" class="font-semibold text-highlighted">Flokoa</span>
        </div>
      </template>

      <template #default="{ collapsed }">
        <UDashboardSearchButton :collapsed="collapsed" class="bg-transparent ring-default" />

        <NamespaceSelect :collapsed="collapsed" />

        <UNavigationMenu
          :collapsed="collapsed"
          :items="links[0]"
          orientation="vertical"
          tooltip
          popover
        />

        <UNavigationMenu
          :collapsed="collapsed"
          :items="links[1]"
          orientation="vertical"
          tooltip
          class="mt-auto"
        />
      </template>

      <template #footer="{ collapsed }">
        <UserMenu :collapsed="collapsed" />
      </template>
    </UDashboardSidebar>

    <UDashboardSearch :groups="groups" />

    <ShortcutHelp />

    <slot />
  </UDashboardGroup>
</template>
