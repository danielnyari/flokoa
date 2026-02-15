import { test, expect, mockApiRoutes, mockProviders } from './fixtures'

test.describe('Providers page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Model Providers heading', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Model Providers' })).toBeVisible()
  })

  test('renders providers table with all rows', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    for (const provider of mockProviders.items) {
      await expect(page.getByText(provider.metadata.name)).toBeVisible()
    }
  })

  test('shows provider type badges', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('openai')).toBeVisible()
    await expect(page.getByText('anthropic')).toBeVisible()
    await expect(page.getByText('bedrock')).toBeVisible()
  })

  test('shows endpoint URLs', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('https://api.openai.com/v1')).toBeVisible()
    await expect(page.getByText('https://api.anthropic.com')).toBeVisible()
  })

  test('shows bedrock region', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('us-east-1')).toBeVisible()
  })

  test('shows API key secret names', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('openai-api-key')).toBeVisible()
    await expect(page.getByText('anthropic-api-key')).toBeVisible()
  })

  test('shows ready and not-ready status badges', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    const readyBadges = page.getByText('Ready', { exact: true })
    await expect(readyBadges.first()).toBeVisible()
    await expect(page.getByText('Not Ready')).toBeVisible()
  })

  test('shows provider count in footer', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('3 provider(s)')).toBeVisible()
  })

  test('search input filters providers by name', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    const searchInput = page.getByPlaceholder('Filter providers...')
    await searchInput.fill('openai')
    await expect(page.getByText('openai-provider')).toBeVisible()
    await expect(page.getByText('anthropic-provider')).not.toBeVisible()
    await expect(page.getByText('1 provider(s)')).toBeVisible()
  })

  test('provider type filter selects specific types', async ({ page, goto }) => {
    await goto('/providers', { waitUntil: 'hydration' })

    // Open the provider type filter and choose "Bedrock"
    const typeSelect = page.locator('select, [role="combobox"]').filter({ hasText: 'All' }).first()
    await typeSelect.click()
    await page.getByRole('option', { name: 'Bedrock' }).click()

    await expect(page.getByText('bedrock-provider')).toBeVisible()
    await expect(page.getByText('openai-provider')).not.toBeVisible()
    await expect(page.getByText('1 provider(s)')).toBeVisible()
  })

  test('empty state when no providers', async ({ page, goto }) => {
    await mockApiRoutes(page, { providers: { items: [] } })
    await goto('/providers', { waitUntil: 'hydration' })

    await expect(page.getByText('0 provider(s)')).toBeVisible()
  })
})
