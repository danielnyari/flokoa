<script setup lang="ts">
definePageMeta({
  layout: false
})

const auth = useAuth()

async function handleLogin() {
  await auth.login()
}
</script>

<template>
  <div class="flex items-center justify-center min-h-screen bg-default">
    <div class="w-full max-w-sm mx-auto p-8">
      <div class="flex flex-col items-center gap-6">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-hexagon" class="size-8 text-primary" />
          <span class="text-2xl font-semibold text-highlighted">Flokoa</span>
        </div>

        <p class="text-sm text-muted text-center">
          Sign in to access the AI Agent Management Platform.
        </p>

        <UButton
          label="Sign in with SSO"
          icon="i-lucide-log-in"
          size="lg"
          block
          @click="handleLogin"
        />

        <p v-if="auth.error.value" class="text-sm text-error text-center">
          {{ auth.error.value }}
        </p>
      </div>
    </div>
  </div>
</template>
