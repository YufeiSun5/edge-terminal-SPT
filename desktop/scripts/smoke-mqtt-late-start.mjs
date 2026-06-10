import fs from 'node:fs/promises'
import path from 'node:path'

const edgeBase = process.env.EDGE_BASE || 'http://127.0.0.1:18080'
const gatewayID = Number(process.env.MQTT_SMOKE_GATEWAY_ID || 1)
const unavailableBroker = process.env.MQTT_SMOKE_UNAVAILABLE_BROKER || 'tcp://127.0.0.1:1884'
const projectID = process.env.PROJECT_ID || '1'
const projectCode = process.env.PROJECT_CODE || 'AC-01'
const targetVarName = process.env.MQTT_SMOKE_WRITE_VAR || 'SP2_WD'
const publishRawName = process.env.MQTT_SMOKE_PUBLISH_RAW_NAME || '台1_39'
const publishVarName = process.env.MQTT_SMOKE_PUBLISH_VAR || 'SP1_WD'
const publishValue = Number(process.env.MQTT_SMOKE_PUBLISH_VALUE || 42.42)
const serviceToken = process.env.EDGE_MAIN_SERVICE_TOKEN || 'edge-main-dev-token-20260603'
const username = process.env.SMOKE_USERNAME || 'admin'
const password = process.env.SMOKE_PASSWORD || 'Admin@12345'
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
const outDir = path.resolve('output/playwright')
const evidencePath = path.join(outDir, `mqtt-late-start-smoke-${stamp}.json`)

const evidence = {
  started_at: new Date().toISOString(),
  scope:
    'controlled MQTT gateway late-start regression: force broker unavailable, verify gateway_offline write result, restore broker, verify subscription and realtime last_update via datachange publish',
  edge_base: edgeBase,
  gateway_id: gatewayID,
  unavailable_broker: unavailableBroker,
  project_id: projectID,
  project_code: projectCode,
  target_write_var: targetVarName,
  publish_var: publishVarName,
  publish_raw_name: publishRawName,
  checks: [],
  assertions: {},
}

function record(name, ok, detail = {}) {
  evidence.checks.push({ name, ok, ...detail })
  evidence.assertions[name] = Boolean(ok)
}

function assertOk(condition, message) {
  if (!condition) throw new Error(message)
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function jsonRequest(method, url, { token, body, allowStatuses = [200], label = url } = {}) {
  const response = await fetch(url, {
    method,
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
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
  if (!allowStatuses.includes(response.status)) {
    throw new Error(`${label} returned ${response.status}: ${text.slice(0, 800)}`)
  }
  return { status: response.status, payload, text }
}

async function login() {
  const result = await jsonRequest('POST', `${edgeBase}/api/v1/auth/login`, {
    body: { username, password },
    label: 'edge login',
  })
  assertOk(result.payload?.access_token, 'edge login did not return access_token')
  return result.payload.access_token
}

async function gatewayStatus() {
  const result = await jsonRequest('GET', `${edgeBase}/api/v1/edge-control/gateways`, {
    token: serviceToken,
    label: 'edge-control gateways',
  })
  return result.payload?.[String(gatewayID)]
}

async function waitForGateway(match, label, timeoutMS = 45000) {
  const deadline = Date.now() + timeoutMS
  let last = null
  while (Date.now() < deadline) {
    last = await gatewayStatus()
    if (last && match(last)) return last
    await sleep(1000)
  }
  throw new Error(`${label} timed out; last=${JSON.stringify(last)}`)
}

async function patchGateway(token, patch, label) {
  return await jsonRequest('PATCH', `${edgeBase}/api/v1/gateway-configs/${gatewayID}`, {
    token,
    body: patch,
    label,
  })
}

async function listRealtime(token) {
  const result = await jsonRequest(
    'GET',
    `${edgeBase}/api/v1/realtime/variables?project_id=${encodeURIComponent(projectID)}`,
    { token, label: 'realtime variables' },
  )
  return Array.isArray(result.payload) ? result.payload : []
}

function findSnapshot(items, varName) {
  return items.find((item) => item.var_name === varName || item.display_name === varName)
}

async function waitForRealtimeUpdate(token, previousLastUpdate, timeoutMS = 20000) {
  const deadline = Date.now() + timeoutMS
  let lastSnapshot = null
  while (Date.now() < deadline) {
    const items = await listRealtime(token)
    lastSnapshot = findSnapshot(items, publishVarName)
    const lastUpdate = lastSnapshot?.last_update || ''
    if (lastUpdate && lastUpdate !== previousLastUpdate) {
      return lastSnapshot
    }
    await sleep(500)
  }
  throw new Error(
    `realtime ${publishVarName} last_update did not change after publish; previous=${previousLastUpdate}; last=${JSON.stringify(
      lastSnapshot,
    )}`,
  )
}

let adminToken = ''
let originalConfig = null
let restored = false

try {
  await fs.mkdir(outDir, { recursive: true })
  await jsonRequest('GET', `${edgeBase}/health`, { label: 'edge health' })
  record('edge_health_reachable', true)

  adminToken = await login()
  record('edge_admin_login', true)

  const originalConfigResp = await jsonRequest('GET', `${edgeBase}/api/v1/gateway-configs/${gatewayID}`, {
    token: adminToken,
    label: 'gateway original config',
  })
  originalConfig = originalConfigResp.payload
  evidence.original_gateway = {
    broker: originalConfig.broker,
    topic: originalConfig.topic,
    write_result_topic: originalConfig.write_result_topic,
    query_all_topic: originalConfig.query_all_topic,
    enabled: originalConfig.enabled,
  }
  assertOk(originalConfig?.broker, 'gateway original config missing broker')
  record('gateway_config_captured', true, evidence.original_gateway)

  const variablesResp = await jsonRequest(
    'GET',
    `${edgeBase}/api/v1/variables?project_id=${encodeURIComponent(
      projectID,
    )}&var_group=${encodeURIComponent('KIO变量')}&enabled=true&writable=true`,
    { token: adminToken, label: 'writable KIO variables' },
  )
  const writableVars = Array.isArray(variablesResp.payload) ? variablesResp.payload : []
  const targetVar = writableVars.find((item) => item.var_name === targetVarName)
  assertOk(targetVar, `writable variable ${targetVarName} was not found`)
  record('writable_target_found', true, {
    var_id_text: targetVar.var_id_text,
    write_path: targetVar.write_path,
  })

  const beforeSnapshots = await listRealtime(adminToken)
  const beforePublish = findSnapshot(beforeSnapshots, publishVarName)
  const previousLastUpdate = beforePublish?.last_update || ''
  evidence.realtime_before_publish = beforePublish || null

  await patchGateway(
    adminToken,
    {
      broker: unavailableBroker,
      enabled: true,
    },
    'force gateway broker unavailable',
  )
  record('gateway_patched_to_unavailable_broker', true, { broker: unavailableBroker })

  const offlineStatus = await waitForGateway(
    (status) => status.broker === unavailableBroker && status.active === false,
    'gateway offline after broker patch',
  )
  record('gateway_offline_observed', true, {
    active: offlineStatus.active,
    broker: offlineStatus.broker,
    last_error: offlineStatus.last_error,
  })

  const commandID = `mqtt-late-start-${stamp}`
  const writeResp = await jsonRequest('POST', `${edgeBase}/api/v1/edge-control/variables/write`, {
    token: serviceToken,
    allowStatuses: [502],
    label: 'offline variable write',
    body: {
      command_id: commandID,
      operator_username: username,
      reason: 'test-ai mqtt late-start offline write smoke',
      payload: {
        project_code: projectCode,
        var_name: targetVarName,
        value: 0,
        wait_ack: false,
      },
    },
  })
  const offlineKIO = writeResp.payload?.result?.kio
  assertOk(writeResp.payload?.ok === false, 'offline write should return ok=false')
  assertOk(offlineKIO?.status === 'gateway_offline', `expected kio.status gateway_offline, got ${offlineKIO?.status}`)
  assertOk(offlineKIO?.broker_accepted === false, 'offline write should not be broker accepted')
  record('offline_write_structured_gateway_offline', true, {
    http_status: writeResp.status,
    error_code: writeResp.payload?.error?.code,
    kio: offlineKIO,
  })

  await patchGateway(
    adminToken,
    {
      broker: originalConfig.broker,
      client_id: originalConfig.client_id,
      username: originalConfig.username,
      topic: originalConfig.topic,
      qos: originalConfig.qos,
      parser_type: originalConfig.parser_type,
      kio_client_id: originalConfig.kio_client_id,
      kio_writer: originalConfig.kio_writer,
      kio_write_username: originalConfig.kio_write_username,
      setdata_topic: originalConfig.setdata_topic,
      write_result_topic: originalConfig.write_result_topic,
      query_all_topic: originalConfig.query_all_topic,
      enabled: originalConfig.enabled,
    },
    'restore gateway broker',
  )
  restored = true
  record('gateway_config_restore_requested', true, { broker: originalConfig.broker })

  const restoredStatus = await waitForGateway(
    (status) =>
      status.broker === originalConfig.broker &&
      status.active === true &&
      Array.isArray(status.subscribed_topics) &&
      status.subscribed_topics.includes(originalConfig.topic) &&
      status.subscribed_topics.includes(originalConfig.write_result_topic),
    'gateway restored and subscribed',
    60000,
  )
  record('gateway_restored_active_with_subscriptions', true, {
    active: restoredStatus.active,
    broker: restoredStatus.broker,
    subscribed_topics: restoredStatus.subscribed_topics,
    last_connected: restoredStatus.last_connected,
    last_full_sync: restoredStatus.last_full_sync,
  })

  const publishPayload = { Objs: [{ N: publishRawName, 1: publishValue }] }
  const publishResp = await jsonRequest('POST', `${edgeBase}/api/v1/gateways/${gatewayID}/publish`, {
    token: adminToken,
    body: {
      topic: originalConfig.topic,
      payload: publishPayload,
      qos: originalConfig.qos,
      retain: false,
    },
    label: 'publish datachange payload',
  })
  assertOk(publishResp.payload?.broker_accepted === true, 'datachange publish was not broker accepted')
  record('datachange_payload_published', true, {
    topic: originalConfig.topic,
    payload: publishPayload,
  })

  const afterSnapshot = await waitForRealtimeUpdate(adminToken, previousLastUpdate)
  evidence.realtime_after_publish = afterSnapshot
  record('realtime_last_update_changed_after_publish', true, {
    previous_last_update: previousLastUpdate,
    next_last_update: afterSnapshot.last_update,
    value: afterSnapshot.value,
  })

  evidence.finished_at = new Date().toISOString()
  evidence.ok = true
} catch (error) {
  evidence.finished_at = new Date().toISOString()
  evidence.ok = false
  evidence.error = error.stack || error.message
  throw error
} finally {
  if (originalConfig && adminToken && !restored) {
    try {
      await patchGateway(
        adminToken,
        {
          broker: originalConfig.broker,
          client_id: originalConfig.client_id,
          username: originalConfig.username,
          topic: originalConfig.topic,
          qos: originalConfig.qos,
          parser_type: originalConfig.parser_type,
          kio_client_id: originalConfig.kio_client_id,
          kio_writer: originalConfig.kio_writer,
          kio_write_username: originalConfig.kio_write_username,
          setdata_topic: originalConfig.setdata_topic,
          write_result_topic: originalConfig.write_result_topic,
          query_all_topic: originalConfig.query_all_topic,
          enabled: originalConfig.enabled,
        },
        'restore gateway broker after failure',
      )
      evidence.restore_after_failure = { ok: true, broker: originalConfig.broker }
    } catch (restoreError) {
      evidence.restore_after_failure = { ok: false, error: restoreError.message }
    }
  }
  await fs.writeFile(evidencePath, `${JSON.stringify(evidence, null, 2)}\n`, 'utf8')
  console.log(`mqtt late-start smoke evidence: ${evidencePath}`)
}
