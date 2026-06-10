import fs from 'node:fs/promises'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { chromium } from 'playwright'

const rootDir = path.resolve('..')
const mainBase = process.env.MAIN_BASE || 'http://127.0.0.1:19080'
const edge1Base = process.env.EDGE1_BASE || 'http://127.0.0.1:18080'
const edge2Base = process.env.EDGE2_BASE || 'http://127.0.0.1:18081'
const serviceToken = process.env.EDGE_MAIN_SERVICE_TOKEN || 'edge-main-dev-token-20260603'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const edge2Config = process.env.EDGE2_CONFIG || 'C:/Users/SunYufei/AppData/Local/Temp/edge-ws-smoke-retest-20260603132415/edge-2.json'
const rendererPort = Number(process.env.EB041_RENDERER_PORT || 5174)
const rendererBase = process.env.APP_BASE || `http://127.0.0.1:${rendererPort}`
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const outDir = path.resolve('output/playwright')
const evidencePath = path.join(outDir, `eb041-main-server-contract-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `eb041-main-server-contract-smoke-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  scope:
    'post-EB041 non-pressure one-main-two-edge contract regression: device_id rejection, auto edge routing, HTTP/WS realtime, command payload routing, edge-2 down negative, browser no direct edge',
  main_base: mainBase,
  edge1_base: edge1Base,
  edge2_base: edge2Base,
  renderer_base: rendererBase,
  browser_path: 'Playwright',
  browser_plugin: 'not available in this Codex session',
  checks: [],
  assertions: {},
  latencies_ms: {},
  browser: {
    requests: [],
    responses: [],
    websockets: [],
    direct_edge_requests: [],
    api_non_main: [],
    console_errors: [],
    page_errors: [],
    pages: [],
  },
}

function now() {
  return Number(process.hrtime.bigint() / 1000000n)
}

function record(name, ok, detail = {}) {
  evidence.checks.push({ name, ok, ...detail })
  evidence.assertions[name] = Boolean(ok)
}

function assertOk(condition, message) {
  if (!condition) throw new Error(message)
}

function unwrapItems(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.items)) return payload.items
  return []
}

function isEdgeURL(rawURL) {
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

async function jsonRequest(method, url, { token, body, headers = {}, allowStatuses = [200], label = url } = {}) {
  const started = now()
  const response = await fetch(url, {
    method,
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let payload = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = text
  }
  const elapsed = now() - started
  if (!allowStatuses.includes(response.status)) {
    throw new Error(`${label} returned ${response.status}: ${text.slice(0, 600)}`)
  }
  return { status: response.status, payload, elapsed, text }
}

async function login(base, label) {
  const result = await jsonRequest('POST', `${base}/api/v1/auth/login`, {
    body: { username, password },
    label: `${label} login`,
  })
  assertOk(result.payload?.access_token, `${label} login did not return access_token`)
  return result.payload.access_token
}

async function waitForHTTP(url, label, timeoutMS = 30000) {
  const deadline = Date.now() + timeoutMS
  let lastError = ''
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url)
      if (response.ok) return response
      lastError = `${response.status} ${await response.text().catch(() => '')}`
    } catch (error) {
      lastError = error.message
    }
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  throw new Error(`${label} did not become reachable at ${url}: ${lastError}`)
}

async function waitWS({ url, onOpen, match, timeoutMS = 15000 }) {
  return await new Promise((resolve, reject) => {
    const started = now()
    const ws = new WebSocket(url)
    const messages = []
    const timer = setTimeout(() => {
      try {
        ws.close()
      } catch {}
      reject(new Error(`websocket timeout: ${url}; messages=${JSON.stringify(messages.slice(-5))}`))
    }, timeoutMS)
    ws.addEventListener('open', () => {
      if (onOpen) onOpen(ws)
    })
    ws.addEventListener('message', (event) => {
      let payload = null
      try {
        payload = JSON.parse(event.data)
      } catch {
        payload = event.data
      }
      messages.push(payload)
      if (match(payload)) {
        clearTimeout(timer)
        try {
          ws.close()
        } catch {}
        resolve({ payload, elapsed: now() - started, messages })
      }
    })
    ws.addEventListener('error', () => {
      clearTimeout(timer)
      reject(new Error(`websocket error: ${url}`))
    })
  })
}

async function powershell(command, timeoutMS = 30000) {
  return await new Promise((resolve, reject) => {
    const child = spawn('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', command], {
      windowsHide: true,
      cwd: rootDir,
    })
    let stdout = ''
    let stderr = ''
    const timer = setTimeout(() => {
      child.kill()
      reject(new Error(`powershell timeout: ${command}`))
    }, timeoutMS)
    child.stdout.on('data', (chunk) => {
      stdout += chunk
    })
    child.stderr.on('data', (chunk) => {
      stderr += chunk
    })
    child.on('error', (error) => {
      clearTimeout(timer)
      reject(error)
    })
    child.on('exit', (code) => {
      clearTimeout(timer)
      if (code === 0) resolve(stdout.trim())
      else reject(new Error(`powershell exited ${code}: ${stderr || stdout}`))
    })
  })
}

async function listeningPID(port) {
  const result = await powershell(
    `$conn = Get-NetTCPConnection -LocalPort ${port} -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1; if ($conn) { $conn.OwningProcess }`,
    10000,
  )
  const value = Number(result)
  return Number.isFinite(value) && value > 0 ? value : 0
}

async function stopPort(port) {
  const pid = await listeningPID(port)
  if (!pid) return { pid: 0, stopped: false }
  await powershell(`Stop-Process -Id ${pid} -Force`, 10000)
  return { pid, stopped: true }
}

async function startEdge2() {
  const config = path.resolve(edge2Config)
  await powershell(
    `$env:EDGE_CONFIG='${config.replaceAll("'", "''")}'; Start-Process -WindowStyle Hidden -FilePath go -ArgumentList @('run','./cmd/edge-backend') -WorkingDirectory '${path.join(rootDir, 'backend').replaceAll("'", "''")}' -PassThru | Select-Object -ExpandProperty Id`,
    10000,
  )
  await waitForHTTP(`${edge2Base}/health`, 'edge-2 backend', 60000)
}

async function startRenderer() {
  try {
    await waitForHTTP(rendererBase, 'existing renderer', 1000)
    return null
  } catch {}
  const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'
  const child = spawn(npm, ['run', 'dev:renderer', '--', '--host', '127.0.0.1', '--port', String(rendererPort)], {
    cwd: path.join(rootDir, 'desktop'),
    env: {
      ...process.env,
      VITE_APP_ROLE: 'main_server',
      VITE_MAIN_API_BASE_URL: mainBase,
    },
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  })
  child.unref()
  await waitForHTTP(rendererBase, 'temporary main_server renderer', 60000)
  return child.pid
}

async function loginPage(page) {
  await page.goto(`${rendererBase}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input').nth(0).fill(username)
  await page.locator('input').nth(1).fill(password)
  const loginResponse = page.waitForResponse((response) => response.url().includes('/api/v1/auth/login') && response.status() === 200, {
    timeout: 15000,
  })
  await page.locator('button[type="submit"]').click()
  await loginResponse
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

async function runBrowserSmoke(edge1Project, edge2Project) {
  const rendererPID = await startRenderer()
  evidence.browser.renderer_pid = rendererPID || 'pre-existing'
  const mainHost = new URL(mainBase).host
  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } })
  let currentPage = 'bootstrap'
  page.on('request', (request) => {
    const rawURL = request.url()
    if (!isBusinessAPI(rawURL) && !isEdgeURL(rawURL)) return
    const entry = { page: currentPage, method: request.method(), url: rawURL }
    evidence.browser.requests.push(entry)
    if (isEdgeURL(rawURL)) evidence.browser.direct_edge_requests.push(entry)
    if (isBusinessAPI(rawURL)) {
      const host = new URL(rawURL).host
      if (host !== mainHost) evidence.browser.api_non_main.push(entry)
    }
  })
  page.on('response', (response) => {
    const rawURL = response.url()
    if (isBusinessAPI(rawURL)) evidence.browser.responses.push({ page: currentPage, status: response.status(), url: rawURL })
  })
  page.on('websocket', (ws) => {
    const rawURL = ws.url()
    const entry = { page: currentPage, url: rawURL }
    evidence.browser.websockets.push(entry)
    if (isEdgeURL(rawURL)) evidence.browser.direct_edge_requests.push({ page: currentPage, method: 'WS', url: rawURL })
    if (isBusinessAPI(rawURL)) {
      const host = new URL(rawURL).host
      if (host !== mainHost) evidence.browser.api_non_main.push({ page: currentPage, method: 'WS', url: rawURL })
    }
  })
  page.on('console', (message) => {
    if (message.type() === 'error') evidence.browser.console_errors.push({ page: currentPage, text: message.text() })
  })
  page.on('pageerror', (error) => {
    evidence.browser.page_errors.push({ page: currentPage, message: error.message })
  })
  try {
    await loginPage(page)
    for (const item of [
      { key: 'station_edge1', url: `${rendererBase}/#/station?project_id=${edge1Project.id}`, expect: ['工位', '检测'] },
      { key: 'station_edge2', url: `${rendererBase}/#/station?project_id=${edge2Project.id}`, expect: ['工位', '检测'] },
      { key: 'variables', url: `${rendererBase}/#/variables`, expect: ['变量', '项目'] },
      { key: 'tasks', url: `${rendererBase}/#/tasks`, expect: ['任务', '发送任务请求'] },
      { key: 'reports', url: `${rendererBase}/#/reports`, expect: ['报表', '任务'] },
      { key: 'notifications', url: `${rendererBase}/#/notifications`, expect: ['通知'] },
    ]) {
      currentPage = item.key
      await page.goto(item.url, { waitUntil: 'domcontentloaded' })
      await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
      await page.waitForTimeout(1200)
      const bodyText = await page.locator('body').innerText().catch(() => '')
      const screenshot = path.join(screenshotDir, `${item.key}.png`)
      await page.screenshot({ path: screenshot, fullPage: true }).catch(() => {})
      evidence.browser.pages.push({
        key: item.key,
        url: item.url,
        body_length: bodyText.length,
        matched_expect: item.expect.filter((text) => bodyText.includes(text)),
        screenshot,
      })
    }
  } finally {
    await browser.close()
    if (rendererPID) {
      await powershell(`Stop-Process -Id ${rendererPID} -Force`, 10000).catch(() => {})
    }
  }
  const pagesRendered = evidence.browser.pages.every((item) => item.body_length > 100 && item.matched_expect.length > 0)
  record('browser_main_server_pages_render_without_direct_edge', pagesRendered && evidence.browser.direct_edge_requests.length === 0 && evidence.browser.api_non_main.length === 0 && evidence.browser.page_errors.length === 0, {
    renderer: rendererBase,
    pages: evidence.browser.pages.map((item) => ({ key: item.key, body_length: item.body_length, matched_expect: item.matched_expect })),
    direct_edge_requests: evidence.browser.direct_edge_requests,
    api_non_main: evidence.browser.api_non_main,
    page_errors: evidence.browser.page_errors,
  })
}

await fs.mkdir(screenshotDir, { recursive: true })

let edge2StoppedForNegative = false
try {
  await waitForHTTP(`${edge1Base}/health`, 'edge-1 backend', 15000)
  await waitForHTTP(`${edge2Base}/health`, 'edge-2 backend', 15000)
  await waitForHTTP(`${mainBase}/health`, 'main-server backend', 15000)

  const mainToken = await login(mainBase, 'main-server')
  const edge1Token = await login(edge1Base, 'edge-1')
  record('login_main_and_edge_1', true)

  const health = await jsonRequest('GET', `${mainBase}/health`, { label: 'main health' })
  const status = await jsonRequest('GET', `${mainBase}/api/v1/main-server/status`, { token: mainToken, label: 'main status' })
  const edgeNodes = status.payload?.edge_nodes ?? health.payload?.edge_nodes ?? []
  record('three_backends_and_main_status_have_two_edges', ['edge-1', 'edge-2'].every((edge) => edgeNodes.some((item) => item.edge_instance_id === edge && item.enabled)), {
    health: health.payload,
    edge_nodes: edgeNodes,
  })

  const edge1ProjectsResp = await jsonRequest('GET', `${mainBase}/api/v1/projects?edge_instance_id=edge-1`, { token: mainToken })
  const edge2ProjectsResp = await jsonRequest('GET', `${mainBase}/api/v1/projects?edge_instance_id=edge-2`, { token: mainToken })
  const edge1Project = unwrapItems(edge1ProjectsResp.payload).reverse().find((item) => item.edge_instance_id === 'edge-1')
  const edge2Project = unwrapItems(edge2ProjectsResp.payload).reverse().find((item) => item.edge_instance_id === 'edge-2')
  assertOk(edge1Project?.id && edge2Project?.id, 'expected at least one synced project for edge-1 and edge-2')
  evidence.projects = { edge1: edge1Project, edge2: edge2Project }
  record('project_auto_routing_candidates_found', true, { edge1_project_id: edge1Project.id, edge2_project_id: edge2Project.id })

  const edgeVarsDevice = await jsonRequest('GET', `${edge1Base}/api/v1/variables?device_id=1`, {
    token: edge1Token,
    allowStatuses: [400],
    label: 'edge variables device_id',
  })
  const edgeRealtimeDevice = await jsonRequest('GET', `${edge1Base}/api/v1/realtime/variables?device_id=1`, {
    token: edge1Token,
    allowStatuses: [400],
    label: 'edge realtime device_id',
  })
  const mainVarsDevice = await jsonRequest('GET', `${mainBase}/api/v1/variables?device_id=1`, {
    token: mainToken,
    allowStatuses: [400],
    label: 'main variables device_id',
  })
  const mainRealtimeDevice = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?device_id=1`, {
    token: mainToken,
    allowStatuses: [400],
    label: 'main realtime device_id',
  })
  record('device_id_query_rejected_on_edge_and_main', [edgeVarsDevice, edgeRealtimeDevice, mainVarsDevice, mainRealtimeDevice].every((item) => item.status === 400 && `${item.text}`.includes('unsupported_query_param')), {
    statuses: {
      edge_variables: edgeVarsDevice.status,
      edge_realtime: edgeRealtimeDevice.status,
      main_variables: mainVarsDevice.status,
      main_realtime: mainRealtimeDevice.status,
    },
  })

  const e1Realtime = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge1Project.id}&limit=20`, { token: mainToken, label: 'edge1 realtime through main' })
  const e2Realtime = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge2Project.id}&limit=50`, { token: mainToken, label: 'edge2 realtime through main' })
  const e1Vars = unwrapItems(e1Realtime.payload)
  const e2Vars = unwrapItems(e2Realtime.payload)
  record('http_realtime_auto_routes_projects_to_both_edges', e1Vars.length > 0 && e2Vars.length > 0, {
    edge1_count: e1Vars.length,
    edge2_count: e2Vars.length,
    edge1_elapsed_ms: e1Realtime.elapsed,
    edge2_elapsed_ms: e2Realtime.elapsed,
  })

  const mismatch = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge2Project.id}&edge_instance_id=edge-1&limit=1`, {
    token: mainToken,
    allowStatuses: [404],
    label: 'project edge mismatch',
  })
  record('project_edge_mismatch_rejected', mismatch.status === 404 && `${mismatch.text}`.includes('project_edge_instance_mismatch'), { body: mismatch.payload })

  const wsRealtime = await waitWS({
    url: `ws://127.0.0.1:19080/api/v1/ws?access_token=${encodeURIComponent(mainToken)}&topic=realtime.variables&project_id=${edge2Project.id}`,
    match: (payload) => payload?.type === 'realtime.variables.snapshot' && payload?.edge_instance_id === 'edge-2',
  })
  record('ws_realtime_auto_routes_edge_2_project', true, { elapsed_ms: wsRealtime.elapsed, sample: wsRealtime.payload })

  const virtualVar = e2Vars.find((item) => item.source_type === 'virtual' && item.var_id_text) || e2Vars.find((item) => item.var_id_text)
  assertOk(virtualVar?.var_id_text, 'edge-2 realtime list did not include a writable smoke variable')
  const commandID = `eb041-ws-edge2-${stamp}`
  const value = virtualVar.is_string ? JSON.stringify({ eb041_probe: stamp }) : 41.041
  const wsCommand = await waitWS({
    url: `ws://127.0.0.1:19080/api/v1/ws?access_token=${encodeURIComponent(mainToken)}`,
    onOpen: (ws) => {
      ws.send(
        JSON.stringify({
          type: 'command.write_variable',
          request_id: `req-${commandID}`,
          command_id: commandID,
          payload: {
            project_id: edge2Project.id,
            var_id: virtualVar.var_id_text,
            value,
            trigger: false,
          },
        }),
      )
    },
    match: (payload) => payload?.type === 'command.ack' && payload?.command_id === commandID,
  })
  const commandStatus = await jsonRequest('GET', `${edge2Base}/api/v1/edge-control/commands/${commandID}`, {
    token: serviceToken,
    label: 'edge-2 command lifecycle',
  })
  const readback = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge2Project.id}&var_id=${virtualVar.var_id_text}`, {
    token: mainToken,
    label: 'main readback after command',
  })
  const readbackItems = unwrapItems(readback.payload)
  const readbackMatched = readbackItems.some((item) => {
    if (virtualVar.is_string) return item.str_value === value
    return Number(item.value) === Number(value)
  })
  record('ws_command_only_payload_routes_to_edge_2', commandStatus.payload?.status === 'success' && readbackMatched, {
    elapsed_ms: wsCommand.elapsed,
    ack: wsCommand.payload,
    command_status: commandStatus.payload,
    readback: readback.payload,
  })

  evidence.latencies_ms = {
    http_realtime_edge_1: e1Realtime.elapsed,
    http_realtime_edge_2: e2Realtime.elapsed,
    ws_realtime_snapshot_edge_2: wsRealtime.elapsed,
    ws_command_ack_edge_2: wsCommand.elapsed,
    edge_command_status_read: commandStatus.elapsed,
    main_realtime_readback: readback.elapsed,
  }

  const stopped = await stopPort(18081)
  edge2StoppedForNegative = stopped.stopped
  record('edge_2_temporarily_stopped_for_negative', edge2StoppedForNegative, stopped)
  await new Promise((resolve) => setTimeout(resolve, 1200))

  const edge2DownRealtime = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge2Project.id}&limit=1`, {
    token: mainToken,
    allowStatuses: [502],
    label: 'edge2 down realtime',
  })
  const edge2DownRuntime = await jsonRequest('GET', `${mainBase}/api/v1/runtime/channels/detail?edge_instance_id=edge-2`, {
    token: mainToken,
    allowStatuses: [502],
    label: 'edge2 down runtime',
  })
  const edge2DownControl = await jsonRequest('POST', `${mainBase}/api/v1/edge-control/variables/write`, {
    token: mainToken,
    body: { payload: { project_id: edge2Project.id, var_id: virtualVar.var_id_text, value, trigger: false } },
    allowStatuses: [502],
    label: 'edge2 down control',
  })
  const edge1StillWorks = await jsonRequest('GET', `${mainBase}/api/v1/realtime/variables?project_id=${edge1Project.id}&limit=1`, {
    token: mainToken,
    label: 'edge1 realtime while edge2 down',
  })
  record('edge_2_down_returns_502_without_breaking_edge_1', [edge2DownRealtime, edge2DownRuntime, edge2DownControl].every((item) => item.status === 502) && edge1StillWorks.status === 200, {
    edge2_statuses: {
      realtime: edge2DownRealtime.status,
      runtime: edge2DownRuntime.status,
      control: edge2DownControl.status,
    },
    edge1_count: unwrapItems(edge1StillWorks.payload).length,
  })

  await startEdge2()
  edge2StoppedForNegative = false
  const edge2Restored = await jsonRequest('GET', `${edge2Base}/health`, { label: 'edge2 restored health' })
  record('edge_2_restored_after_negative', edge2Restored.payload?.status === 'ok', { health: edge2Restored.payload })

  await runBrowserSmoke(edge1Project, edge2Project)

  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')
  console.log(
    JSON.stringify(
      {
        ok: failed.length === 0,
        evidencePath,
        screenshotDir,
        failed: failed.map(([name]) => name),
        projects: { edge1: edge1Project.id, edge2: edge2Project.id },
        latencies_ms: evidence.latencies_ms,
      },
      null,
      2,
    ),
  )
  if (failed.length) throw new Error(`EB041 smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  if (edge2StoppedForNegative) {
    await startEdge2().catch((error) => {
      evidence.edge2_restore_error = error.message
    })
  }
}
