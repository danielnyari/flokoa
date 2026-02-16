<script setup lang="ts">
const auth = useAuth()

const sessionItems = computed(() => {
  if (!auth.isAuthEnabled.value || !auth.user.value) return []

  const items: { label: string, value: string }[] = [
    { label: 'Email', value: auth.user.value.email || 'N/A' },
    { label: 'Name', value: auth.user.value.name || 'N/A' }
  ]

  if (auth.user.value.groups?.length) {
    items.push({ label: 'Groups', value: auth.user.value.groups.join(', ') })
  }

  return items
})
</script>

<template>
  <UPageCard
    title="Authentication"
    description="Authentication is managed through your identity provider via OIDC (Dex)."
    variant="subtle"
  >
    <div v-if="auth.isAuthEnabled.value && auth.user.value" class="flex flex-col gap-3 max-w-sm">
      <div
        v-for="item in sessionItems"
        :key="item.label"
        class="flex justify-between text-sm"
      >
        <span class="text-muted">{{ item.label }}</span>
        <span class="text-highlighted">{{ item.value }}</span>
      </div>
    </div>
    <UAlert
      v-else-if="!auth.isAuthEnabled.value"
      color="neutral"
      variant="subtle"
      icon="i-lucide-info"
      title="Authentication disabled"
      description="Enable Dex in the Helm chart configuration to enforce SSO."
    />
  </UPageCard>

  <UPageCard
    v-if="auth.isAuthEnabled.value"
    title="Session"
    description="Sign out of your current session. You will need to sign in again through your identity provider."
  >
    <template #footer>
      <UButton
        label="Sign out"
        icon="i-lucide-log-out"
        color="error"
        @click="auth.logout()"
      />
    </template>
  </UPageCard>
</template>
