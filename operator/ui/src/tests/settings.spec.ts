import { test, expect, mockApiRoutes } from './fixtures'

test.describe('Settings page', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('shows the Settings heading', async ({ page, goto }) => {
    await goto('/settings', { waitUntil: 'hydration' })
    await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  })

  test('shows settings sub-navigation tabs', async ({ page, goto }) => {
    await goto('/settings', { waitUntil: 'hydration' })

    await expect(page.getByRole('link', { name: 'General' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Members' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Notifications' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Security' })).toBeVisible()
  })

  test('General tab shows profile form', async ({ page, goto }) => {
    await goto('/settings', { waitUntil: 'hydration' })

    await expect(page.getByText('Profile')).toBeVisible()
    await expect(page.getByLabel('Name')).toBeVisible()
    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByLabel('Username')).toBeVisible()
  })

  test('General tab has Save changes button', async ({ page, goto }) => {
    await goto('/settings', { waitUntil: 'hydration' })

    await expect(page.getByRole('button', { name: 'Save changes' })).toBeVisible()
  })

  test('navigates to Security tab', async ({ page, goto }) => {
    await goto('/settings/security', { waitUntil: 'hydration' })

    await expect(page.getByText('Authentication')).toBeVisible()
  })

  test('Security page shows auth-disabled alert when auth is off', async ({ page, goto }) => {
    await goto('/settings/security', { waitUntil: 'hydration' })

    await expect(page.getByText('Authentication disabled')).toBeVisible()
    await expect(page.getByText('Enable Dex in the Helm chart')).toBeVisible()
  })

  test('navigates between settings tabs', async ({ page, goto }) => {
    await goto('/settings', { waitUntil: 'hydration' })

    // Navigate to Security
    await page.getByRole('link', { name: 'Security' }).click()
    await expect(page).toHaveURL(/\/settings\/security/)
    await expect(page.getByText('Authentication')).toBeVisible()

    // Navigate back to General
    await page.getByRole('link', { name: 'General' }).click()
    await expect(page).toHaveURL(/\/settings$/)
    await expect(page.getByText('Profile')).toBeVisible()
  })
})
