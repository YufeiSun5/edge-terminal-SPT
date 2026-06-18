import { useMemo, useState, useEffect } from 'react'
import { useParams, useSearchParams, useNavigate } from 'react-router'
import { ConfigProvider, Tabs, Button, Select, DatePicker } from 'antd'
import dayjs from 'dayjs'
import type { Dayjs } from 'dayjs'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowLeft } from 'lucide-react'
import { getDetectionRun, getDetectionRuns } from '@/features/edge-status/api'
import { DetailedDataTab } from './components/DetailedDataTab'
import { ExcelReportsTab } from './components/ExcelReportsTab'
import { AlarmsEventsTab } from './components/AlarmsEventsTab'
import { DataDownloadTab } from './components/DataDownloadTab'
import './history-query.css'

type TimeRange = [Dayjs | null, Dayjs | null] | null

export function TaskDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { taskId: taskIdStr } = useParams()
  const taskId = Number(taskIdStr)
  const validTaskId = Number.isFinite(taskId) && taskId > 0 ? taskId : 0
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') || 'data'

  const runDetailQuery = useQuery({
    queryKey: ['history', 'run', validTaskId],
    queryFn: () => getDetectionRun(validTaskId),
    enabled: validTaskId > 0,
    retry: false,
  })

  const runsQuery = useQuery({
    queryKey: ['history', 'detection-runs'],
    queryFn: () => getDetectionRuns({ limit: 200 }),
    refetchInterval: 30000,
    retry: false,
  })

  const runs = useMemo(() => runsQuery.data?.items ?? [], [runsQuery.data?.items])
  const selectedRun = runDetailQuery.data ?? runs.find((run) => run.id === taskId)
  const runsForFilters = useMemo(() => {
    if (!selectedRun) return runs
    return [selectedRun, ...runs.filter((run) => run.id !== selectedRun.id)]
  }, [runs, selectedRun])

  const [filterFactory, setFilterFactory] = useState<string | null | undefined>(undefined)
  const [filterConfig, setFilterConfig] = useState<string | null | undefined>(undefined)
  const [filterProject, setFilterProject] = useState<string | null | undefined>(undefined)
  const [filterTimeRange, setFilterTimeRange] = useState<TimeRange | undefined>(undefined)

  const defaultTimeRange = useMemo<TimeRange>(() => {
    if (!selectedRun?.started_at) return null
    const day = dayjs(selectedRun.started_at)
    return [day.startOf('day'), day.endOf('day')]
  }, [selectedRun])
  const effectiveFactory = filterFactory === undefined ? selectedRun?.factory_no || undefined : filterFactory || undefined
  const effectiveConfig =
    filterConfig === undefined ? selectedRun?.config_name || selectedRun?.standard_code || undefined : filterConfig || undefined
  const effectiveProject = filterProject === undefined ? selectedRun?.project_code || undefined : filterProject || undefined
  const effectiveTimeRange = filterTimeRange === undefined ? defaultTimeRange : filterTimeRange

  const factoryOptions = useMemo(() => {
    const factories = new Set(runsForFilters.map(r => r.factory_no).filter(Boolean))
    return Array.from(factories).map(f => ({ label: f, value: f }))
  }, [runsForFilters])

  const configOptions = useMemo(() => {
    const configs = new Set(runsForFilters.map(r => r.config_name || r.standard_code).filter(Boolean))
    return Array.from(configs).map(c => ({ label: c, value: c }))
  }, [runsForFilters])

  const projectOptions = useMemo(() => {
    const codes = new Set(runsForFilters.map(r => r.project_code).filter(Boolean))
    return Array.from(codes).map(c => ({ label: c, value: c }))
  }, [runsForFilters])

  const filteredRuns = useMemo(() => {
    return runsForFilters.filter(r => {
      if (effectiveFactory && r.factory_no !== effectiveFactory) return false;
      if (effectiveConfig && (r.config_name || r.standard_code) !== effectiveConfig) return false;
      if (effectiveProject && r.project_code !== effectiveProject) return false;
      if (effectiveTimeRange && effectiveTimeRange[0] && effectiveTimeRange[1]) {
        const runStart = dayjs(r.started_at);
        if (runStart.isBefore(effectiveTimeRange[0].startOf('day')) || runStart.isAfter(effectiveTimeRange[1].endOf('day'))) {
          return false;
        }
      }
      return true;
    })
  }, [runsForFilters, effectiveFactory, effectiveConfig, effectiveProject, effectiveTimeRange])

  const handleTabChange = (key: string) => {
    setSearchParams({ tab: key }, { replace: true })
  }

  const hasManualFilter = filterFactory !== undefined || filterConfig !== undefined || filterProject !== undefined || filterTimeRange !== undefined

  useEffect(() => {
    if (!hasManualFilter) return
    if (filteredRuns.length > 0 && !filteredRuns.find(r => r.id === taskId)) {
      navigate(`/history/runs/${filteredRuns[0].id}?tab=${activeTab}`)
    }
  }, [activeTab, filteredRuns, hasManualFilter, navigate, taskId])

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

        <div className="history-detailed-view-container">
          <header className="history-task-context-bar glass-panel">
            <Button type="text" icon={<ArrowLeft size={16} />} onClick={() => navigate('/history')} style={{ paddingLeft: 0, paddingRight: 16 }}>
              {t('history.detail.back')}
            </Button>
            <div className="history-divider" style={{ marginRight: 16 }} />
            <div className="context-item">
              <span className="context-label">{t('history.detail.factoryNo')}</span>
              <Select
                allowClear
                showSearch
                placeholder={t('history.detail.allFactoryNo')}
                value={effectiveFactory}
                onChange={(value) => setFilterFactory(value ?? null)}
                options={factoryOptions}
                style={{ width: 140 }}
                variant="borderless"
              />
            </div>
            <div className="history-divider" />
            <div className="context-item">
              <span className="context-label">{t('history.detail.config')}</span>
              <Select
                allowClear
                showSearch
                placeholder={t('history.detail.allConfig')}
                value={effectiveConfig}
                onChange={(value) => setFilterConfig(value ?? null)}
                options={configOptions}
                style={{ width: 160 }}
                variant="borderless"
              />
            </div>
            <div className="history-divider" />
            <div className="context-item">
              <span className="context-label">{t('history.detail.station')}</span>
              <Select
                allowClear
                showSearch
                placeholder={t('history.detail.allStation')}
                value={effectiveProject}
                onChange={(value) => setFilterProject(value ?? null)}
                options={projectOptions}
                style={{ width: 140 }}
                variant="borderless"
              />
            </div>
            <div className="history-divider" />
            <div className="context-item">
              <span className="context-label">{t('history.detail.timeRange')}</span>
              <DatePicker.RangePicker
                allowClear
                value={effectiveTimeRange}
                onChange={(dates) => setFilterTimeRange(dates as TimeRange)}
                style={{ width: 240 }}
                variant="borderless"
              />
            </div>
          </header>

          <div className="history-tabs-container">
            <Tabs
              activeKey={activeTab}
              onChange={handleTabChange}
              className="history-tabs"
              type="card"
              size="small"
              destroyOnHidden
              items={[
                {
                  key: 'data',
                  label: t('history.detail.tabs.data'),
                  children: <DetailedDataTab taskId={taskId} />
                },
                {
                  key: 'reports',
                  label: t('history.detail.tabs.reports'),
                  children: <ExcelReportsTab taskId={taskId} />
                },
                {
                  key: 'alarms',
                  label: t('history.detail.tabs.alarms'),
                  children: <AlarmsEventsTab taskId={taskId} />
                },
                {
                  key: 'downloads',
                  label: t('history.detail.tabs.downloads'),
                  children: <DataDownloadTab taskId={taskId} />
                }
              ]}
            />
          </div>
        </div>
      </div>
    </ConfigProvider>
  )
}
