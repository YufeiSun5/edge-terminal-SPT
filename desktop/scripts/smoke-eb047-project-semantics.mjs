import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.RENDERER_URL || process.env.APP_BASE || 'http://127.0.0.1:5173'
const mainBase = process.env.MAIN_BASE || 'http://127.0.0.1:19080'
const mainHost = new URL(mainBase).host
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `eb047-project-semantics-${stamp}.json`)
const screenshotDir = path.join(outDir, `eb047-project-semantics-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  app_base: appBase,
  main_base: mainBase,
  browser_requests: [],
  browser_responses: [],
  browser_websockets: [],
  direct_edge_browser_requests: [],
  api_non_main: [],
  api_failures: [],
  console_errors: [],
  page_errors: [],
  variables_page: {},
  tasks_page: {},
  assertions: {},
}

function assertOk(condition, message, detail) {
  if (!condition) {
    const suffix = detail === undefined ? '' : `: ${JSON.stringify(detail)}`
    throw new Error(`${message}${suffix}`)
  }
}

function isBusinessAPI(rawURL) {
  return rawURL.includes('/api/v1/')
}

function isEdgeURL(rawURL) {
  return (
    rawURL.includes('127.0.0.1:18080') ||
    rawURL.includes('127.0.0.1:18081') ||
    rawURL.includes('localhost:18080') ||
    rawURL.includes('localhost:18081')
  )
}

function hostOf(rawURL) {
  try {
    return new URL(rawURL).host
  } catch {
    return ''
  }
}

function containsLegacyDeviceAPI(rawURL) {
  return rawURL.includes('/api/v1/devices') || rawURL.includes('device_id=') || rawURL.includes('device_code=')
}

function parsePreview(text) {
  try {
    return JSON.parse(text)
  } catch (error) {
    throw new Error(`task request preview is not valid JSON: ${error.message}; text=${text.slice(0, 500)}`)
  }
}

async function assertReachable(url, label) {
  const response = await fetch(url).catch((error) => {
    throw new Error(`${label} is not reachable at ${url}: ${error.message}`)
  })
  assertOk(response.ok, `${label} returned ${response.status} at ${url}`)
}

async function login(page) {
  await page.goto(`${appBase}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input').nth(0).fill(username)
  await page.locator('input').nth(1).fill(password)
  const loginResponse = page.waitForResponse(
    (response) => response.url().includes('/api/v1/auth/login') && response.status() === 200,
    { timeout: 15000 },
  )
  await page.locator('button[type="submit"]').click()
  await loginResponse
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

async function selectFirstReportVariable(page) {
  const reportRow = page.locator('.task-flow-request-row.report').first()
  await reportRow.waitFor({ timeout: 10000 })
  const variableSelect = reportRow.locator('.ant-select').nth(1)
  await variableSelect.click()
  await page.waitForTimeout(300)
  const option = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option:not(.ant-select-item-option-disabled)').first()
  await option.waitFor({ timeout: 10000 })
  const optionText = (await option.innerText()).trim()
  await option.click()
  return optionText
}

await fs.mkdir(screenshotDir, { recursive: true })
await assertReachable(appBase, 'renderer')
await assertReachable(`${mainBase}/health`, 'main server')

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })
let currentPage = 'bootstrap'

page.on('request', (request) => {
  const rawURL = request.url()
  if (!isBusinessAPI(rawURL) && !isEdgeURL(rawURL)) return
  const entry = { page: currentPage, method: request.method(), url: rawURL }
  evidence.browser_requests.push(entry)
  if (isEdgeURL(rawURL)) evidence.direct_edge_browser_requests.push(entry)
  if (isBusinessAPI(rawURL) && hostOf(rawURL) !== mainHost) evidence.api_non_main.push(entry)
})

page.on('response', async (response) => {
  const rawURL = response.url()
  if (!isBusinessAPI(rawURL)) return
  const entry = { page: currentPage, status: response.status(), url: rawURL, body: '' }
  if (entry.status >= 400) {
    entry.body = (await response.text().catch(() => '')).slice(0, 600)
    evidence.api_failures.push(entry)
  }
  evidence.browser_responses.push(entry)
})

page.on('websocket', (ws) => {
  const rawURL = ws.url()
  const entry = { page: currentPage, url: rawURL }
  evidence.browser_websockets.push(entry)
  if (isEdgeURL(rawURL)) evidence.direct_edge_browser_requests.push({ page: currentPage, method: 'WS', url: rawURL })
  if (isBusinessAPI(rawURL) && hostOf(rawURL) !== mainHost) evidence.api_non_main.push({ page: currentPage, method: 'WS', url: rawURL })
})

page.on('console', (message) => {
  if (message.type() === 'error') evidence.console_errors.push({ page: currentPage, text: message.text() })
})

page.on('pageerror', (error) => {
  evidence.page_errors.push({ page: currentPage, message: error.message })
})

try {
  await login(page)

  currentPage = 'variables'
  await page.goto(`${appBase}/#/variables`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.waitForTimeout(1200)
  const variablesText = await page.locator('body').innerText()
  const variablesHeadingsAndButtons = await page.locator('h1,h2,h3,button,.ant-select-selection-item,.ant-select-selection-placeholder').evaluateAll((items) =>
    items.map((item) => item.textContent?.trim() ?? '').filter(Boolean),
  )
  await page.screenshot({ path: path.join(screenshotDir, 'variables.png'), fullPage: true }).catch(() => {})
  await page.getByRole('button', { name: /创建虚变量|Create virtual variable|仮想変数を作成/ }).click()
  const virtualModal = page.locator('.ant-modal').filter({ hasText: /创建虚变量|Create virtual variable|仮想変数を作成/ })
  await virtualModal.waitFor({ timeout: 10000 })
  const virtualModalText = await virtualModal.innerText()
  await page.screenshot({ path: path.join(screenshotDir, 'variables-create-virtual-modal.png'), fullPage: true }).catch(() => {})
  await page.keyboard.press('Escape')
  await virtualModal.waitFor({ state: 'hidden', timeout: 10000 }).catch(() => {})
  evidence.variables_page = {
    contains_project_copy: variablesText.includes('项目') || variablesText.includes('Project') || variablesText.includes('プロジェクト'),
    key_ui_text: variablesHeadingsAndButtons,
    key_ui_contains_legacy_device_copy: variablesHeadingsAndButtons.some((text) => text.includes('设备') || text.includes('Device') || text.includes('設備')),
    virtual_modal_contains_select_project: virtualModalText.includes('选择项目') || virtualModalText.includes('Select project') || virtualModalText.includes('プロジェクトを選択'),
    virtual_modal_contains_legacy_device_copy: virtualModalText.includes('设备') || virtualModalText.includes('Device') || virtualModalText.includes('設備'),
  }

  currentPage = 'tasks'
  await page.goto(`${appBase}/#/tasks`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.waitForTimeout(1200)
  const tasksText = await page.locator('body').innerText()
  const tasksHeadingsAndButtons = await page.locator('h1,h2,h3,button,.ant-select-selection-item,.ant-select-selection-placeholder').evaluateAll((items) =>
    items.map((item) => item.textContent?.trim() ?? '').filter(Boolean),
  )
  await page.screenshot({ path: path.join(screenshotDir, 'tasks.png'), fullPage: true }).catch(() => {})
  await page.getByRole('button', { name: /发送任务请求|Send task request|タスク要求を送信/ }).click()
  const requestModal = page.locator('.ant-modal').filter({ hasText: /任务请求|Task request|タスク要求/ })
  await requestModal.waitFor({ timeout: 10000 })
  const selectedReportVariable = await selectFirstReportVariable(page)
  await page.waitForTimeout(500)
  const requestModalText = await requestModal.innerText()
  const previewText = await page.locator('.task-flow-request-preview').inputValue()
  const previewPayload = parsePreview(previewText)
  await page.screenshot({ path: path.join(screenshotDir, 'tasks-request-modal.png'), fullPage: true }).catch(() => {})

  evidence.tasks_page = {
    contains_project_copy: tasksText.includes('项目') || tasksText.includes('Project') || tasksText.includes('プロジェクト'),
    key_ui_text: tasksHeadingsAndButtons,
    key_ui_contains_legacy_device_copy: tasksHeadingsAndButtons.some((text) => text.includes('设备') || text.includes('Device') || text.includes('設備')),
    request_modal_contains_project_copy: requestModalText.includes('项目') || requestModalText.includes('Project') || requestModalText.includes('プロジェクト'),
    request_modal_contains_legacy_device_copy: requestModalText.includes('设备') || requestModalText.includes('Device') || requestModalText.includes('設備'),
    selected_report_variable: selectedReportVariable,
    preview_payload: previewPayload,
    preview_text: previewText,
  }

  const legacyAPIRequests = evidence.browser_requests.filter((item) => containsLegacyDeviceAPI(item.url))
  const unexpectedAPIFailures = evidence.api_failures.filter((item) => item.status >= 500)
  const previewTextHasLegacyDevice = previewText.includes('device_id') || previewText.includes('device_code')
  const reports = previewPayload?.report_request?.reports

  evidence.assertions = {
    variables_page_uses_project_copy: evidence.variables_page.contains_project_copy,
    variable_create_modal_uses_project_copy:
      evidence.variables_page.virtual_modal_contains_select_project && !evidence.variables_page.virtual_modal_contains_legacy_device_copy,
    tasks_page_uses_project_copy: evidence.tasks_page.contains_project_copy,
    task_request_modal_uses_project_copy:
      evidence.tasks_page.request_modal_contains_project_copy && !evidence.tasks_page.request_modal_contains_legacy_device_copy,
    task_request_preview_uses_project_id: Number(previewPayload.project_id) > 0,
    task_request_preview_has_native_report_requests: Array.isArray(reports) && reports.length > 0,
    task_request_preview_has_report_params: Boolean(reports?.[0]?.params && typeof reports[0].params === 'object'),
    task_request_preview_has_no_device_aliases: !previewTextHasLegacyDevice,
    browser_business_api_ws_only_main_server: evidence.api_non_main.length === 0,
    browser_did_not_direct_edge: evidence.direct_edge_browser_requests.length === 0,
    no_legacy_device_api_or_query: legacyAPIRequests.length === 0,
    no_page_errors: evidence.page_errors.length === 0,
    no_unexpected_api_failures: unexpectedAPIFailures.length === 0,
  }
  evidence.legacy_api_requests = legacyAPIRequests
  evidence.unexpected_api_failures = unexpectedAPIFailures
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')

  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  const summary = {
    ok: failed.length === 0,
    evidencePath,
    screenshotDir,
    assertions: evidence.assertions,
    request_count: evidence.browser_requests.length,
    websocket_count: evidence.browser_websockets.length,
  }
  console.log(JSON.stringify(summary, null, 2))
  if (failed.length) throw new Error(`EB-047 project semantics smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  await browser.close()
}
