<script setup lang="ts">
definePageMeta({
  layout: false
})

const router = useRouter()
const auth = useAuth()

const error = ref<string | null>(null)

onMounted(async () => {
  const success = await auth.handleCallback()
  if (success) {
    // Redirect to the page the user was trying to access, or home
    const redirect = sessionStorage.getItem('auth_redirect') || '/'
    sessionStorage.removeItem('auth_redirect')
    router.replace(redirect)
  } else {
    error.value = auth.error.value || 'Authentication failed'
  }
})
</script>

<template>
  <div class="flex items-center justify-center min-h-screen">
    <UPageCard class="w-full max-w-sm text-center">
      <template v-if="error">
        <div class="flex flex-col items-center gap-4 p-6">
          <UAlert
            color="error"
            icon="i-lucide-alert-circle"
            title="Authentication failed"
            :description="error"
          />
          <UButton label="Try again" to="/login" />
        </div>
      </template>
      <template v-else>
        <div class="flex flex-col items-center gap-4 p-6">
          <UIcon name="i-lucide-loader-2" class="size-8 text-primary animate-spin" />
          <p class="text-sm text-muted">
            Completing sign in...
          </p>
        </div>
      </template>
    </UPageCard>
  </div>
</template>
