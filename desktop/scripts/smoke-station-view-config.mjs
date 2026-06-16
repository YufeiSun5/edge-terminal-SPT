import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.APP_BASE || process.env.RENDERER_URL || 'http://127.0.0.1:5173'
const edgeBase = process.env.EDGE_BASE || 'http://127.0.0.1:18080'
const projectCode = process.env.STATION_SMOKE_PROJECT_CODE || 'AC-01'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `station-view-config-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `station-view-config-smoke-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  scope: 'settings station-view item configuration smoke with layout_area card_pool/list_layout and station readback',
  app_base: appBase,
  edge_base: edgeBase,
  browser_path: 'Playwright',
  browser_plugin: 'not available in this Codex session',
  setup: {},
  requests: [],
  responses: [],
  console_warnings: [],
  console_errors: [],
  page_errors: [],
  assertions: {},
}

function assertOk(condition, message) {
  if (!condition) throw new Error(message)
}

async function api(pathName, options = {}, token = '', label = pathName) {
  const response = await fetch(new URL(pathName, edgeBase), {
    ...options,
    headers: {
      ...(options.headers || {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  })
  const text = await response.text()
  let body = undefined
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = text
    }
  }
  if (!response.ok) {
    throw new Error(`${label} failed: ${response.status} ${typeof body === 'string' ? body : JSON.stringify(body)}`)
  }
  return body
}

async function loginAPI() {
  const body = await api(
    '/api/v1/auth/login',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    },
    '',
    'login edge backend',
  )
  assertOk(body?.access_token, 'edge login response missing access_token')
  return body.access_token
}

async function loadFixture(token) {
  const projects = await api('/api/v1/projects', {}, token, 'list projects')
  const project = projects.find((item) => item.project_code === projectCode)
  assertOk(project, `project ${projectCode} not found`)
  const variables = await api(`/api/v1/variables?project_id=${project.id}`, {}, token, `list variables for ${projectCode}`)
  const usableVariables = variables.filter((variable) => variable.var_name && (variable.display_name || variable.raw_name || variable.var_name)).slice(0, 2)
  assertOk(usableVariables.length >= 2, `project ${projectCode} needs at least 2 variables for station-view smoke`)
  return { project, variables: usableVariables }
}

async function loginPage(page) {
  await page.goto(`${appBase}/#/login`, { waitUntil: 'domcontentloaded' })
  if (!page.url().includes('/login')) return
  await page.getByLabel('账号', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  const response = page.waitForResponse((item) => item.url().includes('/api/v1/auth/login') && item.status() === 200, { timeout: 15000 })
  await page.locator('button[type="submit"]').click()
  await response
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

async function selectStationVariable(page, modal, sectionIndex, variable) {
  const section = modal.locator('.settings-station-view-card').nth(sectionIndex)
  const select = section.locator('.ant-select')
  const clear = select.locator('.ant-select-clear')
  if (await clear.count()) {
    await clear.click({ force: true }).catch(() => {})
  }
  await select.click()
  const input = select.locator('input').first()
  await input.fill(variable.var_name)
  const dropdown = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').last()
  const option = dropdown.locator('.ant-select-item-option').filter({ hasText: variable.var_name }).first()
  await option.waitFor({ state: 'visible', timeout: 5000 })
  await option.click()
  await page.keyboard.press('Escape').catch(() => {})
  await section.locator('.ant-select-selection-item').filter({ hasText: variable.var_name }).first().waitFor({ state: 'visible', timeout: 5000 })
}

async function clickSettingsModule(page, name) {
  const button = page.locator('.settings-summary button').filter({ hasText: name }).first()
  await button.waitFor({ state: 'visible', timeout: 10000 })
  await button.click()
}

async function selectStationProject(page, project) {
  const toolbar = page.locator('.settings-station-view-toolbar')
  await toolbar.waitFor({ state: 'visible', timeout: 10000 })
  const select = toolbar.locator('.ant-select').first()
  await select.click()
  const input = select.locator('input').first()
  await input.fill(project.project_code)
  const dropdown = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').last()
  const option = dropdown.locator('.ant-select-item-option').filter({ hasText: project.project_code }).first()
  await option.waitFor({ state: 'visible', timeout: 5000 })
  await option.click()
  await toolbar.locator('strong').filter({ hasText: project.project_code }).waitFor({ state: 'visible', timeout: 10000 })
}

await fs.mkdir(screenshotDir, { recursive: true })

const token = await loginAPI()
const fixture = await loadFixture(token)
let originalStationItems = undefined
let stationItemsSaved = false
evidence.setup = {
  project: { id: fixture.project.id, project_code: fixture.project.project_code },
  variables: fixture.variables.map((variable) => ({
    var_id: variable.var_id_text ?? variable.var_id,
    var_name: variable.var_name,
    display_name: variable.display_name,
  })),
}

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })

page.on('request', (request) => {
  if (request.url().includes('/api/v1/')) evidence.requests.push({ method: request.method(), url: request.url() })
})
page.on('response', async (response) => {
  if (!response.url().includes('/api/v1/')) return
  evidence.responses.push({
    status: response.status(),
    url: response.url(),
    body: response.status() >= 400 ? (await response.text().catch(() => '')).slice(0, 400) : '',
  })
})
page.on('console', (message) => {
  if (message.type() === 'warning') evidence.console_warnings.push(message.text())
  if (message.type() === 'error') evidence.console_errors.push(message.text())
})
page.on('pageerror', (error) => evidence.page_errors.push(error.message))

try {
  await loginPage(page)
  await page.goto(`${appBase}/#/settings`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.waitForTimeout(1200)
  await clickSettingsModule(page, '工位显示配置')
  const configPanel = page.locator('.settings-station-view-module')
  await configPanel.waitFor({ state: 'visible', timeout: 10000 })
  await selectStationProject(page, fixture.project)
  originalStationItems = await api(`/api/v1/station-view/items?project_id=${fixture.project.id}`, {}, token, 'get original station view items')
  await page.screenshot({ path: path.join(screenshotDir, 'settings-station-config-before.png'), fullPage: true }).catch(() => {})
  await selectStationVariable(page, configPanel, 0, fixture.variables[0])
  await selectStationVariable(page, configPanel, 1, fixture.variables[1])
  await page.screenshot({ path: path.join(screenshotDir, 'settings-station-config-selected.png'), fullPage: true }).catch(() => {})

  const saveResponse = page.waitForResponse((response) => response.url().includes('/api/v1/station-view/items') && response.request().method() === 'PUT', { timeout: 15000 })
  await configPanel.getByRole('button', { name: /保存|Save/ }).click()
  const saved = await saveResponse
  assertOk(saved.status() === 200, `station-view items save returned ${saved.status()}`)
  stationItemsSaved = true

  const items = await api(`/api/v1/station-view/items?project_id=${fixture.project.id}`, {}, token, 'get saved station view items')
  const cardItem = (items.items ?? []).find((item) => item.layout_area === 'card_pool' && item.binding_key === fixture.variables[0].var_name)
  const listItem = (items.items ?? []).find((item) => item.layout_area === 'list_layout' && item.binding_key === fixture.variables[1].var_name)

  await page.goto(`${appBase}/#/station?project_id=${fixture.project.id}`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.waitForTimeout(1200)
  await page.screenshot({ path: path.join(screenshotDir, 'station-after-config.png'), fullPage: true }).catch(() => {})
  await page.getByText(fixture.variables[0].display_name).first().waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText(fixture.variables[1].display_name).first().waitFor({ state: 'visible', timeout: 10000 })

  const pidButton = page.getByRole('button', { name: /PID/ }).first()
  assertOk((await pidButton.count()) === 1, 'PID action button not found')
  assertOk(!(await pidButton.isDisabled()), 'PID action button is disabled')
  await pidButton.click()
  const pidModal = page.locator('.station-pid-modal')
  await pidModal.waitFor({ state: 'visible', timeout: 10000 })
  const pidTitle = await pidModal.locator('.ant-modal-title').innerText()
  assertOk(pidTitle.includes('PID'), `unexpected PID modal title: ${pidTitle}`)
  await page.screenshot({ path: path.join(screenshotDir, 'station-pid-modal.png'), fullPage: true }).catch(() => {})

  evidence.assertions = {
    settings_station_config_module_visible: true,
    station_view_items_save_200: true,
    saved_card_pool_item: Boolean(cardItem),
    saved_list_layout_item: Boolean(listItem),
    station_page_reads_saved_card_item: true,
    station_page_reads_saved_list_item: true,
    pid_button_visible_and_enabled: true,
    pid_modal_visible: true,
    no_page_errors: evidence.page_errors.length === 0,
  }
  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  evidence.saved_items = items.items
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')
  console.log(
    JSON.stringify(
      {
        ok: failed.length === 0,
        evidencePath,
        screenshotDir,
        assertions: evidence.assertions,
        project_id: fixture.project.id,
      },
      null,
      2,
    ),
  )
  if (failed.length) throw new Error(`station view config smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  if (stationItemsSaved && originalStationItems?.template_uid) {
    await api(
      '/api/v1/station-view/items',
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template_uid: originalStationItems.template_uid, items: originalStationItems.items ?? [] }),
      },
      token,
      'restore original station view items',
    ).catch((error) => {
      evidence.restore_error = error?.message || String(error)
    })
  }
  await browser.close()
}
