import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { Empty, Spin } from 'antd'
import { useTranslation } from 'react-i18next'
import { PlayCircle, X } from 'lucide-react'
import type { TaskLane } from '../model'

type GanttChartModalProps = {
  isOpen: boolean
  lanes: TaskLane[]
  loading?: boolean
  onClose: () => void
  onSelect: (taskId: number) => void
}

export function GanttChartModal({ isOpen, lanes, loading, onClose, onSelect }: GanttChartModalProps) {
  const { t } = useTranslation()
  useEffect(() => {
    document.body.style.overflow = isOpen ? 'hidden' : ''
    return () => {
      document.body.style.overflow = ''
    }
  }, [isOpen])

  if (!isOpen) return null

  return createPortal(
    <div className="history-gantt-root">
      <button className="history-gantt-backdrop" onClick={onClose} aria-label="关闭任务甘特图" />
      <div className="history-gantt-dialog">
        <button className="history-gantt-close" onClick={onClose} aria-label="关闭">
          <X size={18} />
        </button>

        <div className="history-gantt-title">
          <h2>{t('history.timeline.title')}</h2>
          <p>{t('history.timeline.hint')}</p>
        </div>

        <Spin spinning={loading}>
          {lanes.length > 0 ? (
            <div className="history-gantt-chart">
              <div className="history-gantt-axis">
                {[0, 20, 40, 60, 80, 100].map((point) => (
                  <span key={point} style={{ left: `${point}%` }}>
                    {point}%
                  </span>
                ))}
              </div>
              <div className="history-gantt-lines" aria-hidden="true">
                {[0, 20, 40, 60, 80, 100].map((point) => (
                  <span key={point} style={{ left: `${point}%` }} />
                ))}
              </div>

              <div className="history-gantt-lanes">
                {lanes.map((lane) => (
                  <div className="history-gantt-lane" key={lane.projectCode}>
                    <div className="history-gantt-machine">{lane.projectCode}</div>
                    <div className="history-gantt-track">
                      {lane.blocks.map((block) => (
                        <button
                          key={block.id}
                          className="history-gantt-block"
                          style={{ left: `${block.startPercent}%`, width: `${block.widthPercent}%` }}
                          title={`${block.testNo} ${block.startStr}-${block.endStr} ${block.status}`}
                          onClick={() => onSelect(block.id)}
                        >
                          <PlayCircle size={12} />
                          <span>{block.testNo}</span>
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <Empty description={t('history.timeline.empty')} />
          )}
        </Spin>
      </div>
    </div>,
    document.body,
  )
}
