import { useMemo, useState } from 'react'
import { Alert, Button, ConfigProvider, Empty, Input, Modal, Select, Table, Tag } from 'antd'
import type { TableColumnsType } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import zhCN from 'antd/locale/zh_CN'
import { useTranslation } from 'react-i18next'
import {
  ActivitySquare,
  ArrowDownToLine,
  ArrowUpToLine,
  Calendar,
  Cpu,
  Database,
  Download,
  ListFilter,
  Search,
  Server,
  SlidersHorizontal,
} from 'lucide-react'
import { getDetectionRuns, getDetectionRunStorageRoutes } from '@/features/edge-status/api'
import type { DetectionRun, DetectionRunStorageRoute } from '@/shared/api/types'
import { GanttChartModal } from './components/GanttChartModal'
import { HistoryTable } from './components/HistoryTable'
import { TrendChart } from './components/TrendChart'
import { getHistoryData } from './api'
import { buildTaskLanes, defaultSelectedMetrics, formatHistoryTime, historyItemsToSeries } from './model'
import './history-query.css'

function positiveNumber(value: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function runLabel(run?: DetectionRun) {
  if (!run) return '--'
  return `${run.test_no || `#${run.id}`} · ${run.project_code || run.project_id} · ${run.status}`
}

export function HistoryQueryPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const initialTaskId = positiveNumber(searchParams.get('task_id'))
  const initialProjectId = positiveNumber(searchParams.get('project_id') ?? searchParams.get('device_id'))
  const initialTestNo = searchParams.get('test_no') || ''
  const [selectedTaskId, setSelectedTaskId] = useState<number | undefined>(initialTaskId)
  const [projectIdText, setProjectIdText] = useState(initialProjectId ? String(initialProjectId) : '')
  const [testNo, setTestNo] = useState(initialTestNo)
  const [startText, setStartText] = useState(searchParams.get('start') || '')
  const [endText, setEndText] = useState(searchParams.get('end') || '')
  const [selectedMetrics, setSelectedMetrics] = useState<string[]>([])
  const [isGanttOpen, setGanttOpen] = useState(false)
  const [storageSnapshotOpen, setStorageSnapshotOpen] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)

  const runsQuery = useQuery({
    queryKey: ['history', 'detection-runs'],
    queryFn: () => getDetectionRuns({ limit: 200 }),
    refetchInterval: 30000,
    retry: false,
  })

  const runs = useMemo(() => runsQuery.data?.items ?? [], [runsQuery.data?.items])
  const selectedRun = runs.find((run) => run.id === selectedTaskId)
  const projectId = positiveNumber(projectIdText)

  const historyQuery = useQuery({
    queryKey: ['history', 'data', selectedTaskId, projectId, testNo, startText, endText],
    queryFn: () =>
      getHistoryData({
        task_id: selectedTaskId,
        project_id: selectedTaskId ? undefined : projectId,
        test_no: selectedTaskId ? undefined : testNo || undefined,
        start: startText || undefined,
        end: endText || undefined,
        limit: 5000,
      }),
    refetchInterval: 30000,
    retry: false,
  })

  const storageSnapshotQuery = useQuery({
    queryKey: ['history', 'run-storage-routes', selectedTaskId],
    queryFn: () => getDetectionRunStorageRoutes(selectedTaskId!),
    enabled: storageSnapshotOpen && selectedTaskId !== undefined,
    refetchInterval: storageSnapshotOpen ? 10000 : false,
    retry: false,
  })

  const series = useMemo(() => historyItemsToSeries(historyQuery.data?.items ?? []), [historyQuery.data?.items])
  const numericMetrics = useMemo(() => series.metrics.filter((metric) => metric.isNumeric), [series.metrics])
  const taskLanes = useMemo(() => buildTaskLanes(runs), [runs])
  const activeSelectedMetrics = useMemo(() => {
    const allowed = new Set(numericMetrics.map((metric) => metric.key))
    const kept = selectedMetrics.filter((metric) => allowed.has(metric))
    return kept.length > 0 ? kept : defaultSelectedMetrics(numericMetrics)
  }, [numericMetrics, selectedMetrics])

  const storageRouteColumns = useMemo<TableColumnsType<DetectionRunStorageRoute>>(
    () => [
      {
        title: t('history.storage.route'),
        dataIndex: 'route_code',
        key: 'route_code',
        width: 170,
        render: (value: string, record) => (
          <div className="history-storage-route">
            <strong>{value}</strong>
            <span>var_id: {record.var_id_text ?? record.var_id}</span>
          </div>
        ),
      },
      {
        title: t('history.storage.target'),
        dataIndex: 'storage_target',
        key: 'storage_target',
        width: 132,
        render: (value: string) => <Tag color={value === 'wide_table' ? 'blue' : 'default'}>{value}</Tag>,
      },
      {
        title: t('history.storage.tableColumn'),
        key: 'tableColumn',
        width: 260,
        render: (_, record) => (
          <div className="history-storage-values">
            <span>{record.table_name || '--'}</span>
            <span>{record.column_name || '--'} / {record.column_type || '--'}</span>
          </div>
        ),
      },
      {
        title: t('history.storage.trigger'),
        dataIndex: 'trigger_mode',
        key: 'trigger_mode',
        width: 132,
      },
      {
        title: t('history.storage.cycle'),
        dataIndex: 'cycle_ms',
        key: 'cycle_ms',
        width: 110,
        render: (value: number) => (value > 0 ? `${value} ms` : '--'),
      },
      {
        title: t('history.storage.deadband'),
        dataIndex: 'deadband',
        key: 'deadband',
        width: 110,
        render: (value: number) => String(value ?? 0),
      },
      {
        title: t('history.storage.storeOnStart'),
        dataIndex: 'store_on_start',
        key: 'store_on_start',
        width: 120,
        render: (value: boolean) => <Tag color={value ? 'green' : 'default'}>{value ? t('history.storage.yes') : t('history.storage.no')}</Tag>,
      },
    ],
    [t],
  )

  function applyQueryParams(nextTaskId = selectedTaskId) {
    const next = new URLSearchParams()
    if (nextTaskId) next.set('task_id', String(nextTaskId))
    if (!nextTaskId && projectId) next.set('project_id', String(projectId))
    if (!nextTaskId && testNo.trim()) next.set('test_no', testNo.trim())
    if (startText.trim()) next.set('start', startText.trim())
    if (endText.trim()) next.set('end', endText.trim())
    setSearchParams(next, { replace: true })
  }

  function handleRunSelect(taskId: number) {
    const run = runs.find((item) => item.id === taskId)
    setSelectedTaskId(taskId)
    setProjectIdText(run?.project_id ? String(run.project_id) : projectIdText)
    setTestNo(run?.test_no ?? '')
    setGanttOpen(false)
    applyQueryParams(taskId)
  }

  function clearRunSelection() {
    setSelectedTaskId(undefined)
    applyQueryParams(undefined)
  }

  const chartReady = series.rows.length > 0 && activeSelectedMetrics.length > 0

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#1677ff',
          borderRadius: 8,
          colorBgContainer: 'transparent',
          colorText: '#1a1a1a',
          colorTextHeading: '#000000',
          colorBorderSecondary: 'rgba(0, 0, 0, 0.04)',
          fontFamily: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
        },
        components: {
          Table: {
            headerBg: 'transparent',
            headerColor: '#1a1a1a',
            rowHoverBg: 'rgba(0, 0, 0, 0.02)',
            borderColor: 'rgba(0, 0, 0, 0.04)',
          },
        },
      }}
    >
      <div className="history-page prototype-history-page">
        <div className="history-ambient-background" aria-hidden="true">
          <div className="history-orb history-orb-1" />
          <div className="history-orb history-orb-2" />
          <div className="history-orb history-orb-3" />
          <div className="history-noise" />
        </div>

        <header className="history-toolbar prototype-history-toolbar">
          <div className="history-title-row">
            <h1>{t('history.title')}</h1>
            <div className="history-status-group">
              <button className="glass-btn history-primary-btn" onClick={() => setGanttOpen(true)}>
                <ActivitySquare size={14} />
                {t('history.actions.timeline')}
              </button>
              <div className="history-divider" />
              <Select
                className="history-run-select"
                allowClear
                showSearch
                placeholder={t('history.filters.selectRun')}
                value={selectedTaskId}
                loading={runsQuery.isFetching}
                optionFilterProp="label"
                onChange={(value) => (value ? handleRunSelect(value) : clearRunSelection())}
                options={runs.map((run) => ({ value: run.id, label: runLabel(run) }))}
              />
              <button className="glass-btn">
                <Server size={14} className="history-muted-icon" />
                {selectedRun?.project_code || (projectId ? `Project ${projectId}` : '--')}
              </button>
              <button className="glass-btn">
                <Cpu size={14} className="history-muted-icon" />
                {selectedRun?.test_no || testNo || '--'}
              </button>
              <button className="glass-btn history-time-btn">
                <Calendar size={14} className="history-muted-icon" />
                {selectedRun?.started_at ? `${formatHistoryTime(selectedRun.started_at)} - ${formatHistoryTime(selectedRun.ended_at)}` : `${startText || '--'} - ${endText || '--'}`}
              </button>
              <button className="glass-btn history-accent-btn" onClick={() => {
                applyQueryParams()
                void historyQuery.refetch()
              }}>
                <Search size={14} />
                {t('actions.search')}
              </button>
            </div>
          </div>

          <div className="history-action-row">
            {showAdvanced ? (
              <div className="history-advanced-group">
                <label className="glass-btn history-input-btn">
                  <Server size={14} className="history-muted-icon" />
                  <span>{t('history.filters.projectId')}</span>
                  <input value={projectIdText} disabled={!!selectedTaskId} onChange={(event) => setProjectIdText(event.target.value)} />
                </label>
                <label className="glass-btn history-input-btn">
                  <Cpu size={14} className="history-muted-icon" />
                  <span>{t('history.filters.testNo')}</span>
                  <input value={testNo} disabled={!!selectedTaskId} onChange={(event) => setTestNo(event.target.value)} />
                </label>
                <Input
                  className="history-filter-input"
                  prefix={<ArrowUpToLine size={14} />}
                  placeholder={t('history.filters.start')}
                  value={startText}
                  onChange={(event) => setStartText(event.target.value)}
                />
                <Input
                  className="history-filter-input"
                  prefix={<ArrowDownToLine size={14} />}
                  placeholder={t('history.filters.end')}
                  value={endText}
                  onChange={(event) => setEndText(event.target.value)}
                />
                <Select
                  className="history-metric-select"
                  mode="multiple"
                  maxTagCount="responsive"
                  placeholder={t('history.filters.metrics')}
                  value={activeSelectedMetrics}
                  onChange={setSelectedMetrics}
                  options={numericMetrics.map((metric) => ({ value: metric.key, label: metric.title }))}
                />
                <button className="glass-btn">
                  <SlidersHorizontal size={14} className="history-muted-icon" />
                  {t('history.actions.limitLine')}
                </button>
                <div className="history-divider" />
              </div>
            ) : null}
            <button className={showAdvanced ? 'glass-btn history-active-btn' : 'glass-btn'} onClick={() => setShowAdvanced((value) => !value)}>
              <ListFilter size={14} className="history-muted-icon" />
              {showAdvanced ? t('history.actions.collapse') : t('history.actions.advanced')}
            </button>
            <button
              className="glass-btn"
              disabled={!selectedTaskId}
              onClick={() => setStorageSnapshotOpen(true)}
              title={selectedTaskId ? undefined : t('history.storage.requiresTask')}
            >
              <Database size={14} className="history-muted-icon" />
              {t('history.actions.storageSnapshot')}
            </button>
            <div className="history-divider" />
            <button className="glass-btn">
              <Download size={14} className="history-muted-icon" />
              {t('history.actions.exportImage')}
            </button>
            <button className="glass-btn">
              <Download size={14} className="history-muted-icon" />
              {t('history.actions.exportReport')}
            </button>
          </div>
        </header>

        <main className="history-content">
          {historyQuery.isError || runsQuery.isError ? (
            <Alert
              className="history-api-alert"
              type="warning"
              showIcon
              message={t('history.dataSource.apiUnavailable')}
              action={<Button size="small" onClick={() => {
                void runsQuery.refetch()
                void historyQuery.refetch()
              }}>{t('actions.refresh')}</Button>}
            />
          ) : null}
          <section className="history-glass-panel history-chart-panel">
            <div className="history-chart-note">
              <span>* {t('history.chartNote')}</span>
              <span className="history-data-source live">{t('history.dataSource.api')}</span>
            </div>
            <div className="history-chart-body">
              {chartReady ? (
                <TrendChart data={series.rows} metrics={series.metrics} selectedMetrics={activeSelectedMetrics} />
              ) : (
                <Empty description={historyQuery.isFetching ? t('history.dataSource.loading') : t('history.dataSource.empty')} />
              )}
            </div>
          </section>

          <section className="history-glass-panel history-table-panel">
            <HistoryTable data={series.rows} metrics={series.metrics} loading={historyQuery.isFetching} />
          </section>
        </main>

        <GanttChartModal
          isOpen={isGanttOpen}
          lanes={taskLanes}
          loading={runsQuery.isFetching}
          onClose={() => setGanttOpen(false)}
          onSelect={handleRunSelect}
        />
        <Modal
          className="history-storage-modal"
          title={t('history.storage.title')}
          open={storageSnapshotOpen}
          onCancel={() => setStorageSnapshotOpen(false)}
          footer={null}
          centered
          width="min(1120px, calc(100vw - 48px))"
          destroyOnHidden
        >
          <div className="history-storage-toolbar">
            <span>
              {selectedTaskId
                ? t('history.storage.currentRun', { taskId: selectedTaskId, testNo: selectedRun?.test_no ?? (testNo || '--') })
                : t('history.storage.requiresTask')}
            </span>
            <div className="history-storage-toolbar-right">
              <span>{t('history.storage.count', { count: storageSnapshotQuery.data?.count ?? 0 })}</span>
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
    </ConfigProvider>
  )
}
