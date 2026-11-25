import { expect, test } from '@playwright/test'

// Runs after 01-smoke.spec.ts (files run in name order), which created the admin in the shared database.
const username = 'admin'
const password = 'correct horse battery'

test('manage Proxy Hosts end to end', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/hosts$/)
  await expect(page.getByText('No proxy hosts yet')).toBeVisible()

  const sheet = page.getByRole('dialog', { name: /Proxy Host/ })
  const row = (domain: string) => page.getByRole('row').filter({ hasText: domain })
  const addDomain = async (value: string) => {
    const input = sheet.getByLabel('Domain names')
    await input.fill(value)
    await input.press('Enter')
  }

  // Create: the live summary follows the form, Save waits for a change.
  await page.getByRole('button', { name: 'Add Proxy Host' }).click()
  await expect(sheet.getByRole('button', { name: 'Save' })).toBeDisabled()
  await addDomain('App.Example.com')
  await addDomain('www.example.com')
  await sheet.getByLabel('Upstream host').fill('192.168.1.20')
  await sheet.getByLabel('Port').fill('8080')
  await expect(sheet.getByTestId('summary')).toContainText(
    'Requests for app.example.com and www.example.com are forwarded to http://192.168.1.20:8080',
  )
  await expect(sheet.getByText('Unsaved changes')).toBeVisible()
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  await expect(row('app.example.com')).toContainText('+1')
  await expect(row('app.example.com')).toContainText('http://192.168.1.20:8080')
  await expect(row('app.example.com')).toContainText('active')

  // Edit: change the port and disable the host; the table follows without a reload.
  await row('app.example.com').getByRole('button', { name: 'Edit' }).click()
  await expect(sheet.getByLabel('Port')).toHaveValue('8080')
  await sheet.getByLabel('Port').fill('9090')
  await sheet.getByLabel('Enabled').click()
  await expect(sheet.getByTestId('summary')).toContainText('This host is disabled')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  await expect(row('app.example.com')).toContainText('http://192.168.1.20:9090')
  await expect(row('app.example.com')).toContainText('disabled')

  // The change was stored, not only shown: it survives a reload.
  await page.reload()
  await expect(row('app.example.com')).toContainText('disabled')

  // Search filters on any domain.
  await page.getByLabel('Search domains').fill('www.')
  await expect(row('app.example.com')).toBeVisible()
  await page.getByLabel('Search domains').fill('nothing-like-this')
  await expect(page.getByText('No hosts match')).toBeVisible()
  await page.getByLabel('Search domains').fill('')

  // A taken domain is refused by the server and shown on the form, input kept.
  await page.getByRole('button', { name: 'Add Proxy Host' }).click()
  await addDomain('www.example.com')
  await sheet.getByLabel('Upstream host').fill('10.0.0.5')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet.getByText('already used by another host')).toBeVisible()
  await expect(sheet.getByText('www.example.com', { exact: true })).toBeVisible()
  await expect(sheet.getByLabel('Upstream host')).toHaveValue('10.0.0.5')

  // Esc with unsaved input asks first; keep editing, then fix the domain and save.
  await page.keyboard.press('Escape')
  await expect(page.getByRole('alertdialog', { name: 'Discard your changes?' })).toBeVisible()
  await page.getByRole('button', { name: 'Keep editing' }).click()
  await expect(sheet).toBeVisible()
  await sheet.getByRole('button', { name: 'Remove www.example.com' }).click()
  await addDomain('second.example.com')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  await expect(row('second.example.com')).toContainText('http://10.0.0.5:80')

  // Delete asks for confirmation, then the row goes.
  await row('second.example.com').getByRole('button', { name: 'Edit' }).click()
  await sheet.getByRole('button', { name: 'Delete' }).click()
  const confirm = page.getByRole('alertdialog', { name: 'Delete this proxy host?' })
  await confirm.getByRole('button', { name: 'Keep host' }).click()
  await expect(sheet).toBeVisible()
  await sheet.getByRole('button', { name: 'Delete' }).click()
  await confirm.getByRole('button', { name: 'Delete host' }).click()
  await expect(sheet).toBeHidden()
  await expect(row('second.example.com')).toHaveCount(0)
  await expect(row('app.example.com')).toBeVisible()
})
