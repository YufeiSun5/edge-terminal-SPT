import type { DetectionRun, HistoryDataItem } from '@/shared/api/types'

export type HistorySeriesRow = {
  id: number
  time: string
  source_time: string
} & Record<string, number | string | null>

export type HistoryMetricColumn = {
  key: string
  title: string
  varId: string
  varName: string
  isNumeric: boolean
}

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
} & Record<string, number | string>

export type GanttBlock = {
  id: number
  sn: string
  startStr: string
  endStr: string
  startPercent: number
  widthPercent: number
}

export type GanttLane = {
  machineId: string
  blocks: GanttBlock[]
}

export type TaskBlock = {
  id: number
  testNo: string
  projectCode: string
  status: string
  mode: string
  startStr: string
  endStr: string
  startMs: number
  endMs: number
}

export type TaskLane = {
  projectCode: string
  blocks: TaskBlock[]
}

export function historyItemsToSeries(items: HistoryDataItem[]): {
  rows: HistorySeriesRow[]
  metrics: HistoryMetricColumn[]
} {
  const byTime = new Map<string, HistorySeriesRow>()
  const metrics = new Map<string, HistoryMetricColumn>()

  for (const item of items) {
    const metricKey = metricKeyFor(item)
    const numericValue = typeof item.value === 'number' ? item.value : Number(item.str_value)
    const isNumeric = item.value !== undefined && item.value !== null && Number.isFinite(numericValue)
    if (!metrics.has(metricKey)) {
      metrics.set(metricKey, {
        key: metricKey,
        title: metricTitleFor(item),
        varId: String(item.var_id_text ?? item.var_id),
        varName: item.var_name,
        isNumeric,
      })
    } else if (isNumeric) {
      metrics.get(metricKey)!.isNumeric = true
    }

    const sourceTime = item.source_time || item.created_at
    const date = new Date(sourceTime)
    const timeKey = Number.isNaN(date.getTime()) ? sourceTime : date.toISOString()
    const row = byTime.get(timeKey) ?? {
      id: byTime.size,
      time: formatHistoryTime(sourceTime),
      source_time: sourceTime,
    }
    row[metricKey] = isNumeric ? numericValue : item.str_value ?? null
    byTime.set(timeKey, row)
  }

  return {
    rows: Array.from(byTime.values()),
    metrics: Array.from(metrics.values()),
  }
}

export function defaultSelectedMetrics(metrics: HistoryMetricColumn[]) {
  return metrics.filter((metric) => metric.isNumeric).slice(0, 3).map((metric) => metric.key)
}

export function historyItemsToRows(items: HistoryDataItem[]): HistoryRow[] {
  return historyItemsToSeries(items).rows.map((row, index) => ({
    ...row,
    id: index,
    time: row.time,
    tempOut: numberFromRow(row, ['var_tempOut', 'tempOut'], 48.6),
    humidOut: numberFromRow(row, ['var_humidOut', 'humidOut'], 33.2),
    tempIn: numberFromRow(row, ['var_tempIn', 'tempIn'], 22.3),
    humidIn: numberFromRow(row, ['var_humidIn', 'humidIn'], 45.5),
    windIn: numberFromRow(row, ['var_windIn', 'windIn'], 138.6),
    noise: numberFromRow(row, ['var_noise', 'noise'], 45.7),
    pressure: numberFromRow(row, ['var_pressure', 'pressure'], 120),
    power: numberFromRow(row, ['var_power', 'power'], 2.45),
    vibration: numberFromRow(row, ['var_vibration', 'vibration'], 0.45),
  }))
}

export function generateHistoryData(): HistoryRow[] {
  return Array.from({ length: 120 }, (_, index) => {
    const wave = Math.sin(index / 9)
    return {
      id: index,
      time: `${String(Math.floor(index / 12)).padStart(2, '0')}:${String((index % 12) * 5).padStart(2, '0')}`,
      tempOut: round(48.6 + wave * 2.1),
      humidOut: round(33.2 + Math.cos(index / 10) * 1.6),
      tempIn: round(22.3 + wave * 0.9),
      humidIn: round(45.5 + Math.cos(index / 12) * 2.2),
      windIn: round(138.6 + wave * 6),
      noise: round(45.7 + Math.sin(index / 7) * 1.8),
      pressure: round(120 + Math.cos(index / 8) * 4),
      power: round(2.45 + Math.sin(index / 6) * 0.22),
      vibration: round(0.45 + Math.cos(index / 8) * 0.06),
    }
  })
}

export function buildGanttData(): GanttLane[] {
  return [
    {
      machineId: 'EDGE-3D-01',
      blocks: [
        { id: 1, sn: 'FACTORY-001', startStr: '08:00', endStr: '10:30', startPercent: 8, widthPercent: 18 },
        { id: 2, sn: 'FACTORY-002', startStr: '13:00', endStr: '15:20', startPercent: 54, widthPercent: 16 },
      ],
    },
    {
      machineId: 'CRAC-11',
      blocks: [
        { id: 3, sn: 'RUN-AC11', startStr: '09:30', endStr: '12:10', startPercent: 22, widthPercent: 19 },
      ],
    },
  ]
}

export function buildTaskLanes(runs: DetectionRun[]): { lanes: TaskLane[], minTime: number, maxTime: number } {
  const datedRuns = runs.filter((run) => run.started_at)
  if (datedRuns.length === 0) {
    const now = Date.now()
    return { lanes: [], minTime: now - 7 * 24 * 3600 * 1000, maxTime: now }
  }

  const timestamps = datedRuns.flatMap((run) => {
    const start = Date.parse(run.started_at ?? '')
    const rawEnd = Date.parse(run.ended_at || run.expected_end_at || run.updated_at || run.started_at || '')
    return [start, Number.isFinite(rawEnd) ? rawEnd : start]
  }).filter(Number.isFinite)

  const minTime = timestamps.length > 0 ? Math.min(...timestamps) : Date.now() - 7 * 24 * 3600 * 1000
  const maxTime = timestamps.length > 0 ? Math.max(...timestamps, minTime + 60 * 60 * 1000) : Date.now()
  const byProject = new Map<string, TaskBlock[]>()

  for (const run of datedRuns) {
    const start = Date.parse(run.started_at ?? '')
    if (!Number.isFinite(start)) continue
    const rawEnd = Date.parse(run.ended_at || run.expected_end_at || run.updated_at || run.started_at || '')
    const end = Number.isFinite(rawEnd) && rawEnd > start ? rawEnd : start + Math.max(run.duration_sec * 1000, 5 * 60 * 1000)
    const projectCode = run.project_code || String(run.project_id)
    const blocks = byProject.get(projectCode) ?? []
    blocks.push({
      id: run.id,
      testNo: run.test_no,
      projectCode,
      status: run.status,
      mode: run.mode,
      startStr: formatClock(start),
      endStr: formatClock(end),
      startMs: start,
      endMs: end,
    })
    byProject.set(projectCode, blocks)
  }

  return {
    lanes: Array.from(byProject.entries()).map(([projectCode, blocks]) => ({
      projectCode,
      blocks: blocks.sort((left, right) => left.startMs - right.startMs || left.id - right.id),
    })),
    minTime,
    maxTime,
  }
}

export function formatHistoryTime(value?: string) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${formatClock(date.getTime(), true)}`
}

function metricKeyFor(item: HistoryDataItem) {
  return `var_${String(item.var_id_text ?? item.var_id).replace(/[^a-zA-Z0-9_-]/g, '_')}`
}

function metricTitleFor(item: HistoryDataItem) {
  const varId = String(item.var_id_text ?? item.var_id)
  return item.var_name ? `${item.var_name} (${varId})` : varId
}

function numberFromRow(row: HistorySeriesRow, keys: string[], fallback: number) {
  for (const key of keys) {
    const value = row[key]
    if (typeof value === 'number' && Number.isFinite(value)) return value
  }
  return fallback
}

function round(value: number) {
  return Number(value.toFixed(2))
}

function formatClock(value: number, withSeconds = false) {
  const date = new Date(value)
  const base = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
  if (!withSeconds) return base
  return `${base}:${String(date.getSeconds()).padStart(2, '0')}`
}
