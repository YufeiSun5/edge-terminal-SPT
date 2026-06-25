import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router'
import {
  Alert,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Segmented,
  Select,
  Switch,
  Table,
  Tag,
  message,
} from 'antd'
import { useTranslation } from 'react-i18next'
import {
  ChevronDown,
  ChevronUp,
  Droplets,
  Gauge,
  History,
  Power,
  Play,
  Square,
  Thermometer,
  Volume2,
  Waves,
  Wind,
  AlertTriangle,
  Database,
  Minus,
  Plus,
  SlidersHorizontal,
  Trash2,
} from 'lucide-react'
import type {
  DetectionRun,
  DetectionRunStandardItem,
  DetectionPlan,
  DetectionRunReportRequest,
  DetectionRunReportRequestPayload,
  DetectionRunStorageRoute,
  DetectionStandard,
  DetectionStandardItem,
  DetectionStandardItemPayload,
  HistoryDataItem,
  LimitAlarm,
  LimitAlarmScope,
  RealtimeWebSocketSubscription,
  RuntimeDraft,
  StationViewItem,
  StationViewItemPayload,
  StationViewResolvedBinding,
  TagSnapshot,
  TaskFlow,
  TaskFlowRun,
  VariableConfig,
  VariableWriteResult,
  VarIdentifier,
} from '@/shared/api/types'
import { useAuthStore } from '@/features/auth/authStore'
import {
  abnormalStopDetectionRun,
  getActiveDetectionRuns,
  getDetectionRun,
  getDetectionRuns,
  getDetectionRunReportRequests,
  getDetectionRunStorageRoutes,
  getDetectionPlans,
  getDetectionStandards,
  getProjects,
  getLimitAlarms,
  getRealtimeVariables,
  getReportTemplates,
  getRuntimeDraft,
  getStationViewEffective,
  getStationViewItems,
  getStationViewTemplates,
  getTaskFlows,
  getTaskFlowRuns,
  getVariables,
  putRuntimeDraft,
  replaceStationViewItems,
  startDetectionPlan,
  stopDetectionRun,
} from '@/features/edge-status/api'
import {
  RealtimeWebSocketCommandError,
  sendRealtimeWebSocketCommand,
} from '@/features/realtime/realtimeClient'
import { useRealtimeSnapshots } from '@/features/realtime/useRealtimeSnapshots'
import { getHistoryData } from '@/features/history-query/api'
import {
  DetectionConfigEditor,
  type StationDetectionConfigDraft,
} from '@/features/detection-config/DetectionConfigPage'
import { ApiError } from '@/shared/api/http'
import { detectionStandardScopeLabel } from '@/shared/detection/standardScope'
import { languageCode } from '@/shared/i18n/language'
import { StationCardGridStyles } from './components/StationCardGridStyles'
import { StationLightBackground } from './components/StationLightBackground'

type ChartAxisMode = 'standard' | 'auto'

type TrendPoint = {
  time: string
  value: number
  timestamp?: number
  realtime?: boolean
}

type MetricCard = {
  id: string
  itemUid?: string
  label: string
  unit: string
  color: string
  min?: number
  max?: number
  icon: ReactNode
  value?: number
  precision: number
  trend: TrendPoint[]
  axisMode: ChartAxisMode
}

type StationViewBindingWithItem = StationViewResolvedBinding & {
  item_uid?: string
  pinned?: boolean
}

type StartDetectionFormValues = {
  plan_id?: number
  project_id: number
  factory_no: string
  customer_name?: string
  device_model?: string
  test_no?: string
  mode: string
  config_enabled: boolean
  standard_id?: number
  report_requests?: ReportRequestFormRow[]
  duration_min?: number
  operator_note?: string
}

type ReportRequestFormRow = {
  template_id?: number
  report_name?: string
  var_ids?: Array<string | number>
  params_json?: string
}

const stationMetricCardLimit = 12
const stationLayoutAreaCardPool = 'card_pool'
const stationLayoutAreaListLayout = 'list_layout'
const startDetectionCommand = 'start_detection'
const startDetectionModule = 'builtin.start_detection_run'
const startDetectionConfirmAttempts = 8
const startDetectionConfirmIntervalMs = 500
const stationChartRealtimeThrottleMs = 2000
const stationDetectionPreloadNamespace = 'station.detection_preload'
const stationDetectionPreloadTTLSec = 24 * 60 * 60
const emptyRealtimeVarIds: VarIdentifier[] = []

type StationDetectionPreloadDraftData = {
  standard_id?: number
  config_code?: string
  config_name?: string
  config_version?: number
  config_hash?: string
  items?: DetectionStandardItemPayload[]
  process_params?: {
    inlet_area_m2?: number
  }
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function stationDraftFromRuntimeDraft(
  draft: RuntimeDraft<StationDetectionPreloadDraftData> | undefined,
  projectId: number | undefined,
): StationDetectionConfigDraft | undefined {
  if (!draft || projectId === undefined) return undefined
  return {
    projectId,
    standardId: draft.data.standard_id,
    configCode: draft.data.config_code,
    configName: draft.data.config_name,
    configVersion: draft.data.config_version,
    configHash: draft.data.config_hash,
    items: draft.data.items ?? [],
    processParams: draft.data.process_params ?? {},
    updatedAt: Date.parse(draft.updated_at) || Date.now(),
  }
}

function stationBindingDefaultOrder(
  binding: StationViewResolvedBinding,
  fallback: number,
) {
  return Number.isFinite(binding.sort_order) ? binding.sort_order : fallback
}

function stationBindingPinned(binding: StationViewResolvedBinding) {
  return (
    (binding as StationViewResolvedBinding & { pinned?: boolean }).pinned ===
    true
  )
}

function sortStationBindingsByDefaultOrder<
  T extends StationViewResolvedBinding,
>(bindings: T[]): T[] {
  return bindings
    .map((binding, index) => ({ binding, index }))
    .sort((a, b) => {
      const pinnedDiff =
        Number(stationBindingPinned(b.binding)) -
        Number(stationBindingPinned(a.binding))
      if (pinnedDiff !== 0) return pinnedDiff
      const orderDiff =
        stationBindingDefaultOrder(a.binding, a.index) -
        stationBindingDefaultOrder(b.binding, b.index)
      if (orderDiff !== 0) return orderDiff
      return a.index - b.index
    })
    .map((item) => item.binding)
}

function stationViewItemPayloadFromItem(
  item: StationViewItem,
): StationViewItemPayload {
  return {
    item_uid: item.item_uid,
    layout_area: item.layout_area,
    item_type: item.item_type,
    binding_type: item.binding_type,
    binding_key: item.binding_key,
    binding_json: item.binding_json,
    display_json: item.display_json,
    sort_order: item.sort_order,
    pinned: item.pinned,
    visible: item.visible,
  }
}

type AlarmScopeFilter = 'all' | LimitAlarmScope
type PIDWriteState = {
  status: 'idle' | 'pending' | 'ack' | 'sent' | 'error'
  message?: string
  submittedValue?: string
  submittedAt?: string
  result?: VariableWriteResult
}

type PIDSettingItem = {
  key: string
  labelKey: string
  unit?: string
  step: number
  precision: number
}

type PIDSettingGroup = {
  key: string
  titleKey: string
  items: PIDSettingItem[]
}

const pidSettingGroups: PIDSettingGroup[] = [
  {
    key: 'temperature',
    titleKey: 'station.pid.groups.temperature',
    items: [
      {
        key: 'SP1-WD',
        labelKey: 'station.pid.labels.temperatureSetpoint',
        unit: '℃',
        step: 0.1,
        precision: 1,
      },
      { key: 'P1', labelKey: 'station.pid.labels.p1', step: 0.1, precision: 1 },
      { key: 'I1', labelKey: 'station.pid.labels.i1', step: 1, precision: 0 },
      { key: 'D1', labelKey: 'station.pid.labels.d1', step: 1, precision: 0 },
    ],
  },
  {
    key: 'humidity',
    titleKey: 'station.pid.groups.humidity',
    items: [
      {
        key: 'SP2-SD',
        labelKey: 'station.pid.labels.humiditySetpoint',
        unit: '%',
        step: 0.1,
        precision: 1,
      },
      { key: 'P2', labelKey: 'station.pid.labels.p2', step: 0.1, precision: 1 },
      { key: 'I2', labelKey: 'station.pid.labels.i2', step: 1, precision: 0 },
      { key: 'D2', labelKey: 'station.pid.labels.d2', step: 1, precision: 0 },
    ],
  },
  {
    key: 'temperature2',
    titleKey: 'station.pid.groups.temperature2',
    items: [
      {
        key: 'SP2-WD',
        labelKey: 'station.pid.labels.temperatureSetpoint',
        unit: '℃',
        step: 0.1,
        precision: 1,
      },
      { key: 'P3', labelKey: 'station.pid.labels.p3', step: 0.1, precision: 1 },
      { key: 'I3', labelKey: 'station.pid.labels.i3', step: 1, precision: 0 },
      { key: 'D3', labelKey: 'station.pid.labels.d3', step: 1, precision: 0 },
    ],
  },
]

const cardColors = [
  '#c2410c',
  '#0f766e',
  '#2563eb',
  '#b45309',
  '#7c3aed',
  '#15803d',
  '#be185d',
  '#dc2626',
]

type StationTableRow = {
  key: string
  itemUid?: string
  pinned: boolean
  name: string
  standard: string
  value: string
  ok: boolean
}

function formatAlarmValue(value?: number | null) {
  return value === undefined || value === null
    ? '-'
    : Number(value)
        .toFixed(3)
        .replace(/\.?0+$/, '')
}

function formatAlarmTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function alarmDisplayName(
  alarm: Pick<
    LimitAlarm,
    'display_name' | 'display_name_en' | 'display_name_ja' | 'var_name'
  >,
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return alarm.display_name_en || alarm.var_name
  if (currentLanguage === 'ja') return alarm.display_name_ja || alarm.var_name
  return alarm.display_name || alarm.var_name
}

function variableDisplayName(
  variable: Pick<
    VariableConfig,
    'display_name' | 'display_name_en' | 'display_name_ja' | 'var_name'
  >,
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en')
    return variable.display_name_en || variable.var_name
  if (currentLanguage === 'ja')
    return variable.display_name_ja || variable.var_name
  return variable.display_name || variable.var_name
}

function bindingDisplayName(
  binding: StationViewResolvedBinding,
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en')
    return (
      binding.display_name_en ||
      binding.var_name ||
      binding.var_id_text ||
      String(binding.var_id ?? '')
    )
  if (currentLanguage === 'ja')
    return (
      binding.display_name_ja ||
      binding.var_name ||
      binding.var_id_text ||
      String(binding.var_id ?? '')
    )
  return (
    binding.display_name ||
    binding.var_name ||
    binding.var_id_text ||
    String(binding.var_id ?? '')
  )
}

function bindingWireId(
  binding: Pick<StationViewResolvedBinding, 'var_id' | 'var_id_text'>,
) {
  return binding.var_id_text ?? binding.var_id
}

function bindingKey(binding: StationViewResolvedBinding, index: number) {
  return String(
    bindingWireId(binding) ?? `${binding.source}-${binding.var_name ?? index}`,
  )
}

function snapshotKey(snapshot: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  return String(snapshot.var_id_text ?? snapshot.var_id)
}

function bindingLimits(binding: StationViewResolvedBinding) {
  return {
    min: binding.limit_l ?? binding.limit_ll ?? undefined,
    max: binding.limit_h ?? binding.limit_hh ?? undefined,
  }
}

function numericSnapshotValue(snapshot?: TagSnapshot) {
  if (!snapshot || snapshot.is_string || !Number.isFinite(snapshot.value))
    return undefined
  return snapshot.value
}

function standardItemKey(
  item: Pick<DetectionRunStandardItem | DetectionStandardItem | DetectionStandardItemPayload, 'var_id'> & { var_id_text?: string | number },
) {
  return String(item.var_id_text ?? item.var_id)
}

function formatMetricValue(
  value: number | undefined,
  unit: string | undefined,
  precision: number,
) {
  if (value === undefined) return '--'
  return `${value.toFixed(Math.max(0, Math.min(precision, 4)))}${unit ? ` ${unit}` : ''}`
}

function standardItemLimits(
  item?: DetectionRunStandardItem | DetectionStandardItem | DetectionStandardItemPayload,
) {
  return {
    min: item?.limit_l ?? item?.limit_ll ?? undefined,
    max: item?.limit_h ?? item?.limit_hh ?? undefined,
  }
}

function formatLimitRange(
  limits: { min?: number; max?: number },
  unitValue?: string,
) {
  const unit = unitValue ? ` ${unitValue}` : ''
  if (limits.min === undefined && limits.max === undefined) return '--'
  if (limits.min === undefined)
    return `<= ${formatAlarmValue(limits.max)}${unit}`
  if (limits.max === undefined)
    return `>= ${formatAlarmValue(limits.min)}${unit}`
  return `${formatAlarmValue(limits.min)} - ${formatAlarmValue(limits.max)}${unit}`
}

function formatStandardRange(
  binding: StationViewResolvedBinding,
  standardItem?: DetectionRunStandardItem | DetectionStandardItem | DetectionStandardItemPayload,
) {
  const overrideLimits = standardItemLimits(standardItem)
  const limits =
    overrideLimits.min !== undefined || overrideLimits.max !== undefined
      ? overrideLimits
      : bindingLimits(binding)
  return formatLimitRange(limits, binding.unit)
}

function isWithinLimits(
  value: number | undefined,
  binding: StationViewResolvedBinding,
  standardItem?: DetectionRunStandardItem | DetectionStandardItem | DetectionStandardItemPayload,
) {
  if (value === undefined) return true
  const overrideLimits = standardItemLimits(standardItem)
  const limits =
    overrideLimits.min !== undefined || overrideLimits.max !== undefined
      ? overrideLimits
      : bindingLimits(binding)
  if (limits.min !== undefined && value < limits.min) return false
  if (limits.max !== undefined && value > limits.max) return false
  return true
}

function trendFromValue(
  value: number | undefined,
  min?: number,
  max?: number,
  lastUpdate?: string,
): TrendPoint[] {
  const base = value ?? min ?? max ?? 0
  const timestamp = parseTimestamp(lastUpdate) ?? Date.now()
  return Array.from({ length: 7 }, (_, index) => ({
    time: String(index + 1),
    value: base,
    timestamp: timestamp + index,
    realtime: true,
  }))
}

function historyRefetchInterval({
  data,
  activeRun,
  pageVisible,
}: {
  data: unknown
  activeRun?: DetectionRun | null
  pageVisible: boolean
}) {
  if (!activeRun?.id) return false
  if (!pageVisible) return 60000
  if (historyResponseItemCount(data) > 0) return 10000
  const startedAt = parseTimestamp(activeRun.started_at)
  if (startedAt !== undefined && Date.now() - startedAt <= 60000) return 3000
  return 10000
}

function historyResponseItemCount(data: unknown) {
  if (!data || typeof data !== 'object') return 0
  const items = (data as { items?: unknown }).items
  return Array.isArray(items) ? items.length : 0
}

function groupHistoryByVarId(items: HistoryDataItem[]) {
  const groups = new Map<string, HistoryDataItem[]>()
  for (const item of items) {
    const key = String(item.var_id_text ?? item.var_id)
    const group = groups.get(key) ?? []
    group.push(item)
    groups.set(key, group)
  }
  return groups
}

function buildHistoryTrend(items: HistoryDataItem[], precision: number): TrendPoint[] {
  return items
    .filter((item) => typeof item.value === 'number' && Number.isFinite(item.value))
    .sort((left, right) => (parseTimestamp(left.source_time || left.created_at) ?? 0) - (parseTimestamp(right.source_time || right.created_at) ?? 0))
    .slice(-60)
    .map((item) => {
      const sourceTime = item.source_time || item.created_at
      return {
        time: formatTimeLabel(sourceTime),
        value: Number((item.value ?? 0).toFixed(Math.max(0, Math.min(precision, 4)))),
        timestamp: parseTimestamp(sourceTime),
      }
    })
}

function buildCardTrend({
  history,
  value,
  min,
  max,
  lastUpdate,
}: {
  history: TrendPoint[]
  value: number | undefined
  min?: number
  max?: number
  lastUpdate?: string
}) {
  const realtimeTrend = trendFromValue(value, min, max, lastUpdate)
  if (history.length === 0) return realtimeTrend
  if (value === undefined) return history
  const realtimePoint = realtimeTrend[realtimeTrend.length - 1]
  const lastHistoryPoint = history[history.length - 1]
  if (
    realtimePoint.timestamp !== undefined &&
    lastHistoryPoint.timestamp !== undefined &&
    realtimePoint.timestamp <= lastHistoryPoint.timestamp
  ) {
    return history
  }
  return [...history, realtimePoint].slice(-61)
}

function useThrottledValue<T>(value: T, intervalMs: number) {
  const [throttledValue, setThrottledValue] = useState(value)
  const latestValueRef = useRef(value)
  const lastUpdateAtRef = useRef(0)
  const timerRef = useRef<number | undefined>(undefined)

  useEffect(() => {
    latestValueRef.current = value
    const now = Date.now()
    const remaining = intervalMs - (now - lastUpdateAtRef.current)

    if (remaining <= 0) {
      if (timerRef.current !== undefined) {
        window.clearTimeout(timerRef.current)
        timerRef.current = undefined
      }
      lastUpdateAtRef.current = now
      setThrottledValue(value)
      return undefined
    }

    if (timerRef.current === undefined) {
      timerRef.current = window.setTimeout(() => {
        timerRef.current = undefined
        lastUpdateAtRef.current = Date.now()
        setThrottledValue(latestValueRef.current)
      }, remaining)
    }

    return undefined
  }, [intervalMs, value])

  useEffect(
    () => () => {
      if (timerRef.current !== undefined) {
        window.clearTimeout(timerRef.current)
      }
    },
    [],
  )

  return throttledValue
}

function buildChartDomain({
  chartData,
  min,
  max,
  axisMode,
}: {
  chartData: TrendPoint[]
  min?: number
  max?: number
  axisMode: ChartAxisMode
}): [number, number] {
  const dataValues = chartData.map((item) => item.value).filter((value) => Number.isFinite(value))
  const hasStandardRange = min !== undefined && max !== undefined && min < max
  if (dataValues.length === 0) {
    if (hasStandardRange) return [min, max]
    return [0, 1]
  }

  const focusedDomain = expandDomainWithNearbyLimits(
    paddedDomain(dataValues, axisMode === 'auto' ? 0.18 : 0.12),
    min,
    max,
  )
  if (hasStandardRange && axisMode === 'standard') {
    const standardRange = Math.abs(max - min)
    const focusedRange = Math.max(focusedDomain[1] - focusedDomain[0], 0.1)
    return standardRange > focusedRange * 6 ? focusedDomain : [min, max]
  }
  return focusedDomain
}

function paddedDomain(values: number[], ratio: number): [number, number] {
  const yMin = Math.min(...values)
  const yMax = Math.max(...values)
  const valueRange = Math.abs(yMax - yMin)
  const buffer = Math.max(valueRange * ratio, Math.abs(yMax) * 0.02, 1)
  return [Math.floor((yMin - buffer) * 10) / 10, Math.ceil((yMax + buffer) * 10) / 10]
}

function expandDomainWithNearbyLimits(
  domain: [number, number],
  min?: number,
  max?: number,
): [number, number] {
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

function parseTimestamp(value?: string) {
  if (!value || value.startsWith('0001-')) return undefined
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : undefined
}

function formatTimeLabel(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function iconForBinding(binding: StationViewResolvedBinding) {
  const text =
    `${binding.var_group ?? ''} ${binding.var_name ?? ''} ${binding.display_name ?? ''}`.toLowerCase()
  if (text.includes('temp') || text.includes('温')) return Thermometer
  if (text.includes('humid') || text.includes('湿')) return Droplets
  if (text.includes('wind') || text.includes('风')) return Wind
  if (text.includes('noise') || text.includes('噪')) return Volume2
  if (text.includes('vibration') || text.includes('振')) return Waves
  if (text.includes('power') || text.includes('功率')) return Power
  return Gauge
}

function standardItemToStationBinding(
  item:
    | DetectionRunStandardItem
    | DetectionStandardItem
    | StationDetectionConfigDraft['items'][number],
  index: number,
): StationViewBindingWithItem {
  const key = standardItemKey(item)
  return {
    source: 'detection_item',
    var_id: item.var_id,
    var_id_text: item.var_id_text ? String(item.var_id_text) : undefined,
    var_name: item.var_name,
    display_name: item.display_name,
    display_name_en: item.display_name_en,
    display_name_ja: item.display_name_ja,
    unit: item.unit,
    decimal_places: item.decimal_places ?? 2,
    limit_l: item.limit_l,
    limit_h: item.limit_h,
    limit_ll: item.limit_ll,
    limit_hh: item.limit_hh,
    check_enabled: item.check_enabled,
    alarm_enabled: item.alarm_enabled,
    sort_order: item.sort_order ?? index + 1,
    item_uid: `run-standard-${key}`,
  }
}

function mergeMetricBindings(
  templateBindings: StationViewBindingWithItem[],
  configBindings: StationViewBindingWithItem[],
) {
  const result: StationViewBindingWithItem[] = []
  const seen = new Set<string>()
  for (const binding of [...configBindings, ...templateBindings]) {
    const key = bindingWireId(binding)
    const uniqueKey =
      key !== undefined
        ? `var:${String(key)}`
        : `fallback:${binding.item_uid ?? binding.var_name ?? result.length}`
    if (seen.has(uniqueKey)) continue
    seen.add(uniqueKey)
    result.push(binding)
  }
  return result
}

function mergeTableBindings(
  templateBindings: StationViewBindingWithItem[],
  configBindings: StationViewBindingWithItem[],
) {
  const result: StationViewBindingWithItem[] = []
  const seen = new Set<string>()
  for (const binding of [...templateBindings, ...configBindings]) {
    const key = bindingWireId(binding)
    const uniqueKey =
      key !== undefined
        ? `var:${String(key)}`
        : `fallback:${binding.item_uid ?? binding.var_name ?? result.length}`
    if (seen.has(uniqueKey)) continue
    seen.add(uniqueKey)
    result.push(binding)
  }
  return result
}

function buildReportRequest(
  values: StartDetectionFormValues,
  fallbackVarIds: Array<string | number> = [],
): DetectionRunReportRequestPayload | undefined {
  const reports = (values.report_requests ?? [])
    .map((row) => {
      const varIds = (row.var_ids ?? []).filter(
        (item) => item !== undefined && item !== null && item !== '',
      )
      const effectiveVarIds = varIds.length > 0 ? varIds : fallbackVarIds
      if (effectiveVarIds.length === 0) return undefined
      const report: NonNullable<
        DetectionRunReportRequestPayload['reports']
      >[number] = {
        var_ids: effectiveVarIds,
      }
      if (row.template_id) report.template_id = row.template_id
      if (row.report_name?.trim()) report.report_name = row.report_name.trim()
      const paramsText = row.params_json?.trim()
      if (paramsText)
        report.params = JSON.parse(paramsText) as Record<string, unknown>
      return report
    })
    .filter(
      (
        item,
      ): item is NonNullable<
        DetectionRunReportRequestPayload['reports']
      >[number] => Boolean(item),
    )
  return reports.length > 0 ? { enabled: true, reports } : undefined
}

function parseTaskFlowSteps(flow: TaskFlow) {
  const raw = flow.steps_json?.trim()
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (Array.isArray(parsed)) return parsed as Array<{ module?: string }>
    if (typeof parsed === 'object' && parsed)
      return [parsed as { module?: string }]
  } catch {
    return []
  }
  return []
}

function taskFlowStartsDetection(flow: TaskFlow) {
  const condition = (flow.condition_script ?? '').replace(/\s+/g, '')
  const conditionMatches =
    condition === '' ||
    condition.includes(`task_params.command==="${startDetectionCommand}"`) ||
    condition.includes(`task_params.command==='${startDetectionCommand}'`)
  if (!conditionMatches) return false
  if (flow.action_type === startDetectionModule) return true
  return parseTaskFlowSteps(flow).some(
    (step) => step.module === startDetectionModule,
  )
}

function findStartDetectionRequestTarget(flows: TaskFlow[]) {
  for (const flow of flows) {
    if (!flow.enabled) continue
    if (flow.trigger_type !== 'data_change') continue
    if (!taskFlowStartsDetection(flow)) continue
    const variable = (flow.vars ?? []).find((item) => item.role === 'watch')
    if (variable) return { flow, variable }
  }
  return undefined
}

function nonEmptyText(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

function runtimeDraftMatchesStandard(
  draft: StationDetectionConfigDraft | undefined,
  standard: DetectionStandard | undefined,
) {
  if (!draft || !standard || draft.standardId !== standard.id) return false
  const draftHash = nonEmptyText(draft.configHash)
  const standardHash = nonEmptyText(standard.config_hash)
  if (draftHash || standardHash) return draftHash !== '' && draftHash === standardHash
  if (
    draft.configVersion !== undefined &&
    standard.version !== undefined &&
    draft.configVersion !== standard.version
  )
    return false
  return true
}

function runtimeDraftIsStaleForStandard(
  draft: StationDetectionConfigDraft | undefined,
  standard: DetectionStandard | undefined,
) {
  if (!draft || !standard || draft.standardId !== standard.id) return false
  const draftHash = nonEmptyText(draft.configHash)
  const standardHash = nonEmptyText(standard.config_hash)
  if (draftHash && standardHash && draftHash !== standardHash) return true
  return (
    draft.configVersion !== undefined &&
    standard.version !== undefined &&
    draft.configVersion !== standard.version
  )
}

async function waitForNewDetectionRun(
  projectId: number,
  previousMaxRunId: number,
) {
  for (let attempt = 0; attempt < startDetectionConfirmAttempts; attempt += 1) {
    const [activeRuns, recentRuns] = await Promise.all([
      getActiveDetectionRuns().catch(() => [] as DetectionRun[]),
      getDetectionRuns({ project_id: projectId, limit: 5 })
        .then((response) => response.items)
        .catch(() => [] as DetectionRun[]),
    ])
    const nextRun = [...activeRuns, ...recentRuns]
      .filter((run) => run.project_id === projectId && run.id > previousMaxRunId)
      .sort((a, b) => b.id - a.id)[0]
    if (nextRun) return nextRun
    await sleep(startDetectionConfirmIntervalMs)
  }
  return undefined
}

function parseRecordJSON(value: string | undefined) {
  if (!value) return undefined
  try {
    const parsed = JSON.parse(value) as unknown
    return typeof parsed === 'object' && parsed ? parsed as Record<string, unknown> : undefined
  } catch {
    return undefined
  }
}

function taskFlowRunMatchesRequest(run: TaskFlowRun, requestId: string) {
  const input = parseRecordJSON(run.input_snapshot)
  if (input?.request_id === requestId) return true
  const result = parseRecordJSON(run.result_json)
  const context = result?.context
  return (
    typeof context === 'object' &&
    context !== null &&
    (context as Record<string, unknown>).request_id === requestId
  )
}

function taskFlowRunFailureReason(run: TaskFlowRun) {
  const direct = nonEmptyText(run.error_message)
  if (direct) return direct
  const result = parseRecordJSON(run.result_json)
  const steps = Array.isArray(result?.steps) ? result.steps : []
  for (const step of steps) {
    if (typeof step !== 'object' || step === null) continue
    const error = nonEmptyText((step as Record<string, unknown>).error)
    if (error) return error
  }
  return ''
}

function isRuntimeDraftStaleError(reason: string) {
  const normalized = reason.toLowerCase()
  return normalized.includes('runtime draft') && normalized.includes('stale')
}

async function findFailedStartTaskFlowRun(
  projectId: number,
  flowId: number,
  triggerVarId: VarIdentifier,
  requestId: string,
  fromMs: number,
) {
  const runs = await getTaskFlowRuns({
    project_id: projectId,
    flow_id: flowId,
    trigger_var_id: triggerVarId,
    trigger_type: 'data_change',
    from: new Date(fromMs).toISOString(),
    limit: 20,
  })
    .then((response) => response.items)
    .catch(() => [] as TaskFlowRun[])
  return runs.find(
    (run) => run.status === 'failed' && taskFlowRunMatchesRequest(run, requestId),
  )
}

function tagWireId(variable: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  return variable.var_id_text ?? variable.var_id
}

function variableWireId(
  variable: Pick<VariableConfig, 'var_id' | 'var_id_text'>,
) {
  return variable.var_id_text ?? variable.var_id
}

function standardReportVarIds(
  standard?: Pick<DetectionStandard, 'items'> | { items?: DetectionStandardItemPayload[] },
  projectVariables: Array<Pick<VariableConfig, 'var_id' | 'var_id_text' | 'var_name'>> = [],
) {
  const seen = new Set<string>()
  return (standard?.items ?? [])
    .filter((item) => item.store_enabled || item.check_enabled)
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
    .map((item) => {
      const direct = projectVariables.find(
        (variable) =>
          String(variable.var_id_text ?? variable.var_id) ===
          String(item.var_id_text ?? item.var_id),
      )
      if (direct) return direct.var_id_text ?? direct.var_id
      const byName = projectVariables.find(
        (variable) => variable.var_name === item.var_name,
      )
      return byName ? byName.var_id_text ?? byName.var_id : item.var_id_text ?? item.var_id
    })
    .filter((value): value is string | number => {
      if (value === undefined || value === null || value === '') return false
      const key = String(value)
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
}

function normalizePIDKey(value?: string) {
  return (value ?? '').replace(/[^a-zA-Z0-9]/g, '').toUpperCase()
}

function isPIDWritable(variable: Pick<VariableConfig, 'writable' | 'rw_mode'>) {
  return variable.writable || variable.rw_mode.toUpperCase().includes('W')
}

function isBrokerAcceptedWithoutAck(result?: VariableWriteResult) {
  if (!result) return false
  const kioStatus = result.kio?.status
  return (
    (result.broker_accepted === true || result.kio?.broker_accepted === true) &&
    kioStatus === 'ack_timeout_or_unmatched'
  )
}

function isGatewayOfflineWriteResult(result?: VariableWriteResult) {
  if (!result) return false
  const kioStatus = result.kio?.status
  return (
    kioStatus === 'gateway_offline' ||
    (result.kio?.broker_accepted === false &&
      result.kio?.message?.toLowerCase().includes('gateway'))
  )
}

function isPIDReadbackMatch(
  submittedValue: string | undefined,
  currentValue: string,
) {
  const submitted = submittedValue?.trim()
  const current = currentValue.trim()
  if (!submitted || !current) return false
  const submittedNumber = Number(submitted)
  const currentNumber = Number(current)
  if (Number.isFinite(submittedNumber) && Number.isFinite(currentNumber))
    return Math.abs(submittedNumber - currentNumber) < 0.000001
  return submitted === current
}

function findPIDVariable(variables: VariableConfig[], key: string) {
  const normalizedKey = normalizePIDKey(key)
  return variables.find((variable) => {
    const candidates = [
      variable.var_name,
      variable.display_name,
      variable.display_name_en,
      variable.display_name_ja,
      variable.raw_name,
    ]
    return candidates.some(
      (candidate) => normalizePIDKey(candidate) === normalizedKey,
    )
  })
}

function formatPIDNumber(value: number, precision: number) {
  return value.toFixed(Math.max(0, Math.min(precision, 4)))
}

function pidDisplayValue(
  key: string,
  rawValue: number | undefined,
  decimalPlaces = 1,
) {
  if (rawValue === undefined) return ''
  if (key === 'SP2-SD') return formatPIDNumber(rawValue / 10, 1)
  if (key === 'SP1-WD' || key === 'SP2-WD') {
    const precision = Math.max(0, Math.min(decimalPlaces, 4))
    return formatPIDNumber(rawValue / Math.pow(10, precision), precision)
  }
  if (/^P\d+$/i.test(key)) return formatPIDNumber(rawValue / 10, 1)
  if (/^[ID]\d+$/i.test(key)) return formatPIDNumber(rawValue, 0)
  return formatPIDNumber(rawValue, 2)
}

function pidWriteValue(key: string, displayValue: string, decimalPlaces = 1) {
  const numericValue = Number(displayValue)
  if (!Number.isFinite(numericValue)) throw new Error('number')
  if (key === 'SP2-SD') return numericValue * 10
  if (key === 'SP1-WD' || key === 'SP2-WD') {
    const precision = Math.max(0, Math.min(decimalPlaces, 4))
    return numericValue * Math.pow(10, precision)
  }
  if (/^P\d+$/i.test(key)) return numericValue * 10
  return numericValue
}

function coerceWriteValue(
  variable: Pick<VariableConfig, 'data_type'>,
  rawValue: string,
) {
  const value = rawValue.trim()
  if (value === '') throw new Error('empty')
  const dataType = variable.data_type.toUpperCase()
  if (dataType === 'BOOL' || dataType === 'BOOLEAN') {
    if (['1', 'true', 'on', 'yes'].includes(value.toLowerCase())) return true
    if (['0', 'false', 'off', 'no'].includes(value.toLowerCase())) return false
    throw new Error('bool')
  }
  if (
    dataType === 'INT' ||
    dataType === 'INTEGER' ||
    dataType === 'FLOAT' ||
    dataType === 'DOUBLE' ||
    dataType === 'NUMBER'
  ) {
    const numericValue = Number(value)
    if (!Number.isFinite(numericValue)) throw new Error('number')
    return dataType === 'INT' || dataType === 'INTEGER'
      ? Math.trunc(numericValue)
      : numericValue
  }
  return value
}

function displayProjectName(
  project: {
    project_code?: string
    display_name?: string
    display_name_en?: string
    display_name_ja?: string
    name?: string
  },
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en')
    return project.display_name_en || project.project_code || ''
  if (currentLanguage === 'ja')
    return project.display_name_ja || project.project_code || ''
  return project.display_name || project.name || project.project_code || ''
}

function standardDisplayName(
  standard:
    | {
        standard_code: string
        display_name?: string
        display_name_en?: string
        display_name_ja?: string
        name?: string
      }
    | undefined,
  language?: string,
) {
  if (!standard) return ''
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en')
    return standard.display_name_en || standard.standard_code
  if (currentLanguage === 'ja')
    return standard.display_name_ja || standard.standard_code
  return standard.display_name || standard.name || standard.standard_code
}

function detectionPlanLabel(plan: DetectionPlan) {
  const itemName =
    plan.test_item_name || plan.test_item_code || plan.standard_code
  return [plan.factory_no, itemName, plan.plan_no].filter(Boolean).join(' / ')
}

export function StationOperationPage() {
  const { t, i18n } = useTranslation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [messageApi, messageContext] = message.useMessage()
  const [startForm] = Form.useForm<StartDetectionFormValues>()
  const configEnabled = Form.useWatch('config_enabled', startForm)
  const selectedPlanId = Form.useWatch('plan_id', startForm)
  const startProjectId = Form.useWatch('project_id', startForm)
  const selectedStandardId = Form.useWatch('standard_id', startForm)
  const watchedReportRequestRows = Form.useWatch('report_requests', startForm)
  const reportRequestRows = useMemo(
    () => watchedReportRequestRows ?? [],
    [watchedReportRequestRows],
  )
  const [startModalOpen, setStartModalOpen] = useState(false)
  const [configPreviewOpen, setConfigPreviewOpen] = useState(false)
  const [alarmModalOpen, setAlarmModalOpen] = useState(false)
  const [pidModalOpen, setPIDModalOpen] = useState(false)
  const [previewCardOrder, setPreviewCardOrder] = useState<string[]>([])
  const [cardAxisState, setCardAxisState] = useState<{
    scope: string
    modes: Record<string, ChartAxisMode>
  }>({ scope: '', modes: {} })
  const [previewPinnedRows, setPreviewPinnedRows] = useState<
    Record<string, boolean>
  >({})
  const [pidVarGroup, setPIDVarGroup] = useState('')
  const [pidWriteValues, setPIDWriteValues] = useState<Record<string, string>>(
    {},
  )
  const [pidWriteStates, setPIDWriteStates] = useState<
    Record<string, PIDWriteState>
  >({})
  const [storageSnapshotOpen, setStorageSnapshotOpen] = useState(false)
  const [runSnapshotOpen, setRunSnapshotOpen] = useState(false)
  const [alarmScope, setAlarmScope] = useState<AlarmScopeFilter>('all')
  const hasPermission = useAuthStore((state) => state.hasPermission)
  const canStartDetection = hasPermission('start_detection')
  const canStopDetection = hasPermission('stop_detection')
  const pageVisible = usePageVisibility()
  const selectedProjectId = Number(searchParams.get('project_id'))
  const validSelectedProjectId =
    Number.isFinite(selectedProjectId) && selectedProjectId > 0
      ? selectedProjectId
      : undefined
  const selectedEdgeInstanceId =
    searchParams.get('edge_instance_id') || undefined
  const effectiveStartProjectId =
    Number.isFinite(Number(startProjectId)) && Number(startProjectId) > 0
      ? Number(startProjectId)
      : validSelectedProjectId
  const runtimeDraftQueryKey = [
    'station',
    'runtime-draft',
    stationDetectionPreloadNamespace,
    validSelectedProjectId,
    selectedEdgeInstanceId,
  ]
  const stationViewQuery = useQuery({
    queryKey: [
      'station',
      'view-effective',
      validSelectedProjectId,
      selectedEdgeInstanceId,
    ],
    queryFn: () =>
      getStationViewEffective(validSelectedProjectId!, selectedEdgeInstanceId),
    enabled: validSelectedProjectId !== undefined,
    refetchInterval: 10000,
    retry: false,
  })
  const stationViewTemplatesQuery = useQuery({
    queryKey: ['station', 'view-templates'],
    queryFn: () => getStationViewTemplates({ status: 'published' }),
    staleTime: 30000,
    retry: false,
  })
  const stationViewItemsQuery = useQuery({
    queryKey: ['station', 'view-items', validSelectedProjectId],
    queryFn: () => getStationViewItems({ project_id: validSelectedProjectId! }),
    enabled: validSelectedProjectId !== undefined,
    retry: false,
  })
  const stationRealtimeVarIds = stationViewQuery.data?.ws_subscription.var_ids ?? emptyRealtimeVarIds
  const stationRealtimeVarIdsKey = stationRealtimeVarIds.map((value) => String(value)).join(',')
  const stationRealtimeSubscription = useMemo<RealtimeWebSocketSubscription>(
    () => ({
      topics: ['realtime.variables'],
      edge_instance_id: selectedEdgeInstanceId,
      project_id: validSelectedProjectId,
      var_ids: stationRealtimeVarIds,
    }),
    [selectedEdgeInstanceId, stationRealtimeVarIds, validSelectedProjectId],
  )
  const stationRealtime = useRealtimeSnapshots({
    enabled: validSelectedProjectId !== undefined,
    subscription: stationRealtimeSubscription,
    fallbackQueryKey: [
      'edge',
      'realtime-variables',
      validSelectedProjectId,
      selectedEdgeInstanceId,
      stationRealtimeVarIdsKey,
    ],
    fallbackQueryFn: () =>
      getRealtimeVariables(
        validSelectedProjectId
          ? {
              project_id: validSelectedProjectId,
              edge_instance_id: selectedEdgeInstanceId,
              var_id: stationRealtimeVarIds.length > 0 ? stationRealtimeVarIds : undefined,
            }
          : {},
      ),
    fallbackIntervalMs: 2000,
    uiCommitMs: 500,
  })
  const runtimeDraftQuery = useQuery<
    RuntimeDraft<StationDetectionPreloadDraftData> | undefined
  >({
    queryKey: runtimeDraftQueryKey,
    queryFn: async () => {
      try {
        return await getRuntimeDraft<StationDetectionPreloadDraftData>(
          stationDetectionPreloadNamespace,
          {
            scope_type: 'project',
            scope_id: String(validSelectedProjectId!),
            project_id: validSelectedProjectId!,
            edge_instance_id: selectedEdgeInstanceId,
          },
        )
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          return undefined
        }
        throw error
      }
    },
    enabled: validSelectedProjectId !== undefined,
    staleTime: 5000,
    retry: false,
  })
  const pidVariablesQuery = useQuery({
    queryKey: [
      'station',
      'pid-variables',
      validSelectedProjectId,
      selectedEdgeInstanceId,
      pidVarGroup,
    ],
    queryFn: () =>
      getVariables({
        edge_instance_id: selectedEdgeInstanceId,
        project_id: validSelectedProjectId,
        var_group: pidVarGroup.trim() || undefined,
        enabled: true,
      }),
    enabled: pidModalOpen && validSelectedProjectId !== undefined,
    staleTime: 10000,
    retry: false,
  })
  const activeRunsQuery = useQuery({
    queryKey: ['edge', 'active-runs'],
    queryFn: getActiveDetectionRuns,
    refetchInterval: 3000,
    retry: false,
  })
  const projectsQuery = useQuery({
    queryKey: ['edge', 'projects'],
    queryFn: getProjects,
    refetchInterval: 8000,
    retry: false,
  })
  const standardsQuery = useQuery({
    queryKey: ['station', 'detection-standards'],
    queryFn: () => getDetectionStandards({ enabled: true }),
    staleTime: 30000,
    retry: false,
  })
  const configVariablesQuery = useQuery({
    queryKey: ['station', 'config-variables', validSelectedProjectId, selectedEdgeInstanceId],
    queryFn: () =>
      getVariables({
        edge_instance_id: selectedEdgeInstanceId,
        project_id: validSelectedProjectId,
        enabled: true,
      }),
    enabled: configPreviewOpen && validSelectedProjectId !== undefined,
    staleTime: 30000,
    retry: false,
  })
  const reportTemplatesQuery = useQuery({
    queryKey: ['station', 'report-templates'],
    queryFn: () => getReportTemplates({ enabled: true }),
    staleTime: 30000,
    retry: false,
  })
  const detectionPlansQuery = useQuery({
    queryKey: ['station', 'detection-plans', 'pending'],
    queryFn: () => getDetectionPlans({ status: 'pending', limit: 300 }),
    enabled: startModalOpen,
    refetchInterval: startModalOpen ? 10000 : false,
    retry: false,
  })
  const startTaskFlowsQuery = useQuery({
    queryKey: ['station', 'start-task-flows', effectiveStartProjectId],
    queryFn: () =>
      getTaskFlows({
        project_id: effectiveStartProjectId,
        trigger_type: 'data_change',
        enabled: true,
      }),
    enabled: startModalOpen && effectiveStartProjectId !== undefined,
    staleTime: 10000,
    retry: false,
  })
  const alarmsQuery = useQuery({
    queryKey: ['station', 'limit-alarms', validSelectedProjectId, alarmScope],
    queryFn: () =>
      getLimitAlarms({
        limit: 100,
        ...(validSelectedProjectId
          ? { project_id: validSelectedProjectId }
          : {}),
        ...(alarmScope === 'all' ? {} : { scope: alarmScope }),
      }),
    enabled: alarmModalOpen,
    refetchInterval: alarmModalOpen ? 5000 : false,
    retry: false,
  })
  const pidVariables = useMemo(
    () => pidVariablesQuery.data ?? [],
    [pidVariablesQuery.data],
  )
  const pidVarIds = useMemo(
    () =>
      pidVariables.map(variableWireId).filter((value) => value !== undefined),
    [pidVariables],
  )
  const pidSubscriptionKey = `${validSelectedProjectId ?? ''}:${pidVarIds.join(',')}`
  const pidRealtimeSubscription = useMemo<RealtimeWebSocketSubscription>(
    () => ({
      topics: ['realtime.variables'],
      edge_instance_id: selectedEdgeInstanceId,
      project_id: validSelectedProjectId,
      var_ids: pidVarIds,
    }),
    [pidVarIds, selectedEdgeInstanceId, validSelectedProjectId],
  )
  const pidRealtime = useRealtimeSnapshots({
    enabled: pidModalOpen && validSelectedProjectId !== undefined && pidVarIds.length > 0,
    subscription: pidRealtimeSubscription,
    fallbackQueryKey: [
      'station',
      'pid-realtime',
      validSelectedProjectId,
      selectedEdgeInstanceId,
      pidSubscriptionKey,
    ],
    fallbackQueryFn: () =>
      getRealtimeVariables({
        project_id: validSelectedProjectId!,
        edge_instance_id: selectedEdgeInstanceId,
        var_id: pidVarIds,
      }),
    fallbackIntervalMs: 2000,
    uiCommitMs: 500,
  })
  const variables = useMemo(() => stationRealtime.snapshots, [stationRealtime.snapshots])
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === validSelectedProjectId),
    [projects, validSelectedProjectId],
  )
  const stationVariables = useMemo(
    () =>
      validSelectedProjectId
        ? variables.filter(
            (variable) => variable.project_id === validSelectedProjectId,
          )
        : variables,
    [validSelectedProjectId, variables],
  )
  const activeRun = validSelectedProjectId
    ? activeRunsQuery.data?.find(
        (run) => run.project_id === validSelectedProjectId,
      )
    : activeRunsQuery.data?.[0]
  const currentRunDetailQuery = useQuery({
    queryKey: ['station', 'current-run-detail', activeRun?.id],
    queryFn: () => getDetectionRun(activeRun!.id),
    enabled: activeRun !== undefined,
    refetchInterval: activeRun !== undefined ? 10000 : false,
    retry: false,
  })
  const stationHistoryQuery = useQuery({
    queryKey: ['station', 'card-history', validSelectedProjectId, activeRun?.id],
    queryFn: () =>
      getHistoryData({
        project_id: validSelectedProjectId!,
        task_id: activeRun!.id,
        limit: 500,
      }),
    enabled: validSelectedProjectId !== undefined && activeRun !== undefined,
    refetchInterval: (query) =>
      historyRefetchInterval({
        data: query.state.data,
        activeRun: currentRunDetailQuery.data,
        pageVisible,
      }),
    retry: false,
  })
  const cardAxisScope = `${validSelectedProjectId ?? 'none'}:${activeRun?.id ?? 'none'}`
  const cardAxisModes = useMemo(
    () => (cardAxisState.scope === cardAxisScope ? cardAxisState.modes : {}),
    [cardAxisScope, cardAxisState.modes, cardAxisState.scope],
  )
  const storageSnapshotQuery = useQuery({
    queryKey: ['station', 'run-storage-routes', activeRun?.id],
    queryFn: () => getDetectionRunStorageRoutes(activeRun!.id),
    enabled: storageSnapshotOpen && activeRun !== undefined,
    refetchInterval: storageSnapshotOpen ? 10000 : false,
    retry: false,
  })
  const runSnapshotQuery = useQuery({
    queryKey: ['station', 'run-snapshot', activeRun?.id],
    queryFn: () => getDetectionRun(activeRun!.id),
    enabled: runSnapshotOpen && activeRun !== undefined,
    refetchInterval: runSnapshotOpen ? 10000 : false,
    retry: false,
  })
  const reportRequestsQuery = useQuery({
    queryKey: ['station', 'run-report-requests', activeRun?.id],
    queryFn: () => getDetectionRunReportRequests(activeRun!.id),
    enabled: runSnapshotOpen && activeRun !== undefined,
    refetchInterval: runSnapshotOpen ? 10000 : false,
    retry: false,
  })
  const standards = useMemo(
    () => standardsQuery.data ?? [],
    [standardsQuery.data],
  )
  const reportTemplates = useMemo(
    () => reportTemplatesQuery.data ?? [],
    [reportTemplatesQuery.data],
  )
  const pendingPlans = useMemo(
    () => detectionPlansQuery.data?.items ?? [],
    [detectionPlansQuery.data?.items],
  )
  const availableStandards = useMemo(
    () => standards,
    [standards],
  )
  const previewConfig = useMemo(
    () => stationDraftFromRuntimeDraft(runtimeDraftQuery.data, validSelectedProjectId),
    [runtimeDraftQuery.data, validSelectedProjectId],
  )
  const previewStandard = useMemo(
    () =>
      previewConfig
        ? availableStandards.find(
            (standard) => standard.id === previewConfig.standardId,
          )
        : undefined,
    [availableStandards, previewConfig],
  )
  const previewConfigStale = useMemo(
    () => runtimeDraftIsStaleForStandard(previewConfig, previewStandard),
    [previewConfig, previewStandard],
  )
  const effectivePreviewConfig = previewConfigStale ? undefined : previewConfig
  const selectedStartStandard = useMemo(
    () => availableStandards.find((standard) => standard.id === selectedStandardId),
    [availableStandards, selectedStandardId],
  )
  const selectedStartDraft =
    startProjectId === previewConfig?.projectId &&
    selectedStartStandard?.id === previewConfig?.standardId
      ? previewConfig
      : undefined
  const selectedStartDraftStale = runtimeDraftIsStaleForStandard(
    selectedStartDraft,
    selectedStartStandard,
  )
  const selectedStartStandardForReport = useMemo(
    () =>
      selectedStartStandard &&
      effectivePreviewConfig?.standardId === selectedStartStandard.id
        ? { ...selectedStartStandard, items: effectivePreviewConfig.items }
        : selectedStartStandard,
    [effectivePreviewConfig, selectedStartStandard],
  )
  const selectedStandardReportVarIds = useMemo(
    () => standardReportVarIds(selectedStartStandardForReport, stationVariables),
    [selectedStartStandardForReport, stationVariables],
  )
  const hasReportRowsMissingVariables = useMemo(
    () =>
      reportRequestRows.some(
        (row: ReportRequestFormRow) => !row.var_ids || row.var_ids.length === 0,
      ),
    [reportRequestRows],
  )
  const selectedPlan = useMemo(
    () => pendingPlans.find((plan) => plan.id === selectedPlanId),
    [pendingPlans, selectedPlanId],
  )
  const selectedProjectName = useMemo(() => {
    if (!selectedProject) return undefined
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en')
      return selectedProject.display_name_en || selectedProject.project_code
    if (currentLanguage === 'ja')
      return selectedProject.display_name_ja || selectedProject.project_code
    return (
      selectedProject.display_name ||
      selectedProject.name ||
      selectedProject.project_code
    )
  }, [i18n.resolvedLanguage, selectedProject])
  const [isStatusCollapsed, setStatusCollapsed] = useState(false)
  const snapshotsByVarID = useMemo(() => {
    const result = new Map<string, TagSnapshot>()
    for (const variable of stationVariables) {
      result.set(snapshotKey(variable), variable)
    }
    return result
  }, [stationVariables])
  const chartSnapshotsByVarID = useThrottledValue(
    snapshotsByVarID,
    stationChartRealtimeThrottleMs,
  )
  const pidSnapshotsByVarID = useMemo(() => {
    const result = new Map<string, TagSnapshot>(snapshotsByVarID)
    for (const variable of pidRealtime.snapshots) {
      result.set(snapshotKey(variable), variable)
    }
    return result
  }, [pidRealtime.snapshots, snapshotsByVarID])
  const rawStationViewItems = stationViewItemsQuery.data?.items ?? []
  const templateMetricBindings = useMemo<StationViewBindingWithItem[]>(
    () =>
      (stationViewQuery.data?.items ?? [])
        .filter(
          (item) =>
            item.layout_area === stationLayoutAreaCardPool &&
            item.visible !== false,
        )
        .flatMap((item) =>
          (item.resolved_bindings ?? []).map((binding) => ({
            ...binding,
            item_uid: item.item_uid,
            pinned: previewPinnedRows[item.item_uid] ?? item.pinned,
            sort_order: item.sort_order,
          })),
        ),
    [previewPinnedRows, stationViewQuery.data],
  )
  const runMetricBindings = useMemo<StationViewBindingWithItem[]>(
    () =>
      (currentRunDetailQuery.data?.standard_items ?? [])
        .filter((item) => item.check_enabled || item.store_enabled)
        .map((item, index) => standardItemToStationBinding(item, index)),
    [currentRunDetailQuery.data?.standard_items],
  )
  const previewConfigBindings = useMemo<StationViewBindingWithItem[]>(
    () =>
      (effectivePreviewConfig?.items ?? previewStandard?.items ?? [])
        .filter((item) => item.check_enabled !== false || item.store_enabled === true)
        .map((item, index) => standardItemToStationBinding(item, index)),
    [effectivePreviewConfig?.items, previewStandard?.items],
  )
  const displayConfigBindings = activeRun
    ? runMetricBindings
    : previewConfigBindings
  const metricBindings = useMemo(
    () => {
      const sortedTemplateBindings =
        sortStationBindingsByDefaultOrder(templateMetricBindings)
      return mergeMetricBindings(sortedTemplateBindings, displayConfigBindings).slice(
        0,
        stationMetricCardLimit,
      )
    },
    [displayConfigBindings, templateMetricBindings],
  )
  const defaultCardIds = useMemo(
    () => metricBindings.map((binding, index) => bindingKey(binding, index)),
    [metricBindings],
  )
  useEffect(() => {
    setPreviewCardOrder([])
    setPreviewPinnedRows({})
  }, [
    selectedEdgeInstanceId,
    validSelectedProjectId,
    stationViewQuery.data?.template.template_uid,
  ])
  const cardOrder = useMemo(() => {
    const next = previewCardOrder.filter((id) => defaultCardIds.includes(id))
    for (const id of defaultCardIds) {
      if (!next.includes(id)) next.push(id)
    }
    return next.length > 0 ? next : defaultCardIds
  }, [defaultCardIds, previewCardOrder])
  const bindingByCardId = useMemo(() => {
    const result = new Map<string, StationViewBindingWithItem>()
    metricBindings.forEach((binding, index) =>
      result.set(bindingKey(binding, index), binding),
    )
    return result
  }, [metricBindings])
  const standardItemByVarId = useMemo(() => {
    const result = new Map<string, DetectionRunStandardItem>()
    for (const item of currentRunDetailQuery.data?.standard_items ?? []) {
      result.set(standardItemKey(item), item)
    }
    return result
  }, [currentRunDetailQuery.data?.standard_items])
  const previewStandardItemByVarId = useMemo(() => {
    const result = new Map<string, DetectionStandardItem | StationDetectionConfigDraft['items'][number]>()
    for (const item of effectivePreviewConfig?.items ?? previewStandard?.items ?? []) {
      result.set(standardItemKey(item), item)
    }
    return result
  }, [effectivePreviewConfig?.items, previewStandard?.items])
  const displayStandardItemByVarId = activeRun
    ? standardItemByVarId
    : previewStandardItemByVarId
  const historyByVarId = useMemo(
    () => groupHistoryByVarId(stationHistoryQuery.data?.items ?? []),
    [stationHistoryQuery.data?.items],
  )
  const cards = useMemo<MetricCard[]>(
    () =>
      cardOrder
        .map((id, index) => {
          const binding = bindingByCardId.get(id)
          if (!binding) return undefined
          const chartSnapshot =
            bindingWireId(binding) !== undefined
              ? chartSnapshotsByVarID.get(String(bindingWireId(binding)))
              : undefined
          const chartValue = numericSnapshotValue(chartSnapshot)
          const standardItem =
            bindingWireId(binding) !== undefined
              ? displayStandardItemByVarId.get(String(bindingWireId(binding)))
              : undefined
          const bindingLimitValues = bindingLimits(binding)
          const limits = {
            min:
              standardItem?.limit_l ??
              standardItem?.limit_ll ??
              bindingLimitValues.min,
            max:
              standardItem?.limit_h ??
              standardItem?.limit_hh ??
              bindingLimitValues.max,
          }
          const precision =
            standardItem?.decimal_places ?? binding.decimal_places ?? 2
          const history =
            bindingWireId(binding) !== undefined
              ? buildHistoryTrend(
                  historyByVarId.get(String(bindingWireId(binding))) ?? [],
                  precision,
                )
              : []
          const Icon = iconForBinding(binding)
          return {
            id,
            itemUid: binding.item_uid,
            label: bindingDisplayName(binding, i18n.resolvedLanguage),
            unit: binding.unit ?? '',
            color: cardColors[index % cardColors.length],
            icon: <Icon size={15} />,
            value: chartValue,
            precision,
            trend: buildCardTrend({
              history,
              value: chartValue,
              min: limits.min,
              max: limits.max,
              lastUpdate: chartSnapshot?.last_update,
            }),
            axisMode: cardAxisModes[id] ?? 'standard',
            ...(limits.min !== undefined ? { min: limits.min } : {}),
            ...(limits.max !== undefined ? { max: limits.max } : {}),
          }
        })
        .filter((card) => card !== undefined),
    [
      bindingByCardId,
      cardAxisModes,
      cardOrder,
      chartSnapshotsByVarID,
      historyByVarId,
      i18n.resolvedLanguage,
      displayStandardItemByVarId,
    ],
  )

  const templateTableBindings = useMemo<StationViewBindingWithItem[]>(
    () =>
      (stationViewQuery.data?.items ?? [])
        .filter(
          (item) =>
            item.layout_area === stationLayoutAreaListLayout &&
            item.visible !== false,
        )
        .flatMap((item) =>
          (item.resolved_bindings ?? []).map((binding) => ({
            ...binding,
            item_uid: item.item_uid,
            pinned: previewPinnedRows[item.item_uid] ?? item.pinned,
            sort_order: item.sort_order,
          })),
        ),
    [previewPinnedRows, stationViewQuery.data],
  )
  const tableBindings = useMemo(
    () =>
      mergeTableBindings(
        sortStationBindingsByDefaultOrder(templateTableBindings),
        displayConfigBindings,
      ),
    [displayConfigBindings, templateTableBindings],
  )
  const stationRows = useMemo<StationTableRow[]>(
    () =>
      tableBindings.map((binding, index) => {
        const key = bindingKey(binding, index)
        const snapshot =
          bindingWireId(binding) !== undefined
            ? snapshotsByVarID.get(String(bindingWireId(binding)))
            : undefined
        const value = numericSnapshotValue(snapshot)
        const standardItem =
          bindingWireId(binding) !== undefined
            ? displayStandardItemByVarId.get(String(bindingWireId(binding)))
            : undefined
        return {
          key,
          itemUid: binding.item_uid,
          pinned: binding.pinned === true,
          name: bindingDisplayName(binding, i18n.resolvedLanguage),
          standard: formatStandardRange(binding, standardItem),
          value: formatMetricValue(
            value,
            binding.unit ?? '',
            standardItem?.decimal_places ?? binding.decimal_places ?? 2,
          ),
          ok: isWithinLimits(value, binding, standardItem),
        }
      }),
    [
      displayStandardItemByVarId,
      i18n.resolvedLanguage,
      snapshotsByVarID,
      tableBindings,
    ],
  )
  const sortedStationRows = stationRows
  const alarmOn = stationRows.some((row) => !row.ok)

  const refreshRuns = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['edge', 'active-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['station', 'current-run'] }),
      queryClient.invalidateQueries({
        queryKey: ['station', 'view-effective'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['station', 'detection-runs'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['station', 'detection-plans'],
      }),
      queryClient.invalidateQueries({
        queryKey: ['history', 'detection-plans'],
      }),
      queryClient.invalidateQueries({ queryKey: ['history', 'data'] }),
    ])
  }

  const saveRuntimeDraftMutation = useMutation({
    mutationFn: (draft: StationDetectionConfigDraft) =>
      putRuntimeDraft<StationDetectionPreloadDraftData>(
        stationDetectionPreloadNamespace,
        {
          scope_type: 'project',
          scope_id: String(draft.projectId),
          expected_revision: runtimeDraftQuery.data?.revision ?? 0,
          ttl_sec: stationDetectionPreloadTTLSec,
          data: {
            standard_id: draft.standardId,
            config_code: draft.configCode,
            config_name: draft.configName,
            config_version: draft.configVersion,
            config_hash: draft.configHash,
            items: draft.items,
            process_params: draft.processParams,
          },
        },
      ),
    onSuccess: (saved, draft) => {
      queryClient.setQueryData(runtimeDraftQueryKey, saved)
      const nextStandard = availableStandards.find(
        (standard) => standard.id === draft.standardId,
      )
      const draftStandard = nextStandard
        ? { ...nextStandard, items: draft.items }
        : undefined
      const nextVarIds = standardReportVarIds(draftStandard, stationVariables)
      if (!activeRun) {
        startForm.setFieldsValue({
          project_id: draft.projectId,
          mode: nextStandard?.mode ?? startForm.getFieldValue('mode'),
          config_enabled: true,
          standard_id: nextStandard?.id,
        })
        fillEmptyReportVariables(nextVarIds)
      }
      messageApi.success(t('station.configPreview.applied'))
      setConfigPreviewOpen(false)
    },
    onError: (error) => {
      messageApi.error(
        error instanceof Error ? error.message : t('station.config.saveFailed'),
      )
    },
  })

  const saveStationViewItemsMutation = useMutation({
    mutationFn: (items: StationViewItemPayload[]) => {
      const templateUID =
        stationViewItemsQuery.data?.template_uid ??
        stationViewQuery.data?.template.template_uid
      if (!templateUID) throw new Error(t('station.config.noTemplate'))
      return replaceStationViewItems({ template_uid: templateUID, items })
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['station', 'view-effective'],
        }),
        queryClient.invalidateQueries({ queryKey: ['station', 'view-items'] }),
      ])
      setPreviewPinnedRows({})
    },
    onError: (error) => {
      setPreviewPinnedRows({})
      messageApi.error(
        error instanceof Error ? error.message : t('station.config.saveFailed'),
      )
    },
  })

  function saveStationViewItems(
    items: StationViewItem[],
    options?: Parameters<typeof saveStationViewItemsMutation.mutate>[1],
  ) {
    saveStationViewItemsMutation.mutate(
      items.map(stationViewItemPayloadFromItem),
      options,
    )
  }

  function persistCardOrder(ids: string[], previousOrder: string[]) {
    if (rawStationViewItems.length === 0) {
      setPreviewCardOrder(previousOrder)
      return
    }
    const itemUIDsByCardID = new Map<string, string>(
      cards.flatMap(
        (card): Array<[string, string]> =>
          card.itemUid ? [[card.id, card.itemUid]] : [],
      ),
    )
    const orderedItemUIDs = ids
      .map((id) => itemUIDsByCardID.get(id))
      .filter((value): value is string => Boolean(value))
    if (orderedItemUIDs.length === 0) {
      setPreviewCardOrder(previousOrder)
      return
    }
    const nextItems = rawStationViewItems.map((item) => {
      if (item.layout_area !== stationLayoutAreaCardPool) return item
      const orderIndex = orderedItemUIDs.indexOf(item.item_uid)
      if (orderIndex === -1) return item
      return { ...item, sort_order: (orderIndex + 1) * 10 }
    })
    saveStationViewItems(nextItems, {
      onError: () => setPreviewCardOrder(previousOrder),
    })
  }

  function handleCardOrderCommit(ids: string[]) {
    const previousOrder = cardOrder
    setPreviewCardOrder(ids)
    persistCardOrder(ids, previousOrder)
  }

  const handleToggleCardAxisMode = useCallback(
    (cardId: string) =>
      setCardAxisState((current) => {
        const modes = current.scope === cardAxisScope ? current.modes : {}
        return {
          scope: cardAxisScope,
          modes: {
            ...modes,
            [cardId]:
              (modes[cardId] ?? 'standard') === 'standard'
                ? 'auto'
                : 'standard',
          },
        }
      }),
    [cardAxisScope],
  )

  const startRunMutation = useMutation({
    mutationFn: async (values: StartDetectionFormValues) => {
      const requestTarget = findStartDetectionRequestTarget(
        startTaskFlowsQuery.data ?? [],
      )
      if (!requestTarget) {
        throw new Error(t('station.start.errors.startTaskFlowRequired'))
      }
      const requestVariable = requestTarget.variable
      if (values.plan_id) {
        const response = await startDetectionPlan(values.plan_id, {
          project_id: values.project_id,
          operator_note: values.operator_note?.trim() || undefined,
          request_var_name: requestVariable.var_name,
        })
        return response.task
      }
      const selectedStandard = availableStandards.find(
        (standard) => standard.id === values.standard_id,
      )
      const configEnabled = values.config_enabled === true
      const selectedDraft =
        values.project_id === previewConfig?.projectId && selectedStandard
          ? previewConfig
          : undefined
      const draftMatchesStandard =
        configEnabled && runtimeDraftMatchesStandard(selectedDraft, selectedStandard)
      if (
        configEnabled &&
        selectedDraft?.standardId !== undefined &&
        selectedDraft.standardId === selectedStandard?.id &&
        !draftMatchesStandard
      ) {
        throw new Error(t('station.start.errors.staleRuntimeDraft'))
      }
      const requestStandardForReport =
        draftMatchesStandard && selectedStandard && selectedDraft
          ? { ...selectedStandard, items: selectedDraft.items }
          : selectedStandard
      const requestProjectVariables = variables.filter(
        (variable) => variable.project_id === values.project_id,
      )
      const defaultReportVarIds = standardReportVarIds(
        requestStandardForReport,
        requestProjectVariables,
      )
      const taskRequest = {
        command: 'start_detection',
        project_id: values.project_id,
        factory_no: values.factory_no.trim(),
        customer_name: values.customer_name?.trim() || undefined,
        device_model: values.device_model?.trim() || undefined,
        test_no: values.test_no?.trim() || undefined,
        mode: values.mode,
        standard_id: configEnabled && !draftMatchesStandard ? values.standard_id : undefined,
        config_enabled: configEnabled,
        config_code: configEnabled
          ? selectedStandard?.standard_code
          : undefined,
        config_name: configEnabled
          ? standardDisplayName(selectedStandard, i18n.resolvedLanguage)
          : undefined,
        config_version: configEnabled ? selectedStandard?.version : undefined,
        config_hash: configEnabled && !draftMatchesStandard ? selectedStandard?.config_hash : undefined,
        runtime_draft:
          configEnabled && draftMatchesStandard && runtimeDraftQuery.data?.revision
            ? {
                namespace: stationDetectionPreloadNamespace,
                revision: runtimeDraftQuery.data.revision,
              }
            : undefined,
        report_request: buildReportRequest(values, defaultReportVarIds),
        end_policy: values.duration_min ? 'fixed_duration' : 'manual',
        duration_sec: values.duration_min
          ? values.duration_min * 60
          : undefined,
        operator_note: values.operator_note?.trim() || undefined,
        enable_storage: true,
        enable_alarm: true,
      }
      const commandID = `start-detection-${Date.now()}`
      const commandStartedAt = Date.now()
      const previousRecentRuns = await getDetectionRuns({
        project_id: values.project_id,
        limit: 1,
      })
        .then((response) => response.items)
        .catch(() => [] as DetectionRun[])
      const previousProjectActiveRuns = (activeRunsQuery.data ?? []).filter(
        (run) => run.project_id === values.project_id,
      )
      const previousMaxRunId = Math.max(
        0,
        ...previousRecentRuns.map((run) => run.id),
        ...previousProjectActiveRuns.map((run) => run.id),
      )
      const result = await sendRealtimeWebSocketCommand<
        unknown,
        VariableWriteResult
      >({
        type: 'command.write_variable',
        request_id: commandID,
        command_id: commandID,
        payload: {
          edge_instance_id: selectedEdgeInstanceId,
          project_id: values.project_id,
          var_id: String(requestVariable.var_id_text ?? requestVariable.var_id),
          value: JSON.stringify(taskRequest),
          trigger: true,
          max_depth: 3,
        },
      })
      const startedRun = await waitForNewDetectionRun(
        values.project_id,
        previousMaxRunId,
      )
      if (!startedRun) {
        const failedRun = await findFailedStartTaskFlowRun(
          values.project_id,
          requestTarget.flow.id,
          requestVariable.var_id_text ?? requestVariable.var_id,
          commandID,
          commandStartedAt - 5000,
        )
        if (failedRun) {
          const rawReason = taskFlowRunFailureReason(failedRun)
          const reason = isRuntimeDraftStaleError(rawReason)
            ? t('station.start.errors.staleRuntimeDraft')
            : rawReason || failedRun.status
          throw new Error(t('station.start.errors.taskRequestFlowFailed', { reason }))
        }
      }
      if (!startedRun && (result?.triggered ?? 0) <= 0) {
        throw new Error(t('station.start.errors.taskRequestFlowNotTriggered'))
      }
      if (!startedRun) {
        throw new Error(t('station.start.errors.taskRequestStartNotConfirmed'))
      }
      return startedRun
    },
    onSuccess: async () => {
      messageApi.success(t('station.messages.started'))
      setStartModalOpen(false)
      await queryClient.invalidateQueries({ queryKey: ['station', 'runtime-draft'] })
      await refreshRuns()
    },
    onError: (error) => {
      messageApi.error(
        error instanceof Error
          ? error.message
          : t('station.messages.startFailed'),
      )
    },
  })

  const stopRunMutation = useMutation({
    mutationFn: ({
      runId,
      reason,
      abnormal,
    }: {
      runId: number
      reason: string
      abnormal?: boolean
    }) =>
      abnormal
        ? abnormalStopDetectionRun(runId, { reason })
        : stopDetectionRun(runId, { reason }),
    onSuccess: async (_, variables) => {
      messageApi.success(
        variables.abnormal
          ? t('station.messages.abnormalStopped')
          : t('station.messages.stopped'),
      )
      await refreshRuns()
    },
    onError: (error) => {
      messageApi.error(
        error instanceof Error
          ? error.message
          : t('station.messages.stopFailed'),
      )
    },
  })

  const writePIDSetting = useCallback(
    async (
      setting: PIDSettingItem,
      variable: VariableConfig,
      displayValue: string,
    ) => {
      const key = setting.key
      if (!isPIDWritable(variable)) {
        setPIDWriteStates((states) => ({
          ...states,
          [key]: {
            status: 'error',
            message: t('station.pid.readOnly'),
            submittedValue: displayValue,
          },
        }))
        return false
      }
      let value: unknown
      try {
        value = coerceWriteValue(
          variable,
          String(pidWriteValue(setting.key, displayValue, 2)),
        )
      } catch {
        messageApi.error(t('station.pid.invalidValue'))
        setPIDWriteStates((states) => ({
          ...states,
          [key]: {
            status: 'error',
            message: t('station.pid.invalidValue'),
            submittedValue: displayValue,
          },
        }))
        return false
      }
      const commandID = `pid-${key}-${Date.now()}`
      setPIDWriteStates((states) => ({
        ...states,
        [key]: {
          status: 'pending',
          submittedValue: displayValue,
          submittedAt: new Date().toISOString(),
        },
      }))
      setPIDWriteValues((values) => {
        const next = { ...values }
        delete next[key]
        return next
      })
      try {
        const result = await sendRealtimeWebSocketCommand<
          unknown,
          VariableWriteResult
        >({
          type: 'command.write_variable',
          request_id: commandID,
          command_id: commandID,
          payload: {
            var_id: String(variableWireId(variable)),
            edge_instance_id: selectedEdgeInstanceId,
            project_id: validSelectedProjectId,
            var_name: variable.var_name,
            value,
            trigger: false,
            wait_ack: true,
            ack_timeout_sec: 10,
          },
        })
        setPIDWriteStates((states) => ({
          ...states,
          [key]: {
            status: 'ack',
            submittedValue: displayValue,
            submittedAt: new Date().toISOString(),
            result,
          },
        }))
        return true
      } catch (error) {
        const commandError =
          error instanceof RealtimeWebSocketCommandError ? error : undefined
        const result = commandError?.result as VariableWriteResult | undefined
        if (isBrokerAcceptedWithoutAck(result)) {
          setPIDWriteStates((states) => ({
            ...states,
            [key]: {
              status: 'sent',
              message: t('station.pid.sentReadbackPending'),
              submittedValue: displayValue,
              submittedAt: new Date().toISOString(),
              result,
            },
          }))
          return true
        }
        setPIDWriteStates((states) => ({
          ...states,
          [key]: {
            status: 'error',
            message: isGatewayOfflineWriteResult(result)
              ? t('station.pid.gatewayOffline')
              : error instanceof Error
                ? error.message
                : t('station.pid.writeFailed'),
            submittedValue: displayValue,
            submittedAt: new Date().toISOString(),
            result,
          },
        }))
        return false
      }
    },
    [messageApi, selectedEdgeInstanceId, t, validSelectedProjectId],
  )

  async function submitAllPIDSettings() {
    let submitted = 0
    let failed = 0
    for (const group of pidSettingGroups) {
      for (const setting of group.items) {
        const variable = findPIDVariable(pidVariables, setting.key)
        if (!variable) continue
        if (!isPIDWritable(variable)) continue
        const displayValue = pidWriteValues[setting.key] ?? ''
        if (!displayValue.trim()) continue
        const ok = await writePIDSetting(setting, variable, displayValue)
        if (ok) submitted += 1
        else failed += 1
      }
    }
    if (submitted > 0 && failed === 0)
      messageApi.success(t('station.pid.writeAck'))
    if (failed > 0) messageApi.error(t('station.pid.writeFailed'))
  }

  function togglePinnedRow(row: StationTableRow) {
    if (
      !row.itemUid ||
      rawStationViewItems.length === 0 ||
      saveStationViewItemsMutation.isPending
    )
      return
    const nextPinned = !row.pinned
    setPreviewPinnedRows((current) => ({
      ...current,
      [row.itemUid!]: nextPinned,
    }))
    const nextItems = rawStationViewItems.map((item) => {
      if (item.item_uid !== row.itemUid) return item
      return { ...item, pinned: nextPinned }
    })
    saveStationViewItems(nextItems)
  }

  function openConfigPreview() {
    setConfigPreviewOpen(true)
  }

  function applyConfigPreview(draft: StationDetectionConfigDraft) {
    if (!validSelectedProjectId) return
    saveRuntimeDraftMutation.mutate({
      ...draft,
      projectId: validSelectedProjectId,
    })
  }

  function fillEmptyReportVariables(varIds = selectedStandardReportVarIds) {
    if (varIds.length === 0) return
    const currentRows =
      (startForm.getFieldValue('report_requests') as ReportRequestFormRow[]) ??
      []
    if (currentRows.length === 0) {
      startForm.setFieldValue('report_requests', [
        {
          template_id: reportTemplates[0]?.id,
          var_ids: varIds,
          params_json: '{}',
        },
      ])
      return
    }
    startForm.setFieldValue(
      'report_requests',
      currentRows.map((row) => ({
        ...row,
        var_ids: row.var_ids && row.var_ids.length > 0 ? row.var_ids : varIds,
      })),
    )
  }

  function openStartModal() {
    const targetProject = selectedProject ?? projects[0]
    const appliedDraft = targetProject?.id
      ? targetProject.id === effectivePreviewConfig?.projectId
        ? effectivePreviewConfig
        : undefined
      : undefined
    const defaultStandard =
      availableStandards.find((standard) => standard.id === appliedDraft?.standardId) ??
      availableStandards[0]
    const defaultStandardForReport = appliedDraft && defaultStandard
      ? { ...defaultStandard, items: appliedDraft.items }
      : defaultStandard
    const defaultReportVarIds = standardReportVarIds(
      defaultStandardForReport,
      stationVariables,
    )
    startForm.setFieldsValue({
      plan_id: undefined,
      project_id: targetProject?.id,
      factory_no: '',
      customer_name: '',
      device_model: '',
      test_no: '',
      mode: defaultStandard?.mode ?? 'standard',
      config_enabled: Boolean(defaultStandard),
      standard_id: defaultStandard?.id,
      report_requests: [
        {
          template_id: reportTemplates[0]?.id,
          var_ids: defaultReportVarIds,
          params_json: '{\n  "inlet_area_m2": 1.25\n}',
        },
      ],
      duration_min: 60,
    })
    setStartModalOpen(true)
  }

  function applyDetectionPlan(planId?: number) {
    const plan = pendingPlans.find((item) => item.id === planId)
    if (!plan) {
      startForm.setFieldsValue({ plan_id: undefined })
      return
    }
    const matchedStandard = availableStandards.find(
      (standard) => standard.standard_code === plan.standard_code,
    )
    startForm.setFieldsValue({
      plan_id: plan.id,
      factory_no: plan.factory_no,
      customer_name: plan.customer_name,
      device_model: plan.device_model,
      test_no: plan.plan_no,
      mode: plan.mode || matchedStandard?.mode || 'standard',
      config_enabled: true,
      standard_id: matchedStandard?.id,
    })
  }

  function confirmStop(abnormal = false) {
    if (!activeRun) return
    Modal.confirm({
      title: abnormal
        ? t('station.run.abnormalStopTitle')
        : t('station.run.stopTitle'),
      content: abnormal
        ? t('station.run.abnormalStopDesc')
        : t('station.run.stopDesc'),
      okText: abnormal
        ? t('station.actions.abnormalStop')
        : t('station.actions.stop'),
      cancelText: t('actions.cancel'),
      okButtonProps: { danger: abnormal },
      onOk: () =>
        stopRunMutation.mutateAsync({
          runId: activeRun.id,
          reason: abnormal
            ? t('station.run.abnormalDefaultReason')
            : t('station.run.manualStopReason'),
          abnormal,
        }),
    })
  }

  function openHistoryForActiveRun() {
    if (!activeRun) return
    const params = new URLSearchParams({
      task_id: String(activeRun.id),
      project_id: String(activeRun.project_id),
      test_no: activeRun.test_no,
    })
    navigate(`/history?${params.toString()}`)
  }

  const statusProjectCode =
    selectedProject?.project_code ?? activeRun?.project_code ?? 'SN-20230912'
  const statusProject =
    selectedProjectName ?? activeRun?.test_no ?? t('station.status.mockProject')
  const statusConfig = selectedProject?.model_name || activeRun?.mode || 'A'
  const statusTask = activeRun?.test_no ?? t('station.run.idle')
  const selectedStandardLabel =
    activeRun?.standard_code ||
    (previewStandard
      ? standardDisplayName(previewStandard, i18n.resolvedLanguage)
      : availableStandards[0]?.standard_code) ||
    '--'
  const effectiveTemplate = stationViewQuery.data?.template
  const visibleTemplates = stationViewTemplatesQuery.data?.items ?? []
  const enabledAssignments = visibleTemplates.reduce(
    (count, template) =>
      count +
      (template.assignments ?? []).filter((assignment) => assignment.enabled)
        .length,
    0,
  )
  const alarmRows = alarmsQuery.data?.items ?? []
  const alarmScopeOptions = useMemo(
    () => [
      { label: t('station.alarms.scopeAll'), value: 'all' },
      { label: t('station.alarms.scopeDefault'), value: 'default' },
      { label: t('station.alarms.scopeDetection'), value: 'detection' },
    ],
    [t],
  )
  const alarmColumns = useMemo(
    () => [
      {
        title: t('station.alarms.scope'),
        dataIndex: 'scope',
        key: 'scope',
        width: 110,
        render: (scope: string) => (
          <Tag color={scope === 'default' ? 'cyan' : 'volcano'}>
            {scope === 'default'
              ? t('station.alarms.scopeDefault')
              : t('station.alarms.scopeDetection')}
          </Tag>
        ),
      },
      {
        title: t('station.alarms.variable'),
        key: 'variable',
        width: 180,
        render: (_: unknown, record: LimitAlarm) => (
          <div className="station-alarm-variable">
            <strong>{alarmDisplayName(record, i18n.resolvedLanguage)}</strong>
            <span>{record.var_name}</span>
          </div>
        ),
      },
      {
        title: t('station.alarms.level'),
        dataIndex: 'alarm_level',
        key: 'alarm_level',
        width: 84,
        render: (level: string) => (
          <Tag color={level === 'HH' || level === 'LL' ? 'red' : 'orange'}>
            {level}
          </Tag>
        ),
      },
      {
        title: t('station.alarms.status'),
        dataIndex: 'status',
        key: 'status',
        width: 92,
        render: (status: string) => (
          <span className={status === 'active' ? 'status-ng' : 'status-ok'}>
            <span />
            {status === 'active'
              ? t('station.alarms.active')
              : t('station.alarms.closed')}
          </span>
        ),
      },
      {
        title: t('station.alarms.values'),
        key: 'values',
        width: 170,
        render: (_: unknown, record: LimitAlarm) => (
          <div className="station-alarm-values">
            <span>
              {t('station.alarms.startValue')}:{' '}
              {formatAlarmValue(record.start_value)}
            </span>
            <span>
              {t('station.alarms.limitValue')}:{' '}
              {formatAlarmValue(record.limit_value)}
            </span>
            <span>
              {t('station.alarms.recoverValue')}:{' '}
              {formatAlarmValue(record.recover_value)}
            </span>
          </div>
        ),
      },
      {
        title: t('station.alarms.firstSeenAt'),
        dataIndex: 'first_seen_at',
        key: 'first_seen_at',
        width: 170,
        render: formatAlarmTime,
      },
      {
        title: t('station.alarms.lastSeenAt'),
        dataIndex: 'last_seen_at',
        key: 'last_seen_at',
        width: 170,
        render: formatAlarmTime,
      },
    ],
    [i18n.resolvedLanguage, t],
  )
  const storageRouteColumns = useMemo(
    () => [
      {
        title: t('station.storage.route'),
        dataIndex: 'route_code',
        key: 'route_code',
        width: 170,
        render: (value: string, record: DetectionRunStorageRoute) => (
          <div className="station-alarm-variable">
            <strong>{value}</strong>
            <span>var_id: {record.var_id_text ?? record.var_id}</span>
          </div>
        ),
      },
      {
        title: t('station.storage.target'),
        dataIndex: 'storage_target',
        key: 'storage_target',
        width: 132,
        render: (value: string) => (
          <Tag color={value === 'wide_table' ? 'blue' : 'default'}>{value}</Tag>
        ),
      },
      {
        title: t('station.storage.tableColumn'),
        key: 'tableColumn',
        width: 260,
        render: (_: unknown, record: DetectionRunStorageRoute) => (
          <div className="station-alarm-values">
            <span>{record.table_name || '--'}</span>
            <span>
              {record.column_name || '--'} / {record.column_type || '--'}
            </span>
          </div>
        ),
      },
      {
        title: t('station.storage.trigger'),
        dataIndex: 'trigger_mode',
        key: 'trigger_mode',
        width: 132,
      },
      {
        title: t('station.storage.cycle'),
        dataIndex: 'cycle_ms',
        key: 'cycle_ms',
        width: 110,
        render: (value: number) => (value > 0 ? `${value} ms` : '--'),
      },
      {
        title: t('station.storage.deadband'),
        dataIndex: 'deadband',
        key: 'deadband',
        width: 110,
        render: (value: number) => String(value ?? 0),
      },
      {
        title: t('station.storage.storeOnStart'),
        dataIndex: 'store_on_start',
        key: 'store_on_start',
        width: 110,
        render: (value: boolean) => (
          <Tag color={value ? 'green' : 'default'}>
            {value ? t('station.storage.yes') : t('station.storage.no')}
          </Tag>
        ),
      },
    ],
    [t],
  )
  const runSnapshotColumns = useMemo(
    () => [
      {
        title: t('station.snapshot.variable'),
        key: 'variable',
        width: 210,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-variable">
            <strong>{alarmDisplayName(record, i18n.resolvedLanguage)}</strong>
            <span>{record.var_name}</span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.detectionLimit'),
        key: 'detectionLimit',
        width: 210,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>
              LL/L: {formatAlarmValue(record.limit_ll)} /{' '}
              {formatAlarmValue(record.limit_l)}
            </span>
            <span>
              H/HH: {formatAlarmValue(record.limit_h)} /{' '}
              {formatAlarmValue(record.limit_hh)}
            </span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.defaultAlarm'),
        key: 'defaultAlarm',
        width: 130,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <Tag
            color={record.variable_default_alarm_enabled ? 'cyan' : 'default'}
          >
            {record.variable_default_alarm_enabled
              ? t('station.storage.yes')
              : t('station.storage.no')}
          </Tag>
        ),
      },
      {
        title: t('station.snapshot.defaultLimit'),
        key: 'defaultLimit',
        width: 230,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>
              LL/L:{' '}
              {formatAlarmValue(record.variable_default_limit_ll ?? undefined)}{' '}
              / {formatAlarmValue(record.variable_default_limit_l ?? undefined)}
            </span>
            <span>
              H/HH:{' '}
              {formatAlarmValue(record.variable_default_limit_h ?? undefined)} /{' '}
              {formatAlarmValue(record.variable_default_limit_hh ?? undefined)}
            </span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.policy'),
        key: 'policy',
        width: 220,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>
              {t('station.snapshot.deadband')}:{' '}
              {formatAlarmValue(record.variable_default_limit_deadband)}
            </span>
            <span>
              {t('station.snapshot.hold')}:{' '}
              {record.variable_default_violation_hold_ms} /{' '}
              {record.variable_default_recover_hold_ms} ms
            </span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.check'),
        key: 'check',
        width: 180,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>
              {record.alarm_enabled
                ? t('station.snapshot.alarmOn')
                : t('station.snapshot.alarmOff')}
            </span>
            <span>
              {record.check_on_start
                ? t('station.snapshot.checkOnStart')
                : t('station.snapshot.checkByCycle')}{' '}
              / {record.check_cycle_ms} ms
            </span>
          </div>
        ),
      },
    ],
    [i18n.resolvedLanguage, t],
  )
  const reportRequestColumns = useMemo(
    () => [
      {
        title: t('station.snapshot.reportVariable'),
        key: 'variable',
        width: 220,
        render: (_: unknown, record: DetectionRunReportRequest) => (
          <div className="station-alarm-variable">
            <strong>{alarmDisplayName(record, i18n.resolvedLanguage)}</strong>
            <span>
              {record.var_name || record.var_id_text || record.var_id}
            </span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.reportName'),
        dataIndex: 'report_name',
        key: 'report_name',
        width: 180,
        render: (value: string) => value || '-',
      },
      {
        title: t('station.alarms.status'),
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (value: string) => <Tag>{value || 'pending'}</Tag>,
      },
      {
        title: t('station.snapshot.reportExt'),
        key: 'ext',
        width: 280,
        render: (_: unknown, record: DetectionRunReportRequest) => (
          <div className="station-alarm-values">
            <span>{record.ext_1 || '-'}</span>
            <span>{record.ext_2 || '-'}</span>
            <span>{record.ext_3 || '-'}</span>
          </div>
        ),
      },
    ],
    [i18n.resolvedLanguage, t],
  )
  return (
    <div className="station-page">
      {messageContext}
      <StationLightBackground />
      <div className="station-grid">
        <SortableMetricGrid
          cards={cards}
          onOrderCommit={handleCardOrderCommit}
          onToggleAxisMode={handleToggleCardAxisMode}
          t={t}
          warnings={stationViewQuery.data?.warnings ?? []}
        />

        <aside className="station-side">
          <section
            className={
              isStatusCollapsed
                ? 'station-status-card glass-panel collapsed'
                : 'station-status-card glass-panel'
            }
            onClick={() => setStatusCollapsed((value) => !value)}
          >
            {isStatusCollapsed ? (
              <>
                <div className="status-collapsed-left">
                  <strong>{statusProjectCode}</strong>
                  <span>{statusProject}</span>
                  <span>
                    {activeRun ? activeRun.test_no : t('station.run.idle')}
                  </span>
                </div>
                <div
                  className={
                    activeRun
                      ? 'station-ok compact running'
                      : 'station-ok compact'
                  }
                >
                  {activeRun ? 'RUN' : 'OK'}
                </div>
              </>
            ) : (
              <>
                <div className="station-status-top">
                  <div className="station-status-main">
                    <div>
                      <span className="eyebrow">
                        {t('station.status.projectId')}
                      </span>
                      <strong>{statusProjectCode}</strong>
                    </div>
                    <div>
                      <span className="eyebrow">
                        {t('station.status.project')}
                      </span>
                      <strong className="serif">{statusProject}</strong>
                    </div>
                  </div>
                  <div className="station-result">
                    <div
                      className={
                        activeRun ? 'station-ok running' : 'station-ok'
                      }
                    >
                      {activeRun ? 'RUN' : 'OK'}
                    </div>
                    <div className="station-normal">
                      <span />
                      {activeRun
                        ? t('station.run.running')
                        : t('station.status.normal')}
                    </div>
                  </div>
                </div>
                <div className="station-status-meta">
                  <div>
                    <span>{t('station.status.config')}</span>
                    <strong className="serif">{statusConfig}</strong>
                  </div>
                  <div>
                    <span>{t('station.status.task')}</span>
                    <strong>{statusTask}</strong>
                  </div>
                  <div>
                    <span>{t('station.run.standard')}</span>
                    <strong>{selectedStandardLabel}</strong>
                  </div>
                </div>
              </>
            )}
          </section>

          <div className="station-actions">
            <Button
              icon={<SlidersHorizontal size={15} />}
              disabled={!validSelectedProjectId || availableStandards.length === 0}
              onClick={openConfigPreview}
            >
              {t('station.actions.config')}
            </Button>
            <Button
              icon={<Gauge size={15} />}
              disabled={!validSelectedProjectId}
              onClick={() => setPIDModalOpen(true)}
            >
              {t('station.actions.pid')}
            </Button>
            <Button>{t('station.actions.mute')}</Button>
            <Button
              className={alarmOn ? 'alarm-active' : undefined}
              onClick={() => setAlarmModalOpen(true)}
            >
              {t('station.actions.alarmLog')}
            </Button>
            {activeRun ? (
              <>
                <Button
                  icon={<History size={15} />}
                  onClick={openHistoryForActiveRun}
                >
                  {t('station.actions.history')}
                </Button>
                <Button
                  icon={<Database size={15} />}
                  onClick={() => setStorageSnapshotOpen(true)}
                >
                  {t('station.actions.storageSnapshot')}
                </Button>
                <Button
                  icon={<Database size={15} />}
                  onClick={() => setRunSnapshotOpen(true)}
                >
                  {t('station.actions.runSnapshot')}
                </Button>
                <Button
                  icon={<Square size={14} />}
                  disabled={!canStopDetection}
                  loading={stopRunMutation.isPending}
                  onClick={() => confirmStop(false)}
                >
                  {t('station.actions.stop')}
                </Button>
                <Button
                  danger
                  icon={<AlertTriangle size={15} />}
                  disabled={!canStopDetection}
                  loading={stopRunMutation.isPending}
                  onClick={() => confirmStop(true)}
                >
                  {t('station.actions.abnormalStop')}
                </Button>
              </>
            ) : (
              <Button
                className="station-start-action"
                type="primary"
                icon={<Play size={15} />}
                disabled={!canStartDetection || projects.length === 0}
                onClick={openStartModal}
              >
                {t('station.actions.start')}
              </Button>
            )}
          </div>

          <section className="station-table-panel glass-panel">
            <div className="station-table-head">
              <table>
                <thead>
                  <tr>
                    <th>{t('station.table.metric')}</th>
                    <th>{t('station.table.standard')}</th>
                    <th>{t('station.table.value')}</th>
                    <th>{t('station.table.status')}</th>
                  </tr>
                </thead>
              </table>
            </div>
            <div className="station-table-body table-scroll-container">
              <table>
                <tbody>
                  {sortedStationRows.length === 0 ? (
                    <tr className="station-row station-row-empty">
                      <td colSpan={4}>
                        <div className="station-table-empty">
                          <strong>{t('station.view.emptyTableTitle')}</strong>
                          <span>{t('station.view.emptyTableHint')}</span>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    sortedStationRows.map((row) => {
                      return (
                        <tr
                          className={
                            row.pinned ? 'station-row pinned' : 'station-row'
                          }
                          key={row.key}
                          title={row.name}
                          onClick={() => togglePinnedRow(row)}
                        >
                          <td>
                            <span className="pin-indicator" />
                            {row.name}
                          </td>
                          <td>{row.standard}</td>
                          <td className="mono">{row.value}</td>
                          <td>
                            <span
                              className={row.ok ? 'status-ok' : 'status-ng'}
                            >
                              <span />
                              {row.ok ? 'OK' : 'NG'}
                            </span>
                          </td>
                        </tr>
                      )
                    })
                  )}
                </tbody>
              </table>
            </div>
          </section>
        </aside>
      </div>
      <div
        className="station-template-footnote"
        aria-label={t('station.template.trace')}
      >
        <span>{t('station.template.trace')}</span>
        <strong>{effectiveTemplate?.template_code ?? '--'}</strong>
        <span>
          v{effectiveTemplate?.version ?? '-'} /{' '}
          {effectiveTemplate?.status ?? '-'}
        </span>
        <span>
          {t('station.template.visibleTemplates')}:{' '}
          {stationViewTemplatesQuery.isFetching
            ? '...'
            : visibleTemplates.length}
        </span>
        <span>
          {t('station.template.assignments')}:{' '}
          {stationViewTemplatesQuery.isFetching ? '...' : enabledAssignments}
        </span>
      </div>
      <Modal
        className="station-config-preview-modal"
        title={t('station.configPreview.title')}
        open={configPreviewOpen}
        width={1120}
        onCancel={() => setConfigPreviewOpen(false)}
        footer={null}
        destroyOnHidden
      >
        <DetectionConfigEditor
          variant="station-modal"
          projectId={validSelectedProjectId}
          selectedProject={selectedProject}
          initialStandardId={previewConfig?.standardId ?? availableStandards[0]?.id}
          initialDraft={previewConfig}
          running={activeRun !== undefined}
          standards={availableStandards}
          projects={projects}
          variables={configVariablesQuery.data ?? []}
          reportTemplates={reportTemplates}
          onApplyDraft={applyConfigPreview}
        />
      </Modal>
      <Modal
        className="station-run-modal"
        title={t('station.run.startTitle')}
        open={startModalOpen}
        width={980}
        onCancel={() => setStartModalOpen(false)}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={startForm}
          layout="vertical"
          onFinish={(values) => startRunMutation.mutate(values)}
        >
          <div className="station-run-layout">
            <section className="station-run-section">
              <div className="station-run-section-head">
                <span>{t('station.run.basicInfo')}</span>
              </div>
              <div className="station-run-form-grid">
                <Form.Item name="plan_id" label={t('station.run.plan')}>
                  <Select
                    allowClear
                    showSearch
                    loading={detectionPlansQuery.isFetching}
                    optionFilterProp="label"
                    placeholder={t('station.run.planPlaceholder')}
                    onChange={(value) => applyDetectionPlan(value)}
                    options={pendingPlans.map((plan) => ({
                      label: detectionPlanLabel(plan),
                      value: plan.id,
                    }))}
                  />
                </Form.Item>
                <Form.Item
                  name="project_id"
                  label={t('station.run.project')}
                  rules={[{ required: true }]}
                >
                  <Select
                    options={projects.map((project) => ({
                      label: `${displayProjectName(project, i18n.resolvedLanguage)} / ${project.project_code}`,
                      value: project.id,
                    }))}
                  />
                </Form.Item>
                <Form.Item
                  name="factory_no"
                  label={t('station.run.factoryNo')}
                  rules={[{ required: true }]}
                >
                  <Input disabled={Boolean(selectedPlan)} />
                </Form.Item>
                <Form.Item
                  name="customer_name"
                  label={t('station.run.customerName')}
                >
                  <Input disabled={Boolean(selectedPlan)} />
                </Form.Item>
                <Form.Item
                  name="device_model"
                  label={t('station.run.deviceModel')}
                >
                  <Input disabled={Boolean(selectedPlan)} />
                </Form.Item>
                <Form.Item name="test_no" label={t('station.run.testNo')}>
                  <Input
                    disabled={Boolean(selectedPlan)}
                    placeholder={t('station.run.testNoAuto')}
                  />
                </Form.Item>
                <Form.Item
                  name="mode"
                  label={t('station.run.mode')}
                  rules={[{ required: true }]}
                >
                  <Select
                    disabled={Boolean(selectedPlan)}
                    options={[
                      {
                        label: t('station.run.standardMode'),
                        value: 'standard',
                      },
                      {
                        label: t('station.run.performanceMode'),
                        value: 'performance',
                      },
                    ]}
                  />
                </Form.Item>
                <Form.Item
                  name="duration_min"
                  label={t('station.run.durationMin')}
                >
                  <InputNumber
                    min={1}
                    precision={0}
                    style={{ width: '100%' }}
                  />
                </Form.Item>
              </div>
            </section>

            <section className="station-run-section">
              <div className="station-run-section-head">
                <span>{t('station.run.standardAndTemplate')}</span>
              </div>
              <div className="station-run-form-grid station-run-form-grid-compact">
                <Form.Item
                  name="config_enabled"
                  label={t('station.run.configEnabled')}
                  valuePropName="checked"
                >
                  <Switch />
                </Form.Item>
                <Form.Item
                  name="standard_id"
                  label={t('station.run.configName')}
                >
                  <Select
                    allowClear
                    disabled={!configEnabled || Boolean(selectedPlan)}
                    loading={standardsQuery.isFetching}
                    optionFilterProp="label"
                    onChange={(standardId) => {
                      const nextStandard = availableStandards.find(
                        (standard) => standard.id === standardId,
                      )
                      const nextVarIds = standardReportVarIds(
                        nextStandard,
                        stationVariables,
                      )
                      fillEmptyReportVariables(nextVarIds)
                    }}
                    options={availableStandards.map((standard) => ({
                      label: `${standardDisplayName(standard, i18n.resolvedLanguage)} / ${standard.standard_code} / v${standard.version} / ${detectionStandardScopeLabel(standard, t, selectedProject)}`,
                      value: standard.id,
                    }))}
                  />
                </Form.Item>
              </div>
              {selectedStartStandard ? (
                <Tag color={selectedStartStandard.project_id ? 'blue' : selectedStartStandard.project_group ? 'purple' : 'default'}>
                  {detectionStandardScopeLabel(selectedStartStandard, t, selectedProject)}
                </Tag>
              ) : null}
              {configEnabled && !selectedPlan && selectedStartDraftStale ? (
                <Alert
                  type="warning"
                  showIcon
                  message={t('station.start.errors.staleRuntimeDraft')}
                />
              ) : null}
            </section>

            <section className="station-run-section station-run-report-section">
              <div className="station-run-report-head">
                <div>
                  <strong>{t('station.run.reportRequests')}</strong>
                  <span>{t('station.run.reportRequestsHint')}</span>
                </div>
                <div className="station-run-report-actions">
                  <Tag
                    color={
                      selectedStandardReportVarIds.length > 0
                        ? hasReportRowsMissingVariables
                          ? 'gold'
                          : 'green'
                        : 'default'
                    }
                  >
                    {t('station.run.reportAutoVariables', {
                      count: selectedStandardReportVarIds.length,
                    })}
                  </Tag>
                  <Button
                    size="small"
                    disabled={selectedStandardReportVarIds.length === 0}
                    onClick={() => fillEmptyReportVariables()}
                  >
                    {t('station.run.fillReportVariables')}
                  </Button>
                </div>
              </div>
              <Form.List name="report_requests">
                {(fields, { add, remove }) => (
                  <div className="station-run-report-list">
                    {fields.map((field, index) => (
                      <div className="station-run-report-row" key={field.key}>
                        <div className="station-run-report-row-head">
                          <span>
                            {t('station.run.reportRequestIndex', {
                              index: index + 1,
                            })}
                          </span>
                          <Button
                            danger
                            size="small"
                            icon={<Trash2 size={13} />}
                            title={t('station.run.removeReportRequest')}
                            onClick={() => remove(field.name)}
                          />
                        </div>
                        <div className="station-run-report-fields">
                          <Form.Item
                            {...field}
                            name={[field.name, 'template_id']}
                            label={t('station.run.reportTemplate')}
                          >
                            <Select
                              allowClear
                              loading={reportTemplatesQuery.isFetching}
                              options={reportTemplates.map((template) => ({
                                label: `${template.display_name || template.name || template.template_code} / ${template.template_code}`,
                                value: template.id,
                              }))}
                            />
                          </Form.Item>
                          <Form.Item
                            {...field}
                            name={[field.name, 'report_name']}
                            label={t('station.run.reportName')}
                          >
                            <Input />
                          </Form.Item>
                          <Form.Item
                            {...field}
                            className="station-run-report-field-wide"
                            name={[field.name, 'var_ids']}
                            label={t('station.run.reportVariables')}
                            extra={
                              selectedStandardReportVarIds.length > 0
                                ? t('station.run.reportVariablesAutoHint', {
                                    count: selectedStandardReportVarIds.length,
                                  })
                                : t('station.run.reportVariablesRequiredHint')
                            }
                          >
                            <Select
                              allowClear
                              mode="multiple"
                              optionFilterProp="label"
                              placeholder={t(
                                'station.run.reportVariablesPlaceholder',
                              )}
                              options={stationVariables.map((variable) => ({
                                label: `${alarmDisplayName(variable, i18n.resolvedLanguage)} / ${variable.var_name}`,
                                value: tagWireId(variable),
                              }))}
                            />
                          </Form.Item>
                          <Form.Item
                            {...field}
                            className="station-run-report-field-wide"
                            name={[field.name, 'params_json']}
                            label={t('station.run.reportParams')}
                          >
                            <Input.TextArea rows={3} spellCheck={false} />
                          </Form.Item>
                        </div>
                      </div>
                    ))}
                    <Button
                      className="station-run-add-report"
                      size="small"
                      icon={<Plus size={14} />}
                      onClick={() =>
                        add({
                          template_id: reportTemplates[0]?.id,
                          var_ids: selectedStandardReportVarIds,
                          params_json: '{}',
                        })
                      }
                    >
                      {t('station.run.addReportRequest')}
                    </Button>
                  </div>
                )}
              </Form.List>
            </section>

            <section className="station-run-section">
              <Form.Item name="operator_note" label={t('station.run.note')}>
                <Input.TextArea rows={3} />
              </Form.Item>
            </section>
          </div>
          <div className="station-run-modal-footer">
            <Button onClick={() => setStartModalOpen(false)}>
              {t('actions.cancel')}
            </Button>
            <Button
              type="primary"
              htmlType="submit"
              icon={<Play size={15} />}
              loading={startRunMutation.isPending}
            >
              {t('station.actions.start')}
            </Button>
          </div>
        </Form>
      </Modal>
      <Modal
        className="station-alarm-modal station-pid-modal"
        title={t('station.pid.title')}
        open={pidModalOpen}
        onCancel={() => setPIDModalOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setPIDModalOpen(false)}>
            {t('actions.cancel')}
          </Button>,
          <Button key="submit" type="primary" onClick={submitAllPIDSettings}>
            {t('station.pid.submit')}
          </Button>,
        ]}
        centered
        width="min(1180px, calc(100vw - 48px))"
        destroyOnHidden
      >
        <div className="station-alarm-toolbar">
          <div className="station-pid-project">
            <span>
              {validSelectedProjectId
                ? (selectedProjectName ?? statusProjectCode)
                : t('station.status.noProject')}
            </span>
            <Tag>{statusProjectCode}</Tag>
          </div>
          <div className="station-alarm-toolbar-right">
            <Input
              size="small"
              value={pidVarGroup}
              placeholder={t('station.pid.groupPlaceholder')}
              onChange={(event) => setPIDVarGroup(event.target.value)}
              style={{ width: 150 }}
            />
            <Button
              size="small"
              onClick={() => pidVariablesQuery.refetch()}
              loading={pidVariablesQuery.isFetching}
            >
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <div className="station-pid-grid">
          {pidSettingGroups.map((group) => (
            <section className="station-pid-card" key={group.key}>
              <div className="station-pid-card-title">
                <Gauge size={15} />
                <span>{t(group.titleKey)}</span>
              </div>
              <div className="station-pid-card-body">
                {group.items.map((setting) => {
                  const variable = findPIDVariable(pidVariables, setting.key)
                  const snapshot = variable
                    ? pidSnapshotsByVarID.get(String(variableWireId(variable)))
                    : undefined
                  const currentValue = pidDisplayValue(
                    setting.key,
                    numericSnapshotValue(snapshot),
                    2,
                  )
                  const draftValue = pidWriteValues[setting.key] ?? ''
                  const state = pidWriteStates[setting.key]
                  const readback = isPIDReadbackMatch(
                    state?.submittedValue,
                    currentValue,
                  )
                  const color =
                    state?.status === 'ack'
                      ? 'green'
                      : state?.status === 'sent'
                        ? 'cyan'
                        : state?.status === 'error'
                          ? 'red'
                          : state?.status === 'pending'
                            ? 'blue'
                            : 'default'
                  const canWrite = variable ? isPIDWritable(variable) : false
                  const inputBaseValue = draftValue || currentValue || '0'
                  return (
                    <div className="station-pid-setting" key={setting.key}>
                      <div className="station-pid-setting-head">
                        <span>{t(setting.labelKey)}</span>
                        {variable ? (
                          <Tag color={canWrite ? color : 'default'}>
                            {canWrite
                              ? state?.status
                                ? t(`station.pid.${state.status}`)
                                : t('station.pid.idle')
                              : t('station.pid.readOnly')}
                          </Tag>
                        ) : (
                          <Tag color="red">{t('station.pid.noConnection')}</Tag>
                        )}
                      </div>
                      {variable ? (
                        <>
                          <div className="station-pid-variable-name">
                            {variableDisplayName(
                              variable,
                              i18n.resolvedLanguage,
                            )}{' '}
                            / {variable.var_name}
                          </div>
                          <div className="station-pid-current-row">
                            <span>{t('station.pid.currentValue')}</span>
                            <strong>
                              {currentValue || '--'}
                              {setting.unit ? ` ${setting.unit}` : ''}
                            </strong>
                          </div>
                          <div className="station-pid-control">
                            <Button
                              size="small"
                              icon={<Minus size={13} />}
                              aria-label={t('station.pid.decrement')}
                              disabled={!canWrite}
                              onClick={() => {
                                const current = Number(inputBaseValue)
                                setPIDWriteValues((values) => ({
                                  ...values,
                                  [setting.key]: formatPIDNumber(
                                    current - setting.step,
                                    setting.precision,
                                  ),
                                }))
                              }}
                            />
                            <Input
                              size="small"
                              value={draftValue}
                              placeholder={currentValue || '--'}
                              readOnly={!canWrite}
                              onChange={(event) =>
                                setPIDWriteValues((values) => ({
                                  ...values,
                                  [setting.key]: event.target.value,
                                }))
                              }
                              onPressEnter={() => {
                                if (canWrite && draftValue.trim())
                                  void writePIDSetting(
                                    setting,
                                    variable,
                                    draftValue,
                                  )
                              }}
                            />
                            <Button
                              size="small"
                              icon={<Plus size={13} />}
                              aria-label={t('station.pid.increment')}
                              disabled={!canWrite}
                              onClick={() => {
                                const current = Number(inputBaseValue)
                                setPIDWriteValues((values) => ({
                                  ...values,
                                  [setting.key]: formatPIDNumber(
                                    current + setting.step,
                                    setting.precision,
                                  ),
                                }))
                              }}
                            />
                          </div>
                          <div className="station-pid-setting-foot">
                            <span>
                              {snapshot?.last_update
                                ? formatAlarmTime(snapshot.last_update)
                                : '-'}
                            </span>
                            {draftValue ? (
                              <Tag color="gold">
                                {t('station.pid.draftValue')}: {draftValue}
                              </Tag>
                            ) : null}
                            {state?.submittedValue ? (
                              <Tag>
                                {t('station.pid.submittedValue')}:{' '}
                                {state.submittedValue}
                              </Tag>
                            ) : null}
                            {readback ? (
                              <Tag color="cyan">
                                {t('station.pid.readback')}
                              </Tag>
                            ) : null}
                            {state?.message ? (
                              <span>{state.message}</span>
                            ) : null}
                            {state?.result?.kio?.status ? (
                              <span>{state.result.kio.status}</span>
                            ) : null}
                          </div>
                        </>
                      ) : (
                        <div className="station-pid-disconnected">
                          {t('station.pid.noConnection')}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      </Modal>
      <Modal
        className="station-alarm-modal"
        title={t('station.alarms.title')}
        open={alarmModalOpen}
        onCancel={() => setAlarmModalOpen(false)}
        footer={null}
        centered
        width="min(1120px, calc(100vw - 48px))"
        destroyOnHidden
      >
        <div className="station-alarm-toolbar">
          <Segmented
            size="small"
            value={alarmScope}
            options={alarmScopeOptions}
            onChange={(value) => setAlarmScope(value as AlarmScopeFilter)}
          />
          <div className="station-alarm-toolbar-right">
            <span>
              {validSelectedProjectId
                ? t('station.alarms.currentProject', {
                    name: selectedProjectName ?? statusProjectCode,
                  })
                : t('station.alarms.allProjects')}
            </span>
            <Button
              size="small"
              onClick={() => alarmsQuery.refetch()}
              loading={alarmsQuery.isFetching}
            >
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<LimitAlarm>
          rowKey="id"
          size="small"
          columns={alarmColumns}
          dataSource={alarmRows}
          loading={alarmsQuery.isFetching}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          scroll={{ x: 980, y: 480 }}
        />
      </Modal>
      <Modal
        className="station-alarm-modal"
        title={t('station.snapshot.title')}
        open={runSnapshotOpen}
        onCancel={() => setRunSnapshotOpen(false)}
        footer={null}
        centered
        width="min(1240px, calc(100vw - 48px))"
        destroyOnHidden
      >
        <div className="station-alarm-toolbar">
          <span>
            {activeRun
              ? t('station.snapshot.currentRun', { testNo: activeRun.test_no })
              : t('station.run.idle')}
          </span>
          <div className="station-alarm-toolbar-right">
            <span>
              {t('station.snapshot.count', {
                count: runSnapshotQuery.data?.standard_items?.length ?? 0,
              })}
            </span>
            <Button
              size="small"
              onClick={() => runSnapshotQuery.refetch()}
              loading={runSnapshotQuery.isFetching}
            >
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunStandardItem>
          rowKey={(record) =>
            `${record.task_id}-${record.standard_item_id}-${record.var_id_text ?? record.var_id}`
          }
          size="small"
          columns={runSnapshotColumns}
          dataSource={runSnapshotQuery.data?.standard_items ?? []}
          loading={runSnapshotQuery.isFetching}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          scroll={{ x: 1180, y: 480 }}
        />
        <div className="station-alarm-toolbar">
          <span>{t('station.snapshot.reportRequests')}</span>
          <div className="station-alarm-toolbar-right">
            <span>
              {t('station.snapshot.reportRequestCount', {
                count:
                  reportRequestsQuery.data?.count ??
                  runSnapshotQuery.data?.report_requests?.length ??
                  0,
              })}
            </span>
            <Button
              size="small"
              onClick={() => reportRequestsQuery.refetch()}
              loading={reportRequestsQuery.isFetching}
            >
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunReportRequest>
          rowKey={(record) => record.id}
          size="small"
          columns={reportRequestColumns}
          dataSource={
            reportRequestsQuery.data?.items ??
            runSnapshotQuery.data?.report_requests ??
            []
          }
          loading={
            reportRequestsQuery.isFetching || runSnapshotQuery.isFetching
          }
          pagination={false}
          scroll={{ x: 820, y: 220 }}
        />
      </Modal>
      <Modal
        className="station-alarm-modal"
        title={t('station.storage.title')}
        open={storageSnapshotOpen}
        onCancel={() => setStorageSnapshotOpen(false)}
        footer={null}
        centered
        width="min(1120px, calc(100vw - 48px))"
        destroyOnHidden
      >
        <div className="station-alarm-toolbar">
          <span>
            {activeRun
              ? t('station.storage.currentRun', { testNo: activeRun.test_no })
              : t('station.run.idle')}
          </span>
          <div className="station-alarm-toolbar-right">
            <span>
              {t('station.storage.count', {
                count: storageSnapshotQuery.data?.count ?? 0,
              })}
            </span>
            <Button
              size="small"
              onClick={() => storageSnapshotQuery.refetch()}
              loading={storageSnapshotQuery.isFetching}
            >
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunStorageRoute>
          rowKey={(record) =>
            `${record.task_id}-${record.route_id}-${record.var_id_text ?? record.var_id}`
          }
          size="small"
          columns={storageRouteColumns}
          dataSource={storageSnapshotQuery.data?.items ?? []}
          loading={storageSnapshotQuery.isFetching}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          scroll={{ x: 980, y: 480 }}
        />
      </Modal>
    </div>
  )
}

function SortableMetricGrid({
  cards,
  onOrderCommit,
  onToggleAxisMode,
  t,
  warnings,
}: {
  cards: MetricCard[]
  onOrderCommit: (ids: string[]) => void
  onToggleAxisMode: (id: string) => void
  t: (key: string) => string
  warnings: string[]
}) {
  const [activeId, setActiveId] = useState<UniqueIdentifier | null>(null)
  const [activeCardSize, setActiveCardSize] = useState<
    { width: number; height: number } | undefined
  >()
  const [canScrollDown, setCanScrollDown] = useState(false)
  const [canScrollUp, setCanScrollUp] = useState(false)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )
  const activeCard = cards.find((card) => card.id === activeId)

  function cardNodeSelector(id: UniqueIdentifier) {
    return `[data-metric-card-id="${String(id).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"]`
  }

  useEffect(() => {
    const scrollElement = scrollRef.current
    if (!scrollElement) return undefined

    const checkScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = scrollElement
      setCanScrollUp(scrollTop > 2)
      setCanScrollDown(
        scrollHeight > clientHeight &&
          scrollTop + clientHeight < scrollHeight - 2,
      )
    }

    checkScroll()
    scrollElement.addEventListener('scroll', checkScroll)
    window.addEventListener('resize', checkScroll)
    const timer = window.setTimeout(checkScroll, 100)

    return () => {
      window.clearTimeout(timer)
      scrollElement.removeEventListener('scroll', checkScroll)
      window.removeEventListener('resize', checkScroll)
    }
  }, [cards.length])

  function reorderedCardIds(active: UniqueIdentifier, over: UniqueIdentifier) {
    if (active === over) return cards.map((item) => item.id)
    const oldIndex = cards.findIndex((item) => item.id === active)
    const newIndex = cards.findIndex((item) => item.id === over)
    if (oldIndex === -1 || newIndex === -1) return cards.map((item) => item.id)
    return arrayMove(cards, oldIndex, newIndex).map((item) => item.id)
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id)
    const cardNode = document.querySelector<HTMLElement>(
      cardNodeSelector(event.active.id),
    )
    const initialRect =
      cardNode?.getBoundingClientRect() ?? event.active.rect.current.initial
    setActiveCardSize(
      initialRect
        ? { width: initialRect.width, height: initialRect.height }
        : undefined,
    )
  }

  function handleDragEnd(event: DragEndEvent) {
    const currentIds = cards.map((item) => item.id)
    const finalIds = event.over
      ? reorderedCardIds(event.active.id, event.over.id)
      : currentIds
    setActiveId(null)
    setActiveCardSize(undefined)
    if (finalIds.some((id, index) => id !== currentIds[index])) {
      onOrderCommit(finalIds)
    }
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={() => {
        setActiveId(null)
        setActiveCardSize(undefined)
      }}
    >
      <StationCardGridStyles />
      <div className="station-card-grid-shell">
        <div className="grid-scroll-container" ref={scrollRef}>
          {cards.length === 0 ? (
            <div className="station-empty-state">
              <strong>{t('station.view.emptyCardsTitle')}</strong>
              <span>{t('station.view.emptyCardsHint')}</span>
              {warnings.length > 0 ? (
                <ul>
                  {warnings.map((warning) => (
                    <li key={warning}>{warning}</li>
                  ))}
                </ul>
              ) : null}
            </div>
          ) : (
            <div className="station-card-grid">
              <SortableContext
                items={cards.map((card) => card.id)}
                strategy={rectSortingStrategy}
              >
                {cards.map((card) => (
                  <SortableMetricCard
                    key={card.id}
                    card={card}
                    label={card.label}
                    onToggleAxisMode={onToggleAxisMode}
                    t={t}
                  />
                ))}
              </SortableContext>
            </div>
          )}
        </div>
        <button
          className={
            canScrollUp
              ? 'station-scroll-cue top visible'
              : 'station-scroll-cue top'
          }
          onClick={() =>
            scrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
          }
          aria-label={t('station.actions.scrollTop')}
        >
          <ChevronUp size={12} />
        </button>
        <button
          className={
            canScrollDown
              ? 'station-scroll-cue bottom visible'
              : 'station-scroll-cue bottom'
          }
          onClick={() =>
            scrollRef.current?.scrollTo({
              top: scrollRef.current.scrollHeight,
              behavior: 'smooth',
            })
          }
          aria-label={t('station.actions.scrollBottom')}
        >
          <ChevronDown size={12} />
        </button>
      </div>
      <DragOverlay dropAnimation={null}>
        {activeCard ? (
          <MetricCardDragPreview card={activeCard} size={activeCardSize} t={t} />
        ) : null}
      </DragOverlay>
    </DndContext>
  )
}

const SortableMetricCard = memo(function SortableMetricCard({
  card,
  label,
  onToggleAxisMode,
  t,
}: {
  card: MetricCard
  label: string
  onToggleAxisMode: (id: string) => void
  t: (key: string) => string
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: card.id })
  if (isDragging) {
    return (
      <div
        ref={setNodeRef}
        className="metric-card-placeholder"
        style={{ transform: CSS.Translate.toString(transform), transition }}
      />
    )
  }

  return (
    <div
      ref={setNodeRef}
      className="metric-card-shell"
      data-metric-card-id={card.id}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      {...attributes}
      {...listeners}
    >
      <MetricCardView
        card={card}
        label={label}
        onToggleAxisMode={onToggleAxisMode}
        t={t}
      />
    </div>
  )
})

const MetricCardView = memo(function MetricCardView({
  card,
  label,
  onToggleAxisMode,
  t,
  dragging = false,
}: {
  card: MetricCard
  label: string
  onToggleAxisMode: (id: string) => void
  t: (key: string) => string
  dragging?: boolean
}) {
  return (
    <article
      role="button"
      tabIndex={0}
      title={t('station.chart.toggleScale')}
      onClick={() => onToggleAxisMode(card.id)}
      onKeyDown={(event) => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        onToggleAxisMode(card.id)
      }}
      className={
        dragging
          ? 'metric-card glass-panel dragging'
          : 'metric-card glass-panel'
      }
    >
      <div className="metric-card-head">
        <div className="metric-title-group">
          <span
            className="metric-icon"
            style={{ color: card.color, backgroundColor: `${card.color}18` }}
          >
            {card.icon}
          </span>
          <div>
            <h2>{label}</h2>
            <span>{card.unit}</span>
          </div>
        </div>
        <div className="metric-more" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
        <span className="metric-axis-mode">
          {t(
            card.axisMode === 'standard'
              ? 'station.chart.standardScale'
              : 'station.chart.autoScale',
          )}
        </span>
      </div>
      <div className="metric-chart">
        <CardChart
          chartData={card.trend}
          legendName={label}
          color={card.color}
          min={card.min}
          max={card.max}
          axisMode={card.axisMode}
        />
      </div>
    </article>
  )
})

function MetricCardDragPreview({
  card,
  size,
  t,
}: {
  card: MetricCard
  size?: { width: number; height: number }
  t: (key: string) => string
}) {
  return (
    <div
      className="metric-card-drag-preview"
      style={size ? { width: size.width, height: size.height } : undefined}
    >
      <div className="metric-card-drag-preview-head">
        <div className="metric-title-group">
          <span
            className="metric-icon"
            style={{ color: card.color, backgroundColor: `${card.color}18` }}
          >
            {card.icon}
          </span>
          <div>
            <h2>{card.label}</h2>
            <span>{card.unit}</span>
          </div>
        </div>
        <span className="metric-axis-mode">
          {t(
            card.axisMode === 'standard'
              ? 'station.chart.standardScale'
              : 'station.chart.autoScale',
          )}
        </span>
      </div>
      <div className="metric-card-drag-preview-body" aria-hidden="true">
        <span />
        <span />
        <span />
        <span />
      </div>
    </div>
  )
}

function CardChart({
  chartData,
  legendName,
  color,
  min,
  max,
  axisMode,
}: {
  chartData: TrendPoint[]
  legendName: string
  color: string
  min?: number
  max?: number
  axisMode: ChartAxisMode
}) {
  const domain = buildChartDomain({ chartData, min, max, axisMode })
  const { ref, size } = useChartContainerSize(140)

  return (
    <div className="card-chart-line" ref={ref}>
      {size ? (
        <AreaChart
          width={size.width}
          height={size.height}
          data={chartData}
          margin={{ top: 12, right: 10, left: -25, bottom: 0 }}
        >
          <CartesianGrid
            strokeDasharray="3 4"
            vertical={false}
            stroke="rgba(30,27,24,0.06)"
          />
          <XAxis
            dataKey="time"
            stroke="rgba(30,27,24,0.4)"
            fontSize={10}
            tickLine={false}
            axisLine={false}
            fontFamily="-apple-system, sans-serif"
            dy={5}
          />
          <YAxis
            stroke="rgba(30,27,24,0.4)"
            fontSize={10}
            tickLine={false}
            axisLine={false}
            domain={domain}
            fontFamily="-apple-system, sans-serif"
            tickCount={5}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: 'rgba(255, 255, 255, 0.9)',
              backdropFilter: 'blur(16px)',
              borderColor: 'rgba(255,255,255,1)',
              borderRadius: '8px',
              boxShadow: '0 4px 12px rgba(0,0,0,0.05)',
              padding: '4px 8px',
            }}
            itemStyle={{
              color: '#333',
              fontWeight: 600,
              fontFamily: 'Georgia, serif',
              fontSize: 13,
            }}
            labelStyle={{ display: 'none' }}
          />
          {min !== undefined ? (
            <ReferenceLine
              y={min}
              stroke="#ff4d4f"
              strokeDasharray="2 3"
              strokeWidth={1}
              ifOverflow="discard"
              label={{
                position: 'insideTopLeft',
                value: `Min ${min}`,
                fill: '#ff4d4f',
                fontSize: 10,
                fontWeight: 500,
                fontFamily: '-apple-system, sans-serif',
                dy: -5,
              }}
            />
          ) : null}
          {max !== undefined ? (
            <ReferenceLine
              y={max}
              stroke="#8c8c8c"
              strokeDasharray="2 3"
              strokeWidth={1}
              ifOverflow="discard"
              label={{
                position: 'insideTopLeft',
                value: `Max ${max}`,
                fill: '#8c8c8c',
                fontSize: 10,
                fontWeight: 500,
                fontFamily: '-apple-system, sans-serif',
                dy: -5,
              }}
            />
          ) : null}
          <Area
            type="monotone"
            dataKey="value"
            name={legendName}
            stroke={color}
            strokeWidth={2.8}
            strokeLinecap="round"
            strokeLinejoin="round"
            fillOpacity={0}
            fill="transparent"
            isAnimationActive={false}
            activeDot={{ r: 4, strokeWidth: 0, fill: color }}
          />
        </AreaChart>
      ) : null}
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
