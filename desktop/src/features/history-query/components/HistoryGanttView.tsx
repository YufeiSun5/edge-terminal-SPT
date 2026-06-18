import { Spin, Empty, Segmented, Tooltip } from 'antd'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TaskLane } from '../model'

type GanttLayoutMode = 'compact' | 'realtime'

type HistoryGanttViewProps = {
  lanes: TaskLane[]
  dataMinTime: number
  dataMaxTime: number
  loading?: boolean
  onSelect: (taskId: number) => void
}

function formatDateMarker(timeMs: number, windowMs: number) {
  const date = new Date(timeMs)
  if (windowMs > 30 * 24 * 3600 * 1000) {
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
  }
  if (windowMs < 24 * 3600 * 1000) {
    return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
  }
  return `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}

function formatCompactDate(timeMs: number) {
  const date = new Date(timeMs)
  return `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

export function HistoryGanttView({ lanes, dataMinTime, dataMaxTime, loading, onSelect }: HistoryGanttViewProps) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)

  const [layoutMode, setLayoutMode] = useState<GanttLayoutMode>('compact')
  const [viewWindow, setViewWindow] = useState<[number, number] | null>(null)
  const dragState = useRef({ isDragging: false, lastX: 0, scrollLeft: 0 })
  const fallbackWindow = useMemo<[number, number]>(() => {
    const defaultWindowMs = 7 * 24 * 3600 * 1000
    const end = dataMaxTime > 0 ? dataMaxTime : defaultWindowMs
    const start = dataMinTime > 0 && dataMinTime < end ? Math.max(dataMinTime, end - defaultWindowMs) : end - defaultWindowMs
    return [start, end]
  }, [dataMaxTime, dataMinTime])
  const [viewStart, viewEnd] = viewWindow || fallbackWindow
  const windowMs = viewEnd - viewStart

  const compactLayout = useMemo(() => {
    const gapThresholdMs = 4 * 60 * 60 * 1000
    const pxPerUnit = 13
    const normalGapUnits = 2
    const longGapUnits = 5
    const layout = new Map<number, { leftPx: number, widthPx: number, gapMs: number }>()
    const markers: { leftPx: number, label: string }[] = []
    const blocks = lanes
      .flatMap((lane) => lane.blocks)
      .filter((block) => block.endMs >= viewStart && block.startMs <= viewEnd)
      .slice()
      .sort((a, b) => a.startMs - b.startMs || a.id - b.id)

    let cursorUnits = 0
    let previousEnd: number | null = null
    let lastMarkerDate = ''

    for (const block of blocks) {
      const gapMs = previousEnd === null ? 0 : Math.max(0, block.startMs - previousEnd)
      if (previousEnd !== null) {
        cursorUnits += gapMs > gapThresholdMs ? longGapUnits : normalGapUnits
      }

      const markerLabel = formatCompactDate(block.startMs)
      if (markerLabel !== lastMarkerDate) {
        markers.push({ leftPx: cursorUnits * pxPerUnit, label: markerLabel })
        lastMarkerDate = markerLabel
      }

      const durationHours = Math.max((block.endMs - block.startMs) / (60 * 60 * 1000), 0.25)
      const widthUnits = clamp(9 + Math.log2(durationHours + 1) * 5, 12, 28)
      layout.set(block.id, {
        leftPx: cursorUnits * pxPerUnit,
        widthPx: widthUnits * pxPerUnit,
        gapMs: gapMs > gapThresholdMs ? gapMs : 0,
      })
      cursorUnits += widthUnits
      previousEnd = Math.max(previousEnd ?? block.endMs, block.endMs)
    }

    return {
      layout,
      markers,
      trackWidth: Math.max(760, Math.ceil(cursorUnits * pxPerUnit) + 48),
    }
  }, [lanes, viewEnd, viewStart])

  useEffect(() => {
    if (layoutMode !== 'compact') return
    const scrollElement = scrollRef.current
    if (!scrollElement) return
    requestAnimationFrame(() => {
      scrollElement.scrollLeft = Math.max(0, scrollElement.scrollWidth - scrollElement.clientWidth)
    })
  }, [compactLayout.trackWidth, layoutMode])

  const handleWheel = (e: React.WheelEvent<HTMLDivElement>) => {
    if (e.deltaY === 0) return
    if (e.cancelable) e.preventDefault()

    const zoomFactor = e.deltaY > 0 ? 1.2 : 1 / 1.2
    const newWindowMs = windowMs * zoomFactor

    const minZoom = 60 * 60 * 1000 // 1 hour
    const maxZoom = 10 * 365 * 24 * 60 * 60 * 1000 // 10 years
    const clampedNewWindowMs = clamp(newWindowMs, minZoom, maxZoom)

    const trackRect = e.currentTarget.querySelector<HTMLElement>('.history-gantt-track')?.getBoundingClientRect()
    const rect = trackRect ?? e.currentTarget.getBoundingClientRect()
    const mousePercent = clamp((e.clientX - rect.left) / Math.max(rect.width, 1), 0, 1)
    const mouseTime = viewStart + windowMs * mousePercent
    const newStart = mouseTime - clampedNewWindowMs * mousePercent
    const newEnd = newStart + clampedNewWindowMs

    setViewWindow([newStart, newEnd])
  }

  const handleMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    dragState.current = {
      isDragging: true,
      lastX: e.clientX,
      scrollLeft: e.currentTarget.scrollLeft,
    }
  }

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!dragState.current.isDragging) return
    const deltaX = e.clientX - dragState.current.lastX
    if (deltaX === 0) return

    if (layoutMode === 'compact') {
      e.currentTarget.scrollLeft = dragState.current.scrollLeft - deltaX
      return
    }

    dragState.current.lastX = e.clientX
    const rect = e.currentTarget.getBoundingClientRect()
    const timeShift = -(deltaX / rect.width) * windowMs

    setViewWindow(prev => {
      const [s, end] = prev || [viewStart, viewEnd]
      return [s + timeShift, end + timeShift]
    })
  }

  const handleMouseUpOrLeave = () => {
    dragState.current.isDragging = false
  }

  const markers = [0, 20, 40, 60, 80, 100].map(percent => {
    const time = viewStart + windowMs * (percent / 100)
    return { percent, label: formatDateMarker(time, windowMs) }
  })

  const formatGap = (gapMs: number) => {
    const totalMinutes = Math.max(1, Math.round(gapMs / 60000))
    const days = Math.floor(totalMinutes / (24 * 60))
    const hours = Math.floor((totalMinutes - days * 24 * 60) / 60)
    const minutes = totalMinutes % 60
    if (days > 0) return t('history.timeline.gapDaysHours', { days, hours })
    if (hours > 0) return t('history.timeline.gapHoursMinutes', { hours, minutes })
    return t('history.timeline.gapMinutes', { minutes })
  }

  return (
    <div className="history-gantt-body">
      <div className="history-gantt-title">
        <div className="history-gantt-legend">
          <span className="history-legend-item mode-standard"><span className="legend-color"></span> {t('history.timeline.standardTest')}</span>
          <span className="history-legend-item mode-free"><span className="legend-color"></span> {t('history.timeline.freeLongRun')}</span>
          <span className="history-legend-item"><span className="status-dot status-running"></span> {t('history.timeline.running')}</span>
          <span className="history-legend-item"><span className="status-dot status-completed"></span> {t('history.timeline.completed')}</span>
          <span className="history-legend-item"><span className="history-gap-sample"></span> {t('history.timeline.compactedGap')}</span>
        </div>
        <Segmented
          size="small"
          value={layoutMode}
          onChange={(value) => setLayoutMode(value as GanttLayoutMode)}
          options={[
            { value: 'compact', label: t('history.timeline.compactMode') },
            { value: 'realtime', label: t('history.timeline.realtimeMode') },
          ]}
        />
      </div>

      <Spin spinning={loading} classNames={{ root: 'history-gantt-spin' }}>
        {lanes.length > 0 ? (
          <div
            className={`history-gantt-scroll-container mode-${layoutMode}`}
            ref={scrollRef}
            onWheel={layoutMode === 'realtime' ? handleWheel : undefined}
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUpOrLeave}
            onMouseLeave={handleMouseUpOrLeave}
          >
            <div className="history-gantt-chart">
              <div className={`history-gantt-axis mode-${layoutMode}`} style={layoutMode === 'compact' ? { width: compactLayout.trackWidth } : undefined}>
                {(layoutMode === 'compact' ? compactLayout.markers : markers).map((marker) => (
                  <span key={`${marker.label}-${'percent' in marker ? marker.percent : marker.leftPx}`} style={'percent' in marker ? { left: `${marker.percent}%` } : { left: marker.leftPx }}>
                    {marker.label}
                  </span>
                ))}
              </div>
              <div className={`history-gantt-lines mode-${layoutMode}`} style={layoutMode === 'compact' ? { width: compactLayout.trackWidth } : undefined} aria-hidden="true">
                {(layoutMode === 'compact' ? compactLayout.markers : markers).map((marker) => (
                  <span key={`${marker.label}-${'percent' in marker ? marker.percent : marker.leftPx}`} style={'percent' in marker ? { left: `${marker.percent}%` } : { left: marker.leftPx }} />
                ))}
              </div>

              <div className="history-gantt-lanes">
                {lanes.map((lane) => (
                  <div className="history-gantt-lane" key={lane.projectCode}>
                    <div className="history-gantt-machine">{lane.projectCode}</div>
                    <div className={`history-gantt-track ${layoutMode === 'compact' ? 'gantt-compact-track' : ''}`} style={layoutMode === 'compact' ? { width: compactLayout.trackWidth } : undefined}>
                      {lane.blocks.map((block) => {
                        const compactBlock = compactLayout.layout.get(block.id)
                        const startPercent = ((block.startMs - viewStart) / windowMs) * 100
                        const widthPercent = ((block.endMs - block.startMs) / windowMs) * 100
                        if (layoutMode === 'realtime' && (startPercent > 100 || startPercent + widthPercent < 0)) return null
                        if (layoutMode === 'compact' && !compactBlock) return null

                        const isRunning = block.status === 'Running' || block.status === '正在检测' || !block.endMs || block.endMs > dataMaxTime
                        const statusClass = isRunning ? 'running' : 'completed'
                        const modeClass = block.mode === 'standard' ? 'standard' : 'free'
                        const blockStyle = layoutMode === 'compact' && compactBlock
                          ? { left: compactBlock.leftPx, width: compactBlock.widthPx }
                          : { left: `${startPercent}%`, width: `${widthPercent}%` }

                        return (
                          <div key={block.id}>
                            {layoutMode === 'compact' && compactBlock?.gapMs ? (
                              <Tooltip title={t('history.timeline.gapTooltip', { gap: formatGap(compactBlock.gapMs) })}>
                                <span className="history-gantt-gap-marker" style={{ left: Math.max(0, compactBlock.leftPx - 38) }} />
                              </Tooltip>
                            ) : null}
                            <button
                              className={`history-gantt-block block-mode-${modeClass}`}
                              style={blockStyle}
                              title={`${block.testNo} ${block.startStr}-${block.endStr} ${block.status}`}
                              onClick={() => onSelect(block.id)}
                              onMouseDown={(e) => e.stopPropagation()}
                            >
                              <span className={`status-dot status-${statusClass}`} />
                              <span>{block.testNo}</span>
                            </button>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        ) : (
          <div className="history-gantt-empty">
            <Empty description={t('history.timeline.empty')} />
          </div>
        )}
      </Spin>
    </div>
  )
}
