import { Table } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import type { HistoryMetricColumn, HistorySeriesRow } from '../model'

type HistoryTableProps = {
  data: HistorySeriesRow[]
  metrics: HistoryMetricColumn[]
  loading?: boolean
}

export function HistoryTable({ data, metrics, loading }: HistoryTableProps) {
  const { t } = useTranslation()
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

  return (
    <div className="history-table-wrap">
      <Table
        className="antd-custom-table"
        columns={columns}
        dataSource={data}
        loading={loading}
        rowKey="id"
        rowClassName={(_, index) => (index % 2 === 0 ? 'row-even' : 'row-odd')}
        scroll={{ x: 'max-content', y: 'calc(100cqh - 80px)' }}
        virtual
        pagination={{
          defaultPageSize: 100,
          showSizeChanger: true,
          pageSizeOptions: ['100', '200', '500'],
          showTotal: (total) => t('history.table.total', { total }),
          size: 'small',
          style: { marginTop: '12px', marginBottom: 0 },
        }}
        size="small"
        bordered={false}
      />
    </div>
  )
}
