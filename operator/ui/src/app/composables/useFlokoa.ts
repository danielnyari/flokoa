import type { AgentList, ModelList, ModelProviderList, AgentToolList, AgentWorkflowList, AgentWorkflow, WorkflowRunList, WorkflowRun } from '~/types'

const API_BASE = '/api/v1alpha1'

export function useFlokoa() {
  const auth = useAuth()
  const { current: currentNamespace } = useNamespace()

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

  // Build path with optional namespace override.
  // If no explicit namespace is passed, falls back to the global namespace filter.
  function namespacedPath(resource: string, namespace?: string): string {
    const ns = namespace ?? currentNamespace.value
    return ns
      ? `${API_BASE}/namespaces/${ns}/${resource}`
      : `${API_BASE}/${resource}`
  }

  function listAgents(namespace?: string) {
    const path = computed(() => namespacedPath('agents', namespace))
    return useFetch<AgentList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listModels(namespace?: string) {
    const path = computed(() => namespacedPath('models', namespace))
    return useFetch<ModelList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listModelProviders(namespace?: string) {
    const path = computed(() => namespacedPath('modelproviders', namespace))
    return useFetch<ModelProviderList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listAgentTools(namespace?: string) {
    const path = computed(() => namespacedPath('agenttools', namespace))
    return useFetch<AgentToolList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listAgentWorkflows(namespace?: string) {
    const path = computed(() => namespacedPath('agentworkflows', namespace))
    return useFetch<AgentWorkflowList>(path, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function getAgentWorkflow(namespace: string, name: string) {
    return useFetch<AgentWorkflow>(`${API_BASE}/namespaces/${namespace}/agentworkflows/${name}`, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function listWorkflowRuns(namespace: string, workflowName: string) {
    return useFetch<WorkflowRunList>(`${API_BASE}/namespaces/${namespace}/agentworkflows/${workflowName}/runs`, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function getWorkflowRun(namespace: string, workflowName: string, runName: string) {
    return useFetch<WorkflowRun>(`${API_BASE}/namespaces/${namespace}/agentworkflows/${workflowName}/runs/${runName}`, {
      lazy: true,
      headers: computed(() => authHeaders()),
      onResponseError
    })
  }

  function submitWorkflowRun(namespace: string, workflowName: string, parameters?: Record<string, string>) {
    return $fetch<WorkflowRun>(`${API_BASE}/namespaces/${namespace}/agentworkflows/${workflowName}/runs`, {
      method: 'POST',
      headers: authHeaders(),
      body: parameters ? { parameters } : {},
      onResponseError
    })
  }

  // Watch URL builders for useListWatch composable.
  // These return URL strings for the SSE watch endpoints.

  function watchUrl(resource: string, namespace?: string): string {
    const ns = namespace ?? currentNamespace.value
    return ns
      ? `${API_BASE}/watch/namespaces/${ns}/${resource}`
      : `${API_BASE}/watch/${resource}`
  }

  function watchWorkflowRunsUrl(namespace: string, workflowName: string): string {
    return `${API_BASE}/watch/namespaces/${namespace}/agentworkflows/${workflowName}/runs`
  }

  return {
    listAgents,
    listModels,
    listModelProviders,
    listAgentTools,
    listAgentWorkflows,
    getAgentWorkflow,
    listWorkflowRuns,
    getWorkflowRun,
    submitWorkflowRun,
    // URL builders for list-watch pattern
    namespacedPath,
    watchUrl,
    watchWorkflowRunsUrl
  }
}
