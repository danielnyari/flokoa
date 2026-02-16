<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{
  collapsed?: boolean
}>()

const colorMode = useColorMode()
const auth = useAuth()

const displayUser = computed(() => {
  if (auth.isAuthEnabled.value && auth.user.value) {
    return {
      name: auth.user.value.name || auth.user.value.email || 'User',
      avatar: {
        alt: auth.user.value.name || auth.user.value.email || 'User'
      }
    }
  }
  return {
    name: 'Admin',
    avatar: {
      alt: 'Admin'
    }
  }
})

const items = computed<DropdownMenuItem[][]>(() => {
  const menu: DropdownMenuItem[][] = [[{
    type: 'label',
    label: displayUser.value.name,
    avatar: displayUser.value.avatar
  }], [{
    label: 'Settings',
    icon: 'i-lucide-settings',
    to: '/settings'
  }], [{
    label: 'Appearance',
    icon: 'i-lucide-sun-moon',
    children: [{
      label: 'Light',
      icon: 'i-lucide-sun',
      type: 'checkbox',
      checked: colorMode.value === 'light',
      onSelect(e: Event) {
        e.preventDefault()
        colorMode.preference = 'light'
      }
    }, {
      label: 'Dark',
      icon: 'i-lucide-moon',
      type: 'checkbox',
      checked: colorMode.value === 'dark',
      onUpdateChecked(checked: boolean) {
        if (checked) {
          colorMode.preference = 'dark'
        }
      },
      onSelect(e: Event) {
        e.preventDefault()
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
  }]]

  if (auth.isAuthEnabled.value) {
    menu.push([{
      label: 'Sign out',
      icon: 'i-lucide-log-out',
      onSelect() {
        auth.logout()
      }
    }])
  }

  return menu
})
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'center', collisionPadding: 12 }"
    :ui="{ content: collapsed ? 'w-48' : 'w-(--reka-dropdown-menu-trigger-width)' }"
  >
    <UButton
      v-bind="{
        ...displayUser,
        label: collapsed ? undefined : displayUser?.name,
        trailingIcon: collapsed ? undefined : 'i-lucide-chevrons-up-down'
      }"
      color="neutral"
      variant="ghost"
      block
      :square="collapsed"
      class="data-[state=open]:bg-elevated"
      :ui="{
        trailingIcon: 'text-dimmed'
      }"
    />
  </UDropdownMenu>
</template>
