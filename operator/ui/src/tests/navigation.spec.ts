import { test, expect, mockApiRoutes } from './fixtures'

test.describe('Sidebar navigation', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('sidebar shows Flokoa branding', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })
    await expect(page.getByText('Flokoa')).toBeVisible()
  })

  test('sidebar contains all main navigation links', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    const nav = page.locator('nav')
    await expect(nav.getByText('Home')).toBeVisible()
    await expect(nav.getByText('Agents')).toBeVisible()
    await expect(nav.getByText('Models')).toBeVisible()
    await expect(nav.getByText('Providers')).toBeVisible()
    await expect(nav.getByText('Tools')).toBeVisible()
    await expect(nav.getByText('Settings')).toBeVisible()
  })

  test('navigating to Agents page via sidebar', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await page.locator('nav').getByText('Agents').click()
    await expect(page).toHaveURL(/\/agents/)
    await expect(page.getByRole('heading', { name: 'Agents' })).toBeVisible()
  })

  test('navigating to Models page via sidebar', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await page.locator('nav').getByText('Models').click()
    await expect(page).toHaveURL(/\/models/)
    await expect(page.getByRole('heading', { name: 'Models' })).toBeVisible()
  })

  test('navigating to Providers page via sidebar', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await page.locator('nav').getByText('Providers').click()
    await expect(page).toHaveURL(/\/providers/)
    await expect(page.getByRole('heading', { name: 'Model Providers' })).toBeVisible()
  })

  test('navigating to Tools page via sidebar', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await page.locator('nav').getByText('Tools').click()
    await expect(page).toHaveURL(/\/tools/)
    await expect(page.getByRole('heading', { name: 'Agent Tools' })).toBeVisible()
  })

  test('navigating back Home from another page', async ({ page, goto }) => {
    await goto('/agents', { waitUntil: 'hydration' })

    await page.locator('nav').getByText('Home').click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  })

  test('sidebar footer shows user menu', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    // When auth is disabled, shows "Admin"
    await expect(page.getByText('Admin')).toBeVisible()
  })

  test('sidebar shows Documentation and GitHub links', async ({ page, goto }) => {
    await goto('/', { waitUntil: 'hydration' })

    await expect(page.getByText('Documentation')).toBeVisible()
    await expect(page.getByText('GitHub')).toBeVisible()
  })
})
