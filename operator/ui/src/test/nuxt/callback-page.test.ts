import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mountSuspended, mockNuxtImport } from '@nuxt/test-utils/runtime'
import CallbackPage from '~/pages/auth/callback.vue'

const handleCallbackMock = vi.fn()

// Mock useAuth composable
const { useAuthMock } = vi.hoisted(() => {
  return {
    useAuthMock: vi.fn(() => ({
      handleCallback: handleCallbackMock,
      error: ref<string | null>(null),
      loading: ref(false),
      isAuthEnabled: computed(() => true),
      isAuthenticated: computed(() => false),
      config: ref({ enabled: true, issuerUrl: 'https://dex.example.com', clientId: 'flokoa' }),
      user: ref(null),
      getAccessToken: vi.fn(() => null),
      login: vi.fn(),
      logout: vi.fn(),
      init: vi.fn()
    }))
  }
})

mockNuxtImport('useAuth', () => useAuthMock)

beforeEach(() => {
  vi.clearAllMocks()
  // handleCallback returns a pending promise by default so onMounted doesn't complete
  handleCallbackMock.mockReturnValue(new Promise(() => {}))
})

describe('auth callback page', () => {
  it('should show loading spinner initially', async () => {
    const component = await mountSuspended(CallbackPage)

    expect(component.text()).toContain('Completing sign in')
  })

  it('should render the page card wrapper', async () => {
    const component = await mountSuspended(CallbackPage)

    // Verify the page structure renders
    expect(component.html()).toContain('Completing sign in')
  })
})
