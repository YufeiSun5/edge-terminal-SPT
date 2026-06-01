import { useEffect } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { Alert, Spin } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from './authStore'
import { getCurrentUser } from './api'
import { hasAccessToken, onUnauthorized } from '@/shared/auth/tokenStore'

export function ProtectedRoute({ permissions = [] }: { permissions?: string[] }) {
  const { t } = useTranslation()
  const location = useLocation()
  const authenticated = useAuthStore((state) => state.authenticated)
  const setPrincipal = useAuthStore((state) => state.setPrincipal)
  const clearSession = useAuthStore((state) => state.clearSession)
  const hasAnyPermission = useAuthStore((state) => state.hasAnyPermission)

  useEffect(() => {
    const unsubscribe = onUnauthorized(clearSession)
    return () => {
      unsubscribe()
    }
  }, [clearSession])

  const sessionQuery = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: getCurrentUser,
    enabled: hasAccessToken() && !authenticated,
    retry: false,
  })

  useEffect(() => {
    if (sessionQuery.data) setPrincipal(sessionQuery.data)
  }, [sessionQuery.data, setPrincipal])

  useEffect(() => {
    if (sessionQuery.isError) clearSession()
  }, [clearSession, sessionQuery.isError])

  if (sessionQuery.isFetching) {
    return (
      <div className="auth-forbidden-page">
        <Spin />
      </div>
    )
  }

  if (!authenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  if (permissions.length > 0 && !hasAnyPermission(permissions)) {
    return (
      <div className="auth-forbidden-page">
        <div className="auth-forbidden-panel">
          <Alert
            showIcon
            type="error"
            message={t('auth.forbiddenTitle')}
            description={t('auth.forbiddenDesc')}
          />
        </div>
      </div>
    )
  }

  return <Outlet />
}
