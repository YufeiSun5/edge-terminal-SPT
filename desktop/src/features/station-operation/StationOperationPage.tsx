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
import { Button, Form, Input, InputNumber, Modal, Select, message } from 'antd'
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
} from 'lucide-react'
import type { DetectionRunStartPayload, TagSnapshot } from '@/shared/api/types'
import { useAuthStore } from '@/features/auth/authStore'
import {
  abnormalStopDetectionRun,
  getActiveDetectionRuns,
  getDetectionStandards,
  getDevices,
  getRealtimeVariables,
  getReportTemplates,
  startDetectionRun,
  stopDetectionRun,
} from '@/features/edge-status/api'
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
  device_id: number
  test_no: string
  mode: string
  standard_id?: number
  report_template_id?: number
  duration_min?: number
  operator_note?: string
}

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

export function StationOperationPage() {
  const { t, i18n } = useTranslation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [messageApi, messageContext] = message.useMessage()
  const [startForm] = Form.useForm<StartDetectionFormValues>()
  const [startModalOpen, setStartModalOpen] = useState(false)
  const hasPermission = useAuthStore((state) => state.hasPermission)
  const canStartDetection = hasPermission('start_detection')
  const canStopDetection = hasPermission('stop_detection')
  const selectedDeviceId = Number(searchParams.get('device_id'))
  const validSelectedDeviceId = Number.isFinite(selectedDeviceId) && selectedDeviceId > 0 ? selectedDeviceId : undefined
  const variablesQuery = useQuery({
    queryKey: ['edge', 'realtime-variables', validSelectedDeviceId],
    queryFn: () => getRealtimeVariables(validSelectedDeviceId ? { project_id: validSelectedDeviceId } : {}),
    refetchInterval: 2000,
    retry: false,
  })
  const activeRunsQuery = useQuery({
    queryKey: ['edge', 'active-runs'],
    queryFn: getActiveDetectionRuns,
    refetchInterval: 3000,
    retry: false,
  })
  const devicesQuery = useQuery({
    queryKey: ['edge', 'devices'],
    queryFn: getDevices,
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

  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const devices = useMemo(() => devicesQuery.data ?? [], [devicesQuery.data])
  const selectedDevice = useMemo(
    () => devices.find((device) => device.id === validSelectedDeviceId),
    [devices, validSelectedDeviceId],
  )
  const stationVariables = useMemo(
    () =>
      validSelectedDeviceId
        ? variables.filter((variable) => variable.device_id === validSelectedDeviceId)
        : variables,
    [validSelectedDeviceId, variables],
  )
  const activeRun =
    activeRunsQuery.data?.find((run) => run.device_id === validSelectedDeviceId) ?? activeRunsQuery.data?.[0]
  const selectedRunDeviceId = activeRun?.device_id ?? validSelectedDeviceId
  const standards = useMemo(() => standardsQuery.data ?? [], [standardsQuery.data])
  const reportTemplates = useMemo(() => reportTemplatesQuery.data ?? [], [reportTemplatesQuery.data])
  const availableStandards = useMemo(
    () =>
      standards.filter(
        (standard) => !selectedRunDeviceId || !standard.device_id || standard.device_id === selectedRunDeviceId,
      ),
    [selectedRunDeviceId, standards],
  )
  const selectedDeviceName = useMemo(() => {
    if (!selectedDevice) return undefined
    if (i18n.resolvedLanguage === 'en') return selectedDevice.display_name_en || selectedDevice.display_name || selectedDevice.name || selectedDevice.device_code
    if (i18n.resolvedLanguage === 'ja') return selectedDevice.display_name_ja || selectedDevice.display_name || selectedDevice.name || selectedDevice.device_code
    return selectedDevice.display_name || selectedDevice.name || selectedDevice.device_code
  }, [i18n.resolvedLanguage, selectedDevice])
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
        device_id: values.device_id,
        test_no: values.test_no.trim(),
        mode: values.mode,
        standard_id: values.standard_id,
        report_template_id: values.report_template_id,
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
    const targetDevice = selectedDevice ?? devices[0]
    startForm.setFieldsValue({
      device_id: targetDevice?.id,
      test_no: `RUN-${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}`,
      mode: availableStandards[0]?.mode ?? 'standard',
      standard_id: availableStandards[0]?.id,
      report_template_id: reportTemplates[0]?.id,
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
      device_id: String(activeRun.device_id),
      test_no: activeRun.test_no,
    })
    navigate(`/history?${params.toString()}`)
  }

  const statusDevice = selectedDevice?.device_code ?? activeRun?.device_code ?? 'SN-20230912'
  const statusProject = selectedDeviceName ?? activeRun?.test_no ?? t('station.status.mockProject')
  const statusConfig = selectedDevice?.model_name || activeRun?.mode || 'A'
  const statusTask = activeRun?.test_no ?? t('station.run.idle')
  const selectedStandardLabel = activeRun?.standard_code || availableStandards[0]?.standard_code || '--'

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
                  <strong>{statusDevice}</strong>
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
                      <strong>{statusDevice}</strong>
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
            <Button className={alarmOn ? 'alarm-active' : undefined}>{t('station.actions.alarmLog')}</Button>
            {activeRun ? (
              <>
                <Button
                  icon={<History size={15} />}
                  onClick={openHistoryForActiveRun}
                >
                  {t('station.actions.history')}
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
                disabled={!canStartDetection || devices.length === 0}
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
            <Form.Item name="device_id" label={t('station.run.device')} rules={[{ required: true }]}>
              <Select
                options={devices.map((device) => ({
                  label: `${device.display_name || device.name || device.device_code} / ${device.device_code}`,
                  value: device.id,
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
                  label: `${standard.display_name || standard.standard_code} / ${standard.standard_code}`,
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
