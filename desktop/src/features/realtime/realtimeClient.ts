import { fetchEventSource } from '@microsoft/fetch-event-source'
import { getAccessToken } from '@/shared/auth/tokenStore'
import { env } from '@/shared/config/env'
import type { RealtimeWebSocketCommand, RealtimeWebSocketEnvelope, RealtimeWebSocketSubscription } from '@/shared/api/types'

type RealtimeEventHandlers<TPayload> = {
  onMessage: (payload: TPayload) => void
  onError?: (error: unknown) => void
  signal?: AbortSignal
}

export function subscribeSse<TPayload>(
  path: string,
  { onMessage, onError, signal }: RealtimeEventHandlers<TPayload>,
) {
  const token = getAccessToken()

  return fetchEventSource(`${env.apiBaseUrl}${path}`, {
    signal,
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    onmessage(message) {
      if (!message.data) {
        return
      }
      onMessage(JSON.parse(message.data) as TPayload)
    },
    onerror(error) {
      onError?.(error)
      throw error
    },
  })
}

type RealtimeWebSocketHandlers = {
  onMessage: (message: RealtimeWebSocketEnvelope) => void
  onStatus?: (status: RealtimeWebSocketStatus) => void
  onClose?: () => void
  onError?: (error: Event | Error) => void
  subscription?: RealtimeWebSocketSubscription
}

export type RealtimeWebSocketStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

type SharedRealtimeWebSocket = {
  key: string
  subscription?: RealtimeWebSocketSubscription
  socket?: WebSocket
  handlers: Set<RealtimeWebSocketHandlers>
  reconnectAttempt: number
  reconnectTimer?: number
  reconnecting: boolean
}

const sharedRealtimeSockets = new Map<string, SharedRealtimeWebSocket>()
const reconnectBackoffMs = [500, 1000, 2000, 5000, 10000, 30000]
let onlineListenerInstalled = false

export class RealtimeWebSocketCommandError extends Error {
  code?: string
  result?: unknown

  constructor(message: string, code?: string, result?: unknown) {
    super(message)
    this.name = 'RealtimeWebSocketCommandError'
    this.code = code
    this.result = result
  }
}

export function subscribeRealtimeWebSocket({ onMessage, onStatus, onClose, onError, subscription }: RealtimeWebSocketHandlers) {
  installOnlineReconnectListener()

  const key = buildRealtimeWebSocketKey(subscription)
  const handler: RealtimeWebSocketHandlers = { onMessage, onStatus, onClose, onError, subscription }
  const existing = sharedRealtimeSockets.get(key)
  if (existing) {
    existing.handlers.add(handler)
    handler.onStatus?.(existing.socket?.readyState === WebSocket.OPEN ? 'open' : existing.reconnecting ? 'reconnecting' : 'connecting')
    return () => {
      releaseRealtimeHandler(existing, handler)
    }
  }

  const shared: SharedRealtimeWebSocket = {
    key,
    subscription,
    handlers: new Set([handler]),
    reconnectAttempt: 0,
    reconnecting: false,
  }
  sharedRealtimeSockets.set(key, shared)
  connectSharedRealtimeSocket(shared, false)

  return () => {
    releaseRealtimeHandler(shared, handler)
  }
}

function connectSharedRealtimeSocket(shared: SharedRealtimeWebSocket, reconnecting: boolean) {
  if (shared.handlers.size === 0) return
  clearReconnectTimer(shared)
  shared.reconnecting = reconnecting
  notifyRealtimeStatus(shared, reconnecting ? 'reconnecting' : 'connecting')
  const socket = new WebSocket(buildRealtimeWebSocketURL(shared.subscription))
  shared.socket = socket
  socket.addEventListener('open', () => {
    if (shared.handlers.size === 0) {
      socket.close()
      return
    }
    shared.reconnectAttempt = 0
    shared.reconnecting = false
    notifyRealtimeStatus(shared, 'open')
  })
  socket.addEventListener('message', (event) => {
    try {
      const message = JSON.parse(String(event.data)) as RealtimeWebSocketEnvelope
      shared.handlers.forEach((item) => item.onMessage(message))
    } catch (error) {
      const messageError = error instanceof Error ? error : new Error('invalid websocket message')
      shared.handlers.forEach((item) => item.onError?.(messageError))
    }
  })
  socket.addEventListener('error', (event) => {
    shared.handlers.forEach((item) => item.onError?.(event))
  })
  socket.addEventListener('close', () => {
    if (shared.socket !== socket) return
    shared.socket = undefined
    shared.handlers.forEach((item) => item.onClose?.())
    scheduleRealtimeReconnect(shared)
  })
}

function scheduleRealtimeReconnect(shared: SharedRealtimeWebSocket, immediate = false) {
  if (shared.handlers.size === 0) {
    sharedRealtimeSockets.delete(shared.key)
    return
  }
  if (shared.reconnectTimer !== undefined) return
  const baseDelay = reconnectBackoffMs[Math.min(shared.reconnectAttempt, reconnectBackoffMs.length - 1)]
  const jitter = immediate ? 0 : Math.floor(Math.random() * Math.min(250, baseDelay / 2))
  const delay = immediate ? 0 : baseDelay + jitter
  shared.reconnectAttempt += 1
  shared.reconnecting = true
  notifyRealtimeStatus(shared, 'reconnecting')
  shared.reconnectTimer = window.setTimeout(() => {
    shared.reconnectTimer = undefined
    connectSharedRealtimeSocket(shared, true)
  }, delay)
}

function releaseRealtimeHandler(shared: SharedRealtimeWebSocket, handler: RealtimeWebSocketHandlers) {
  shared.handlers.delete(handler)
  if (shared.handlers.size > 0) return
  clearReconnectTimer(shared)
  sharedRealtimeSockets.delete(shared.key)
  notifyRealtimeStatus(shared, 'closed')
  const socket = shared.socket
  shared.socket = undefined
  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    socket.close()
  }
}

function clearReconnectTimer(shared: SharedRealtimeWebSocket) {
  if (shared.reconnectTimer === undefined) return
  window.clearTimeout(shared.reconnectTimer)
  shared.reconnectTimer = undefined
}

function notifyRealtimeStatus(shared: SharedRealtimeWebSocket, status: RealtimeWebSocketStatus) {
  shared.handlers.forEach((item) => item.onStatus?.(status))
}

function installOnlineReconnectListener() {
  if (onlineListenerInstalled || typeof window === 'undefined') return
  onlineListenerInstalled = true
  window.addEventListener('online', () => {
    sharedRealtimeSockets.forEach((shared) => {
      if (shared.socket && (shared.socket.readyState === WebSocket.OPEN || shared.socket.readyState === WebSocket.CONNECTING)) {
        return
      }
      clearReconnectTimer(shared)
      scheduleRealtimeReconnect(shared, true)
    })
  })
}

function buildRealtimeWebSocketURL(subscription?: RealtimeWebSocketSubscription) {
  const token = getAccessToken()
  const url = new URL('/api/v1/ws', env.apiBaseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) url.searchParams.set('access_token', token)
  appendRealtimeSubscriptionParams(url, subscription)
  return url
}

function buildRealtimeWebSocketKey(subscription?: RealtimeWebSocketSubscription) {
  const url = new URL('/api/v1/ws', env.apiBaseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  appendRealtimeSubscriptionParams(url, subscription)
  return url.toString()
}

function appendRealtimeSubscriptionParams(url: URL, subscription?: RealtimeWebSocketSubscription) {
  subscription?.topics.forEach((topic) => url.searchParams.append('topic', topic))
  if (subscription?.edge_instance_id) url.searchParams.set('edge_instance_id', subscription.edge_instance_id)
  if (subscription?.project_id !== undefined) url.searchParams.set('project_id', String(subscription.project_id))
  if (subscription?.source_type) url.searchParams.set('source_type', subscription.source_type)
  if (subscription?.gateway_id !== undefined) url.searchParams.set('gateway_id', String(subscription.gateway_id))
  subscription?.var_ids?.forEach((varId) => url.searchParams.append('var_id', String(varId)))
}

export function sendRealtimeWebSocketCommand<TPayload = unknown, TResult = unknown>(
  command: RealtimeWebSocketCommand<TPayload>,
  timeoutMs = 12000,
) {
  const token = getAccessToken()
  const url = new URL('/api/v1/ws', env.apiBaseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) url.searchParams.set('access_token', token)

  return new Promise<TResult>((resolve, reject) => {
    const socket = new WebSocket(url)
    let settled = false
    const timer = window.setTimeout(() => {
      cleanup()
      reject(new Error('websocket command timeout'))
    }, timeoutMs)

    function cleanup() {
      window.clearTimeout(timer)
      socket.removeEventListener('open', handleOpen)
      socket.removeEventListener('message', handleMessage)
      socket.removeEventListener('error', handleError)
      socket.removeEventListener('close', handleClose)
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        socket.close()
      }
    }

    function handleOpen() {
      socket.send(JSON.stringify(command))
    }

    function handleMessage(event: MessageEvent) {
      let message: RealtimeWebSocketEnvelope
      try {
        message = JSON.parse(String(event.data)) as RealtimeWebSocketEnvelope
      } catch {
        return
      }
      if (message.command_id !== command.command_id) return
      if (message.type === 'command.ack') {
        settled = true
        cleanup()
        const payload = message.payload as { result?: TResult } | TResult | undefined
        resolve(payload && typeof payload === 'object' && 'result' in payload ? (payload.result as TResult) : (payload as TResult))
        return
      }
      if (message.type === 'error') {
        settled = true
        cleanup()
        const payload = message.payload as { result?: unknown } | undefined
        reject(new RealtimeWebSocketCommandError(
          message.error?.message || message.error?.code || 'websocket command failed',
          message.error?.code,
          payload?.result,
        ))
      }
    }

    function handleError(event: Event) {
      settled = true
      cleanup()
      reject(event instanceof ErrorEvent ? event.error || new Error(event.message) : new Error('websocket command failed'))
    }

    function handleClose() {
      cleanup()
      if (!settled) {
        reject(new Error('websocket closed before command ack'))
      }
    }

    socket.addEventListener('open', handleOpen)
    socket.addEventListener('message', handleMessage)
    socket.addEventListener('error', handleError)
    socket.addEventListener('close', handleClose)
  })
}
