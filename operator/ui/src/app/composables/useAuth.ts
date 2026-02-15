import { createSharedComposable } from '@vueuse/core'

export interface AuthConfig {
  enabled: boolean
  issuerUrl: string
  clientId: string
}

export interface AuthUser {
  sub: string
  email: string
  name: string
  groups: string[]
}

interface OIDCDiscovery {
  authorization_endpoint: string
  token_endpoint: string
  userinfo_endpoint: string
  end_session_endpoint?: string
}

interface TokenResponse {
  access_token: string
  id_token: string
  refresh_token?: string
  expires_in: number
  token_type: string
}

// Generate a random string for PKCE and state parameters
function generateRandomString(length: number): string {
  const array = new Uint8Array(length)
  crypto.getRandomValues(array)
  return Array.from(array, b => b.toString(36)).join('').slice(0, length)
}

// Generate PKCE code verifier and challenge
async function generatePKCE() {
  const verifier = generateRandomString(64)
  const encoder = new TextEncoder()
  const data = encoder.encode(verifier)
  const digest = await crypto.subtle.digest('SHA-256', data)
  const challenge = btoa(String.fromCharCode(...new Uint8Array(digest)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
  return { verifier, challenge }
}

// Parse JWT payload without validation (validation happens server-side)
function parseJwtPayload(token: string): Record<string, unknown> {
  const payload = token.split('.')[1]
  if (!payload) return {}
  const decoded = atob(payload.replace(/-/g, '+').replace(/_/g, '/'))
  return JSON.parse(decoded)
}

const _useAuth = () => {
  const config = ref<AuthConfig | null>(null)
  const discovery = ref<OIDCDiscovery | null>(null)
  const user = ref<AuthUser | null>(null)
  const loading = ref(true)
  const error = ref<string | null>(null)

  // Tokens stored in memory only — never persisted to localStorage/sessionStorage
  let accessToken: string | null = null
  let refreshToken: string | null = null
  let tokenExpiresAt = 0
  let refreshTimer: ReturnType<typeof setTimeout> | null = null

  const isAuthenticated = computed(() => !!user.value)
  const isAuthEnabled = computed(() => config.value?.enabled ?? false)

  // Fetch auth configuration from the backend
  async function fetchConfig(): Promise<AuthConfig> {
    const response = await fetch('/api/v1alpha1/auth/config')
    if (!response.ok) {
      throw new Error(`Failed to fetch auth config: ${response.statusText}`)
    }
    return response.json()
  }

  // Fetch OIDC discovery document from Dex
  async function fetchDiscovery(issuerUrl: string): Promise<OIDCDiscovery> {
    const response = await fetch(`${issuerUrl}/.well-known/openid-configuration`)
    if (!response.ok) {
      throw new Error(`Failed to fetch OIDC discovery: ${response.statusText}`)
    }
    return response.json()
  }

  // Exchange authorization code for tokens
  async function exchangeCode(code: string, redirectUri: string): Promise<TokenResponse> {
    const codeVerifier = sessionStorage.getItem('pkce_verifier')
    if (!codeVerifier) {
      throw new Error('Missing PKCE code verifier')
    }
    sessionStorage.removeItem('pkce_verifier')

    const params = new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: redirectUri,
      client_id: config.value!.clientId,
      code_verifier: codeVerifier
    })

    const response = await fetch(discovery.value!.token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: params.toString()
    })

    if (!response.ok) {
      const body = await response.text()
      throw new Error(`Token exchange failed: ${body}`)
    }

    return response.json()
  }

  // Set tokens from a token response
  function setTokens(tokens: TokenResponse) {
    accessToken = tokens.access_token
    refreshToken = tokens.refresh_token ?? null
    tokenExpiresAt = Date.now() + tokens.expires_in * 1000

    // Parse user info from the ID token
    const claims = parseJwtPayload(tokens.id_token)
    user.value = {
      sub: (claims.sub as string) || '',
      email: (claims.email as string) || '',
      name: (claims.name as string) || (claims.email as string) || '',
      groups: (claims.groups as string[]) || []
    }

    // Schedule token refresh at 80% of the lifetime
    scheduleRefresh(tokens.expires_in)
  }

  // Proactively refresh the access token before it expires
  function scheduleRefresh(expiresInSeconds: number) {
    if (refreshTimer) {
      clearTimeout(refreshTimer)
    }
    const refreshAt = expiresInSeconds * 0.8 * 1000
    refreshTimer = setTimeout(() => {
      refreshAccessToken()
    }, refreshAt)
  }

  // Refresh access token using the refresh token
  async function refreshAccessToken() {
    if (!refreshToken || !discovery.value || !config.value) {
      logout()
      return
    }

    try {
      const params = new URLSearchParams({
        grant_type: 'refresh_token',
        refresh_token: refreshToken,
        client_id: config.value.clientId
      })

      const response = await fetch(discovery.value.token_endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: params.toString()
      })

      if (!response.ok) {
        logout()
        return
      }

      const tokens: TokenResponse = await response.json()
      setTokens(tokens)
    } catch {
      logout()
    }
  }

  // Get the current access token, refreshing if needed
  function getAccessToken(): string | null {
    if (!accessToken) return null
    if (Date.now() >= tokenExpiresAt) {
      // Token expired — trigger refresh but return null for now
      refreshAccessToken()
      return null
    }
    return accessToken
  }

  // Initiate the OIDC login flow by redirecting to Dex
  async function login() {
    if (!config.value || !discovery.value) return

    const { verifier, challenge } = await generatePKCE()

    // Store PKCE verifier in sessionStorage (needed for the callback)
    sessionStorage.setItem('pkce_verifier', verifier)

    const state = generateRandomString(32)
    sessionStorage.setItem('oauth_state', state)

    const redirectUri = `${window.location.origin}/auth/callback`

    const params = new URLSearchParams({
      response_type: 'code',
      client_id: config.value.clientId,
      redirect_uri: redirectUri,
      scope: 'openid profile email groups offline_access',
      state,
      code_challenge: challenge,
      code_challenge_method: 'S256'
    })

    window.location.href = `${discovery.value.authorization_endpoint}?${params.toString()}`
  }

  // Handle the OAuth callback (called from the callback page)
  async function handleCallback(): Promise<boolean> {
    const url = new URL(window.location.href)
    const code = url.searchParams.get('code')
    const state = url.searchParams.get('state')
    const savedState = sessionStorage.getItem('oauth_state')

    sessionStorage.removeItem('oauth_state')

    if (!code || !state || state !== savedState) {
      error.value = 'Invalid OAuth callback parameters'
      return false
    }

    try {
      const redirectUri = `${window.location.origin}/auth/callback`
      const tokens = await exchangeCode(code, redirectUri)
      setTokens(tokens)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Token exchange failed'
      return false
    }
  }

  // Clear auth state and redirect to login
  function logout() {
    accessToken = null
    refreshToken = null
    tokenExpiresAt = 0
    user.value = null

    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }

    // If there's an end session endpoint, redirect there
    if (discovery.value?.end_session_endpoint) {
      const params = new URLSearchParams({
        post_logout_redirect_uri: window.location.origin
      })
      window.location.href = `${discovery.value.end_session_endpoint}?${params.toString()}`
    }
  }

  // Initialize auth — called once on app startup
  async function init() {
    try {
      loading.value = true
      error.value = null

      config.value = await fetchConfig()

      if (!config.value.enabled) {
        // Auth disabled — skip auth setup, allow full access
        loading.value = false
        return
      }

      discovery.value = await fetchDiscovery(config.value.issuerUrl)
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Auth initialization failed'
    } finally {
      loading.value = false
    }
  }

  return {
    config,
    user,
    loading,
    error,
    isAuthenticated,
    isAuthEnabled,
    getAccessToken,
    login,
    handleCallback,
    logout,
    init
  }
}

export const useAuth = createSharedComposable(_useAuth)
