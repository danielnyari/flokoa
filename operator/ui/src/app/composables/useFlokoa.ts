import type { AgentList, ModelList, ModelProviderList, AgentToolList } from '~/types'

const API_BASE = '/api/v1alpha1'

export function useFlokoa() {
  const auth = useAuth()

  // Build fetch options with auth header when available.
  // Returns a getter so that useFetch re-evaluates the token on each request,
  // ensuring refreshed tokens are picked up automatically.
  function authHeaders(): Record<string, string> {
    const token = auth.getAccessToken()
    if (token) {
      return { Authorization: `Bearer ${token}` }
    }
    return {}
  }

  // Shared error handler for 401 responses
  function onResponseError({ response }: { response: { status: number } }) {
    if (response.status === 401) {
      auth.logout()
      navigateTo('/login')
    }
  }

  function listAgents(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agents`
      : `${API_BASE}/agents`
    return useFetch<AgentList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listModels(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/models`
      : `${API_BASE}/models`
    return useFetch<ModelList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listModelProviders(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/modelproviders`
      : `${API_BASE}/modelproviders`
    return useFetch<ModelProviderList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listAgentTools(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agenttools`
      : `${API_BASE}/agenttools`
    return useFetch<AgentToolList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  return {
    listAgents,
    listModels,
    listModelProviders,
    listAgentTools
  }
}
