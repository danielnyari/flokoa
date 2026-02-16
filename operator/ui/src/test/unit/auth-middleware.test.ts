import { describe, expect, it } from 'vitest'

/**
 * Unit tests for auth middleware routing logic.
 * Tests the pure decision logic of route protection separately from
 * the Nuxt middleware runtime.
 */

interface AuthState {
  loading: boolean
  isAuthEnabled: boolean
  isAuthenticated: boolean
}

type MiddlewareResult = 'allow' | 'redirect-login'

const publicRoutes = ['/login', '/auth/callback']

/**
 * Pure function that mirrors the middleware decision logic.
 * Returns what action the middleware should take.
 */
function resolveAuthMiddleware(path: string, auth: AuthState): MiddlewareResult {
  // Public routes bypass auth
  if (publicRoutes.some(route => path.startsWith(route))) {
    return 'allow'
  }

  // While loading, redirect to login to prevent flash of protected content
  if (auth.loading) {
    return 'redirect-login'
  }

  // Auth disabled → allow everything
  if (!auth.isAuthEnabled) {
    return 'allow'
  }

  // Not authenticated → redirect to login
  if (!auth.isAuthenticated) {
    return 'redirect-login'
  }

  return 'allow'
}

describe('auth middleware routing logic', () => {
  describe('public routes', () => {
    it('should allow /login regardless of auth state', () => {
      expect(resolveAuthMiddleware('/login', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('allow')
    })

    it('should allow /auth/callback regardless of auth state', () => {
      expect(resolveAuthMiddleware('/auth/callback', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('allow')
    })

    it('should allow /auth/callback with query params', () => {
      expect(resolveAuthMiddleware('/auth/callback?code=abc&state=xyz', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('allow')
    })
  })

  describe('loading state', () => {
    it('should redirect to login while auth is loading', () => {
      expect(resolveAuthMiddleware('/', {
        loading: true,
        isAuthEnabled: false,
        isAuthenticated: false
      })).toBe('redirect-login')
    })

    it('should redirect protected routes to login while loading', () => {
      expect(resolveAuthMiddleware('/settings/security', {
        loading: true,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('redirect-login')
    })
  })

  describe('auth disabled', () => {
    it('should allow all routes when auth is disabled', () => {
      expect(resolveAuthMiddleware('/', {
        loading: false,
        isAuthEnabled: false,
        isAuthenticated: false
      })).toBe('allow')
    })

    it('should allow settings when auth is disabled', () => {
      expect(resolveAuthMiddleware('/settings/security', {
        loading: false,
        isAuthEnabled: false,
        isAuthenticated: false
      })).toBe('allow')
    })
  })

  describe('auth enabled, authenticated', () => {
    it('should allow authenticated users to access protected routes', () => {
      expect(resolveAuthMiddleware('/', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: true
      })).toBe('allow')
    })

    it('should allow authenticated users to access settings', () => {
      expect(resolveAuthMiddleware('/settings/security', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: true
      })).toBe('allow')
    })
  })

  describe('auth enabled, unauthenticated', () => {
    it('should redirect unauthenticated users from root to login', () => {
      expect(resolveAuthMiddleware('/', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('redirect-login')
    })

    it('should redirect unauthenticated users from settings to login', () => {
      expect(resolveAuthMiddleware('/settings/security', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('redirect-login')
    })

    it('should redirect unauthenticated users from deep paths to login', () => {
      expect(resolveAuthMiddleware('/agents/my-agent/details', {
        loading: false,
        isAuthEnabled: true,
        isAuthenticated: false
      })).toBe('redirect-login')
    })
  })
})
