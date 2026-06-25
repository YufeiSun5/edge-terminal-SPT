import { useEffect, useMemo } from 'react'
import { Alert, Button, Spin } from 'antd'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { verifySsoTicket } from './api'
import { useAuthStore } from './authStore'

export function SsoHandoffPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const params = useMemo(() => ssoParamsFromLocation(), [])
  const verifyMutation = useMutation({
    mutationFn: () => verifySsoTicket({ ticket: params.ticket, edge_id: params.edgeId }),
    onSuccess: (session) => {
      setSession(session)
      navigate('/', { replace: true })
    },
  })
  const { isPending, isSuccess, mutate } = verifyMutation

  useEffect(() => {
    if (!params.ticket || !params.edgeId || isPending || isSuccess) return
    mutate()
  }, [isPending, isSuccess, mutate, params.edgeId, params.ticket])

  if (!params.ticket || !params.edgeId) {
    return (
      <div className="auth-forbidden-page">
        <div className="auth-forbidden-panel">
          <Alert
            showIcon
            type="error"
            message={t('auth.ssoInvalidTitle')}
            description={t('auth.ssoInvalidDesc')}
          />
          <Button onClick={() => navigate('/login', { replace: true })}>{t('auth.login')}</Button>
        </div>
      </div>
    )
  }

  if (verifyMutation.isError) {
    return (
      <div className="auth-forbidden-page">
        <div className="auth-forbidden-panel">
          <Alert
            showIcon
            type="error"
            message={t('auth.ssoFailed')}
            description={verifyMutation.error instanceof Error ? verifyMutation.error.message : t('auth.ssoFailed')}
          />
          <Button onClick={() => navigate('/login', { replace: true })}>{t('auth.login')}</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="auth-forbidden-page">
      <div className="auth-forbidden-panel">
        <Spin />
        <span>{t('auth.ssoVerifying')}</span>
      </div>
    </div>
  )
}

function ssoParamsFromLocation() {
  const fromHash = new URLSearchParams(window.location.hash.split('?')[1] ?? '')
  const fromSearch = new URLSearchParams(window.location.search)
  return {
    ticket: firstNonEmpty(fromHash.get('ticket'), fromSearch.get('ticket')),
    edgeId: firstNonEmpty(fromHash.get('edge_id'), fromSearch.get('edge_id')),
  }
}

function firstNonEmpty(...values: Array<string | null>) {
  for (const value of values) {
    const trimmed = value?.trim()
    if (trimmed) return trimmed
  }
  return ''
}
