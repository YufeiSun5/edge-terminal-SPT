import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Badge, Button, Empty, Popover, Segmented, Select, Tooltip, message } from 'antd'
import { useTranslation } from 'react-i18next'
import {
  BarChart3,
  Bell,
  CheckCheck,
  FileText,
  Clock,
  FolderOpen,
  Languages,
  LayoutDashboard,
  LogOut,
  Send,
  RefreshCw,
  RotateCcw,
  Server,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  TriangleAlert,
  UserRound,
  Workflow,
  LayoutGrid,
  PieChart,
  ChevronDown,
  ChevronRight,
} from 'lucide-react'
import { openExternal, openLogs, restartSidecar } from '@/shared/desktop/desktopBridge'
import { createSsoTicket, logout } from '@/features/auth/api'
import { useAuthStore } from '@/features/auth/authStore'
import {
  getMainReportNotificationUnreadCount,
  getMainReportNotifications,
  getNotificationUnreadCount,
  getNotifications,
  getProjects,
  markAllMainReportNotificationsRead,
  markAllNotificationsRead,
  markMainReportNotificationRead,
  markNotificationRead,
} from '@/features/edge-status/api'
import { subscribeRealtimeWebSocket } from '@/features/realtime/realtimeClient'
import { env } from '@/shared/config/env'
import type { MainReportNotification, Project, UserNotification } from '@/shared/api/types'
import { languageCode } from '@/shared/i18n/language'
import { queryClient } from './queryClient'

const navItems = [
  { path: '/', key: 'station', icon: LayoutGrid, permissions: ['view_realtime'] },
  { path: '/model-cockpit', key: 'modelCockpit', icon: PieChart, permissions: ['view_realtime'] },
  { path: '/history', key: 'history', icon: Clock, permissions: ['view_history'] },
  { path: '/report-settings', key: 'reportSettings', icon: FileText, permissions: ['system_settings'] },
  { path: '/notifications', key: 'notifications', icon: Bell, permissions: ['view_realtime', 'view_history', 'system_settings'] },
  { path: '/alarms', key: 'alarms', icon: TriangleAlert, permissions: ['view_realtime'] },
  { path: '/variables', key: 'variables', icon: SlidersHorizontal, permissions: ['manage_variables'] },
  { path: '/detection-config', key: 'detectionConfig', icon: ShieldCheck, permissions: ['manage_variables'] },
  { path: '/tasks', key: 'tasks', icon: Workflow, permissions: ['system_settings'] },
  { path: '/settings', key: 'settings', icon: Settings2, permissions: ['manage_variables', 'manage_gateways', 'system_settings', 'manage_users'] },
  { path: '/debug', key: 'debug', icon: LayoutDashboard, permissions: ['system_settings'] },
]

const notificationTypeOptions = [
  { value: 'alarm.limit.enter', labelKey: 'notifications.types.alarmEnter' },
  { value: 'alarm.limit.recover', labelKey: 'notifications.types.alarmRecover' },
  { value: 'detection.run_started', labelKey: 'notifications.types.runStarted' },
  { value: 'detection.run_stopped', labelKey: 'notifications.types.runStopped' },
  { value: 'detection.result_ok', labelKey: 'notifications.types.resultOk' },
  { value: 'detection.result_ng', labelKey: 'notifications.types.resultNg' },
  { value: 'report.job', labelKey: 'notifications.types.reportJob' },
]

function reportNotificationToUserNotification(notification: MainReportNotification): UserNotification {
  const payload = notification.payload ?? {}
  const taskId = Number(payload.task_id ?? 0)
  const projectId = Number(payload.project_id ?? 0)
  const reportName = String(payload.report_name ?? notification.title ?? '')
  return {
    id: -notification.id,
    event_uid: `main-report-${notification.id}`,
    type: 'report.job',
    level: notification.level,
    target_type: 'all',
    target_id: String(notification.job_id),
    project_id: Number.isFinite(projectId) ? projectId : 0,
    project_code: typeof payload.project_code === 'string' ? payload.project_code : undefined,
    task_id: Number.isFinite(taskId) && taskId > 0 ? taskId : undefined,
    test_no: typeof payload.test_no === 'string' ? payload.test_no : undefined,
    display_name: reportName || notification.title,
    message: notification.message,
    payload: { ...payload, report_notification_id: notification.id, job_id: notification.job_id },
    occurred_at: notification.created_at,
    created_at: notification.created_at,
    read_at: notification.read_at,
  }
}

function sortNotifications(items: UserNotification[]) {
  return items.sort((left, right) => {
    const leftTime = Date.parse(left.occurred_at || left.created_at || '')
    const rightTime = Date.parse(right.occurred_at || right.created_at || '')
    return (Number.isFinite(rightTime) ? rightTime : 0) - (Number.isFinite(leftTime) ? leftTime : 0)
  })
}

export function ShellLayout() {
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const [messageApi, contextHolder] = message.useMessage()
  const [stationExpanded, setStationExpanded] = useState(true)
  const [historyExpanded, setHistoryExpanded] = useState(true)
  const [reportSettingsExpanded, setReportSettingsExpanded] = useState(true)
  const [notificationOpen, setNotificationOpen] = useState(false)
  const [notificationUnreadFilter, setNotificationUnreadFilter] = useState<'all' | 'unread'>('all')
  const [notificationTypeFilter, setNotificationTypeFilter] = useState<string>()
  const [notificationProjectFilter, setNotificationProjectFilter] = useState<number>()
  const [time, setTime] = useState(() => new Date().toLocaleTimeString('zh-CN', { hour12: false }))
  const user = useAuthStore((state) => state.user)
  const hasAnyPermission = useAuthStore((state) => state.hasAnyPermission)
  const clearSession = useAuthStore((state) => state.clearSession)
  const projectsQuery = useQuery({
    queryKey: ['shell', 'projects'],
    queryFn: getProjects,
    refetchInterval: 10000,
    retry: false,
  })
  const unreadQuery = useQuery({
    queryKey: ['shell', 'notifications', 'unread-count'],
    queryFn: async () => {
      const base = await getNotificationUnreadCount()
      if (env.runtimeRole !== 'main_server') return base
      const report = await getMainReportNotificationUnreadCount()
      return { unread: base.unread + report.unread }
    },
    refetchInterval: 10000,
    retry: false,
  })
  const notificationsQuery = useQuery({
    queryKey: ['shell', 'notifications', 'latest', notificationUnreadFilter, notificationTypeFilter ?? 'all', notificationProjectFilter ?? 'all'],
    queryFn: async () => {
      if (notificationTypeFilter === 'report.job') {
        if (env.runtimeRole !== 'main_server' || notificationProjectFilter) {
          return { items: [], total: 0, limit: 20, offset: 0 }
        }
        const report = await getMainReportNotifications({
          limit: 20,
          unread: notificationUnreadFilter === 'unread' ? true : undefined,
        })
        return { ...report, items: report.items.map(reportNotificationToUserNotification) }
      }
      const base = await getNotifications({
        limit: 20,
        unread: notificationUnreadFilter === 'unread' ? true : undefined,
        type: notificationTypeFilter,
        project_id: notificationProjectFilter,
      })
      if (env.runtimeRole !== 'main_server' || notificationProjectFilter || (notificationTypeFilter && notificationTypeFilter !== 'report.job')) {
        return base
      }
      const report = await getMainReportNotifications({
        limit: 20,
        unread: notificationUnreadFilter === 'unread' ? true : undefined,
      })
      const reportItems = report.items.map(reportNotificationToUserNotification)
      const items = sortNotifications([...base.items, ...reportItems]).slice(0, 20)
      return { ...base, items, total: base.total + report.total }
    },
    enabled: notificationOpen,
    staleTime: 5000,
    retry: false,
  })

  const stationProjects = projectsQuery.data ?? []
  const activeProjectId = new URLSearchParams(location.search).get('project_id')
  const visibleNavItems = navItems.filter((item) => hasAnyPermission(item.permissions))
  const displayProjectName = (project: Pick<Project, 'project_code' | 'name' | 'display_name' | 'display_name_en' | 'display_name_ja'>) => {
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return project.display_name_en || project.project_code
    if (currentLanguage === 'ja') return project.display_name_ja || project.project_code
    return project.display_name || project.name || project.project_code
  }

  const logoutMutation = useMutation({
    mutationFn: logout,
    onSettled: () => {
      clearSession()
      queryClient.clear()
      navigate('/login', { replace: true })
    },
  })

  const ssoMutation = useMutation({
    mutationFn: createSsoTicket,
    onSuccess: async (response) => {
      if (!response.main_site_url) {
        messageApi.warning(t('auth.ssoNotConfigured'))
        return
      }
      await openExternal(response.main_site_url)
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('auth.ssoFailed')),
  })
  const markReadMutation = useMutation({
    mutationFn: (notificationId: number) => notificationId < 0 ? markMainReportNotificationRead(Math.abs(notificationId)) : markNotificationRead(notificationId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['shell', 'notifications'] }),
      ])
    },
  })
  const markAllReadMutation = useMutation({
    mutationFn: async () => {
      const base = await markAllNotificationsRead()
      if (env.runtimeRole !== 'main_server') return base
      const report = await markAllMainReportNotificationsRead()
      return { updated: base.updated + report.updated }
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['shell', 'notifications'] }),
      ])
    },
  })

  useEffect(() => {
    const timer = window.setInterval(() => {
      setTime(new Date().toLocaleTimeString('zh-CN', { hour12: false }))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    let reconnectTimer = 0
    let disposed = false
    let unsubscribe: (() => void) | undefined

    const connect = () => {
      const scheduleReconnect = () => {
        if (disposed || reconnectTimer) return
        reconnectTimer = window.setTimeout(() => {
          reconnectTimer = 0
          connect()
        }, 3000)
      }

      unsubscribe = subscribeRealtimeWebSocket({
        subscription: { topics: ['notifications'] },
        onMessage: (message) => {
          if (message.type !== 'notification.event') return
          void queryClient.invalidateQueries({ queryKey: ['shell', 'notifications'] })
        },
        onClose: scheduleReconnect,
        onError: scheduleReconnect,
      })
    }

    connect()

    return () => {
      disposed = true
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      unsubscribe?.()
    }
  }, [])

  function notificationTitle(notification: UserNotification) {
    return notification.display_name || notification.var_name || notification.test_no || notification.type
  }

  function openNotificationTarget(notification: UserNotification) {
    if (!notification.read_at) markReadMutation.mutate(notification.id)
    setNotificationOpen(false)

    const params = new URLSearchParams()
    if (notification.type.startsWith('alarm.')) {
      if (notification.project_id) params.set('project_id', String(notification.project_id))
      if (notification.task_id) {
        params.set('task_id', String(notification.task_id))
        params.set('scope', 'detection')
      }
      if (notification.var_id_text ?? notification.var_id) params.set('var_id', String(notification.var_id_text ?? notification.var_id))
      params.set('status', 'active')
      navigate(`/alarms?${params.toString()}`)
      return
    }

    if (notification.type === 'report.job') {
      if (notification.task_id && hasAnyPermission(['view_history'])) {
        navigate(`/history/runs/${notification.task_id}?tab=reports`)
      } else {
        navigate('/history')
      }
      return
    }

    if (notification.task_id && hasAnyPermission(['view_history'])) {
      params.set('task_id', String(notification.task_id))
      if (notification.project_id) params.set('project_id', String(notification.project_id))
      if (notification.test_no) params.set('test_no', notification.test_no)
      navigate(`/history?${params.toString()}`)
      return
    }

    if (notification.project_id && hasAnyPermission(['view_realtime'])) {
      params.set('project_id', String(notification.project_id))
      navigate({ pathname: '/', search: params.toString() })
    }
  }

  const notificationContent = (
    <div className="notification-popover">
      <div className="notification-popover-head">
        <div>
          <strong>{t('notifications.title')}</strong>
          <span>
            {t('notifications.unread', { count: unreadQuery.data?.unread ?? 0 })}
            {notificationsQuery.data ? ` · ${t('notifications.filtered', { count: notificationsQuery.data.total })}` : ''}
          </span>
        </div>
        <Button
          size="small"
          icon={<CheckCheck size={14} />}
          loading={markAllReadMutation.isPending}
          onClick={() => markAllReadMutation.mutate()}
        >
          {t('notifications.readAll')}
        </Button>
      </div>
      <Button className="notification-page-link" type="link" onClick={() => {
        setNotificationOpen(false)
        navigate('/notifications')
      }}>
        {t('notifications.openCenter')}
      </Button>
      <div className="notification-filters">
        <Segmented
          size="small"
          value={notificationUnreadFilter}
          onChange={(value) => setNotificationUnreadFilter(value as 'all' | 'unread')}
          options={[
            { label: t('notifications.filters.all'), value: 'all' },
            { label: t('notifications.filters.unread'), value: 'unread' },
          ]}
        />
        <Select
          allowClear
          size="small"
          className="notification-filter-select"
          placeholder={t('notifications.filters.type')}
          value={notificationTypeFilter}
          onChange={(value) => setNotificationTypeFilter(value)}
          options={notificationTypeOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }))}
        />
        <Select
          allowClear
          showSearch
          size="small"
          className="notification-filter-select"
          optionFilterProp="label"
          placeholder={t('notifications.filters.project')}
          value={notificationProjectFilter}
          onChange={(value) => setNotificationProjectFilter(value)}
          options={stationProjects.map((project) => ({ value: project.id, label: displayProjectName(project) }))}
        />
      </div>
      <div className="notification-list">
        {(notificationsQuery.data?.items.length ?? 0) === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('notifications.empty')} />
        ) : (
          notificationsQuery.data?.items.map((notification) => (
            <button
              className={notification.read_at ? 'notification-item read' : `notification-item ${notification.level}`}
              key={notification.id}
              title={t('notifications.openTarget')}
              onClick={() => openNotificationTarget(notification)}
            >
              <span className="notification-level-dot" />
              <span className="notification-item-main">
                <strong>{notificationTitle(notification)}</strong>
                <span>{notification.message || notification.type}</span>
                <small>{new Date(notification.occurred_at || notification.created_at).toLocaleString()}</small>
              </span>
            </button>
          ))
        )}
      </div>
    </div>
  )

  return (
    <div className="workbench-shell">
      {contextHolder}
      <aside className="workbench-sidebar">
        <div className="workbench-brand">
          <span className="brand-mark">
            <Server aria-hidden="true" />
          </span>
          <div>
            <h1 className="brand-title">{t('app.title')}</h1>
            <p className="brand-subtitle">{t('app.subtitle')}</p>
          </div>
        </div>

        <nav className="workbench-nav" aria-label={t('nav.main')}>
          {visibleNavItems.map((item) => {
            const isStationNav = item.key === 'station'
            const stationActive = isStationNav && (location.pathname === '/' || location.pathname === '/station')
            const isHistoryNav = item.key === 'history'
            const historyActive = isHistoryNav && location.pathname.startsWith('/history')
            const isReportSettingsNav = item.key === 'reportSettings'
            const reportSettingsActive = isReportSettingsNav && location.pathname.startsWith('/report-settings')

            return (
              <div className="nav-group" key={item.path}>
                {isStationNav ? (
                  <button
                    className={stationActive ? 'nav-link nav-button active' : 'nav-link nav-button'}
                    type="button"
                    onClick={() => setStationExpanded((value) => !value)}
                    aria-expanded={stationExpanded}
                  >
                    <item.icon size={18} />
                    <span>{t(`nav.${item.key}`)}</span>
                    {stationProjects.length > 0 ? (
                      <span className="nav-expand-icon">
                        {stationExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                      </span>
                    ) : null}
                  </button>
                ) : isHistoryNav ? (
                  <button
                    className={historyActive ? 'nav-link nav-button active' : 'nav-link nav-button'}
                    type="button"
                    onClick={() => setHistoryExpanded((value) => !value)}
                    aria-expanded={historyExpanded}
                  >
                    <item.icon size={18} />
                    <span>{t('nav.historyAndPlans')}</span>
                    <span className="nav-expand-icon">
                      {historyExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                    </span>
                  </button>
                ) : isReportSettingsNav ? (
                  <button
                    className={reportSettingsActive ? 'nav-link nav-button active' : 'nav-link nav-button'}
                    type="button"
                    onClick={() => setReportSettingsExpanded((value) => !value)}
                    aria-expanded={reportSettingsExpanded}
                  >
                    <item.icon size={18} />
                    <span>{t('nav.reportSettings')}</span>
                    <span className="nav-expand-icon">
                      {reportSettingsExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                    </span>
                  </button>
                ) : (
                  <NavLink
                    to={item.path}
                    end={item.path === '/'}
                    className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
                  >
                    <item.icon size={18} />
                    <span>{t(`nav.${item.key}`)}</span>
                  </NavLink>
                )}
                {isStationNav && stationExpanded && stationProjects.length > 0 ? (
                  <div className="nav-subtree">
                    {stationProjects.map((project) => {
                      const search = `?project_id=${project.id}`
                      const active = stationActive && activeProjectId === String(project.id)
                      const projectName = displayProjectName(project)
                      return (
                        <Link
                          className={active ? 'nav-sublink active' : 'nav-sublink'}
                          key={project.id}
                          to={{ pathname: '/', search }}
                          title={projectName}
                        >
                          <span>{projectName}</span>
                        </Link>
                      )
                    })}
                  </div>
                ) : null}
                {isHistoryNav && historyExpanded ? (
                  <div className="nav-subtree">
                    {[
                      { path: '/history', key: 'gantt' },
                      { path: '/history/list', key: 'historyList' },
                      { path: '/history/plans', key: 'plans' },
                    ].map((subItem) => (
                      <NavLink
                        className={({ isActive }) => (isActive ? 'nav-sublink active' : 'nav-sublink')}
                        end={subItem.path === '/history'}
                        key={subItem.path}
                        to={subItem.path}
                      >
                        <span>{t(`nav.historyChildren.${subItem.key}`)}</span>
                      </NavLink>
                    ))}
                  </div>
                ) : null}
                {isReportSettingsNav && reportSettingsExpanded ? (
                  <div className="nav-subtree">
                    {[
                      { path: '/report-settings/templates', key: 'templates' },
                      { path: '/report-settings/plan-imports', key: 'planImports' },
                    ].map((subItem) => (
                      <NavLink
                        className={({ isActive }) => (isActive ? 'nav-sublink active' : 'nav-sublink')}
                        key={subItem.path}
                        to={subItem.path}
                      >
                        <span>{t(`nav.reportSettingsChildren.${subItem.key}`)}</span>
                      </NavLink>
                    ))}
                  </div>
                ) : null}
              </div>
            )
          })}
        </nav>

        <div className="workbench-sidebar-footer">
          <span className="status-dot" />
          <span>
            {user?.username ?? t('auth.guest')} · {time}
          </span>
        </div>
      </aside>

      <section className="workbench-main">
        <header className="app-header">
          <div className="header-left">
            <span className="header-section">
              <BarChart3 size={15} />
              {t('app.workspace')}
            </span>
          </div>

          <div className="header-actions">
            <span className="header-user">
              <UserRound size={14} />
              {user?.username}
            </span>
            <Popover
              content={notificationContent}
              open={notificationOpen}
              autoAdjustOverflow={false}
              onOpenChange={(open) => {
                setNotificationOpen(open)
                if (open) void notificationsQuery.refetch()
              }}
              placement="bottomRight"
              trigger="click"
            >
              <Badge count={unreadQuery.data?.unread ?? 0} size="small">
                <Button className="icon-button" icon={<Bell size={16} />} aria-label={t('notifications.title')} />
              </Badge>
            </Popover>
            {hasAnyPermission(['sso_handoff']) ? (
              <Tooltip title={t('auth.openMainSite')}>
                <Button
                  className="icon-button"
                  icon={<Send size={16} />}
                  loading={ssoMutation.isPending}
                  onClick={() => ssoMutation.mutate()}
                />
              </Tooltip>
            ) : null}
            <Segmented
              size="small"
              value={i18n.resolvedLanguage ?? 'zh'}
              onChange={(value) => void i18n.changeLanguage(String(value))}
              options={[
                { label: '中文', value: 'zh', icon: <Languages size={14} /> },
                { label: 'EN', value: 'en' },
                { label: '日本語', value: 'ja' },
              ]}
            />
            <Tooltip title={t('actions.refresh')}>
              <Button
                className="icon-button"
                icon={<RefreshCw size={16} />}
                onClick={() => void queryClient.invalidateQueries()}
              />
            </Tooltip>
            <Tooltip title={t('actions.openLogs')}>
              <Button className="icon-button" icon={<FolderOpen size={16} />} onClick={() => void openLogs()} />
            </Tooltip>
            <Tooltip title={t('actions.restart')}>
              <Button
                className="icon-button"
                icon={<RotateCcw size={16} />}
                onClick={() => void restartSidecar()}
              />
            </Tooltip>
            <Tooltip title={t('auth.logout')}>
              <Button
                className="icon-button"
                icon={<LogOut size={16} />}
                loading={logoutMutation.isPending}
                onClick={() => logoutMutation.mutate()}
              />
            </Tooltip>
          </div>
        </header>
        <main className="workbench-content">
          <Outlet />
        </main>
      </section>
    </div>
  )
}
