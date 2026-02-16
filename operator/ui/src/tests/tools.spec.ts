import { test, expect, mockApiRoutes, mockTools } from './fixtures'

test.describe('Tools page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Agent Tools heading', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Agent Tools' })).toBeVisible()
  })

  test('renders tools table with all rows', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    for (const tool of mockTools.items) {
      await expect(page.getByText(tool.metadata.name)).toBeVisible()
    }
  })

  test('shows tool type badges', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    // Both tools are openapi type
    const typeBadges = page.getByText('openapi')
    await expect(typeBadges.first()).toBeVisible()
  })

  test('shows tool descriptions', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('Provides real-time weather data')).toBeVisible()
    await expect(page.getByText('Search the web for relevant')).toBeVisible()
  })

  test('shows URL source for URL-based tools', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('https://api.weather.example.com/openapi.json')).toBeVisible()
  })

  test('shows service reference for service-based tools', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('search-service:8080')).toBeVisible()
  })

  test('shows timeout values', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('30s').first()).toBeVisible()
    await expect(page.getByText('60s')).toBeVisible()
  })

  test('shows tool count in footer', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('2 tool(s)')).toBeVisible()
  })

  test('search input filters tools by name', async ({ page, goto }) => {
    await goto('/tools', { waitUntil: 'hydration' })

    const searchInput = page.getByPlaceholder('Filter tools...')
    await searchInput.fill('weather')
    await expect(page.getByText('weather-tool')).toBeVisible()
    await expect(page.getByText('search-tool')).not.toBeVisible()
    await expect(page.getByText('1 tool(s)')).toBeVisible()
  })

  test('empty state when no tools', async ({ page, goto }) => {
    await mockApiRoutes(page, { tools: { items: [] } })
    await goto('/tools', { waitUntil: 'hydration' })

    await expect(page.getByText('0 tool(s)')).toBeVisible()
  })
})
