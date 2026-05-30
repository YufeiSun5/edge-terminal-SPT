import { Table } from 'antd'
import type { TableColumnsType } from 'antd'
import type { HistoryRow } from '../model'

type HistoryTableProps = {
  data: HistoryRow[]
}

export function HistoryTable({ data }: HistoryTableProps) {
  const columns: TableColumnsType<HistoryRow> = [
    { title: '时间戳', dataIndex: 'time', key: 'time', fixed: 'left', width: 100, align: 'center' },
    { title: '吹出口温度(℃)', dataIndex: 'tempOut', key: 'tempOut', width: 140, align: 'center' },
    { title: '吹出口湿度(%RH)', dataIndex: 'humidOut', key: 'humidOut', width: 150, align: 'center' },
    { title: '吸入口温度(℃)', dataIndex: 'tempIn', key: 'tempIn', width: 140, align: 'center' },
    { title: '吸入口湿度(%RH)', dataIndex: 'humidIn', key: 'humidIn', width: 150, align: 'center' },
    { title: '吸入风量(m³/h)', dataIndex: 'windIn', key: 'windIn', width: 140, align: 'center' },
    { title: '设备噪音(dB)', dataIndex: 'noise', key: 'noise', width: 130, align: 'center' },
    { title: '系统压力(kPa)', dataIndex: 'pressure', key: 'pressure', width: 130, align: 'center' },
    { title: '设备功率(kW)', dataIndex: 'power', key: 'power', width: 130, align: 'center' },
    { title: '震动位移(mm)', dataIndex: 'vibration', key: 'vibration', width: 130, align: 'center' },
  ]

  for (let index = 1; index <= 32; index += 1) {
    columns.push({
      title: `测试指标 V${index}`,
      dataIndex: `var${index}`,
      key: `var${index}`,
      width: 120,
      align: 'center',
    })
  }

  return (
    <div className="history-table-wrap">
      <Table
        className="antd-custom-table"
        columns={columns}
        dataSource={data}
        rowKey="id"
        rowClassName={(_, index) => (index % 2 === 0 ? 'row-even' : 'row-odd')}
        scroll={{ x: 'max-content', y: 'calc(100cqh - 80px)' }}
        virtual
        pagination={{
          defaultPageSize: 100,
          showSizeChanger: true,
          pageSizeOptions: ['100', '200', '500'],
          showTotal: (total) => `共 ${total} 条数据`,
          size: 'small',
          style: { marginTop: '12px', marginBottom: 0 },
        }}
        size="small"
        bordered={false}
      />
    </div>
  )
}
