<script setup lang="ts">
definePageMeta({
  layout: false
})

const auth = useAuth()

const providers = [{
  label: 'Sign in with SSO',
  icon: 'i-lucide-log-in',
  onClick: () => auth.login()
}]
</script>

<template>
  <div class="flex items-center justify-center min-h-screen">
    <UPageCard class="w-full max-w-md">
      <UAuthForm
        title="Flokoa"
        description="Sign in to access the AI Agent Management Platform."
        icon="i-lucide-hexagon"
        :providers="providers"
        :fields="[]"
        :submit="{ class: 'hidden' }"
      >
        <template #footer>
          <UAlert
            v-if="auth.error.value"
            color="error"
            icon="i-lucide-alert-circle"
            :title="auth.error.value"
          />
        </template>
      </UAuthForm>
    </UPageCard>
  </div>
</template>
