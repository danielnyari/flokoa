import { test, expect, mockApiRoutes, mockAgents } from './fixtures'

test.describe('Agents page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Agents heading', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Agents' })).toBeVisible()
  })

  test('renders agents table with all rows', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    for (const agent of mockAgents.items) {
      await expect(page.getByText(agent.metadata.name)).toBeVisible()
    }
  })

  test('shows namespace for each agent', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('default').first()).toBeVisible()
    await expect(page.getByText('production')).toBeVisible()
  })

  test('displays phase badges with correct text', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('Running')).toBeVisible()
    await expect(page.getByText('Pending')).toBeVisible()
    await expect(page.getByText('Failed')).toBeVisible()
  })

  test('shows framework column', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('pydantic-ai')).toBeVisible()
    await expect(page.getByText('langchain')).toBeVisible()
  })

  test('shows agent count in footer', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('3 agent(s)')).toBeVisible()
  })

  test('search input filters agents by name', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    const searchInput = page.getByPlaceholder('Filter agents...')
    await expect(searchInput).toBeVisible()

    await searchInput.fill('weather')
    await expect(page.getByText('weather-agent')).toBeVisible()
    await expect(page.getByText('search-agent')).not.toBeVisible()
    await expect(page.getByText('1 agent(s)')).toBeVisible()
  })

  test('phase filter selects specific phases', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    // Open the phase filter select and choose "Failed"
    const phaseSelect = page.locator('select, [role="combobox"]').filter({ hasText: 'All' }).first()
    await phaseSelect.click()
    await page.getByRole('option', { name: 'Failed' }).click()

    await expect(page.getByText('broken-agent')).toBeVisible()
    await expect(page.getByText('weather-agent')).not.toBeVisible()
    await expect(page.getByText('1 agent(s)')).toBeVisible()
  })

  test('shows URL for agents that have one', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('http://weather-agent.default.svc:8080')).toBeVisible()
  })

  test('shows ready replicas as fraction', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    // weather-agent has 2/2 ready
    await expect(page.getByText('2/2')).toBeVisible()
  })

  test('empty state when no agents', async ({ page, goto }) => {
    await mockApiRoutes(page, { agents: { items: [] } })
    await goto('/agents', { waitUntil: 'hydration' })

    await expect(page.getByText('0 agent(s)')).toBeVisible()
  })

  test('clearing search shows all agents again', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    const searchInput = page.getByPlaceholder('Filter agents...')
    await searchInput.fill('weather')
    await expect(page.getByText('1 agent(s)')).toBeVisible()

    await searchInput.clear()
    await expect(page.getByText('3 agent(s)')).toBeVisible()
  })
})
