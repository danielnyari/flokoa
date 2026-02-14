import type { AgentList, ModelList, ModelProviderList, AgentToolList } from '~/types'

const API_BASE = '/api/v1alpha1'

export function useFlokoa() {
  function listAgents(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agents`
      : `${API_BASE}/agents`
    return useFetch<AgentList>(path, { lazy: true })
  }

  function listModels(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/models`
      : `${API_BASE}/models`
    return useFetch<ModelList>(path, { lazy: true })
  }

  function listModelProviders(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/modelproviders`
      : `${API_BASE}/modelproviders`
    return useFetch<ModelProviderList>(path, { lazy: true })
  }

  function listAgentTools(namespace?: string) {
    const path = namespace
      ? `${API_BASE}/namespaces/${namespace}/agenttools`
      : `${API_BASE}/agenttools`
    return useFetch<AgentToolList>(path, { lazy: true })
  }

  return {
    listAgents,
    listModels,
    listModelProviders,
    listAgentTools
  }
}
