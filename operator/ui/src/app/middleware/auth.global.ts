export default defineNuxtRouteMiddleware((to) => {
  // Public routes that don't require auth
  const publicRoutes = ['/login', '/auth/callback']
  if (publicRoutes.some(route => to.path.startsWith(route))) {
    return
  }

  const auth = useAuth()

  // Wait for auth to initialize
  if (auth.loading.value) {
    return
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
