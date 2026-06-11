// Flokoa CRD types matching the operator gRPC/REST API responses

// Common Kubernetes metadata
export interface ObjectMeta {
  name: string
  namespace: string
  uid?: string
  resourceVersion?: string
  creationTimestamp?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface Condition {
  type: string
  status: string
  lastTransitionTime?: string
  reason?: string
  message?: string
}

// ─── Agent CRD ──────────────────────────────────────────────────────

export type AgentPhase = 'Pending' | 'Running' | 'Failed'
export type IsolationTier = 'shared' | 'session'

export interface AgentSkill {
  id: string
  name: string
  description?: string
  tags?: string[]
  examples?: string[]
  inputModes?: string[]
  outputModes?: string[]
}

export interface AgentCardOverride {
  name?: string
  description?: string
  version?: string
  defaultInputModes?: string[]
  defaultOutputModes?: string[]
  skills?: AgentSkill[]
}

export interface NamespacedRef {
  name: string
  namespace?: string
}

export interface AgentSpec {
  card?: AgentCardOverride
  runtime?: {
    image?: string
    runnerVersion?: string
    isolation?: IsolationTier
    replicas?: number
    env?: Array<{ name: string, value?: string }>
    resources?: Record<string, unknown>
  }
  // Inline pydantic-ai AgentSpec fragment (JSON form).
  spec?: Record<string, unknown>
  modelRef?: NamespacedRef
  instructionRefs?: NamespacedRef[]
  tools?: NamespacedRef[]
  secretRefs?: Record<string, { name: string, key: string }>
}

export interface AgentStatus {
  phase?: AgentPhase
  url?: string
  specHash?: string
  runnerVersion?: string
  injectedCapabilities?: string[]
  replicas?: number
  availableReplicas?: number
  conditions?: Condition[]
  observedGeneration?: number
}

export interface Agent {
  metadata: ObjectMeta
  spec: AgentSpec
  status?: AgentStatus
}

export interface AgentList {
  items: Agent[]
}

// ─── Model CRD ──────────────────────────────────────────────────────

export type ProviderType = 'openai' | 'anthropic' | 'google' | 'bedrock'

export interface ModelSpec {
  model: string
  providerRef: {
    name: string
    namespace?: string
  }
  settings?: {
    temperature?: string
    maxTokens?: number
    topP?: string
    topK?: number
    presencePenalty?: string
    frequencyPenalty?: string
    timeoutSeconds?: number
    parallelToolCalls?: boolean
    stopSequences?: string[]
    seed?: number
    extra?: Record<string, unknown>
  }
}

export interface ModelStatus {
  conditions?: Condition[]
  observedGeneration?: number
  resolvedProvider?: {
    provider: ProviderType
    namespace: string
    name: string
  }
  ready?: boolean
}

export interface Model {
  metadata: ObjectMeta
  spec: ModelSpec
  status?: ModelStatus
}

export interface ModelList {
  items: Model[]
}

// ─── ModelProvider CRD ───────────────────────────────────────────────

export interface ModelProviderSpec {
  apiKeySecretRef?: {
    name: string
    key?: string
  }
  openai?: {
    baseURL?: string
  }
  anthropic?: {
    baseURL?: string
  }
  google?: {
    project?: string
    location?: string
  }
  bedrock?: {
    region?: string
  }
  tls?: {
    insecureSkipVerify?: boolean
    useSystemCAs?: boolean
  }
  defaultHeaders?: Record<string, string>
}

export interface ModelProviderStatus {
  provider?: ProviderType
  conditions?: Condition[]
  observedGeneration?: number
  secretHash?: string
  ready?: boolean
}

export interface ModelProvider {
  metadata: ObjectMeta
  spec: ModelProviderSpec
  status?: ModelProviderStatus
}

export interface ModelProviderList {
  items: ModelProvider[]
}

// ─── AgentTool CRD ──────────────────────────────────────────────────

export type AgentToolType = 'mcp' | 'openapi'
export type MCPTransport = 'streamableHTTP' | 'sse'

export interface AgentToolSpec {
  type?: AgentToolType
  description?: string
  url?: string
  serviceRef?: {
    name: string
    namespace?: string
    port?: number
    portName?: string
  }
  path?: string
  transport?: MCPTransport
  headers?: Record<string, string>
  headerSecrets?: Array<{ name: string, secretRef: { name: string, key: string } }>
  toolPrefix?: string
  allowedTools?: string[]
  timeoutSeconds?: number
}

export interface AgentToolStatus {
  conditions?: Condition[]
  observedGeneration?: number
}

export interface AgentTool {
  metadata: ObjectMeta
  spec: AgentToolSpec
  status?: AgentToolStatus
}

export interface AgentToolList {
  items: AgentTool[]
}

// ─── AgentWorkflow CRD ──────────────────────────────────────────────

export interface WorkflowParam {
  name: string
  description?: string
  value?: string
}

export interface WorkflowTask {
  name: string
  type: string // "agent", "agentTask", "switch"
  dependsOn?: string[]
  condition?: string
}

export interface AgentWorkflowSpec {
  description?: string
  params?: WorkflowParam[]
  tasks: WorkflowTask[]
  timeout?: string
}

export interface AgentWorkflowStatus {
  ready?: boolean
  workflowTemplateName?: string
  specHash?: string
  conditions?: Condition[]
  observedGeneration?: number
}

export interface AgentWorkflow {
  metadata: ObjectMeta
  spec: AgentWorkflowSpec
  status?: AgentWorkflowStatus
}

export interface AgentWorkflowList {
  items: AgentWorkflow[]
}

// ─── Workflow Run (Argo Workflow) ───────────────────────────────────

export type RunPhase
  = 'RUN_PHASE_UNSPECIFIED'
    | 'RUN_PHASE_PENDING'
    | 'RUN_PHASE_RUNNING'
    | 'RUN_PHASE_SUCCEEDED'
    | 'RUN_PHASE_FAILED'
    | 'RUN_PHASE_ERROR'

export type NodeType
  = 'NODE_TYPE_UNSPECIFIED'
    | 'NODE_TYPE_POD'
    | 'NODE_TYPE_STEPS'
    | 'NODE_TYPE_DAG'
    | 'NODE_TYPE_TASK_GROUP'
    | 'NODE_TYPE_RETRY'
    | 'NODE_TYPE_SKIPPED'
    | 'NODE_TYPE_SUSPEND'
    | 'NODE_TYPE_PLUGIN'

export interface WorkflowRunNode {
  id: string
  name: string
  displayName: string
  type: NodeType
  phase: RunPhase
  startedAt?: string
  finishedAt?: string
  message?: string
  templateName?: string
  children?: string[]
  inputs?: Record<string, string>
  outputs?: Record<string, string>
}

export interface WorkflowRun {
  metadata: ObjectMeta
  phase: RunPhase
  startedAt?: string
  finishedAt?: string
  progress?: string
  message?: string
  nodes?: WorkflowRunNode[]
  parameters?: Record<string, string>
}

export interface WorkflowRunList {
  items: WorkflowRun[]
}

// ─── A2A Agent Card (from /.well-known/agent.json) ─────────────────

export interface A2AAgentCard {
  name: string
  description: string
  url: string
  version?: string
  provider?: {
    organization: string
    url?: string
  }
  documentationUrl?: string
  capabilities?: {
    streaming?: boolean
    pushNotifications?: boolean
    stateTransitionHistory?: boolean
  }
  authentication?: {
    schemes?: string[]
    credentials?: unknown
  }
  defaultInputModes?: string[]
  defaultOutputModes?: string[]
  skills?: Array<{
    id: string
    name: string
    description?: string
    tags?: string[]
    examples?: string[]
    inputModes?: string[]
    outputModes?: string[]
  }>
}

// ─── Dashboard types ────────────────────────────────────────────────

export interface Stat {
  title: string
  icon: string
  value: number | string
  variation?: number
}

export interface Notification {
  id: number
  unread?: boolean
  body: string
  date: string
}
