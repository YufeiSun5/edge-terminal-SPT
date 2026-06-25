import { useEffect, useMemo, useRef, useState } from 'react'
import type { MutableRefObject } from 'react'
import { useQuery } from '@tanstack/react-query'
import { subscribeRealtimeWebSocket } from './realtimeClient'
import type { RealtimeWebSocketStatus } from './realtimeClient'
import type {
  RealtimeVariablesSnapshotPayload,
  RealtimeWebSocketSubscription,
  TagSnapshot,
  VarIdentifier,
} from '@/shared/api/types'

export type RealtimeSnapshotSource = 'none' | 'http' | 'ws'

export type UseRealtimeSnapshotsOptions = {
  enabled?: boolean
  subscription: RealtimeWebSocketSubscription
  fallbackQueryKey: readonly unknown[]
  fallbackQueryFn: () => Promise<TagSnapshot[]>
  fallbackIntervalMs?: number
  uiCommitMs?: number
}

export function useRealtimeSnapshots({
  enabled = true,
  subscription,
  fallbackQueryKey,
  fallbackQueryFn,
  fallbackIntervalMs = 2000,
  uiCommitMs = 500,
}: UseRealtimeSnapshotsOptions) {
  const [status, setStatus] = useState<RealtimeWebSocketStatus>('closed')
  const [snapshots, setSnapshots] = useState<TagSnapshot[]>([])
  const [source, setSource] = useState<RealtimeSnapshotSource>('none')
  const [lastUpdatedAt, setLastUpdatedAt] = useState<string>()
  const [lastError, setLastError] = useState<unknown>()
  const latestByVarRef = useRef(new Map<string, TagSnapshot>())
  const commitTimerRef = useRef<number | undefined>(undefined)
  const subscriptionKey = useMemo(() => realtimeSubscriptionKey(subscription), [subscription])
  const connectionStatus = enabled ? status : 'closed'

  const fallbackQuery = useQuery({
    queryKey: fallbackQueryKey,
    queryFn: fallbackQueryFn,
    enabled,
    refetchOnWindowFocus: connectionStatus !== 'open',
    refetchInterval: connectionStatus === 'open' ? false : fallbackIntervalMs,
    retry: false,
  })

  useEffect(() => {
    latestByVarRef.current = new Map()
    if (commitTimerRef.current !== undefined) {
      window.clearTimeout(commitTimerRef.current)
      commitTimerRef.current = undefined
    }
    const resetTimer = window.setTimeout(() => {
      setSnapshots([])
      setSource('none')
      setLastUpdatedAt(undefined)
      setLastError(undefined)
    }, 0)
    return () => window.clearTimeout(resetTimer)
  }, [subscriptionKey])

  useEffect(() => {
    if (!enabled || !fallbackQuery.data) return
    mergeSnapshotsInto(latestByVarRef.current, fallbackQuery.data)
    commitSnapshots({
      map: latestByVarRef.current,
      setSnapshots,
      setSource,
      setLastUpdatedAt,
      source: connectionStatus === 'open' ? 'ws' : 'http',
    })
  }, [connectionStatus, enabled, fallbackQuery.data])

  useEffect(() => {
    if (!enabled) return undefined
    return subscribeRealtimeWebSocket({
      subscription,
      onStatus: setStatus,
      onMessage: (envelope) => {
        if (envelope.type !== 'realtime.variables.snapshot') return
        const payload = envelope.payload as RealtimeVariablesSnapshotPayload | undefined
        mergeSnapshotsInto(latestByVarRef.current, payload?.items ?? [])
        scheduleCommit({
          timerRef: commitTimerRef,
          delayMs: uiCommitMs,
          map: latestByVarRef.current,
          setSnapshots,
          setSource,
          setLastUpdatedAt,
        })
      },
      onError: (error) => {
        setLastError(error)
      },
    })
  }, [enabled, subscription, uiCommitMs])

  useEffect(() => () => {
    if (commitTimerRef.current !== undefined) {
      window.clearTimeout(commitTimerRef.current)
    }
  }, [])

  return {
    snapshots,
    status: connectionStatus,
    source,
    lastUpdatedAt,
    lastError: lastError ?? fallbackQuery.error,
    fallbackQuery,
  }
}

function scheduleCommit({
  timerRef,
  delayMs,
  map,
  setSnapshots,
  setSource,
  setLastUpdatedAt,
}: {
  timerRef: MutableRefObject<number | undefined>
  delayMs: number
  map: Map<string, TagSnapshot>
  setSnapshots: (snapshots: TagSnapshot[]) => void
  setSource: (source: RealtimeSnapshotSource) => void
  setLastUpdatedAt: (value?: string) => void
}) {
  if (timerRef.current !== undefined) return
  timerRef.current = window.setTimeout(() => {
    timerRef.current = undefined
    commitSnapshots({ map, setSnapshots, setSource, setLastUpdatedAt, source: 'ws' })
  }, Math.max(0, delayMs))
}

function commitSnapshots({
  map,
  setSnapshots,
  setSource,
  setLastUpdatedAt,
  source,
}: {
  map: Map<string, TagSnapshot>
  setSnapshots: (snapshots: TagSnapshot[]) => void
  setSource: (source: RealtimeSnapshotSource) => void
  setLastUpdatedAt: (value?: string) => void
  source: RealtimeSnapshotSource
}) {
  const next = Array.from(map.values())
  setSnapshots(next)
  setSource(next.length > 0 ? source : 'none')
  setLastUpdatedAt(latestSnapshotTime(next))
}

function mergeSnapshotsInto(target: Map<string, TagSnapshot>, snapshots: TagSnapshot[]) {
  for (const snapshot of snapshots) {
    const key = snapshotKey(snapshot)
    if (!key) continue
    target.set(key, snapshot)
  }
}

function snapshotKey(snapshot: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  const value = snapshot.var_id_text ?? snapshot.var_id
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}

function realtimeSubscriptionKey(subscription: RealtimeWebSocketSubscription) {
  return JSON.stringify({
    topics: subscription.topics,
    edge_instance_id: subscription.edge_instance_id ?? '',
    source_type: subscription.source_type ?? '',
    gateway_id: subscription.gateway_id ?? '',
    project_id: subscription.project_id ?? '',
    var_ids: (subscription.var_ids ?? []).map((value: VarIdentifier) => String(value)),
  })
}

function latestSnapshotTime(snapshots: TagSnapshot[]) {
  let latest = 0
  let value = ''
  for (const snapshot of snapshots) {
    const time = Date.parse(snapshot.last_update)
    if (!Number.isFinite(time) || time <= 0 || snapshot.last_update.startsWith('0001-')) continue
    if (time > latest) {
      latest = time
      value = snapshot.last_update
    }
  }
  return value || undefined
}
