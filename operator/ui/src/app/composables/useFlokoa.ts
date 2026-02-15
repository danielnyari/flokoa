import type { AgentList, ModelList, ModelProviderList, AgentToolList } from '~/types'

const API_BASE = '/api/v1alpha1'

export function useFlokoa() {
  const auth = useAuth()

  // Build fetch options with auth header when available
  function authHeaders(): Record<string, string> {
    const token = auth.getAccessToken()
    if (token) {
      return { Authorization: `Bearer ${token}` }
    }
    return {}
  }

  function listAgents(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agents`
      : `${API_BASE}/agents`
    return useFetch<AgentList>(path, {
      lazy: true,
      headers: authHeaders(),
      onResponseError({ response }) {
        if (response.status === 401) {
          auth.logout()
          navigateTo('/login')
        }
      }
    })
  }

  function listModels(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/models`
      : `${API_BASE}/models`
    return useFetch<ModelList>(path, {
      lazy: true,
      headers: authHeaders(),
      onResponseError({ response }) {
        if (response.status === 401) {
          auth.logout()
          navigateTo('/login')
        }
      }
    })
  }

  function listModelProviders(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/modelproviders`
      : `${API_BASE}/modelproviders`
    return useFetch<ModelProviderList>(path, {
      lazy: true,
      headers: authHeaders(),
      onResponseError({ response }) {
        if (response.status === 401) {
          auth.logout()
          navigateTo('/login')
        }
      }
    })
  }

  function listAgentTools(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agenttools`
      : `${API_BASE}/agenttools`
    return useFetch<AgentToolList>(path, {
      lazy: true,
      headers: authHeaders(),
      onResponseError({ response }) {
        if (response.status === 401) {
          auth.logout()
          navigateTo('/login')
        }
      }
    })
  }

  return {
    listAgents,
    listModels,
    listModelProviders,
    listAgentTools
  }
}
