import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.APP_BASE || process.env.RENDERER_URL || 'http://127.0.0.1:5173'
const mainBase = process.env.MAIN_BASE || process.env.API_BASE || 'http://127.0.0.1:19080'
const edgeInstanceID = process.env.EDGE_INSTANCE_ID || 'edge-1'
const username = process.env.SMOKE_USERNAME || process.env.SMOKE_USER || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const testNo = process.env.TEST_NO || `EB050-PAGE-${stamp}`
const evidencePath = path.join(outDir, `eb050-notification-page-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `eb050-notification-page-smoke-${stamp}`)

const mainHost = new URL(mainBase).host
const evidence = {
  started_at: new Date().toISOString(),
  app_base: appBase,
  main_base: mainBase,
  edge_instance_id: edgeInstanceID,
  test_no: testNo,
  browser_requests: [],
  browser_responses: [],
  browser_websockets: [],
  browser_ws_notifications: [],
  direct_edge_browser_requests: [],
  api_non_main: [],
  api_failures: [],
  console_errors: [],
  console_warnings: [],
  page_errors: [],
  assertions: {},
}

function assertOk(condition, message, details) {
  if (!condition) {
    const suffix = details === undefined ? '' : `\n${JSON.stringify(details, null, 2)}`
    throw new Error(`${message}${suffix}`)
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function isEdgeURL(rawURL) {
  return rawURL.includes('127.0.0.1:18080') || rawURL.includes('127.0.0.1:18081') || rawURL.includes('localhost:18080') || rawURL.includes('localhost:18081')
}

function isBusinessAPI(rawURL) {
  return rawURL.includes('/api/v1/')
}

async function request(method, pathName, token, body) {
  const response = await fetch(new URL(pathName, mainBase), {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let payload
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = text
  }
  return { status: response.status, payload }
}

async function loginAPI() {
  const response = await request('POST', '/api/v1/auth/login', '', { username, password })
  assertOk(response.status === 200 && response.payload?.access_token, 'main-server API login failed', response)
  return response.payload.access_token
}

async function findIdleProject(token) {
  const projects = await request('GET', `/api/v1/projects?edge_instance_id=${encodeURIComponent(edgeInstanceID)}&limit=80`, token)
  assertOk(projects.status === 200 && Array.isArray(projects.payload), 'projects query failed', projects)
  for (const project of projects.payload) {
    const projectID = Number(project.id)
    if (!projectID) continue
    const current = await request('GET', `/api/v1/detection-runs/current?project_id=${projectID}&edge_instance_id=${encodeURIComponent(edgeInstanceID)}`, token)
    if (current.status === 404) return project
  }
  throw new Error(`no idle project found for ${edgeInstanceID}`)
}

async function loginPage(page) {
  await page.goto(`${appBase}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input').nth(0).fill(username)
  await page.locator('input').nth(1).fill(password)
  const loginResponse = page.waitForResponse((response) => response.url().includes('/api/v1/auth/login') && response.status() === 200, { timeout: 15000 })
  await page.locator('button[type="submit"]').click()
  await loginResponse
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

function taskIDFromStart(payload) {
  const candidates = [payload?.result?.id, payload?.result?.task_id, payload?.id, payload?.task_id]
  for (const value of candidates) {
    const parsed = Number(value)
    if (Number.isFinite(parsed) && parsed > 0) return parsed
  }
  return 0
}

async function waitForText(page, text, timeout = 15000) {
  await page.waitForFunction((value) => document.body.innerText.includes(value), text, { timeout })
}

async function setKeyword(page, value) {
  const input = page.getByPlaceholder(/搜索消息|Search|検索/)
  await input.fill(value)
  await page.waitForTimeout(800)
}

async function setTypeFilter(page, label) {
  const typeSelect = page.locator('.ops-center-toolbar .ant-select').nth(1)
  await typeSelect.click()
  const dropdown = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').last()
  const option = dropdown.locator('.ant-select-item-option').filter({ hasText: label }).first()
  await option.waitFor({ state: 'visible', timeout: 8000 })
  await option.click()
  await page.waitForTimeout(800)
}

async function clickReadAll(page) {
  const response = page.waitForResponse((item) => item.url().includes('/api/v1/notifications/read-all') && item.request().method() === 'POST', { timeout: 15000 })
  await page.getByRole('button', { name: /全部已读|Mark all|すべて既読/ }).last().click()
  return response
}

async function unreadCount(token, type) {
  const response = await request('GET', `/api/v1/notifications/unread-count?keyword=${encodeURIComponent(testNo)}&type=${encodeURIComponent(type)}`, token)
  assertOk(response.status === 200, `unread-count failed for ${type}`, response)
  return Number(response.payload?.unread ?? 0)
}

await fs.mkdir(screenshotDir, { recursive: true })
await fs.mkdir(outDir, { recursive: true })

const mainToken = await loginAPI()
const project = await findIdleProject(mainToken)
const projectID = Number(project.id)

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })

page.on('request', (requestItem) => {
  const rawURL = requestItem.url()
  if (!isBusinessAPI(rawURL) && !isEdgeURL(rawURL)) return
  const entry = { method: requestItem.method(), url: rawURL }
  evidence.browser_requests.push(entry)
  if (isEdgeURL(rawURL)) evidence.direct_edge_browser_requests.push(entry)
  if (isBusinessAPI(rawURL)) {
    try {
      if (new URL(rawURL).host !== mainHost) evidence.api_non_main.push(entry)
    } catch {
      evidence.api_non_main.push(entry)
    }
  }
})

page.on('response', async (response) => {
  const rawURL = response.url()
  if (!isBusinessAPI(rawURL)) return
  const entry = { status: response.status(), url: rawURL, body: '' }
  if (response.status() >= 400) {
    entry.body = (await response.text().catch(() => '')).slice(0, 500)
    evidence.api_failures.push(entry)
  }
  evidence.browser_responses.push(entry)
})

page.on('websocket', (ws) => {
  const entry = { url: ws.url(), frames_received: 0 }
  evidence.browser_websockets.push(entry)
  if (isEdgeURL(ws.url())) evidence.direct_edge_browser_requests.push({ method: 'WS', url: ws.url() })
  try {
    if (isBusinessAPI(ws.url()) && new URL(ws.url()).host !== mainHost) evidence.api_non_main.push({ method: 'WS', url: ws.url() })
  } catch {
    evidence.api_non_main.push({ method: 'WS', url: ws.url() })
  }
  ws.on('framereceived', (frame) => {
    entry.frames_received += 1
    try {
      const message = JSON.parse(String(frame.payload))
      if (message.type === 'notification.event') {
        evidence.browser_ws_notifications.push({
          type: message.type,
          edge_instance_id: message.edge_instance_id,
          notification_type: message.payload?.type,
          test_no: message.payload?.test_no,
          message: message.payload?.message,
        })
      }
    } catch {
      // Non-JSON WS frames are not part of the API envelope.
    }
  })
})

page.on('console', (message) => {
  const entry = { type: message.type(), text: message.text() }
  if (message.type() === 'error') evidence.console_errors.push(entry)
  if (message.type() === 'warning' || message.type() === 'warn') evidence.console_warnings.push(entry)
})

page.on('pageerror', (error) => {
  evidence.page_errors.push({ message: error.message })
})

let taskID = 0
try {
  await loginPage(page)
  await page.goto(`${appBase}/#/notifications`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.waitForTimeout(1200)

  const notificationSocket = evidence.browser_websockets.find((item) => item.url.includes('/api/v1/ws') && item.url.includes('topic=notifications'))
  assertOk(notificationSocket, 'notification center did not open topic=notifications websocket', evidence.browser_websockets)

  const start = await request('POST', `/api/v1/detection-runs?edge_instance_id=${encodeURIComponent(edgeInstanceID)}`, mainToken, {
    project_id: projectID,
    edge_instance_id: edgeInstanceID,
    test_no: testNo,
    mode: 'custom',
    duration_sec: 30,
    operator_note: 'EB-050 notification page smoke',
  })
  assertOk(start.status === 200, 'detection start failed', start)
  taskID = taskIDFromStart(start.payload)
  assertOk(taskID > 0, 'detection start response missing task id', start.payload)

  await waitForText(page, testNo, 15000)
  await page.screenshot({ path: path.join(screenshotDir, 'notification-arrived.png'), fullPage: true }).catch(() => {})

  const stop = await request('POST', `/api/v1/detection-runs/${taskID}/stop?edge_instance_id=${encodeURIComponent(edgeInstanceID)}`, mainToken, { reason: 'EB-050 page smoke stop' })
  assertOk(stop.status === 200, 'detection stop failed', stop)
  await waitForText(page, 'detection run stopped', 15000).catch(async () => {
    await page.getByRole('button', { name: /刷新|Refresh|更新/ }).click()
    await waitForText(page, 'detection run stopped', 10000)
  })

  await setKeyword(page, testNo)
  await setTypeFilter(page, '检测开始')
  await waitForText(page, 'detection run started', 10000)
  const readStartedResponse = await clickReadAll(page)
  const readStarted = await readStartedResponse.json()
  assertOk(Number(readStarted?.updated ?? 0) >= 1, 'page scoped read-all should mark started notification', readStarted)
  await page.waitForTimeout(1000)

  const unreadStartedAfter = await unreadCount(mainToken, 'detection.run_started')
  const unreadStoppedAfterStartedRead = await unreadCount(mainToken, 'detection.run_stopped')
  assertOk(unreadStartedAfter === 0, 'started notification should be read after page scoped read-all', { unreadStartedAfter })
  assertOk(unreadStoppedAfterStartedRead >= 1, 'stopped notification should remain unread after started scoped read-all', { unreadStoppedAfterStartedRead })

  await setTypeFilter(page, '检测停止')
  await waitForText(page, 'detection run stopped', 10000)
  const bodyText = await page.locator('body').innerText()
  assertOk(bodyText.includes('未读') && bodyText.includes(testNo), 'stopped notification should still be visible as unread in page after started read-all')
  await page.screenshot({ path: path.join(screenshotDir, 'stopped-still-unread.png'), fullPage: true }).catch(() => {})

  const notificationWarnings = evidence.console_warnings.filter((item) => item.text.includes('/api/v1/ws') || item.text.toLowerCase().includes('websocket'))
  assertOk(notificationWarnings.length === 0, 'notification page still emitted websocket warning', notificationWarnings)
  assertOk(evidence.console_errors.length === 0, 'notification page emitted console errors', evidence.console_errors)
  assertOk(evidence.page_errors.length === 0, 'notification page emitted page errors', evidence.page_errors)
  assertOk(evidence.direct_edge_browser_requests.length === 0, 'main_server notification page directly contacted edge backend', evidence.direct_edge_browser_requests)
  assertOk(evidence.api_non_main.length === 0, 'main_server notification page used non-main API host', evidence.api_non_main)
  assertOk(evidence.browser_ws_notifications.some((item) => item.test_no === testNo), 'browser notification websocket did not receive this test notification', evidence.browser_ws_notifications)

  evidence.assertions = {
    notification_ws_opened: true,
    browser_ws_received_test_notification: true,
    page_list_refreshed_with_test_no: true,
    scoped_read_all_did_not_cross_type: true,
    browser_business_api_ws_only_main_server: true,
    no_notification_ws_warning: true,
  }
  evidence.project = { id: projectID, project_code: project.project_code, name: project.name }
  evidence.task_id = taskID
  evidence.read_started = readStarted
  evidence.unread_after_started_read = {
    detection_run_started: unreadStartedAfter,
    detection_run_stopped: unreadStoppedAfterStartedRead,
  }
  evidence.finished_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2))
  console.log(JSON.stringify({ ok: true, evidence: evidencePath, task_id: taskID, test_no: testNo }, null, 2))
} catch (error) {
  if (taskID > 0) {
    await request('POST', `/api/v1/detection-runs/${taskID}/stop?edge_instance_id=${encodeURIComponent(edgeInstanceID)}`, mainToken, { reason: 'EB-050 page smoke cleanup' }).catch(() => {})
  }
  evidence.failed_at = new Date().toISOString()
  evidence.error = error instanceof Error ? error.message : String(error)
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2)).catch(() => {})
  throw error
} finally {
  await browser.close().catch(() => {})
}
