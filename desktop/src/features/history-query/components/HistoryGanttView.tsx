import { Spin, Empty } from 'antd'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TaskLane } from '../model'

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

export function HistoryGanttView({ lanes, dataMinTime, dataMaxTime, loading, onSelect }: HistoryGanttViewProps) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)

  const [viewWindow, setViewWindow] = useState<[number, number] | null>(null)
  const dragState = useRef({ isDragging: false, lastX: 0 })
  const fallbackWindow = useMemo<[number, number]>(() => {
    const defaultWindowMs = 7 * 24 * 3600 * 1000
    const end = dataMaxTime > 0 ? dataMaxTime : defaultWindowMs
    const start = dataMinTime > 0 && dataMinTime < end ? Math.max(dataMinTime, end - defaultWindowMs) : end - defaultWindowMs
    return [start, end]
  }, [dataMaxTime, dataMinTime])
  const [viewStart, viewEnd] = viewWindow || fallbackWindow
  const windowMs = viewEnd - viewStart

  const handleWheel = (e: React.WheelEvent<HTMLDivElement>) => {
    if (e.deltaY === 0) return
    e.preventDefault()

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
    dragState.current = { isDragging: true, lastX: e.clientX }
  }

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!dragState.current.isDragging) return
    const deltaX = e.clientX - dragState.current.lastX
    if (deltaX === 0) return

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

  return (
    <div className="history-gantt-body">
      <div className="history-gantt-title">
        <div className="history-gantt-legend">
          <span className="history-legend-item mode-standard"><span className="legend-color"></span> {t('history.timeline.standardTest')}</span>
          <span className="history-legend-item mode-free"><span className="legend-color"></span> {t('history.timeline.freeLongRun')}</span>
          <span className="history-legend-item"><span className="status-dot status-running"></span> {t('history.timeline.running')}</span>
          <span className="history-legend-item"><span className="status-dot status-completed"></span> {t('history.timeline.completed')}</span>
        </div>
      </div>

      <Spin spinning={loading} wrapperClassName="history-gantt-spin">
        {lanes.length > 0 ? (
          <div
            className="history-gantt-scroll-container"
            ref={scrollRef}
            onWheel={handleWheel}
            onMouseDown={handleMouseDown}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUpOrLeave}
            onMouseLeave={handleMouseUpOrLeave}
          >
            <div className="history-gantt-chart">
              <div className="history-gantt-axis">
                {markers.map((marker) => (
                  <span key={marker.percent} style={{ left: `${marker.percent}%` }}>
                    {marker.label}
                  </span>
                ))}
              </div>
              <div className="history-gantt-lines" aria-hidden="true">
                {markers.map((marker) => (
                  <span key={marker.percent} style={{ left: `${marker.percent}%` }} />
                ))}
              </div>

              <div className="history-gantt-lanes">
                {lanes.map((lane) => (
                  <div className="history-gantt-lane" key={lane.projectCode}>
                    <div className="history-gantt-machine">{lane.projectCode}</div>
                    <div className="history-gantt-track">
                      {lane.blocks.map((block) => {
                        const startPercent = ((block.startMs - viewStart) / windowMs) * 100
                        const widthPercent = ((block.endMs - block.startMs) / windowMs) * 100
                        if (startPercent > 100 || startPercent + widthPercent < 0) return null

                        const isRunning = block.status === 'Running' || block.status === '正在检测' || !block.endMs || block.endMs > dataMaxTime
                        const statusClass = isRunning ? 'running' : 'completed'
                        const modeClass = block.mode === 'standard' ? 'standard' : 'free'

                        return (
                          <button
                            key={block.id}
                            className={`history-gantt-block block-mode-${modeClass}`}
                            style={{ left: `${startPercent}%`, width: `${widthPercent}%` }}
                            title={`${block.testNo} ${block.startStr}-${block.endStr} ${block.status}`}
                            onClick={() => onSelect(block.id)}
                            onMouseDown={(e) => e.stopPropagation()}
                          >
                            <span className={`status-dot status-${statusClass}`} />
                            <span>{block.testNo}</span>
                          </button>
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
