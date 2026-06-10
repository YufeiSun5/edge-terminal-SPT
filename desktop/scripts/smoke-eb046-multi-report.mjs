import fs from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'

const edgeAppBase = process.env.EDGE_APP_BASE || 'http://127.0.0.1:5173'
const mainAppBase = process.env.MAIN_APP_BASE || 'http://127.0.0.1:5174'
const edgeBase = process.env.EDGE_BASE || 'http://127.0.0.1:18080'
const mainBase = process.env.MAIN_BASE || 'http://127.0.0.1:19080'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const edgeInstanceID = process.env.EDGE_INSTANCE_ID || 'edge-1'
const waitMs = Number(process.env.EB046_WAIT_MS || 1800)
const outDir = path.resolve('output/playwright')
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const evidencePath = path.join(outDir, `eb046-multi-report-smoke-${stamp}.json`)
const screenshotDir = path.join(outDir, `eb046-multi-report-smoke-${stamp}`)

const evidence = {
  started_at: new Date().toISOString(),
  flow_under_test:
    'station page start detection -> report_request.reports[] with two reports -> edge report request snapshots -> main-server report jobs/artifacts -> reports page download',
  browser_path: 'Playwright',
  browser_plugin: 'not available in this Codex session',
  edge_app_base: edgeAppBase,
  main_app_base: mainAppBase,
  edge_base: edgeBase,
  main_base: mainBase,
  edge_instance_id: edgeInstanceID,
  setup: {},
  browser_requests: [],
  browser_responses: [],
  browser_websockets: [],
  direct_edge_browser_requests_in_main_server: [],
  main_server_non_main_api: [],
  edge_browser_non_edge_api: [],
  console_errors: [],
  page_errors: [],
  runs: [],
  assertions: {},
}

function assertOk(condition, message) {
  if (!condition) throw new Error(message)
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function suffix() {
  return `${Date.now().toString().slice(-8)}${Math.floor(Math.random() * 1000).toString().padStart(3, '0')}`
}

function safeVarID(offset) {
  return 760000000 + Math.floor(Math.random() * 10000000) + offset
}

function isEdgeURL(rawURL) {
  return rawURL.includes('127.0.0.1:18080') || rawURL.includes('127.0.0.1:18081') || rawURL.includes('localhost:18080') || rawURL.includes('localhost:18081')
}

function isBusinessAPI(rawURL) {
  return rawURL.includes('/api/v1/')
}

function hostOf(rawURL) {
  try {
    return new URL(rawURL).host
  } catch {
    return ''
  }
}

function isExpectedEdgeHost(rawURL) {
  return hostOf(rawURL) === new URL(edgeBase).host
}

async function api(base, pathName, options = {}, token = '', label = pathName) {
  const response = await fetch(new URL(pathName, base), {
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

async function loginAPI(base) {
  const body = await api(
    base,
    '/api/v1/auth/login',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    },
    '',
    `${base} login`,
  )
  assertOk(body?.access_token, `${base} login response missing access_token`)
  return body.access_token
}

async function ensureReachable(url, label) {
  const response = await fetch(url).catch((error) => {
    throw new Error(`${label} is not reachable at ${url}: ${error.message}`)
  })
  assertOk(response.ok, `${label} returned ${response.status} at ${url}`)
}

async function createFixture(edgeToken) {
  const id = suffix()
  const project = await api(
    edgeBase,
    '/api/v1/projects',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        project_code: `EB046-${id}`,
        site_no: 'eb046-smoke',
        edge_instance_id: edgeInstanceID,
        name: `EB046 multi report ${id}`,
        display_name: `EB046多报表${id}`,
        display_name_en: `EB046 multi report ${id}`,
        display_name_ja: `EB046複数帳票${id}`,
      }),
    },
    edgeToken,
    'create EB046 smoke project',
  )
  const vars = []
  for (const [index, name] of ['eb046_temp_', 'eb046_pressure_'].entries()) {
    const variable = await api(
      edgeBase,
      '/api/v1/variables',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          var_id: safeVarID(index + 1),
          source_type: 'virtual',
          project_id: project.id,
          var_group: 'EB046 smoke',
          var_name: `${name}${id}`,
          display_name: index === 0 ? `温度${id}` : `压力${id}`,
          display_name_en: index === 0 ? `Temperature ${id}` : `Pressure ${id}`,
          display_name_ja: index === 0 ? `温度${id}` : `圧力${id}`,
          data_type: 'FLOAT',
          unit: index === 0 ? 'C' : 'Pa',
          enabled: true,
        }),
      },
      edgeToken,
      `create variable ${index + 1}`,
    )
    vars.push(variable)
  }
  for (const variable of vars) {
    const routes = await api(edgeBase, `/api/v1/storage-routes?project_id=${project.id}&var_id=${variable.var_id_text ?? variable.var_id}`, {}, edgeToken, 'list storage routes')
    assertOk(Array.isArray(routes) && routes.length > 0, `expected default storage route for var_id=${variable.var_id_text ?? variable.var_id}`)
    await api(
      edgeBase,
      `/api/v1/storage-routes/${routes[0].id}`,
      {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: true, trigger_mode: 'on_cycle', cycle_ms: 500, store_on_start: true }),
      },
      edgeToken,
      'enable storage route',
    )
  }
  const template = await api(
    edgeBase,
    '/api/v1/report-templates',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        template_code: `EB046_RPT_${id}`,
        name: `EB046 Report ${id}`,
        display_name: `EB046报表${id}`,
        file_ref: 'templates/default-report-template.xlsx',
        file_kind: 'xlsx',
        version: 1,
        enabled: true,
        params_schema: { cell_mapping: { version: 1, sheet: 'Default_Report', items: [] } },
      }),
    },
    edgeToken,
    'create report template',
  )
  const standard = await api(
    edgeBase,
    '/api/v1/detection-standards',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        standard_code: `EB046_STD_${id}`,
        name: `EB046 Standard ${id}`,
        display_name: `EB046标准${id}`,
        project_id: project.id,
        project_code: project.project_code,
        mode: 'standard',
        enabled: true,
        report_template_id: template.id,
        items: vars.map((variable, index) => ({
          var_id: variable.var_id_text ?? variable.var_id,
          var_name: variable.var_name,
          display_name: variable.display_name,
          unit: variable.unit,
          check_enabled: true,
          alarm_enabled: true,
          store_enabled: true,
          check_on_start: true,
          limit_l: index === 0 ? 0 : 30,
          limit_h: index === 0 ? 50 : 90,
          quality_policy: 'ignore_bad',
          sort_order: index + 1,
        })),
      }),
    },
    edgeToken,
    'create detection standard',
  )
  return { id, project, vars, template, standard }
}

async function loginPage(page, appBase) {
  await page.goto(`${appBase}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.locator('input').nth(0).fill(username)
  await page.locator('input').nth(1).fill(password)
  const response = page.waitForResponse((item) => item.url().includes('/api/v1/auth/login') && item.status() === 200, { timeout: 15000 })
  await page.locator('button[type="submit"]').click()
  await response
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

async function selectReportVariable(page, row, variable) {
  const item = row.locator('.ant-form-item').filter({ hasText: '本次报表变量' }).locator('.ant-select')
  const searchInput = item.locator('input').first()
  await item.click()
  await searchInput.fill(variable.var_name)
  const dropdown = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').last()
  const option = dropdown.locator('.ant-select-item-option').filter({ hasText: variable.var_name }).first()
  await option.waitFor({ state: 'visible', timeout: 5000 })
  await option.click()
  await page.keyboard.press('Escape').catch(() => {})
  await row.locator('.ant-form-item').filter({ hasText: '报表名称' }).locator('input').click()
  await page.waitForTimeout(150)
  const selectedText = await item.innerText().catch(() => '')
  assertOk(selectedText.includes(variable.var_name), `report variable selection did not stick for ${variable.var_name}: ${selectedText}`)
}

async function selectReportTemplate(page, row, template) {
  const item = row.locator('.ant-form-item').filter({ hasText: '报表模板' }).locator('.ant-select')
  await item.click()
  const dropdown = page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').last()
  const option = dropdown.locator('.ant-select-item-option').filter({ hasText: template.template_code }).first()
  await option.waitFor({ state: 'visible', timeout: 5000 })
  await option.click()
  await page.keyboard.press('Escape').catch(() => {})
  await row.locator('.ant-form-item').filter({ hasText: '报表名称' }).locator('input').click()
  await page.waitForTimeout(150)
  const selectedText = await item.innerText().catch(() => '')
  assertOk(selectedText.includes(template.template_code), `report template selection did not stick for ${template.template_code}: ${selectedText}`)
}

async function fillReportRow(page, modal, index, reportName, variable, template, paramsJSON) {
  const row = modal.locator('.station-run-report-row').nth(index)
  await selectReportTemplate(page, row, template)
  await row.locator('.ant-form-item').filter({ hasText: '报表名称' }).locator('input').fill(reportName)
  await selectReportVariable(page, row, variable)
  await row.locator('.ant-form-item').filter({ hasText: '报表 params JSON' }).locator('textarea').fill(paramsJSON)
}

async function startRunFromStationPage(page, appBase, fixture, mode) {
  const testNo = `EB046-${mode}-${fixture.id}`
  await page.goto(`${appBase}/#/station?project_id=${fixture.project.id}&edge_instance_id=${edgeInstanceID}`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.getByRole('button', { name: /开始检测|Start run|試験開始/ }).click({ timeout: 15000 })
  const modal = page.locator('.station-run-modal')
  await modal.waitFor({ state: 'visible', timeout: 10000 })
  await modal.locator('.station-run-report-row').first().waitFor({ state: 'visible', timeout: 10000 })
  await modal.getByLabel('任务编号').fill(testNo)
  await fillReportRow(page, modal, 0, `EB046 ${mode} report A`, fixture.vars[0], fixture.template, '{"operator_note":"EB046 smoke A"}')
  await modal.getByRole('button', { name: /添加报表|Add report|帳票を追加/ }).click()
  await modal.locator('.station-run-report-row').nth(1).waitFor({ state: 'visible', timeout: 5000 })
  await fillReportRow(page, modal, 1, `EB046 ${mode} report B`, fixture.vars[1], fixture.template, '{"operator_note":"EB046 smoke B"}')
  const reportRowsText = await modal.locator('.station-run-report-row').allInnerTexts()
  assertOk(reportRowsText.length >= 2, `expected two report rows before submit: ${JSON.stringify(reportRowsText)}`)
  assertOk(reportRowsText[0].includes(fixture.vars[0].var_name), `first report row missing first variable: ${JSON.stringify(reportRowsText)}`)
  assertOk(!reportRowsText[0].includes(fixture.vars[1].var_name), `first report row unexpectedly contains second variable: ${JSON.stringify(reportRowsText)}`)
  assertOk(reportRowsText[1].includes(fixture.vars[1].var_name), `second report row missing second variable: ${JSON.stringify(reportRowsText)}`)
  assertOk(reportRowsText.every((text) => text.includes(fixture.template.template_code)), `report rows missing smoke template: ${JSON.stringify(reportRowsText)}`)
  await page.screenshot({ path: path.join(screenshotDir, `${mode}-station-report-request-modal.png`), fullPage: true }).catch(() => {})
  const startResponse = page.waitForResponse(
    (response) => response.url().includes('/api/v1/detection-runs') && response.request().method() === 'POST',
    { timeout: 20000 },
  )
  await modal.getByRole('button', { name: /开始检测|Start run|試験開始/ }).click()
  const response = await startResponse
  const responseBody = await response.json()
  const run = responseBody?.result ?? responseBody
  assertOk(run?.id && run?.test_no === testNo, `unexpected start run response: ${JSON.stringify(run)}`)
  assertOk((run.report_requests ?? []).length >= 2, `start response should include at least two report requests: ${JSON.stringify(responseBody)}`)
  return run
}

async function writeVariablesWS(edgeToken, projectID, values) {
  const url = new URL('/api/v1/ws', edgeBase)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.searchParams.set('access_token', edgeToken)
  url.searchParams.set('topic', 'realtime.variables')
  url.searchParams.set('project_id', String(projectID))
  const ws = new WebSocket(url)
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout waiting ws open')), 10000)
    ws.addEventListener('open', () => {
      clearTimeout(timer)
      resolve()
    })
    ws.addEventListener('error', () => {
      clearTimeout(timer)
      reject(new Error('websocket open failed'))
    })
  })
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout waiting connection.ready')), 10000)
    ws.addEventListener('message', (event) => {
      const msg = JSON.parse(event.data)
      if (msg.type === 'connection.ready') {
        clearTimeout(timer)
        resolve()
      }
    })
  })
  for (const [varID, value] of Object.entries(values)) {
    const commandID = `eb046-${varID}-${Date.now()}`
    ws.send(JSON.stringify({
      type: 'command.write_variable',
      request_id: `req-${commandID}`,
      command_id: commandID,
      payload: { var_id: varID, value, trigger: false },
    }))
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`timeout waiting write ack ${varID}`)), 10000)
      const listener = (event) => {
        const msg = JSON.parse(event.data)
        if (msg.command_id !== commandID) return
        clearTimeout(timer)
        ws.removeEventListener('message', listener)
        if (msg.error) reject(new Error(`write ${varID} failed: ${msg.error.code} ${msg.error.message}`))
        else if (msg.type !== 'command.ack') reject(new Error(`write ${varID} returned ${msg.type}`))
        else resolve()
      }
      ws.addEventListener('message', listener)
    })
  }
  ws.close()
}

async function waitForHistory(edgeToken, taskID, projectID, expectedVarIDs) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const body = await api(edgeBase, `/api/v1/history/data?task_id=${taskID}&project_id=${projectID}&limit=100`, {}, edgeToken, 'history data')
    const items = body?.items ?? []
    const seen = new Set(items.map((item) => String(item.var_id_text ?? item.var_id)))
    if (expectedVarIDs.every((id) => seen.has(String(id)))) return items
    await sleep(500)
  }
  throw new Error(`history rows not found for task=${taskID} vars=${expectedVarIDs.join(',')}`)
}

async function waitForMainReportSuccess(mainToken, taskID, expectedJobs) {
  let enqueueResult
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const readiness = await api(mainBase, `/api/v1/main-server/report-readiness?task_id=${taskID}&edge_instance_id=${edgeInstanceID}`, {}, mainToken, 'main report readiness')
    if (readiness?.readiness?.overall_status === 'ready') break
    await sleep(500)
  }
  enqueueResult = await api(
    mainBase,
    '/api/v1/main-server/report-jobs/enqueue',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ task_id: taskID, edge_instance_id: edgeInstanceID, force: true }),
    },
    mainToken,
    'enqueue report jobs',
  )
  assertOk((enqueueResult.jobs ?? []).length >= expectedJobs, `expected at least ${expectedJobs} enqueued jobs: ${JSON.stringify(enqueueResult)}`)
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const list = await api(mainBase, `/api/v1/main-server/report-jobs?task_id=${taskID}&edge_instance_id=${edgeInstanceID}&limit=20`, {}, mainToken, 'list report jobs')
    const jobs = list.items ?? []
    const success = jobs.filter((job) => job.status === 'success' && job.artifact_name)
    if (success.length >= expectedJobs) return { enqueueResult, jobs, success }
    await sleep(1000)
  }
  throw new Error(`report jobs did not reach success for task=${taskID}`)
}

async function verifyArtifacts(mainToken, jobs) {
  const artifacts = []
  for (const job of jobs) {
    const response = await fetch(new URL(`/api/v1/main-server/report-jobs/${job.id}/artifact`, mainBase), {
      headers: { Authorization: `Bearer ${mainToken}` },
    })
    assertOk(response.ok, `artifact download failed job=${job.id} status=${response.status}`)
    const bytes = await response.arrayBuffer()
    assertOk(bytes.byteLength > 1000, `artifact too small for job=${job.id}: ${bytes.byteLength}`)
    artifacts.push({ job_id: job.id, artifact_name: job.artifact_name, bytes: bytes.byteLength })
  }
  return artifacts
}

async function verifyReportsPage(page, taskID, expectedJobs) {
  await page.goto(`${mainAppBase}/#/reports`, { waitUntil: 'domcontentloaded' })
  await page.waitForLoadState('networkidle', { timeout: 12000 }).catch(() => {})
  await page.locator('.report-toolbar .ant-input-number-input').fill(String(taskID))
  await page.waitForTimeout(1800)
  const bodyText = await page.locator('body').innerText()
  await page.screenshot({ path: path.join(screenshotDir, `main-reports-task-${taskID}.png`), fullPage: true }).catch(() => {})
  assertOk(bodyText.includes(String(taskID)), `reports page did not show task_id=${taskID}`)
  assertOk((bodyText.match(/success|成功|成功/g) ?? []).length >= expectedJobs || bodyText.includes('生成完成'), 'reports page did not show successful jobs')
}

function attachBrowserEvidence(page, mode) {
  page.on('request', (request) => {
    const rawURL = request.url()
    if (!isBusinessAPI(rawURL) && !isEdgeURL(rawURL)) return
    const entry = { mode, method: request.method(), url: rawURL }
    evidence.browser_requests.push(entry)
    if (mode === 'main_server' && isEdgeURL(rawURL)) evidence.direct_edge_browser_requests_in_main_server.push(entry)
    if (mode === 'main_server' && isBusinessAPI(rawURL)) {
      const host = hostOf(rawURL)
      if (host !== new URL(mainBase).host) evidence.main_server_non_main_api.push(entry)
    }
    if (mode === 'edge' && isBusinessAPI(rawURL) && !isExpectedEdgeHost(rawURL)) {
      evidence.edge_browser_non_edge_api.push(entry)
    }
  })
  page.on('response', async (response) => {
    const rawURL = response.url()
    if (!isBusinessAPI(rawURL)) return
    evidence.browser_responses.push({ mode, status: response.status(), url: rawURL })
  })
  page.on('websocket', (ws) => {
    const entry = { mode, url: ws.url() }
    evidence.browser_websockets.push(entry)
    if (mode === 'main_server' && isEdgeURL(ws.url())) evidence.direct_edge_browser_requests_in_main_server.push({ mode, method: 'WS', url: ws.url() })
    if (mode === 'main_server' && isBusinessAPI(ws.url()) && hostOf(ws.url()) !== new URL(mainBase).host) {
      evidence.main_server_non_main_api.push({ mode, method: 'WS', url: ws.url() })
    }
    if (mode === 'edge' && isBusinessAPI(ws.url()) && !isExpectedEdgeHost(ws.url())) {
      evidence.edge_browser_non_edge_api.push({ mode, method: 'WS', url: ws.url() })
    }
  })
  page.on('console', (message) => {
    if (message.type() === 'error') evidence.console_errors.push({ mode, text: message.text() })
  })
  page.on('pageerror', (error) => evidence.page_errors.push({ mode, message: error.message }))
}

await fs.mkdir(screenshotDir, { recursive: true })
await ensureReachable(edgeAppBase, 'edge renderer')
await ensureReachable(mainAppBase, 'main renderer')
await ensureReachable(`${edgeBase}/health`, 'edge backend')
await ensureReachable(`${mainBase}/health`, 'main backend')

const edgeToken = await loginAPI(edgeBase)
const mainToken = await loginAPI(mainBase)
const fixture = await createFixture(edgeToken)
evidence.setup = {
  project: { id: fixture.project.id, project_code: fixture.project.project_code, edge_instance_id: fixture.project.edge_instance_id },
  vars: fixture.vars.map((variable) => ({ var_id: variable.var_id_text ?? variable.var_id, var_name: variable.var_name })),
  template: { id: fixture.template.id, template_code: fixture.template.template_code },
  standard: { id: fixture.standard.id, standard_code: fixture.standard.standard_code },
}

const browser = await chromium.launch({ headless: true })
const edgePage = await browser.newPage({ viewport: { width: 1440, height: 950 } })
const mainPage = await browser.newPage({ viewport: { width: 1440, height: 950 } })
const startedRunIDs = new Set()
attachBrowserEvidence(edgePage, 'edge')
attachBrowserEvidence(mainPage, 'main_server')

try {
  await loginPage(edgePage, edgeAppBase)
  const edgeRun = await startRunFromStationPage(edgePage, edgeAppBase, fixture, 'edge')
  startedRunIDs.add(edgeRun.id)
  await writeVariablesWS(edgeToken, fixture.project.id, {
    [fixture.vars[0].var_id_text ?? fixture.vars[0].var_id]: 21.5,
    [fixture.vars[1].var_id_text ?? fixture.vars[1].var_id]: 66.5,
  })
  await sleep(waitMs)
  await waitForHistory(edgeToken, edgeRun.id, fixture.project.id, fixture.vars.map((variable) => variable.var_id_text ?? variable.var_id))
  await api(edgeBase, `/api/v1/detection-runs/${edgeRun.id}/stop`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason: 'EB046 edge smoke finished' }),
  }, edgeToken, 'stop edge run')
  startedRunIDs.delete(edgeRun.id)
  const edgeRequests = await api(edgeBase, `/api/v1/detection-runs/${edgeRun.id}/report-requests`, {}, edgeToken, 'edge report requests')
  assertOk((edgeRequests.items ?? []).length >= 2, `edge run should freeze at least two report requests: ${JSON.stringify(edgeRequests)}`)
  assertOk(
    edgeRequests.items.every((item) => item.template_code === fixture.template.template_code && Number(item.template_id) === Number(fixture.template.id)),
    `edge report requests did not use smoke template ${fixture.template.template_code}: ${JSON.stringify(edgeRequests.items)}`,
  )
  const edgeReport = await waitForMainReportSuccess(mainToken, edgeRun.id, 2)
  const edgeArtifacts = await verifyArtifacts(mainToken, edgeReport.success.slice(0, 2))
  evidence.runs.push({ mode: 'edge', run: edgeRun, report_requests: edgeRequests.items, jobs: edgeReport.jobs, artifacts: edgeArtifacts })

  await loginPage(mainPage, mainAppBase)
  const mainRun = await startRunFromStationPage(mainPage, mainAppBase, fixture, 'main')
  startedRunIDs.add(mainRun.id)
  await writeVariablesWS(edgeToken, fixture.project.id, {
    [fixture.vars[0].var_id_text ?? fixture.vars[0].var_id]: 22.5,
    [fixture.vars[1].var_id_text ?? fixture.vars[1].var_id]: 67.5,
  })
  await sleep(waitMs)
  await waitForHistory(edgeToken, mainRun.id, fixture.project.id, fixture.vars.map((variable) => variable.var_id_text ?? variable.var_id))
  await api(edgeBase, `/api/v1/detection-runs/${mainRun.id}/stop`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason: 'EB046 main_server smoke finished' }),
  }, edgeToken, 'stop main run')
  startedRunIDs.delete(mainRun.id)
  const mainRequests = await api(mainBase, `/api/v1/detection-runs/${mainRun.id}/report-requests?edge_instance_id=${edgeInstanceID}`, {}, mainToken, 'main report requests')
  assertOk((mainRequests.items ?? []).length >= 2, `main run should expose at least two report requests: ${JSON.stringify(mainRequests)}`)
  assertOk(
    mainRequests.items.every((item) => item.template_code === fixture.template.template_code && Number(item.template_id) === Number(fixture.template.id)),
    `main report requests did not use smoke template ${fixture.template.template_code}: ${JSON.stringify(mainRequests.items)}`,
  )
  const mainReport = await waitForMainReportSuccess(mainToken, mainRun.id, 2)
  const mainArtifacts = await verifyArtifacts(mainToken, mainReport.success.slice(0, 2))
  await verifyReportsPage(mainPage, mainRun.id, 2)
  evidence.runs.push({ mode: 'main_server', run: mainRun, report_requests: mainRequests.items, jobs: mainReport.jobs, artifacts: mainArtifacts })

  evidence.assertions = {
    edge_page_created_two_report_requests: evidence.runs.find((run) => run.mode === 'edge')?.report_requests?.length >= 2,
    main_page_created_two_report_requests: evidence.runs.find((run) => run.mode === 'main_server')?.report_requests?.length >= 2,
    report_requests_used_smoke_template: evidence.runs.every((run) =>
      run.report_requests.every((item) => item.template_code === fixture.template.template_code && Number(item.template_id) === Number(fixture.template.id)),
    ),
    main_report_jobs_success: evidence.runs.every((run) => (run.jobs ?? []).filter((job) => job.status === 'success').length >= 2),
    artifacts_downloaded: evidence.runs.every((run) => (run.artifacts ?? []).length >= 2 && run.artifacts.every((artifact) => artifact.bytes > 1000)),
    edge_browser_api_only_18080: evidence.edge_browser_non_edge_api.length === 0,
    main_server_browser_did_not_direct_edge: evidence.direct_edge_browser_requests_in_main_server.length === 0,
    main_server_browser_api_only_19080: evidence.main_server_non_main_api.length === 0,
    no_page_errors: evidence.page_errors.length === 0,
  }
  const failed = Object.entries(evidence.assertions).filter(([, ok]) => !ok)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8')
  const summary = {
    ok: failed.length === 0,
    evidencePath,
    screenshotDir,
    assertions: evidence.assertions,
    runs: evidence.runs.map((run) => ({
      mode: run.mode,
      task_id: run.run.id,
      test_no: run.run.test_no,
      report_requests: run.report_requests.length,
      success_jobs: run.jobs.filter((job) => job.status === 'success').length,
      artifacts: run.artifacts.length,
    })),
    main_server_direct_edge_browser_requests: evidence.direct_edge_browser_requests_in_main_server.length,
  }
  console.log(JSON.stringify(summary, null, 2))
  if (failed.length) throw new Error(`EB046 smoke failed: ${failed.map(([name]) => name).join(', ')}`)
} catch (error) {
  evidence.error = error?.message || String(error)
  evidence.completed_at = new Date().toISOString()
  await fs.writeFile(evidencePath, JSON.stringify(evidence, null, 2), 'utf8').catch(() => {})
  console.error(JSON.stringify({ ok: false, evidencePath, screenshotDir, error: evidence.error, assertions: evidence.assertions }, null, 2))
  throw error
} finally {
  for (const runID of startedRunIDs) {
    await api(edgeBase, `/api/v1/detection-runs/${runID}/stop`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason: 'EB046 smoke cleanup after failure' }),
    }, edgeToken, `cleanup run ${runID}`).catch((error) => {
      evidence.cleanup_error = `${evidence.cleanup_error ? `${evidence.cleanup_error}; ` : ''}${error.message}`
    })
  }
  await browser.close()
}
