import fs from 'node:fs'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

const apiBase = process.env.API_BASE || 'http://127.0.0.1:19080'
const edgeInstanceId = process.env.EDGE_INSTANCE_ID || 'edge-1'
const username = process.env.SMOKE_USER || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const outputDir = path.resolve('output/playwright')
const runId = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const testNo = process.env.TEST_NO || `EB050-${runId}`

function assert(condition, message, details) {
  if (!condition) {
    const suffix = details === undefined ? '' : `\n${JSON.stringify(details, null, 2)}`
    throw new Error(`${message}${suffix}`)
  }
}

async function request(method, urlPath, token, body) {
  const response = await fetch(`${apiBase}${urlPath}`, {
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

async function login() {
  const response = await request('POST', '/api/v1/auth/login', '', { username, password })
  assert(response.status === 200 && response.payload?.access_token, 'login failed', response)
  return response.payload.access_token
}

async function findIdleProject(token) {
  const projects = await request('GET', `/api/v1/projects?edge_instance_id=${encodeURIComponent(edgeInstanceId)}&limit=50`, token)
  assert(projects.status === 200 && Array.isArray(projects.payload), 'projects query failed', projects)
  for (const project of projects.payload) {
    const projectID = Number(project.id)
    if (!projectID) continue
    const current = await request('GET', `/api/v1/detection-runs/current?project_id=${projectID}&edge_instance_id=${encodeURIComponent(edgeInstanceId)}`, token)
    if (current.status === 404) return project
  }
  throw new Error(`no idle project found for ${edgeInstanceId}`)
}

function openNotificationWS(token) {
  assert(typeof WebSocket === 'function', 'Node.js runtime does not provide WebSocket')
  const url = new URL('/api/v1/ws', apiBase)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.searchParams.set('access_token', token)
  url.searchParams.set('topic', 'notifications')
  url.searchParams.set('edge_instance_id', edgeInstanceId)

  const events = []
  const socket = new WebSocket(url)
  let openedAt
  let closedAt
  let closeEvent
  socket.addEventListener('open', () => {
    openedAt = new Date().toISOString()
  })
  socket.addEventListener('message', (event) => {
    try {
      events.push(JSON.parse(String(event.data)))
    } catch {
      events.push({ type: 'unparsed', payload: String(event.data) })
    }
  })
  socket.addEventListener('close', (event) => {
    closedAt = new Date().toISOString()
    closeEvent = { code: event.code, reason: event.reason, was_clean: event.wasClean }
  })
  return {
    url: url.toString().replace(token, '<token>'),
    events,
    waitOpen: async () => {
      for (let i = 0; i < 50; i += 1) {
        if (openedAt) return
        await delay(100)
      }
      throw new Error('notification websocket did not open')
    },
    waitNotification: async () => {
      for (let i = 0; i < 120; i += 1) {
        const event = events.find((item) => item.type === 'notification.event' && item.payload?.test_no === testNo)
        if (event) return event
        await delay(250)
      }
      throw new Error('notification.event was not received for test run')
    },
    close: () => {
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) socket.close()
    },
    status: () => ({ opened_at: openedAt, closed_at: closedAt, close_event: closeEvent, event_count: events.length }),
  }
}

function detectionRunID(payload) {
  const candidates = [
    payload?.result?.id,
    payload?.result?.task?.id,
    payload?.result?.run?.id,
    payload?.result?.task_id,
    payload?.id,
    payload?.task_id,
  ]
  for (const value of candidates) {
    const parsed = Number(value)
    if (Number.isFinite(parsed) && parsed > 0) return parsed
  }
  return 0
}

async function pollNotificationCount(token, params, expectedAtLeast) {
  const query = new URLSearchParams(params)
  for (let i = 0; i < 40; i += 1) {
    const response = await request('GET', `/api/v1/notifications/unread-count?${query.toString()}`, token)
    assert(response.status === 200, 'unread count query failed', response)
    const unread = Number(response.payload?.unread ?? 0)
    if (unread >= expectedAtLeast) return unread
    await delay(250)
  }
  throw new Error(`unread count did not reach ${expectedAtLeast}`)
}

async function main() {
  fs.mkdirSync(outputDir, { recursive: true })
  const token = await login()
  const project = await findIdleProject(token)
  const projectID = Number(project.id)

  const ws = openNotificationWS(token)
  await ws.waitOpen()

  const startPayload = {
    project_id: projectID,
    test_no: testNo,
    mode: 'custom',
    duration_sec: 30,
    operator_note: 'EB-050 notification smoke',
    edge_instance_id: edgeInstanceId,
  }
  const started = await request('POST', `/api/v1/detection-runs?edge_instance_id=${encodeURIComponent(edgeInstanceId)}`, token, startPayload)
  assert(started.status === 200, 'detection start failed', started)
  const taskID = detectionRunID(started.payload)
  assert(taskID > 0, 'detection start response missing task id', started.payload)

  const notificationEvent = await ws.waitNotification()

  const stopped = await request('POST', `/api/v1/detection-runs/${taskID}/stop?edge_instance_id=${encodeURIComponent(edgeInstanceId)}`, token, { reason: 'EB-050 notification smoke stop' })
  assert(stopped.status === 200, 'detection stop failed', stopped)
  await delay(1000)

  const unreadStarted = await pollNotificationCount(token, { keyword: testNo, type: 'detection.run_started' }, 1)
  const unreadStopped = await pollNotificationCount(token, { keyword: testNo, type: 'detection.run_stopped' }, 1)
  const list = await request('GET', `/api/v1/notifications?keyword=${encodeURIComponent(testNo)}&limit=20`, token)
  assert(list.status === 200 && list.payload?.total >= 2, 'notification list should include start and stop events', list)

  const readStarted = await request('POST', `/api/v1/notifications/read-all?keyword=${encodeURIComponent(testNo)}&type=detection.run_started`, token)
  assert(readStarted.status === 200 && Number(readStarted.payload?.updated) >= 1, 'scoped read-all failed', readStarted)

  const unreadStartedAfter = await pollNotificationCount(token, { keyword: testNo, type: 'detection.run_started' }, 0)
  const stoppedAfterScopedRead = await pollNotificationCount(token, { keyword: testNo, type: 'detection.run_stopped' }, 1)
  assert(unreadStartedAfter === 0, 'started notification should be read after scoped read-all')
  assert(stoppedAfterScopedRead >= 1, 'stopped notification should remain unread after started scoped read-all')

  const readStopped = await request('POST', `/api/v1/notifications/read-all?keyword=${encodeURIComponent(testNo)}&type=detection.run_stopped`, token)
  assert(readStopped.status === 200 && Number(readStopped.payload?.updated) >= 1, 'stop scoped read-all failed', readStopped)

  ws.close()
  await delay(250)

  const evidence = {
    api_base: apiBase,
    edge_instance_id: edgeInstanceId,
    project: { id: projectID, project_code: project.project_code, name: project.name },
    test_no: testNo,
    task_id: taskID,
    websocket: ws.status(),
    notification_event: notificationEvent,
    unread_before_read: { detection_run_started: unreadStarted, detection_run_stopped: unreadStopped },
    read_started: readStarted.payload,
    read_stopped: readStopped.payload,
    list_total: list.payload.total,
  }
  const evidencePath = path.join(outputDir, `eb050-notification-smoke-${runId}.json`)
  fs.writeFileSync(evidencePath, JSON.stringify(evidence, null, 2))
  console.log(JSON.stringify({ ok: true, evidence: evidencePath, task_id: taskID, test_no: testNo }, null, 2))
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
