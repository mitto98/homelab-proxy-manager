import { expect, test } from '@playwright/test'

// One flow against the real binary, in order: the Panel starts with no users.
const username = 'admin'
const password = 'correct horse battery'

test('first-run setup, sign out, sign in, deep link', async ({ page }) => {
  // Any page lands on setup while no user exists.
  await page.goto('/hosts')
  await expect(page).toHaveURL(/\/setup$/)
  await expect(page.getByText('Create the first administrator')).toBeVisible()

  // Client-side validation first.
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByLabel('Confirm password').fill('something else')
  await page.getByRole('button', { name: 'Create admin' }).click()
  await expect(page.getByText('Passwords do not match')).toBeVisible()

  // Create the admin and land in the shell.
  await page.getByLabel('Confirm password').fill(password)
  await page.getByRole('button', { name: 'Create admin' }).click()
  await expect(page).toHaveURL(/\/hosts$/)
  await expect(page.getByTestId('user-name')).toHaveText(username)
  await expect(page.getByRole('navigation', { name: 'Main' }).getByRole('link')).toHaveText([
    'Proxy Hosts',
  ])

  // A deep link reload goes through the SPA fallback and the session survives.
  await page.reload()
  await expect(page).toHaveURL(/\/hosts$/)
  await expect(page.getByTestId('user-name')).toHaveText(username)

  // Setup is no longer reachable.
  await page.goto('/setup')
  await expect(page).toHaveURL(/\/hosts$/)

  // Sign out (a bodiless POST: the server answers 415 without the JSON content type).
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/login$/)

  // The session is really gone: the deep link now asks for a login and remembers the route.
  await page.goto('/hosts')
  await expect(page).toHaveURL(/\/login\?redirect=\/hosts$/)

  // A wrong password is reported on the form.
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill('not the password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toHaveText('invalid username or password')

  // Sign in again and return to the intended page.
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/hosts$/)
  await expect(page.getByTestId('user-name')).toHaveText(username)
})

test('the theme override applies and survives a reload', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/hosts$/)

  const html = page.locator('html')
  await page.getByRole('button', { name: 'Dark theme' }).click()
  await expect(html).toHaveClass(/dark/)
  await page.reload()
  await expect(html).toHaveClass(/dark/)
  await page.getByRole('button', { name: 'Light theme' }).click()
  await expect(html).not.toHaveClass(/dark/)
})
