// Global enum display mappings for proto enum values.
// The gRPC-gateway returns proto string names (e.g. "AGENT_PHASE_RUNNING"),
// while the SSE watch endpoint returns numeric values (e.g. 2).
// These utilities handle both formats.

// ─── Agent Phase ────────────────────────────────────────────────────

type BadgeColor = 'success' | 'error' | 'warning' | 'info' | 'neutral'

const AGENT_PHASE_LABELS: Record<string | number, string> = {
  // Proto string names (from gRPC-gateway)
  AGENT_PHASE_PENDING: 'Pending',
  AGENT_PHASE_RUNNING: 'Running',
  AGENT_PHASE_FAILED: 'Failed',
  AGENT_PHASE_UNSPECIFIED: 'Unknown',
  // Numeric values (from SSE watch / encoding/json)
  1: 'Pending',
  2: 'Running',
  3: 'Failed',
  0: 'Unknown',
  // CRD-style strings (just in case)
  Pending: 'Pending',
  Running: 'Running',
  Failed: 'Failed',
}

const AGENT_PHASE_COLORS: Record<string, BadgeColor> = {
  Running: 'success',
  Pending: 'warning',
  Failed: 'error',
  Unknown: 'neutral',
}

export function agentPhaseLabel(value?: string | number | null): string {
  if (value == null) return 'Unknown'
  return AGENT_PHASE_LABELS[value] ?? 'Unknown'
}

export function agentPhaseColor(value?: string | number | null): BadgeColor {
  return AGENT_PHASE_COLORS[agentPhaseLabel(value)] ?? 'neutral'
}

/** Check if a raw phase value represents a specific resolved label (e.g. "Running"). */
export function isAgentPhase(value: string | number | undefined | null, label: string): boolean {
  return agentPhaseLabel(value) === label
}

// ─── Framework ──────────────────────────────────────────────────────

export interface FrameworkInfo {
  label: string
  id: string
  logoLight?: string  // Logo for light mode (in /public)
  logoDark?: string   // Logo for dark mode (in /public)
  url?: string        // Docs URL (logo links here)
}

const FW_PYDANTIC_AI: FrameworkInfo = { label: 'Pydantic AI', id: 'pydantic-ai', logoLight: '/logos/pydantic-ai-light.svg', logoDark: '/logos/pydantic-ai-dark.svg', url: 'https://ai.pydantic.dev' }
const FW_LANGCHAIN: FrameworkInfo = { label: 'LangChain', id: 'langchain', url: 'https://python.langchain.com' }
const FW_GOOGLE_ADK: FrameworkInfo = { label: 'Google ADK', id: 'google-adk', url: 'https://google.github.io/adk-docs' }
const FW_MARVIN: FrameworkInfo = { label: 'Marvin', id: 'marvin', url: 'https://www.askmarvin.ai' }
const FW_AUTOGEN: FrameworkInfo = { label: 'AutoGen', id: 'autogen', url: 'https://microsoft.github.io/autogen' }
const FW_A2A: FrameworkInfo = { label: 'A2A', id: 'a2a', url: 'https://google.github.io/A2A' }
const FW_UNKNOWN: FrameworkInfo = { label: 'Unknown', id: 'unknown' }

const FRAMEWORK_MAP: Record<string | number, FrameworkInfo> = {
  // Proto string names
  FRAMEWORK_PYDANTIC_AI: FW_PYDANTIC_AI,
  FRAMEWORK_LANGCHAIN: FW_LANGCHAIN,
  FRAMEWORK_GOOGLE_ADK: FW_GOOGLE_ADK,
  FRAMEWORK_MARVIN: FW_MARVIN,
  FRAMEWORK_AUTOGEN: FW_AUTOGEN,
  FRAMEWORK_A2A: FW_A2A,
  FRAMEWORK_UNSPECIFIED: FW_UNKNOWN,
  // Numeric values
  1: FW_PYDANTIC_AI,
  2: FW_LANGCHAIN,
  3: FW_GOOGLE_ADK,
  4: FW_MARVIN,
  5: FW_AUTOGEN,
  6: FW_A2A,
  0: FW_UNKNOWN,
  // CRD-style strings
  'pydantic-ai': FW_PYDANTIC_AI,
  'langchain': FW_LANGCHAIN,
  'google-adk': FW_GOOGLE_ADK,
  'marvin': FW_MARVIN,
  'autogen': FW_AUTOGEN,
  'a2a': FW_A2A,
}

export function frameworkInfo(value?: string | number | null): FrameworkInfo | null {
  if (value == null || value === 0 || value === 'FRAMEWORK_UNSPECIFIED') return null
  return FRAMEWORK_MAP[value] ?? null
}

export function frameworkLabel(value?: string | number | null): string | null {
  return frameworkInfo(value)?.label ?? null
}

// ─── Run Phase (Argo Workflow) ──────────────────────────────────────

const RUN_PHASE_LABELS: Record<string | number, string> = {
  RUN_PHASE_PENDING: 'Pending',
  RUN_PHASE_RUNNING: 'Running',
  RUN_PHASE_SUCCEEDED: 'Succeeded',
  RUN_PHASE_FAILED: 'Failed',
  RUN_PHASE_ERROR: 'Error',
  RUN_PHASE_UNSPECIFIED: 'Unknown',
  1: 'Pending',
  2: 'Running',
  3: 'Succeeded',
  4: 'Failed',
  5: 'Error',
  0: 'Unknown',
}

const RUN_PHASE_COLORS: Record<string, BadgeColor> = {
  Running: 'info',
  Succeeded: 'success',
  Failed: 'error',
  Error: 'error',
  Pending: 'warning',
  Unknown: 'neutral',
}

const RUN_PHASE_BORDER_COLORS: Record<string, string> = {
  Running: 'border-info',
  Succeeded: 'border-success',
  Failed: 'border-error',
  Error: 'border-error',
  Pending: 'border-default',
  Unknown: 'border-default',
}

export function runPhaseLabel(value?: string | number | null): string {
  if (value == null) return 'Unknown'
  return RUN_PHASE_LABELS[value] ?? 'Unknown'
}

export function runPhaseColor(value?: string | number | null): BadgeColor {
  return RUN_PHASE_COLORS[runPhaseLabel(value)] ?? 'neutral'
}

export function runPhaseBorderColor(value?: string | number | null): string {
  return RUN_PHASE_BORDER_COLORS[runPhaseLabel(value)] ?? 'border-default'
}

export function isRunPhase(value: string | number | undefined | null, label: string): boolean {
  return runPhaseLabel(value) === label
}

// ─── Runtime Type ───────────────────────────────────────────────────

const RUNTIME_TYPE_LABELS: Record<string | number, string> = {
  RUNTIME_TYPE_STANDARD: 'Standard',
  RUNTIME_TYPE_TEMPLATE: 'Template',
  RUNTIME_TYPE_UNSPECIFIED: 'Unknown',
  1: 'Standard',
  2: 'Template',
  0: 'Unknown',
  standard: 'Standard',
  template: 'Template',
}

export function runtimeTypeLabel(value?: string | number | null): string | null {
  if (value == null || value === 0 || value === 'RUNTIME_TYPE_UNSPECIFIED') return null
  return RUNTIME_TYPE_LABELS[value] ?? null
}

// ─── Node Type ──────────────────────────────────────────────────────

const NODE_TYPE_LABELS: Record<string | number, string> = {
  NODE_TYPE_POD: 'Pod',
  NODE_TYPE_DAG: 'DAG',
  NODE_TYPE_PLUGIN: 'Plugin',
  NODE_TYPE_STEPS: 'Steps',
  NODE_TYPE_SUSPEND: 'Suspend',
  NODE_TYPE_TASK_GROUP: 'Task Group',
  NODE_TYPE_RETRY: 'Retry',
  NODE_TYPE_SKIPPED: 'Skipped',
  NODE_TYPE_UNSPECIFIED: 'Unknown',
}

export function nodeTypeLabel(value?: string | number | null): string {
  if (value == null) return 'Unknown'
  return NODE_TYPE_LABELS[value] ?? 'Unknown'
}

// ─── Timestamp Helpers ─────────────────────────────────────────────

/**
 * Normalise a timestamp value to a Date-compatible input.
 * gRPC-gateway returns ISO 8601 strings, but the SSE watch endpoint
 * uses encoding/json which serialises protobuf Timestamps as
 * { seconds: number, nanos: number }.
 */
export function normaliseTimestamp(value: unknown): string | null {
  if (value == null) return null
  if (typeof value === 'string') return value
  if (typeof value === 'object' && 'seconds' in (value as Record<string, unknown>)) {
    const seconds = Number((value as Record<string, unknown>).seconds)
    if (!Number.isFinite(seconds)) return null
    return new Date(seconds * 1000).toISOString()
  }
  return null
}
