import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.RENDERER_URL || process.env.APP_BASE || 'http://127.0.0.1:5173'
const apiBase = process.env.API_BASE || process.env.MAIN_BASE || 'http://127.0.0.1:19080'
const apiURL = new URL(apiBase)
const projectID = process.env.PROJECT_ID || '1'
const edgeInstanceID = process.env.EDGE_INSTANCE_ID || 'edge-1'
const edgeProjectID = process.env.EDGE1_PROJECT_ID || '136'
const wrongEdgeInstanceID = process.env.WRONG_EDGE_INSTANCE_ID || 'edge-2'
const emptyVarGroup = process.env.EMPTY_PID_VAR_GROUP || 'NO_SUCH_PID_GROUP'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `pid-negative-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `pid-negative-smoke-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  app_base: appBase,
  api_base: apiBase,
  project_id: projectID,
  edge_instance_id: edgeInstanceID,
  edge_project_id: edgeProjectID,
  wrong_edge_instance_id: wrongEdgeInstanceID,
  write_mode: false,
  write_skipped_reason: 'Negative PID smoke never sends command.write_variable and never writes KIO/PLC.',
  http: {},
  websocket: {},
  page: {},
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

async function request(pathname, token) {
  const response = await fetch(new URL(pathname, apiBase), {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
  const text = await response.text()
  let body = text
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = text
    }
  }
  return { status: response.status, ok: response.ok, body }
}

async function login() {
  const response = await fetch(new URL('/api/v1/auth/login', apiBase), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const body = await response.json()
  if (!response.ok || !body.access_token) {
    throw new Error(`login failed: ${response.status} ${JSON.stringify(body)}`)
  }
  return body.access_token
}

function websocketURL(pathname) {
  const url = new URL(pathname, apiBase)
  url.protocol = apiURL.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

async function expectWebSocketNotOpen(pathname, label) {
  const url = websocketURL(pathname)
  return new Promise((resolve) => {
    const ws = new WebSocket(url)
    const result = {
      label,
      url: redactURL(url),
      opened: false,
      closed: false,
      errored: false,
      close_code: undefined,
      close_reason: '',
    }
    const timer = setTimeout(() => {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) ws.close()
      resolve(result)
    }, 2500)
    ws.addEventListener('open', () => {
      result.opened = true
      ws.close()
    })
    ws.addEventListener('error', () => {
      result.errored = true
    })
    ws.addEventListener('close', (event) => {
      clearTimeout(timer)
      result.closed = true
      result.close_code = event.code
      result.close_reason = event.reason
      resolve(result)
    })
  })
}

function okCode(body, code) {
  return body && typeof body === 'object' && body.code === code
}

await fs.mkdir(screenshotDir, { recursive: true })

try {
  const health = await request('/health')
  if (!health.ok) throw new Error(`backend health failed: ${health.status}`)
  const token = await login()

  const unauthVariables = await request(`/api/v1/variables?project_id=${projectID}&edge_instance_id=${edgeInstanceID}&var_group=KIO%E5%8F%98%E9%87%8F&writable=true&enabled=true`)
  const ambiguousStation = await request(`/api/v1/station-view/effective?project_id=${projectID}`, token)
  const stationMismatch = await request(`/api/v1/station-view/effective?project_id=${edgeProjectID}&edge_instance_id=${wrongEdgeInstanceID}`, token)
  const realtimeMismatch = await request(`/api/v1/realtime/variables?project_id=${edgeProjectID}&edge_instance_id=${wrongEdgeInstanceID}`, token)
  const emptyPIDVariables = await request(`/api/v1/variables?project_id=${projectID}&edge_instance_id=${edgeInstanceID}&var_group=${encodeURIComponent(emptyVarGroup)}&writable=true&enabled=true`, token)

  evidence.http = {
    unauth_variables: unauthVariables,
    ambiguous_station_without_edge: ambiguousStation,
    station_edge_mismatch: stationMismatch,
    realtime_edge_mismatch: realtimeMismatch,
    empty_pid_variables: emptyPIDVariables,
  }

  const noTokenWS = await expectWebSocketNotOpen(
    `/api/v1/ws?topic=realtime.variables&project_id=${projectID}&edge_instance_id=${edgeInstanceID}`,
    'missing access_token',
  )
  const mismatchWS = await expectWebSocketNotOpen(
    `/api/v1/ws?access_token=${encodeURIComponent(token)}&topic=realtime.variables&project_id=${edgeProjectID}&edge_instance_id=${wrongEdgeInstanceID}`,
    'project edge mismatch',
  )
  evidence.websocket = { missing_token: noTokenWS, project_edge_mismatch: mismatchWS }

  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })
  const pageAPIFailures = []
  const pageErrors = []
  const pageRequests = []
  page.on('request', (requestItem) => {
    const rawURL = requestItem.url()
    if (rawURL.includes('/api/v1/')) pageRequests.push({ method: requestItem.method(), url: redactURL(rawURL) })
  })
  page.on('response', async (response) => {
    const rawURL = response.url()
    if (!rawURL.includes('/api/v1/')) return
    if (response.status() >= 400 && !rawURL.includes('/detection-runs/current') && !rawURL.includes('/detection-runs/active')) {
      pageAPIFailures.push({
        status: response.status(),
        url: redactURL(rawURL),
        body: (await response.text().catch(() => '')).slice(0, 600),
      })
    }
  })
  page.on('pageerror', (error) => pageErrors.push(error.message))

  try {
    await page.goto(`${appBase}/#/login`, { waitUntil: 'domcontentloaded' })
    await page.locator('input').nth(0).fill(username)
    await page.locator('input').nth(1).fill(password)
    const loginResponse = page.waitForResponse((response) => response.url().includes('/api/v1/auth/login') && response.status() === 200, { timeout: 15000 })
    await page.locator('button[type="submit"]').click()
    await loginResponse
    await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })

    await page.goto(`${appBase}/#/station?project_id=${encodeURIComponent(projectID)}&edge_instance_id=${encodeURIComponent(edgeInstanceID)}`, { waitUntil: 'domcontentloaded' })
    await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
    await page.getByRole('button', { name: /PID|pid/i }).click()
    await page.locator('.station-pid-modal').waitFor({ state: 'visible', timeout: 15000 })
    const emptyResponse = page.waitForResponse(
      (response) => response.url().includes('/api/v1/variables') && response.url().includes(encodeURIComponent(emptyVarGroup)),
      { timeout: 15000 },
    )
    await page.locator('.station-pid-modal input').first().fill(emptyVarGroup)
    await emptyResponse
    await page.waitForTimeout(1000)
    const modalText = await page.locator('.station-pid-modal').innerText()
    const settings = await page.locator('.station-pid-setting').evaluateAll((nodes) =>
      nodes.map((node) => {
        const text = node.textContent || ''
        return { text: text.slice(0, 300), connected: !/无连接|No connection|未接続/.test(text) }
      }),
    )
    const screenshot = path.join(screenshotDir, 'empty-pid-group.png')
    await page.screenshot({ path: screenshot, fullPage: true }).catch(() => {})
    evidence.page = {
      requests: pageRequests,
      api_failures: pageAPIFailures,
      page_errors: pageErrors,
      modal_text_sample: modalText.slice(0, 1000),
      setting_count: settings.length,
      connected_setting_count: settings.filter((item) => item.connected).length,
      disconnected_setting_count: settings.filter((item) => !item.connected).length,
      screenshot,
    }
  } finally {
    await browser.close()
  }

  evidence.assertions = {
    unauth_variables_rejected: [401, 403].includes(unauthVariables.status),
    ambiguous_station_requires_edge: ambiguousStation.status === 409 && okCode(ambiguousStation.body, 'station_view_edge_instance_ambiguous'),
    station_edge_mismatch_rejected: stationMismatch.status === 404 && okCode(stationMismatch.body, 'station_view_edge_instance_mismatch'),
    realtime_edge_mismatch_rejected: realtimeMismatch.status === 404 && okCode(realtimeMismatch.body, 'project_edge_instance_mismatch'),
    empty_pid_group_returns_empty: emptyPIDVariables.status === 200 && Array.isArray(emptyPIDVariables.body) && emptyPIDVariables.body.length === 0,
    websocket_without_token_not_opened: noTokenWS.opened === false,
    websocket_mismatch_not_opened: mismatchWS.opened === false,
    empty_pid_group_page_disconnected: evidence.page.setting_count > 0 && evidence.page.connected_setting_count === 0,
    empty_pid_group_page_no_unexpected_api_failures: evidence.page.api_failures.length === 0,
    empty_pid_group_page_no_errors: evidence.page.page_errors.length === 0,
  }
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')

  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  const summary = {
    ok: failed.length === 0,
    evidencePath,
    screenshotDir,
    assertions: evidence.assertions,
    write_mode: false,
  }
  console.log(JSON.stringify(summary, null, 2))
  if (failed.length) throw new Error(`PID negative smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
}
