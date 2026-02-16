import { test, expect, mockApiRoutes, mockModels } from './fixtures'

test.describe('Models page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Models heading', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Models' })).toBeVisible()
  })

  test('renders models table with all rows', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    for (const model of mockModels.items) {
      await expect(page.getByText(model.metadata.name)).toBeVisible()
    }
  })

  test('displays model IDs', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('gpt-4o').first()).toBeVisible()
    await expect(page.getByText('claude-sonnet-4-20250514')).toBeVisible()
  })

  test('shows provider type badges', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('openai')).toBeVisible()
    await expect(page.getByText('anthropic')).toBeVisible()
  })

  test('shows temperature values', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('0.7')).toBeVisible()
    await expect(page.getByText('0.5')).toBeVisible()
  })

  test('shows max token values', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('4,096')).toBeVisible()
    await expect(page.getByText('8,192')).toBeVisible()
  })

  test('shows ready status badges', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    // Both models are ready
    const readyBadges = page.getByText('Ready', { exact: true })
    await expect(readyBadges.first()).toBeVisible()
  })

  test('shows model count in footer', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('2 model(s)')).toBeVisible()
  })

  test('search input filters models by name', async ({ page, goto }) => {
    await goto('/models', { waitUntil: 'hydration' })

    const searchInput = page.getByPlaceholder('Filter models...')
    await searchInput.fill('gpt')
    await expect(page.getByText('gpt-4o').first()).toBeVisible()
    await expect(page.getByText('claude-sonnet')).not.toBeVisible()
    await expect(page.getByText('1 model(s)')).toBeVisible()
  })

  test('empty state when no models', async ({ page, goto }) => {
    await mockApiRoutes(page, { models: { items: [] } })
    await goto('/models', { waitUntil: 'hydration' })

    await expect(page.getByText('0 model(s)')).toBeVisible()
  })
})
