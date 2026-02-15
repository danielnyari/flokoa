import { describe, expect, it, vi } from 'vitest'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import SecuritySettings from '~/pages/settings/security.vue'

const logoutMock = vi.fn()

// Mock useAuth with auth enabled and a user
const { useAuthMock } = vi.hoisted(() => {
  return {
    useAuthMock: vi.fn(() => ({
      isAuthEnabled: computed(() => true),
      isAuthenticated: computed(() => true),
      user: ref({
        sub: 'user-123',
        email: 'admin@flokoa.ai',
        name: 'Admin User',
        groups: ['platform-admins', 'developers']
      }),
      error: ref(null),
      loading: ref(false),
      config: ref({ enabled: true, issuerUrl: 'https://dex.example.com', clientId: 'flokoa' }),
      getAccessToken: vi.fn(() => 'mock-token'),
      login: vi.fn(),
      handleCallback: vi.fn(),
      logout: logoutMock,
      init: vi.fn()
    }))
  }
})

mockNuxtImport('useAuth', () => useAuthMock)

describe('security settings page', () => {
  it('should display user session info when auth is enabled', async () => {
    const component = await mountSuspended(SecuritySettings)

    expect(component.text()).toContain('Authentication')
    expect(component.text()).toContain('admin@flokoa.ai')
    expect(component.text()).toContain('Admin User')
    expect(component.text()).toContain('platform-admins, developers')
  })

  it('should show sign out button when auth is enabled', async () => {
    const component = await mountSuspended(SecuritySettings)

    expect(component.text()).toContain('Sign out')
    expect(component.text()).toContain('Session')
  })

  it('should show disabled auth message when auth is not enabled', async () => {
    useAuthMock.mockReturnValueOnce({
      isAuthEnabled: computed(() => false),
      isAuthenticated: computed(() => false),
      user: ref(null),
      error: ref(null),
      loading: ref(false),
      config: ref({ enabled: false, issuerUrl: '', clientId: '' }),
      getAccessToken: vi.fn(() => null),
      login: vi.fn(),
      handleCallback: vi.fn(),
      logout: vi.fn(),
      init: vi.fn()
    })

    const component = await mountSuspended(SecuritySettings)

    expect(component.text()).toContain('Authentication disabled')
    expect(component.text()).toContain('Enable Dex in the Helm chart')
  })

  it('should not show session card when auth is disabled', async () => {
    useAuthMock.mockReturnValueOnce({
      isAuthEnabled: computed(() => false),
      isAuthenticated: computed(() => false),
      user: ref(null),
      error: ref(null),
      loading: ref(false),
      config: ref({ enabled: false, issuerUrl: '', clientId: '' }),
      getAccessToken: vi.fn(() => null),
      login: vi.fn(),
      handleCallback: vi.fn(),
      logout: vi.fn(),
      init: vi.fn()
    })

    const component = await mountSuspended(SecuritySettings)

    expect(component.text()).not.toContain('Sign out')
    expect(component.text()).not.toContain('Session')
  })
})
