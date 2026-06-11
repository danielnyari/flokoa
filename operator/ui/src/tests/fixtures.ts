/**
 * Shared test fixtures for Playwright E2E tests.
 *
 * Provides API route mocking so pages can render without a real backend.
 * The auth config endpoint returns { enabled: false } by default,
 * which lets the auth middleware allow all routes.
 */
import { test as base, expect } from '@nuxt/test-utils/playwright'
import type { Page, Route } from '@playwright/test'

// ─── Mock data ──────────────────────────────────────────────────────

export const mockAgents = {
  items: [
    {
      metadata: {
        name: 'weather-agent',
        namespace: 'default',
        uid: 'agent-1',
        creationTimestamp: new Date(Date.now() - 3600_000).toISOString()
      },
      spec: {
        runtime: { replicas: 2 },
        spec: { model: 'openai:gpt-5-mini' }
      },
      status: {
        phase: 'Running',
        url: 'http://weather-agent.default.svc:8080',
        replicas: 2,
        availableReplicas: 2,
        detectedFramework: 'pydantic-ai'
      }
    },
    {
      metadata: {
        name: 'search-agent',
        namespace: 'production',
        uid: 'agent-2',
        creationTimestamp: new Date(Date.now() - 7200_000).toISOString()
      },
      spec: {
        runtime: { replicas: 1 },
        spec: { model: 'anthropic:claude-sonnet-4-5' }
      },
      status: {
        phase: 'Pending',
        replicas: 1,
        availableReplicas: 0
      }
    },
    {
      metadata: {
        name: 'broken-agent',
        namespace: 'default',
        uid: 'agent-3',
        creationTimestamp: new Date(Date.now() - 86400_000).toISOString()
      },
      spec: {
        runtime: { replicas: 1 },
        spec: { model: 'openai:gpt-5-mini' }
      },
      status: {
        phase: 'Failed',
        replicas: 1,
        availableReplicas: 0
      }
    }
  ]
}

export const mockModels = {
  items: [
    {
      metadata: {
        name: 'gpt-4o',
        namespace: 'default',
        uid: 'model-1',
        creationTimestamp: new Date(Date.now() - 3600_000).toISOString()
      },
      spec: {
        model: 'gpt-4o',
        providerRef: { name: 'openai-provider' },
        settings: { temperature: '0.7', maxTokens: 4096 }
      },
      status: {
        ready: true,
        resolvedProvider: { provider: 'openai', namespace: 'default', name: 'openai-provider' }
      }
    },
    {
      metadata: {
        name: 'claude-sonnet',
        namespace: 'default',
        uid: 'model-2',
        creationTimestamp: new Date(Date.now() - 7200_000).toISOString()
      },
      spec: {
        model: 'claude-sonnet-4-20250514',
        providerRef: { name: 'anthropic-provider' },
        settings: { temperature: '0.5', maxTokens: 8192 }
      },
      status: {
        ready: true,
        resolvedProvider: { provider: 'anthropic', namespace: 'default', name: 'anthropic-provider' }
      }
    }
  ]
}

export const mockProviders = {
  items: [
    {
      metadata: {
        name: 'openai-provider',
        namespace: 'default',
        uid: 'provider-1',
        creationTimestamp: new Date(Date.now() - 86400_000).toISOString()
      },
      spec: {
        openai: { baseURL: 'https://api.openai.com/v1' },
        apiKeySecretRef: { name: 'openai-api-key', key: 'token' }
      },
      status: {
        provider: 'openai',
        ready: true
      }
    },
    {
      metadata: {
        name: 'anthropic-provider',
        namespace: 'default',
        uid: 'provider-2',
        creationTimestamp: new Date(Date.now() - 86400_000).toISOString()
      },
      spec: {
        anthropic: { baseURL: 'https://api.anthropic.com' },
        apiKeySecretRef: { name: 'anthropic-api-key', key: 'token' }
      },
      status: {
        provider: 'anthropic',
        ready: true
      }
    },
    {
      metadata: {
        name: 'bedrock-provider',
        namespace: 'production',
        uid: 'provider-3',
        creationTimestamp: new Date(Date.now() - 172800_000).toISOString()
      },
      spec: {
        bedrock: { region: 'us-east-1' }
      },
      status: {
        provider: 'bedrock',
        ready: false
      }
    }
  ]
}

export const mockTools = {
  items: [
    {
      metadata: {
        name: 'weather-tool',
        namespace: 'default',
        uid: 'tool-1',
        creationTimestamp: new Date(Date.now() - 3600_000).toISOString()
      },
      spec: {
        type: 'mcp',
        description: 'Provides real-time weather data for any location worldwide.',
        url: 'https://api.weather.example.com/mcp',
        timeoutSeconds: 30
      }
    },
    {
      metadata: {
        name: 'search-tool',
        namespace: 'default',
        uid: 'tool-2',
        creationTimestamp: new Date(Date.now() - 7200_000).toISOString()
      },
      spec: {
        type: 'mcp',
        description: 'Search the web for relevant documents and information.',
        serviceRef: { name: 'search-service', port: 8080 },
        path: '/mcp',
        timeoutSeconds: 60
      }
    }
  ]
}

export const authConfigDisabled = { enabled: false, issuerUrl: '', clientId: '' }

// ─── Route handler ──────────────────────────────────────────────────

/** Intercept all Flokoa API routes with mock data. */
export async function mockApiRoutes(page: Page, overrides?: {
  agents?: typeof mockAgents
  models?: typeof mockModels
  providers?: typeof mockProviders
  tools?: typeof mockTools
  authConfig?: typeof authConfigDisabled
}) {
  const agents = overrides?.agents ?? mockAgents
  const models = overrides?.models ?? mockModels
  const providers = overrides?.providers ?? mockProviders
  const tools = overrides?.tools ?? mockTools
  const authConfig = overrides?.authConfig ?? authConfigDisabled

  await page.route('**/api/v1alpha1/auth/config', (route: Route) => {
    return route.fulfill({ json: authConfig })
  })
  await page.route('**/api/v1alpha1/agents', (route: Route) => {
    return route.fulfill({ json: agents })
  })
  await page.route('**/api/v1alpha1/models', (route: Route) => {
    return route.fulfill({ json: models })
  })
  await page.route('**/api/v1alpha1/modelproviders', (route: Route) => {
    return route.fulfill({ json: providers })
  })
  await page.route('**/api/v1alpha1/agenttools', (route: Route) => {
    return route.fulfill({ json: tools })
  })
}

// Re-export for convenience
export { expect }
export const test = base
