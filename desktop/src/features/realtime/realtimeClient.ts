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
  onClose?: () => void
  onError?: (error: Event | Error) => void
  subscription?: RealtimeWebSocketSubscription
}

type SharedRealtimeWebSocket = {
  socket: WebSocket
  handlers: Set<RealtimeWebSocketHandlers>
}

const sharedRealtimeSockets = new Map<string, SharedRealtimeWebSocket>()

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

export function subscribeRealtimeWebSocket({ onMessage, onClose, onError, subscription }: RealtimeWebSocketHandlers) {
  const token = getAccessToken()
  const url = new URL('/api/v1/ws', env.apiBaseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) url.searchParams.set('access_token', token)
  subscription?.topics.forEach((topic) => url.searchParams.append('topic', topic))
  if (subscription?.edge_instance_id) url.searchParams.set('edge_instance_id', subscription.edge_instance_id)
  if (subscription?.project_id !== undefined) url.searchParams.set('project_id', String(subscription.project_id))
  if (subscription?.source_type) url.searchParams.set('source_type', subscription.source_type)
  if (subscription?.gateway_id !== undefined) url.searchParams.set('gateway_id', String(subscription.gateway_id))
  subscription?.var_ids?.forEach((varId) => url.searchParams.append('var_id', String(varId)))

  const key = url.toString()
  const handler: RealtimeWebSocketHandlers = { onMessage, onClose, onError, subscription }
  const existing = sharedRealtimeSockets.get(key)
  if (existing) {
    existing.handlers.add(handler)
    return () => {
      existing.handlers.delete(handler)
      if (existing.handlers.size === 0 && existing.socket.readyState === WebSocket.OPEN) {
        sharedRealtimeSockets.delete(key)
        existing.socket.close()
      }
      if (existing.handlers.size === 0 && existing.socket.readyState === WebSocket.CLOSED) {
        sharedRealtimeSockets.delete(key)
      }
    }
  }

  const socket = new WebSocket(url)
  const shared: SharedRealtimeWebSocket = { socket, handlers: new Set([handler]) }
  sharedRealtimeSockets.set(key, shared)
  socket.addEventListener('open', () => {
    if (shared.handlers.size === 0) {
      sharedRealtimeSockets.delete(key)
      socket.close()
    }
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
    sharedRealtimeSockets.delete(key)
    shared.handlers.forEach((item) => item.onClose?.())
  })

  return () => {
    shared.handlers.delete(handler)
    if (shared.handlers.size === 0 && socket.readyState === WebSocket.OPEN) {
      sharedRealtimeSockets.delete(key)
      socket.close()
    }
    if (shared.handlers.size === 0 && socket.readyState === WebSocket.CLOSED) {
      sharedRealtimeSockets.delete(key)
    }
  }
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
