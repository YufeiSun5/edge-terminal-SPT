import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import ExcelJS from 'exceljs'
import { Alert, Button, Checkbox, Empty, Input, InputNumber, Select, Space, Statistic, Tag, Typography, Upload, message } from 'antd'
import type { UploadProps } from 'antd'
import { CheckCircle2, Crosshair, FileSpreadsheet, Plus, Trash2, UploadCloud } from 'lucide-react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { confirmMainReportPlanImport, getVariablesPage, parseMainReportPlanImport } from '@/features/edge-status/api'
import { createLuckysheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
import type { SpreadsheetCellSelection } from '@/features/spreadsheet/luckysheetAdapter'
import { env } from '@/shared/config/env'
import type { PlanImportCellMapping, PlanImportIssue, PlanImportRow } from '@/shared/api/types'
import './reports.css'

type WorkbookPreview = {
  sheets: string[]
  activeSheet: string
  rows: string[][]
}

type MappingRowConfig = {
  id: string
  rowNumber?: number
  varName: string
  limitL: string
  limitLValue: string
  limitH: string
  limitHValue: string
  unit: string
  unitValue: string
  formulaJson: string
  formulaJsonValue: string
  checkEnabled: boolean
}

const commonFieldKeys = ['project_code', 'params_json', 'test_no', 'factory_no', 'customer_name', 'device_model', 'template_code', 'report_name'] as const
type CommonCellKey = (typeof commonFieldKeys)[number]
type MappingRowCellField = 'limitL' | 'limitH' | 'unit' | 'formulaJson'
type CellPickTarget =
  | { scope: 'common'; key: CommonCellKey }
  | { scope: 'row'; rowId: string; field: MappingRowCellField }

const cellPickMessageKey = 'report-plan-cell-pick'

const defaultCommonCells: Record<string, string> = {
  project_code: '',
  params_json: '',
  test_no: '',
  factory_no: '',
  customer_name: '',
  device_model: '',
  template_code: '',
  report_name: '',
}

const defaultCommonValues: Record<CommonCellKey, string> = {
  project_code: '',
  params_json: '',
  test_no: '',
  factory_no: '',
  customer_name: '',
  device_model: '',
  template_code: '',
  report_name: '',
}

function nextMappingRow(): MappingRowConfig {
  return {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    varName: '',
    limitL: '',
    limitLValue: '',
    limitH: '',
    limitHValue: '',
    unit: '',
    unitValue: '',
    formulaJson: '',
    formulaJsonValue: '',
    checkEnabled: true,
  }
}

function cellPickTargetKey(target: CellPickTarget) {
  return target.scope === 'common' ? `common:${target.key}` : `row:${target.rowId}:${target.field}`
}

function selectionDisplayValue(selection: SpreadsheetCellSelection) {
  return selection.value.trim()
}

function sourceCellAddress(selection: SpreadsheetCellSelection) {
  return selection.mergedAddress && selection.mergedAddress !== selection.address ? selection.mergedAddress : selection.address
}

function issueText(issues?: PlanImportIssue[]) {
  if (!issues?.length) return ''
  return issues.map((issue) => `${issue.field}:${issue.code}`).join('; ')
}

function limitValueText(value?: number) {
  return value === undefined || value === null ? '-' : String(value)
}

function rowReviewState(record: PlanImportRow) {
  if (record.issues?.length) return 'issue'
  if (record.needs_confirm || record.limit.needs_confirmation) return 'confirm'
  if (record.limit.limit_l === undefined && record.limit.limit_h === undefined) return 'no-limit'
  return 'ready'
}

function excelPreviewValue(value: unknown): string {
  if (value == null) return ''
  if (value instanceof Date) return value.toISOString()
  if (typeof value !== 'object') return String(value)
  if ('richText' in value && Array.isArray(value.richText)) {
    return value.richText.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }
  if ('result' in value) return excelPreviewValue(value.result)
  if ('text' in value && typeof value.text === 'string') return value.text
  if ('formula' in value && typeof value.formula === 'string') return `=${value.formula}`
  if ('hyperlink' in value && typeof value.hyperlink === 'string') return value.hyperlink
  return ''
}

async function readWorkbookPreview(file: File, sheetName?: string): Promise<WorkbookPreview> {
  const buffer = await file.arrayBuffer()
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.load(buffer)
  const sheets = workbook.worksheets.map((sheet) => sheet.name)
  const activeSheet = sheetName && sheets.includes(sheetName) ? sheetName : sheets[0] || ''
  const sheet = workbook.getWorksheet(activeSheet)
  const rows: string[][] = []
  if (sheet) {
    const maxRow = Math.min(sheet.rowCount, 30)
    const maxColumn = Math.min(sheet.columnCount, 12)
    for (let rowIndex = 1; rowIndex <= maxRow; rowIndex += 1) {
      const row = sheet.getRow(rowIndex)
      const values: string[] = []
      for (let columnIndex = 1; columnIndex <= maxColumn; columnIndex += 1) {
        const cell = row.getCell(columnIndex)
        const raw = excelPreviewValue(cell.value)
        values.push(raw)
      }
      rows.push(values)
    }
  }
  return { sheets, activeSheet, rows }
}

export function ReportPlanImportPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [allowNeedsConfirmation, setAllowNeedsConfirmation] = useState(false)
  const [workbookPreview, setWorkbookPreview] = useState<WorkbookPreview | null>(null)
  const [previewError, setPreviewError] = useState('')
  const [useCellMapping, setUseCellMapping] = useState(false)
  const [commonCells, setCommonCells] = useState<Record<string, string>>(defaultCommonCells)
  const [commonValues, setCommonValues] = useState<Record<CommonCellKey, string>>(defaultCommonValues)
  const [mappingRows, setMappingRows] = useState<MappingRowConfig[]>([nextMappingRow()])
  const [parsedRows, setParsedRows] = useState<PlanImportRow[]>([])
  const [variableSearch, setVariableSearch] = useState('')
  const [cellPickTarget, setCellPickTarget] = useState<CellPickTarget | null>(null)
  const [pickedCellValues, setPickedCellValues] = useState<Record<string, SpreadsheetCellSelection>>({})
  const adapterRef = useRef<ReturnType<typeof createLuckysheetAdapter> | null>(null)
  const cellPickTargetRef = useRef<CellPickTarget | null>(null)
  const isMainServer = env.runtimeRole === 'main_server'
  const variablesQuery = useQuery({
    queryKey: ['report-plan-import-variables', variableSearch.trim()],
    queryFn: () =>
      getVariablesPage({
        enabled: true,
        keyword: variableSearch.trim() || undefined,
        limit: variableSearch.trim() ? 100 : 500,
      }),
    enabled: isMainServer,
    staleTime: 60_000,
  })
  const currentLanguage = i18n.language || 'zh'
  const variableItems = useMemo(() => variablesQuery.data?.items ?? [], [variablesQuery.data?.items])

  useEffect(() => {
    cellPickTargetRef.current = cellPickTarget
  }, [cellPickTarget])

  const handleSpreadsheetCellSelect = useCallback(
    (selection: SpreadsheetCellSelection) => {
      const target = cellPickTargetRef.current
      if (!target) return
      const targetKey = cellPickTargetKey(target)
      const value = selectionDisplayValue(selection)
      if (target.scope === 'common') {
        setCommonCells((current) => ({ ...current, [target.key]: selection.address }))
        setCommonValues((current) => ({ ...current, [target.key]: value }))
      } else {
        setMappingRows((current) =>
          current.map((row) =>
            row.id === target.rowId
              ? target.field === 'limitH'
                ? { ...row, limitH: selection.address, limitHValue: value }
                : target.field === 'limitL'
                  ? { ...row, limitL: selection.address, limitLValue: value }
                  : target.field === 'unit'
                    ? { ...row, unit: selection.address, unitValue: value }
                    : { ...row, formulaJson: selection.address, formulaJsonValue: value }
              : row,
          ),
        )
      }
      setPickedCellValues((current) => ({ ...current, [targetKey]: selection }))
      setUseCellMapping(true)
      cellPickTargetRef.current = null
      setCellPickTarget(null)
      messageApi.success({
        key: cellPickMessageKey,
        content: t('reportSettings.planImport.mapping.cellPicked', {
          cell: sourceCellAddress(selection),
          value: value || t('reportSettings.planImport.mapping.emptyCellValue'),
        }),
      })
    },
    [messageApi, t],
  )

  const startCellPick = useCallback(
    (target: CellPickTarget) => {
      cellPickTargetRef.current = target
      setCellPickTarget(target)
      setUseCellMapping(true)
      messageApi.info({ key: cellPickMessageKey, content: t('reportSettings.planImport.mapping.pickCellHint') })
    },
    [messageApi, t],
  )

  const cancelCellPick = useCallback(() => {
    cellPickTargetRef.current = null
    setCellPickTarget(null)
  }, [])

  const pickedCellSourceAddress = useCallback(
    (target: CellPickTarget) => {
      const selection = pickedCellValues[cellPickTargetKey(target)]
      if (!selection) return ''
      return sourceCellAddress(selection)
    },
    [pickedCellValues],
  )

  const cellPickerAddon = useCallback(
    (target: CellPickTarget, source?: string) => {
      const active = cellPickTarget ? cellPickTargetKey(cellPickTarget) === cellPickTargetKey(target) : false
      return (
        <span className="report-cell-picker-addon">
          {source ? (
            <span className="report-cell-source-badge" title={t('reportSettings.planImport.mapping.sourceCell', { cell: source })}>
              {source}
            </span>
          ) : null}
          <Button
            className="report-cell-picker-button"
            type={active ? 'primary' : 'text'}
            size="small"
            icon={<Crosshair size={13} />}
            aria-label={t('reportSettings.planImport.mapping.pickCell')}
            onClick={() => startCellPick(target)}
          />
        </span>
      )
    },
    [cellPickTarget, startCellPick, t],
  )

  useEffect(() => {
    if (!selectedFile) {
      return
    }
    let cancelled = false
    readWorkbookPreview(selectedFile)
      .then((preview) => {
        if (!cancelled) {
          setWorkbookPreview(preview)
          setPreviewError('')
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setWorkbookPreview(null)
          setPreviewError(error instanceof Error ? error.message : String(error))
        }
      })
    return () => {
      cancelled = true
    }
  }, [selectedFile])

  useEffect(() => {
    if (!selectedFile) {
      adapterRef.current?.unmount()
      adapterRef.current = null
      return
    }

    const adapter = createLuckysheetAdapter()
    adapterRef.current = adapter
    let cancelled = false

    adapter
      .mount({
        containerId: 'report-plan-import-luckysheet',
        readonly: true,
        toolbar: false,
        sheetbar: true,
        deferCreate: true,
        onCellSelect: handleSpreadsheetCellSelect,
      })
      .then(() => {
        if (cancelled) return undefined
        return adapter.importFile(selectedFile)
      })
      .catch((error) => {
        if (!cancelled) setPreviewError(error instanceof Error ? error.message : String(error))
      })

    return () => {
      cancelled = true
      adapter.unmount()
      if (adapterRef.current === adapter) adapterRef.current = null
    }
  }, [handleSpreadsheetCellSelect, selectedFile])

  const mappingPayload = useMemo<PlanImportCellMapping | undefined>(() => {
    if (!useCellMapping || !workbookPreview) return undefined
    const common: Record<string, string> = {}
    const commonValuesPayload: Record<string, string> = {}
    for (const key of commonFieldKeys) {
      const value = commonValues[key].trim()
      const cell = commonCells[key]?.trim()
      if (value) {
        commonValuesPayload[key] = value
      } else if (cell) {
        common[key] = cell
      }
    }
    const rows = mappingRows
      .map((row) => {
        const fields: Record<string, string> = {}
        const values: Record<string, string> = {}
        if (row.varName.trim()) values.var_name = row.varName.trim()
        if (row.limitLValue.trim()) values.limit_l = row.limitLValue.trim()
        else if (row.limitL.trim()) fields.limit_l = row.limitL.trim()
        if (row.limitHValue.trim()) values.limit_h = row.limitHValue.trim()
        else if (row.limitH.trim()) fields.limit_h = row.limitH.trim()
        if (row.unitValue.trim()) values.unit = row.unitValue.trim()
        else if (row.unit.trim()) fields.unit = row.unit.trim()
        if (row.formulaJsonValue.trim()) values.formula_json = row.formulaJsonValue.trim()
        else if (row.formulaJson.trim()) fields.formula_json = row.formulaJson.trim()
        values.check_enabled = row.checkEnabled ? 'true' : 'false'
        return {
          row_number: row.rowNumber,
          fields,
          values,
        }
      })
      .filter((row) => Object.keys(row.values ?? {}).length > 0 || Object.keys(row.fields ?? {}).length > 0)
    if (!rows.length) return undefined
    return { sheet: workbookPreview.activeSheet, common, common_values: commonValuesPayload, rows }
  }, [commonCells, commonValues, mappingRows, useCellMapping, workbookPreview])
  const variableOptions = useMemo(() => {
    const locale = currentLanguage.toLowerCase().startsWith('ja') ? 'ja' : currentLanguage.toLowerCase().startsWith('en') ? 'en' : 'zh'
    const displayName = (variable: (typeof variableItems)[number]) => {
      const localized = locale === 'ja' ? variable.display_name_ja : locale === 'en' ? variable.display_name_en : variable.display_name
      return (localized || variable.display_name || variable.display_name_en || variable.display_name_ja || variable.var_name).trim()
    }
    const score = (variable: (typeof variableItems)[number]) => {
      const label = displayName(variable)
      if (!label) return 0
      if (label === variable.var_name) return 1
      if (/^[A-Za-z0-9_-]+$/.test(label)) return 2
      return 3
    }
    const byVarName = new Map<string, (typeof variableItems)[number]>()
    for (const variable of variableItems) {
      const varName = variable.var_name.trim()
      if (!varName) continue
      const current = byVarName.get(varName)
      if (!current || score(variable) > score(current)) {
        byVarName.set(varName, variable)
      }
    }
    return Array.from(byVarName.values())
      .sort((left, right) => displayName(left).localeCompare(displayName(right), currentLanguage))
      .map((variable) => ({
        label: displayName(variable),
        value: variable.var_name,
      }))
  }, [currentLanguage, variableItems])

  const parseMutation = useMutation({
    mutationFn: () => {
      if (!selectedFile) throw new Error(t('reportSettings.planImport.fileRequired'))
      return parseMainReportPlanImport(selectedFile, undefined, mappingPayload)
    },
    onSuccess: (draft) => {
      setParsedRows(draft.rows)
      messageApi.success(t('reportSettings.planImport.parsed', { count: draft.rows.length }))
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.planImport.parseFailed')),
  })

  const draft = parseMutation.data
  const confirmableRows = useMemo(() => parsedRows, [parsedRows])

  const confirmMutation = useMutation({
    mutationFn: () =>
      confirmMainReportPlanImport({
        rows: confirmableRows,
        allow_needs_confirmation: allowNeedsConfirmation,
      }),
    onSuccess: (result) => {
      messageApi.success(t('reportSettings.planImport.confirmed', { standards: result.created_standards, plans: result.created_plans }))
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.planImport.confirmFailed')),
  })

  const uploadProps: UploadProps = {
    accept: '.xlsx',
    maxCount: 1,
    showUploadList: false,
    beforeUpload: (file) => {
      setSelectedFile(file as File)
      parseMutation.reset()
      confirmMutation.reset()
      setParsedRows([])
      setPickedCellValues({})
      cancelCellPick()
      return false
    },
    onRemove: () => {
      setSelectedFile(null)
      parseMutation.reset()
      setParsedRows([])
      setWorkbookPreview(null)
      setPickedCellValues({})
      cancelCellPick()
    },
  }

  const updateParsedRowCheckEnabled = (rowNumber: number, checkEnabled: boolean) => {
    setParsedRows((current) => current.map((row) => (row.row_number === rowNumber ? { ...row, check_enabled: checkEnabled } : row)))
  }

  const updateParsedRowLimit = (rowNumber: number, field: 'limit_l' | 'limit_h', value: number | null) => {
    setParsedRows((current) =>
      current.map((row) =>
        row.row_number === rowNumber
          ? {
              ...row,
              needs_confirm: false,
              limit: {
                ...row.limit,
                [field]: value ?? undefined,
                needs_confirmation: false,
                error: undefined,
              },
            }
          : row,
      ),
    )
  }

  if (!isMainServer) {
    return (
      <div className="reports-page">
        <Alert type="info" showIcon message={t('reportSettings.mainServerOnly')} />
      </div>
    )
  }

  return (
    <div className="reports-page">
      {contextHolder}
      <section className="report-plan-command-bar">
        <div className="report-plan-title-block">
          <span className="report-eyebrow">{t('reportSettings.eyebrow')}</span>
          <h1>{t('reportSettings.planImport.title')}</h1>
          <p>{t('reportSettings.planImport.subtitle')}</p>
        </div>
        <div className="report-plan-command-actions">
          <Upload {...uploadProps}>
            <Button icon={<UploadCloud size={14} />}>{t('reportSettings.planImport.pickFile')}</Button>
          </Upload>
          {selectedFile ? (
            <span className="report-plan-file-chip">
              <FileSpreadsheet size={14} />
              <span>{selectedFile.name}</span>
              <Button
                type="text"
                size="small"
                icon={<Trash2 size={13} />}
                onClick={() => {
                  setSelectedFile(null)
                  parseMutation.reset()
                  confirmMutation.reset()
                  setParsedRows([])
                  setWorkbookPreview(null)
                  setPickedCellValues({})
                  cancelCellPick()
                }}
              />
            </span>
          ) : null}
          <Button type="primary" icon={<FileSpreadsheet size={14} />} disabled={!selectedFile} loading={parseMutation.isPending} onClick={() => parseMutation.mutate()}>
            {t('reportSettings.planImport.parse')}
          </Button>
          <Checkbox checked={allowNeedsConfirmation} onChange={(event) => setAllowNeedsConfirmation(event.target.checked)}>
            {t('reportSettings.planImport.allowNeedsConfirmation')}
          </Checkbox>
          <Button
            icon={<CheckCircle2 size={14} />}
            disabled={!draft || confirmableRows.length === 0}
            loading={confirmMutation.isPending}
            onClick={() => confirmMutation.mutate()}
          >
            {t('reportSettings.planImport.confirm')}
          </Button>
        </div>
      </section>

      {selectedFile ? (
        <section className="report-panel report-plan-mapping-panel">
          <div className="report-panel-heading report-plan-mapping-heading">
            <div>
              <Typography.Title level={4}>{t('reportSettings.planImport.mapping.title')}</Typography.Title>
              <Typography.Text type="secondary">{t('reportSettings.planImport.mapping.subtitle')}</Typography.Text>
            </div>
            <Checkbox checked={useCellMapping} onChange={(event) => setUseCellMapping(event.target.checked)}>
              {t('reportSettings.planImport.mapping.enable')}
            </Checkbox>
          </div>
          {previewError ? <Alert className="report-alert" type="warning" showIcon message={previewError} /> : null}
          <div className="report-plan-mapping-grid">
            <div className="report-plan-mapping-form">
              <div className="report-plan-mapping-stack">
                  {workbookPreview ? (
                    <>
                    <div className="report-plan-common-block">
                      <Select
                        value={workbookPreview.activeSheet}
                        options={workbookPreview.sheets.map((sheet) => ({ label: sheet, value: sheet }))}
                        onChange={(sheet) => {
                          if (!selectedFile) return
                          readWorkbookPreview(selectedFile, sheet)
                            .then(setWorkbookPreview)
                            .catch((error) => setPreviewError(error instanceof Error ? error.message : String(error)))
                        }}
                      />
                      <div className="report-plan-common-grid">
                        {commonFieldKeys.map((key) => {
                          const target: CellPickTarget = { scope: 'common', key }
                          const source = pickedCellSourceAddress(target)
                          return (
                            <div className="report-cell-picker-field" key={key}>
                              <Input
                                addonBefore={t(`reportSettings.planImport.mapping.fields.${key}`)}
                                addonAfter={cellPickerAddon(target, source)}
                                value={commonValues[key]}
                                placeholder={t('reportSettings.planImport.mapping.valuePlaceholder')}
                                onChange={(event) => setCommonValues((current) => ({ ...current, [key]: event.target.value }))}
                              />
                            </div>
                          )
                        })}
                      </div>
                    </div>
                    <div className="report-plan-variable-block">
                      <button className="report-plan-add-variable-card" type="button" onClick={() => setMappingRows((current) => [...current, nextMappingRow()])}>
                        <Plus size={16} />
                        <span>{t('reportSettings.planImport.mapping.addRow')}</span>
                      </button>
                      <div className="report-plan-row-list">
                        {mappingRows.map((row, index) => (
                          <div className="report-plan-row-config" key={row.id}>
                            <div className="report-plan-row-config-head">
                              <strong>{t('reportSettings.planImport.review.title')} {index + 1}</strong>
                              <Checkbox
                                checked={row.checkEnabled}
                                onChange={(event) =>
                                  setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, checkEnabled: event.target.checked } : item)))
                                }
                              >
                                {t('reportSettings.planImport.mapping.fields.check_enabled')}
                              </Checkbox>
                            </div>
                            <Select
                              className="report-plan-row-wide"
                              showSearch
                              allowClear
                              loading={variablesQuery.isFetching}
                              placeholder={t('reportSettings.planImport.mapping.fields.systemVariable')}
                              value={row.varName || undefined}
                              options={variableOptions}
                              optionFilterProp="label"
                              filterOption={false}
                              onSearch={setVariableSearch}
                              onChange={(value) =>
                                setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, varName: value || '' } : item)))
                              }
                            />
                            <div className="report-plan-row-limit-stack">
                              {(() => {
                                const target: CellPickTarget = { scope: 'row', rowId: row.id, field: 'limitH' }
                                return (
                                  <div className="report-cell-picker-field">
                                    <Input
                                      addonBefore={t('reportSettings.planImport.mapping.fields.limit_h')}
                                      addonAfter={cellPickerAddon(target, pickedCellSourceAddress(target))}
                                      value={row.limitHValue}
                                      placeholder={t('reportSettings.planImport.mapping.valuePlaceholder')}
                                      onChange={(event) =>
                                        setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, limitHValue: event.target.value } : item)))
                                      }
                                    />
                                  </div>
                                )
                              })()}
                              <div className="report-plan-row-limit-divider">{t('reportSettings.planImport.review.range')}</div>
                              {(() => {
                                const target: CellPickTarget = { scope: 'row', rowId: row.id, field: 'limitL' }
                                return (
                                  <div className="report-cell-picker-field">
                                    <Input
                                      addonBefore={t('reportSettings.planImport.mapping.fields.limit_l')}
                                      addonAfter={cellPickerAddon(target, pickedCellSourceAddress(target))}
                                      value={row.limitLValue}
                                      placeholder={t('reportSettings.planImport.mapping.valuePlaceholder')}
                                      onChange={(event) =>
                                        setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, limitLValue: event.target.value } : item)))
                                      }
                                    />
                                  </div>
                                )
                              })()}
                            </div>
                            {(() => {
                              const target: CellPickTarget = { scope: 'row', rowId: row.id, field: 'unit' }
                              return (
                                <div className="report-cell-picker-field">
                                  <Input
                                    addonBefore={t('reportSettings.planImport.mapping.fields.unit')}
                                    addonAfter={cellPickerAddon(target, pickedCellSourceAddress(target))}
                                    value={row.unitValue}
                                    placeholder={t('reportSettings.planImport.mapping.valuePlaceholder')}
                                    onChange={(event) =>
                                      setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, unitValue: event.target.value } : item)))
                                    }
                                  />
                                </div>
                              )
                            })()}
                            {(() => {
                              const target: CellPickTarget = { scope: 'row', rowId: row.id, field: 'formulaJson' }
                              return (
                                <div className="report-cell-picker-field">
                                  <Input
                                    addonBefore={t('reportSettings.planImport.mapping.fields.formula_json')}
                                    addonAfter={cellPickerAddon(target, pickedCellSourceAddress(target))}
                                    value={row.formulaJsonValue}
                                    placeholder={t('reportSettings.planImport.mapping.valuePlaceholder')}
                                    onChange={(event) =>
                                      setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, formulaJsonValue: event.target.value } : item)))
                                    }
                                  />
                                </div>
                              )
                            })()}
                            <Button
                              icon={<Trash2 size={14} />}
                              disabled={mappingRows.length <= 1}
                              onClick={() => setMappingRows((current) => current.filter((item) => item.id !== row.id))}
                            >
                              {t('common.delete')} {index + 1}
                            </Button>
                          </div>
                        ))}
                      </div>
                    </div>
                    </>
                  ) : null}
                    {draft ? (
                      <div className="report-plan-result-panel">
                        <div className="report-plan-result-summary">
                          <Statistic title={t('reportSettings.planImport.summary.total')} value={draft.summary.total_rows} />
                          <Statistic title={t('reportSettings.planImport.summary.ready')} value={draft.summary.ready_rows} />
                          <Statistic title={t('reportSettings.planImport.summary.issues')} value={draft.summary.rows_with_issues} />
                          <Statistic title={t('reportSettings.planImport.summary.confirm')} value={draft.summary.needs_confirmation} />
                        </div>
                        {draft.issues?.length ? (
                          <Alert
                            className="report-alert"
                            type="warning"
                            showIcon
                            message={t('reportSettings.planImport.issueSummary', { count: draft.issues.length })}
                            description={issueText(draft.issues)}
                          />
                        ) : null}
                        {confirmMutation.data ? (
                          <Alert
                            className="report-alert"
                            type="success"
                            showIcon
                            message={t('reportSettings.planImport.resultTitle')}
                            description={
                              <Space direction="vertical" size={6}>
                                <Typography.Text>
                                  {t('reportSettings.planImport.resultDesc', {
                                    standards: confirmMutation.data.created_standards,
                                    plans: confirmMutation.data.created_plans,
                                    status: confirmMutation.data.plan_creation_status,
                                  })}
                                </Typography.Text>
                                <Space wrap>
                                  {confirmMutation.data.standards.map((standard) => (
                                    <Link key={`standard-${standard.id}`} to="/detection-config">
                                      {t('reportSettings.planImport.openStandard', { code: standard.standard_code })}
                                    </Link>
                                  ))}
                                  {confirmMutation.data.plans.map((plan) => (
                                    <Link key={`plan-${plan.id}`} to="/history/plans">
                                      {t('reportSettings.planImport.openPlan', { code: plan.plan_no })}
                                    </Link>
                                  ))}
                                </Space>
                              </Space>
                            }
                          />
                        ) : null}
                        <div className="report-plan-card-grid" aria-label={t('reportSettings.planImport.review.title')}>
                          {parsedRows.map((record) => {
                            const issues = issueText(record.issues)
                            const reviewState = rowReviewState(record)
                            const unit = record.limit.unit || record.unit || ''
                            return (
                              <article className={`report-plan-review-card ${reviewState}`} key={record.row_number}>
                                <div className="report-plan-review-head">
                                  <div>
                                    <Typography.Text strong className="report-plan-review-title">
                                      {record.variable_match?.display_name || record.variable_match?.var_name || record.variable_raw || '-'}
                                    </Typography.Text>
                                    <span className="report-plan-review-subtitle">
                                      {record.variable_match?.var_name || record.variable_raw || '-'}
                                    </span>
                                  </div>
                                  {issues ? (
                                    <Tag color="red">{t('reportSettings.planImport.review.issue')}</Tag>
                                  ) : reviewState === 'confirm' ? (
                                    <Tag color="gold">{t('reportSettings.planImport.needsConfirmation')}</Tag>
                                  ) : reviewState === 'no-limit' ? (
                                    <Tag color="default">{t('reportSettings.planImport.review.noLimit')}</Tag>
                                  ) : (
                                    <Tag color="green">{t('reportSettings.planImport.ready')}</Tag>
                                  )}
                                </div>
                                <div className="report-plan-review-meta">
                                  <span>{t('reportSettings.planImport.review.excelRow', { row: record.row_number })}</span>
                                  <span>{record.project_match?.project_code || record.project_code || '-'}</span>
                                </div>
                                <div className="report-plan-limit-stack">
                                  <label className="report-plan-limit-cell upper">
                                    <span>{t('reportSettings.planImport.review.upper')}</span>
                                    <InputNumber
                                      value={record.limit.limit_h ?? null}
                                      placeholder={t('reportSettings.planImport.review.optional')}
                                      onChange={(value) => updateParsedRowLimit(record.row_number, 'limit_h', value)}
                                      controls={false}
                                    />
                                    <em>{unit}</em>
                                  </label>
                                  <div className="report-plan-limit-band">
                                    <strong>{t('reportSettings.planImport.review.range')}</strong>
                                    <span>
                                      {record.limit.limit_l === undefined && record.limit.limit_h === undefined
                                        ? t('reportSettings.planImport.review.noLimitHint')
                                        : `${limitValueText(record.limit.limit_l)} - ${limitValueText(record.limit.limit_h)} ${unit}`}
                                    </span>
                                  </div>
                                  <label className="report-plan-limit-cell lower">
                                    <span>{t('reportSettings.planImport.review.lower')}</span>
                                    <InputNumber
                                      value={record.limit.limit_l ?? null}
                                      placeholder={t('reportSettings.planImport.review.optional')}
                                      onChange={(value) => updateParsedRowLimit(record.row_number, 'limit_l', value)}
                                      controls={false}
                                    />
                                    <em>{unit}</em>
                                  </label>
                                </div>
                                <div className="report-plan-review-foot">
                                  <Checkbox checked={record.check_enabled ?? true} onChange={(event) => updateParsedRowCheckEnabled(record.row_number, event.target.checked)}>
                                    {t('reportSettings.planImport.mapping.fields.check_enabled')}
                                  </Checkbox>
                                  {record.formula_json ? <span>{t('reportSettings.planImport.columns.formula')}: {record.formula_json}</span> : null}
                                  {record.params ? <span>{t('reportSettings.planImport.columns.params')}: {JSON.stringify(record.params)}</span> : null}
                                </div>
                              </article>
                            )
                          })}
                        </div>
                      </div>
                    ) : null}
                </div>
            </div>
            <div className={`report-plan-preview report-plan-luckysheet-preview${cellPickTarget ? ' picking' : ''}`}>
              {cellPickTarget ? (
                <div className="report-cell-pick-banner">
                  <span>{t('reportSettings.planImport.mapping.pickCellHint')}</span>
                  <Button size="small" onClick={cancelCellPick}>
                    {t('common.cancel')}
                  </Button>
                </div>
              ) : null}
              <div id="report-plan-import-luckysheet" className="report-luckysheet-host" />
            </div>
          </div>
        </section>
      ) : null}

      {!selectedFile ? (
        <section className="report-panel report-empty-panel">
          <Empty description={t('reportSettings.planImport.empty')} />
        </section>
      ) : null}
    </div>
  )
}
