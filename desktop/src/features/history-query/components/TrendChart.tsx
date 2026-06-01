import { Area, AreaChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { HistoryMetricColumn, HistorySeriesRow } from '../model'

type TrendChartProps = {
  data: HistorySeriesRow[]
  metrics: HistoryMetricColumn[]
  selectedMetrics: string[]
}

const colors = ['#1677ff', '#13c2c2', '#ff4d4f', '#722ed1', '#fa8c16', '#52c41a']

export function TrendChart({ data, metrics, selectedMetrics }: TrendChartProps) {
  const selected = metrics.filter((metric) => metric.isNumeric && selectedMetrics.includes(metric.key))

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data} margin={{ top: 10, right: 30, left: 0, bottom: 0 }}>
        <defs>
          {selected.map((metric, index) => (
            <linearGradient id={`historyMetric${index}`} key={metric.key} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={colors[index % colors.length]} stopOpacity={0.2} />
              <stop offset="95%" stopColor={colors[index % colors.length]} stopOpacity={0} />
            </linearGradient>
          ))}
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="#e8e8e8" vertical={false} />
        <XAxis
          dataKey="time"
          stroke="#8c8c8c"
          tick={{ fill: '#666', fontSize: 12 }}
          axisLine={{ stroke: '#e8e8e8' }}
          tickLine={false}
        />
        <YAxis stroke="#8c8c8c" tick={{ fill: '#666', fontSize: 12 }} axisLine={false} tickLine={false} />
        <Tooltip
          contentStyle={{
            backgroundColor: 'rgba(255, 255, 255, 0.95)',
            backdropFilter: 'blur(10px)',
            border: '1px solid #e8e8e8',
            borderRadius: '8px',
            color: '#333',
            boxShadow: '0 6px 16px rgba(0,0,0,0.08)',
          }}
          itemStyle={{ color: '#333', fontWeight: 500 }}
          labelStyle={{ color: '#666', marginBottom: '8px' }}
        />
        <Legend verticalAlign="top" height={36} iconType="circle" wrapperStyle={{ fontSize: '13px' }} />
        {selected.map((metric, index) => (
          <Area
            key={metric.key}
            type="monotone"
            dataKey={metric.key}
            name={metric.title}
            stroke={colors[index % colors.length]}
            strokeWidth={2}
            fillOpacity={1}
            fill={`url(#historyMetric${index})`}
            activeDot={{ r: 4, strokeWidth: 0 }}
            connectNulls
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  )
}
