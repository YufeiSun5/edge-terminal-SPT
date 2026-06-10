import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  defaultDropAnimationSideEffects,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
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
import { Area, AreaChart, CartesianGrid, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router'
import { Button, Form, Input, InputNumber, Modal, Segmented, Select, Table, Tag, message } from 'antd'
import { useTranslation } from 'react-i18next'
import {
  ChevronDown,
  ChevronUp,
  Droplets,
  Gauge,
  History,
  Power,
  Play,
  Save,
  Settings,
  Square,
  Thermometer,
  Volume2,
  Waves,
  Wind,
  AlertTriangle,
  Database,
  Minus,
  Plus,
  Trash2,
} from 'lucide-react'
import type {
  DetectionRunStandardItem,
  DetectionRunReportRequest,
  DetectionRunReportRequestPayload,
  DetectionRunStartPayload,
  DetectionRunStorageRoute,
  LimitAlarm,
  LimitAlarmScope,
  RealtimeVariablesSnapshotPayload,
  StationViewItem,
  StationViewItemPayload,
  StationViewResolvedBinding,
  TagSnapshot,
  VariableConfig,
  VariableWriteResult,
} from '@/shared/api/types'
import { useAuthStore } from '@/features/auth/authStore'
import {
  abnormalStopDetectionRun,
  getActiveDetectionRuns,
  getCurrentDetectionRun,
  getDetectionRun,
  getDetectionRunReportRequests,
  getDetectionRunStorageRoutes,
  getDetectionStandards,
  getProjects,
  getLimitAlarms,
  getRealtimeVariables,
  getReportTemplates,
  getStationViewEffective,
  getStationViewItems,
  getStationViewTemplates,
  getVariables,
  replaceStationViewItems,
  startDetectionRun,
  stopDetectionRun,
} from '@/features/edge-status/api'
import { RealtimeWebSocketCommandError, sendRealtimeWebSocketCommand, subscribeRealtimeWebSocket } from '@/features/realtime/realtimeClient'
import { languageCode } from '@/shared/i18n/language'
import { StationCardGridStyles } from './components/StationCardGridStyles'
import { StationLightBackground } from './components/StationLightBackground'

type TrendPoint = {
  time: string
  value: number
}

type MetricCard = {
  id: string
  label: string
  unit: string
  color: string
  min?: number
  max?: number
  icon: ReactNode
  value?: number
  precision: number
  trend: TrendPoint[]
}

type StartDetectionFormValues = {
  project_id: number
  test_no: string
  mode: string
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

type StationViewPreference = {
  cardOrder?: string[]
  pinnedRows?: string[]
}

const stationViewPreferenceStorageKey = 'spindle.station.operation.preferences.v1'
const emptyStationPreferenceList: string[] = []
const stationMetricCardLimit = 12
const stationLayoutAreaCardPool = 'card_pool'
const stationLayoutAreaListLayout = 'list_layout'

function stationViewPreferenceScope(projectId?: number, edgeInstanceId?: string) {
  if (!projectId) return ''
  return `${edgeInstanceId?.trim() || 'auto'}:${projectId}`
}

function stringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  return value.filter((item): item is string => typeof item === 'string')
}

function normalizeStationViewPreference(value: unknown): StationViewPreference {
  if (!value || typeof value !== 'object') return {}
  const record = value as Record<string, unknown>
  return {
    cardOrder: stringArray(record.cardOrder),
    pinnedRows: stringArray(record.pinnedRows),
  }
}

function readStationViewPreferences(): Record<string, StationViewPreference> {
  if (typeof window === 'undefined') return {}
  try {
    const raw = window.localStorage.getItem(stationViewPreferenceStorageKey)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object') return {}
    return Object.fromEntries(
      Object.entries(parsed as Record<string, unknown>).map(([key, value]) => [
        key,
        normalizeStationViewPreference(value),
      ]),
    )
  } catch {
    return {}
  }
}

function writeStationViewPreferences(preferences: Record<string, StationViewPreference>) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(stationViewPreferenceStorageKey, JSON.stringify(preferences))
  } catch {
    // Local UI preference persistence is best-effort and must not block station operation.
  }
}

function stationBindingDefaultOrder(binding: StationViewResolvedBinding, fallback: number) {
  return Number.isFinite(binding.sort_order) ? binding.sort_order : fallback
}

function sortStationBindingsByDefaultOrder(bindings: StationViewResolvedBinding[]) {
  return bindings
    .map((binding, index) => ({ binding, index }))
    .sort((a, b) => {
      const orderDiff = stationBindingDefaultOrder(a.binding, a.index) - stationBindingDefaultOrder(b.binding, b.index)
      if (orderDiff !== 0) return orderDiff
      return a.index - b.index
    })
    .map((item) => item.binding)
}

function uniqueStrings(values: Array<string | undefined>) {
  const seen = new Set<string>()
  const result: string[] = []
  values.forEach((value) => {
    const next = value?.trim()
    if (!next || seen.has(next)) return
    seen.add(next)
    result.push(next)
  })
  return result
}

function stationViewVarNamesFromItems(items: StationViewItem[], layoutArea: string) {
  return uniqueStrings(
    items
      .filter((item) => item.layout_area === layoutArea && item.visible !== false)
      .flatMap((item) => {
        if (item.binding_type === 'var_name') return [item.binding_key]
        return (item.resolved_bindings ?? []).map((binding) => binding.var_name)
      }),
  )
}

function stationViewItemUID(layoutArea: string, varName: string, index: number) {
  const safeName = varName.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 80) || `item_${index + 1}`
  return `${layoutArea}_${safeName}_${index + 1}`
}

function buildStationViewItemPayload(layoutArea: typeof stationLayoutAreaCardPool | typeof stationLayoutAreaListLayout, varName: string, index: number): StationViewItemPayload {
  return {
    item_uid: stationViewItemUID(layoutArea, varName, index),
    layout_area: layoutArea,
    item_type: layoutArea === stationLayoutAreaCardPool ? 'metric_card' : 'inspection_row',
    binding_type: 'var_name',
    binding_key: varName,
    sort_order: (index + 1) * 10,
    visible: true,
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
      { key: 'SP1-WD', labelKey: 'station.pid.labels.temperatureSetpoint', unit: '℃', step: 0.1, precision: 1 },
      { key: 'P1', labelKey: 'station.pid.labels.p1', step: 0.1, precision: 1 },
      { key: 'I1', labelKey: 'station.pid.labels.i1', step: 1, precision: 0 },
      { key: 'D1', labelKey: 'station.pid.labels.d1', step: 1, precision: 0 },
    ],
  },
  {
    key: 'humidity',
    titleKey: 'station.pid.groups.humidity',
    items: [
      { key: 'SP2-SD', labelKey: 'station.pid.labels.humiditySetpoint', unit: '%', step: 0.1, precision: 1 },
      { key: 'P2', labelKey: 'station.pid.labels.p2', step: 0.1, precision: 1 },
      { key: 'I2', labelKey: 'station.pid.labels.i2', step: 1, precision: 0 },
      { key: 'D2', labelKey: 'station.pid.labels.d2', step: 1, precision: 0 },
    ],
  },
  {
    key: 'temperature2',
    titleKey: 'station.pid.groups.temperature2',
    items: [
      { key: 'SP2-WD', labelKey: 'station.pid.labels.temperatureSetpoint', unit: '℃', step: 0.1, precision: 1 },
      { key: 'P3', labelKey: 'station.pid.labels.p3', step: 0.1, precision: 1 },
      { key: 'I3', labelKey: 'station.pid.labels.i3', step: 1, precision: 0 },
      { key: 'D3', labelKey: 'station.pid.labels.d3', step: 1, precision: 0 },
    ],
  },
]

const cardColors = ['#c2410c', '#0f766e', '#2563eb', '#b45309', '#7c3aed', '#15803d', '#be185d', '#dc2626']

type StationTableRow = {
  key: string
  name: string
  standard: string
  value: string
  ok: boolean
}

function formatAlarmValue(value?: number | null) {
  return value === undefined || value === null ? '-' : Number(value).toFixed(3).replace(/\.?0+$/, '')
}

function formatAlarmTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function alarmDisplayName(
  alarm: Pick<LimitAlarm, 'display_name' | 'display_name_en' | 'display_name_ja' | 'var_name'>,
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return alarm.display_name_en || alarm.var_name
  if (currentLanguage === 'ja') return alarm.display_name_ja || alarm.var_name
  return alarm.display_name || alarm.var_name
}

function variableDisplayName(
  variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'var_name'>,
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return variable.display_name_en || variable.var_name
  if (currentLanguage === 'ja') return variable.display_name_ja || variable.var_name
  return variable.display_name || variable.var_name
}

function bindingDisplayName(binding: StationViewResolvedBinding, language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return binding.display_name_en || binding.var_name || binding.var_id_text || String(binding.var_id ?? '')
  if (currentLanguage === 'ja') return binding.display_name_ja || binding.var_name || binding.var_id_text || String(binding.var_id ?? '')
  return binding.display_name || binding.var_name || binding.var_id_text || String(binding.var_id ?? '')
}

function bindingWireId(binding: Pick<StationViewResolvedBinding, 'var_id' | 'var_id_text'>) {
  return binding.var_id_text ?? binding.var_id
}

function bindingKey(binding: StationViewResolvedBinding, index: number) {
  return String(bindingWireId(binding) ?? `${binding.source}-${binding.var_name ?? index}`)
}

function snapshotKey(snapshot: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  return String(snapshot.var_id_text ?? snapshot.var_id)
}

function runBindingFromStandardItem(item: DetectionRunStandardItem): StationViewResolvedBinding {
  return {
    source: 'detection_item',
    var_id: item.var_id,
    var_id_text: item.var_id_text,
    var_name: item.var_name,
    display_name: item.display_name,
    display_name_en: item.display_name_en,
    display_name_ja: item.display_name_ja,
    unit: item.unit,
    decimal_places: item.decimal_places,
    limit_ll: item.limit_ll,
    limit_l: item.limit_l,
    limit_h: item.limit_h,
    limit_hh: item.limit_hh,
    check_enabled: item.check_enabled,
    alarm_enabled: item.alarm_enabled,
    sort_order: item.sort_order,
  }
}

function tableBindingFromSnapshot(variable: TagSnapshot, index: number): StationViewResolvedBinding {
  return {
    source: 'project_variable',
    var_id: variable.var_id,
    var_id_text: variable.var_id_text,
    var_name: variable.var_name,
    var_group: variable.var_group,
    display_name: variable.display_name,
    display_name_en: variable.display_name_en,
    display_name_ja: variable.display_name_ja,
    unit: '',
    decimal_places: 2,
    sort_order: index + 1,
  }
}

function bindingLimits(binding: StationViewResolvedBinding) {
  return {
    min: binding.limit_l ?? binding.limit_ll ?? undefined,
    max: binding.limit_h ?? binding.limit_hh ?? undefined,
  }
}

function numericSnapshotValue(snapshot?: TagSnapshot) {
  if (!snapshot || snapshot.is_string || !Number.isFinite(snapshot.value)) return undefined
  return snapshot.value
}

function formatMetricValue(value: number | undefined, unit: string | undefined, precision: number) {
  if (value === undefined) return '--'
  return `${value.toFixed(Math.max(0, Math.min(precision, 4)))}${unit ? ` ${unit}` : ''}`
}

function formatStandardRange(binding: StationViewResolvedBinding) {
  const limits = bindingLimits(binding)
  const unit = binding.unit ? ` ${binding.unit}` : ''
  if (limits.min === undefined && limits.max === undefined) return '--'
  if (limits.min === undefined) return `<= ${formatAlarmValue(limits.max)}${unit}`
  if (limits.max === undefined) return `>= ${formatAlarmValue(limits.min)}${unit}`
  return `${formatAlarmValue(limits.min)} - ${formatAlarmValue(limits.max)}${unit}`
}

function isWithinLimits(value: number | undefined, binding: StationViewResolvedBinding) {
  if (value === undefined) return true
  const limits = bindingLimits(binding)
  if (limits.min !== undefined && value < limits.min) return false
  if (limits.max !== undefined && value > limits.max) return false
  return true
}

function trendFromValue(value: number | undefined, min?: number, max?: number): TrendPoint[] {
  const base = value ?? min ?? max ?? 0
  return Array.from({ length: 7 }, (_, index) => ({
    time: String(index + 1),
    value: base,
  }))
}

function iconForBinding(binding: StationViewResolvedBinding) {
  const text = `${binding.var_group ?? ''} ${binding.var_name ?? ''} ${binding.display_name ?? ''}`.toLowerCase()
  if (text.includes('temp') || text.includes('温')) return Thermometer
  if (text.includes('humid') || text.includes('湿')) return Droplets
  if (text.includes('wind') || text.includes('风')) return Wind
  if (text.includes('noise') || text.includes('噪')) return Volume2
  if (text.includes('vibration') || text.includes('振')) return Waves
  if (text.includes('power') || text.includes('功率')) return Power
  return Gauge
}

function buildReportRequest(values: StartDetectionFormValues): DetectionRunReportRequestPayload | undefined {
  const reports = (values.report_requests ?? [])
    .map((row) => {
      const varIds = (row.var_ids ?? []).filter((item) => item !== undefined && item !== null && item !== '')
      if (varIds.length === 0) return undefined
      const report: NonNullable<DetectionRunReportRequestPayload['reports']>[number] = {
        var_ids: varIds,
      }
      if (row.template_id) report.template_id = row.template_id
      if (row.report_name?.trim()) report.report_name = row.report_name.trim()
      const paramsText = row.params_json?.trim()
      if (paramsText) report.params = JSON.parse(paramsText) as Record<string, unknown>
      return report
    })
    .filter((item): item is NonNullable<DetectionRunReportRequestPayload['reports']>[number] => Boolean(item))
  return reports.length > 0 ? { enabled: true, reports } : undefined
}

function tagWireId(variable: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  return variable.var_id_text ?? variable.var_id
}

function variableWireId(variable: Pick<VariableConfig, 'var_id' | 'var_id_text'>) {
  return variable.var_id_text ?? variable.var_id
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
  return (result.broker_accepted === true || result.kio?.broker_accepted === true) && kioStatus === 'ack_timeout_or_unmatched'
}

function isGatewayOfflineWriteResult(result?: VariableWriteResult) {
  if (!result) return false
  const kioStatus = result.kio?.status
  return kioStatus === 'gateway_offline' || (result.kio?.broker_accepted === false && result.kio?.message?.toLowerCase().includes('gateway'))
}

function isPIDReadbackMatch(submittedValue: string | undefined, currentValue: string) {
  const submitted = submittedValue?.trim()
  const current = currentValue.trim()
  if (!submitted || !current) return false
  const submittedNumber = Number(submitted)
  const currentNumber = Number(current)
  if (Number.isFinite(submittedNumber) && Number.isFinite(currentNumber)) return Math.abs(submittedNumber - currentNumber) < 0.000001
  return submitted === current
}

function findPIDVariable(variables: VariableConfig[], key: string) {
  const normalizedKey = normalizePIDKey(key)
  return variables.find((variable) => {
    const candidates = [variable.var_name, variable.display_name, variable.display_name_en, variable.display_name_ja, variable.raw_name]
    return candidates.some((candidate) => normalizePIDKey(candidate) === normalizedKey)
  })
}

function formatPIDNumber(value: number, precision: number) {
  return value.toFixed(Math.max(0, Math.min(precision, 4)))
}

function pidDisplayValue(key: string, rawValue: number | undefined, decimalPlaces = 1) {
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

function coerceWriteValue(variable: Pick<VariableConfig, 'data_type'>, rawValue: string) {
  const value = rawValue.trim()
  if (value === '') throw new Error('empty')
  const dataType = variable.data_type.toUpperCase()
  if (dataType === 'BOOL' || dataType === 'BOOLEAN') {
    if (['1', 'true', 'on', 'yes'].includes(value.toLowerCase())) return true
    if (['0', 'false', 'off', 'no'].includes(value.toLowerCase())) return false
    throw new Error('bool')
  }
  if (dataType === 'INT' || dataType === 'INTEGER' || dataType === 'FLOAT' || dataType === 'DOUBLE' || dataType === 'NUMBER') {
    const numericValue = Number(value)
    if (!Number.isFinite(numericValue)) throw new Error('number')
    return dataType === 'INT' || dataType === 'INTEGER' ? Math.trunc(numericValue) : numericValue
  }
  return value
}

function displayProjectName(
  project: { project_code?: string; display_name?: string; display_name_en?: string; display_name_ja?: string; name?: string },
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return project.display_name_en || project.project_code || ''
  if (currentLanguage === 'ja') return project.display_name_ja || project.project_code || ''
  return project.display_name || project.name || project.project_code || ''
}

function standardDisplayName(
  standard: { standard_code: string; display_name?: string; display_name_en?: string; display_name_ja?: string; name?: string },
  language?: string,
) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return standard.display_name_en || standard.standard_code
  if (currentLanguage === 'ja') return standard.display_name_ja || standard.standard_code
  return standard.display_name || standard.name || standard.standard_code
}

export function StationOperationPage() {
  const { t, i18n } = useTranslation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [messageApi, messageContext] = message.useMessage()
  const [startForm] = Form.useForm<StartDetectionFormValues>()
  const [startModalOpen, setStartModalOpen] = useState(false)
  const [alarmModalOpen, setAlarmModalOpen] = useState(false)
  const [pidModalOpen, setPIDModalOpen] = useState(false)
  const [stationConfigOpen, setStationConfigOpen] = useState(false)
  const [stationCardConfigVarNames, setStationCardConfigVarNames] = useState<string[]>([])
  const [stationListConfigVarNames, setStationListConfigVarNames] = useState<string[]>([])
  const [pidVarGroup, setPIDVarGroup] = useState('')
  const [pidWriteValues, setPIDWriteValues] = useState<Record<string, string>>({})
  const [pidWriteStates, setPIDWriteStates] = useState<Record<string, PIDWriteState>>({})
  const [storageSnapshotOpen, setStorageSnapshotOpen] = useState(false)
  const [runSnapshotOpen, setRunSnapshotOpen] = useState(false)
  const [alarmScope, setAlarmScope] = useState<AlarmScopeFilter>('all')
  const hasPermission = useAuthStore((state) => state.hasPermission)
  const canStartDetection = hasPermission('start_detection')
  const canStopDetection = hasPermission('stop_detection')
  const canManageStationView = hasPermission('system_settings')
  const selectedProjectId = Number(searchParams.get('project_id'))
  const validSelectedProjectId = Number.isFinite(selectedProjectId) && selectedProjectId > 0 ? selectedProjectId : undefined
  const selectedEdgeInstanceId = searchParams.get('edge_instance_id') || undefined
  const stationViewQuery = useQuery({
    queryKey: ['station', 'view-effective', validSelectedProjectId, selectedEdgeInstanceId],
    queryFn: () => getStationViewEffective(validSelectedProjectId!, selectedEdgeInstanceId),
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
    enabled: stationConfigOpen && validSelectedProjectId !== undefined,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['edge', 'realtime-variables', validSelectedProjectId, selectedEdgeInstanceId],
    queryFn: () => getRealtimeVariables(validSelectedProjectId ? { project_id: validSelectedProjectId, edge_instance_id: selectedEdgeInstanceId } : {}),
    enabled: validSelectedProjectId !== undefined,
    staleTime: 30000,
    retry: false,
  })
  const pidVariablesQuery = useQuery({
    queryKey: ['station', 'pid-variables', validSelectedProjectId, selectedEdgeInstanceId, pidVarGroup],
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
  const currentRunQuery = useQuery({
    queryKey: ['station', 'current-run', validSelectedProjectId],
    queryFn: () => getCurrentDetectionRun(validSelectedProjectId!),
    enabled: validSelectedProjectId !== undefined && stationViewQuery.data?.http_companion.current_run_required === true,
    refetchInterval: stationViewQuery.data?.http_companion.current_run_required ? 5000 : false,
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
  const reportTemplatesQuery = useQuery({
    queryKey: ['station', 'report-templates'],
    queryFn: () => getReportTemplates({ enabled: true }),
    staleTime: 30000,
    retry: false,
  })
  const alarmsQuery = useQuery({
    queryKey: ['station', 'limit-alarms', validSelectedProjectId, alarmScope],
    queryFn: () =>
      getLimitAlarms({
        limit: 100,
        ...(validSelectedProjectId ? { project_id: validSelectedProjectId } : {}),
        ...(alarmScope === 'all' ? {} : { scope: alarmScope }),
      }),
    enabled: alarmModalOpen,
    refetchInterval: alarmModalOpen ? 5000 : false,
    retry: false,
  })
  const [wsSnapshotState, setWsSnapshotState] = useState<{ key: string; items: TagSnapshot[] }>({ key: '', items: [] })
  const [pidWsSnapshotState, setPIDWsSnapshotState] = useState<{ key: string; items: TagSnapshot[] }>({ key: '', items: [] })
  const wsVarIdsKey = (stationViewQuery.data?.ws_subscription.var_ids ?? []).join(',')
  const wsSubscriptionKey = `${validSelectedProjectId ?? ''}:${wsVarIdsKey}`
  useEffect(() => {
    if (!validSelectedProjectId || !stationViewQuery.data) return undefined
    return subscribeRealtimeWebSocket({
      subscription: {
        topics: ['realtime.variables'],
        edge_instance_id: selectedEdgeInstanceId,
        project_id: validSelectedProjectId,
        var_ids: stationViewQuery.data.ws_subscription.var_ids,
      },
      onMessage: (envelope) => {
        if (envelope.type !== 'realtime.variables.snapshot') return
        const payload = envelope.payload as RealtimeVariablesSnapshotPayload | undefined
        setWsSnapshotState({ key: wsSubscriptionKey, items: payload?.items ?? [] })
      },
    })
  }, [selectedEdgeInstanceId, validSelectedProjectId, stationViewQuery.data, wsSubscriptionKey])
  const pidVariables = useMemo(() => pidVariablesQuery.data ?? [], [pidVariablesQuery.data])
  const pidVarIds = useMemo(() => pidVariables.map(variableWireId).filter((value) => value !== undefined), [pidVariables])
  const pidSubscriptionKey = `${validSelectedProjectId ?? ''}:${pidVarIds.join(',')}`
  useEffect(() => {
    if (!pidModalOpen || !validSelectedProjectId || pidVarIds.length === 0) return undefined
    return subscribeRealtimeWebSocket({
      subscription: {
        topics: ['realtime.variables'],
        edge_instance_id: selectedEdgeInstanceId,
        project_id: validSelectedProjectId,
        var_ids: pidVarIds,
      },
      onMessage: (envelope) => {
        if (envelope.type !== 'realtime.variables.snapshot') return
        const payload = envelope.payload as RealtimeVariablesSnapshotPayload | undefined
        setPIDWsSnapshotState({ key: pidSubscriptionKey, items: payload?.items ?? [] })
      },
    })
  }, [pidModalOpen, selectedEdgeInstanceId, validSelectedProjectId, pidSubscriptionKey, pidVarIds])
  const variables = useMemo(() => {
    const merged = new Map<string, TagSnapshot>()
    for (const variable of variablesQuery.data ?? []) {
      merged.set(snapshotKey(variable), variable)
    }
    const currentWSSnapshots = wsSnapshotState.key === wsSubscriptionKey ? wsSnapshotState.items : []
    for (const variable of currentWSSnapshots) {
      merged.set(snapshotKey(variable), variable)
    }
    return Array.from(merged.values())
  }, [variablesQuery.data, wsSnapshotState, wsSubscriptionKey])
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === validSelectedProjectId),
    [projects, validSelectedProjectId],
  )
  const stationVariables = useMemo(
    () =>
      validSelectedProjectId
        ? variables.filter((variable) => variable.project_id === validSelectedProjectId)
        : variables,
    [validSelectedProjectId, variables],
  )
  const stationVariableOptions = useMemo(() => {
    const seen = new Set<string>()
    return stationVariables.flatMap((variable) => {
      if (!variable.var_name || seen.has(variable.var_name)) return []
      seen.add(variable.var_name)
      return [{
        label: `${alarmDisplayName(variable, i18n.resolvedLanguage)} / ${variable.var_name}`,
        value: variable.var_name,
      }]
    })
  }, [i18n.resolvedLanguage, stationVariables])
  const activeRun = validSelectedProjectId
    ? activeRunsQuery.data?.find((run) => run.project_id === validSelectedProjectId)
    : activeRunsQuery.data?.[0]
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
  const selectedRunProjectId = activeRun?.project_id ?? validSelectedProjectId
  const standards = useMemo(() => standardsQuery.data ?? [], [standardsQuery.data])
  const reportTemplates = useMemo(() => reportTemplatesQuery.data ?? [], [reportTemplatesQuery.data])
  const availableStandards = useMemo(
    () =>
      standards.filter(
        (standard) => !selectedRunProjectId || !standard.project_id || standard.project_id === selectedRunProjectId,
      ),
    [selectedRunProjectId, standards],
  )
  const selectedProjectName = useMemo(() => {
    if (!selectedProject) return undefined
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return selectedProject.display_name_en || selectedProject.project_code
    if (currentLanguage === 'ja') return selectedProject.display_name_ja || selectedProject.project_code
    return selectedProject.display_name || selectedProject.name || selectedProject.project_code
  }, [i18n.resolvedLanguage, selectedProject])
  const stationPreferenceKey = useMemo(
    () => stationViewPreferenceScope(validSelectedProjectId, selectedEdgeInstanceId),
    [selectedEdgeInstanceId, validSelectedProjectId],
  )
  const [stationPreferences, setStationPreferences] = useState<Record<string, StationViewPreference>>(
    readStationViewPreferences,
  )
  const currentStationPreference = stationPreferenceKey ? stationPreferences[stationPreferenceKey] : undefined
  const manualCardOrder = currentStationPreference?.cardOrder ?? emptyStationPreferenceList
  const pinnedRows = currentStationPreference?.pinnedRows ?? emptyStationPreferenceList
  const updateStationPreference = useCallback(
    (updater: (current: StationViewPreference) => StationViewPreference) => {
      if (!stationPreferenceKey) return
      setStationPreferences((preferences) => {
        const next = {
          ...preferences,
          [stationPreferenceKey]: updater(preferences[stationPreferenceKey] ?? {}),
        }
        writeStationViewPreferences(next)
        return next
      })
    },
    [stationPreferenceKey],
  )
  const [isStatusCollapsed, setStatusCollapsed] = useState(false)
  const snapshotsByVarID = useMemo(() => {
    const result = new Map<string, TagSnapshot>()
    for (const variable of stationVariables) {
      result.set(snapshotKey(variable), variable)
    }
    return result
  }, [stationVariables])
  const pidSnapshotsByVarID = useMemo(() => {
    const result = new Map<string, TagSnapshot>(snapshotsByVarID)
    const currentWSSnapshots = pidWsSnapshotState.key === pidSubscriptionKey ? pidWsSnapshotState.items : []
    for (const variable of currentWSSnapshots) {
      result.set(snapshotKey(variable), variable)
    }
    return result
  }, [pidSubscriptionKey, pidWsSnapshotState, snapshotsByVarID])
  const templateMetricBindings = useMemo(
    () =>
      (stationViewQuery.data?.items ?? [])
        .filter((item) => item.layout_area === stationLayoutAreaCardPool && item.visible !== false)
        .flatMap((item) => item.resolved_bindings ?? []),
    [stationViewQuery.data],
  )
  const metricBindings = useMemo(
    () =>
      sortStationBindingsByDefaultOrder(templateMetricBindings).slice(0, stationMetricCardLimit),
    [templateMetricBindings],
  )
  const defaultCardIds = useMemo(() => metricBindings.map((binding, index) => bindingKey(binding, index)), [metricBindings])
  const cardOrder = useMemo(() => {
    const next = manualCardOrder.filter((id) => defaultCardIds.includes(id))
    for (const id of defaultCardIds) {
      if (!next.includes(id)) next.push(id)
    }
    return next
  }, [defaultCardIds, manualCardOrder])
  const bindingByCardId = useMemo(() => {
    const result = new Map<string, StationViewResolvedBinding>()
    metricBindings.forEach((binding, index) => result.set(bindingKey(binding, index), binding))
    return result
  }, [metricBindings])
  const cards = useMemo<MetricCard[]>(
    () =>
      cardOrder
        .map((id, index) => {
          const binding = bindingByCardId.get(id)
          if (!binding) return undefined
          const snapshot = bindingWireId(binding) !== undefined ? snapshotsByVarID.get(String(bindingWireId(binding))) : undefined
          const value = numericSnapshotValue(snapshot)
          const limits = bindingLimits(binding)
          const Icon = iconForBinding(binding)
          return {
            id,
            label: bindingDisplayName(binding, i18n.resolvedLanguage),
            unit: binding.unit ?? '',
            color: cardColors[index % cardColors.length],
            icon: <Icon size={15} />,
            value,
            precision: binding.decimal_places ?? 2,
            trend: trendFromValue(value, limits.min, limits.max),
            ...(limits.min !== undefined ? { min: limits.min } : {}),
            ...(limits.max !== undefined ? { max: limits.max } : {}),
          }
        })
        .filter((card) => card !== undefined),
    [bindingByCardId, cardOrder, i18n.resolvedLanguage, snapshotsByVarID],
  )

  const runBindings = useMemo(
    () => (currentRunQuery.data?.standard_items ?? []).map(runBindingFromStandardItem),
    [currentRunQuery.data],
  )
  const templateTableBindings = useMemo(
    () =>
      (stationViewQuery.data?.items ?? [])
        .filter((item) => item.layout_area === stationLayoutAreaListLayout && item.visible !== false)
        .flatMap((item) => item.resolved_bindings ?? []),
    [stationViewQuery.data],
  )
  const fallbackTableBindings = useMemo(
    () => stationVariables.map(tableBindingFromSnapshot),
    [stationVariables],
  )
  const tableBindings = useMemo(
    () => {
      if (runBindings.length > 0) return sortStationBindingsByDefaultOrder(runBindings)
      if (templateTableBindings.length > 0) return sortStationBindingsByDefaultOrder(templateTableBindings)
      return sortStationBindingsByDefaultOrder(fallbackTableBindings)
    },
    [fallbackTableBindings, runBindings, templateTableBindings],
  )
  const stationRows = useMemo<StationTableRow[]>(
    () =>
      tableBindings.map((binding, index) => {
        const key = bindingKey(binding, index)
        const snapshot = bindingWireId(binding) !== undefined ? snapshotsByVarID.get(String(bindingWireId(binding))) : undefined
        const value = numericSnapshotValue(snapshot)
        return {
          key,
          name: bindingDisplayName(binding, i18n.resolvedLanguage),
          standard: formatStandardRange(binding),
          value: formatMetricValue(value, binding.unit ?? '', binding.decimal_places ?? 2),
          ok: isWithinLimits(value, binding),
        }
      }),
    [i18n.resolvedLanguage, snapshotsByVarID, tableBindings],
  )
  const sortedStationRows = useMemo(
    () =>
      [...stationRows].sort((a, b) => {
        const aIndex = pinnedRows.indexOf(a.key)
        const bIndex = pinnedRows.indexOf(b.key)
        if (aIndex !== -1 && bIndex !== -1) return aIndex - bIndex
        if (aIndex !== -1) return -1
        if (bIndex !== -1) return 1
        return 0
      }),
    [pinnedRows, stationRows],
  )
  const alarmOn = stationRows.some((row) => !row.ok)

  const refreshRuns = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['edge', 'active-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['station', 'current-run'] }),
      queryClient.invalidateQueries({ queryKey: ['station', 'view-effective'] }),
      queryClient.invalidateQueries({ queryKey: ['station', 'detection-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['history', 'data'] }),
    ])
  }

  useEffect(() => {
    if (!stationConfigOpen || !stationViewItemsQuery.data) return
    setStationCardConfigVarNames(
      stationViewVarNamesFromItems(stationViewItemsQuery.data.items, stationLayoutAreaCardPool).slice(0, stationMetricCardLimit),
    )
    setStationListConfigVarNames(
      stationViewVarNamesFromItems(stationViewItemsQuery.data.items, stationLayoutAreaListLayout),
    )
  }, [stationConfigOpen, stationViewItemsQuery.data])

  const saveStationViewItemsMutation = useMutation({
    mutationFn: () => {
      const templateUID = stationViewItemsQuery.data?.template_uid ?? stationViewQuery.data?.template.template_uid
      if (!templateUID) throw new Error(t('station.config.noTemplate'))
      const cardItems = stationCardConfigVarNames
        .slice(0, stationMetricCardLimit)
        .map((varName, index) => buildStationViewItemPayload(stationLayoutAreaCardPool, varName, index))
      const listItems = stationListConfigVarNames
        .map((varName, index) => buildStationViewItemPayload(stationLayoutAreaListLayout, varName, index))
      return replaceStationViewItems({ template_uid: templateUID, items: [...cardItems, ...listItems] })
    },
    onSuccess: async () => {
      messageApi.success(t('station.config.saveSuccess'))
      setStationConfigOpen(false)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['station', 'view-effective'] }),
        queryClient.invalidateQueries({ queryKey: ['station', 'view-items'] }),
      ])
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('station.config.saveFailed'))
    },
  })

  const startRunMutation = useMutation({
    mutationFn: (values: StartDetectionFormValues) => {
      const payload: DetectionRunStartPayload = {
        project_id: values.project_id,
        test_no: values.test_no.trim(),
        mode: values.mode,
        standard_id: values.standard_id,
        report_request: buildReportRequest(values),
        duration_sec: values.duration_min ? values.duration_min * 60 : undefined,
        operator_note: values.operator_note?.trim() || undefined,
      }
      return startDetectionRun(payload)
    },
    onSuccess: async () => {
      messageApi.success(t('station.messages.started'))
      setStartModalOpen(false)
      await refreshRuns()
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('station.messages.startFailed'))
    },
  })

  const stopRunMutation = useMutation({
    mutationFn: ({ runId, reason, abnormal }: { runId: number; reason: string; abnormal?: boolean }) =>
      abnormal ? abnormalStopDetectionRun(runId, { reason }) : stopDetectionRun(runId, { reason }),
    onSuccess: async (_, variables) => {
      messageApi.success(variables.abnormal ? t('station.messages.abnormalStopped') : t('station.messages.stopped'))
      await refreshRuns()
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('station.messages.stopFailed'))
    },
  })

  const writePIDSetting = useCallback(async (setting: PIDSettingItem, variable: VariableConfig, displayValue: string) => {
    const key = setting.key
    if (!isPIDWritable(variable)) {
      setPIDWriteStates((states) => ({
        ...states,
        [key]: { status: 'error', message: t('station.pid.readOnly'), submittedValue: displayValue },
      }))
      return false
    }
    let value: unknown
    try {
      value = coerceWriteValue(variable, String(pidWriteValue(setting.key, displayValue, 2)))
    } catch {
      messageApi.error(t('station.pid.invalidValue'))
      setPIDWriteStates((states) => ({
        ...states,
        [key]: { status: 'error', message: t('station.pid.invalidValue'), submittedValue: displayValue },
      }))
      return false
    }
    const commandID = `pid-${key}-${Date.now()}`
    setPIDWriteStates((states) => ({
      ...states,
      [key]: { status: 'pending', submittedValue: displayValue, submittedAt: new Date().toISOString() },
    }))
    setPIDWriteValues((values) => {
      const next = { ...values }
      delete next[key]
      return next
    })
    try {
      const result = await sendRealtimeWebSocketCommand<unknown, VariableWriteResult>({
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
        [key]: { status: 'ack', submittedValue: displayValue, submittedAt: new Date().toISOString(), result },
      }))
      return true
    } catch (error) {
      const commandError = error instanceof RealtimeWebSocketCommandError ? error : undefined
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
  }, [messageApi, selectedEdgeInstanceId, t, validSelectedProjectId])

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
    if (submitted > 0 && failed === 0) messageApi.success(t('station.pid.writeAck'))
    if (failed > 0) messageApi.error(t('station.pid.writeFailed'))
  }

  function togglePinnedRow(key: string) {
    updateStationPreference((preference) => {
      const rows = preference.pinnedRows ?? []
      return {
        ...preference,
        pinnedRows: rows.includes(key) ? rows.filter((row) => row !== key) : [...rows, key],
      }
    })
  }

  function openStartModal() {
    const targetProject = selectedProject ?? projects[0]
    startForm.setFieldsValue({
      project_id: targetProject?.id,
      test_no: `RUN-${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}`,
      mode: availableStandards[0]?.mode ?? 'standard',
      standard_id: availableStandards[0]?.id,
      report_requests: [{ template_id: reportTemplates[0]?.id, var_ids: [], params_json: '{\n  "inlet_area_m2": 1.25\n}' }],
      duration_min: 60,
    })
    setStartModalOpen(true)
  }

  function confirmStop(abnormal = false) {
    if (!activeRun) return
    Modal.confirm({
      title: abnormal ? t('station.run.abnormalStopTitle') : t('station.run.stopTitle'),
      content: abnormal ? t('station.run.abnormalStopDesc') : t('station.run.stopDesc'),
      okText: abnormal ? t('station.actions.abnormalStop') : t('station.actions.stop'),
      cancelText: t('actions.cancel'),
      okButtonProps: { danger: abnormal },
      onOk: () =>
        stopRunMutation.mutateAsync({
          runId: activeRun.id,
          reason: abnormal ? t('station.run.abnormalDefaultReason') : t('station.run.manualStopReason'),
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

  const statusProjectCode = selectedProject?.project_code ?? activeRun?.project_code ?? 'SN-20230912'
  const statusProject = selectedProjectName ?? activeRun?.test_no ?? t('station.status.mockProject')
  const statusConfig = selectedProject?.model_name || activeRun?.mode || 'A'
  const statusTask = activeRun?.test_no ?? t('station.run.idle')
  const selectedStandardLabel = activeRun?.standard_code || availableStandards[0]?.standard_code || '--'
  const effectiveTemplate = stationViewQuery.data?.template
  const visibleTemplates = stationViewTemplatesQuery.data?.items ?? []
  const enabledAssignments = visibleTemplates.reduce(
    (count, template) => count + (template.assignments ?? []).filter((assignment) => assignment.enabled).length,
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
            {scope === 'default' ? t('station.alarms.scopeDefault') : t('station.alarms.scopeDetection')}
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
        render: (level: string) => <Tag color={level === 'HH' || level === 'LL' ? 'red' : 'orange'}>{level}</Tag>,
      },
      {
        title: t('station.alarms.status'),
        dataIndex: 'status',
        key: 'status',
        width: 92,
        render: (status: string) => (
          <span className={status === 'active' ? 'status-ng' : 'status-ok'}>
            <span />
            {status === 'active' ? t('station.alarms.active') : t('station.alarms.closed')}
          </span>
        ),
      },
      {
        title: t('station.alarms.values'),
        key: 'values',
        width: 170,
        render: (_: unknown, record: LimitAlarm) => (
          <div className="station-alarm-values">
            <span>{t('station.alarms.startValue')}: {formatAlarmValue(record.start_value)}</span>
            <span>{t('station.alarms.limitValue')}: {formatAlarmValue(record.limit_value)}</span>
            <span>{t('station.alarms.recoverValue')}: {formatAlarmValue(record.recover_value)}</span>
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
        render: (value: string) => <Tag color={value === 'wide_table' ? 'blue' : 'default'}>{value}</Tag>,
      },
      {
        title: t('station.storage.tableColumn'),
        key: 'tableColumn',
        width: 260,
        render: (_: unknown, record: DetectionRunStorageRoute) => (
          <div className="station-alarm-values">
            <span>{record.table_name || '--'}</span>
            <span>{record.column_name || '--'} / {record.column_type || '--'}</span>
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
        render: (value: boolean) => <Tag color={value ? 'green' : 'default'}>{value ? t('station.storage.yes') : t('station.storage.no')}</Tag>,
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
            <span>LL/L: {formatAlarmValue(record.limit_ll)} / {formatAlarmValue(record.limit_l)}</span>
            <span>H/HH: {formatAlarmValue(record.limit_h)} / {formatAlarmValue(record.limit_hh)}</span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.defaultAlarm'),
        key: 'defaultAlarm',
        width: 130,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <Tag color={record.variable_default_alarm_enabled ? 'cyan' : 'default'}>
            {record.variable_default_alarm_enabled ? t('station.storage.yes') : t('station.storage.no')}
          </Tag>
        ),
      },
      {
        title: t('station.snapshot.defaultLimit'),
        key: 'defaultLimit',
        width: 230,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>LL/L: {formatAlarmValue(record.variable_default_limit_ll ?? undefined)} / {formatAlarmValue(record.variable_default_limit_l ?? undefined)}</span>
            <span>H/HH: {formatAlarmValue(record.variable_default_limit_h ?? undefined)} / {formatAlarmValue(record.variable_default_limit_hh ?? undefined)}</span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.policy'),
        key: 'policy',
        width: 220,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>{t('station.snapshot.deadband')}: {formatAlarmValue(record.variable_default_limit_deadband)}</span>
            <span>{t('station.snapshot.hold')}: {record.variable_default_violation_hold_ms} / {record.variable_default_recover_hold_ms} ms</span>
          </div>
        ),
      },
      {
        title: t('station.snapshot.check'),
        key: 'check',
        width: 180,
        render: (_: unknown, record: DetectionRunStandardItem) => (
          <div className="station-alarm-values">
            <span>{record.alarm_enabled ? t('station.snapshot.alarmOn') : t('station.snapshot.alarmOff')}</span>
            <span>{record.check_on_start ? t('station.snapshot.checkOnStart') : t('station.snapshot.checkByCycle')} / {record.check_cycle_ms} ms</span>
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
            <span>{record.var_name || record.var_id_text || record.var_id}</span>
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
          onOrderChange={(ids) =>
            updateStationPreference((preference) => ({
              ...preference,
              cardOrder: ids,
            }))
          }
          t={t}
          warnings={stationViewQuery.data?.warnings ?? []}
        />

        <aside className="station-side">
          <section
            className={isStatusCollapsed ? 'station-status-card glass-panel collapsed' : 'station-status-card glass-panel'}
            onClick={() => setStatusCollapsed((value) => !value)}
          >
            {isStatusCollapsed ? (
              <>
                <div className="status-collapsed-left">
                  <strong>{statusProjectCode}</strong>
                  <span>{statusProject}</span>
                  <span>{activeRun ? activeRun.test_no : t('station.run.idle')}</span>
                </div>
                <div className={activeRun ? 'station-ok compact running' : 'station-ok compact'}>{activeRun ? 'RUN' : 'OK'}</div>
              </>
            ) : (
              <>
                <div className="station-status-top">
                  <div className="station-status-main">
                    <div>
                      <span className="eyebrow">{t('station.status.projectId')}</span>
                      <strong>{statusProjectCode}</strong>
                    </div>
                    <div>
                      <span className="eyebrow">{t('station.status.project')}</span>
                      <strong className="serif">{statusProject}</strong>
                    </div>
                  </div>
                  <div className="station-result">
                    <div className={activeRun ? 'station-ok running' : 'station-ok'}>{activeRun ? 'RUN' : 'OK'}</div>
                    <div className="station-normal">
                      <span />
                      {activeRun ? t('station.run.running') : t('station.status.normal')}
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
              icon={<Settings size={15} />}
              disabled={!validSelectedProjectId || !canManageStationView}
              onClick={() => setStationConfigOpen(true)}
            >
              {t('station.actions.config')}
            </Button>
            <Button icon={<Gauge size={15} />} disabled={!validSelectedProjectId} onClick={() => setPIDModalOpen(true)}>
              {t('station.actions.pid')}
            </Button>
            <Button>{t('station.actions.mute')}</Button>
            <Button className={alarmOn ? 'alarm-active' : undefined} onClick={() => setAlarmModalOpen(true)}>
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
                      const pinned = pinnedRows.includes(row.key)
                      return (
                        <tr
                          className={pinned ? 'station-row pinned' : 'station-row'}
                          key={row.key}
                          onClick={() => togglePinnedRow(row.key)}
                        >
                          <td>
                            <span className="pin-indicator" />
                            {row.name}
                          </td>
                          <td>{row.standard}</td>
                          <td className="mono">{row.value}</td>
                          <td>
                            <span className={row.ok ? 'status-ok' : 'status-ng'}>
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
      <div className="station-template-footnote" aria-label={t('station.template.trace')}>
        <span>{t('station.template.trace')}</span>
        <strong>{effectiveTemplate?.template_code ?? '--'}</strong>
        <span>v{effectiveTemplate?.version ?? '-'} / {effectiveTemplate?.status ?? '-'}</span>
        <span>{t('station.template.visibleTemplates')}: {stationViewTemplatesQuery.isFetching ? '...' : visibleTemplates.length}</span>
        <span>{t('station.template.assignments')}: {stationViewTemplatesQuery.isFetching ? '...' : enabledAssignments}</span>
      </div>
      <Modal
        className="station-alarm-modal station-config-modal"
        title={t('station.config.title')}
        open={stationConfigOpen}
        onCancel={() => setStationConfigOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setStationConfigOpen(false)}>{t('actions.cancel')}</Button>,
          <Button
            key="submit"
            type="primary"
            icon={<Save size={15} />}
            loading={saveStationViewItemsMutation.isPending}
            onClick={() => saveStationViewItemsMutation.mutate()}
          >
            {t('actions.save')}
          </Button>,
        ]}
        centered
        width="min(860px, calc(100vw - 48px))"
        destroyOnHidden
      >
        <div className="station-config-body">
          <div className="station-config-context">
            <span>{selectedProjectName ?? statusProjectCode}</span>
            <Tag>{effectiveTemplate?.template_code ?? stationViewItemsQuery.data?.template_uid ?? '--'}</Tag>
          </div>
          <section className="station-config-section">
            <div className="station-config-section-head">
              <strong>{t('station.config.cardPool')}</strong>
              <span>{t('station.config.cardHint', { count: stationMetricCardLimit })}</span>
            </div>
            <Select
              mode="multiple"
              allowClear
              showSearch
              maxTagCount="responsive"
              optionFilterProp="label"
              loading={stationViewItemsQuery.isFetching || variablesQuery.isFetching}
              value={stationCardConfigVarNames}
              placeholder={t('station.config.variablePlaceholder')}
              options={stationVariableOptions}
              onChange={(next) => setStationCardConfigVarNames(next.slice(0, stationMetricCardLimit))}
            />
          </section>
          <section className="station-config-section">
            <div className="station-config-section-head">
              <strong>{t('station.config.listLayout')}</strong>
              <span>{t('station.config.listHint')}</span>
            </div>
            <Select
              mode="multiple"
              allowClear
              showSearch
              maxTagCount="responsive"
              optionFilterProp="label"
              loading={stationViewItemsQuery.isFetching || variablesQuery.isFetching}
              value={stationListConfigVarNames}
              placeholder={t('station.config.variablePlaceholder')}
              options={stationVariableOptions}
              onChange={setStationListConfigVarNames}
            />
          </section>
        </div>
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
        <Form form={startForm} layout="vertical" onFinish={(values) => startRunMutation.mutate(values)}>
          <div className="station-run-layout">
            <section className="station-run-section">
              <div className="station-run-section-head">
                <span>{t('station.run.basicInfo')}</span>
              </div>
              <div className="station-run-form-grid">
                <Form.Item name="project_id" label={t('station.run.project')} rules={[{ required: true }]}>
                  <Select
                    options={projects.map((project) => ({
                      label: `${displayProjectName(project, i18n.resolvedLanguage)} / ${project.project_code}`,
                      value: project.id,
                    }))}
                  />
                </Form.Item>
                <Form.Item name="test_no" label={t('station.run.testNo')} rules={[{ required: true }]}>
                  <Input />
                </Form.Item>
                <Form.Item name="mode" label={t('station.run.mode')} rules={[{ required: true }]}>
                  <Select
                    options={[
                      { label: t('station.run.standardMode'), value: 'standard' },
                      { label: t('station.run.performanceMode'), value: 'performance' },
                    ]}
                  />
                </Form.Item>
                <Form.Item name="duration_min" label={t('station.run.durationMin')}>
                  <InputNumber min={1} precision={0} style={{ width: '100%' }} />
                </Form.Item>
              </div>
            </section>

            <section className="station-run-section">
              <div className="station-run-section-head">
                <span>{t('station.run.standardAndTemplate')}</span>
              </div>
              <div className="station-run-form-grid station-run-form-grid-compact">
                <Form.Item name="standard_id" label={t('station.run.standard')}>
                  <Select
                    allowClear
                    loading={standardsQuery.isFetching}
                    options={availableStandards.map((standard) => ({
                      label: `${standardDisplayName(standard, i18n.resolvedLanguage)} / ${standard.standard_code}`,
                      value: standard.id,
                    }))}
                  />
                </Form.Item>
              </div>
            </section>

            <section className="station-run-section station-run-report-section">
              <div className="station-run-report-head">
                <div>
                  <strong>{t('station.run.reportRequests')}</strong>
                  <span>{t('station.run.reportRequestsHint')}</span>
                </div>
              </div>
              <Form.List name="report_requests">
                {(fields, { add, remove }) => (
                  <div className="station-run-report-list">
                    {fields.map((field, index) => (
                      <div className="station-run-report-row" key={field.key}>
                        <div className="station-run-report-row-head">
                          <span>{t('station.run.reportRequestIndex', { index: index + 1 })}</span>
                          <Button
                            danger
                            size="small"
                            icon={<Trash2 size={13} />}
                            title={t('station.run.removeReportRequest')}
                            onClick={() => remove(field.name)}
                          />
                        </div>
                        <div className="station-run-report-fields">
                          <Form.Item {...field} name={[field.name, 'template_id']} label={t('station.run.reportTemplate')}>
                            <Select
                              allowClear
                              loading={reportTemplatesQuery.isFetching}
                              options={reportTemplates.map((template) => ({
                                label: `${template.display_name || template.name || template.template_code} / ${template.template_code}`,
                                value: template.id,
                              }))}
                            />
                          </Form.Item>
                          <Form.Item {...field} name={[field.name, 'report_name']} label={t('station.run.reportName')}>
                            <Input />
                          </Form.Item>
                          <Form.Item {...field} className="station-run-report-field-wide" name={[field.name, 'var_ids']} label={t('station.run.reportVariables')}>
                            <Select
                              allowClear
                              mode="multiple"
                              optionFilterProp="label"
                              options={stationVariables.map((variable) => ({
                                label: `${alarmDisplayName(variable, i18n.resolvedLanguage)} / ${variable.var_name}`,
                                value: tagWireId(variable),
                              }))}
                            />
                          </Form.Item>
                          <Form.Item {...field} className="station-run-report-field-wide" name={[field.name, 'params_json']} label={t('station.run.reportParams')}>
                            <Input.TextArea rows={3} spellCheck={false} />
                          </Form.Item>
                        </div>
                      </div>
                    ))}
                    <Button className="station-run-add-report" size="small" icon={<Plus size={14} />} onClick={() => add({ template_id: reportTemplates[0]?.id, var_ids: [], params_json: '{}' })}>
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
            <Button onClick={() => setStartModalOpen(false)}>{t('actions.cancel')}</Button>
            <Button type="primary" htmlType="submit" icon={<Play size={15} />} loading={startRunMutation.isPending}>
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
          <Button key="cancel" onClick={() => setPIDModalOpen(false)}>{t('actions.cancel')}</Button>,
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
            <span>{validSelectedProjectId ? selectedProjectName ?? statusProjectCode : t('station.status.noProject')}</span>
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
            <Button size="small" onClick={() => pidVariablesQuery.refetch()} loading={pidVariablesQuery.isFetching}>
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
                  const snapshot = variable ? pidSnapshotsByVarID.get(String(variableWireId(variable))) : undefined
                  const currentValue = pidDisplayValue(setting.key, numericSnapshotValue(snapshot), 2)
                  const draftValue = pidWriteValues[setting.key] ?? ''
                  const state = pidWriteStates[setting.key]
                  const readback = isPIDReadbackMatch(state?.submittedValue, currentValue)
                  const color = state?.status === 'ack' ? 'green' : state?.status === 'sent' ? 'cyan' : state?.status === 'error' ? 'red' : state?.status === 'pending' ? 'blue' : 'default'
                  const canWrite = variable ? isPIDWritable(variable) : false
                  const inputBaseValue = draftValue || currentValue || '0'
                  return (
                    <div className="station-pid-setting" key={setting.key}>
                      <div className="station-pid-setting-head">
                        <span>{t(setting.labelKey)}</span>
                        {variable ? (
                          <Tag color={canWrite ? color : 'default'}>{canWrite ? (state?.status ? t(`station.pid.${state.status}`) : t('station.pid.idle')) : t('station.pid.readOnly')}</Tag>
                        ) : (
                          <Tag color="red">{t('station.pid.noConnection')}</Tag>
                        )}
                      </div>
                      {variable ? (
                        <>
                          <div className="station-pid-variable-name">
                            {variableDisplayName(variable, i18n.resolvedLanguage)} / {variable.var_name}
                          </div>
                          <div className="station-pid-current-row">
                            <span>{t('station.pid.currentValue')}</span>
                            <strong>{currentValue || '--'}{setting.unit ? ` ${setting.unit}` : ''}</strong>
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
                                  [setting.key]: formatPIDNumber(current - setting.step, setting.precision),
                                }))
                              }}
                            />
                            <Input
                              size="small"
                              value={draftValue}
                              placeholder={currentValue || '--'}
                              readOnly={!canWrite}
                              onChange={(event) => setPIDWriteValues((values) => ({ ...values, [setting.key]: event.target.value }))}
                              onPressEnter={() => {
                                if (canWrite && draftValue.trim()) void writePIDSetting(setting, variable, draftValue)
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
                                  [setting.key]: formatPIDNumber(current + setting.step, setting.precision),
                                }))
                              }}
                            />
                          </div>
                          <div className="station-pid-setting-foot">
                            <span>{snapshot?.last_update ? formatAlarmTime(snapshot.last_update) : '-'}</span>
                            {draftValue ? <Tag color="gold">{t('station.pid.draftValue')}: {draftValue}</Tag> : null}
                            {state?.submittedValue ? <Tag>{t('station.pid.submittedValue')}: {state.submittedValue}</Tag> : null}
                            {readback ? <Tag color="cyan">{t('station.pid.readback')}</Tag> : null}
                            {state?.message ? <span>{state.message}</span> : null}
                            {state?.result?.kio?.status ? <span>{state.result.kio.status}</span> : null}
                          </div>
                        </>
                      ) : (
                        <div className="station-pid-disconnected">{t('station.pid.noConnection')}</div>
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
              {validSelectedProjectId ? t('station.alarms.currentProject', { name: selectedProjectName ?? statusProjectCode }) : t('station.alarms.allProjects')}
            </span>
            <Button size="small" onClick={() => alarmsQuery.refetch()} loading={alarmsQuery.isFetching}>
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
            <span>{t('station.snapshot.count', { count: runSnapshotQuery.data?.standard_items?.length ?? 0 })}</span>
            <Button size="small" onClick={() => runSnapshotQuery.refetch()} loading={runSnapshotQuery.isFetching}>
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunStandardItem>
          rowKey={(record) => `${record.task_id}-${record.standard_item_id}-${record.var_id_text ?? record.var_id}`}
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
            <span>{t('station.snapshot.reportRequestCount', { count: reportRequestsQuery.data?.count ?? runSnapshotQuery.data?.report_requests?.length ?? 0 })}</span>
            <Button size="small" onClick={() => reportRequestsQuery.refetch()} loading={reportRequestsQuery.isFetching}>
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunReportRequest>
          rowKey={(record) => record.id}
          size="small"
          columns={reportRequestColumns}
          dataSource={reportRequestsQuery.data?.items ?? runSnapshotQuery.data?.report_requests ?? []}
          loading={reportRequestsQuery.isFetching || runSnapshotQuery.isFetching}
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
            <span>{t('station.storage.count', { count: storageSnapshotQuery.data?.count ?? 0 })}</span>
            <Button size="small" onClick={() => storageSnapshotQuery.refetch()} loading={storageSnapshotQuery.isFetching}>
              {t('actions.refresh')}
            </Button>
          </div>
        </div>
        <Table<DetectionRunStorageRoute>
          rowKey={(record) => `${record.task_id}-${record.route_id}-${record.var_id_text ?? record.var_id}`}
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
  onOrderChange,
  t,
  warnings,
}: {
  cards: MetricCard[]
  onOrderChange: (ids: string[]) => void
  t: (key: string) => string
  warnings: string[]
}) {
  const [activeId, setActiveId] = useState<UniqueIdentifier | null>(null)
  const [droppingId, setDroppingId] = useState<UniqueIdentifier | null>(null)
  const [canScrollDown, setCanScrollDown] = useState(false)
  const [canScrollUp, setCanScrollUp] = useState(false)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const activeCard = cards.find((card) => card.id === activeId)

  useEffect(() => {
    const scrollElement = scrollRef.current
    if (!scrollElement) return undefined

    const checkScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = scrollElement
      setCanScrollUp(scrollTop > 2)
      setCanScrollDown(scrollHeight > clientHeight && scrollTop + clientHeight < scrollHeight - 2)
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

  function reorderCards(active: UniqueIdentifier, over: UniqueIdentifier) {
    if (active === over) return
    const oldIndex = cards.findIndex((item) => item.id === active)
    const newIndex = cards.findIndex((item) => item.id === over)
    if (oldIndex === -1 || newIndex === -1) return
    onOrderChange(arrayMove(cards, oldIndex, newIndex).map((item) => item.id))
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id)
  }

  function handleDragOver(event: DragOverEvent) {
    if (!event.over) return
    reorderCards(event.active.id, event.over.id)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveId(null)
    setDroppingId(event.active.id)
    window.setTimeout(() => setDroppingId(null), 500)
  }

  const dropAnimation = {
    sideEffects: defaultDropAnimationSideEffects({
      styles: {
        active: { opacity: '1' },
      },
    }),
    duration: 500,
    easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
      onDragCancel={() => {
        setActiveId(null)
        setDroppingId(null)
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
              <SortableContext items={cards.map((card) => card.id)} strategy={rectSortingStrategy}>
                {cards.map((card) => (
                  <SortableMetricCard
                    key={card.id}
                    card={card}
                    label={card.label}
                    isDropping={droppingId === card.id}
                  />
                ))}
              </SortableContext>
            </div>
          )}
        </div>
        <button
          className={canScrollUp ? 'station-scroll-cue top visible' : 'station-scroll-cue top'}
          onClick={() => scrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })}
          aria-label={t('station.actions.scrollTop')}
        >
          <ChevronUp size={12} />
        </button>
        <button
          className={canScrollDown ? 'station-scroll-cue bottom visible' : 'station-scroll-cue bottom'}
          onClick={() => scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })}
          aria-label={t('station.actions.scrollBottom')}
        >
          <ChevronDown size={12} />
        </button>
      </div>
      <DragOverlay dropAnimation={dropAnimation}>
        {activeCard ? <MetricCardView card={activeCard} label={activeCard.label} dragging /> : null}
      </DragOverlay>
    </DndContext>
  )
}

function SortableMetricCard({ card, label, isDropping }: { card: MetricCard; label: string; isDropping: boolean }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: card.id })
  if (isDragging || isDropping) {
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
      style={{ transform: CSS.Translate.toString(transform), transition }}
      {...attributes}
      {...listeners}
    >
      <MetricCardView card={card} label={label} />
    </div>
  )
}

function MetricCardView({ card, label, dragging = false }: { card: MetricCard; label: string; dragging?: boolean }) {
  return (
    <article className={dragging ? 'metric-card glass-panel dragging' : 'metric-card glass-panel'}>
      <div className="metric-card-head">
        <div className="metric-title-group">
          <span className="metric-icon" style={{ color: card.color, backgroundColor: `${card.color}18` }}>
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
      </div>
      <div className="metric-chart">
        <CardChart chartData={card.trend} legendName={label} min={card.min} max={card.max} />
      </div>
    </article>
  )
}

function CardChart({
  chartData,
  legendName,
  min,
  max,
}: {
  chartData: TrendPoint[]
  legendName: string
  min?: number
  max?: number
}) {
  const dataValues = chartData.map((item) => item.value)
  const dataMin = Math.min(...dataValues)
  const dataMax = Math.max(...dataValues)
  const yMin = Math.min(dataMin, min ?? dataMin)
  const yMax = Math.max(dataMax, max ?? dataMax)
  const range = yMax - yMin
  const buffer = range === 0 ? yMax * 0.1 || 1 : range * 0.1
  const domain = [Math.floor(yMin - buffer), Math.ceil(yMax + buffer)]

  return (
    <div className="card-chart-line">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={chartData} margin={{ top: 20, right: 10, left: -25, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 4" vertical={false} stroke="rgba(30,27,24,0.06)" />
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
            itemStyle={{ color: '#333', fontWeight: 600, fontFamily: 'Georgia, serif', fontSize: 13 }}
            labelStyle={{ display: 'none' }}
          />
          {min !== undefined ? (
            <ReferenceLine
              y={min}
              stroke="#ff4d4f"
              strokeDasharray="2 3"
              strokeWidth={1}
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
            stroke="#333"
            strokeWidth={1.5}
            fillOpacity={0.02}
            fill="#333"
            isAnimationActive={false}
            activeDot={{ r: 3, strokeWidth: 0, fill: '#333' }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
