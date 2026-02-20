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
export type Framework = 'pydantic-ai' | 'langchain' | 'google-adk' | 'marvin' | 'autogen' | 'a2a'
export type RuntimeType = 'standard' | 'template'

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

export interface AgentSpec {
  card?: AgentCardOverride
  runtime?: {
    type?: RuntimeType
    standard?: {
      container?: Record<string, unknown>
      replicas?: number
    }
    template?: {
      config?: Record<string, unknown>
      replicas?: number
    }
  }
  model?: {
    name: string
    namespace?: string
  }
  instruction?: {
    template?: string
    instructionRef?: {
      name: string
      namespace?: string
    }
  }
  framework?: Framework
  tools?: Array<{
    name?: string
    template?: Record<string, unknown>
    toolRef?: {
      name: string
      namespace?: string
    }
  }>
}

export interface AgentStatus {
  phase?: AgentPhase
  backend?: string
  url?: string
  replicas?: number
  availableReplicas?: number
  detectedFramework?: Framework
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
  parameters?: {
    temperature?: string
    maxTokens?: number
    topP?: string
    topK?: number
    presencePenalty?: string
    frequencyPenalty?: string
    timeOut?: number
    parallelToolCalls?: boolean
    stopSequences?: string[]
    seed?: number
    openai?: Record<string, unknown>
    anthropic?: Record<string, unknown>
    google?: Record<string, unknown>
    bedrock?: Record<string, unknown>
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

export type AgentToolType = 'openapi'

export interface AgentToolSpec {
  type: AgentToolType
  description: string
  openApi?: {
    url?: string
    serviceRef?: {
      name: string
      namespace?: string
      port?: number
      portName?: string
    }
    openApiSchema?: {
      value?: Record<string, unknown>
      valueFrom?: {
        name: string
        key: string
      }
      endpointPath?: string
    }
    timeoutSeconds?: number
    headers?: Record<string, string>
  }
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
