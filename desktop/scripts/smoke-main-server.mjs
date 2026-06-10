import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const appBase = process.env.RENDERER_URL || process.env.APP_BASE || 'http://127.0.0.1:5173'
const mainBase = process.env.MAIN_BASE || 'http://127.0.0.1:19080'
const mainHost = new URL(mainBase).host
const viteAppRole = process.env.VITE_APP_ROLE || ''
const viteMainAPIBase = process.env.VITE_MAIN_API_BASE_URL || ''
const mistakenMainServerAPIBase = process.env.VITE_MAIN_SERVER_API_BASE_URL || ''
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const edge1ProjectID = process.env.EDGE1_PROJECT_ID || '136'
const edge2ProjectID = process.env.EDGE2_PROJECT_ID || '138'
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `main-server-regression-${stamp}.json`)
const screenshotDir = path.join(outDir, `main-server-regression-${stamp}`)

const pages = [
  { key: 'debug', url: `${appBase}/#/debug`, expect: ['浏览器预览', '实时变量', '主服务器后端'] },
  { key: 'settings', url: `${appBase}/#/settings`, expect: ['系统', '运行', '配置', 'settings'] },
  { key: 'tasks', url: `${appBase}/#/tasks`, expect: ['任务', 'task-flows', '发送任务请求'] },
  { key: 'station_edge1', url: `${appBase}/#/station?project_id=${edge1ProjectID}`, expect: ['工位', '检测', 'WS-SMOKE-E1'] },
  { key: 'station_edge2', url: `${appBase}/#/station?project_id=${edge2ProjectID}`, expect: ['工位', '检测', 'WS-SMOKE-E2'] },
  { key: 'variables', url: `${appBase}/#/variables`, expect: ['变量', '项目', 'virtual'] },
  { key: 'history', url: `${appBase}/#/history`, expect: ['历史', '查询', '任务'] },
  { key: 'alarms', url: `${appBase}/#/alarms`, expect: ['报警', '超限', '任务'] },
  { key: 'notifications', url: `${appBase}/#/notifications`, expect: ['通知', '未读', '类型'] },
]

const evidence = {
  started_at: new Date().toISOString(),
  app_role: 'main_server',
  browser_path: 'Playwright',
  browser_plugin: 'not available in this Codex session',
  app_base: appBase,
  main_base: mainBase,
  env_preflight: {
    vite_app_role: viteAppRole || undefined,
    vite_main_api_base_url: viteMainAPIBase || undefined,
    vite_main_server_api_base_url_present: Boolean(mistakenMainServerAPIBase),
  },
  projects: { edge1: edge1ProjectID, edge2: edge2ProjectID },
  pages: [],
  requests: [],
  responses: [],
  websockets: [],
  direct_edge_browser_requests: [],
  api_non_main: [],
  api_failures: [],
  console_errors: [],
  page_errors: [],
  assertions: {},
}

function hostOf(rawURL) {
  try {
    return new URL(rawURL).host
  } catch {
    return ''
  }
}

function assertMainServerEnv() {
  const errors = []
  if (viteAppRole !== 'main_server') {
    errors.push(`VITE_APP_ROLE must be main_server for this smoke; got ${viteAppRole || '<unset>'}.`)
  }
  if (mistakenMainServerAPIBase) {
    errors.push('VITE_MAIN_SERVER_API_BASE_URL is not read by the renderer; use VITE_MAIN_API_BASE_URL instead.')
  }
  if (!viteMainAPIBase) {
    errors.push('VITE_MAIN_API_BASE_URL must be set to the main-server API base, for example http://127.0.0.1:19080.')
  } else if (hostOf(viteMainAPIBase) !== mainHost) {
    errors.push(`VITE_MAIN_API_BASE_URL host must match MAIN_BASE host ${mainHost}; got ${hostOf(viteMainAPIBase) || viteMainAPIBase}.`)
  }
  if (viteMainAPIBase && isEdgeUrl(viteMainAPIBase)) {
    errors.push(`VITE_MAIN_API_BASE_URL points to an edge backend (${viteMainAPIBase}); main_server smoke must target ${mainBase}.`)
  }
  if (errors.length) {
    const error = new Error(`main-server smoke environment preflight failed: ${errors.join(' ')}`)
    error.preflight_errors = errors
    throw error
  }
  evidence.env_preflight.ok = true
}

function isEdgeUrl(rawURL) {
  return (
    rawURL.includes('127.0.0.1:18080') ||
    rawURL.includes('127.0.0.1:18081') ||
    rawURL.includes('localhost:18080') ||
    rawURL.includes('localhost:18081')
  )
}

function isBusinessAPI(rawURL) {
  return rawURL.includes('/api/v1/')
}

function isAllowedDiagnostic(item) {
  const url = item.url
  const status = item.status
  const text = `${item.body ?? ''}`
  if (status === 404 && (url.includes('/detection-runs/current') || url.includes('/detection-runs/active'))) return true
  if (status === 404 && text.includes('not_found') && url.includes('/main-server/report-jobs/')) return true
  return false
}

async function assertReachable(url, label) {
  const response = await fetch(url).catch((error) => {
    throw new Error(`${label} is not reachable at ${url}: ${error.message}`)
  })
  if (!response.ok) {
    throw new Error(`${label} returned ${response.status} at ${url}`)
  }
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

function routeFromURL(rawURL) {
  try {
    const parsed = new URL(rawURL)
    return parsed.hash || parsed.pathname
  } catch {
    return rawURL
  }
}

await fs.mkdir(screenshotDir, { recursive: true })
try {
  assertMainServerEnv()
  if (process.env.SMOKE_PREFLIGHT_ONLY === '1') {
    evidence.completed_at = new Date().toISOString()
    await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')
    console.log(JSON.stringify({ ok: true, evidencePath, preflight_only: true, env_preflight: evidence.env_preflight }, null, 2))
    process.exit(0)
  }
  await assertReachable(appBase, 'renderer')
  await assertReachable(`${mainBase}/health`, 'main server')
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.preflight_errors = error?.preflight_errors || undefined
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, preflight_errors: evidence.preflight_errors }, null, 2))
  throw error
}

const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })
let currentPageKey = 'bootstrap'

page.on('request', (request) => {
  const rawURL = request.url()
  const entry = { page: currentPageKey, method: request.method(), url: rawURL }
  if (isBusinessAPI(rawURL) || isEdgeUrl(rawURL)) evidence.requests.push(entry)
  if (isEdgeUrl(rawURL)) evidence.direct_edge_browser_requests.push(entry)
  if (isBusinessAPI(rawURL)) {
    try {
      const parsed = new URL(rawURL)
      if (parsed.host !== mainHost) evidence.api_non_main.push(entry)
    } catch {
      evidence.api_non_main.push(entry)
    }
  }
})

page.on('response', async (response) => {
  const rawURL = response.url()
  if (!isBusinessAPI(rawURL)) return
  const entry = { page: currentPageKey, status: response.status(), url: rawURL, body: '' }
  if (entry.status >= 400) {
    entry.body = (await response.text().catch(() => '')).slice(0, 600)
    evidence.api_failures.push(entry)
  }
  evidence.responses.push(entry)
})

page.on('websocket', (ws) => {
  const rawURL = ws.url()
  const entry = { page: currentPageKey, url: rawURL }
  evidence.websockets.push(entry)
  if (isEdgeUrl(rawURL)) evidence.direct_edge_browser_requests.push({ page: currentPageKey, method: 'WS', url: rawURL })
  if (isBusinessAPI(rawURL)) {
    try {
      const parsed = new URL(rawURL)
      if (parsed.host !== mainHost) evidence.api_non_main.push({ page: currentPageKey, method: 'WS', url: rawURL })
    } catch {
      evidence.api_non_main.push({ page: currentPageKey, method: 'WS', url: rawURL })
    }
  }
})

page.on('console', (message) => {
  if (message.type() === 'error') {
    evidence.console_errors.push({ page: currentPageKey, text: message.text() })
  }
})

page.on('pageerror', (error) => {
  evidence.page_errors.push({ page: currentPageKey, message: error.message })
})

try {
  await login(page)

  for (const item of pages) {
    currentPageKey = item.key
    await page.goto(item.url, { waitUntil: 'domcontentloaded' })
    await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
    await page.waitForTimeout(1500)
    const bodyText = await page.locator('body').innerText().catch(() => '')
    const screenshot = path.join(screenshotDir, `${item.key}.png`)
    await page.screenshot({ path: screenshot, fullPage: true }).catch(() => {})
    const matched = item.expect.filter((fragment) => bodyText.includes(fragment))
    evidence.pages.push({
      key: item.key,
      url: item.url,
      final_url: page.url(),
      route: routeFromURL(page.url()),
      body_length: bodyText.length,
      matched_expect: matched,
      screenshot,
      sample: bodyText.slice(0, 800),
    })
  }

  const unexpectedAPIFailures = evidence.api_failures.filter((entry) => !isAllowedDiagnostic(entry))
  evidence.assertions = {
    all_pages_rendered: evidence.pages.every((item) => item.body_length > 100 && item.matched_expect.length > 0),
    browser_did_not_direct_edge: evidence.direct_edge_browser_requests.length === 0,
    all_business_api_or_ws_use_main_server: evidence.api_non_main.length === 0,
    no_unexpected_api_failures: unexpectedAPIFailures.length === 0,
    no_page_errors: evidence.page_errors.length === 0,
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
    page_count: evidence.pages.length,
    api_failure_count: evidence.api_failures.length,
    unexpected_api_failure_count: unexpectedAPIFailures.length,
    direct_edge_browser_request_count: evidence.direct_edge_browser_requests.length,
    websocket_count: evidence.websockets.length,
  }
  console.log(JSON.stringify(summary, null, 2))
  if (failed.length) throw new Error(`main-server frontend smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  await browser.close()
}
