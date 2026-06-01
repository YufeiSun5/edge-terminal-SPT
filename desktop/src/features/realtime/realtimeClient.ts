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

export function subscribeRealtimeWebSocket({ onMessage, onClose, onError, subscription }: RealtimeWebSocketHandlers) {
  const token = getAccessToken()
  const url = new URL('/api/v1/ws', env.apiBaseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) url.searchParams.set('access_token', token)
  subscription?.topics.forEach((topic) => url.searchParams.append('topic', topic))
  if (subscription?.project_id !== undefined) url.searchParams.set('project_id', String(subscription.project_id))
  if (subscription?.source_type) url.searchParams.set('source_type', subscription.source_type)
  if (subscription?.gateway_id !== undefined) url.searchParams.set('gateway_id', String(subscription.gateway_id))
  subscription?.var_ids?.forEach((varId) => url.searchParams.append('var_id', String(varId)))

  const socket = new WebSocket(url)
  socket.addEventListener('message', (event) => {
    try {
      onMessage(JSON.parse(String(event.data)) as RealtimeWebSocketEnvelope)
    } catch (error) {
      onError?.(error instanceof Error ? error : new Error('invalid websocket message'))
    }
  })
  socket.addEventListener('error', (event) => onError?.(event))
  socket.addEventListener('close', () => onClose?.())

  return () => {
    if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
      socket.close()
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
        reject(new Error(message.error?.message || message.error?.code || 'websocket command failed'))
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
