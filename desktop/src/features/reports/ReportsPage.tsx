import { useEffect, useMemo, useState } from 'react'
import { Button, InputNumber, Segmented, Select, Tag, message } from 'antd'
import { Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useTranslation } from 'react-i18next'
import {
  ChartLine,
  Calculator,
  Download,
  Eye,
  FileSpreadsheet,
  History,
  Move,
  PanelLeft,
  RefreshCw,
  Save,
  Upload,
} from 'lucide-react'
import { createLuckysheetAdapter, type SpreadsheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
import './reports.css'

type ReportMode = 'entry' | 'view'

type ReportTemplate = {
  id: string
  name: string
  code: string
  version: string
  updatedAt: string
}

type ReportRun = {
  id: string
  serial: string
  project: string
  result: 'OK' | 'NG'
  generatedAt: string
}

type ReportField = {
  id: string
  label: string
  value: string
  cell: string
  required?: boolean
}

type TrendPoint = {
  time: string
  supplyTemp: number
  returnHumidity: number
}

type ImagePlacement = {
  left: number
  top: number
  width: number
  height: number
}

type FormulaResult = {
  address: string
  label: string
  formula: string
  value: string | number
}

type FormulaDefinition = {
  label: string
  formula: string
  resultCell: string
  unit: string
}

const templates: ReportTemplate[] = [
  { id: 'factory', name: '出厂性能检测报告', code: 'FACTORY-PERF', version: 'V3', updatedAt: '2026-05-31 18:30' },
  { id: 'daily', name: '运行日报模板', code: 'EDGE-DAILY', version: 'V1', updatedAt: '2026-05-30 09:12' },
]

const runs: ReportRun[] = [
  { id: 'edge-3d-01', serial: 'EDGE-3D-01', project: 'Spindle Lab', result: 'OK', generatedAt: '2026-06-01 09:35' },
  { id: 'crac-11', serial: 'CRAC-11', project: '项目 11', result: 'NG', generatedAt: '2026-05-31 20:48' },
]

const initialReportFields: ReportField[] = [
  { id: 'model', label: '机型', value: 'CRAC-EDGE', cell: 'C4', required: true },
  { id: 'serial', label: '出厂编号', value: 'EDGE-3D-01', cell: 'H4', required: true },
  { id: 'customer', label: '客户名称', value: 'Spindle Lab', cell: 'C5', required: true },
  { id: 'supplyTemp', label: '吹出口温度', value: '48.6 °C', cell: 'D13' },
  { id: 'returnHumidity', label: '吸入口湿度', value: '33.2 %RH', cell: 'D14' },
  { id: 'result', label: '检测结论', value: 'OK', cell: 'H28' },
]

const trendPoints: TrendPoint[] = [
  { time: '08:00', supplyTemp: 44.2, returnHumidity: 29.8 },
  { time: '09:00', supplyTemp: 47.4, returnHumidity: 31.4 },
  { time: '10:00', supplyTemp: 52.1, returnHumidity: 34.2 },
  { time: '11:00', supplyTemp: 55.8, returnHumidity: 35.8 },
  { time: '12:00', supplyTemp: 59.4, returnHumidity: 33.2 },
  { time: '13:00', supplyTemp: 53.6, returnHumidity: 30.9 },
  { time: '14:00', supplyTemp: 48.6, returnHumidity: 32.6 },
]

const defaultImagePlacement: ImagePlacement = {
  left: 300,
  top: 348,
  width: 360,
  height: 190,
}

const formulaDefinitions: FormulaDefinition[] = [
  { label: '平均吹出口温度', formula: '=ROUND(AVERAGE(C32:C38),1)', resultCell: 'J25', unit: '°C' },
  { label: '最高吹出口温度', formula: '=MAX(C32:C38)', resultCell: 'J26', unit: '°C' },
  { label: '平均吸入口湿度', formula: '=ROUND(AVERAGE(E32:E38),1)', resultCell: 'J27', unit: '%RH' },
  { label: '温度优化率', formula: '=ROUND((55-J25)/7*100,1)', resultCell: 'J28', unit: '%' },
]

function cell(value: string | number, options: Record<string, unknown> = {}) {
  return {
    v: value,
    m: String(value),
    ct: { fa: 'General', t: 'g' },
    ff: 0,
    fs: 11,
    fc: '#0f2e57',
    ...options,
  }
}

type LuckysheetCellData = {
  r: number
  c: number
  v: ReturnType<typeof cell>
}

function parseCellAddress(address: string) {
  const match = address.trim().toUpperCase().match(/^([A-Z]{1,3})([1-9]\d*)$/)
  if (!match) return null

  const [, letters, rowText] = match
  let column = 0
  for (const letter of letters) {
    column = column * 26 + letter.charCodeAt(0) - 64
  }

  return {
    r: Number(rowText) - 1,
    c: column - 1,
  }
}

function upsertCell(celldata: LuckysheetCellData[], row: number, column: number, value: ReturnType<typeof cell>) {
  const existing = celldata.find((item) => item.r === row && item.c === column)
  if (existing) {
    existing.v = value
    return
  }
  celldata.push({ r: row, c: column, v: value })
}

function applyFieldMappings(celldata: LuckysheetCellData[], fields: ReportField[]) {
  fields.forEach((field) => {
    const target = parseCellAddress(field.cell)
    if (!target) return
    upsertCell(celldata, target.r, target.c, cell(field.value, { fc: '#1677ff', bl: field.required ? 1 : 0 }))
  })
}

function getFormulaResult(results: FormulaResult[], address: string) {
  return results.find((result) => result.address === address)?.value
}

function getCellValueFromSheet(sheet: Record<string, unknown>, row: number, column: number) {
  const data = sheet.data as unknown[][] | undefined
  const dataCell = data?.[row]?.[column] as { v?: unknown; m?: unknown } | undefined
  if (dataCell?.v !== undefined && dataCell.v !== null && dataCell.v !== '') return dataCell.v
  if (dataCell?.m !== undefined && dataCell.m !== null && dataCell.m !== '') return dataCell.m

  const celldata = sheet.celldata as LuckysheetCellData[] | undefined
  const source = celldata?.find((item) => item.r === row && item.c === column)?.v
  return source?.v ?? source?.m ?? null
}

function readNumberFromSheet(sheet: Record<string, unknown>, row: number, column: number) {
  const value = getCellValueFromSheet(sheet, row, column)
  if (typeof value === 'number') return value
  if (typeof value === 'string') {
    const matched = value.replace(/,/g, '').match(/-?\d+(\.\d+)?/)
    return matched ? Number(matched[0]) : 0
  }
  return 0
}

function readRangeNumbers(sheet: Record<string, unknown>, range: string) {
  const [start, end] = range.split(':')
  const startCell = parseCellAddress(start)
  const endCell = parseCellAddress(end)
  if (!startCell || !endCell) return []

  const values: number[] = []
  const minRow = Math.min(startCell.r, endCell.r)
  const maxRow = Math.max(startCell.r, endCell.r)
  const minColumn = Math.min(startCell.c, endCell.c)
  const maxColumn = Math.max(startCell.c, endCell.c)
  for (let row = minRow; row <= maxRow; row += 1) {
    for (let column = minColumn; column <= maxColumn; column += 1) {
      values.push(readNumberFromSheet(sheet, row, column))
    }
  }
  return values
}

function splitFormulaArguments(content: string) {
  const args: string[] = []
  let depth = 0
  let current = ''
  for (const char of content) {
    if (char === '(') depth += 1
    if (char === ')') depth -= 1
    if (char === ',' && depth === 0) {
      args.push(current.trim())
      current = ''
      continue
    }
    current += char
  }
  if (current.trim()) args.push(current.trim())
  return args
}

function evaluateFormula(formula: string, sheet: Record<string, unknown>, computed: Record<string, number> = {}): number {
  let expression = formula.trim().replace(/^=/, '')

  const roundMatch = expression.match(/^ROUND\((.*)\)$/i)
  if (roundMatch) {
    const [inner, digits = '0'] = splitFormulaArguments(roundMatch[1])
    const value: number = evaluateFormula(`=${inner}`, sheet, computed)
    return Number(Number(value).toFixed(Number(digits)))
  }

  expression = expression.replace(/(AVERAGE|SUM|MIN|MAX)\(([A-Z]{1,3}[1-9]\d*:[A-Z]{1,3}[1-9]\d*)\)/gi, (_match, fn, range) => {
    const values = readRangeNumbers(sheet, range)
    if (!values.length) return '0'
    if (fn.toUpperCase() === 'AVERAGE') return String(values.reduce((sum, value) => sum + value, 0) / values.length)
    if (fn.toUpperCase() === 'SUM') return String(values.reduce((sum, value) => sum + value, 0))
    if (fn.toUpperCase() === 'MIN') return String(Math.min(...values))
    return String(Math.max(...values))
  })

  expression = expression.replace(/\b[A-Z]{1,3}[1-9]\d*\b/g, (address) => {
    if (computed[address] !== undefined) return String(computed[address])
    const target = parseCellAddress(address)
    return target ? String(readNumberFromSheet(sheet, target.r, target.c)) : '0'
  })

  if (!/^[\d+\-*/().\s]+$/.test(expression)) return 0
  try {
    return Number(Function(`"use strict"; return (${expression})`)())
  } catch {
    return 0
  }
}

function readFormulaResultsFromWorkbook(workbook: unknown[]) {
  const sheet = (workbook[0] ?? {}) as Record<string, unknown>
  const results: FormulaResult[] = []
  const computed: Record<string, number> = {}

  formulaDefinitions.forEach((definition, index) => {
    const formulaFromSheet = getCellValueFromSheet(sheet, 24 + index, 3)
    const formula = typeof formulaFromSheet === 'string' && formulaFromSheet.startsWith('=') ? formulaFromSheet : definition.formula
    const rawValue = evaluateFormula(formula, sheet, computed)
    const value = Number.isInteger(rawValue) ? rawValue : Number(rawValue.toFixed(2))
    computed[definition.resultCell] = Number(value)
    results.push({
      address: definition.resultCell,
      label: definition.label,
      formula,
      value: definition.unit ? `${value} ${definition.unit}` : value,
    })
  })

  return results
}

function scalePoint(value: number, min: number, max: number, height: number, top: number) {
  if (max === min) return top + height / 2
  return top + height - ((value - min) / (max - min)) * height
}

function encodeSvgDataUrl(svg: string) {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}

function buildTrendChartSvg(points: TrendPoint[]) {
  const width = 360
  const height = 190
  const plot = { left: 42, top: 24, width: 292, height: 112 }
  const values = points.flatMap((point) => [point.supplyTemp, point.returnHumidity])
  const min = Math.floor(Math.min(...values) / 5) * 5
  const max = Math.ceil(Math.max(...values) / 5) * 5

  const pointToPair = (point: TrendPoint, index: number, key: 'supplyTemp' | 'returnHumidity') => {
    const x = plot.left + (index / Math.max(points.length - 1, 1)) * plot.width
    const y = scalePoint(point[key], min, max, plot.height, plot.top)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }

  const supplyLine = points.map((point, index) => pointToPair(point, index, 'supplyTemp')).join(' ')
  const humidityLine = points.map((point, index) => pointToPair(point, index, 'returnHumidity')).join(' ')
  const gridLines = [0, 0.25, 0.5, 0.75, 1]
    .map((ratio) => {
      const y = plot.top + plot.height * ratio
      return `<line x1="${plot.left}" y1="${y}" x2="${plot.left + plot.width}" y2="${y}" stroke="#d7e7fb" stroke-width="1" stroke-dasharray="4 4"/>`
    })
    .join('')
  const xTicks = points
    .map((point, index) => {
      const x = plot.left + (index / Math.max(points.length - 1, 1)) * plot.width
      return `<text x="${x}" y="170" text-anchor="middle" font-size="10" fill="#59708f">${point.time}</text>`
    })
    .join('')

  return encodeSvgDataUrl(`
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <rect width="${width}" height="${height}" rx="14" fill="#f6fbff"/>
  <rect x="1" y="1" width="${width - 2}" height="${height - 2}" rx="13" fill="none" stroke="#d7e9ff"/>
  <text x="18" y="20" font-size="13" font-weight="700" fill="#0b4fb4">任务趋势曲线</text>
  <g>${gridLines}</g>
  <text x="16" y="32" font-size="9" fill="#7a8ca5">${max}</text>
  <text x="16" y="137" font-size="9" fill="#7a8ca5">${min}</text>
  <polyline points="${supplyLine}" fill="none" stroke="#1677ff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
  <polyline points="${humidityLine}" fill="none" stroke="#12a37f" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/>
  <g>${xTicks}</g>
  <circle cx="206" cy="18" r="4" fill="#1677ff"/><text x="216" y="21" font-size="10" fill="#48617e">吹出口温度</text>
  <circle cx="288" cy="18" r="4" fill="#12a37f"/><text x="298" y="21" font-size="10" fill="#48617e">吸入口湿度</text>
</svg>`)
}

function buildSheetData(
  mode: ReportMode,
  template: ReportTemplate,
  run: ReportRun,
  fields: ReportField[],
  chartLoaded: boolean,
  imagePlacement: ImagePlacement,
  formulaResults: FormulaResult[],
) {
  const celldata: LuckysheetCellData[] = [
    { r: 0, c: 0, v: cell('精密空调性能检测报告', { bl: 1, fs: 22, fc: '#0b4fb4', ht: 0, bg: '#eaf4ff' }) },
    { r: 1, c: 0, v: cell(`模板 ${template.code} · ${template.version}`, { fc: '#6b7c93', bg: '#eaf4ff' }) },
    { r: 3, c: 0, v: cell('机型', { bl: 1, bg: '#f2f7ff' }) },
    { r: 3, c: 2, v: cell('', { bl: 1 }) },
    { r: 3, c: 5, v: cell('出厂编号', { bl: 1, bg: '#f2f7ff' }) },
    { r: 3, c: 7, v: cell('', { bl: 1 }) },
    { r: 4, c: 0, v: cell('客户名称', { bl: 1, bg: '#f2f7ff' }) },
    { r: 4, c: 2, v: cell('', { bl: 1 }) },
    { r: 4, c: 5, v: cell('检测时间', { bl: 1, bg: '#f2f7ff' }) },
    { r: 4, c: 7, v: cell(run.generatedAt) },
    { r: 7, c: 0, v: cell('检测项目', { bl: 1, bg: '#dfeeff', ht: 1 }) },
    { r: 7, c: 3, v: cell('标准范围', { bl: 1, bg: '#dfeeff', ht: 1 }) },
    { r: 7, c: 5, v: cell('实测值', { bl: 1, bg: '#dfeeff', ht: 1 }) },
    { r: 7, c: 7, v: cell('结论', { bl: 1, bg: '#dfeeff', ht: 1 }) },
    { r: 8, c: 0, v: cell('吹出口温度') },
    { r: 8, c: 3, v: cell('48 - 55 °C') },
    { r: 8, c: 5, v: cell('48.6 °C', { fc: '#1677ff', bl: 1 }) },
    { r: 8, c: 7, v: cell('OK', { fc: '#0f8b60', bl: 1 }) },
    { r: 9, c: 0, v: cell('吸入口湿度') },
    { r: 9, c: 3, v: cell('20 - 40 %RH') },
    { r: 9, c: 5, v: cell('33.2 %RH', { fc: '#1677ff', bl: 1 }) },
    { r: 9, c: 7, v: cell('OK', { fc: '#0f8b60', bl: 1 }) },
    { r: 10, c: 0, v: cell('吸入风量') },
    { r: 10, c: 3, v: cell('120 - 160 m³/h') },
    { r: 10, c: 5, v: cell('138.6 m³/h', { fc: '#1677ff', bl: 1 }) },
    { r: 10, c: 7, v: cell('OK', { fc: '#0f8b60', bl: 1 }) },
    { r: 11, c: 0, v: cell('设备噪音') },
    { r: 11, c: 3, v: cell('40 - 75 dB') },
    { r: 11, c: 5, v: cell('45.7 dB', { fc: '#1677ff', bl: 1 }) },
    { r: 11, c: 7, v: cell('OK', { fc: '#0f8b60', bl: 1 }) },
    { r: 14, c: 0, v: cell(mode === 'entry' ? '录入备注' : '查看备注', { bl: 1, bg: '#f2f7ff' }) },
    { r: 14, c: 2, v: cell(mode === 'entry' ? '请在表格中补齐现场记录项' : '当前为只读查看态，接口接入后显示真实报表', { fc: '#52657a' }) },
    { r: 18, c: 0, v: cell('字段映射', { bl: 1, bg: '#f2f7ff' }) },
    { r: 19, c: 0, v: cell('字段') },
    { r: 19, c: 2, v: cell('值') },
    { r: 19, c: 5, v: cell('目标单元格') },
  ]
  fields.forEach((field, index) => {
    const row = 20 + index
    celldata.push(
      { r: row, c: 0, v: cell(field.label) },
      { r: row, c: 2, v: cell(field.value, { fc: '#1677ff' }) },
      { r: row, c: 5, v: cell(field.cell, { fc: field.cell ? '#0f8b60' : '#d48806', bl: 1 }) },
    )
  })

  celldata.push(
    { r: 22, c: 0, v: cell('公式区域', { bl: 1, bg: '#eaf4ff', fc: '#0b4fb4' }) },
    { r: 23, c: 0, v: cell('指标', { bl: 1, bg: '#dfeeff' }) },
    { r: 23, c: 3, v: cell('公式', { bl: 1, bg: '#dfeeff' }) },
    { r: 23, c: 9, v: cell('输出', { bl: 1, bg: '#dfeeff' }) },
  )
  formulaDefinitions.forEach((definition, index) => {
    const row = 24 + index
    const resultTarget = parseCellAddress(definition.resultCell)
    const resultValue = getFormulaResult(formulaResults, definition.resultCell) ?? '待结算'
    celldata.push(
      { r: row, c: 0, v: cell(definition.label) },
      { r: row, c: 3, v: cell(definition.formula, { fc: '#6b7c93' }) },
    )
    if (resultTarget) {
      celldata.push(
        {
          r: resultTarget.r,
          c: resultTarget.c,
          v: cell(String(resultValue), { ct: { fa: '@', t: 's' }, fc: resultValue === '待结算' ? '#8c6d1f' : '#0f8b60', bl: 1 }),
        },
      )
    }
  })
  celldata.push(
    { r: 29, c: 0, v: cell('任务趋势曲线数据', { bl: 1, bg: '#eaf4ff', fc: '#0b4fb4' }) },
    { r: 30, c: 0, v: cell('时间', { bl: 1, bg: '#dfeeff' }) },
    { r: 30, c: 2, v: cell('吹出口温度(°C)', { bl: 1, bg: '#dfeeff' }) },
    { r: 30, c: 4, v: cell('吸入口湿度(%RH)', { bl: 1, bg: '#dfeeff' }) },
  )
  trendPoints.forEach((point, index) => {
    const row = 31 + index
    celldata.push(
      { r: row, c: 0, v: cell(point.time) },
      { r: row, c: 2, v: cell(point.supplyTemp, { fc: '#1677ff' }) },
      { r: row, c: 4, v: cell(point.returnHumidity, { fc: '#12a37f' }) },
    )
  })
  applyFieldMappings(celldata, fields)

  const sheet = {
    name: mode === 'entry' ? '报表录入' : '报表查看',
    index: '0',
    status: 1,
    order: 0,
    row: 42,
    column: 11,
    celldata,
    config: {
      merge: {
        '0_0': { r: 0, c: 0, rs: 1, cs: 9 },
        '1_0': { r: 1, c: 0, rs: 1, cs: 9 },
        '14_2': { r: 14, c: 2, rs: 2, cs: 7 },
        '18_0': { r: 18, c: 0, rs: 1, cs: 5 },
        '22_0': { r: 22, c: 0, rs: 1, cs: 8 },
        '29_0': { r: 29, c: 0, rs: 1, cs: 8 },
      },
      columnlen: { 0: 118, 1: 82, 2: 132, 3: 128, 4: 76, 5: 108, 6: 104, 7: 104, 8: 104, 9: 104 },
      rowlen: { 0: 42, 1: 30, 7: 34, 18: 30 },
    },
  }

  if (!chartLoaded) return [sheet]

  const chartWidth = 360
  const chartHeight = 190
  return {
    sheets: [sheet],
    images: {
      report_trend_chart: {
        type: '3',
        src: buildTrendChartSvg(trendPoints),
        originWidth: chartWidth,
        originHeight: chartHeight,
        default: {
          width: imagePlacement.width,
          height: imagePlacement.height,
          left: imagePlacement.left,
          top: imagePlacement.top,
        },
        crop: {
          width: chartWidth,
          height: chartHeight,
          offsetLeft: 0,
          offsetTop: 0,
        },
        isFixedPos: false,
        fixedLeft: imagePlacement.left,
        fixedTop: imagePlacement.top,
        border: {
          width: 1,
          radius: 'solid',
          style: 'solid',
          color: '#93c5fd',
        },
      },
    },
  }
}

function ReportSheet({
  adapter,
  mode,
  template,
  run,
  fields,
  chartLoaded,
  imagePlacement,
  formulaResults,
}: {
  adapter: SpreadsheetAdapter
  mode: ReportMode
  template: ReportTemplate
  run: ReportRun
  fields: ReportField[]
  chartLoaded: boolean
  imagePlacement: ImagePlacement
  formulaResults: FormulaResult[]
}) {
  const containerId = useMemo(() => `report-sheet-${mode}-${template.id}-${run.id}`, [mode, run.id, template.id])

  useEffect(() => {
    void adapter.mount({
      containerId,
      data: buildSheetData(mode, template, run, fields, chartLoaded, imagePlacement, formulaResults),
      readonly: mode === 'view',
      toolbar: mode === 'entry',
      sheetbar: true,
    })
    return () => adapter.unmount()
  }, [adapter, chartLoaded, containerId, fields, formulaResults, imagePlacement, mode, run, template])

  return <div id={containerId} className="report-sheet-host" />
}

export function ReportsPage() {
  const { t } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [mode, setMode] = useState<ReportMode>('entry')
  const [templateId, setTemplateId] = useState(templates[0].id)
  const [runId, setRunId] = useState(runs[0].id)
  const [fields, setFields] = useState<ReportField[]>(initialReportFields)
  const [chartLoaded, setChartLoaded] = useState(false)
  const [imagePlacement, setImagePlacement] = useState<ImagePlacement>(defaultImagePlacement)
  const [formulaResults, setFormulaResults] = useState<FormulaResult[]>([])
  const [sheetAdapter] = useState(() => createLuckysheetAdapter())
  const selectedTemplate = templates.find((template) => template.id === templateId) ?? templates[0]
  const selectedRun = runs.find((run) => run.id === runId) ?? runs[0]
  const mappedCount = fields.filter((field) => field.cell.trim()).length

  function updateFieldCell(fieldId: string, cellValue: string) {
    setFields((current) =>
      current.map((field) => (field.id === fieldId ? { ...field, cell: cellValue.toUpperCase().replace(/[^A-Z0-9]/g, '') } : field)),
    )
  }

  function loadTaskChartToSheet() {
    setChartLoaded(true)
    messageApi.success(t('reports.chartLoaded'))
  }

  function updateImagePlacement(key: keyof ImagePlacement, value: number | null) {
    if (value === null) return
    setImagePlacement((current) => ({ ...current, [key]: value }))
  }

  function calculateReportFormulas() {
    const results = readFormulaResultsFromWorkbook(sheetAdapter.getWorkbook())
    setFormulaResults(results)
    if (results.length) {
      messageApi.success(`已读取 ${results.length} 个公式并输出结果`)
    } else {
      messageApi.warning('当前表格没有读取到公式区域')
    }
  }

  return (
    <div className="reports-page">
      {contextHolder}
      <div className="report-ambient-background" aria-hidden="true">
        <span className="report-orb report-orb-1" />
        <span className="report-orb report-orb-2" />
        <span className="report-orb report-orb-3" />
        <span className="report-noise" />
      </div>

      <header className="report-toolbar">
        <div>
          <span className="report-eyebrow">{t('reports.eyebrow')}</span>
          <h1>{t('reports.title')}</h1>
        </div>
        <div className="report-actions">
          <Segmented
            value={mode}
            onChange={(value) => setMode(value as ReportMode)}
            options={[
              { label: t('reports.entry'), value: 'entry', icon: <FileSpreadsheet size={14} /> },
              { label: t('reports.view'), value: 'view', icon: <Eye size={14} /> },
            ]}
          />
          <Button icon={<RefreshCw size={15} />}>{t('actions.refresh')}</Button>
        </div>
      </header>

      <section className="report-layout">
        <aside className="report-side-panel report-glass-panel">
          <div className="report-section-title">
            <PanelLeft size={16} />
            <strong>{mode === 'entry' ? t('reports.templatePanel') : t('reports.historyPanel')}</strong>
          </div>

          <label className="report-field">
            <span>{t('reports.template')}</span>
            <Select
              value={templateId}
              onChange={setTemplateId}
              options={templates.map((template) => ({ value: template.id, label: `${template.name} ${template.version}` }))}
            />
          </label>

          <label className="report-field">
            <span>{t('reports.run')}</span>
            <Select
              value={runId}
              onChange={(value) => {
                setRunId(value)
                setChartLoaded(false)
              }}
              options={runs.map((run) => ({ value: run.id, label: `${run.serial} · ${run.project}` }))}
            />
          </label>

          <div className="report-meta-card">
            <span>{selectedTemplate.code}</span>
            <strong>{selectedTemplate.name}</strong>
            <small>{t('reports.updatedAt', { value: selectedTemplate.updatedAt })}</small>
          </div>

          <div className="report-field-list">
            <div className="report-section-title compact">
              <History size={15} />
              <strong>{t('reports.bindings', { mapped: mappedCount, total: fields.length })}</strong>
            </div>
            {fields.map((field) => (
              <div className="report-binding" key={field.id}>
                <div>
                  <strong>{field.label}</strong>
                  <span>{field.value}</span>
                </div>
                <input
                  aria-label={`${field.label} ${t('reports.cell')}`}
                  value={field.cell}
                  onChange={(event) => updateFieldCell(field.id, event.target.value)}
                />
              </div>
            ))}
          </div>

          <div className="report-chart-card">
            <div className="report-section-title compact">
              <ChartLine size={15} />
              <strong>{t('reports.taskTrend')}</strong>
            </div>
            <div className="report-chart">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={trendPoints} margin={{ top: 8, right: 12, bottom: 0, left: -22 }}>
                  <XAxis dataKey="time" tickLine={false} axisLine={false} tick={{ fontSize: 10, fill: 'rgba(30, 27, 24, 0.48)' }} />
                  <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 10, fill: 'rgba(30, 27, 24, 0.42)' }} />
                  <Tooltip />
                  <Line type="monotone" dataKey="supplyTemp" dot={false} stroke="#1677ff" strokeWidth={2.4} />
                  <Line type="monotone" dataKey="returnHumidity" dot={false} stroke="#12a37f" strokeWidth={2.2} />
                </LineChart>
              </ResponsiveContainer>
            </div>
            <Button size="small" type={chartLoaded ? 'default' : 'primary'} icon={<ChartLine size={14} />} onClick={loadTaskChartToSheet}>
              {chartLoaded ? t('reports.chartLoadedState') : t('reports.loadChart')}
            </Button>
            <Tag color={chartLoaded ? 'blue' : 'default'}>{chartLoaded ? t('reports.loaded') : t('reports.mockOnly')}</Tag>
          </div>

          <div className="report-tool-card">
            <div className="report-section-title compact">
              <Move size={15} />
              <strong>图表嵌入位置</strong>
            </div>
            <div className="report-placement-grid">
              <label>
                <span>X</span>
                <InputNumber size="small" min={0} max={1200} value={imagePlacement.left} onChange={(value) => updateImagePlacement('left', value)} />
              </label>
              <label>
                <span>Y</span>
                <InputNumber size="small" min={0} max={900} value={imagePlacement.top} onChange={(value) => updateImagePlacement('top', value)} />
              </label>
              <label>
                <span>W</span>
                <InputNumber size="small" min={120} max={760} value={imagePlacement.width} onChange={(value) => updateImagePlacement('width', value)} />
              </label>
              <label>
                <span>H</span>
                <InputNumber size="small" min={80} max={420} value={imagePlacement.height} onChange={(value) => updateImagePlacement('height', value)} />
              </label>
            </div>
          </div>

          <div className="report-tool-card">
            <div className="report-section-title compact">
              <Calculator size={15} />
              <strong>公式结算</strong>
            </div>
            <Button size="small" type="primary" icon={<Calculator size={14} />} onClick={calculateReportFormulas}>
              读取公式并结算输出
            </Button>
            <div className="report-formula-list">
              {(formulaResults.length ? formulaResults : formulaDefinitions.map((item) => ({ ...item, address: item.resultCell, value: '待结算' }))).map((result) => (
                <div className="report-formula-item" key={result.address}>
                  <span>{result.address}</span>
                  <strong>{result.value}</strong>
                  <small>{result.formula}</small>
                </div>
              ))}
            </div>
          </div>
        </aside>

        <main className="report-workspace report-glass-panel">
          <div className="report-workspace-head">
            <div>
              <strong>{mode === 'entry' ? t('reports.entryTitle') : t('reports.viewTitle')}</strong>
              <span>
                {selectedRun.serial} · {selectedRun.project} · {selectedRun.result}
              </span>
            </div>
            <div className="report-workspace-actions">
              <Button icon={<Upload size={15} />} disabled={mode === 'view'}>
                {t('reports.import')}
              </Button>
              <Button icon={<Save size={15} />} disabled>
                {t('reports.save')}
              </Button>
              <Button
                type="primary"
                icon={<Download size={15} />}
                onClick={() => messageApi.info(t('reports.staticHint'))}
              >
                {t('reports.export')}
              </Button>
            </div>
          </div>
          <div className="report-sheet-shell">
            <ReportSheet
              adapter={sheetAdapter}
              mode={mode}
              template={selectedTemplate}
              run={selectedRun}
              fields={fields}
              chartLoaded={chartLoaded}
              imagePlacement={imagePlacement}
              formulaResults={formulaResults}
            />
          </div>
        </main>
      </section>
    </div>
  )
}
