import { useEffect, useMemo, useRef, useState } from 'react'
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
  Settings,
  Square,
  Thermometer,
  Volume2,
  Waves,
  Wind,
  AlertTriangle,
  Database,
} from 'lucide-react'
import type {
  DetectionRunStandardItem,
  DetectionRunReportRequest,
  DetectionRunReportRequestPayload,
  DetectionRunStartPayload,
  DetectionRunStorageRoute,
  LimitAlarm,
  LimitAlarmScope,
  TagSnapshot,
} from '@/shared/api/types'
import { useAuthStore } from '@/features/auth/authStore'
import {
  abnormalStopDetectionRun,
  getActiveDetectionRuns,
  getDetectionRun,
  getDetectionRunReportRequests,
  getDetectionRunStorageRoutes,
  getDetectionStandards,
  getProjects,
  getLimitAlarms,
  getRealtimeVariables,
  getReportTemplates,
  startDetectionRun,
  stopDetectionRun,
} from '@/features/edge-status/api'
import { languageCode } from '@/shared/i18n/language'
import { StationCardGridStyles } from './components/StationCardGridStyles'
import { StationLightBackground } from './components/StationLightBackground'

type TrendPoint = {
  time: string
  value: number
}

type MetricCard = {
  id: string
  labelKey: string
  unit: string
  color: string
  min: number
  max: number
  icon: ReactNode
  value: number
}

type StartDetectionFormValues = {
  project_id: number
  test_no: string
  mode: string
  standard_id?: number
  report_template_id?: number
  report_var_ids?: Array<string | number>
  report_ext_1?: string
  report_ext_2?: string
  report_ext_3?: string
  duration_min?: number
  operator_note?: string
}

type AlarmScopeFilter = 'all' | LimitAlarmScope

const chartData: TrendPoint[] = [
  { time: '08:00', value: 44 },
  { time: '09:00', value: 47.5 },
  { time: '10:00', value: 52.1 },
  { time: '11:00', value: 55.4 },
  { time: '12:00', value: 59.7 },
  { time: '13:00', value: 54.3 },
  { time: '14:00', value: 51.2 },
]

const metricSeed = [
  { id: 'temp_out', labelKey: 'station.metrics.tempOut', unit: '℃', color: '#c2410c', min: 48, max: 55, icon: Thermometer },
  { id: 'humid_out', labelKey: 'station.metrics.humidOut', unit: '%RH', color: '#0f766e', min: 20, max: 40, icon: Droplets },
  { id: 'wind_in', labelKey: 'station.metrics.windIn', unit: 'm³/h', color: '#2563eb', min: 120, max: 160, icon: Wind },
  { id: 'noise', labelKey: 'station.metrics.noise', unit: 'dB', color: '#b45309', min: 40, max: 75, icon: Volume2 },
  { id: 'vibration', labelKey: 'station.metrics.vibration', unit: 'mm', color: '#7c3aed', min: 0.5, max: 2, icon: Waves },
  { id: 'temp_in', labelKey: 'station.metrics.tempIn', unit: '℃', color: '#15803d', min: 22, max: 28, icon: Thermometer },
  { id: 'pressure', labelKey: 'station.metrics.pressure', unit: 'kPa', color: '#be185d', min: 100, max: 150, icon: Gauge },
  { id: 'power', labelKey: 'station.metrics.power', unit: 'kW', color: '#dc2626', min: 2, max: 4.5, icon: Power },
]

function valueFromSnapshot(tags: TagSnapshot[], index: number, fallback: number) {
  const tag = tags[index]
  if (!tag || tag.is_string || !Number.isFinite(tag.value)) return fallback
  return tag.value
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

function buildReportRequest(values: StartDetectionFormValues): DetectionRunReportRequestPayload | undefined {
  const varIds = (values.report_var_ids ?? []).filter((item) => item !== undefined && item !== null && item !== '')
  const payload: DetectionRunReportRequestPayload = {}
  if (varIds.length > 0) payload.var_ids = varIds
  if (values.report_ext_1?.trim()) payload.ext_1 = values.report_ext_1.trim()
  if (values.report_ext_2?.trim()) payload.ext_2 = values.report_ext_2.trim()
  if (values.report_ext_3?.trim()) payload.ext_3 = values.report_ext_3.trim()
  return Object.keys(payload).length > 0 ? payload : undefined
}

function tagWireId(variable: Pick<TagSnapshot, 'var_id' | 'var_id_text'>) {
  return variable.var_id_text ?? variable.var_id
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
  const [storageSnapshotOpen, setStorageSnapshotOpen] = useState(false)
  const [runSnapshotOpen, setRunSnapshotOpen] = useState(false)
  const [alarmScope, setAlarmScope] = useState<AlarmScopeFilter>('all')
  const hasPermission = useAuthStore((state) => state.hasPermission)
  const canStartDetection = hasPermission('start_detection')
  const canStopDetection = hasPermission('stop_detection')
  const selectedProjectId = Number(searchParams.get('project_id') ?? searchParams.get('device_id'))
  const validSelectedProjectId = Number.isFinite(selectedProjectId) && selectedProjectId > 0 ? selectedProjectId : undefined
  const variablesQuery = useQuery({
    queryKey: ['edge', 'realtime-variables', validSelectedProjectId],
    queryFn: () => getRealtimeVariables(validSelectedProjectId ? { project_id: validSelectedProjectId } : {}),
    refetchInterval: 2000,
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
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === validSelectedProjectId),
    [projects, validSelectedProjectId],
  )
  const stationVariables = useMemo(
    () =>
      validSelectedProjectId
        ? variables.filter((variable) => variable.project_id === validSelectedProjectId || variable.device_id === validSelectedProjectId)
        : variables,
    [validSelectedProjectId, variables],
  )
  const activeRun =
    activeRunsQuery.data?.find((run) => run.project_id === validSelectedProjectId || run.device_id === validSelectedProjectId) ?? activeRunsQuery.data?.[0]
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
  const selectedRunProjectId = activeRun?.project_id ?? activeRun?.device_id ?? validSelectedProjectId
  const standards = useMemo(() => standardsQuery.data ?? [], [standardsQuery.data])
  const reportTemplates = useMemo(() => reportTemplatesQuery.data ?? [], [reportTemplatesQuery.data])
  const availableStandards = useMemo(
    () =>
      standards.filter(
        (standard) => !selectedRunProjectId || !standard.project_id || standard.project_id === selectedRunProjectId || standard.device_id === selectedRunProjectId,
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
  const [cardOrder, setCardOrder] = useState(metricSeed.map((item) => item.id))
  const [isStatusCollapsed, setStatusCollapsed] = useState(false)
  const [pinnedRows, setPinnedRows] = useState<string[]>([])
  const cards = useMemo<MetricCard[]>(
    () =>
      cardOrder.map((id, index) => {
        const seed = metricSeed.find((item) => item.id === id) ?? metricSeed[0]
        const Icon = seed.icon
        return {
          ...seed,
          icon: <Icon size={15} />,
          value: valueFromSnapshot(stationVariables, index, (seed.min + seed.max) / 2),
        }
      }),
    [cardOrder, stationVariables],
  )

  const stationRows = useMemo(
    () => {
      const baseRows = cards.map((card) => ({
        name: t(card.labelKey),
        standard: `${card.min} - ${card.max} ${card.unit}`,
        value: `${card.value.toFixed(card.unit === 'mm' ? 3 : 1)} ${card.unit}`,
        ok: card.value >= card.min && card.value <= card.max,
      }))
      const extraRows = [
        ['station.metrics.humidIn', '40 - 60 %RH', '45.3 %RH'],
        ['station.metrics.compressorSuctionTemp', '10 - 15 ℃', '12.4 ℃'],
        ['station.metrics.compressorDischargeTemp', '70 - 90 ℃', '85.6 ℃'],
        ['station.metrics.evaporatorOutletTemp', '5 - 12 ℃', '8.2 ℃'],
        ['station.metrics.condenserOutletTemp', '30 - 45 ℃', '35.4 ℃'],
        ['station.metrics.expansionValveOutletTemp', '2 - 8 ℃', '5.1 ℃'],
        ['station.metrics.coolingWaterInletTemp', '25 - 32 ℃', '28.5 ℃'],
        ['station.metrics.coolingWaterOutletTemp', '30 - 38 ℃', '33.2 ℃'],
        ['station.metrics.humidifierWaterTemp', '10 - 20 ℃', '15.6 ℃'],
        ['station.metrics.reheaterOutletTemp', '35 - 50 ℃', '42.1 ℃'],
      ].map(([nameKey, standard, value]) => ({
        name: t(nameKey),
        standard,
        value,
        ok: true,
      }))
      return [...baseRows, ...extraRows]
    },
    [cards, t],
  )
  const sortedStationRows = useMemo(
    () =>
      [...stationRows].sort((a, b) => {
        const aIndex = pinnedRows.indexOf(a.name)
        const bIndex = pinnedRows.indexOf(b.name)
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
      queryClient.invalidateQueries({ queryKey: ['station', 'detection-runs'] }),
      queryClient.invalidateQueries({ queryKey: ['history', 'data'] }),
    ])
  }

  const startRunMutation = useMutation({
    mutationFn: (values: StartDetectionFormValues) => {
      const payload: DetectionRunStartPayload = {
        project_id: values.project_id,
        test_no: values.test_no.trim(),
        mode: values.mode,
        standard_id: values.standard_id,
        report_template_id: values.report_template_id,
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

  function togglePinnedRow(name: string) {
    setPinnedRows((rows) => (rows.includes(name) ? rows.filter((row) => row !== name) : [...rows, name]))
  }

  function openStartModal() {
    const targetProject = selectedProject ?? projects[0]
    startForm.setFieldsValue({
      project_id: targetProject?.id,
      test_no: `RUN-${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}`,
      mode: availableStandards[0]?.mode ?? 'standard',
      standard_id: availableStandards[0]?.id,
      report_template_id: reportTemplates[0]?.id,
      report_var_ids: [],
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
      project_id: String(activeRun.project_id ?? activeRun.device_id),
      test_no: activeRun.test_no,
    })
    navigate(`/history?${params.toString()}`)
  }

  const statusProjectCode = selectedProject?.project_code ?? activeRun?.project_code ?? activeRun?.device_code ?? 'SN-20230912'
  const statusProject = selectedProjectName ?? activeRun?.test_no ?? t('station.status.mockProject')
  const statusConfig = selectedProject?.model_name || activeRun?.mode || 'A'
  const statusTask = activeRun?.test_no ?? t('station.run.idle')
  const selectedStandardLabel = activeRun?.standard_code || availableStandards[0]?.standard_code || '--'
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
        <SortableMetricGrid cards={cards} onOrderChange={setCardOrder} t={t} />

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
                      <span className="eyebrow">{t('station.status.device')}</span>
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
            <Button icon={<Settings size={15} />}>{t('station.actions.config')}</Button>
            <Button>{t('station.actions.pid')}</Button>
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
                  {sortedStationRows.map((row) => {
                    const pinned = pinnedRows.includes(row.name)
                    return (
                      <tr
                        className={pinned ? 'station-row pinned' : 'station-row'}
                        key={row.name}
                        onClick={() => togglePinnedRow(row.name)}
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
                  })}
                </tbody>
              </table>
            </div>
          </section>
        </aside>
      </div>
      <Modal
        className="station-run-modal"
        title={t('station.run.startTitle')}
        open={startModalOpen}
        onCancel={() => setStartModalOpen(false)}
        footer={null}
        destroyOnHidden
      >
        <Form form={startForm} layout="vertical" onFinish={(values) => startRunMutation.mutate(values)}>
          <div className="station-run-form-grid">
            <Form.Item name="project_id" label={t('station.run.device')} rules={[{ required: true }]}>
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
            <Form.Item name="report_template_id" label={t('station.run.reportTemplate')}>
              <Select
                allowClear
                loading={reportTemplatesQuery.isFetching}
                options={reportTemplates.map((template) => ({
                  label: `${template.display_name || template.name || template.template_code} / ${template.template_code}`,
                  value: template.id,
                }))}
              />
            </Form.Item>
            <Form.Item className="station-run-form-wide" name="report_var_ids" label={t('station.run.reportVariables')}>
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
            <Form.Item name="report_ext_1" label={t('station.run.reportExt1')}>
              <Input />
            </Form.Item>
            <Form.Item name="report_ext_2" label={t('station.run.reportExt2')}>
              <Input />
            </Form.Item>
            <Form.Item name="report_ext_3" label={t('station.run.reportExt3')}>
              <Input />
            </Form.Item>
            <Form.Item className="station-run-form-wide" name="operator_note" label={t('station.run.note')}>
              <Input.TextArea rows={3} />
            </Form.Item>
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
}: {
  cards: MetricCard[]
  onOrderChange: (ids: string[]) => void
  t: (key: string) => string
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
          <div className="station-card-grid">
            <SortableContext items={cards.map((card) => card.id)} strategy={rectSortingStrategy}>
              {cards.map((card) => (
                <SortableMetricCard
                  key={card.id}
                  card={card}
                  label={t(card.labelKey)}
                  isDropping={droppingId === card.id}
                />
              ))}
            </SortableContext>
          </div>
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
        {activeCard ? <MetricCardView card={activeCard} label={t(activeCard.labelKey)} dragging /> : null}
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
        <CardChart chartData={chartData} legendName={label} min={card.min} max={card.max} />
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
  min: number
  max: number
}) {
  const dataValues = chartData.map((item) => item.value)
  const dataMin = Math.min(...dataValues)
  const dataMax = Math.max(...dataValues)
  const yMin = Math.min(dataMin, min)
  const yMax = Math.max(dataMax, max)
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
