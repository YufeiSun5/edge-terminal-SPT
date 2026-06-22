import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import ExcelJS from 'exceljs'
import { Alert, Button, Checkbox, Empty, Input, Select, Space, Statistic, Tag, Typography, Upload, message } from 'antd'
import type { UploadProps } from 'antd'
import { CheckCircle2, FileSpreadsheet, Plus, Trash2, UploadCloud } from 'lucide-react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { confirmMainReportPlanImport, getVariablesPage, parseMainReportPlanImport } from '@/features/edge-status/api'
import { createLuckysheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
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
  limitH: string
  unit: string
  formulaJson: string
  checkEnabled: boolean
}

const commonFieldKeys = ['project_code', 'params_json', 'test_no', 'factory_no', 'customer_name', 'device_model', 'template_code', 'report_name'] as const

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

function nextMappingRow(): MappingRowConfig {
  return {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    varName: '',
    limitL: '',
    limitH: '',
    unit: '',
    formulaJson: '',
    checkEnabled: true,
  }
}

function issueText(issues?: PlanImportIssue[]) {
  if (!issues?.length) return ''
  return issues.map((issue) => `${issue.field}:${issue.code}`).join('; ')
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
  const [mappingRows, setMappingRows] = useState<MappingRowConfig[]>([nextMappingRow()])
  const [parsedRows, setParsedRows] = useState<PlanImportRow[]>([])
  const adapterRef = useRef<ReturnType<typeof createLuckysheetAdapter> | null>(null)
  const isMainServer = env.runtimeRole === 'main_server'
  const variablesQuery = useQuery({
    queryKey: ['report-plan-import-variables'],
    queryFn: () => getVariablesPage({ enabled: true, limit: 2000 }),
    enabled: isMainServer,
    staleTime: 60_000,
  })
  const currentLanguage = i18n.language || 'zh'
  const variableItems = useMemo(() => variablesQuery.data?.items ?? [], [variablesQuery.data?.items])

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
  }, [selectedFile])

  const mappingPayload = useMemo<PlanImportCellMapping | undefined>(() => {
    if (!useCellMapping || !workbookPreview) return undefined
    const common = Object.fromEntries(Object.entries(commonCells).filter(([, cell]) => cell.trim() !== ''))
    const rows = mappingRows
      .map((row) => {
        const fields: Record<string, string> = {}
        if (row.limitL.trim()) fields.limit_l = row.limitL.trim()
        if (row.limitH.trim()) fields.limit_h = row.limitH.trim()
        if (row.unit.trim()) fields.unit = row.unit.trim()
        if (row.formulaJson.trim()) fields.formula_json = row.formulaJson.trim()
        const values: Record<string, string> = {}
        if (row.varName.trim()) values.var_name = row.varName.trim()
        values.check_enabled = row.checkEnabled ? 'true' : 'false'
        return {
          row_number: row.rowNumber,
          fields,
          values,
        }
      })
      .filter((row) => Object.keys(row.values ?? {}).length > 0 || Object.keys(row.fields ?? {}).length > 0)
    if (!rows.length) return undefined
    return { sheet: workbookPreview.activeSheet, common, rows }
  }, [commonCells, mappingRows, useCellMapping, workbookPreview])
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
    beforeUpload: (file) => {
      setSelectedFile(file as File)
      parseMutation.reset()
      confirmMutation.reset()
      setParsedRows([])
      return false
    },
    onRemove: () => {
      setSelectedFile(null)
      parseMutation.reset()
      setParsedRows([])
      setWorkbookPreview(null)
    },
  }

  const updateParsedRowCheckEnabled = (rowNumber: number, checkEnabled: boolean) => {
    setParsedRows((current) => current.map((row) => (row.row_number === rowNumber ? { ...row, check_enabled: checkEnabled } : row)))
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
      <header className="report-toolbar">
        <div>
          <span className="report-eyebrow">{t('reportSettings.eyebrow')}</span>
          <h1>{t('reportSettings.planImport.title')}</h1>
          <p>{t('reportSettings.planImport.subtitle')}</p>
        </div>
      </header>

      <section className="report-enqueue-bar">
        <Space wrap>
          <Upload {...uploadProps}>
            <Button icon={<UploadCloud size={14} />}>{t('reportSettings.planImport.pickFile')}</Button>
          </Upload>
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
        </Space>
      </section>

      {selectedFile ? (
        <section className="report-panel report-plan-mapping-panel">
          <div className="report-panel-heading">
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
              {workbookPreview ? (
                <Space direction="vertical" size={12}>
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
                      {commonFieldKeys.map((key) => (
                        <Input
                          key={key}
                          addonBefore={t(`reportSettings.planImport.mapping.fields.${key}`)}
                          value={commonCells[key]}
                          placeholder="B2"
                          onChange={(event) => setCommonCells((current) => ({ ...current, [key]: event.target.value }))}
                        />
                      ))}
                    </div>
                    <div className="report-plan-row-list">
                      {mappingRows.map((row, index) => (
                        <div className="report-plan-row-config" key={row.id}>
                          <Select
                            className="report-plan-row-wide"
                            showSearch
                            allowClear
                            loading={variablesQuery.isFetching}
                            placeholder={t('reportSettings.planImport.mapping.fields.systemVariable')}
                            value={row.varName || undefined}
                            options={variableOptions}
                            optionFilterProp="label"
                            onChange={(value) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, varName: value || '' } : item)))
                            }
                          />
                          <Input
                            addonBefore={t('reportSettings.planImport.mapping.fields.limit_l')}
                            value={row.limitL}
                            placeholder="C12"
                            onChange={(event) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, limitL: event.target.value } : item)))
                            }
                          />
                          <Input
                            addonBefore={t('reportSettings.planImport.mapping.fields.limit_h')}
                            value={row.limitH}
                            placeholder="D12"
                            onChange={(event) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, limitH: event.target.value } : item)))
                            }
                          />
                          <Input
                            addonBefore={t('reportSettings.planImport.mapping.fields.unit')}
                            value={row.unit}
                            placeholder="E12"
                            onChange={(event) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, unit: event.target.value } : item)))
                            }
                          />
                          <Input
                            addonBefore={t('reportSettings.planImport.mapping.fields.formula_json')}
                            value={row.formulaJson}
                            placeholder="F12"
                            onChange={(event) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, formulaJson: event.target.value } : item)))
                            }
                          />
                          <Checkbox
                            checked={row.checkEnabled}
                            onChange={(event) =>
                              setMappingRows((current) => current.map((item) => (item.id === row.id ? { ...item, checkEnabled: event.target.checked } : item)))
                            }
                          >
                            {t('reportSettings.planImport.mapping.fields.check_enabled')}
                          </Checkbox>
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
                    <Button icon={<Plus size={14} />} onClick={() => setMappingRows((current) => [...current, nextMappingRow()])}>
                      {t('reportSettings.planImport.mapping.addRow')}
                    </Button>
                    <Button
                      type="primary"
                      icon={<FileSpreadsheet size={14} />}
                      disabled={!selectedFile}
                      loading={parseMutation.isPending}
                      onClick={() => parseMutation.mutate()}
                    >
                      {t('reportSettings.planImport.mapping.read')}
                    </Button>
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
                        <div className="report-plan-result-list">
                          {parsedRows.map((record) => {
                            const issues = issueText(record.issues)
                            return (
                              <div className="report-plan-result-row" key={record.row_number}>
                                <div className="report-plan-result-head">
                                  <Typography.Text strong>
                                    {record.variable_match?.display_name || record.variable_match?.var_name || record.variable_raw || '-'}
                                  </Typography.Text>
                                  {issues ? (
                                    <Tag color="red">{issues}</Tag>
                                  ) : record.needs_confirm || record.limit.needs_confirmation ? (
                                    <Tag color="gold">{t('reportSettings.planImport.needsConfirmation')}</Tag>
                                  ) : (
                                    <Tag color="green">{t('reportSettings.planImport.ready')}</Tag>
                                  )}
                                </div>
                                <div className="report-plan-result-meta">
                                  <span>{record.variable_match?.var_name || record.variable_raw || '-'}</span>
                                  <span>{record.project_match?.project_code || record.project_code || '-'}</span>
                                  <span>
                                    {t('reportSettings.planImport.columns.limit')}: {record.limit.limit_l ?? '-'} / {record.limit.limit_h ?? '-'}{' '}
                                    {record.limit.unit || record.unit || ''}
                                  </span>
                                  {record.formula_json ? <span>{t('reportSettings.planImport.columns.formula')}: {record.formula_json}</span> : null}
                                  {record.params ? <span>{t('reportSettings.planImport.columns.params')}: {JSON.stringify(record.params)}</span> : null}
                                </div>
                                <Checkbox checked={record.check_enabled ?? true} onChange={(event) => updateParsedRowCheckEnabled(record.row_number, event.target.checked)}>
                                  {t('reportSettings.planImport.mapping.fields.check_enabled')}
                                </Checkbox>
                              </div>
                            )
                          })}
                        </div>
                      </div>
                    ) : null}
                </Space>
              ) : null}
            </div>
            <div className="report-plan-preview report-plan-luckysheet-preview">
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
