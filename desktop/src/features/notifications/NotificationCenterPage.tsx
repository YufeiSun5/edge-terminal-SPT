import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Badge, Button, DatePicker, Empty, Input, Select, Table, Tag, message } from 'antd'
import type { TableColumnsType, TablePaginationConfig } from 'antd'
import dayjs from 'dayjs'
import type { Dayjs } from 'dayjs'
import { useNavigate } from 'react-router'
import { CheckCheck, RefreshCw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  getProjects,
  markMainReportNotificationRead,
  markNotificationRead,
} from '@/features/edge-status/api'
import { subscribeRealtimeWebSocket } from '@/features/realtime/realtimeClient'
import {
  canIncludeReportNotifications,
  emptyNotificationList,
  getVisibleBaseNotifications,
  getVisibleBaseUnreadCount,
  getVisibleReportNotifications,
  getVisibleReportUnreadCount,
  markVisibleBaseNotificationsRead,
  markVisibleReportNotificationsRead,
  notificationTypeOptions,
  reportNotificationType,
  sortNotifications,
} from '@/features/notifications/notificationPolicy'
import type { NotificationListParams, Project, UserNotification } from '@/shared/api/types'
import { queryClient } from '@/app/queryClient'
import { languageCode } from '@/shared/i18n/language'
import './notification-center.css'

const levelOptions = ['info', 'success', 'warning', 'error']

type NotificationFilters = {
  unread?: boolean
  type?: string
  level?: string
  project_id?: number
  keyword?: string
  from?: string
  to?: string
}

export function NotificationCenterPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [messageApi, contextHolder] = message.useMessage()
  const [filters, setFilters] = useState<NotificationFilters>({})
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20 })

  const projectsQuery = useQuery({
    queryKey: ['notification-center', 'projects'],
    queryFn: getProjects,
    staleTime: 30000,
    retry: false,
  })

  const queryParams = useMemo<NotificationListParams>(
    () => ({
      ...filters,
      limit: pagination.pageSize,
      offset: (pagination.current - 1) * pagination.pageSize,
    }),
    [filters, pagination],
  )

  const notificationsQuery = useQuery({
    queryKey: ['notification-center', 'items', queryParams],
    queryFn: async () => {
      const limit = queryParams.limit ?? 20
      const offset = queryParams.offset ?? 0
      const includeReports = canIncludeReportNotifications(filters)
      if (filters.type === reportNotificationType) {
        if (!includeReports) return emptyNotificationList(limit, offset)
        return getVisibleReportNotifications({
          unread: filters.unread,
          level: filters.level,
          limit,
          offset,
        }, t)
      }
      if (!includeReports) return getVisibleBaseNotifications(queryParams)

      const fetchLimit = offset + limit
      const [base, report] = await Promise.all([
        getVisibleBaseNotifications({ ...queryParams, limit: fetchLimit, offset: 0 }),
        getVisibleReportNotifications({ unread: filters.unread, level: filters.level, limit: fetchLimit, offset: 0 }, t),
      ])
      const merged = sortNotifications([...base.items, ...report.items]).slice(offset, offset + limit)
      return { items: merged, total: base.total + report.total, limit, offset }
    },
    refetchInterval: 10000,
    retry: false,
  })

  const unreadQuery = useQuery({
    queryKey: ['notification-center', 'unread', filters],
    queryFn: async () => {
      const includeReports = canIncludeReportNotifications(filters)
      if (filters.type === reportNotificationType) {
        if (!includeReports) return { unread: 0 }
        return getVisibleReportUnreadCount({ unread: filters.unread, level: filters.level })
      }
      const base = await getVisibleBaseUnreadCount(filters)
      if (!includeReports) return base
      const report = await getVisibleReportUnreadCount({ unread: filters.unread, level: filters.level })
      return { unread: base.unread + report.unread }
    },
    refetchInterval: 10000,
    retry: false,
  })

  const timeRangeValue = useMemo<[Dayjs, Dayjs] | null>(() => {
    if (!filters.from || !filters.to) return null
    const from = dayjs(filters.from)
    const to = dayjs(filters.to)
    if (!from.isValid() || !to.isValid()) return null
    return [from, to]
  }, [filters.from, filters.to])

  const markReadMutation = useMutation({
    mutationFn: (id: number) => id < 0 ? markMainReportNotificationRead(Math.abs(id)) : markNotificationRead(id),
    onSuccess: async () => {
      await invalidateNotifications()
    },
  })

  const markAllMutation = useMutation({
    mutationFn: async () => {
      const includeReports = canIncludeReportNotifications(filters)
      if (filters.type === reportNotificationType) {
        if (!includeReports) return { updated: 0 }
        return markVisibleReportNotificationsRead({ unread: filters.unread, level: filters.level })
      }
      const base = await markVisibleBaseNotificationsRead(filters)
      if (!includeReports) return base
      const report = await markVisibleReportNotificationsRead({ unread: filters.unread, level: filters.level })
      return { updated: base.updated + report.updated }
    },
    onSuccess: async (response) => {
      messageApi.success(t('notifications.center.readAllDone', { count: response.updated }))
      await invalidateNotifications()
    },
  })

  useEffect(() => {
    return subscribeRealtimeWebSocket({
      subscription: { topics: ['notifications'] },
      onMessage: (message) => {
        if (message.type !== 'notification.event') return
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ['notification-center'] }),
          queryClient.invalidateQueries({ queryKey: ['shell', 'notifications'] }),
        ])
      },
    })
  }, [])

  async function invalidateNotifications() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['notification-center'] }),
      queryClient.invalidateQueries({ queryKey: ['shell', 'notifications'] }),
    ])
  }

  function updateFilters(next: Partial<NotificationFilters>) {
    setPagination((value) => ({ ...value, current: 1 }))
    setFilters((value) => ({ ...value, ...next }))
  }

  function notificationTitle(notification: UserNotification) {
    return notification.display_name || notification.var_name || notification.test_no || notification.type
  }

  function displayProject(project?: Pick<Project, 'project_code' | 'name' | 'display_name' | 'display_name_en' | 'display_name_ja'>) {
    if (!project) return ''
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return project.display_name_en || project.project_code
    if (currentLanguage === 'ja') return project.display_name_ja || project.project_code
    return project.display_name || project.name || project.project_code
  }

  function openNotification(notification: UserNotification) {
    if (!notification.read_at) markReadMutation.mutate(notification.id)
    const params = new URLSearchParams()
    if (notification.project_id) params.set('project_id', String(notification.project_id))
    if (notification.task_id) params.set('task_id', String(notification.task_id))
    if (notification.var_id_text ?? notification.var_id) params.set('var_id', String(notification.var_id_text ?? notification.var_id))

    if (notification.type.startsWith('alarm.')) {
      if (notification.task_id) params.set('scope', 'detection')
      params.set('status', 'active')
      navigate(`/alarms?${params.toString()}`)
      return
    }

    if (notification.type === reportNotificationType) {
      if (notification.task_id) {
        navigate(`/history/runs/${notification.task_id}?tab=reports`)
      } else {
        navigate('/history')
      }
      return
    }

    if (notification.task_id) {
      navigate(`/history?${params.toString()}`)
      return
    }

    navigate({ pathname: '/', search: params.toString() })
  }

  const columns: TableColumnsType<UserNotification> = [
      {
        title: t('notifications.center.columns.status'),
        key: 'status',
        width: 96,
        render: (_, record) => (
          <Badge status={record.read_at ? 'default' : 'processing'} text={record.read_at ? t('notifications.center.read') : t('notifications.center.unread')} />
        ),
      },
      {
        title: t('notifications.center.columns.type'),
        dataIndex: 'type',
        key: 'type',
        width: 180,
        render: (value: string, record) => <Tag color={levelColor(record.level)}>{typeLabel(value, t)}</Tag>,
      },
      {
        title: t('notifications.center.columns.message'),
        key: 'message',
        render: (_, record) => (
          <button className="center-link-cell" onClick={() => openNotification(record)}>
            <strong>{notificationTitle(record)}</strong>
            <span>{record.message || record.type}</span>
          </button>
        ),
      },
      {
        title: t('notifications.center.columns.project'),
        dataIndex: 'project_code',
        key: 'project',
        width: 140,
        render: (value: string, record) => value || record.project_id || '-',
      },
      {
        title: t('notifications.center.columns.time'),
        dataIndex: 'occurred_at',
        key: 'occurred_at',
        width: 190,
        render: (value: string, record) => formatDate(value || record.created_at),
      },
      {
        title: t('notifications.center.columns.actions'),
        key: 'actions',
        width: 116,
        render: (_, record) => (
          <Button size="small" disabled={Boolean(record.read_at)} onClick={() => markReadMutation.mutate(record.id)}>
            {t('notifications.center.markRead')}
          </Button>
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
    <div className="ops-center-page notification-center-page">
      {contextHolder}
      <div className="ops-center-bg" aria-hidden="true" />
      <section className="ops-center-header glass-panel">
        <div>
          <span className="ops-center-eyebrow">{t('notifications.center.eyebrow')}</span>
          <h2>{t('notifications.center.title')}</h2>
          <p>{t('notifications.center.desc')}</p>
        </div>
        <div className="ops-center-summary">
          <strong>{unreadQuery.data?.unread ?? 0}</strong>
          <span>{t('notifications.center.unreadCount')}</span>
        </div>
      </section>

      <section className="ops-center-panel glass-panel">
        <div className="ops-center-toolbar">
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('notifications.filters.all')}
            value={filters.unread === undefined ? undefined : String(filters.unread)}
            options={[
              { value: 'true', label: t('notifications.filters.unread') },
              { value: 'false', label: t('notifications.center.read') },
            ]}
            onChange={(value) => updateFilters({ unread: value === undefined ? undefined : value === 'true' })}
          />
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('notifications.filters.type')}
            value={filters.type}
            options={notificationTypeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }))}
            onChange={(value) => updateFilters({ type: value })}
          />
          <Select
            allowClear
            className="ops-center-filter"
            placeholder={t('notifications.center.level')}
            value={filters.level}
            options={levelOptions.map((value) => ({ value, label: t(`notifications.levels.${value}`) }))}
            onChange={(value) => updateFilters({ level: value })}
          />
          <Select
            allowClear
            showSearch
            className="ops-center-filter"
            optionFilterProp="label"
            placeholder={t('notifications.filters.project')}
            value={filters.project_id}
            options={(projectsQuery.data ?? []).map((project) => ({ value: project.id, label: displayProject(project) }))}
            onChange={(value) => updateFilters({ project_id: value })}
          />
          <Input
            allowClear
            className="ops-center-search"
            prefix={<Search size={14} />}
            placeholder={t('notifications.center.keyword')}
            value={filters.keyword}
            onChange={(event) => updateFilters({ keyword: event.target.value || undefined })}
          />
          <DatePicker.RangePicker
            allowClear
            showTime
            className="ops-center-date-range"
            value={timeRangeValue}
            placeholder={[t('notifications.center.from'), t('notifications.center.to')]}
            onChange={(value) => {
              updateFilters({
                from: value?.[0]?.toISOString(),
                to: value?.[1]?.toISOString(),
              })
            }}
          />
          <div className="ops-center-actions">
            <Button icon={<RefreshCw size={14} />} onClick={() => notificationsQuery.refetch()} loading={notificationsQuery.isFetching}>
              {t('actions.refresh')}
            </Button>
            <Button type="primary" icon={<CheckCheck size={14} />} onClick={() => markAllMutation.mutate()} loading={markAllMutation.isPending}>
              {t('notifications.readAll')}
            </Button>
          </div>
        </div>

        <Table<UserNotification>
          rowKey="id"
          className="ops-center-table"
          columns={columns}
          dataSource={notificationsQuery.data?.items ?? []}
          loading={notificationsQuery.isFetching}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('notifications.empty')} /> }}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: notificationsQuery.data?.total ?? 0,
            size: 'small',
            showSizeChanger: true,
            showTotal: (total, range) => t('notifications.center.paginationTotal', { from: range[0], to: range[1], total }),
            pageSizeOptions: [20, 50, 100],
          }}
          scroll={{ x: 960, y: 'calc(100vh - 470px)' }}
          onChange={handleTableChange}
        />
      </section>
    </div>
  )
}

function typeLabel(type: string, t: (key: string) => string) {
  const option = notificationTypeOptions.find((item) => item.value === type)
  return option ? t(option.labelKey) : type
}

function levelColor(level: string) {
  if (level === 'success') return 'green'
  if (level === 'warning') return 'gold'
  if (level === 'error') return 'red'
  return 'blue'
}

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
