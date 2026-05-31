const rendererUrl = process.env.RENDERER_URL || 'http://127.0.0.1:5173'
const backendUrl = process.env.BACKEND_URL || 'http://127.0.0.1:18080'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'

async function apiFetch(path, options = {}, token = '') {
  const response = await fetch(`${backendUrl}${path}`, {
    ...options,
    headers: {
      ...(options.headers || {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  })
  const text = await response.text()
  let body
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = text
    }
  }
  return { response, body }
}

function assertOk(condition, message) {
  if (!condition) {
    throw new Error(message)
  }
}

async function smokeBackend() {
  const health = await apiFetch('/health')
  assertOk(health.response.ok, `health failed: ${health.response.status}`)
  assertOk(health.body?.status === 'ok', `unexpected health body: ${JSON.stringify(health.body)}`)

  const login = await apiFetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  assertOk(login.response.ok, `login failed: ${login.response.status} ${JSON.stringify(login.body)}`)
  const token = login.body?.access_token
  assertOk(token, 'login response missing access_token')

  const projects = await apiFetch('/api/v1/projects', {}, token)
  assertOk(projects.response.ok, `projects failed: ${projects.response.status}`)
  assertOk(Array.isArray(projects.body), 'projects response is not an array')

  const legacyDevices = await apiFetch('/api/v1/devices', {}, token)
  assertOk(legacyDevices.response.status === 404, `legacy devices endpoint should be 404, got ${legacyDevices.response.status}`)

  if (projects.body.length > 0) {
    const members = await apiFetch(`/api/v1/projects/${projects.body[0].id}/members`, {}, token)
    assertOk(members.response.ok, `project members failed: ${members.response.status}`)
    assertOk(Array.isArray(members.body?.items), 'project members response missing items array')
  }

  return { projectCount: projects.body.length, health: health.body }
}

async function clickSummaryButton(page, text) {
  await page.locator('button').filter({ hasText: text }).last().click()
}

async function smokeSettingsUI() {
  const { chromium } = await import('playwright')
  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage()
  try {
    await page.goto(`${rendererUrl}/#/login`, { waitUntil: 'domcontentloaded' })
    await page.locator('input').nth(0).fill(username)
    await page.locator('input').nth(1).fill(password)
    await page.getByRole('button', { name: /登录|Login|ログイン/ }).click()
    await page.waitForTimeout(1500)

    await page.goto(`${rendererUrl}/#/settings`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(1500)

    await clickSummaryButton(page, '用户管理')
    await page.getByRole('heading', { name: '项目成员' }).waitFor({ timeout: 10000 })
    await page.getByText('通知接收人').first().waitFor({ timeout: 10000 })
    await page.getByRole('columnheader', { name: '接收通知' }).waitFor({ timeout: 10000 })

    await clickSummaryButton(page, '系统设置')
    await page.getByRole('button', { name: '运行日志预览' }).first().click()
    await page.locator('.ant-modal').filter({ hasText: '运行日志预览' }).waitFor({ timeout: 10000 })
    const openDrawers = await page.locator('.ant-drawer-open').count()
    assertOk(openDrawers === 0, `expected log preview modal, found ${openDrawers} open drawer(s)`)

    return true
  } finally {
    await browser.close()
  }
}

async function main() {
  const backend = await smokeBackend()
  await smokeSettingsUI()
  console.log(`settings smoke passed: projects=${backend.projectCount}, tags=${backend.health.tags ?? 0}`)
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
