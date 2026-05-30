import { fetchEventSource } from '@microsoft/fetch-event-source'
import { getAccessToken } from '@/shared/auth/tokenStore'
import { env } from '@/shared/config/env'

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

  return fetchEventSource(`${env.edgeApiBaseUrl}${path}`, {
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
