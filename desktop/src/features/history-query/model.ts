import type { HistoryDataItem } from '@/shared/api/types'

export type HistoryRow = {
  id: number
  time: string
  tempOut: number
  humidOut: number
  tempIn: number
  humidIn: number
  windIn: number
  noise: number
  pressure: number
  power: number
  vibration: number
} & Record<`var${number}`, number>

type HistoryMetricKey = Exclude<keyof HistoryRow, 'id' | 'time'>

const historyMetricMap: Record<string, HistoryMetricKey> = {
  supply_air_temp: 'tempOut',
  temp: 'tempOut',
  tempOut: 'tempOut',
  supply_air_humidity: 'humidOut',
  humidity: 'humidOut',
  humidOut: 'humidOut',
  inlet_air_temp: 'tempIn',
  tempIn: 'tempIn',
  inlet_air_humidity: 'humidIn',
  humidIn: 'humidIn',
  inlet_airflow: 'windIn',
  windIn: 'windIn',
  noise: 'noise',
  pressure: 'pressure',
  power: 'power',
  vibration: 'vibration',
}

export type TaskBlock = {
  id: string
  startStr: string
  endStr: string
  startPercent: number
  widthPercent: number
  sn: string
}

export type TaskLane = {
  machineId: string
  blocks: TaskBlock[]
}

export function generateHistoryData(): HistoryRow[] {
  return Array.from({ length: 1000 }).map((_, index) => {
    const date = new Date(2026, 4, 27, 0, 0, 0)
    date.setMinutes(date.getMinutes() + index)
    const row = {
      id: index,
      time: `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`,
      tempOut: +(48 + Math.random() * 5).toFixed(2),
      humidOut: +(30 + Math.random() * 10).toFixed(2),
      tempIn: +(22 + Math.random() * 3).toFixed(2),
      humidIn: +(40 + Math.random() * 10).toFixed(2),
      windIn: +(140 + Math.random() * 20).toFixed(2),
      noise: +(60 + Math.random() * 10).toFixed(2),
      pressure: +(120 + Math.random() * 15).toFixed(2),
      power: +(50 + Math.random() * 8).toFixed(2),
      vibration: +(1.2 + Math.random() * 0.5).toFixed(3),
    } as HistoryRow

    for (let item = 1; item <= 33; item += 1) {
      row[`var${item}`] = +(Math.random() * 100).toFixed(2)
    }
    return row
  })
}

export function historyItemsToRows(items: HistoryDataItem[]): HistoryRow[] {
  const byTime = new Map<string, HistoryRow>()
  const dynamicMetricIndex = new Map<string, `var${number}`>()

  for (const item of items) {
    const date = new Date(item.source_time)
    const timeKey = Number.isNaN(date.getTime()) ? item.source_time : date.toISOString()
    const label = Number.isNaN(date.getTime())
      ? item.source_time
      : `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
    let row = byTime.get(timeKey)
    if (!row) {
      row = {
        id: byTime.size,
        time: label,
        tempOut: 0,
        humidOut: 0,
        tempIn: 0,
        humidIn: 0,
        windIn: 0,
        noise: 0,
        pressure: 0,
        power: 0,
        vibration: 0,
      } as HistoryRow
      byTime.set(timeKey, row)
    }

    const numericValue = typeof item.value === 'number' ? item.value : Number(item.str_value)
    if (Number.isNaN(numericValue)) continue

    const mappedKey = historyMetricMap[item.var_name] ?? dynamicMetricIndex.get(item.var_name)
    if (mappedKey) {
      row[mappedKey] = numericValue
      continue
    }

    const nextIndex = dynamicMetricIndex.size + 1
    if (nextIndex <= 33) {
      const dynamicKey = `var${nextIndex}` as `var${number}`
      dynamicMetricIndex.set(item.var_name, dynamicKey)
      row[dynamicKey] = numericValue
    }
  }

  return Array.from(byTime.values())
}

export function buildGanttData(): TaskLane[] {
  const formatTime = (value: number) => {
    const hour = Math.floor(value)
    const minute = Math.floor((value - hour) * 60)
    return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
  }

  return Array.from({ length: 12 }).map((_, laneIndex) => {
    const blocks: TaskBlock[] = []
    let currentHour = 0
    const blockCount = (laneIndex % 3) + 2

    for (let blockIndex = 0; blockIndex < blockCount; blockIndex += 1) {
      const gap = 0.7 + ((laneIndex + blockIndex) % 4) * 0.6
      const startHour = currentHour + gap
      if (startHour >= 22) break

      const duration = Math.min(1.4 + ((laneIndex + blockIndex) % 5) * 0.42, 24 - startHour)
      const endHour = startHour + duration
      blocks.push({
        id: `m${laneIndex + 1}-b${blockIndex}`,
        startStr: formatTime(startHour),
        endStr: formatTime(endHour),
        startPercent: (startHour / 24) * 100,
        widthPercent: (duration / 24) * 100,
        sn: `A-10${(laneIndex + blockIndex) % 9}`,
      })
      currentHour = endHour
    }

    return {
      machineId: `测试机 ${laneIndex + 1}`,
      blocks,
    }
  })
}
