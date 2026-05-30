import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Segmented, Tooltip, message } from 'antd'
import { useTranslation } from 'react-i18next'
import {
  ActivitySquare,
  BarChart3,
  Box,
  FileSearch,
  FolderOpen,
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  Send,
  RefreshCw,
  RotateCcw,
  Server,
  Settings2,
  ShieldCheck,
  UserRound,
  Workflow,
} from 'lucide-react'
import { openExternal, openLogs, restartSidecar } from '@/shared/desktop/desktopBridge'
import { createSsoTicket, logout } from '@/features/auth/api'
import { useAuthStore } from '@/features/auth/authStore'
import { getDevices } from '@/features/edge-status/api'
import { queryClient } from './queryClient'

const navItems = [
  { path: '/', key: 'station', icon: ActivitySquare, permissions: ['view_realtime'] },
  { path: '/model-cockpit', key: 'modelCockpit', icon: Box, permissions: ['view_realtime'] },
  { path: '/history', key: 'history', icon: FileSearch, permissions: ['view_history'] },
  { path: '/detection-config', key: 'detectionConfig', icon: ShieldCheck, permissions: ['manage_variables'] },
  { path: '/tasks', key: 'tasks', icon: Workflow, permissions: ['system_settings'] },
  { path: '/settings', key: 'settings', icon: Settings2, permissions: ['manage_variables', 'manage_gateways', 'system_settings', 'manage_users'] },
  { path: '/debug', key: 'debug', icon: LayoutDashboard, permissions: ['system_settings'] },
]

export function ShellLayout() {
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const [messageApi, contextHolder] = message.useMessage()
  const [collapsed, setCollapsed] = useState(false)
  const [time, setTime] = useState(() => new Date().toLocaleTimeString('zh-CN', { hour12: false }))
  const user = useAuthStore((state) => state.user)
  const hasAnyPermission = useAuthStore((state) => state.hasAnyPermission)
  const clearSession = useAuthStore((state) => state.clearSession)
  const devicesQuery = useQuery({
    queryKey: ['shell', 'devices'],
    queryFn: getDevices,
    refetchInterval: 10000,
    retry: false,
  })

  const stationDevices = devicesQuery.data ?? []
  const visibleNavItems = navItems.filter((item) => hasAnyPermission(item.permissions))
  const displayDeviceName = (device: { device_code: string; name?: string; display_name?: string; display_name_en?: string; display_name_ja?: string }) => {
    if (i18n.resolvedLanguage === 'en') return device.display_name_en || device.display_name || device.name || device.device_code
    if (i18n.resolvedLanguage === 'ja') return device.display_name_ja || device.display_name || device.name || device.device_code
    return device.display_name || device.name || device.device_code
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

  useEffect(() => {
    const timer = window.setInterval(() => {
      setTime(new Date().toLocaleTimeString('zh-CN', { hour12: false }))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [])

  return (
    <div className="workbench-shell">
      {contextHolder}
      <aside className={collapsed ? 'workbench-sidebar collapsed' : 'workbench-sidebar'}>
        <div className="workbench-brand">
          <span className="brand-mark">
            <Server aria-hidden="true" />
          </span>
          {!collapsed && (
            <div>
              <h1 className="brand-title">{t('app.title')}</h1>
              <p className="brand-subtitle">{t('app.subtitle')}</p>
            </div>
          )}
        </div>

        <nav className="workbench-nav" aria-label={t('nav.main')}>
          {visibleNavItems.map((item) => (
            <div className="nav-group" key={item.path}>
              <NavLink
                to={item.path}
                end={item.path === '/'}
                className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
                title={collapsed ? t(`nav.${item.key}`) : undefined}
              >
                <item.icon size={18} />
                {!collapsed && <span>{t(`nav.${item.key}`)}</span>}
              </NavLink>
              {item.key === 'station' && !collapsed && stationDevices.length > 0 ? (
                <div className="nav-subtree">
                  {stationDevices.map((device) => {
                    const search = `?device_id=${device.id}`
                    const active = location.pathname === '/' && location.search === search
                    const deviceName = displayDeviceName(device)
                    return (
                      <Link
                        className={active ? 'nav-sublink active' : 'nav-sublink'}
                        key={device.id}
                        to={{ pathname: '/', search }}
                        title={deviceName}
                      >
                        <span>{deviceName}</span>
                      </Link>
                    )
                  })}
                </div>
              ) : null}
            </div>
          ))}
        </nav>

        <div className="workbench-sidebar-footer">
          <span className="status-dot" />
          {!collapsed && (
            <span>
              {user?.username ?? t('auth.guest')} · {time}
            </span>
          )}
        </div>
      </aside>

      <section className="workbench-main">
        <header className="app-header">
          <div className="header-left">
            <Button
              className="icon-button"
              icon={<Menu size={16} />}
              onClick={() => setCollapsed((value) => !value)}
            />
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
