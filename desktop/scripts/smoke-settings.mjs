const rendererUrlCandidates = process.env.RENDERER_URL
  ? [process.env.RENDERER_URL]
  : ['http://127.0.0.1:5173', 'http://127.0.0.1:4173']
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

function varKey(value) {
  return String(value)
}

function smokeStamp() {
  const timePart = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
  const randomPart = Math.floor(Math.random() * 10000).toString().padStart(4, '0')
  return `${timePart}${randomPart}`
}

async function resolveRendererUrl() {
  for (const candidate of rendererUrlCandidates) {
    try {
      const response = await fetch(candidate)
      if (response.ok) return candidate
    } catch {
      // Try the next common Vite port.
    }
  }
  throw new Error(`renderer is not reachable at ${rendererUrlCandidates.join(' or ')}`)
}

async function apiJson(path, options = {}, token = '', label = path) {
  const result = await apiFetch(path, options, token)
  assertOk(result.response.ok, `${label} failed: ${result.response.status} ${JSON.stringify(result.body)}`)
  return result.body
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

  return { projectCount: projects.body.length, health: health.body, token }
}

async function smokeProjectBusinessFlows(token) {
  const stamp = smokeStamp()
  const projectCode = `FE-SMOKE-${stamp.slice(-10)}`
  const project = await apiJson(
    '/api/v1/projects',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        project_code: projectCode,
        site_no: 'fe-smoke',
        name: `前端 smoke ${stamp}`,
        display_name: `前端 smoke ${stamp}`,
        display_name_en: `Frontend smoke ${stamp}`,
        display_name_ja: `フロント smoke ${stamp}`,
      }),
    },
    token,
    'create project',
  )

  const assignedVarName = `fe_smoke_assigned_${stamp}`
  const unassignedVarName = `fe_smoke_unassigned_${stamp}`
  const assignedVariable = await apiJson(
    '/api/v1/variables',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        source_type: 'virtual',
        var_name: assignedVarName,
        data_type: 'FLOAT',
        display_name: `已分配 smoke ${stamp}`,
        display_name_en: `Assigned smoke ${stamp}`,
        display_name_ja: `割当 smoke ${stamp}`,
        unit: 'kPa',
        enabled: false,
      }),
    },
    token,
    'create unassigned virtual variable',
  )
  assertOk(!assignedVariable.project_id, 'new virtual variable should start unassigned for assignment smoke')

  const unassignedVariable = await apiJson(
    '/api/v1/variables',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        source_type: 'virtual',
        var_name: unassignedVarName,
        data_type: 'STRING',
        display_name: `未分配 smoke ${stamp}`,
        display_name_en: `Unassigned smoke ${stamp}`,
        display_name_ja: `未割当 smoke ${stamp}`,
        enabled: false,
      }),
    },
    token,
    'create unassigned pool variable',
  )
  assertOk(!unassignedVariable.project_id, 'unassigned pool smoke variable should remain unassigned')

  const assignment = await apiJson(
    `/api/v1/variables/${assignedVariable.var_id_text ?? assignedVariable.var_id}/assignment`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project_id: project.id, var_group: 'smoke', enabled: true }),
    },
    token,
    'assign variable to project',
  )
  assertOk(assignment.status === 'ok', `unexpected assignment response: ${JSON.stringify(assignment)}`)

  const variables = await apiJson(`/api/v1/variables?keyword=${encodeURIComponent(assignedVarName)}`, {}, token, 'list assigned variable')
  const reloadedVariable = variables.find((item) => varKey(item.var_id_text ?? item.var_id) === varKey(assignedVariable.var_id_text ?? assignedVariable.var_id))
  assertOk(reloadedVariable, 'assigned variable not found after assignment')
  assertOk(reloadedVariable.project_id === project.id, `assigned variable project mismatch: ${JSON.stringify(reloadedVariable)}`)
  assertOk(reloadedVariable.enabled === true, 'assigned variable should be enabled')

  const routesByProject = await apiJson(`/api/v1/storage-routes?project_id=${project.id}`, {}, token, 'list storage routes by project')
  assertOk(Array.isArray(routesByProject), 'storage routes response should be an array')
  assertOk(routesByProject.length > 0, 'assigned variable should create a default storage route')
  assertOk(routesByProject.every((route) => route.project_id === project.id), 'storage route project filter returned another project')
  assertOk(
    routesByProject.some((route) => varKey(route.var_id_text ?? route.var_id) === varKey(assignedVariable.var_id_text ?? assignedVariable.var_id)),
    'storage route project filter did not include assigned variable route',
  )

  const routesByVariable = await apiJson(
    `/api/v1/storage-routes?project_id=${project.id}&var_id=${assignedVariable.var_id_text ?? assignedVariable.var_id}`,
    {},
    token,
    'list storage routes by project and variable',
  )
  assertOk(routesByVariable.length > 0, 'storage route variable filter returned no routes')
  assertOk(routesByVariable.every((route) => route.project_id === project.id), 'storage route variable filter returned another project')

  const testNo = `FE-SMOKE-${stamp}`
  const run = await apiJson(
    '/api/v1/detection-runs',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        project_id: project.id,
        test_no: testNo,
        factory_no: `FACTORY-${stamp}`,
        mode: 'standard',
        duration_sec: 60,
        operator_note: 'desktop smoke project flow',
      }),
    },
    token,
    'start detection run',
  )
  assertOk(run.project_id === project.id, `started run project mismatch: ${JSON.stringify(run)}`)
  assertOk(run.status === 'running', `started run should be running: ${JSON.stringify(run)}`)

  const currentRun = await apiJson(`/api/v1/detection-runs/current?project_id=${project.id}`, {}, token, 'current detection run')
  assertOk(currentRun.id === run.id, `current run mismatch: ${JSON.stringify(currentRun)}`)

  const stoppedRun = await apiJson(
    `/api/v1/detection-runs/${run.id}/stop`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason: 'desktop smoke finished' }),
    },
    token,
    'stop detection run',
  )
  assertOk(stoppedRun.status === 'stopped', `stopped run should be stopped: ${JSON.stringify(stoppedRun)}`)

  const runs = await apiJson(`/api/v1/detection-runs?project_id=${project.id}&limit=5`, {}, token, 'list detection runs by project')
  assertOk(Array.isArray(runs.items), 'detection run list should include items')
  assertOk(runs.items.some((item) => item.id === run.id), 'project-filtered detection run list missing smoke run')

  return {
    project,
    assignedVarName,
    unassignedVarName,
    assignedVarId: assignedVariable.var_id_text ?? assignedVariable.var_id,
    unassignedVarId: unassignedVariable.var_id_text ?? unassignedVariable.var_id,
    routeCount: routesByProject.length,
    runId: run.id,
  }
}

async function clickSummaryButton(page, text) {
  await page.locator('button').filter({ hasText: text }).last().click()
}

async function loginPage(page, rendererUrl) {
  await page.goto(`${rendererUrl}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1000)
  if (!page.url().includes('/login')) return
  await page.getByLabel('账号', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  const response = page.waitForResponse((item) => item.url().includes('/api/v1/auth/login') && item.status() === 200, { timeout: 15000 })
  await page.getByRole('button', { name: /登录|Login|ログイン/ }).click()
  await response
  await page.waitForFunction(() => !window.location.hash.includes('/login'), null, { timeout: 15000 })
}

async function smokeSettingsUI(rendererUrl, businessFlow) {
  const { chromium } = await import('playwright')
  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage()
  try {
    await loginPage(page, rendererUrl)

    await page.goto(`${rendererUrl}/#/variables`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(1500)

    await page.getByText('全部变量').first().waitFor({ timeout: 10000 })
    await page.getByRole('button', { name: '创建虚变量' }).waitFor({ timeout: 10000 })
    await page.getByPlaceholder(/搜索变量|Search/).fill(businessFlow.assignedVarName)
    await page.getByText(businessFlow.assignedVarName).first().waitFor({ timeout: 10000 })

    await page.getByRole('button', { name: '创建虚变量' }).click()
    await page.locator('.ant-modal').filter({ hasText: '创建虚变量' }).waitFor({ timeout: 10000 })
    await page.getByLabel('选择项目').waitFor({ timeout: 10000 })
    await page.getByLabel('英文显示名').waitFor({ timeout: 10000 })
    await page.getByLabel('日文显示名').waitFor({ timeout: 10000 })
    await page.keyboard.press('Escape')
    await page.locator('.ant-modal').filter({ hasText: '创建虚变量' }).waitFor({ state: 'hidden', timeout: 10000 })

    await page.getByPlaceholder(/搜索变量|Search/).fill(businessFlow.unassignedVarName)
    await page.getByText('未分配变量池').first().waitFor({ timeout: 10000 })
    await page.getByText(businessFlow.unassignedVarName).first().waitFor({ timeout: 10000 })
    await page.locator('.variable-config-unassigned').locator('button').filter({ hasText: '全选' }).click()
    await page.locator('button').filter({ hasText: '批量分配 1 个' }).waitFor({ timeout: 10000 })

    await page.goto(`${rendererUrl}/#/settings`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(1500)
    await clickSummaryButton(page, '存储路由')
    await page.getByPlaceholder('搜索路由、变量、表列或项目').fill(businessFlow.assignedVarName)
    await page.getByText(businessFlow.assignedVarName).first().waitFor({ timeout: 10000 })

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
  const rendererUrl = await resolveRendererUrl()
  const backend = await smokeBackend()
  const businessFlow = await smokeProjectBusinessFlows(backend.token)
  await smokeSettingsUI(rendererUrl, businessFlow)
  console.log(
    `settings smoke passed: projects=${backend.projectCount}, tags=${backend.health.tags ?? 0}, ` +
    `smokeProject=${businessFlow.project.project_code}, routes=${businessFlow.routeCount}, run=${businessFlow.runId}`,
  )
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
