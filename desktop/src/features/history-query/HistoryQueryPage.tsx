import { useMemo, useState } from 'react'
import { ConfigProvider, Table, Button, Alert, Segmented } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { LayoutGrid, Table as TableIcon } from 'lucide-react'
import { getDetectionRuns } from '@/features/edge-status/api'
import type { DetectionRun } from '@/shared/api/types'
import { HistoryGanttView } from './components/HistoryGanttView'
import { buildTaskLanes, formatHistoryTime } from './model'
import './history-query.css'

export function HistoryQueryPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [viewMode, setViewMode] = useState<'gantt' | 'table'>('gantt')

  const runsQuery = useQuery({
    queryKey: ['history', 'detection-runs'],
    queryFn: () => getDetectionRuns({ limit: 200 }),
    refetchInterval: 30000,
    retry: false,
  })

  const runs = useMemo(() => runsQuery.data?.items ?? [], [runsQuery.data?.items])
  const { lanes: taskLanes, minTime: dataMinTime, maxTime: dataMaxTime } = useMemo(() => buildTaskLanes(runs), [runs])

  const handleRunSelect = (taskId: number) => {
    navigate(`/history/runs/${taskId}?tab=data`)
  }

  const summaryColumns: TableColumnsType<DetectionRun> = [
    { title: t('history.detail.factoryNo'), dataIndex: 'factory_no', key: 'factory_no', render: (val) => val || '--' },
    { title: t('history.detail.reports.testNo'), dataIndex: 'test_no', key: 'test_no', render: (val) => val || '--' },
    { title: t('history.timeline.projectStation'), dataIndex: 'project_code', key: 'project_code' },
    { title: t('history.timeline.testType'), dataIndex: 'mode', key: 'mode', render: (val) => val === 'standard' ? t('history.timeline.standardTest') : t('history.timeline.freeLongRun') },
    { title: t('history.filters.start'), dataIndex: 'started_at', key: 'started_at', render: (val) => formatHistoryTime(val) || '--' },
    { title: t('history.filters.end'), dataIndex: 'ended_at', key: 'ended_at', render: (val) => formatHistoryTime(val) || '--' },
    { title: t('history.timeline.result'), dataIndex: 'status', key: 'status' },
    {
      title: t('history.timeline.operation'),
      key: 'action',
      render: (_, record) => (
        <Button type="link" size="small" onClick={() => handleRunSelect(record.id)}>{t('history.timeline.viewDetail')}</Button>
      )
    },
  ]

  return (
    <ConfigProvider
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
      <div className="history-page prototype-history-page history-dense-layout">
        <div className="history-ambient-background" aria-hidden="true">
          <div className="history-orb history-orb-1" />
          <div className="history-orb history-orb-2" />
          <div className="history-orb history-orb-3" />
          <div className="history-noise" />
        </div>

        <main className="history-content">
          {runsQuery.isError ? (
            <Alert
              className="history-api-alert"
              type="warning"
              showIcon
              message={t('history.dataSource.apiUnavailable')}
              action={<Button size="small" onClick={() => {
                void runsQuery.refetch()
              }}>{t('actions.refresh')}</Button>}
            />
          ) : null}

          <div className="history-gantt-view glass-panel">
            <div className="history-gantt-toolbar">
              <div className="history-gantt-toolbar-left">
                <Segmented
                  value={viewMode}
                  onChange={(value) => setViewMode(value as 'gantt' | 'table')}
                  options={[
                    { value: 'gantt', label: <div className="history-segmented-item"><LayoutGrid size={14} /> {t('history.timeline.gantt')}</div> },
                    { value: 'table', label: <div className="history-segmented-item"><TableIcon size={14} /> {t('history.timeline.taskList')}</div> },
                  ]}
                />
              </div>
            </div>

            {viewMode === 'gantt' ? (
              <HistoryGanttView
                lanes={taskLanes}
                dataMinTime={dataMinTime}
                dataMaxTime={dataMaxTime}
                loading={runsQuery.isFetching}
                onSelect={handleRunSelect}
              />
            ) : (
              <div className="history-gantt-body">
                <div className="history-gantt-title">
                  <h2>{t('history.timeline.summaryList')}</h2>
                </div>
                <div className="history-panel-body" style={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
                  <Table
                    rowKey="id"
                    dataSource={runs}
                    columns={summaryColumns}
                    size="small"
                    loading={runsQuery.isFetching}
                    pagination={{ pageSize: 20, size: "small" }}
                  />
                </div>
              </div>
            )}
          </div>
        </main>
      </div>
    </ConfigProvider>
  )
}
