import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import LoginPage from '~/pages/login.vue'

// Use a shared reactive error ref so we can mutate it between tests
const authError = ref<string | null>(null)

const { useAuthMock } = vi.hoisted(() => {
  return {
    useAuthMock: vi.fn()
  }
})

// Configure the mock to always use our shared authError ref
useAuthMock.mockImplementation(() => ({
  login: vi.fn(),
  error: authError,
  loading: ref(false),
  isAuthEnabled: computed(() => true),
  isAuthenticated: computed(() => false),
  config: ref({ enabled: true, issuerUrl: 'https://dex.example.com', clientId: 'flokoa' }),
  user: ref(null),
  getAccessToken: vi.fn(() => null),
  handleCallback: vi.fn(),
  logout: vi.fn(),
  init: vi.fn()
}))

mockNuxtImport('useAuth', () => useAuthMock)

beforeEach(() => {
  authError.value = null
})

describe('login page', () => {
  it('should render the login page with title and description', async () => {
    const component = await mountSuspended(LoginPage)

    expect(component.text()).toContain('Flokoa')
    expect(component.text()).toContain('Sign in to access the AI Agent Management Platform')
  })

  it('should render the SSO sign-in button', async () => {
    const component = await mountSuspended(LoginPage)

    expect(component.text()).toContain('Sign in with SSO')
  })

  it('should not display error alert when there is no error', async () => {
    const component = await mountSuspended(LoginPage)

    // The error text should not appear anywhere
    expect(component.text()).not.toContain('discovery failed')
  })

  it('should display error alert when auth has an error', async () => {
    // Set the error before mounting
    authError.value = 'OIDC discovery failed'

    const component = await mountSuspended(LoginPage)

    // Wait for Vue reactivity to flush
    await nextTick()

    expect(component.html()).toContain('OIDC discovery failed')
  })
})
