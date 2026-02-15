<script setup lang="ts">
const auth = useAuth()
</script>

<template>
  <UPageCard
    title="Authentication"
    description="Authentication is managed through your identity provider via OIDC (Dex)."
    variant="subtle"
  >
    <div class="flex flex-col gap-3 max-w-sm">
      <div v-if="auth.isAuthEnabled.value && auth.user.value" class="flex flex-col gap-2 text-sm">
        <div class="flex justify-between">
          <span class="text-muted">Email</span>
          <span class="text-highlighted">{{ auth.user.value.email || 'N/A' }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-muted">Name</span>
          <span class="text-highlighted">{{ auth.user.value.name || 'N/A' }}</span>
        </div>
        <div v-if="auth.user.value.groups?.length" class="flex justify-between">
          <span class="text-muted">Groups</span>
          <span class="text-highlighted">{{ auth.user.value.groups.join(', ') }}</span>
        </div>
      </div>
      <p v-else-if="!auth.isAuthEnabled.value" class="text-sm text-muted">
        Authentication is currently disabled. Enable Dex in the Helm chart configuration to enforce SSO.
      </p>
    </div>
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
