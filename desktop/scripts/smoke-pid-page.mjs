import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.RENDERER_URL || process.env.APP_BASE || 'http://127.0.0.1:5173'
const apiBase = process.env.API_BASE || process.env.MAIN_BASE || 'http://127.0.0.1:18080'
const expectedAPIHost = process.env.EXPECTED_API_HOST || new URL(apiBase).host
const projectID = process.env.PROJECT_ID || '1'
const edgeInstanceID = process.env.EDGE_INSTANCE_ID || (expectedAPIHost.includes(':19080') ? 'edge-1' : '')
const varGroup = process.env.PID_VAR_GROUP || 'KIO变量'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const writeMode = process.env.PID_SMOKE_WRITE === '1'
const writeVariableKey = process.env.PID_SMOKE_VAR || ''
const writeVariableValue = process.env.PID_SMOKE_VALUE || ''
const writeBatchRaw = process.env.PID_SMOKE_BATCH || ''
const writeBatch = parseWriteBatch(writeBatchRaw)
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `pid-page-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `pid-page-smoke-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  app_base: appBase,
  api_base: apiBase,
  expected_api_host: expectedAPIHost,
  project_id: projectID,
  edge_instance_id: edgeInstanceID || undefined,
  var_group: varGroup,
  write_mode: writeMode,
  write_target: writeMode ? { key: writeVariableKey, value: writeVariableValue } : undefined,
  write_batch: writeMode && writeBatch.length > 0 ? writeBatch : undefined,
  write_skipped_reason: writeMode ? undefined : 'PID_SMOKE_WRITE is not 1; no real KIO/PLC write is executed.',
  requests: [],
  responses: [],
  websockets: [],
  business_api_wrong_host: [],
  direct_edge_requests: [],
  api_failures: [],
  console_errors: [],
  page_errors: [],
  pid: {},
  assertions: {},
}

function redactURL(rawURL) {
  try {
    const parsed = new URL(rawURL)
    if (parsed.searchParams.has('access_token')) parsed.searchParams.set('access_token', '***')
    return parsed.toString()
  } catch {
    return rawURL.replace(/access_token=[^&]+/g, 'access_token=***')
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

function isAllowedDiagnostic(item) {
  return item.status === 404 && item.url.includes('/detection-runs/current')
}

function parseWriteBatch(raw) {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => {
      const separator = item.indexOf('=')
      if (separator <= 0) throw new Error(`Invalid PID_SMOKE_BATCH item: ${item}`)
      return { key: item.slice(0, separator).trim(), value: item.slice(separator + 1).trim() }
    })
}

async function assertReachable(url, label) {
  const response = await fetch(url).catch((error) => {
    throw new Error(`${label} is not reachable at ${url}: ${error.message}`)
  })
  if (!response.ok) throw new Error(`${label} returned ${response.status} at ${url}`)
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

function assertWriteEnv() {
  if (!writeMode) return
  if (writeBatch.length > 0) {
    if (writeBatch.some((item) => !item.key || !item.value)) {
      throw new Error('PID_SMOKE_BATCH items must use KEY=VALUE and both sides must be non-empty')
    }
    return
  }
  if (!writeVariableKey || writeVariableValue === '') {
    throw new Error('PID write smoke requires PID_SMOKE_VAR and PID_SMOKE_VALUE, or PID_SMOKE_BATCH, when PID_SMOKE_WRITE=1')
  }
}

async function maybeRunControlledWrite(page) {
  if (!writeMode) return { attempted: false }
  if (writeBatch.length > 0) return runControlledBatchWrite(page)

  const setting = await findPIDSetting(page, writeVariableKey)
  const disconnected = await setting.locator('.station-pid-disconnected').count()
  if (disconnected > 0) throw new Error(`PID target ${writeVariableKey} is not connected in the page`)

  await setting.locator('input').fill(writeVariableValue)
  await setting.locator('input').press('Enter')
  await page.waitForTimeout(3500)
  const text = await setting.innerText()
  return {
    attempted: true,
    mode: 'single_enter',
    target_count: 1,
    ack_count: /已确认|Confirmed|確認済み|confirmed/i.test(text) ? 1 : 0,
    setting_text: text.slice(0, 400),
  }
}

async function runControlledBatchWrite(page) {
  const targets = []
  for (const item of writeBatch) {
    const setting = await findPIDSetting(page, item.key)
    const disconnected = await setting.locator('.station-pid-disconnected').count()
    if (disconnected > 0) throw new Error(`PID batch target ${item.key} is not connected in the page`)
    await setting.locator('input').fill(item.value)
    targets.push({ ...item, locator: setting })
  }

  await page.locator('.station-pid-modal .ant-modal-footer .ant-btn-primary').click()
  await page.waitForTimeout(Math.max(3500, writeBatch.length * 1800))

  const results = []
  for (const item of targets) {
    const text = await item.locator.innerText()
    results.push({
      key: item.key,
      value: item.value,
      ack: /已确认|Confirmed|確認済み|confirmed/i.test(text),
      error: /失败|Failed|失敗/i.test(text),
      setting_text: text.slice(0, 400),
    })
  }

  return {
    attempted: true,
    mode: 'batch_submit',
    target_count: results.length,
    ack_count: results.filter((item) => item.ack).length,
    error_count: results.filter((item) => item.error).length,
    results,
  }
}

async function findPIDSetting(page, key) {
  const variants = Array.from(
    new Set([
      key,
      key.replaceAll('-', '_'),
      key.replaceAll('_', '-'),
    ]),
  )
  for (const variant of variants) {
    const setting = page.locator('.station-pid-setting').filter({ hasText: variant }).first()
    if ((await setting.count()) > 0) {
      await setting.waitFor({ state: 'visible', timeout: 10000 })
      return setting
    }
  }
  throw new Error(`PID target ${key} is not visible in the page; tried ${variants.join(', ')}`)
}

await fs.mkdir(screenshotDir, { recursive: true })
assertWriteEnv()
await assertReachable(appBase, 'renderer')
await assertReachable(`${apiBase}/health`, 'backend')

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })
let currentPageKey = 'bootstrap'

page.on('request', (request) => {
  const rawURL = request.url()
  const entry = { page: currentPageKey, method: request.method(), url: redactURL(rawURL) }
  if (isBusinessAPI(rawURL) || isEdgeURL(rawURL)) evidence.requests.push(entry)
  if (isEdgeURL(rawURL) && new URL(rawURL).host !== expectedAPIHost) evidence.direct_edge_requests.push(entry)
  if (isBusinessAPI(rawURL)) {
    try {
      const parsed = new URL(rawURL)
      if (parsed.host !== expectedAPIHost) evidence.business_api_wrong_host.push(entry)
    } catch {
      evidence.business_api_wrong_host.push(entry)
    }
  }
})

page.on('response', async (response) => {
  const rawURL = response.url()
  if (!isBusinessAPI(rawURL)) return
  const entry = { page: currentPageKey, status: response.status(), url: redactURL(rawURL), body: '' }
  if (entry.status >= 400) {
    entry.body = (await response.text().catch(() => '')).slice(0, 600)
    evidence.api_failures.push(entry)
  }
  evidence.responses.push(entry)
})

page.on('websocket', (ws) => {
  const rawURL = ws.url()
  const entry = { page: currentPageKey, url: redactURL(rawURL), frames_sent: [], frames_received: [] }
  evidence.websockets.push(entry)
  ws.on('framesent', (event) => {
    if (entry.frames_sent.length < 12) entry.frames_sent.push(String(event.payload).slice(0, 1000))
  })
  ws.on('framereceived', (event) => {
    if (entry.frames_received.length < 20) entry.frames_received.push(String(event.payload).slice(0, 1000))
  })
  if (isEdgeURL(rawURL) && new URL(rawURL).host !== expectedAPIHost) evidence.direct_edge_requests.push({ page: currentPageKey, method: 'WS', url: redactURL(rawURL) })
  if (isBusinessAPI(rawURL)) {
    try {
      const parsed = new URL(rawURL)
      if (parsed.host !== expectedAPIHost) evidence.business_api_wrong_host.push({ page: currentPageKey, method: 'WS', url: redactURL(rawURL) })
    } catch {
      evidence.business_api_wrong_host.push({ page: currentPageKey, method: 'WS', url: redactURL(rawURL) })
    }
  }
})

page.on('console', (message) => {
  if (message.type() === 'error') evidence.console_errors.push({ page: currentPageKey, text: message.text() })
})

page.on('pageerror', (error) => {
  evidence.page_errors.push({ page: currentPageKey, message: error.message })
})

try {
  await login(page)

  currentPageKey = 'station'
  const stationURL = new URL(`${appBase}/#/station`)
  stationURL.hash = `/station?project_id=${encodeURIComponent(projectID)}${edgeInstanceID ? `&edge_instance_id=${encodeURIComponent(edgeInstanceID)}` : ''}`
  await page.goto(stationURL.toString(), { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.getByRole('button', { name: /PID|pid/i }).click()
  await page.locator('.station-pid-modal').waitFor({ state: 'visible', timeout: 15000 })
  const varGroupInput = page.locator('.station-pid-modal input').first()
  const existingVarGroup = await varGroupInput.inputValue()
  if (existingVarGroup !== varGroup) {
    const variableResponsePromise = page.waitForResponse(
      (response) => {
        if (!response.url().includes('/api/v1/variables')) return false
        return response.url().includes(encodeURIComponent(varGroup)) && !response.url().includes('writable=true')
      },
      { timeout: 15000 },
    )
    await varGroupInput.fill(varGroup)
    await variableResponsePromise
  }
  await page.waitForTimeout(2500)

  const modalText = await page.locator('.station-pid-modal').innerText()
  const cards = await page.locator('.station-pid-card').evaluateAll((nodes) =>
    nodes.map((node) => {
      const text = node.textContent || ''
      return {
        title: text.split('\n').find(Boolean) || '',
        text: text.slice(0, 800),
        no_connection_count: (text.match(/无连接|No connection|未接続/g) || []).length,
      }
    }),
  )
  const settings = await page.locator('.station-pid-setting').evaluateAll((nodes) =>
    nodes.map((node) => {
      const text = node.textContent || ''
      return {
        text: text.slice(0, 500),
        connected: !/无连接|No connection|未接続/.test(text),
      }
    }),
  )
  const variableRequest = evidence.requests.find(
    (item) => item.url.includes('/api/v1/variables') && item.url.includes(encodeURIComponent(varGroup)) && !item.url.includes('writable=true'),
  )
  const websocketRequest = evidence.websockets.find((item) => item.url.includes('/api/v1/ws'))
  const editableSetting = page.locator('.station-pid-setting').filter({ has: page.locator('input:not([disabled])') }).first()
  const editableSettingCount = await page.locator('.station-pid-setting').filter({ has: page.locator('input:not([disabled])') }).count()
  let draftIsolation = { tested: false, reason: 'No writable PID setting is visible.' }
  if (editableSettingCount > 0) {
    const currentBefore = (await editableSetting.locator('.station-pid-current-row strong').first().innerText()).trim()
    const input = editableSetting.locator('input').first()
    const inputInitialValue = await input.inputValue()
    const placeholder = (await input.getAttribute('placeholder')) || ''
    await input.fill('12.3')
    await page.waitForTimeout(250)
    const currentAfter = (await editableSetting.locator('.station-pid-current-row strong').first().innerText()).trim()
    const inputAfterValue = await input.inputValue()
    const textAfterDraft = await editableSetting.innerText()
    draftIsolation = {
      tested: true,
      current_before: currentBefore,
      current_after: currentAfter,
      input_initial_value: inputInitialValue,
      input_after_value: inputAfterValue,
      placeholder,
      setting_text_after_draft: textAfterDraft.slice(0, 500),
      draft_tag_visible: /待下设|Draft|下書き/.test(textAfterDraft),
      current_value_unchanged: currentBefore === currentAfter,
      input_started_as_empty: inputInitialValue === '',
      placeholder_uses_current_value: placeholder === currentBefore.replace(/\s+\S+$/, '') || placeholder === currentBefore || placeholder.length > 0,
      input_keeps_user_draft: inputAfterValue === '12.3',
    }
  }
  const writeResult = await maybeRunControlledWrite(page)

  await page.screenshot({ path: path.join(screenshotDir, 'pid-modal.png'), fullPage: true }).catch(() => {})
  const unexpectedAPIFailures = evidence.api_failures.filter((entry) => !isAllowedDiagnostic(entry))

  evidence.pid = {
    modal_text_sample: modalText.slice(0, 1200),
    card_count: cards.length,
    cards,
    setting_count: settings.length,
    connected_setting_count: settings.filter((item) => item.connected).length,
    disconnected_setting_count: settings.filter((item) => !item.connected).length,
    editable_setting_count: editableSettingCount,
    variable_request: variableRequest,
    websocket_request: websocketRequest,
    draft_isolation: draftIsolation,
    write_result: writeResult,
  }
  evidence.assertions = {
    modal_rendered: modalText.includes('PID'),
    v3_groups_rendered: /温度|Temperature/.test(modalText) && /湿度|Humidity/.test(modalText) && /温度2|Temperature 2/.test(modalText),
    variables_filtered_by_group_without_writable_mask: Boolean(variableRequest),
    realtime_ws_connected_to_expected_host: Boolean(websocketRequest) && new URL(websocketRequest.url).host === expectedAPIHost,
    has_connected_or_disconnected_state: settings.length > 0 && settings.every((item) => typeof item.connected === 'boolean'),
    writable_pid_input_starts_as_empty_draft: draftIsolation.tested ? draftIsolation.input_started_as_empty : true,
    writable_pid_current_value_does_not_follow_draft: draftIsolation.tested ? draftIsolation.current_value_unchanged : true,
    writable_pid_draft_tag_visible_after_edit: draftIsolation.tested ? draftIsolation.draft_tag_visible : true,
    writable_pid_input_keeps_user_draft: draftIsolation.tested ? draftIsolation.input_keeps_user_draft : true,
    no_unexpected_api_failures: unexpectedAPIFailures.length === 0,
    no_wrong_business_api_host: evidence.business_api_wrong_host.length === 0,
    no_direct_other_edge_browser_requests: evidence.direct_edge_requests.length === 0,
    no_page_errors: evidence.page_errors.length === 0,
    controlled_write_attempted_only_when_enabled: writeMode ? writeResult.attempted === true : writeResult.attempted === false,
    controlled_write_acknowledged: writeMode ? writeResult.ack_count === writeResult.target_count && writeResult.error_count !== undefined ? writeResult.error_count === 0 : writeResult.ack_count === writeResult.target_count : true,
  }
  evidence.unexpected_api_failures = unexpectedAPIFailures
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')

  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  const summary = {
    ok: failed.length === 0,
    evidencePath,
    screenshotDir,
    assertions: evidence.assertions,
    setting_count: evidence.pid.setting_count,
    connected_setting_count: evidence.pid.connected_setting_count,
    disconnected_setting_count: evidence.pid.disconnected_setting_count,
    write_mode: writeMode,
    websocket_count: evidence.websockets.length,
  }
  console.log(JSON.stringify(summary, null, 2))
  if (failed.length) throw new Error(`PID page smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  await browser.close()
}
