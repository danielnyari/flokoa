export default defineNuxtRouteMiddleware((to) => {
  // Public routes that don't require auth
  const publicRoutes = ['/login', '/auth/callback']
  if (publicRoutes.some(route => to.path.startsWith(route))) {
    return
  }

  const auth = useAuth()

  // While auth is initializing, redirect to login to prevent flash of protected content.
  // The login page will redirect back after init completes if auth is disabled.
  if (auth.loading.value) {
    return navigateTo('/login')
  }

  // If auth is disabled, allow all routes
  if (!auth.isAuthEnabled.value) {
    return
  }

  // If not authenticated, save the target path and redirect to login
  if (!auth.isAuthenticated.value) {
    sessionStorage.setItem('auth_redirect', to.fullPath)
    return navigateTo('/login')
  }
})
