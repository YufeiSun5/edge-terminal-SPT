import { useMemo, useState } from 'react'
import { Alert, ConfigProvider } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import zhCN from 'antd/locale/zh_CN'
import { useTranslation } from 'react-i18next'
import {
  ActivitySquare,
  ArrowDownToLine,
  ArrowUpToLine,
  Calendar,
  Cpu,
  Download,
  ListFilter,
  Search,
  Server,
  SlidersHorizontal,
} from 'lucide-react'
import { GanttChartModal } from './components/GanttChartModal'
import { HistoryTable } from './components/HistoryTable'
import { TrendChart } from './components/TrendChart'
import { getHistoryData } from './api'
import { generateHistoryData, historyItemsToRows } from './model'
import './history-query.css'

export function HistoryQueryPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const [selectedMetrics] = useState(['tempOut', 'humidOut', 'pressure'])
  const [isGanttOpen, setGanttOpen] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const taskId = Number(searchParams.get('task_id'))
  const deviceId = Number(searchParams.get('device_id'))
  const selectedTaskId = Number.isFinite(taskId) && taskId > 0 ? taskId : undefined
  const selectedDeviceId = Number.isFinite(deviceId) && deviceId > 0 ? deviceId : undefined
  const selectedTestNo = searchParams.get('test_no') || undefined
  const [machineId, setMachineId] = useState(selectedTestNo ?? '测试机一')
  const [sn, setSn] = useState(selectedDeviceId ? String(selectedDeviceId) : 'A-102')
  const [timeRange, setTimeRange] = useState('2026-05-27 00:00 - 23:59')
  const historyQuery = useQuery({
    queryKey: ['history', 'data', machineId, sn, timeRange, selectedTaskId, selectedDeviceId, selectedTestNo],
    queryFn: () =>
      getHistoryData({
        task_id: selectedTaskId,
        device_id: selectedDeviceId,
        test_no: selectedTaskId ? undefined : selectedTestNo,
        limit: 5000,
      }),
    refetchInterval: 30000,
    retry: false,
  })
  const fallbackData = useMemo(() => generateHistoryData(), [])
  const apiRows = useMemo(() => historyItemsToRows(historyQuery.data?.items ?? []), [historyQuery.data?.items])
  const data = apiRows.length > 0 ? apiRows : fallbackData
  const usingApiData = apiRows.length > 0

  function handleGanttSelect(selectedMachine: string, selectedSn: string, startTime: string, endTime: string) {
    setMachineId(selectedMachine)
    setSn(selectedSn)
    setTimeRange(`2026-05-27 ${startTime} - ${endTime}`)
    setGanttOpen(false)
  }

  return (
    <ConfigProvider
      locale={zhCN}
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
      <div className="history-page prototype-history-page">
        <div className="history-ambient-background" aria-hidden="true">
          <div className="history-orb history-orb-1" />
          <div className="history-orb history-orb-2" />
          <div className="history-orb history-orb-3" />
          <div className="history-noise" />
        </div>

        <header className="history-toolbar prototype-history-toolbar">
          <div className="history-title-row">
            <h1>{t('history.title')}</h1>
            <div className="history-status-group">
              <button className="glass-btn history-primary-btn" onClick={() => setGanttOpen(true)}>
                <ActivitySquare size={14} />
                {t('history.actions.timeline')}
              </button>
              <div className="history-divider" />
              <button className="glass-btn">
                <Server size={14} className="history-muted-icon" />
                {machineId}
              </button>
              <button className="glass-btn">
                <Cpu size={14} className="history-muted-icon" />
                {sn}
              </button>
              <button className="glass-btn history-time-btn">
                <Calendar size={14} className="history-muted-icon" />
                {timeRange}
              </button>
              <button className="glass-btn history-accent-btn">
                <Search size={14} />
                {t('actions.search')}
              </button>
            </div>
          </div>

          <div className="history-action-row">
            {showAdvanced ? (
              <div className="history-advanced-group">
                <label className="glass-btn history-input-btn">
                  <ArrowUpToLine size={14} className="history-muted-icon" />
                  <span>{t('history.actions.upper')}</span>
                  <input defaultValue="55.0" />
                </label>
                <label className="glass-btn history-input-btn">
                  <ArrowDownToLine size={14} className="history-muted-icon" />
                  <span>{t('history.actions.lower')}</span>
                  <input defaultValue="10.0" />
                </label>
                <button className="glass-btn">
                  <SlidersHorizontal size={14} className="history-muted-icon" />
                  {t('history.actions.limitLine')}
                </button>
                <div className="history-divider" />
              </div>
            ) : null}
            <button className={showAdvanced ? 'glass-btn history-active-btn' : 'glass-btn'} onClick={() => setShowAdvanced((value) => !value)}>
              <ListFilter size={14} className="history-muted-icon" />
              {showAdvanced ? t('history.actions.collapse') : t('history.actions.advanced')}
            </button>
            <div className="history-divider" />
            <button className="glass-btn">
              <Download size={14} className="history-muted-icon" />
              {t('history.actions.exportImage')}
            </button>
            <button className="glass-btn">
              <Download size={14} className="history-muted-icon" />
              {t('history.actions.exportReport')}
            </button>
          </div>
        </header>

        <main className="history-content">
          {historyQuery.isError ? (
            <Alert className="history-api-alert" type="warning" showIcon message={t('history.dataSource.apiUnavailable')} />
          ) : null}
          <section className="history-glass-panel history-chart-panel">
            <div className="history-chart-note">
              <span>* {t('history.chartNote')}</span>
              <span className={usingApiData ? 'history-data-source live' : 'history-data-source sample'}>
                {usingApiData ? t('history.dataSource.api') : t('history.dataSource.sample')}
              </span>
            </div>
            <div className="history-chart-body">
              <TrendChart data={data} selectedMetrics={selectedMetrics} />
            </div>
          </section>

          <section className="history-glass-panel history-table-panel">
            <HistoryTable data={data} />
          </section>
        </main>

        <GanttChartModal isOpen={isGanttOpen} onClose={() => setGanttOpen(false)} onSelect={handleGanttSelect} />
      </div>
    </ConfigProvider>
  )
}
