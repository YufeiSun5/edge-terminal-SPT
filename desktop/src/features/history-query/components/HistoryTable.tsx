import { useLayoutEffect, useRef, useState } from 'react'
import { Table } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import type { HistoryMetricColumn, HistorySeriesRow } from '../model'

type HistoryTableProps = {
  data: HistorySeriesRow[]
  metrics: HistoryMetricColumn[]
  loading?: boolean
  className?: string
}

export function HistoryTable({ data, metrics, loading, className }: HistoryTableProps) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const [scrollY, setScrollY] = useState(160)
  const columns: TableColumnsType<HistorySeriesRow> = [
    { title: t('history.table.time'), dataIndex: 'time', key: 'time', fixed: 'left', width: 150, align: 'center' },
    ...metrics.map((metric) => ({
      title: metric.title,
      dataIndex: metric.key,
      key: metric.key,
      width: Math.max(150, Math.min(260, metric.title.length * 12)),
      align: 'center' as const,
      render: (value: number | string | null | undefined) => value ?? '--',
    })),
  ]

  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container) return

    const updateHeight = () => {
      const containerHeight = container.clientHeight
      const paginationHeight = container.querySelector('.ant-pagination')?.getBoundingClientRect().height ?? 40
      const headerReserve = 48
      const gapReserve = 12
      setScrollY(Math.max(48, Math.floor(containerHeight - paginationHeight - headerReserve - gapReserve)))
    }

    updateHeight()
    const observer = new ResizeObserver(updateHeight)
    observer.observe(container)
    return () => observer.disconnect()
  }, [])

  return (
    <div ref={containerRef} className={className ? `history-table-wrap ${className}` : 'history-table-wrap'}>
      <Table
        className="antd-custom-table"
        columns={columns}
        dataSource={data}
        loading={loading}
        rowKey="source_time"
        rowClassName={(_, index) => (index % 2 === 0 ? 'row-even' : 'row-odd')}
        scroll={{ x: 'max-content', y: scrollY }}
        virtual
        pagination={{
          defaultPageSize: 100,
          showSizeChanger: true,
          pageSizeOptions: ['100', '200', '500'],
          showTotal: (total) => t('history.table.total', { total }),
          size: 'small',
          style: { marginTop: 8, marginBottom: 0 },
        }}
        size="small"
        bordered={false}
      />
    </div>
  )
}
