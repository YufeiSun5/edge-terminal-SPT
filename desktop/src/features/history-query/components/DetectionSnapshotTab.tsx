import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Alert, Empty, Segmented, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Clock3, Database, FileText, Gauge, History, SlidersHorizontal } from 'lucide-react'
import {
  getDetectionRun,
  getDetectionRunEvents,
  getDetectionRunReportRequests,
  getDetectionRunStorageRoutes,
  getDetectionRunSummary,
} from '@/features/edge-status/api'
import type {
  DetectionRun,
  DetectionRunEvent,
  DetectionRunReportRequest,
  DetectionRunStandardItem,
  DetectionRunStorageRoute,
  DetectionRunSummary,
} from '@/shared/api/types'

type SnapshotView = 'config' | 'storage' | 'reports' | 'events'

type RevisionGroup = {
  revision: number
  effectiveFrom?: string
  effectiveTo?: string
  items: DetectionRunStandardItem[]
}

function compactDate(value?: string) {
  if (!value) return '-'
  return value.replace('T', ' ').replace(/\.\d+.*$/, '')
}

function formatValue(value?: number | null) {
  if (value === undefined || value === null) return '-'
  return Number.isFinite(value) ? String(value) : '-'
}

function limitText(record: DetectionRunStandardItem) {
  return `LL/L ${formatValue(record.limit_ll)} / ${formatValue(record.limit_l)} · H/HH ${formatValue(record.limit_h)} / ${formatValue(record.limit_hh)}`
}

function displayVariable(record: Pick<DetectionRunStandardItem, 'display_name' | 'var_name' | 'var_id' | 'var_id_text'>) {
  return record.display_name || record.var_name || String(record.var_id_text ?? record.var_id)
}

function statusColor(value?: string) {
  switch ((value || '').toLowerCase()) {
    case 'running':
      return 'processing'
    case 'stopped':
    case 'completed':
    case 'ok':
    case 'success':
    case 'succeeded':
      return 'success'
    case 'ng':
    case 'failed':
      return 'error'
    case 'paused':
    case 'waiting':
    case 'pending':
      return 'warning'
    default:
      return 'default'
  }
}

function translatedValue(t: TFunction, baseKey: string, value?: string) {
  const normalized = (value || '').trim()
  if (!normalized) return '-'
  const key = normalized.toLowerCase().replaceAll('.', '_').replaceAll('-', '_').replaceAll(' ', '_')
  return t(`${baseKey}.${key}`, { defaultValue: normalized })
}

function parseJSON(value?: string) {
  if (!value) return null
  try {
    return JSON.parse(value) as unknown
  } catch {
    return value
  }
}

function prettyJSON(value: unknown) {
  if (value === undefined || value === null || value === '') return '-'
  if (typeof value === 'string') return value
  return JSON.stringify(value, null, 2)
}

function buildRevisionGroups(items: DetectionRunStandardItem[]): RevisionGroup[] {
  const byRevision = new Map<number, DetectionRunStandardItem[]>()
  for (const item of items) {
    const revision = item.config_revision || 1
    const group = byRevision.get(revision) ?? []
    group.push(item)
    byRevision.set(revision, group)
  }
  return Array.from(byRevision.entries())
    .map(([revision, groupItems]) => {
      const effectiveFrom = groupItems.map((item) => item.effective_from).filter(Boolean).sort()[0]
      const effectiveTo = groupItems.map((item) => item.effective_to).filter(Boolean).sort().at(-1)
      return { revision, effectiveFrom, effectiveTo, items: groupItems }
    })
    .sort((left, right) => left.revision - right.revision)
}

function SummaryMetric({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode
  label: string
  value: ReactNode
  tone?: string
}) {
  return (
    <div className={`history-snapshot-metric ${tone || ''}`}>
      <span className="history-snapshot-metric-icon">{icon}</span>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function SnapshotSummary({ run, summary }: { run?: DetectionRun; summary?: DetectionRunSummary }) {
  const { t } = useTranslation()
  return (
    <section className="history-snapshot-summary">
      <SummaryMetric
        icon={<Gauge size={16} />}
        label={t('history.detail.snapshot.result')}
        value={<Tag color={statusColor(summary?.result_status || run?.status)}>{translatedValue(t, 'history.detail.alarms.statuses', summary?.result_status || run?.status)}</Tag>}
      />
      <SummaryMetric
        icon={<Clock3 size={16} />}
        label={t('history.detail.snapshot.duration')}
        value={summary ? t('history.detail.snapshot.durationMs', { ms: summary.duration_ms }) : '-'}
      />
      <SummaryMetric
        icon={<History size={16} />}
        label={t('history.detail.snapshot.revisionCount')}
        value={run?.current_config_revision || 1}
        tone={(run?.current_config_revision || 1) > 1 ? 'warning' : undefined}
      />
      <SummaryMetric
        icon={<Database size={16} />}
        label={t('history.detail.snapshot.historyRows')}
        value={summary?.history_rows ?? '-'}
      />
      <SummaryMetric
        icon={<SlidersHorizontal size={16} />}
        label={t('history.detail.snapshot.alarmTotal')}
        value={summary?.alarm_total ?? '-'}
        tone={(summary?.alarm_total ?? 0) > 0 ? 'danger' : undefined}
      />
      <SummaryMetric
        icon={<FileText size={16} />}
        label={t('history.detail.snapshot.reportRequests')}
        value={run?.report_requests?.length ?? '-'}
      />
    </section>
  )
}

export function DetectionSnapshotTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const [view, setView] = useState<SnapshotView>('config')
  const [selectedRevisionNumber, setSelectedRevisionNumber] = useState<number | null>(null)
  const runQuery = useQuery({
    queryKey: ['history', 'run', 'snapshot', taskId],
    queryFn: () => getDetectionRun(taskId),
    enabled: taskId > 0,
    retry: false,
  })
  const summaryQuery = useQuery({
    queryKey: ['history', 'run', 'snapshot-summary', taskId],
    queryFn: () => getDetectionRunSummary(taskId),
    enabled: taskId > 0,
    retry: false,
  })
  const eventsQuery = useQuery({
    queryKey: ['history', 'run', 'snapshot-events', taskId],
    queryFn: () => getDetectionRunEvents(taskId, 300),
    enabled: taskId > 0,
    retry: false,
  })
  const storageRoutesQuery = useQuery({
    queryKey: ['history', 'run', 'snapshot-storage-routes', taskId],
    queryFn: () => getDetectionRunStorageRoutes(taskId),
    enabled: taskId > 0,
    retry: false,
  })
  const reportRequestsQuery = useQuery({
    queryKey: ['history', 'run', 'snapshot-report-requests', taskId],
    queryFn: () => getDetectionRunReportRequests(taskId),
    enabled: taskId > 0,
    retry: false,
  })

  const run = runQuery.data
  const revisions = useMemo(() => buildRevisionGroups(run?.standard_items ?? []), [run?.standard_items])
  const configEvents = useMemo(
    () => (eventsQuery.data?.items ?? []).filter((event) => event.event_type === 'config_applied' || event.event_type === 'limits_updated' || event.event_type === 'config_apply_failed'),
    [eventsQuery.data?.items],
  )
  const latestRevision = revisions.at(-1)
  const selectedRevision =
    revisions.find((revision) => revision.revision === selectedRevisionNumber) ??
    latestRevision ??
    revisions[0]
  const customConfig = parseJSON(run?.custom_config_json)

  const standardColumns: ColumnsType<DetectionRunStandardItem> = [
    {
      title: t('history.detail.snapshot.variable'),
      key: 'variable',
      width: 220,
      render: (_, record) => (
        <div className="history-snapshot-variable">
          <strong>{displayVariable(record)}</strong>
          <span>{record.var_name || record.var_id_text || record.var_id}</span>
        </div>
      ),
    },
    {
      title: t('history.detail.snapshot.limits'),
      key: 'limits',
      width: 260,
      render: (_, record) => <span>{limitText(record)}</span>,
    },
    {
      title: t('history.detail.snapshot.policy'),
      key: 'policy',
      width: 240,
      render: (_, record) => (
        <div className="history-snapshot-stack">
          <span>{record.check_method || '-'}</span>
          <span>{t('history.detail.snapshot.deadband')}: {formatValue(record.limit_deadband)}</span>
          <span>{t('history.detail.snapshot.holdMs')}: {record.violation_hold_ms} / {record.recover_hold_ms}</span>
        </div>
      ),
    },
    {
      title: t('history.detail.snapshot.flags'),
      key: 'flags',
      width: 220,
      render: (_, record) => (
        <Space size={4} wrap>
          <Tag color={record.check_enabled ? 'blue' : 'default'}>{record.check_enabled ? t('history.detail.snapshot.checkOn') : t('history.detail.snapshot.checkOff')}</Tag>
          <Tag color={record.alarm_enabled ? 'orange' : 'default'}>{record.alarm_enabled ? t('history.detail.snapshot.alarmOn') : t('history.detail.snapshot.alarmOff')}</Tag>
          <Tag color={record.store_enabled ? 'green' : 'default'}>{record.store_enabled ? t('history.detail.snapshot.storeOn') : t('history.detail.snapshot.storeOff')}</Tag>
        </Space>
      ),
    },
  ]

  const storageColumns: ColumnsType<DetectionRunStorageRoute> = [
    { title: t('history.detail.snapshot.variable'), dataIndex: 'var_name', key: 'var_name', width: 180, render: (value, record) => value || record.var_id_text || record.var_id },
    { title: t('history.detail.snapshot.table'), dataIndex: 'table_name', key: 'table_name', width: 180 },
    { title: t('history.detail.snapshot.column'), dataIndex: 'column_name', key: 'column_name', width: 160 },
    { title: t('history.detail.snapshot.storageTarget'), dataIndex: 'storage_target', key: 'storage_target', width: 150 },
    { title: t('history.detail.snapshot.triggerMode'), dataIndex: 'trigger_mode', key: 'trigger_mode', width: 150 },
    { title: t('history.detail.snapshot.cycleMs'), dataIndex: 'cycle_ms', key: 'cycle_ms', width: 110 },
  ]

  const reportColumns: ColumnsType<DetectionRunReportRequest> = [
    { title: t('history.detail.snapshot.reportName'), dataIndex: 'report_name', key: 'report_name', width: 180, render: (value) => value || '-' },
    { title: t('history.detail.snapshot.variable'), dataIndex: 'var_name', key: 'var_name', width: 180, render: (value, record) => value || record.var_id_text || record.var_id },
    { title: t('history.detail.snapshot.status'), dataIndex: 'status', key: 'status', width: 120, render: (value) => <Tag color={statusColor(value)}>{value || 'pending'}</Tag> },
    { title: t('history.detail.snapshot.params'), dataIndex: 'params', key: 'params', render: (value) => <code className="history-snapshot-inline-json">{prettyJSON(value)}</code> },
  ]

  const eventColumns: ColumnsType<DetectionRunEvent> = [
    { title: t('history.detail.snapshot.time'), dataIndex: 'occurred_at', key: 'occurred_at', width: 180, render: compactDate },
    { title: t('history.detail.snapshot.eventType'), dataIndex: 'event_type', key: 'event_type', width: 180, render: (value) => translatedValue(t, 'history.detail.alarms.eventTypes', value) },
    { title: t('history.detail.snapshot.message'), dataIndex: 'message', key: 'message', width: 240, render: (value) => value || '-' },
    { title: t('history.detail.snapshot.detail'), dataIndex: 'detail', key: 'detail', render: (value) => <code className="history-snapshot-inline-json">{prettyJSON(parseJSON(value))}</code> },
  ]

  if (runQuery.isError) {
    return (
      <div className="history-tab-content history-snapshot-tab">
        <Empty description={t('history.detail.snapshot.empty')} />
      </div>
    )
  }

  return (
    <div className="history-tab-content history-snapshot-tab">
      <div className="history-snapshot-header">
        <div>
          <Typography.Title level={4}>{t('history.detail.snapshot.title')}</Typography.Title>
          <Typography.Text type="secondary">
            {run ? `${run.test_no || '-'} · ${run.project_code || '-'} · ${compactDate(run.started_at)}` : t('history.dataSource.loading')}
          </Typography.Text>
        </div>
        <Segmented
          value={view}
          onChange={(value) => setView(value as SnapshotView)}
          options={[
            { value: 'config', label: t('history.detail.snapshot.views.config') },
            { value: 'storage', label: t('history.detail.snapshot.views.storage') },
            { value: 'reports', label: t('history.detail.snapshot.views.reports') },
            { value: 'events', label: t('history.detail.snapshot.views.events') },
          ]}
        />
      </div>

      <SnapshotSummary run={run} summary={summaryQuery.data} />

      {(run?.current_config_revision || 1) > 1 || configEvents.length > 0 ? (
        <Alert
          className="history-snapshot-alert"
          type="warning"
          showIcon
          message={t('history.detail.snapshot.revisionNotice')}
        />
      ) : null}

      {view === 'config' ? (
        <section className="history-snapshot-layout">
          <aside className="history-snapshot-revision-list">
            <Typography.Title level={5}>{t('history.detail.snapshot.revisions')}</Typography.Title>
            {revisions.length > 0 ? revisions.map((revision) => (
              <button
                key={revision.revision}
                type="button"
                className={revision.revision === selectedRevision?.revision ? 'history-snapshot-revision active' : 'history-snapshot-revision'}
                onClick={() => setSelectedRevisionNumber(revision.revision)}
              >
                <strong>{t('history.detail.snapshot.revision', { revision: revision.revision })}</strong>
                <span>{compactDate(revision.effectiveFrom || run?.started_at)} - {compactDate(revision.effectiveTo || run?.ended_at)}</span>
                <small>{t('history.detail.snapshot.itemCount', { count: revision.items.length })}</small>
              </button>
            )) : <Empty description={t('history.detail.snapshot.noStandardItems')} />}
            <div className="history-snapshot-json-block">
              <span>{t('history.detail.snapshot.startParams')}</span>
              <pre>{prettyJSON(customConfig)}</pre>
            </div>
          </aside>
          <section className="history-snapshot-main">
            <Table<DetectionRunStandardItem>
              rowKey={(record) => `${record.id}-${record.var_id_text ?? record.var_id}`}
              size="small"
              columns={standardColumns}
              dataSource={selectedRevision?.items ?? []}
              loading={runQuery.isFetching}
              pagination={{ pageSize: 12, showSizeChanger: false }}
              scroll={{ x: 940 }}
            />
          </section>
        </section>
      ) : null}

      {view === 'storage' ? (
        <Table<DetectionRunStorageRoute>
          rowKey={(record) => record.id}
          className="history-snapshot-table"
          size="small"
          columns={storageColumns}
          dataSource={storageRoutesQuery.data?.items ?? run?.storage_routes ?? []}
          loading={storageRoutesQuery.isFetching || runQuery.isFetching}
          pagination={{ pageSize: 16, showSizeChanger: false }}
          scroll={{ x: 900 }}
        />
      ) : null}

      {view === 'reports' ? (
        <Table<DetectionRunReportRequest>
          rowKey={(record) => record.id}
          className="history-snapshot-table"
          size="small"
          columns={reportColumns}
          dataSource={reportRequestsQuery.data?.items ?? run?.report_requests ?? []}
          loading={reportRequestsQuery.isFetching || runQuery.isFetching}
          pagination={{ pageSize: 16, showSizeChanger: false }}
          scroll={{ x: 900 }}
        />
      ) : null}

      {view === 'events' ? (
        <Table<DetectionRunEvent>
          rowKey={(record) => record.id}
          className="history-snapshot-table"
          size="small"
          columns={eventColumns}
          dataSource={eventsQuery.data?.items ?? []}
          loading={eventsQuery.isFetching}
          pagination={{ pageSize: 16, showSizeChanger: false }}
          scroll={{ x: 920 }}
        />
      ) : null}
    </div>
  )
}
