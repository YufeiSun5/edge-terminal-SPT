import { useEffect, useMemo, useState } from 'react'
import type { CSSProperties, KeyboardEvent, ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Empty } from 'antd'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router'
import { Area, AreaChart, CartesianGrid, ReferenceLine, Tooltip, XAxis, YAxis } from 'recharts'
import {
  SealCheck as BadgeCheck,
  Barcode,
  Cpu,
  Drop as Droplets,
  Flask as FlaskConical,
  Gauge,
  Heartbeat as HeartPulse,
  Power,
  Pulse,
  Thermometer,
  Timer,
  SpeakerHigh as Volume2,
  Wind,
} from '@phosphor-icons/react'
import type { Icon as PhosphorIcon } from '@phosphor-icons/react'
import { getCurrentDetectionRun, getLimitAlarms, getRealtimeVariables, getStationViewEffective } from '@/features/edge-status/api'
import { getHistoryData } from '@/features/history-query/api'
import { ApiError } from '@/shared/api/http'
import type { DetectionRun, DetectionRunStandardItem, HistoryDataItem, StationViewResolvedBinding, TagSnapshot, VarIdentifier } from '@/shared/api/types'
import { CockpitModelStage } from '@/features/model-cockpit/components/CockpitModelStage'
import { StationLightBackground } from '@/features/station-operation/components/StationLightBackground'
import { languageCode } from '@/shared/i18n/language'
import './model-cockpit.css'

type TopCardConfig = {
  labelKey: string
  hintKey: string
  value: string
  icon: PhosphorIcon
}

type TrendAxisMode = 'standard' | 'auto'

export function ModelCockpitPage() {
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const lightPos = useCockpitLight()
  const pageVisible = usePageVisibility()
  const [trendAxisState, setTrendAxisState] = useState<{ scope: string; modes: Record<string, TrendAxisMode> }>({ scope: '', modes: {} })
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search])
  const selectedProjectId = parsePositiveInt(searchParams.get('project_id'))
  const selectedEdgeInstanceId = searchParams.get('edge_instance_id') || undefined

  useEffect(() => {
    const resetScroll = () => {
      window.scrollTo({ top: 0, left: 0 })
      document.documentElement.scrollTop = 0
      document.body.scrollTop = 0
      document.querySelector<HTMLElement>('.workbench-content')?.scrollTo({ top: 0, left: 0 })
    }
    resetScroll()
    const frame = window.requestAnimationFrame(resetScroll)
    const timer = window.setTimeout(resetScroll, 120)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(timer)
    }
  }, [])

  const stationViewQuery = useQuery({
    queryKey: ['model-cockpit', 'station-view', selectedProjectId, selectedEdgeInstanceId ?? 'local'],
    queryFn: () => getStationViewEffective(selectedProjectId!, selectedEdgeInstanceId),
    enabled: selectedProjectId !== undefined,
    refetchInterval: 10000,
    retry: false,
  })
  const bindings = useMemo(() => stationViewBindings(stationViewQuery.data?.items ?? []), [stationViewQuery.data?.items])
  const cockpitVarIds = useMemo(() => uniqueVarIds(bindings), [bindings])
  const realtimeQuery = useQuery({
    queryKey: ['model-cockpit', 'realtime', selectedProjectId, selectedEdgeInstanceId ?? 'local', cockpitVarIds.join('|')],
    queryFn: () => getRealtimeVariables({ project_id: selectedProjectId!, edge_instance_id: selectedEdgeInstanceId, var_id: cockpitVarIds }),
    enabled: selectedProjectId !== undefined && cockpitVarIds.length > 0,
    refetchInterval: 2000,
    retry: false,
  })
  const currentRunQuery = useQuery({
    queryKey: ['model-cockpit', 'current-run', selectedProjectId],
    queryFn: () => getCurrentRunOrNull(selectedProjectId!),
    enabled: selectedProjectId !== undefined,
    refetchInterval: 5000,
    retry: false,
  })
  const currentRunId = currentRunQuery.data?.id
  const trendAxisScope = `${selectedProjectId ?? 'none'}:${currentRunId ?? 'none'}`
  const trendAxisModes = trendAxisState.scope === trendAxisScope ? trendAxisState.modes : {}
  const alarmsQuery = useQuery({
    queryKey: ['model-cockpit', 'alarms', selectedProjectId, currentRunQuery.data?.id ?? 'none'],
    queryFn: () =>
      getLimitAlarms({
        project_id: selectedProjectId!,
        task_id: currentRunQuery.data?.id,
        status: 'active',
        limit: 50,
      }),
    enabled: selectedProjectId !== undefined,
    refetchInterval: 5000,
    retry: false,
  })
  const historyQuery = useQuery({
    queryKey: ['model-cockpit', 'history', selectedProjectId, currentRunId ?? 'none', cockpitVarIds.slice(0, 4).join('|')],
    queryFn: () => getHistoryData({ project_id: selectedProjectId!, task_id: currentRunId!, limit: 300 }),
    enabled: selectedProjectId !== undefined && currentRunId !== undefined && cockpitVarIds.length > 0,
    refetchInterval: (query) =>
      historyRefetchInterval({
        data: query.state.data,
        currentRun: currentRunQuery.data,
        pageVisible,
      }),
    retry: false,
  })
  const metricCards = useMemo(
    () =>
      resolveMetricCards({
        bindings,
        snapshots: realtimeQuery.data ?? [],
        standardItems: currentRunQuery.data?.standard_items ?? [],
        historyItems: historyQuery.data?.items ?? [],
        language: languageCode(i18n.resolvedLanguage),
      }),
    [bindings, currentRunQuery.data?.standard_items, historyQuery.data?.items, i18n.resolvedLanguage, realtimeQuery.data],
  )
  const monitorMetrics = metricCards.slice(0, 10)
  const trendCards = metricCards.filter((metric) => metric.numericValue !== undefined || metric.history.length > 0).slice(0, 2)
  const topCards = useMemo(
    () => buildTopCards(stationViewQuery.data, currentRunQuery.data, alarmsQuery.data?.items.length ?? 0, t, i18n.resolvedLanguage),
    [alarmsQuery.data?.items.length, currentRunQuery.data, i18n.resolvedLanguage, stationViewQuery.data, t],
  )
  const lastUpdatedAt = latestSnapshotTime(realtimeQuery.data ?? [])

  return (
    <div
      className="model-cockpit-page"
      style={{ '--light-x': `${lightPos.pageX}px`, '--light-y': `${lightPos.pageY}px` } as CSSProperties}
    >
      <StationLightBackground scopeClassName="model-cockpit-page" />

      <header className="cockpit-title-row">
        <div className="cockpit-title-bar">
          <span>{t('modelCockpit.title')}</span>
        </div>
      </header>

      <section className="cockpit-top-row" aria-label={t('modelCockpit.title')}>
        {topCards.map((card, index) => (
          <CockpitGlassPanel className={`cockpit-top-card cockpit-card-tone-${index}`} key={card.labelKey} title={t(card.labelKey)}>
            <TopCardContent card={card} />
          </CockpitGlassPanel>
        ))}
      </section>

      <section className="cockpit-main-card glass-panel" aria-label={t('modelCockpit.title')}>
        <div className="cockpit-monitor-panel" aria-label={t('modelCockpit.realtime.title')}>
          <div className="cockpit-section-title">
            <span />
            <strong>{t('modelCockpit.realtime.title')}</strong>
          </div>
          <div className="cockpit-monitor-table table-scroll-container">
            {selectedProjectId === undefined ? (
              <CockpitEmpty description={t('modelCockpit.messages.selectProject')} />
            ) : monitorMetrics.length === 0 ? (
              <CockpitEmpty description={stationViewQuery.isError ? t('modelCockpit.messages.stationViewUnavailable') : t('modelCockpit.messages.noMetrics')} />
            ) : (
              <table>
                <thead>
                  <tr>
                    <th>{t('modelCockpit.realtime.item')}</th>
                    <th>{t('modelCockpit.realtime.value')}</th>
                    <th>{t('modelCockpit.metric.limit')}</th>
                    <th>{t('modelCockpit.realtime.factor')}</th>
                    <th>{t('modelCockpit.realtime.result')}</th>
                  </tr>
                </thead>
                <tbody>
                  {monitorMetrics.map((metric) => {
                    const Icon = metric.icon
                    return (
                      <tr key={metric.id}>
                        <td>
                          <span className="cockpit-monitor-icon">
                            <Icon aria-hidden="true" weight="regular" />
                          </span>
                          {metric.label}
                        </td>
                        <td className="mono">
                          {metric.value} {formatUnit(metric.unit)}
                        </td>
                        <td className="cockpit-limit-cell">{formatLimitRange(metric)}</td>
                        <td className="cockpit-factor-cell">{metric.optimizationFactor}</td>
                        <td>
                          <span className={`cockpit-state ${statusClassName(metric.status)}`}>{metric.status}</span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>
          <div className="cockpit-monitor-footer">
            <span />
            {t('modelCockpit.realtime.updatedAt', { value: lastUpdatedAt ? formatDateTime(lastUpdatedAt) : '-' })}
          </div>
        </div>

        <main className="cockpit-center-stage" aria-label={t('modelCockpit.title')}>
          <CockpitModelStage />
        </main>
      </section>

      <aside className="cockpit-right-stack" aria-label={t('modelCockpit.status.telemetry')}>
        {trendCards.length === 0 ? (
          [t('modelCockpit.charts.temperature'), t('modelCockpit.charts.humidity')].map((title, index) => (
            <CockpitGlassPanel className={`cockpit-trend-card cockpit-card-tone-${index + 2}`} key={title} title={title}>
              <TrendEmptyState description={trendEmptyDescription(selectedProjectId, currentRunQuery.data, currentRunQuery.isSuccess, t)} />
            </CockpitGlassPanel>
          ))
        ) : (
          trendCards.map((metric, index) => (
            <CockpitGlassPanel className={`cockpit-trend-card cockpit-card-tone-${index + 2}`} key={metric.id} title={metric.label}>
              <TrendCardContent
                metric={metric}
                axisMode={trendAxisModes[metric.id] ?? 'standard'}
                onToggleAxisMode={() =>
                  setTrendAxisState((current) => {
                    const modes = current.scope === trendAxisScope ? current.modes : {}
                    return {
                      scope: trendAxisScope,
                      modes: {
                        ...modes,
                        [metric.id]: (modes[metric.id] ?? 'standard') === 'standard' ? 'auto' : 'standard',
                      },
                    }
                  })
                }
              />
            </CockpitGlassPanel>
          ))
        )}
      </aside>
    </div>
  )
}

type ResolvedMetricCard = {
  id: string
  label: string
  value: string
  numericValue?: number
  unit: string
  limitMin?: number | null
  limitMax?: number | null
  optimizationFactor: string
  status: 'OK' | 'NG' | '--'
  icon: PhosphorIcon
  decimalPlaces: number
  history: TrendPoint[]
  lastUpdate?: string
}

type TrendPoint = {
  time: string
  value: number
  timestamp?: number
  realtime?: boolean
}

function historyRefetchInterval({
  data,
  currentRun,
  pageVisible,
}: {
  data: unknown
  currentRun?: DetectionRun | null
  pageVisible: boolean
}) {
  if (!currentRun?.id) return false
  if (!pageVisible) return 60000
  if (historyResponseItemCount(data) > 0) return 10000
  const startedAt = parseTimestamp(currentRun.started_at)
  if (startedAt !== undefined && Date.now() - startedAt <= 60000) return 3000
  return 10000
}

function historyResponseItemCount(data: unknown) {
  if (!data || typeof data !== 'object') return 0
  const items = (data as { items?: unknown }).items
  return Array.isArray(items) ? items.length : 0
}

function trendEmptyDescription(
  selectedProjectId: number | undefined,
  currentRun: DetectionRun | null | undefined,
  currentRunLoaded: boolean,
  t: (key: string) => string,
) {
  if (selectedProjectId === undefined) return t('modelCockpit.messages.selectProject')
  if (currentRunLoaded && !currentRun) return t('modelCockpit.messages.noCurrentRun')
  return t('modelCockpit.messages.noHistory')
}

function parsePositiveInt(value: string | null) {
  if (!value) return undefined
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}

async function getCurrentRunOrNull(projectId: number) {
  try {
    return await getCurrentDetectionRun(projectId)
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}

function stationViewBindings(items: Array<{ visible: boolean; sort_order: number; resolved_bindings?: StationViewResolvedBinding[] }>) {
  const sortedBindings = items
    .filter((item) => item.visible)
    .flatMap((item) => (item.resolved_bindings ?? []).map((binding) => ({ ...binding, sort_order: binding.sort_order || item.sort_order })))
    .filter((binding) => binding.var_id_text || binding.var_id !== undefined || binding.var_name)
    .sort((left, right) => left.sort_order - right.sort_order)

  return uniqueStationViewBindings(sortedBindings)
}

function uniqueStationViewBindings(bindings: StationViewResolvedBinding[]) {
  const seen = new Set<string>()
  return bindings.filter((binding) => {
    const key = bindingKey(binding)
    if (!key) return true
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function uniqueVarIds(bindings: StationViewResolvedBinding[]): VarIdentifier[] {
  const seen = new Set<string>()
  const result: VarIdentifier[] = []
  for (const binding of bindings) {
    const raw = binding.var_id_text ?? binding.var_id
    if (raw === undefined || raw === null || raw === '') continue
    const key = String(raw)
    if (seen.has(key)) continue
    seen.add(key)
    result.push(raw)
  }
  return result
}

function varKey(value?: VarIdentifier | string | null) {
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}

function snapshotKey(snapshot: TagSnapshot) {
  return varKey(snapshot.var_id_text ?? snapshot.var_id)
}

function bindingKey(binding: StationViewResolvedBinding) {
  return varKey(binding.var_id_text ?? binding.var_id ?? binding.var_name)
}

function itemName(
  item: Pick<StationViewResolvedBinding, 'display_name' | 'display_name_en' | 'display_name_ja' | 'var_name' | 'var_id_text'>,
  language: string,
) {
  if (language === 'en') return item.display_name_en || item.display_name || item.var_name || item.var_id_text || '-'
  if (language === 'ja') return item.display_name_ja || item.display_name || item.var_name || item.var_id_text || '-'
  return item.display_name || item.var_name || item.display_name_en || item.display_name_ja || item.var_id_text || '-'
}

function resolveMetricCards({
  bindings,
  snapshots,
  standardItems,
  historyItems,
  language,
}: {
  bindings: StationViewResolvedBinding[]
  snapshots: TagSnapshot[]
  standardItems: DetectionRunStandardItem[]
  historyItems: HistoryDataItem[]
  language: string
}): ResolvedMetricCard[] {
  const snapshotById = new Map(snapshots.map((snapshot) => [snapshotKey(snapshot), snapshot]))
  const standardById = new Map(standardItems.map((item) => [varKey(item.var_id_text ?? item.var_id), item]))
  const historyById = groupHistoryByVarId(historyItems)

  return bindings.map((binding, index) => {
    const key = bindingKey(binding) || `${binding.var_name ?? 'binding'}-${index}`
    const snapshot = snapshotById.get(key)
    const standardItem = standardById.get(key)
    const decimalPlaces = standardItem?.decimal_places ?? binding.decimal_places ?? 2
    const numericValue = snapshot && !snapshot.is_string && Number.isFinite(snapshot.value) ? snapshot.value : undefined
    const rawValue = snapshot?.is_string ? snapshot.str_value : numericValue !== undefined ? formatMetricValue(numericValue, decimalPlaces) : '-'
    const limitMin = pickNumberOrNull(standardItem?.limit_l, binding.limit_l, standardItem?.limit_ll, binding.limit_ll)
    const limitMax = pickNumberOrNull(standardItem?.limit_h, binding.limit_h, standardItem?.limit_hh, binding.limit_hh)
    const label = itemName(binding, language)
    return {
      id: key,
      label,
      value: rawValue || '-',
      numericValue,
      unit: standardItem?.unit || binding.unit || '',
      limitMin,
      limitMax,
      optimizationFactor: computeOptimizationFactor({
        label,
        varName: binding.var_name,
        value: numericValue,
        min: limitMin,
        max: limitMax,
      }),
      status: resolveStatus(numericValue, limitMin, limitMax),
      icon: iconForMetric(binding),
      decimalPlaces,
      history: buildHistoryTrend(historyById.get(key) ?? [], decimalPlaces),
      lastUpdate: snapshot?.last_update,
    }
  })
}

function groupHistoryByVarId(items: HistoryDataItem[]) {
  const groups = new Map<string, HistoryDataItem[]>()
  for (const item of items) {
    const key = varKey(item.var_id_text ?? item.var_id)
    if (!key) continue
    const list = groups.get(key) ?? []
    list.push(item)
    groups.set(key, list)
  }
  return groups
}

function buildHistoryTrend(items: HistoryDataItem[], decimalPlaces: number): TrendPoint[] {
  return items
    .filter((item) => typeof item.value === 'number' && Number.isFinite(item.value))
    .sort((left, right) => (parseTimestamp(left.source_time || left.created_at) ?? 0) - (parseTimestamp(right.source_time || right.created_at) ?? 0))
    .slice(-40)
    .map((item) => {
      const sourceTime = item.source_time || item.created_at
      return {
        time: formatTimeLabel(sourceTime),
        value: Number((item.value ?? 0).toFixed(Math.max(0, Math.min(decimalPlaces, 2)))),
        timestamp: parseTimestamp(sourceTime),
      }
    })
}

function iconForMetric(binding: Pick<StationViewResolvedBinding, 'var_name' | 'display_name' | 'display_name_en' | 'var_group' | 'unit'>): PhosphorIcon {
  const text = `${binding.var_name ?? ''} ${binding.display_name ?? ''} ${binding.display_name_en ?? ''} ${binding.var_group ?? ''} ${binding.unit ?? ''}`.toLowerCase()
  if (text.includes('湿') || text.includes('humid') || text.includes('rh')) return Droplets
  if (text.includes('温') || text.includes('temp') || text.includes('degc') || text.includes('℃')) return Thermometer
  if (text.includes('压') || text.includes('pressure') || text.includes('kpa')) return Gauge
  if (text.includes('风') || text.includes('air') || text.includes('flow')) return Wind
  if (text.includes('噪') || text.includes('noise') || text.includes('db')) return Volume2
  if (text.includes('震') || text.includes('vibration')) return Pulse
  if (text.includes('功') || text.includes('power') || text.includes('kw')) return Power
  return HeartPulse
}

function buildTopCards(
  stationView: Awaited<ReturnType<typeof getStationViewEffective>> | undefined,
  currentRun: DetectionRun | null | undefined,
  activeAlarmCount: number,
  t: (key: string, options?: Record<string, unknown>) => string,
  language?: string,
): TopCardConfig[] {
  const project = stationView?.project
  const lang = languageCode(language)
  const projectName = project
    ? lang === 'en'
      ? project.display_name_en || project.project_code
      : lang === 'ja'
        ? project.display_name_ja || project.project_code
        : project.display_name || project.name || project.project_code
    : '-'
  const model = currentRun?.device_model || project?.model_name || '-'
  const result = currentRun ? (activeAlarmCount > 0 ? 'NG' : formatRunStatus(currentRun.status)) : t('modelCockpit.status.noCurrentRun')
  return [
    { labelKey: 'modelCockpit.cards.model', hintKey: 'modelCockpit.cards.modelHint', value: model, icon: Cpu },
    { labelKey: 'modelCockpit.cards.serial', hintKey: 'modelCockpit.cards.serialHint', value: currentRun?.factory_no || project?.project_code || '-', icon: Barcode },
    { labelKey: 'modelCockpit.cards.customer', hintKey: 'modelCockpit.cards.customerHint', value: currentRun?.customer_name || projectName, icon: FlaskConical },
    { labelKey: 'modelCockpit.cards.duration', hintKey: 'modelCockpit.cards.durationHint', value: formatRunDuration(currentRun), icon: Timer },
    { labelKey: 'modelCockpit.cards.result', hintKey: 'modelCockpit.cards.resultHint', value: result, icon: BadgeCheck },
  ]
}

function formatRunStatus(status: string) {
  if (!status) return '-'
  if (status === 'running') return 'RUNNING'
  if (status === 'stopped') return 'OK'
  return status.toUpperCase()
}

function formatRunDuration(run?: DetectionRun | null) {
  if (!run) return '-'
  const seconds = run.status === 'running' && run.started_at ? Math.max(run.duration_sec, Math.floor((Date.now() - Date.parse(run.started_at)) / 1000)) : run.duration_sec
  if (!Number.isFinite(seconds) || seconds < 0) return '-'
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = Math.floor(seconds % 60)
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
}

function resolveStatus(value: number | undefined, min?: number | null, max?: number | null): 'OK' | 'NG' | '--' {
  if (value === undefined) return '--'
  if (typeof min === 'number' && value < min) return 'NG'
  if (typeof max === 'number' && value > max) return 'NG'
  return 'OK'
}

function statusClassName(status: 'OK' | 'NG' | '--') {
  if (status === 'OK') return 'ok'
  if (status === 'NG') return 'ng'
  return 'neutral'
}

function pickNumberOrNull(...values: Array<number | null | undefined>) {
  return values.find((value): value is number => typeof value === 'number' && Number.isFinite(value)) ?? null
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
  return value
}

function formatDateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatTimeLabel(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function parseTimestamp(value?: string) {
  if (!value || value.startsWith('0001-')) return undefined
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : undefined
}

function formatLimitRange(metric: ResolvedMetricCard) {
  if (typeof metric.limitMin !== 'number' && typeof metric.limitMax !== 'number') return '-'
  const min = typeof metric.limitMin === 'number' ? formatLimit(metric.limitMin) : '-'
  const max = typeof metric.limitMax === 'number' ? formatLimit(metric.limitMax) : '-'
  return `${min}-${max} ${formatUnit(metric.unit)}`
}

function computeOptimizationFactor({
  label,
  varName,
  value,
  min,
  max,
}: {
  label: string
  varName?: string | null
  value?: number
  min?: number | null
  max?: number | null
}) {
  if (value === undefined || !Number.isFinite(value)) return '-'
  if (isMaxOnlyOptimizationMetric(label, varName)) {
    if (typeof max !== 'number' || !Number.isFinite(max) || max === 0) return '-'
    return `${((1 + (max - value) / max) * 100).toFixed(1)}%`
  }
  if (typeof min !== 'number' || typeof max !== 'number' || !Number.isFinite(min) || !Number.isFinite(max) || max - min === 0) return '-'
  const middle = (max + min) / 2
  const halfRange = (max - min) / 2
  return `${((2 - Math.abs(value - middle) / halfRange) * 100).toFixed(1)}%`
}

function isMaxOnlyOptimizationMetric(label: string, varName?: string | null) {
  const text = `${label} ${varName ?? ''}`.toLowerCase()
  return text.includes('震') || text.includes('振') || text.includes('vibration') || text.includes('噪') || text.includes('noise')
}

function CockpitEmpty({ description }: { description: string }) {
  return (
    <div className="cockpit-empty-state">
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description} />
    </div>
  )
}

function TrendEmptyState({ description }: { description: string }) {
  const { t } = useTranslation()
  return (
    <div className="cockpit-trend-empty">
      <div className="cockpit-trend-empty-grid" aria-hidden="true">
        <span />
        <span />
        <span />
      </div>
      <strong>{description}</strong>
      <small>{t('modelCockpit.messages.trendPlaceholder')}</small>
    </div>
  )
}

function formatMetricValue(value: number, decimals: number) {
  return value.toFixed(Math.max(0, Math.min(decimals, 2)))
}

function CockpitGlassPanel({ children, className, title }: { children?: ReactNode; className?: string; title?: ReactNode }) {
  return (
    <section className={['cockpit-glass-panel', 'glass-panel', className].filter(Boolean).join(' ')}>
      <div className="cockpit-panel-title">
        <span>{title}</span>
      </div>
      <div className="cockpit-panel-body">{children}</div>
    </section>
  )
}

function TopCardContent({ card }: { card: TopCardConfig }) {
  const { t } = useTranslation()
  const Icon = card.icon
  return (
    <div className="cockpit-top-content">
      <div className="cockpit-top-icon">
        <Icon aria-hidden="true" weight="light" />
      </div>
      <div className="cockpit-top-copy">
        <strong>{card.value}</strong>
        <small>{t(card.hintKey)}</small>
      </div>
    </div>
  )
}

function TrendCardContent({ metric, axisMode, onToggleAxisMode }: { metric: ResolvedMetricCard; axisMode: TrendAxisMode; onToggleAxisMode: () => void }) {
  const { t } = useTranslation()
  const data = useMemo(() => buildTrendData(metric), [metric])
  const domain = useMemo(() => buildTrendDomain(metric, data, axisMode), [axisMode, data, metric])
  const modeLabel = t(axisMode === 'standard' ? 'modelCockpit.charts.standardScale' : 'modelCockpit.charts.autoScale')
  const { ref, size } = useChartContainerSize(132)
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onToggleAxisMode()
  }

  return (
    <div
      className="cockpit-trend-content"
      role="button"
      tabIndex={0}
      title={t('modelCockpit.charts.toggleScale')}
      onClick={onToggleAxisMode}
      onKeyDown={handleKeyDown}
    >
      <div className="cockpit-trend-summary">
        <span>{metric.label}</span>
        <strong>
          {metric.value} <small>{formatUnit(metric.unit)}</small>
        </strong>
      </div>
      <div className="cockpit-trend-mode" aria-label={t('modelCockpit.charts.scaleMode')}>
        {modeLabel}
      </div>
      <div className="cockpit-trend-chart" ref={ref}>
        {size ? (
          <AreaChart width={size.width} height={size.height} data={data} margin={{ top: 10, right: 8, left: -24, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 4" vertical={false} stroke="rgba(7, 48, 110, 0.08)" />
            <XAxis dataKey="time" tickLine={false} axisLine={false} dy={6} fontSize={10} stroke="rgba(7, 48, 110, 0.45)" />
            <YAxis tickLine={false} axisLine={false} width={42} fontSize={10} stroke="rgba(7, 48, 110, 0.45)" domain={domain} tickCount={4} />
            {typeof metric.limitMin === 'number' ? <ReferenceLine y={metric.limitMin} stroke="rgba(22,119,255,0.58)" strokeDasharray="4 4" ifOverflow="discard" /> : null}
            {typeof metric.limitMax === 'number' ? <ReferenceLine y={metric.limitMax} stroke="rgba(255,77,79,0.58)" strokeDasharray="4 4" ifOverflow="discard" /> : null}
            <Tooltip
              contentStyle={{
                backgroundColor: 'rgba(255, 255, 255, 0.92)',
                border: '1px solid rgba(255,255,255,0.78)',
                borderRadius: 8,
                boxShadow: '0 12px 28px rgba(7,48,110,0.12)',
                color: '#07306e',
              }}
              labelStyle={{ color: 'rgba(7,48,110,0.56)', fontSize: 11 }}
              itemStyle={{ color: '#1677ff', fontWeight: 700 }}
              formatter={(value) => [`${Number(value).toFixed(Math.max(0, Math.min(metric.decimalPlaces, 2)))} ${formatUnit(metric.unit)}`, metric.label]}
            />
            <Area
              type="monotone"
              dataKey="value"
              stroke="#1677ff"
              strokeWidth={2}
              fill="rgba(22, 119, 255, 0.12)"
              isAnimationActive={false}
              activeDot={{ r: 3, strokeWidth: 0, fill: '#1677ff' }}
            />
          </AreaChart>
        ) : null}
      </div>
    </div>
  )
}

function useChartContainerSize(minHeight: number) {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  const [size, setSize] = useState<{ width: number; height: number } | null>(null)

  useEffect(() => {
    if (!node) return undefined
    const update = () => {
      const rect = node.getBoundingClientRect()
      const width = Math.floor(rect.width)
      if (width <= 0) return
      setSize({
        width,
        height: Math.max(minHeight, Math.floor(rect.height)),
      })
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(node)
    return () => observer.disconnect()
  }, [minHeight, node])

  return { ref: setNode, size }
}

function buildTrendData(metric: ResolvedMetricCard): TrendPoint[] {
  const currentPoints = buildCurrentValueTrend(metric)
  if (metric.history.length === 0) return currentPoints
  if (currentPoints.length === 0) return metric.history
  const lastHistoryPoint = metric.history[metric.history.length - 1]
  const currentPoint = currentPoints[0]
  if (currentPoint.timestamp !== undefined && lastHistoryPoint.timestamp !== undefined && currentPoint.timestamp <= lastHistoryPoint.timestamp) {
    return metric.history
  }
  if (currentPoint.time === lastHistoryPoint.time && currentPoint.value === lastHistoryPoint.value) return metric.history
  return [...metric.history, currentPoint].slice(-41)
}

function buildTrendDomain(metric: ResolvedMetricCard, data: TrendPoint[], axisMode: TrendAxisMode): [number, number] {
  const hasLowerLimit = typeof metric.limitMin === 'number'
  const hasUpperLimit = typeof metric.limitMax === 'number'
  const dataValues = data.map((item) => item.value).filter((value) => Number.isFinite(value))
  const hasStandardRange = hasLowerLimit && hasUpperLimit && metric.limitMin! < metric.limitMax!
  if (dataValues.length === 0) {
    if (hasStandardRange) return [metric.limitMin!, metric.limitMax!]
    return [0, 1]
  }

  const focusedDomain = expandTrendDomainWithNearbyLimits(
    paddedTrendDomain(dataValues, axisMode === 'auto' ? 0.18 : 0.12),
    hasLowerLimit ? metric.limitMin! : undefined,
    hasUpperLimit ? metric.limitMax! : undefined,
  )
  if (axisMode === 'standard' && hasStandardRange) {
    const standardRange = Math.abs(metric.limitMax! - metric.limitMin!)
    const focusedRange = Math.max(focusedDomain[1] - focusedDomain[0], 0.1)
    return standardRange > focusedRange * 6 ? focusedDomain : [metric.limitMin!, metric.limitMax!]
  }
  return focusedDomain
}

function paddedTrendDomain(values: number[], ratio: number): [number, number] {
  const min = Math.min(...values)
  const max = Math.max(...values)
  const valueRange = Math.abs(max - min)
  const padding = Math.max(valueRange * ratio, Math.abs(max) * 0.02, 1)
  return [Math.floor((min - padding) * 10) / 10, Math.ceil((max + padding) * 10) / 10]
}

function expandTrendDomainWithNearbyLimits(domain: [number, number], min?: number, max?: number): [number, number] {
  let [lower, upper] = domain
  const range = Math.max(upper - lower, 0.1)
  if (min !== undefined && min >= lower - range * 0.35 && min <= upper + range * 0.35) {
    lower = Math.min(lower, min)
    upper = Math.max(upper, min)
  }
  if (max !== undefined && max >= lower - range * 0.35 && max <= upper + range * 0.35) {
    lower = Math.min(lower, max)
    upper = Math.max(upper, max)
  }
  if (lower === upper) return [lower - 1, upper + 1]
  return [lower, upper]
}

function buildCurrentValueTrend(metric: ResolvedMetricCard): TrendPoint[] {
  if (metric.numericValue === undefined) return []
  const timestamp = parseTimestamp(metric.lastUpdate) ?? Date.now()
  return [{ time: formatTimeLabel(new Date(timestamp).toISOString()), value: metric.numericValue, timestamp, realtime: true }]
}

function formatLimit(value: number) {
  if (Math.abs(value) >= 10 || Number.isInteger(value)) return String(Math.round(value * 10) / 10)
  return value.toFixed(2)
}

function formatUnit(unit: string) {
  if (unit === 'degC') return '°C'
  if (unit === 'm3/h') return 'm³/h'
  return unit
}

type CockpitLightPosition = {
  pageX: number
  pageY: number
  viewportX: number
  viewportY: number
}

function useCockpitLight() {
  const [lightPos, setLightPos] = useState<CockpitLightPosition>({
    pageX: window.innerWidth / 2,
    pageY: window.innerHeight / 2,
    viewportX: window.innerWidth / 2,
    viewportY: window.innerHeight / 2,
  })

  useEffect(() => {
    let frame = 0
    const cycle = 20000
    const start = Date.now()

    const animate = () => {
      const pageRect = document.querySelector<HTMLElement>('.model-cockpit-page')?.getBoundingClientRect()
      if (!pageRect) {
        frame = window.requestAnimationFrame(animate)
        return
      }
      const progress = ((Date.now() - start) % cycle) / cycle
      const angle = progress * Math.PI * 2
      const x = pageRect.left + pageRect.width / 2 + Math.cos(angle) * pageRect.width * 0.35
      const y = pageRect.top + pageRect.height / 2 + Math.sin(angle) * pageRect.height * 0.25
      setLightPos({ pageX: x - pageRect.left, pageY: y - pageRect.top, viewportX: x, viewportY: y })
      document.querySelectorAll<HTMLElement>('.model-cockpit-page .glass-panel').forEach((panel) => {
        const rect = panel.getBoundingClientRect()
        panel.style.setProperty('--mouse-x', `${x - rect.left}px`)
        panel.style.setProperty('--mouse-y', `${y - rect.top}px`)
      })
      frame = window.requestAnimationFrame(animate)
    }

    frame = window.requestAnimationFrame(animate)
    return () => window.cancelAnimationFrame(frame)
  }, [])

  return lightPos
}

function usePageVisibility() {
  const [pageVisible, setPageVisible] = useState(() => !document.hidden)

  useEffect(() => {
    const handleVisibilityChange = () => {
      setPageVisible(!document.hidden)
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [])

  return pageVisible
}

// force reload
