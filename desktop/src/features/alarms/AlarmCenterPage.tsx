import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, DatePicker, Descriptions, Empty, Input, Modal, Select, Table, Tag } from 'antd'
import type { TableColumnsType, TablePaginationConfig } from 'antd'
import dayjs from 'dayjs'
import type { Dayjs } from 'dayjs'
import { useNavigate, useSearchParams } from 'react-router'
import { ExternalLink, RefreshCw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getDetectionRun, getDevices, getLimitAlarms } from '@/features/edge-status/api'
import type { Device, LimitAlarm, LimitAlarmListParams, LimitAlarmScope } from '@/shared/api/types'
import '../notifications/notification-center.css'

type AlarmFilters = {
  scope?: LimitAlarmScope
  project_id?: number
  task_id?: number
  var_id?: string
  status?: string
  alarm_level?: string
  from?: string
  to?: string
}

const alarmScopeOptions: Array<{ value: LimitAlarmScope; labelKey: string }> = [
  { value: 'default', labelKey: 'alarmCenter.scopes.default' },
  { value: 'detection', labelKey: 'alarmCenter.scopes.detection' },
]

const alarmStatusOptions = ['active', 'recovered', 'closed']
const alarmLevelOptions = ['LL', 'L', 'H', 'HH']

export function AlarmCenterPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = useMemo(() => readFilters(searchParams), [searchParams])
  const [keyword, setKeyword] = useState(searchParams.get('keyword') ?? '')
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20 })
  const [selectedTaskId, setSelectedTaskId] = useState<number>()

  const projectsQuery = useQuery({
    queryKey: ['alarm-center', 'projects'],
    queryFn: getDevices,
    staleTime: 30000,
    retry: false,
  })

  const queryParams = useMemo<LimitAlarmListParams>(
    () => ({
      ...filters,
      limit: pagination.pageSize,
      offset: (pagination.current - 1) * pagination.pageSize,
    }),
    [filters, pagination],
  )

  const alarmsQuery = useQuery({
    queryKey: ['alarm-center', 'items', queryParams],
    queryFn: () => getLimitAlarms(queryParams),
    refetchInterval: 10000,
    retry: false,
  })

  const taskDetailQuery = useQuery({
    queryKey: ['alarm-center', 'task-detail', selectedTaskId],
    queryFn: () => getDetectionRun(selectedTaskId!),
    enabled: selectedTaskId !== undefined,
    retry: false,
  })

  const timeRangeValue = useMemo<[Dayjs, Dayjs] | null>(() => {
    if (!filters.from || !filters.to) return null
    const from = dayjs(filters.from)
    const to = dayjs(filters.to)
    if (!from.isValid() || !to.isValid()) return null
    return [from, to]
  }, [filters.from, filters.to])

  function updateFilters(next: Partial<AlarmFilters>, nextKeyword = keyword) {
    const merged = { ...filters, ...next }
    setPagination((value) => ({ ...value, current: 1 }))
    const params = new URLSearchParams()
    if (merged.scope) params.set('scope', merged.scope)
    if (merged.project_id) params.set('project_id', String(merged.project_id))
    if (merged.task_id) params.set('task_id', String(merged.task_id))
    if (merged.var_id) params.set('var_id', String(merged.var_id))
    if (merged.status) params.set('status', merged.status)
    if (merged.alarm_level) params.set('alarm_level', merged.alarm_level)
    if (merged.from) params.set('from', merged.from)
    if (merged.to) params.set('to', merged.to)
    if (nextKeyword) params.set('keyword', nextKeyword)
    setSearchParams(params, { replace: true })
  }

  function displayProject(project?: Pick<Device, 'device_code' | 'project_code' | 'name' | 'display_name' | 'display_name_en' | 'display_name_ja'>) {
    if (!project) return ''
    if (i18n.resolvedLanguage === 'en') return project.display_name_en || project.display_name || project.name || project.project_code || project.device_code
    if (i18n.resolvedLanguage === 'ja') return project.display_name_ja || project.display_name || project.name || project.project_code || project.device_code
    return project.display_name || project.name || project.project_code || project.device_code
  }

  function alarmName(alarm: LimitAlarm) {
    if (i18n.resolvedLanguage === 'en') return alarm.display_name_en || alarm.display_name || alarm.var_name
    if (i18n.resolvedLanguage === 'ja') return alarm.display_name_ja || alarm.display_name || alarm.var_name
    return alarm.display_name || alarm.var_name
  }

  function openHistory(record: LimitAlarm) {
    const params = new URLSearchParams()
    if (record.task_id) params.set('task_id', String(record.task_id))
    if (record.project_id) params.set('project_id', String(record.project_id))
    if (record.test_no) params.set('test_no', record.test_no)
    navigate(`/history?${params.toString()}`)
  }

  function openStation(record: LimitAlarm) {
    const params = new URLSearchParams()
    if (record.project_id) params.set('project_id', String(record.project_id))
    navigate({ pathname: '/', search: params.toString() })
  }

  const columns: TableColumnsType<LimitAlarm> = [
      {
        title: t('alarmCenter.columns.scope'),
        dataIndex: 'scope',
        key: 'scope',
        width: 116,
        render: (value: string) => <Tag color={value === 'default' ? 'cyan' : 'blue'}>{value === 'default' ? t('alarmCenter.scopes.default') : t('alarmCenter.scopes.detection')}</Tag>,
      },
      {
        title: t('alarmCenter.columns.variable'),
        key: 'variable',
        width: 220,
        render: (_, record) => (
          <div className="ops-cell-stack">
            <strong>{alarmName(record)}</strong>
            <span>{record.var_name}</span>
          </div>
        ),
      },
      {
        title: t('alarmCenter.columns.level'),
        dataIndex: 'alarm_level',
        key: 'alarm_level',
        width: 86,
        render: (value: string) => <Tag color={value === 'HH' || value === 'LL' ? 'red' : 'gold'}>{value}</Tag>,
      },
      {
        title: t('alarmCenter.columns.status'),
        dataIndex: 'status',
        key: 'status',
        width: 104,
        render: (value: string) => <span className={value === 'active' ? 'ops-status active' : 'ops-status recovered'}>{value === 'active' ? t('alarmCenter.status.active') : t('alarmCenter.status.recovered')}</span>,
      },
      {
        title: t('alarmCenter.columns.values'),
        key: 'values',
        width: 190,
        render: (_, record) => (
          <div className="ops-cell-stack">
            <span>{t('alarmCenter.values.start')}: {formatValue(record.start_value)}</span>
            <span>{t('alarmCenter.values.limit')}: {formatValue(record.limit_value)}</span>
            <span>{t('alarmCenter.values.recover')}: {formatValue(record.recover_value)}</span>
          </div>
        ),
      },
      {
        title: t('alarmCenter.columns.project'),
        dataIndex: 'project_code',
        key: 'project',
        width: 130,
        render: (value: string, record) => value || record.project_id || '-',
      },
      {
        title: t('alarmCenter.columns.time'),
        dataIndex: 'first_seen_at',
        key: 'first_seen_at',
        width: 190,
        render: formatDate,
      },
      {
        title: t('alarmCenter.columns.actions'),
        key: 'actions',
        width: 190,
        fixed: 'right',
        render: (_, record) => (
          <div className="ops-row-actions">
            <Button size="small" icon={<ExternalLink size={13} />} onClick={() => openStation(record)}>
              {t('alarmCenter.actions.station')}
            </Button>
            {record.task_id ? (
              <>
                <Button size="small" onClick={() => setSelectedTaskId(record.task_id)}>
                  {t('alarmCenter.actions.taskDetail')}
                </Button>
                <Button size="small" onClick={() => openHistory(record)}>
                  {t('alarmCenter.actions.history')}
                </Button>
              </>
            ) : null}
          </div>
        ),
      },
    ]

  function handleTableChange(next: TablePaginationConfig) {
    setPagination({
      current: next.current ?? 1,
      pageSize: next.pageSize ?? pagination.pageSize,
    })
  }

  return (
    <div className="ops-center-page alarm-center-page">
      <div className="ops-center-bg" aria-hidden="true" />
      <section className="ops-center-header glass-panel">
        <div>
          <span className="ops-center-eyebrow">{t('alarmCenter.eyebrow')}</span>
          <h2>{t('alarmCenter.title')}</h2>
          <p>{t('alarmCenter.desc')}</p>
        </div>
        <div className="ops-center-summary">
          <strong>{alarmsQuery.data?.total ?? 0}</strong>
          <span>{t('alarmCenter.total')}</span>
        </div>
      </section>

      <section className="ops-center-panel glass-panel">
        <div className="ops-center-toolbar">
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('alarmCenter.filters.scope')}
            value={filters.scope}
            options={alarmScopeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }))}
            onChange={(value) => updateFilters({ scope: value })}
          />
          <Select
            allowClear
            showSearch
            className="ops-center-filter"
            optionFilterProp="label"
            placeholder={t('alarmCenter.filters.project')}
            value={filters.project_id}
            options={(projectsQuery.data ?? []).map((project) => ({ value: project.id, label: displayProject(project) }))}
            onChange={(value) => updateFilters({ project_id: value })}
          />
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('alarmCenter.filters.status')}
            value={filters.status}
            options={alarmStatusOptions.map((value) => ({ value, label: t(`alarmCenter.status.${value === 'closed' ? 'recovered' : value}`) }))}
            onChange={(value) => updateFilters({ status: value })}
          />
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('alarmCenter.filters.level')}
            value={filters.alarm_level}
            options={alarmLevelOptions.map((value) => ({ value, label: value }))}
            onChange={(value) => updateFilters({ alarm_level: value })}
          />
          <Input
            allowClear
            className="ops-center-search"
            prefix={<Search size={14} />}
            placeholder={t('alarmCenter.filters.varId')}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            onPressEnter={() => updateFilters({ var_id: keyword || undefined }, keyword)}
          />
          <DatePicker.RangePicker
            allowClear
            showTime
            className="ops-center-date-range"
            value={timeRangeValue}
            placeholder={[t('alarmCenter.filters.from'), t('alarmCenter.filters.to')]}
            onChange={(value) => {
              updateFilters({
                from: value?.[0]?.toISOString(),
                to: value?.[1]?.toISOString(),
              })
            }}
          />
          <div className="ops-center-actions">
            <Button onClick={() => updateFilters({ var_id: keyword || undefined }, keyword)}>{t('actions.search')}</Button>
            <Button icon={<RefreshCw size={14} />} onClick={() => alarmsQuery.refetch()} loading={alarmsQuery.isFetching}>
              {t('actions.refresh')}
            </Button>
          </div>
        </div>

        <Table<LimitAlarm>
          rowKey="id"
          className="ops-center-table"
          columns={columns}
          dataSource={alarmsQuery.data?.items ?? []}
          loading={alarmsQuery.isFetching}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('alarmCenter.empty')} /> }}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: alarmsQuery.data?.total ?? 0,
            showSizeChanger: true,
            pageSizeOptions: [20, 50, 100],
          }}
          scroll={{ x: 1220, y: 'calc(100vh - 370px)' }}
          onChange={handleTableChange}
        />
      </section>

      <Modal
        width={760}
        title={t('alarmCenter.taskDetail.title')}
        open={selectedTaskId !== undefined}
        footer={[
          <Button key="close" onClick={() => setSelectedTaskId(undefined)}>
            {t('actions.close')}
          </Button>,
          <Button
            key="history"
            type="primary"
            disabled={!taskDetailQuery.data}
            onClick={() => {
              const task = taskDetailQuery.data
              if (!task) return
              const params = new URLSearchParams()
              params.set('task_id', String(task.id))
              if (task.project_id) params.set('project_id', String(task.project_id))
              if (task.test_no) params.set('test_no', task.test_no)
              navigate(`/history?${params.toString()}`)
            }}
          >
            {t('alarmCenter.actions.history')}
          </Button>,
        ]}
        onCancel={() => setSelectedTaskId(undefined)}
      >
        <Descriptions
          bordered
          size="small"
          column={2}
          items={[
            { key: 'task_id', label: 'task_id', children: taskDetailQuery.data?.id ?? selectedTaskId ?? '-' },
            { key: 'test_no', label: t('alarmCenter.taskDetail.testNo'), children: taskDetailQuery.data?.test_no ?? '-' },
            { key: 'project', label: t('alarmCenter.taskDetail.project'), children: taskDetailQuery.data?.project_code ?? taskDetailQuery.data?.project_id ?? '-' },
            { key: 'status', label: t('alarmCenter.taskDetail.status'), children: taskDetailQuery.data?.status ?? '-' },
            { key: 'standard', label: t('alarmCenter.taskDetail.standard'), children: taskDetailQuery.data?.standard_code || '-' },
            { key: 'report', label: t('alarmCenter.taskDetail.report'), children: taskDetailQuery.data?.report_template_code || '-' },
            { key: 'started_at', label: t('alarmCenter.taskDetail.startedAt'), children: formatDate(taskDetailQuery.data?.started_at) },
            { key: 'ended_at', label: t('alarmCenter.taskDetail.endedAt'), children: formatDate(taskDetailQuery.data?.ended_at) },
            { key: 'duration', label: t('alarmCenter.taskDetail.duration'), children: `${taskDetailQuery.data?.duration_sec ?? 0}s` },
            { key: 'end_type', label: t('alarmCenter.taskDetail.endType'), children: taskDetailQuery.data?.end_type || '-' },
            { key: 'note', label: t('alarmCenter.taskDetail.note'), children: taskDetailQuery.data?.operator_note || '-' },
          ]}
        />
      </Modal>
    </div>
  )
}

function readFilters(searchParams: URLSearchParams): AlarmFilters {
  const projectId = Number(searchParams.get('project_id'))
  const taskId = Number(searchParams.get('task_id'))
  return {
    scope: (searchParams.get('scope') || undefined) as LimitAlarmScope | undefined,
    project_id: Number.isFinite(projectId) && projectId > 0 ? projectId : undefined,
    task_id: Number.isFinite(taskId) && taskId > 0 ? taskId : undefined,
    var_id: searchParams.get('var_id') || undefined,
    status: searchParams.get('status') || undefined,
    alarm_level: searchParams.get('alarm_level') || undefined,
    from: searchParams.get('from') || undefined,
    to: searchParams.get('to') || undefined,
  }
}

function formatValue(value?: number | null) {
  if (value === undefined || value === null) return '-'
  return Number(value).toFixed(3).replace(/\.?0+$/, '')
}

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
