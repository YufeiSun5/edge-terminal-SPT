import { useMemo, useState, useRef, useEffect } from 'react'
import { Empty, Input, InputNumber, Select, Switch } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TrendChart } from './TrendChart'
import { HistoryTable } from './HistoryTable'
import { getHistoryData } from '../api'
import { defaultSelectedMetrics, historyItemsToSeries } from '../model'
import { SlidersHorizontal, Database, Download, ArrowUpToLine, ArrowDownToLine } from 'lucide-react'

export function DetailedDataTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const [selectedMetrics, setSelectedMetrics] = useState<string[]>([])

  const [showAdvanced, setShowAdvanced] = useState(false)
  const [advMinMax, setAdvMinMax] = useState(true)
  const [axisLimitEnabled, setAxisLimitEnabled] = useState(false)
  const [axisLowerLimit, setAxisLowerLimit] = useState<number | null>(null)
  const [axisUpperLimit, setAxisUpperLimit] = useState<number | null>(null)

  // Mocks for local filtering
  const [startText, setStartText] = useState('')
  const [endText, setEndText] = useState('')

  const historyQuery = useQuery({
    queryKey: ['history', 'data', taskId, startText, endText],
    queryFn: () =>
      getHistoryData({
        task_id: taskId,
        start: startText || undefined,
        end: endText || undefined,
        limit: 5000,
      }),
    refetchInterval: 30000,
    retry: false,
  })

  const series = useMemo(() => historyItemsToSeries(historyQuery.data?.items ?? []), [historyQuery.data?.items])
  const numericMetrics = useMemo(() => series.metrics.filter((metric) => metric.isNumeric), [series.metrics])

  const activeSelectedMetrics = useMemo(() => {
    const allowed = new Set(numericMetrics.map((metric) => metric.key))
    const kept = selectedMetrics.filter((metric) => allowed.has(metric))
    return kept.length > 0 ? kept : defaultSelectedMetrics(numericMetrics)
  }, [numericMetrics, selectedMetrics])

  const chartReady = series.rows.length > 0 && activeSelectedMetrics.length > 0

  // Layout gears logic
  type LayoutMode = 'chart-only' | 'split' | 'table-only'
  const [layoutMode, setLayoutMode] = useState<'chart-only' | 'split' | 'table-only'>('split')
  const dragRef = useRef<{ startY: number; startMode: LayoutMode; hasMoved: boolean }>({
    startY: 0,
    startMode: 'split',
    hasMoved: false,
  })

  const handleMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    dragRef.current = { startY: e.clientY, startMode: layoutMode, hasMoved: false }
    document.body.style.userSelect = 'none'
  }

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!dragRef.current.startY) return

      const deltaY = e.clientY - dragRef.current.startY
      if (Math.abs(deltaY) > 5) {
        dragRef.current.hasMoved = true
      }

      const threshold = 60
      let newMode = dragRef.current.startMode

      if (dragRef.current.startMode === 'split') {
        if (deltaY > threshold) newMode = 'chart-only'
        else if (deltaY < -threshold) newMode = 'table-only'
      } else if (dragRef.current.startMode === 'chart-only') {
        if (deltaY < -threshold) newMode = 'split'
      } else if (dragRef.current.startMode === 'table-only') {
        if (deltaY > threshold) newMode = 'split'
      }

      setLayoutMode(newMode)
    }

    const handleMouseUp = () => {
      if (dragRef.current.startY && !dragRef.current.hasMoved) {
        setLayoutMode((prev) => {
          if (prev === 'split') return 'chart-only'
          if (prev === 'chart-only') return 'table-only'
          return 'split'
        })
      }
      dragRef.current.startY = 0
      document.body.style.userSelect = ''
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [layoutMode])

  return (
    <div className="history-tab-content history-detailed-data-tab">
      <div className="history-detailed-action-bar">
        <div className="history-action-left">
          <Input
            className="history-filter-input"
            prefix={<ArrowUpToLine size={14} />}
            placeholder={t('history.filters.start')}
            value={startText}
            onChange={(event) => setStartText(event.target.value)}
          />
          <Input
            className="history-filter-input"
            prefix={<ArrowDownToLine size={14} />}
            placeholder={t('history.filters.end')}
            value={endText}
            onChange={(event) => setEndText(event.target.value)}
          />
          <Select
            className="history-metric-select"
            mode="multiple"
            maxTagCount="responsive"
            placeholder={t('history.filters.metrics')}
            value={activeSelectedMetrics}
            onChange={setSelectedMetrics}
            options={numericMetrics.map((metric) => ({ value: metric.key, label: metric.title }))}
          />
        </div>
        <div className="history-action-right">
          <div className={showAdvanced ? 'history-inline-advanced open' : 'history-inline-advanced'}
            aria-hidden={!showAdvanced}
          >
            <div className="history-axis-limit-row">
              <span>{t('history.detail.data.setAxisLimits')}</span>
              <Switch size="small" checked={axisLimitEnabled} onChange={setAxisLimitEnabled} />
            </div>
            <div className="history-axis-limit-inputs">
              <InputNumber
                size="small"
                placeholder={t('history.actions.lower')}
                value={axisLowerLimit}
                disabled={!axisLimitEnabled}
                onChange={(value) => setAxisLowerLimit(typeof value === 'number' ? value : null)}
              />
              <InputNumber
                size="small"
                placeholder={t('history.actions.upper')}
                value={axisUpperLimit}
                disabled={!axisLimitEnabled}
                onChange={(value) => setAxisUpperLimit(typeof value === 'number' ? value : null)}
              />
            </div>
            <div className="history-axis-limit-row">
              <span>{t('history.detail.data.showExtremes')}</span>
              <Switch size="small" checked={advMinMax} onChange={setAdvMinMax} />
            </div>
          </div>
          <button
            className={showAdvanced ? 'glass-btn history-active-btn' : 'glass-btn'}
            onClick={() => setShowAdvanced((value) => !value)}
            aria-expanded={showAdvanced}
          >
            <SlidersHorizontal size={14} className="history-muted-icon" />
            {t('history.actions.advanced')}
          </button>
          <button className="glass-btn">
            <Database size={14} className="history-muted-icon" />
            {t('history.actions.storageSnapshot')}
          </button>
          <button className="glass-btn">
            <Download size={14} className="history-muted-icon" />
            {t('history.actions.exportImage')}
          </button>
          <button className="glass-btn">
            <Download size={14} className="history-muted-icon" />
            {t('history.detail.data.exportDetailedTable')}
          </button>
        </div>
      </div>

      {layoutMode !== 'table-only' && (
        <section className="history-chart-panel" style={{ flex: layoutMode === 'chart-only' ? 1 : '0 0 42%' }}>
          <div className="history-chart-note">
            <span>{t('history.detail.data.sampleRefresh')}</span>
            <span className="history-data-source live">API</span>
          </div>
          <div className="history-chart-body">
            {chartReady ? (
              <TrendChart
                data={series.rows}
                metrics={series.metrics}
                selectedMetrics={activeSelectedMetrics}
                yAxisMode={axisLimitEnabled ? 'manual' : 'auto'}
                yMin={axisLowerLimit}
                yMax={axisUpperLimit}
              />
            ) : (
              <Empty description={historyQuery.isFetching ? t('history.dataSource.loading') : t('history.dataSource.empty')} />
            )}
          </div>
        </section>
      )}

      <div className="history-resize-handle" onMouseDown={handleMouseDown} title={t('history.detail.data.resizeHint')}>
        <div className="history-resize-line" />
      </div>

      {layoutMode !== 'chart-only' && (
        <HistoryTable
          className="history-detail-table"
          data={series.rows}
          metrics={series.metrics}
          loading={historyQuery.isFetching}
        />
      )}
    </div>
  )
}
