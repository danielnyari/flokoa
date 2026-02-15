import { test, expect, mockApiRoutes, mockAgents, mockModels, mockProviders, mockTools } from './fixtures'

test.describe('Dashboard page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Overview heading', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  })

  test('renders all four stat cards', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    // Wait for data to load (stats stop showing "...")
    await expect(page.getByText('Agents').first()).toBeVisible()
    await expect(page.getByText('Models').first()).toBeVisible()
    await expect(page.getByText('Providers').first()).toBeVisible()
    await expect(page.getByText('Tools').first()).toBeVisible()
  })

  test('displays correct agent count and status', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    // Total agents count
    const agentsCard = page.locator('a[href="/agents"]')
    await expect(agentsCard.getByText(String(mockAgents.items.length))).toBeVisible()
    // Description shows running/failed breakdown
    await expect(agentsCard.getByText('1 running, 1 failed')).toBeVisible()
  })

  test('displays correct model count', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    const modelsCard = page.locator('a[href="/models"]')
    await expect(modelsCard.getByText(String(mockModels.items.length))).toBeVisible()
    await expect(modelsCard.getByText('2 ready')).toBeVisible()
  })

  test('displays correct provider count', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    const providersCard = page.locator('a[href="/providers"]')
    await expect(providersCard.getByText(String(mockProviders.items.length))).toBeVisible()
    await expect(providersCard.getByText('2 ready')).toBeVisible()
  })

  test('displays correct tool count', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    const toolsCard = page.locator('a[href="/tools"]')
    await expect(toolsCard.getByText(String(mockTools.items.length))).toBeVisible()
    await expect(toolsCard.getByText('2 configured')).toBeVisible()
  })

  test('shows Recent Agents section with agent entries', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await expect(page.getByText('Recent Agents')).toBeVisible()
    // Each agent name should appear in the list
    await expect(page.getByText('weather-agent')).toBeVisible()
    await expect(page.getByText('search-agent')).toBeVisible()
    await expect(page.getByText('broken-agent')).toBeVisible()
  })

  test('shows agent phase badges in recent agents', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await expect(page.getByText('Running')).toBeVisible()
    await expect(page.getByText('Pending')).toBeVisible()
    await expect(page.getByText('Failed')).toBeVisible()
  })

  test('stat cards link to their respective pages', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    // Click Agents stat card and verify navigation
    await page.locator('a[href="/agents"]').click()
    await expect(page).toHaveURL(/\/agents/)
  })

  test('"View all" button links to agents page', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    const viewAll = page.getByRole('link', { name: 'View all' })
    await expect(viewAll).toBeVisible()
    await expect(viewAll).toHaveAttribute('href', '/agents')
  })

  test('shows empty state when no agents exist', async ({ page, goto }) => {
    await mockApiRoutes(page, {
      agents: { items: [] },
      models: { items: [] },
      providers: { items: [] },
      tools: { items: [] }
    })
    await goto('/', { waitUntil: 'hydration' })

    await expect(page.getByText('No agents found')).toBeVisible()
  })
})
