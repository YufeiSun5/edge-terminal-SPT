import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { PlayCircle, X } from 'lucide-react'
import { buildGanttData } from '../model'

type GanttChartModalProps = {
  isOpen: boolean
  onClose: () => void
  onSelect: (machineId: string, sn: string, startTime: string, endTime: string) => void
}

export function GanttChartModal({ isOpen, onClose, onSelect }: GanttChartModalProps) {
  const [lanes] = useState(() => buildGanttData())

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
          <h2>Mission Timeline.</h2>
          <p>点击任意任务胶囊，即可将该时段及设备参数同步至主界面查询面板</p>
        </div>

        <div className="history-gantt-chart">
          <div className="history-gantt-axis">
            {[0, 4, 8, 12, 16, 20, 24].map((hour) => (
              <span key={hour} style={{ left: `${(hour / 24) * 100}%` }}>
                {hour}:00
              </span>
            ))}
          </div>
          <div className="history-gantt-lines" aria-hidden="true">
            {[0, 4, 8, 12, 16, 20, 24].map((hour) => (
              <span key={hour} style={{ left: `${(hour / 24) * 100}%` }} />
            ))}
          </div>

          <div className="history-gantt-lanes">
            {lanes.map((lane) => (
              <div className="history-gantt-lane" key={lane.machineId}>
                <div className="history-gantt-machine">{lane.machineId}</div>
                <div className="history-gantt-track">
                  {lane.blocks.map((block) => (
                    <button
                      key={block.id}
                      className="history-gantt-block"
                      style={{ left: `${block.startPercent}%`, width: `${block.widthPercent}%` }}
                      onClick={() => onSelect(lane.machineId, block.sn, block.startStr, block.endStr)}
                    >
                      <PlayCircle size={12} />
                      <span>{block.sn}</span>
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  )
}
