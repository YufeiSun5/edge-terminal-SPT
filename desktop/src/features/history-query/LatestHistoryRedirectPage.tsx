import { useEffect } from 'react'
import { Alert, Button, Spin } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getDetectionRuns } from '@/features/edge-status/api'
import './history-query.css'

export function LatestHistoryRedirectPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const runsQuery = useQuery({
    queryKey: ['history', 'latest-run'],
    queryFn: () => getDetectionRuns({ limit: 1 }),
    retry: false,
  })
  const latestRun = runsQuery.data?.items?.[0]

  useEffect(() => {
    if (!latestRun) return
    navigate(`/history/runs/${latestRun.id}?tab=data`, { replace: true })
  }, [latestRun, navigate])

  if (runsQuery.isError) {
    return (
      <div className="history-page prototype-history-page history-dense-layout">
        <main className="history-content">
          <Alert
            className="history-api-alert"
            type="warning"
            showIcon
            message={t('history.dataSource.apiUnavailable')}
            action={<Button size="small" onClick={() => void runsQuery.refetch()}>{t('actions.refresh')}</Button>}
          />
        </main>
      </div>
    )
  }

  if (!runsQuery.isFetching && !latestRun) {
    return (
      <div className="history-page prototype-history-page history-dense-layout">
        <main className="history-content">
          <Alert className="history-api-alert" type="info" showIcon message={t('history.timeline.empty')} />
        </main>
      </div>
    )
  }

  return (
    <div className="history-page prototype-history-page history-dense-layout">
      <main className="history-content">
        <Spin />
      </main>
    </div>
  )
}
