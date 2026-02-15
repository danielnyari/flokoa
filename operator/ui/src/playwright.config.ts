import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'
import type { ConfigOptions } from '@nuxt/test-utils/playwright'

export default defineConfig<ConfigOptions>({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? 'github' : 'html',
  timeout: 120_000,
  expect: {
    timeout: 10_000
  },
  use: {
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    nuxt: {
      rootDir: fileURLToPath(new URL('.', import.meta.url)),
      // Use dev mode to avoid a full build per test worker
      dev: true,
      // Override the static preset so @nuxt/test-utils can start a server
      nuxtConfig: {
        nitro: {
          preset: 'node-server'
        }
      }
    }
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ]
})
