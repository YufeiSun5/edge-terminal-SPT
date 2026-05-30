import { Area, AreaChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { HistoryRow } from '../model'

type TrendChartProps = {
  data: HistoryRow[]
  selectedMetrics: string[]
}

const colorMap = {
  tempOut: '#ff4d4f',
  humidOut: '#13c2c2',
  pressure: '#1677ff',
} as const

const nameMap = {
  tempOut: '吹出口温度',
  humidOut: '吹出口湿度',
  pressure: '系统压力',
} as const

export function TrendChart({ data, selectedMetrics }: TrendChartProps) {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data} margin={{ top: 10, right: 30, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id="colorTemp" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={colorMap.tempOut} stopOpacity={0.2} />
            <stop offset="95%" stopColor={colorMap.tempOut} stopOpacity={0} />
          </linearGradient>
          <linearGradient id="colorHumid" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={colorMap.humidOut} stopOpacity={0.2} />
            <stop offset="95%" stopColor={colorMap.humidOut} stopOpacity={0} />
          </linearGradient>
          <linearGradient id="colorPressure" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={colorMap.pressure} stopOpacity={0.2} />
            <stop offset="95%" stopColor={colorMap.pressure} stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="#e8e8e8" vertical={false} />
        <XAxis
          dataKey="time"
          stroke="#8c8c8c"
          tick={{ fill: '#666', fontSize: 12 }}
          axisLine={{ stroke: '#e8e8e8' }}
          tickLine={false}
        />
        <YAxis
          yAxisId="left"
          stroke="#8c8c8c"
          tick={{ fill: '#666', fontSize: 12 }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          yAxisId="right"
          orientation="right"
          stroke="#8c8c8c"
          tick={{ fill: '#666', fontSize: 12 }}
          axisLine={false}
          tickLine={false}
        />
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

        {selectedMetrics.includes('tempOut') ? (
          <Area yAxisId="left" type="monotone" dataKey="tempOut" name={nameMap.tempOut} stroke={colorMap.tempOut} strokeWidth={2} fillOpacity={1} fill="url(#colorTemp)" activeDot={{ r: 4, strokeWidth: 0 }} />
        ) : null}
        {selectedMetrics.includes('humidOut') ? (
          <Area yAxisId="right" type="monotone" dataKey="humidOut" name={nameMap.humidOut} stroke={colorMap.humidOut} strokeWidth={2} fillOpacity={1} fill="url(#colorHumid)" activeDot={{ r: 4, strokeWidth: 0 }} />
        ) : null}
        {selectedMetrics.includes('pressure') ? (
          <Area yAxisId="left" type="monotone" dataKey="pressure" name={nameMap.pressure} stroke={colorMap.pressure} strokeWidth={2} fillOpacity={1} fill="url(#colorPressure)" activeDot={{ r: 4, strokeWidth: 0 }} />
        ) : null}
      </AreaChart>
    </ResponsiveContainer>
  )
}
