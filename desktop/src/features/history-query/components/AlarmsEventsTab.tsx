import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Empty, Space, Table, Tag, Timeline, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AlertTriangle, Clock } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { getDetectionRunEvents, getLimitAlarms } from '@/features/edge-status/api'
import type { DetectionRunEvent, LimitAlarm } from '@/shared/api/types'

type EventListRow = {
  key: string
  kind: 'event' | 'alarm'
  time: string
  title: string
  status?: string
}

function compactDate(value?: string) {
  if (!value) return '-'
  return value.replace('T', ' ').replace(/\.\d+.*$/, '')
}

function statusColor(status?: string) {
  if (!status) return 'default'
  if (status === 'active') return 'red'
  if (status === 'recovered') return 'green'
  return 'default'
}

function translatedValue(t: TFunction, baseKey: string, value?: string) {
  const normalized = (value || '').trim()
  if (!normalized) return '-'
  const key = normalized.toLowerCase().replaceAll('.', '_').replaceAll('-', '_').replaceAll(' ', '_')
  const translated = t(`${baseKey}.${key}`, { defaultValue: normalized })
  return translated || normalized
}

function eventTitle(event: DetectionRunEvent, t: TFunction) {
  return translatedValue(t, 'history.detail.alarms.eventTypes', event.event_type)
}

function timelineColor(row: EventListRow) {
  if (row.kind === 'alarm') return row.status === 'recovered' ? 'green' : 'red'
  return 'blue'
}

function timelineDot(row: EventListRow) {
  if (row.kind === 'alarm') return <AlertTriangle size={15} />
  return <Clock size={15} />
}

function alarmTitle(alarm: LimitAlarm) {
  return alarm.display_name || alarm.var_name || String(alarm.var_id_text ?? alarm.var_id)
}

function buildRows(events: DetectionRunEvent[], alarms: LimitAlarm[], t: TFunction): EventListRow[] {
  const eventRows = events.map((event) => ({
    key: `event-${event.id}`,
    kind: 'event' as const,
    time: event.occurred_at || event.created_at,
    title: eventTitle(event, t),
  }))

  const alarmRows = alarms.map((alarm) => ({
    key: `alarm-${alarm.id}`,
    kind: 'alarm' as const,
    time: alarm.first_seen_at || alarm.created_at,
    title: alarm.message || alarmTitle(alarm),
    status: alarm.status,
  }))

  return [...eventRows, ...alarmRows].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime())
}

export function AlarmsEventsTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const eventsQuery = useQuery({
    queryKey: ['history', 'run', 'events', taskId],
    queryFn: () => getDetectionRunEvents(taskId, 300),
    retry: false,
  })

  const alarmsQuery = useQuery({
    queryKey: ['history', 'run', 'limit-alarms', taskId],
    queryFn: () => getLimitAlarms({ scope: 'detection', task_id: taskId, limit: 300 }),
    retry: false,
  })

  const rows = useMemo(
    () => buildRows(eventsQuery.data?.items ?? [], alarmsQuery.data?.items ?? [], t),
    [alarmsQuery.data?.items, eventsQuery.data?.items, t],
  )

  const columns: ColumnsType<EventListRow> = [
    {
      title: t('history.detail.alarms.type'),
      dataIndex: 'kind',
      width: 90,
      render: (kind: EventListRow['kind']) => (
        <Tag color={kind === 'alarm' ? 'red' : 'blue'}>
          {kind === 'alarm' ? t('history.detail.alarms.alarm') : t('history.detail.alarms.event')}
        </Tag>
      ),
    },
    {
      title: t('history.detail.alarms.time'),
      dataIndex: 'time',
      width: 180,
      render: (value: string) => compactDate(value),
    },
    {
      title: t('history.detail.alarms.content'),
      dataIndex: 'title',
      render: (value: string) => <Typography.Text>{value || '-'}</Typography.Text>,
    },
    {
      title: t('history.detail.alarms.status'),
      dataIndex: 'status',
      width: 100,
      render: (status?: string) => status ? <Tag color={statusColor(status)}>{translatedValue(t, 'history.detail.alarms.statuses', status)}</Tag> : '-',
    },
  ]

  return (
    <div className="history-tab-content history-alarms-events-layout">
      <aside className="history-events-timeline-panel">
        <div className="history-panel-header">
          <Typography.Title level={4}>{t('history.detail.alarms.timeline')}</Typography.Title>
        </div>
        <div className="history-panel-body history-timeline-container">
          {rows.length > 0 ? (
            <Timeline
              items={rows.map((row) => ({
                color: timelineColor(row),
                icon: timelineDot(row),
                content: (
                  <Space orientation="vertical" size={0}>
                    <Typography.Text>{row.title}</Typography.Text>
                    <Typography.Text type="secondary">{compactDate(row.time)}</Typography.Text>
                  </Space>
                ),
              }))}
            />
          ) : (
            <Empty description={t('history.detail.alarms.empty')} />
          )}
        </div>
      </aside>

      <section className="history-events-list-panel">
        <div className="history-panel-header">
          <Typography.Title level={4}>{t('history.detail.alarms.eventList')}</Typography.Title>
        </div>
        <div className="history-panel-body">
          <Table<EventListRow>
            loading={eventsQuery.isFetching || alarmsQuery.isFetching}
            dataSource={rows}
            columns={columns}
            rowKey="key"
            pagination={{ pageSize: 50, showSizeChanger: false }}
            size="small"
            locale={{ emptyText: t('history.detail.alarms.empty') }}
          />
        </div>
      </section>
    </div>
  )
}
