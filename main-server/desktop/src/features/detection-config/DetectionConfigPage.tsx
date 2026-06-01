import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import { Edit3, Plus, Save, Trash2 } from 'lucide-react'
import { queryClient } from '@/app/queryClient'
import {
  createDetectionStandard,
  deleteDetectionStandard,
  getDetectionStandard,
  getDetectionStandards,
  getProjects,
  getReportTemplates,
  getVariables,
  replaceDetectionStandardItems,
  updateDetectionStandard,
} from '@/features/edge-status/api'
import type { DetectionStandard, DetectionStandardItemPayload, DetectionStandardPayload, Project, ReportTemplate, VarIdentifier, VariableConfig } from '@/shared/api/types'
import '@/features/settings/settings.css'
import './detection-config.css'

type DetectionStandardFormValues = DetectionStandardPayload

const LEGACY_DETECTION_ITEMS = [
  '吸入口表面积',
  '吹出口温度',
  '吹出口湿度',
  '吸入风量',
  '设备噪音',
  '震动位移',
  '吸入口温度',
  '吸入口湿度',
  '压缩机吸入管温度',
  '压缩机吐出口温度',
  '蒸发器出口温度',
  '冷凝器出口温度',
  '膨胀阀出口温度',
  '冷却水入口温度',
  '冷却水出口温度',
  '加湿器给水口温度',
  '再热器出口温度',
  '干燥过滤器入口温度',
  '干燥过滤器出口温度',
]

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  if (language === 'en') return variable.display_name_en || variable.display_name || variable.raw_name || variable.var_name
  if (language === 'ja') return variable.display_name_ja || variable.display_name || variable.raw_name || variable.var_name
  return variable.display_name || variable.raw_name || variable.var_name
}

function standardItemTitle(item: DetectionStandardItemPayload, language?: string) {
  if (language === 'en') return item.display_name_en || item.display_name || item.var_name
  if (language === 'ja') return item.display_name_ja || item.display_name || item.var_name
  return item.display_name || item.var_name
}

function variableWireId(variable: Pick<VariableConfig, 'var_id' | 'var_id_text'>): string {
  return variable.var_id_text ?? String(variable.var_id)
}

function varKey(value?: VarIdentifier | null) {
  return value === undefined || value === null || value === '' ? '' : String(value)
}

function sameVarId(left?: VarIdentifier | null, right?: VarIdentifier | null) {
  return varKey(left) === varKey(right)
}

function standardProjectId(standard: Pick<DetectionStandard, 'project_id' | 'device_id'>) {
  return standard.project_id ?? standard.device_id
}

function standardProjectCode(standard: Pick<DetectionStandard, 'project_code' | 'device_code'>) {
  return standard.project_code || standard.device_code
}

function projectCode(project?: Pick<Project, 'project_code' | 'device_code'>) {
  return project?.project_code || project?.device_code || ''
}

function reportTemplateTitle(template?: Pick<ReportTemplate, 'template_code' | 'display_name' | 'name'>) {
  if (!template) return ''
  return template.display_name || template.name || template.template_code
}

function normalizeStandardItems(items: DetectionStandard['items'] = []): DetectionStandardItemPayload[] {
  return items.map((item) => ({
    var_id: item.var_id_text ?? item.var_id,
    var_name: item.var_name,
    display_name: item.display_name,
    display_name_en: item.display_name_en,
    display_name_ja: item.display_name_ja,
    check_enabled: item.check_enabled,
    alarm_enabled: item.alarm_enabled,
    store_enabled: item.store_enabled,
    check_cycle_ms: item.check_cycle_ms,
    check_on_start: item.check_on_start,
    required: item.required,
    check_method: item.check_method,
    target_value: item.target_value,
    limit_ll: item.limit_ll ?? null,
    limit_l: item.limit_l ?? null,
    limit_h: item.limit_h ?? null,
    limit_hh: item.limit_hh ?? null,
    limit_deadband: item.limit_deadband,
    violation_hold_ms: item.violation_hold_ms,
    recover_hold_ms: item.recover_hold_ms,
    quality_policy: item.quality_policy,
    unit: item.unit,
    decimal_places: item.decimal_places,
    sort_order: item.sort_order,
  }))
}

export function DetectionConfigPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [selectedStandardId, setSelectedStandardId] = useState<number | undefined>()
  const [editingStandard, setEditingStandard] = useState<DetectionStandard | undefined>()
  const [standardItems, setStandardItems] = useState<DetectionStandardItemPayload[]>([])
  const [draftStandardId, setDraftStandardId] = useState<number | undefined>()
  const [standardVariableId, setStandardVariableId] = useState<VarIdentifier | undefined>()
  const [standardModalOpen, setStandardModalOpen] = useState(false)
  const [standardForm] = Form.useForm<DetectionStandardFormValues>()

  const standardsQuery = useQuery({
    queryKey: ['detection-config', 'standards'],
    queryFn: () => getDetectionStandards(),
    retry: false,
  })
  const projectsQuery = useQuery({
    queryKey: ['detection-config', 'projects'],
    queryFn: getProjects,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['detection-config', 'variables'],
    queryFn: () => getVariables(),
    retry: false,
  })
  const reportTemplatesQuery = useQuery({
    queryKey: ['detection-config', 'report-templates'],
    queryFn: () => getReportTemplates({ enabled: true }),
    retry: false,
  })

  const standards = useMemo(() => standardsQuery.data ?? [], [standardsQuery.data])
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const reportTemplates = useMemo(() => reportTemplatesQuery.data ?? [], [reportTemplatesQuery.data])
  const standardVariables = useMemo(() => {
    const byName = new Map<string, VariableConfig>()
    variables.forEach((variable) => {
      const key = variable.var_name || variable.raw_name
      if (!key) return
      const existing = byName.get(key)
      const existingScore = existing ? Number(Boolean(existing.display_name)) + Number(Boolean(existing.display_name_en)) + Number(Boolean(existing.display_name_ja)) : -1
      const nextScore = Number(Boolean(variable.display_name)) + Number(Boolean(variable.display_name_en)) + Number(Boolean(variable.display_name_ja))
      if (!existing || nextScore > existingScore) {
        byName.set(key, variable)
      }
    })
    return Array.from(byName.values()).sort((left, right) => variableTitle(left, i18n.resolvedLanguage).localeCompare(variableTitle(right, i18n.resolvedLanguage), i18n.resolvedLanguage))
  }, [i18n.resolvedLanguage, variables])
  const selectedStandard = standards.find((item) => item.id === selectedStandardId) ?? standards[0]
  const selectedStandardDetailQuery = useQuery({
    queryKey: ['detection-config', 'standard-detail', selectedStandard?.id],
    queryFn: () => getDetectionStandard(selectedStandard?.id as number),
    enabled: Boolean(selectedStandard?.id),
    retry: false,
  })
  const selectedStandardDetail = selectedStandardDetailQuery.data ?? selectedStandard
  const selectedStandardItems = useMemo(() => {
    if (!selectedStandardDetail) return []
    if (draftStandardId === selectedStandardDetail.id) return standardItems
    return normalizeStandardItems(selectedStandardDetail.items)
  }, [draftStandardId, selectedStandardDetail, standardItems])

  const displayProjectName = useCallback((project: Project) => {
    const code = projectCode(project)
    if (i18n.resolvedLanguage === 'en') return project.display_name_en || project.display_name || project.name || code
    if (i18n.resolvedLanguage === 'ja') return project.display_name_ja || project.display_name || project.name || code
    return project.display_name || project.name || code
  }, [i18n.resolvedLanguage])

  const displayStandardName = useCallback((standard: DetectionStandard) => {
    if (i18n.resolvedLanguage === 'en') return standard.display_name_en || standard.display_name || standard.name || standard.standard_code
    if (i18n.resolvedLanguage === 'ja') return standard.display_name_ja || standard.display_name || standard.name || standard.standard_code
    return standard.display_name || standard.name || standard.standard_code
  }, [i18n.resolvedLanguage])

  const standardOptions = useMemo(() => standards.map((standard) => ({
    label: `${displayStandardName(standard)} · ${standard.standard_code}`,
    value: standard.id,
  })), [displayStandardName, standards])
  const projectOptions = useMemo(() => projects.map((project) => ({
    label: `${displayProjectName(project)} · ${projectCode(project)}`,
    value: project.id,
  })), [displayProjectName, projects])
  const reportTemplateOptions = useMemo(() => reportTemplates.map((template) => ({
    label: `${reportTemplateTitle(template)} · ${template.template_code}`,
    value: template.id,
  })), [reportTemplates])

  const standardVariableOptions = useMemo(() => standardVariables.map((variable) => ({
    label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
    value: variableWireId(variable),
  })), [i18n.resolvedLanguage, standardVariables])

  async function openStandardModal(standard?: DetectionStandard) {
    setEditingStandard(standard)
    setStandardVariableId(undefined)
    if (standard) {
      const detail = await getDetectionStandard(standard.id)
      standardForm.setFieldsValue({
        standard_code: detail.standard_code,
        name: detail.name,
        display_name: detail.display_name,
        display_name_en: detail.display_name_en,
        display_name_ja: detail.display_name_ja,
        project_id: standardProjectId(detail),
        project_code: standardProjectCode(detail),
        mode: detail.mode,
        report_template_id: detail.report_template_id,
        version: detail.version,
        enabled: detail.enabled,
        remark: detail.remark,
      })
      setStandardItems(normalizeStandardItems(detail.items))
      setDraftStandardId(detail.id)
    } else {
      standardForm.setFieldsValue({
        standard_code: `STD-${Date.now().toString().slice(-6)}`,
        mode: 'standard',
        version: 1,
        enabled: true,
        report_template_id: undefined,
        remark: '',
      })
      setStandardItems([])
      setDraftStandardId(undefined)
    }
    setStandardModalOpen(true)
  }

  function addStandardItem(variableId?: VarIdentifier) {
    if (!variableId) return
    const variable = standardVariables.find((item) => sameVarId(variableWireId(item), variableId))
    const currentItems = standardModalOpen ? standardItems : selectedStandardItems
    if (!variable || currentItems.some((item) => item.var_name === variable.var_name)) return
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) => [
      ...(standardModalOpen ? items : currentItems),
      {
        var_id: variableWireId(variable),
        var_name: variable.var_name,
        display_name: variable.display_name || variable.raw_name || variable.var_name,
        display_name_en: variable.display_name_en,
        display_name_ja: variable.display_name_ja,
        check_enabled: true,
        alarm_enabled: true,
        store_enabled: true,
        check_cycle_ms: 0,
        check_on_start: true,
        required: true,
        check_method: variable.data_type === 'BOOL' ? 'bool_equals' : 'numeric_range',
        target_value: '',
        limit_ll: null,
        limit_l: null,
        limit_h: null,
        limit_hh: null,
        limit_deadband: 0,
        violation_hold_ms: 0,
        recover_hold_ms: 0,
        quality_policy: 'ignore_bad',
        unit: variable.unit,
        decimal_places: variable.decimal_places,
        sort_order: currentItems.length + 1,
      },
    ])
    setStandardVariableId(undefined)
  }

  function patchStandardItem(varId: VarIdentifier, patch: Partial<DetectionStandardItemPayload>) {
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) => {
      const currentItems = standardModalOpen ? items : selectedStandardItems
      return currentItems.map((item) => sameVarId(item.var_id, varId) ? { ...item, ...patch } : item)
    })
  }

  function removeStandardItem(varId: VarIdentifier) {
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) => {
      const currentItems = standardModalOpen ? items : selectedStandardItems
      return currentItems.filter((item) => !sameVarId(item.var_id, varId)).map((item, index) => ({ ...item, sort_order: index + 1 }))
    })
  }

  const saveStandardMutation = useMutation({
    mutationFn: async (values: DetectionStandardFormValues) => {
      const project = projects.find((item) => item.id === values.project_id)
      const payload: DetectionStandardPayload = {
        ...values,
        project_code: projectCode(project) || values.project_code || '',
        mode: values.mode || 'standard',
        version: editingStandard?.version ?? 1,
        enabled: values.enabled ?? true,
        items: standardItems,
      }
      if (editingStandard) {
        const updated = await updateDetectionStandard(editingStandard.id, payload)
        return replaceDetectionStandardItems(updated.id, standardItems)
      }
      return createDetectionStandard(payload)
    },
    onSuccess: async () => {
      setStandardModalOpen(false)
      setEditingStandard(undefined)
      setStandardItems([])
      setDraftStandardId(undefined)
      setStandardVariableId(undefined)
      standardForm.resetFields()
      messageApi.success(t('settings.messages.standardSaved'))
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveCurrentStandardMutation = useMutation({
    mutationFn: async () => {
      if (!selectedStandardDetail) throw new Error(t('detectionConfig.noSelection'))
      const payload: DetectionStandardPayload = {
        standard_code: selectedStandardDetail.standard_code,
        name: selectedStandardDetail.name,
        display_name: selectedStandardDetail.display_name,
        display_name_en: selectedStandardDetail.display_name_en,
        display_name_ja: selectedStandardDetail.display_name_ja,
        project_id: standardProjectId(selectedStandardDetail),
        project_code: standardProjectCode(selectedStandardDetail),
        mode: selectedStandardDetail.mode || 'standard',
        report_template_id: selectedStandardDetail.report_template_id,
        version: selectedStandardDetail.version ?? 1,
        enabled: selectedStandardDetail.enabled ?? true,
        remark: selectedStandardDetail.remark,
        items: selectedStandardItems,
      }
      await updateDetectionStandard(selectedStandardDetail.id, payload)
      return replaceDetectionStandardItems(selectedStandardDetail.id, selectedStandardItems)
    },
    onSuccess: async () => {
      messageApi.success(t('settings.messages.standardSaved'))
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const deleteStandardMutation = useMutation({
    mutationFn: (standard: DetectionStandard) => deleteDetectionStandard(standard.id),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.standardDeleted'))
      setSelectedStandardId(undefined)
      setDraftStandardId(undefined)
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const editableItemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 210,
      render: (_, record) => (
        <div className="detection-variable-name">
          <strong>{standardItemTitle(record, i18n.resolvedLanguage)}</strong>
          <span>{record.var_name}</span>
        </div>
      ),
    },
    {
      title: t('settings.standards.check'),
      dataIndex: 'check_enabled',
      key: 'check_enabled',
      width: 80,
      render: (_, record) => <Switch size="small" checked={record.check_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { check_enabled: checked })} />,
    },
    {
      title: t('settings.standards.store'),
      dataIndex: 'store_enabled',
      key: 'store_enabled',
      width: 80,
      render: (_, record) => <Switch size="small" checked={record.store_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { store_enabled: checked })} />,
    },
    {
      title: t('settings.standards.alarm'),
      dataIndex: 'alarm_enabled',
      key: 'alarm_enabled',
      width: 80,
      render: (_, record) => <Switch size="small" checked={record.alarm_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { alarm_enabled: checked })} />,
    },
    {
      title: t('settings.standards.checkOnStart'),
      dataIndex: 'check_on_start',
      key: 'check_on_start',
      width: 110,
      render: (_, record) => <Switch size="small" checked={record.check_on_start ?? true} onChange={(checked) => patchStandardItem(record.var_id, { check_on_start: checked })} />,
    },
    {
      title: t('settings.standards.checkCycle'),
      dataIndex: 'check_cycle_ms',
      key: 'check_cycle_ms',
      width: 130,
      render: (_, record) => <InputNumber size="small" min={0} precision={0} value={record.check_cycle_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { check_cycle_ms: value ?? 0 })} />,
    },
    {
      title: t('settings.standards.checkMethod'),
      dataIndex: 'check_method',
      key: 'check_method',
      width: 150,
      render: (_, record) => (
        <Select
          size="small"
          value={record.check_method ?? 'numeric_range'}
          onChange={(value) => patchStandardItem(record.var_id, { check_method: value })}
          options={[
            { label: t('settings.standards.checkMethods.numericRange'), value: 'numeric_range' },
            { label: t('settings.standards.checkMethods.boolEquals'), value: 'bool_equals' },
            { label: t('settings.standards.checkMethods.stringEquals'), value: 'string_equals' },
            { label: t('settings.standards.checkMethods.regex'), value: 'regex' },
          ]}
        />
      ),
    },
    {
      title: t('settings.standards.targetValue'),
      dataIndex: 'target_value',
      key: 'target_value',
      width: 130,
      render: (_, record) => <Input size="small" value={record.target_value ?? ''} onChange={(event) => patchStandardItem(record.var_id, { target_value: event.target.value })} />,
    },
    { title: 'LL', dataIndex: 'limit_ll', key: 'limit_ll', width: 96, render: (_, record) => <InputNumber size="small" value={record.limit_ll ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_ll: value })} /> },
    { title: 'L', dataIndex: 'limit_l', key: 'limit_l', width: 96, render: (_, record) => <InputNumber size="small" value={record.limit_l ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_l: value })} /> },
    { title: 'H', dataIndex: 'limit_h', key: 'limit_h', width: 96, render: (_, record) => <InputNumber size="small" value={record.limit_h ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_h: value })} /> },
    { title: 'HH', dataIndex: 'limit_hh', key: 'limit_hh', width: 96, render: (_, record) => <InputNumber size="small" value={record.limit_hh ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_hh: value })} /> },
    { title: t('settings.standards.limitDeadband'), dataIndex: 'limit_deadband', key: 'limit_deadband', width: 120, render: (_, record) => <InputNumber size="small" min={0} value={record.limit_deadband ?? 0} onChange={(value) => patchStandardItem(record.var_id, { limit_deadband: value ?? 0 })} /> },
    { title: t('settings.standards.violationHold'), dataIndex: 'violation_hold_ms', key: 'violation_hold_ms', width: 130, render: (_, record) => <InputNumber size="small" min={0} value={record.violation_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { violation_hold_ms: value ?? 0 })} /> },
    { title: t('settings.standards.recoverHold'), dataIndex: 'recover_hold_ms', key: 'recover_hold_ms', width: 130, render: (_, record) => <InputNumber size="small" min={0} value={record.recover_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { recover_hold_ms: value ?? 0 })} /> },
    {
      title: t('settings.standards.qualityPolicy'),
      dataIndex: 'quality_policy',
      key: 'quality_policy',
      width: 150,
      render: (_, record) => (
        <Select
          size="small"
          value={record.quality_policy ?? 'ignore_bad'}
          onChange={(value) => patchStandardItem(record.var_id, { quality_policy: value })}
          options={[
            { label: t('settings.standards.qualityPolicies.ignoreBad'), value: 'ignore_bad' },
            { label: t('settings.standards.qualityPolicies.recordInvalid'), value: 'record_invalid' },
            { label: t('settings.standards.qualityPolicies.failOnBad'), value: 'fail_on_bad' },
          ]}
        />
      ),
    },
    { title: t('settings.variables.unit'), dataIndex: 'unit', key: 'unit', width: 80 },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 70,
      render: (_, record) => <Button danger size="small" icon={<Trash2 size={13} />} onClick={() => removeStandardItem(record.var_id)} />,
    },
  ]

  return (
    <div className="detection-config-page">
      {contextHolder}
      <div className="history-ambient-background" aria-hidden="true">
        <div className="history-orb history-orb-1" />
        <div className="history-orb history-orb-2" />
        <div className="history-orb history-orb-3" />
        <div className="history-noise" />
      </div>

      <section className="detection-config-workspace glass-panel">
        <header className="detection-config-hero">
          <div className="detection-standard-toolbar">
            <Select
              className="detection-standard-select"
              showSearch
              value={selectedStandard?.id}
              loading={standardsQuery.isFetching}
              optionFilterProp="label"
              placeholder={t('detectionConfig.selectStandard')}
              onChange={setSelectedStandardId}
              options={standardOptions}
            />
            <Button icon={<Plus size={15} />} type="primary" onClick={() => void openStandardModal()}>
              {t('settings.standards.create')}
            </Button>
          </div>
          <div className="detection-config-actions">
            {selectedStandardDetail ? (
              <>
                <Button icon={<Save size={15} />} type="primary" loading={saveCurrentStandardMutation.isPending} onClick={() => saveCurrentStandardMutation.mutate()}>
                  {t('settings.standards.save')}
                </Button>
                <Button icon={<Edit3 size={15} />} onClick={() => void openStandardModal(selectedStandardDetail)}>
                  {t('settings.standards.edit')}
                </Button>
                <Popconfirm
                  title={t('settings.standards.deleteConfirm', { code: selectedStandardDetail.standard_code })}
                  okText={t('settings.users.delete')}
                  cancelText={t('settings.actions.cancel')}
                  onConfirm={() => deleteStandardMutation.mutate(selectedStandardDetail)}
                >
                  <Button danger icon={<Trash2 size={15} />} loading={deleteStandardMutation.isPending}>
                    {t('settings.users.delete')}
                  </Button>
                </Popconfirm>
              </>
            ) : null}
          </div>
        </header>

        <div className="detection-config-grid">
        <main className="detection-config-main">
          <div className="detection-panel-head">
            <div>
              <span className="settings-eyebrow">{selectedStandardDetail ? selectedStandardDetail.standard_code : t('detectionConfig.noSelection')}</span>
              <h2>{selectedStandardDetail ? displayStandardName(selectedStandardDetail) : t('detectionConfig.selectStandard')}</h2>
            </div>
            {selectedStandardDetail ? (
              <Space>
                <Tag color={selectedStandardDetail.enabled ? 'success' : 'default'}>{selectedStandardDetail.enabled ? t('status.online') : t('status.offline')}</Tag>
                <Tag>{selectedStandardDetail.mode || 'standard'}</Tag>
              </Space>
            ) : null}
          </div>

          {selectedStandardDetail ? (
            <div className="detection-inline-meta">
              <Tag>{standardProjectId(selectedStandardDetail) ? standardProjectCode(selectedStandardDetail) : t('settings.standards.global')}</Tag>
              <Tag>V{selectedStandardDetail.version}</Tag>
              <Tag>{selectedStandardItems.length} {t('settings.standards.items')}</Tag>
              <Tag>{reportTemplateTitle(reportTemplates.find((template) => template.id === selectedStandardDetail.report_template_id)) || t('settings.standards.reportTemplate')}</Tag>
              {selectedStandardDetail.remark ? <Tag>{selectedStandardDetail.remark}</Tag> : null}
            </div>
          ) : null}

          <div className="detection-table-toolbar">
            <div className="settings-standard-item-picker">
              <Select
                showSearch
                allowClear
                value={standardVariableId}
                placeholder={t('settings.standards.addVariable')}
                optionFilterProp="label"
                onChange={setStandardVariableId}
                options={standardVariableOptions}
              />
              <Button size="small" icon={<Plus size={14} />} onClick={() => addStandardItem(standardVariableId)} disabled={!standardVariableId || !selectedStandardDetail}>
                {t('settings.standards.add')}
              </Button>
            </div>
            <div className="detection-legacy-tags">
              {LEGACY_DETECTION_ITEMS.map((item) => <Tag key={item}>{item}</Tag>)}
            </div>
          </div>

          <Table
            className="detection-config-table settings-standard-items-table"
            size="small"
            virtual
            rowKey={(record) => varKey(record.var_id)}
            loading={standardsQuery.isFetching || selectedStandardDetailQuery.isFetching}
            columns={editableItemColumns}
            dataSource={selectedStandardDetail ? selectedStandardItems : []}
            scroll={{ x: 1650, y: 520 }}
            pagination={{
              defaultPageSize: 30,
              pageSizeOptions: [20, 30, 50, 100],
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total) => `${total} ${t('settings.standards.items')}`,
              size: 'small',
            }}
          />
        </main>
        </div>
      </section>

      <Modal
        title={editingStandard ? t('settings.standards.edit') : t('settings.standards.create')}
        open={standardModalOpen}
        width={1120}
        onCancel={() => {
          setStandardModalOpen(false)
          setEditingStandard(undefined)
          setStandardItems([])
          setDraftStandardId(undefined)
          setStandardVariableId(undefined)
          standardForm.resetFields()
        }}
        footer={null}
      >
        <Form form={standardForm} layout="vertical" onFinish={(values) => saveStandardMutation.mutate(values)}>
          <div className="settings-form-grid modal-grid">
            <Form.Item name="standard_code" label={t('settings.standards.code')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="display_name" label={t('settings.standards.displayName')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="name" label={t('settings.standards.internalName')}>
              <Input />
            </Form.Item>
            <Form.Item name="project_id" label={t('settings.variables.selectProject')}>
              <Select allowClear options={projectOptions} />
            </Form.Item>
            <Form.Item name="mode" label={t('settings.standards.mode')}>
              <Input />
            </Form.Item>
            <Form.Item name="report_template_id" label={t('settings.standards.reportTemplate')}>
              <Select
                allowClear
                loading={reportTemplatesQuery.isFetching}
                options={reportTemplateOptions}
              />
            </Form.Item>
            <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item className="settings-form-wide" name="remark" label={t('settings.standards.remark')}>
              <Input.TextArea rows={2} />
            </Form.Item>
          </div>
          <div className="settings-standard-items-head">
            <div>
              <strong>{t('settings.standards.items')}</strong>
              <span>{t('detectionConfig.itemsHint')}</span>
            </div>
            <div className="settings-standard-item-picker">
              <Select
                showSearch
                allowClear
                value={standardVariableId}
                placeholder={t('settings.standards.addVariable')}
                optionFilterProp="label"
                onChange={setStandardVariableId}
                options={standardVariableOptions}
              />
              <Button size="small" icon={<Plus size={14} />} onClick={() => addStandardItem(standardVariableId)} disabled={!standardVariableId}>
                {t('settings.standards.add')}
              </Button>
            </div>
          </div>
          <Table
            className="settings-standard-items-table"
            size="small"
            virtual
            rowKey={(record) => varKey(record.var_id)}
            columns={editableItemColumns}
            dataSource={standardItems}
            scroll={{ x: 1650, y: 320 }}
            pagination={{
              defaultPageSize: 20,
              pageSizeOptions: [20, 30, 50, 100],
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total) => `${total} ${t('settings.standards.items')}`,
              size: 'small',
            }}
          />
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveStandardMutation.isPending}>
              {t('settings.standards.save')}
            </Button>
          </div>
        </Form>
      </Modal>
    </div>
  )
}
