import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, Button, Checkbox, Form, Input, InputNumber, Modal, Pagination, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import { Edit3, Plus, RotateCcw, Save } from 'lucide-react'
import { queryClient } from '@/app/queryClient'
import { env } from '@/shared/config/env'
import {
  assignVariable,
  bulkRemapKioProjects,
  createVariable,
  getProjects,
  getVariablesPage,
  updateVariable,
} from '@/features/edge-status/api'
import type {
  BulkRemapKioProjectsResult,
  Project,
  VarIdentifier,
  VariableAssignmentPayload,
  VariableConfig,
  VariableCreatePayload,
  VariablePatchPayload,
} from '@/shared/api/types'
import { languageCode } from '@/shared/i18n/language'
import '@/features/settings/settings.css'
import '@/features/detection-config/detection-config.css'
import './variable-config.css'

type VariableEditFormValues = VariablePatchPayload
type VariableAssignFormValues = Pick<VariableAssignmentPayload, 'var_group' | 'enabled'> & { project_id: number }
type VirtualVariableFormValues = VariableCreatePayload & { project_id: number; var_name: string; data_type: string }
type VariableFilter = 'all' | 'known' | 'unknown' | number
type KioProjectRemapFormValues = {
  project_count?: number
  project_code_prefix?: string
  project_display_prefix?: string
  project_en_prefix?: string
  project_ja_prefix?: string
  raw_project_prefix?: string
  var_group?: string
  var_name_prefix?: string
  remap_var_name?: boolean
  enable?: boolean
}
type ProjectGroupRow = {
  row_type: 'project_group'
  group_key: string
  group_title: string
  group_code: string
  group_count: number
}
type VariableTableRow = VariableConfig | ProjectGroupRow

function isProjectGroupRow(row: VariableTableRow): row is ProjectGroupRow {
  return 'row_type' in row && row.row_type === 'project_group'
}

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return variable.display_name_en || variable.var_name || variable.raw_name
  if (currentLanguage === 'ja') return variable.display_name_ja || variable.var_name || variable.raw_name
  return variable.display_name || variable.raw_name || variable.var_name
}

function variableTitleColumn(language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return 'settings.variables.displayNameEn'
  if (currentLanguage === 'ja') return 'settings.variables.displayNameJa'
  return 'settings.variables.displayName'
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

function variableProjectId(variable: Pick<VariableConfig, 'project_id'>) {
  return variable.project_id
}

function variableProjectCode(variable: Pick<VariableConfig, 'project_code'>) {
  return variable.project_code
}

function projectCode(project?: Pick<Project, 'project_code'>) {
  return project?.project_code || ''
}

function normalizeVariableWritePayload<T extends VariableEditFormValues | VirtualVariableFormValues>(values: T) {
  return {
    ...values,
    writable: values.writable ?? false,
    rw_mode: values.rw_mode || 'R',
    write_requires_audit: values.write_requires_audit ?? true,
    default_alarm_enabled: values.default_alarm_enabled ?? false,
    default_limit_deadband: values.default_limit_deadband ?? 0,
    default_violation_hold_ms: values.default_violation_hold_ms ?? 0,
    default_recover_hold_ms: values.default_recover_hold_ms ?? 0,
  }
}

export function VariableConfigPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [viewportHeight, setViewportHeight] = useState(() => window.innerHeight)
  const [variableFilter, setVariableFilter] = useState<VariableFilter>('all')
  const [variableKeyword, setVariableKeyword] = useState('')
  const [variablePage, setVariablePage] = useState(1)
  const [variablePageSize, setVariablePageSize] = useState(100)
  const [selectedVariable, setSelectedVariable] = useState<VariableConfig | undefined>()
  const [selectedUnassignedIds, setSelectedUnassignedIds] = useState<VarIdentifier[]>([])
  const [variableModalOpen, setVariableModalOpen] = useState(false)
  const [batchAssignModalOpen, setBatchAssignModalOpen] = useState(false)
  const [virtualVariableModalOpen, setVirtualVariableModalOpen] = useState(false)
  const [kioRemapModalOpen, setKioRemapModalOpen] = useState(false)
  const [kioRemapResult, setKioRemapResult] = useState<BulkRemapKioProjectsResult | undefined>()
  const [variableEditForm] = Form.useForm<VariableEditFormValues>()
  const [variableAssignForm] = Form.useForm<VariableAssignFormValues>()
  const [batchAssignForm] = Form.useForm<VariableAssignFormValues>()
  const [virtualVariableForm] = Form.useForm<VirtualVariableFormValues>()
  const [kioRemapForm] = Form.useForm<KioProjectRemapFormValues>()

  const variableListParams = useMemo(() => {
    const params = {
      keyword: variableKeyword || undefined,
      limit: variablePageSize,
      offset: (variablePage - 1) * variablePageSize,
    }
    if (variableFilter === 'known') return { ...params, assigned: true }
    if (variableFilter === 'unknown') return { ...params, assigned: false }
    if (typeof variableFilter === 'number') return { ...params, project_id: variableFilter }
    return params
  }, [variableFilter, variableKeyword, variablePage, variablePageSize])

  const variablesQuery = useQuery({
    queryKey: ['variable-config', 'variables', variableListParams],
    queryFn: () => getVariablesPage(variableListParams),
    retry: false,
  })
  const projectsQuery = useQuery({
    queryKey: ['variable-config', 'projects'],
    queryFn: getProjects,
    retry: false,
  })

  const variables = useMemo(() => variablesQuery.data?.items ?? [], [variablesQuery.data])
  const variableTotal = variablesQuery.data?.total ?? 0
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  useEffect(() => {
    const handleResize = () => setViewportHeight(window.innerHeight)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])
  const filteredVariables = variables
  const unassignedVariables = useMemo(() => filteredVariables.filter((variable) => !variableProjectId(variable)), [filteredVariables])
  const selectedUnassignedVariables = useMemo(
    () => unassignedVariables.filter((variable) => selectedUnassignedIds.some((id) => sameVarId(id, variableWireId(variable)))),
    [selectedUnassignedIds, unassignedVariables],
  )
  const projectOptions = useMemo(() => projects.map((project) => ({
    label: `${displayProjectName(project, i18n.resolvedLanguage)} · ${projectCode(project)}`,
    value: project.id,
  })), [i18n.resolvedLanguage, projects])
  const projectGroupedRows = useMemo<VariableTableRow[]>(() => {
    const projectById = new Map(projects.map((project) => [project.id, project]))
    const groups = new Map<string, { title: string; code: string; items: VariableConfig[]; order: number }>()

    filteredVariables.forEach((variable) => {
      const projectId = variableProjectId(variable)
      const project = projectId ? projectById.get(projectId) : undefined
      const key = projectId ? `project:${projectId}` : 'project:unassigned'
      const code = project ? projectCode(project) : t('settings.variables.unassigned')
      const title = project ? displayProjectName(project, i18n.resolvedLanguage) : t('settings.variables.unassignedPool')
      const order = projectId ?? Number.MAX_SAFE_INTEGER
      const group = groups.get(key) ?? { title, code, items: [], order }
      group.items.push(variable)
      groups.set(key, group)
    })

    return Array.from(groups.entries())
      .sort(([, left], [, right]) => left.order - right.order || left.title.localeCompare(right.title, i18n.resolvedLanguage))
      .flatMap(([key, group]) => [
        {
          row_type: 'project_group' as const,
          group_key: key,
          group_title: group.title,
          group_code: group.code,
          group_count: group.items.length,
        },
        ...group.items.sort((left, right) =>
          variableTitle(left, i18n.resolvedLanguage).localeCompare(variableTitle(right, i18n.resolvedLanguage), i18n.resolvedLanguage)
          || left.var_name.localeCompare(right.var_name, i18n.resolvedLanguage),
        ),
      ])
  }, [filteredVariables, i18n.resolvedLanguage, projects, t])

  const saveVariableMutation = useMutation({
    mutationFn: (values: VariableEditFormValues) => {
      if (!selectedVariable) throw new Error(t('settings.variables.noVariable'))
      const payload = normalizeVariableWritePayload(values)
      if (payload.writable && !['W', 'RW'].includes(payload.rw_mode || '')) throw new Error(t('settings.variables.writeModeRequired'))
      if (payload.writable && !payload.write_path?.trim()) throw new Error(t('settings.variables.writePathRequired'))
      if (payload.writable && !payload.write_data_type?.trim()) throw new Error(t('settings.variables.writeDataTypeRequired'))
      return updateVariable(variableWireId(selectedVariable), payload)
    },
    onSuccess: async () => {
      setVariableModalOpen(false)
      setSelectedVariable(undefined)
      messageApi.success(t('settings.messages.variableSaved'))
      await invalidateVariables()
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.variableSaveFailed')),
  })

  const createVirtualVariableMutation = useMutation({
    mutationFn: (values: VirtualVariableFormValues) => {
      const project = projects.find((item) => item.id === values.project_id)
      if (!project) throw new Error(t('settings.messages.selectVariableProject'))
      const payload = normalizeVariableWritePayload(values)
      if (payload.writable && !['W', 'RW'].includes(payload.rw_mode || '')) throw new Error(t('settings.variables.writeModeRequired'))
      if (payload.writable && !payload.write_path?.trim()) throw new Error(t('settings.variables.writePathRequired'))
      if (payload.writable && !payload.write_data_type?.trim()) throw new Error(t('settings.variables.writeDataTypeRequired'))
      return createVariable({
        ...payload,
        project_code: projectCode(project),
        source_type: 'virtual',
        gateway_id: 0,
        source_topic: 'virtual',
        source_path: payload.var_name,
        raw_name: payload.var_name,
        json_path: payload.var_name,
        display_name: payload.display_name || payload.var_name,
        display_name_en: payload.display_name_en,
        display_name_ja: payload.display_name_ja,
      })
    },
    onSuccess: async () => {
      setVirtualVariableModalOpen(false)
      virtualVariableForm.resetFields()
      messageApi.success(t('settings.messages.virtualVariableCreated'))
      await invalidateVariables()
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.virtualVariableCreateFailed')),
  })

  const assignVariableMutation = useMutation({
    mutationFn: (values: VariableAssignFormValues) => {
      if (!selectedVariable) throw new Error(t('settings.variables.noVariable'))
      const project = projects.find((item) => item.id === values.project_id)
      if (!project) throw new Error(t('settings.messages.selectVariableProject'))
      return assignVariable(variableWireId(selectedVariable), {
        project_id: project.id,
        project_code: projectCode(project),
        var_group: values.var_group,
        enabled: values.enabled,
      })
    },
    onSuccess: async () => {
      setVariableModalOpen(false)
      setSelectedVariable(undefined)
      messageApi.success(t('settings.messages.variableAssigned'))
      await invalidateVariables()
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.variableAssignFailed')),
  })

  const batchAssignVariableMutation = useMutation({
    mutationFn: async (values: VariableAssignFormValues) => {
      const project = projects.find((item) => item.id === values.project_id)
      if (!project || selectedUnassignedVariables.length === 0) throw new Error(t('settings.messages.selectVariableProject'))
      await Promise.all(selectedUnassignedVariables.map((variable) =>
        assignVariable(variableWireId(variable), {
          project_id: project.id,
          project_code: projectCode(project),
          var_group: values.var_group,
          enabled: values.enabled,
        }),
      ))
      return selectedUnassignedVariables.length
    },
    onSuccess: async (count) => {
      setBatchAssignModalOpen(false)
      setSelectedUnassignedIds([])
      messageApi.success(t('settings.messages.variablesAssigned', { count }))
      await invalidateVariables()
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.variableAssignFailed')),
  })

  const toggleVariableMutation = useMutation({
    mutationFn: (variable: VariableConfig) => updateVariable(variableWireId(variable), { enabled: !variable.enabled }),
    onSuccess: invalidateVariables,
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.variableSaveFailed')),
  })

  const kioRemapMutation = useMutation({
    mutationFn: ({ values, dryRun }: { values: KioProjectRemapFormValues; dryRun: boolean }) =>
      bulkRemapKioProjects({ ...values, remap_var_name: values.remap_var_name ?? true, enable: values.enable ?? true, dry_run: dryRun }),
    onSuccess: async (result) => {
      setKioRemapResult(result)
      messageApi.success(result.dry_run ? t('settings.variables.kioRemapDryRunDone') : t('settings.variables.kioRemapDone', { count: result.updated }))
      if (!result.dry_run) await invalidateVariables()
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('settings.messages.variableSaveFailed')),
  })

  function invalidateVariables() {
    return Promise.all([
      queryClient.invalidateQueries({ queryKey: ['variable-config', 'variables'] }),
      queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] }),
      queryClient.invalidateQueries({ queryKey: ['detection-config', 'variables'] }),
    ])
  }

  function openVariableModal(variable: VariableConfig) {
    setSelectedVariable(variable)
    if (variableProjectId(variable)) {
      variableEditForm.setFieldsValue({
        var_name: variable.var_name,
        display_name: variable.display_name,
        display_name_en: variable.display_name_en,
        display_name_ja: variable.display_name_ja,
        data_type: variable.data_type,
        unit: variable.unit,
        decimal_places: variable.decimal_places,
        scale_factor: variable.scale_factor,
        offset_val: variable.offset_val,
        rw_mode: variable.rw_mode || 'R',
        writable: variable.writable,
        write_source_id: variable.write_source_id,
        write_path: variable.write_path,
        write_data_type: variable.write_data_type,
        write_min: variable.write_min,
        write_max: variable.write_max,
        write_enum: variable.write_enum,
        write_requires_audit: variable.write_requires_audit,
        suspicious_value: variable.suspicious_value,
        debounce_threshold: variable.debounce_threshold,
        debounce_ms: variable.debounce_ms,
        deadband: variable.deadband,
        default_alarm_enabled: variable.default_alarm_enabled,
        default_limit_ll: variable.default_limit_ll,
        default_limit_l: variable.default_limit_l,
        default_limit_h: variable.default_limit_h,
        default_limit_hh: variable.default_limit_hh,
        default_limit_deadband: variable.default_limit_deadband,
        default_violation_hold_ms: variable.default_violation_hold_ms,
        default_recover_hold_ms: variable.default_recover_hold_ms,
        apply_to_running: false,
        var_group: variable.var_group,
        enabled: variable.enabled,
      })
    } else {
      variableAssignForm.setFieldsValue({ project_id: undefined as unknown as number, var_group: '', enabled: true })
    }
    setVariableModalOpen(true)
  }

  function openVirtualVariableModal() {
    virtualVariableForm.setFieldsValue({
      data_type: 'INT',
      decimal_places: 0,
      scale_factor: 1,
      offset_val: 0,
      rw_mode: 'R',
      writable: false,
      write_source_id: 0,
      write_requires_audit: true,
      debounce_ms: 0,
      deadband: 0,
      default_alarm_enabled: false,
      default_limit_deadband: 0,
      default_violation_hold_ms: 0,
      default_recover_hold_ms: 0,
      enabled: true,
    })
    setVirtualVariableModalOpen(true)
  }

  function openBatchAssignModal() {
    batchAssignForm.setFieldsValue({ project_id: undefined as unknown as number, var_group: '', enabled: true })
    setBatchAssignModalOpen(true)
  }

  function openKioRemapModal() {
    kioRemapForm.setFieldsValue({
      project_count: 8,
      project_code_prefix: 'AC',
      project_display_prefix: '项目',
      project_en_prefix: 'Project ',
      project_ja_prefix: 'プロジェクト',
      raw_project_prefix: '台',
      var_group: 'KIO变量',
      var_name_prefix: 'kio',
      remap_var_name: true,
      enable: true,
    })
    setKioRemapResult(undefined)
    setKioRemapModalOpen(true)
  }

  async function submitKioRemap(dryRun: boolean) {
    const values = await kioRemapForm.validateFields()
    kioRemapMutation.mutate({ values, dryRun })
  }

  function toggleUnassignedSelection(variableId: VarIdentifier) {
    setSelectedUnassignedIds((ids) =>
      ids.some((id) => sameVarId(id, variableId)) ? ids.filter((id) => !sameVarId(id, variableId)) : [...ids, variableId],
    )
  }

  const variableColumns: TableColumnsType<VariableTableRow> = [
    {
      title: 'ID',
      dataIndex: 'var_id',
      key: 'var_id',
      width: 190,
      fixed: 'left',
      render: (_, record) => (
        isProjectGroupRow(record) ? (
          <div className="variable-config-group-label">
            <strong>{record.group_title}</strong>
            <span>{record.group_code} · {record.group_count}</span>
          </div>
        ) : <span className="variable-config-id">{record.var_id_text ?? record.var_id}</span>
      ),
    },
    {
      title: t(variableTitleColumn(i18n.resolvedLanguage)),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      fixed: 'left',
      render: (_, record) => (
        isProjectGroupRow(record) ? null : (
          <button className="detection-link-button" onClick={() => openVariableModal(record)}>
            <strong>{variableTitle(record, i18n.resolvedLanguage)}</strong>
          </button>
        )
      ),
    },
    {
      title: t('settings.variables.varName'),
      dataIndex: 'var_name',
      key: 'var_name',
      width: 180,
      fixed: 'left',
      render: (_, record) => isProjectGroupRow(record) ? null : <span className="variable-config-mapped-name">{record.var_name}</span>,
    },
    { title: t('settings.variables.rawName'), dataIndex: 'raw_name', key: 'raw_name', width: 160, render: (_, record) => isProjectGroupRow(record) ? null : record.raw_name },
    { title: t('settings.variables.path'), dataIndex: 'source_path', key: 'source_path', width: 240, render: (_, record) => isProjectGroupRow(record) ? null : record.source_path },
    { title: t('settings.variables.jsonPath'), dataIndex: 'json_path', key: 'json_path', width: 220, render: (_, record) => isProjectGroupRow(record) ? null : record.json_path },
    { title: 'Gateway', dataIndex: 'gateway_id', key: 'gateway_id', width: 90, render: (_, record) => isProjectGroupRow(record) ? null : record.gateway_id },
    {
      title: t('settings.variables.type'),
      dataIndex: 'data_type',
      key: 'data_type',
      width: 90,
      render: (value, record) => isProjectGroupRow(record) ? null : variableProjectId(record) ? value : <span className="settings-muted">{t('settings.variables.readonly')}</span>,
    },
    {
      title: t('settings.variables.unit'),
      dataIndex: 'unit',
      key: 'unit',
      width: 90,
      render: (value, record) => isProjectGroupRow(record) ? null : variableProjectId(record) ? value : <span className="settings-muted">-</span>,
    },
    {
      title: t('settings.variables.project'),
      dataIndex: 'project_code',
      key: 'project_code',
      width: 130,
      render: (_, record) => isProjectGroupRow(record) ? null : variableProjectId(record) ? variableProjectCode(record) : <Tag>{t('settings.variables.unassigned')}</Tag>,
    },
    { title: t('settings.variables.group'), dataIndex: 'var_group', key: 'var_group', width: 120, render: (_, record) => isProjectGroupRow(record) ? null : record.var_group },
    {
      title: t('settings.variables.writeMode'),
      dataIndex: 'rw_mode',
      key: 'rw_mode',
      width: 110,
      render: (value, record) => isProjectGroupRow(record) ? null : variableProjectId(record) ? <Tag color={record.writable ? 'processing' : 'default'}>{record.writable ? value : 'R'}</Tag> : <span className="settings-muted">-</span>,
    },
    {
      title: t('settings.variables.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      fixed: 'right',
      render: (_, record) => isProjectGroupRow(record) ? null : variableProjectId(record) ? (
        <Switch size="small" checked={record.enabled} loading={toggleVariableMutation.isPending} onChange={() => toggleVariableMutation.mutate(record)} />
      ) : <span className="settings-muted">{t('settings.variables.afterAssign')}</span>,
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 110,
      fixed: 'right',
      render: (_, record) => (
        isProjectGroupRow(record) ? null : (
          <Button size="small" icon={<Edit3 size={14} />} onClick={() => openVariableModal(record)}>
            {variableProjectId(record) ? t('settings.variables.edit') : t('settings.variables.assign')}
          </Button>
        )
      ),
    },
  ]

  const selectedLabel = variableFilter === 'all'
    ? t('settings.groups.allVariables')
    : variableFilter === 'known'
      ? t('settings.variables.known')
      : variableFilter === 'unknown'
        ? t('settings.variables.unknown')
        : displayProjectName(projects.find((project) => project.id === variableFilter), i18n.resolvedLanguage)
  const tableScrollY = Math.max(420, viewportHeight - (unassignedVariables.length ? 270 : 210))

  return (
    <div className="detection-config-page variable-config-page">
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
              value={variableFilter}
              optionFilterProp="label"
              onChange={(value) => {
                setVariableFilter(value)
                setVariablePage(1)
                setSelectedUnassignedIds([])
              }}
              options={[
                { label: variableFilter === 'all' ? `${t('settings.groups.allVariables')} · ${variableTotal}` : t('settings.groups.allVariables'), value: 'all' },
                { label: variableFilter === 'known' ? `${t('settings.variables.known')} · ${variableTotal}` : t('settings.variables.known'), value: 'known' },
                { label: variableFilter === 'unknown' ? `${t('settings.variables.unknown')} · ${variableTotal}` : t('settings.variables.unknown'), value: 'unknown' },
                ...projects.map((project) => ({
                  label: variableFilter === project.id
                    ? `${displayProjectName(project, i18n.resolvedLanguage)} · ${variableTotal}`
                    : `${displayProjectName(project, i18n.resolvedLanguage)} · ${projectCode(project)}`,
                  value: project.id,
                })),
              ]}
            />
            <Input.Search
              allowClear
              className="variable-config-search"
              placeholder={t('settings.variables.search')}
              value={variableKeyword}
              onChange={(event) => {
                setVariableKeyword(event.target.value)
                setVariablePage(1)
                setSelectedUnassignedIds([])
              }}
            />
          </div>
          <div className="detection-config-actions">
            {env.runtimeFeatures.kioManage ? (
              <Button icon={<RotateCcw size={15} />} onClick={openKioRemapModal}>
                {t('settings.variables.kioRemap')}
              </Button>
            ) : null}
            <Button icon={<Plus size={15} />} onClick={openVirtualVariableModal}>
              {t('settings.variables.createVirtual')}
            </Button>
          </div>
        </header>

        <div className="detection-config-grid">
          <main className="detection-config-main">
            <div className="detection-panel-head">
              <div>
                <span className="settings-eyebrow">{t('settings.variables.title')}</span>
                <h2>{selectedLabel}</h2>
              </div>
              <Space>
                <Tag>{variableTotal} {t('settings.groups.allVariables')}</Tag>
                <Tag color={unassignedVariables.length ? 'warning' : 'success'}>
                  {unassignedVariables.length} {t('settings.variables.unassigned')}
                </Tag>
              </Space>
            </div>

            {unassignedVariables.length ? (
              <div className="variable-config-unassigned">
                <div>
                  <strong>{t('settings.variables.unassignedPool')}</strong>
                  <span>{t('settings.variables.unassignedPoolHint')}</span>
                </div>
                <Space wrap>
                  <Tag>{selectedUnassignedVariables.length} / {unassignedVariables.length}</Tag>
                  <Button size="small" onClick={() => setSelectedUnassignedIds(unassignedVariables.map(variableWireId))}>{t('settings.variables.selectAll')}</Button>
                  <Button size="small" disabled={!selectedUnassignedVariables.length} onClick={() => setSelectedUnassignedIds([])}>{t('settings.variables.clearSelection')}</Button>
                  <Button size="small" type="primary" disabled={!selectedUnassignedVariables.length} onClick={openBatchAssignModal}>
                    {t('settings.variables.batchAssign', { count: selectedUnassignedVariables.length })}
                  </Button>
                </Space>
              </div>
            ) : null}

            <Table
              className="detection-config-table variable-config-table"
              size="small"
              virtual
              rowKey={(record) => isProjectGroupRow(record) ? record.group_key : variableWireId(record)}
              rowClassName={(record) => isProjectGroupRow(record) ? 'variable-config-group-row' : ''}
              rowSelection={unassignedVariables.length ? {
                selectedRowKeys: selectedUnassignedIds.map(String),
                getCheckboxProps: (record) => ({ disabled: isProjectGroupRow(record) || Boolean(variableProjectId(record)) }),
                onSelect: (record) => {
                  if (!isProjectGroupRow(record)) toggleUnassignedSelection(variableWireId(record))
                },
                onSelectAll: (selected, selectedRows) => {
                  const selectableRows = selectedRows.filter((item): item is VariableConfig => !isProjectGroupRow(item) && !variableProjectId(item))
                  setSelectedUnassignedIds(selected ? selectableRows.map(variableWireId) : [])
                },
              } : undefined}
              loading={variablesQuery.isFetching}
              columns={variableColumns}
              dataSource={projectGroupedRows}
              scroll={{ x: 1720, y: tableScrollY }}
              pagination={false}
            />
            <div className="variable-config-pagination">
              <Pagination
                size="small"
                current={variablePage}
                pageSize={variablePageSize}
                total={variableTotal}
                showSizeChanger
                showQuickJumper
                pageSizeOptions={[20, 30, 50, 100]}
                showTotal={(total) => `${total} ${t('settings.groups.allVariables')}`}
                onChange={(page, pageSize) => {
                  setVariablePage(page)
                  setVariablePageSize(pageSize)
                  setSelectedUnassignedIds([])
                }}
              />
            </div>
          </main>
        </div>
      </section>

      <VariableEditorModal
        open={variableModalOpen}
        selectedVariable={selectedVariable}
        projectOptions={projectOptions}
        variableEditForm={variableEditForm}
        variableAssignForm={variableAssignForm}
        savePending={saveVariableMutation.isPending}
        assignPending={assignVariableMutation.isPending}
        onCancel={() => {
          setVariableModalOpen(false)
          setSelectedVariable(undefined)
        }}
        onSave={(values) => saveVariableMutation.mutate(values)}
        onAssign={(values) => assignVariableMutation.mutate(values)}
      />

      <VirtualVariableModal
        open={virtualVariableModalOpen}
        projectOptions={projectOptions}
        form={virtualVariableForm}
        pending={createVirtualVariableMutation.isPending}
        onCancel={() => {
          setVirtualVariableModalOpen(false)
          virtualVariableForm.resetFields()
        }}
        onCreate={(values) => createVirtualVariableMutation.mutate(values)}
      />

      <BatchAssignModal
        open={batchAssignModalOpen}
        count={selectedUnassignedVariables.length}
        projectOptions={projectOptions}
        form={batchAssignForm}
        pending={batchAssignVariableMutation.isPending}
        onCancel={() => setBatchAssignModalOpen(false)}
        onAssign={(values) => batchAssignVariableMutation.mutate(values)}
      />

      <KioRemapModal
        open={kioRemapModalOpen}
        form={kioRemapForm}
        result={kioRemapResult}
        pending={kioRemapMutation.isPending}
        onCancel={() => setKioRemapModalOpen(false)}
        onSubmit={submitKioRemap}
      />
    </div>
  )
}

function displayProjectName(project: Project | undefined, language?: string) {
  if (!project) return ''
  const code = projectCode(project)
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return project.display_name_en || code
  if (currentLanguage === 'ja') return project.display_name_ja || code
  return project.display_name || project.name || code
}

function VariableEditorModal({
  open,
  selectedVariable,
  projectOptions,
  variableEditForm,
  variableAssignForm,
  savePending,
  assignPending,
  onCancel,
  onSave,
  onAssign,
}: {
  open: boolean
  selectedVariable?: VariableConfig
  projectOptions: Array<{ label: string; value: number }>
  variableEditForm: ReturnType<typeof Form.useForm<VariableEditFormValues>>[0]
  variableAssignForm: ReturnType<typeof Form.useForm<VariableAssignFormValues>>[0]
  savePending: boolean
  assignPending: boolean
  onCancel: () => void
  onSave: (values: VariableEditFormValues) => void
  onAssign: (values: VariableAssignFormValues) => void
}) {
  const { t } = useTranslation()
  return (
    <Modal title={selectedVariable && variableProjectId(selectedVariable) ? t('settings.variables.edit') : t('settings.variables.assignTitle')} open={open} width={1080} onCancel={onCancel} footer={null}>
      {selectedVariable ? (
        <div className="settings-variable-modal">
          <div className="settings-readonly-grid">
            <div><span>{t('settings.variables.rawName')}</span><strong>{selectedVariable.raw_name || '-'}</strong></div>
            <div><span>{t('settings.variables.path')}</span><strong>{selectedVariable.source_path || '-'}</strong></div>
            <div><span>{t('settings.variables.jsonPath')}</span><strong>{selectedVariable.json_path || '-'}</strong></div>
            <div><span>Gateway</span><strong>{selectedVariable.gateway_id}</strong></div>
          </div>
          {variableProjectId(selectedVariable) ? (
            <Form form={variableEditForm} layout="vertical" onFinish={onSave}>
              <VariableBasicFields />
              <VariableWriteFields />
              <VariableRuntimeFields />
              <VariableDefaultAlarmFields includeApplyToRunning />
              <div className="settings-form-actions">
                <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={savePending}>{t('settings.variables.save')}</Button>
              </div>
            </Form>
          ) : (
            <Form form={variableAssignForm} layout="vertical" onFinish={onAssign}>
              <Alert className="settings-modal-alert" showIcon type="info" title={t('settings.variables.unassignedReadonly')} />
              <div className="settings-form-grid modal-grid">
                <Form.Item name="project_id" label={t('settings.variables.selectProject')} rules={[{ required: true }]}><Select options={projectOptions} /></Form.Item>
                <Form.Item name="var_group" label={t('settings.variables.group')}><Input /></Form.Item>
                <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked"><Switch /></Form.Item>
              </div>
              <div className="settings-form-actions">
                <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={assignPending}>{t('settings.variables.assign')}</Button>
              </div>
            </Form>
          )}
        </div>
      ) : null}
    </Modal>
  )
}

function VariableBasicFields() {
  const { t } = useTranslation()
  return (
    <VariableFormSection title={t('settings.variables.basicSection')} description={t('settings.variables.basicSectionHint')} defaultOpen>
      <div className="settings-form-grid modal-grid">
        <Form.Item name="var_name" label={t('settings.variables.varName')} rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="display_name" label={t('settings.variables.displayName')}><Input /></Form.Item>
        <Form.Item name="display_name_en" label={t('settings.variables.displayNameEn')}><Input /></Form.Item>
        <Form.Item name="display_name_ja" label={t('settings.variables.displayNameJa')}><Input /></Form.Item>
        <Form.Item name="data_type" label={t('settings.variables.type')}><Select options={variableTypeOptions} /></Form.Item>
        <Form.Item name="unit" label={t('settings.variables.unit')}><Input /></Form.Item>
        <Form.Item name="decimal_places" label={t('settings.variables.decimalPlaces')}><InputNumber min={0} max={8} /></Form.Item>
        <Form.Item name="scale_factor" label={t('settings.variables.scaleFactor')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="offset_val" label={t('settings.variables.offsetVal')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="var_group" label={t('settings.variables.group')}><Input /></Form.Item>
        <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked"><Switch /></Form.Item>
      </div>
    </VariableFormSection>
  )
}

function VariableWriteFields() {
  const { t } = useTranslation()
  return (
    <VariableFormSection title={t('settings.variables.writeSection')} description={t('settings.variables.writeSectionHint')}>
      <div className="settings-form-grid modal-grid">
        <Form.Item name="rw_mode" label={t('settings.variables.writeMode')}><Select options={[{ label: 'R', value: 'R' }, { label: 'W', value: 'W' }, { label: 'RW', value: 'RW' }]} /></Form.Item>
        <Form.Item name="writable" label={t('settings.variables.writable')} valuePropName="checked"><Switch /></Form.Item>
        <Form.Item name="write_source_id" label={t('settings.variables.writeSourceId')}><InputNumber min={0} /></Form.Item>
        <Form.Item name="write_path" label={t('settings.variables.writePath')}><Input /></Form.Item>
        <Form.Item name="write_data_type" label={t('settings.variables.writeDataType')}><Select allowClear options={variableTypeOptions} /></Form.Item>
        <Form.Item name="write_min" label={t('settings.variables.writeMin')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="write_max" label={t('settings.variables.writeMax')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="write_enum" label={t('settings.variables.writeEnum')}><Input /></Form.Item>
        <Form.Item name="write_requires_audit" label={t('settings.variables.writeRequiresAudit')} valuePropName="checked"><Switch /></Form.Item>
      </div>
    </VariableFormSection>
  )
}

function VariableRuntimeFields() {
  const { t } = useTranslation()
  return (
    <VariableFormSection title={t('settings.variables.runtimeSection')} description={t('settings.variables.runtimeSectionHint')}>
      <div className="settings-form-grid modal-grid">
        <Form.Item name="suspicious_value" label={t('settings.variables.suspiciousValue')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="debounce_threshold" label={t('settings.variables.debounceThreshold')}><InputNumber min={0} step={0.1} /></Form.Item>
        <Form.Item name="debounce_ms" label={t('settings.variables.debounceMs')}><InputNumber min={0} /></Form.Item>
        <Form.Item name="deadband" label={t('settings.variables.runtimeDeadband')}><InputNumber min={0} step={0.1} /></Form.Item>
      </div>
    </VariableFormSection>
  )
}

function VariableDefaultAlarmFields({ includeApplyToRunning = false }: { includeApplyToRunning?: boolean }) {
  const { t } = useTranslation()
  return (
    <VariableFormSection title={t('settings.variables.defaultAlarmSection')} description={t('settings.variables.defaultAlarmSectionHint')}>
      <Alert className="settings-modal-alert" showIcon type="info" title={t('settings.variables.defaultAlarmHint')} />
      <div className="settings-form-grid modal-grid">
        <Form.Item name="default_alarm_enabled" label={t('settings.variables.defaultAlarmEnabled')} valuePropName="checked"><Switch /></Form.Item>
        <Form.Item name="default_limit_ll" label={t('settings.variables.defaultLimitLL')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="default_limit_l" label={t('settings.variables.defaultLimitL')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="default_limit_h" label={t('settings.variables.defaultLimitH')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="default_limit_hh" label={t('settings.variables.defaultLimitHH')}><InputNumber step={0.1} /></Form.Item>
        <Form.Item name="default_limit_deadband" label={t('settings.variables.defaultLimitDeadband')}><InputNumber min={0} step={0.1} /></Form.Item>
        <Form.Item name="default_violation_hold_ms" label={t('settings.variables.defaultViolationHold')}><InputNumber min={0} /></Form.Item>
        <Form.Item name="default_recover_hold_ms" label={t('settings.variables.defaultRecoverHold')}><InputNumber min={0} /></Form.Item>
      </div>
      {includeApplyToRunning ? (
        <Form.Item name="apply_to_running" valuePropName="checked" className="settings-apply-running-field">
          <Checkbox>{t('settings.variables.applyToRunning')}</Checkbox>
        </Form.Item>
      ) : null}
    </VariableFormSection>
  )
}

function VirtualVariableModal({
  open,
  projectOptions,
  form,
  pending,
  onCancel,
  onCreate,
}: {
  open: boolean
  projectOptions: Array<{ label: string; value: number }>
  form: ReturnType<typeof Form.useForm<VirtualVariableFormValues>>[0]
  pending: boolean
  onCancel: () => void
  onCreate: (values: VirtualVariableFormValues) => void
}) {
  const { t } = useTranslation()
  return (
    <Modal title={t('settings.variables.createVirtual')} open={open} width={900} onCancel={onCancel} footer={null}>
      <Form form={form} layout="vertical" onFinish={onCreate}>
        <Alert className="settings-modal-alert" showIcon type="info" title={t('settings.variables.virtualHint')} />
        <div className="settings-form-grid modal-grid">
          <Form.Item name="project_id" label={t('settings.variables.selectProject')} rules={[{ required: true }]}><Select options={projectOptions} /></Form.Item>
          <Form.Item name="var_name" label={t('settings.variables.varName')} rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="display_name" label={t('settings.variables.displayName')} rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="display_name_en" label={t('settings.variables.displayNameEn')}><Input /></Form.Item>
          <Form.Item name="display_name_ja" label={t('settings.variables.displayNameJa')}><Input /></Form.Item>
          <Form.Item name="data_type" label={t('settings.variables.type')} rules={[{ required: true }]}><Select options={variableTypeOptions} /></Form.Item>
          <Form.Item name="unit" label={t('settings.variables.unit')}><Input /></Form.Item>
          <Form.Item name="decimal_places" label={t('settings.variables.decimalPlaces')}><InputNumber min={0} max={8} /></Form.Item>
          <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked"><Switch /></Form.Item>
        </div>
        <VariableDefaultAlarmFields />
        <div className="settings-form-actions">
          <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={pending}>{t('settings.variables.createVirtual')}</Button>
        </div>
      </Form>
    </Modal>
  )
}

function BatchAssignModal({
  open,
  count,
  projectOptions,
  form,
  pending,
  onCancel,
  onAssign,
}: {
  open: boolean
  count: number
  projectOptions: Array<{ label: string; value: number }>
  form: ReturnType<typeof Form.useForm<VariableAssignFormValues>>[0]
  pending: boolean
  onCancel: () => void
  onAssign: (values: VariableAssignFormValues) => void
}) {
  const { t } = useTranslation()
  return (
    <Modal title={t('settings.variables.batchAssignTitle', { count })} open={open} width={640} onCancel={onCancel} footer={null}>
      <Form form={form} layout="vertical" onFinish={onAssign}>
        <Alert className="settings-modal-alert" showIcon type="info" title={t('settings.variables.batchAssignHint', { count })} />
        <div className="settings-form-grid modal-grid">
          <Form.Item name="project_id" label={t('settings.variables.selectProject')} rules={[{ required: true }]}><Select options={projectOptions} /></Form.Item>
          <Form.Item name="var_group" label={t('settings.variables.group')}><Input /></Form.Item>
          <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked"><Switch /></Form.Item>
        </div>
        <div className="settings-form-actions">
          <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={pending}>{t('settings.variables.batchAssign', { count })}</Button>
        </div>
      </Form>
    </Modal>
  )
}

function KioRemapModal({
  open,
  form,
  result,
  pending,
  onCancel,
  onSubmit,
}: {
  open: boolean
  form: ReturnType<typeof Form.useForm<KioProjectRemapFormValues>>[0]
  result?: BulkRemapKioProjectsResult
  pending: boolean
  onCancel: () => void
  onSubmit: (dryRun: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <Modal title={t('settings.variables.kioRemapTitle')} open={open} width={920} onCancel={onCancel} footer={null}>
      <Alert className="settings-modal-alert" showIcon type="info" title={t('settings.variables.kioRemapHint')} />
      <Form form={form} layout="vertical">
        <div className="settings-form-grid modal-grid">
          <Form.Item name="project_count" label={t('settings.variables.kioProjectCount')} rules={[{ required: true }]}><InputNumber min={1} max={8} /></Form.Item>
          <Form.Item name="project_code_prefix" label={t('settings.variables.kioProjectCodePrefix')}><Input /></Form.Item>
          <Form.Item name="project_display_prefix" label={t('settings.variables.kioProjectDisplayPrefix')}><Input /></Form.Item>
          <Form.Item name="raw_project_prefix" label={t('settings.variables.kioRawProjectPrefix')}><Input /></Form.Item>
          <Form.Item name="var_group" label={t('settings.variables.group')}><Input /></Form.Item>
          <Form.Item name="var_name_prefix" label={t('settings.variables.kioVarNamePrefix')}><Input /></Form.Item>
          <Form.Item name="remap_var_name" label={t('settings.variables.kioRemapVarName')} valuePropName="checked"><Switch /></Form.Item>
          <Form.Item name="enable" label={t('settings.variables.enabled')} valuePropName="checked"><Switch /></Form.Item>
        </div>
        <div className="settings-form-actions">
          <Button loading={pending} onClick={() => void onSubmit(true)}>{t('settings.variables.kioDryRun')}</Button>
          <Popconfirm title={t('settings.variables.kioExecuteConfirm')} okText={t('settings.variables.kioExecute')} cancelText={t('settings.actions.cancel')} onConfirm={() => void onSubmit(false)}>
            <Button type="primary" danger icon={<RotateCcw size={15} />} loading={pending}>{t('settings.variables.kioExecute')}</Button>
          </Popconfirm>
        </div>
      </Form>
      {result ? (
        <div className="settings-kio-remap-result">
          <div className="settings-kio-remap-summary">
            <Tag color={result.dry_run ? 'processing' : 'success'}>{result.dry_run ? t('settings.variables.kioDryRun') : t('settings.variables.kioExecuted')}</Tag>
            <span>{t('settings.variables.kioResultSummary', result)}</span>
          </div>
          <Table
            size="small"
            rowKey={(record) => String(record.var_id_text ?? record.var_id)}
            pagination={false}
            columns={[
              { title: t('settings.variables.rawName'), dataIndex: 'raw_name', key: 'raw_name', width: 140 },
              { title: t('settings.variables.varName'), dataIndex: 'new_var_name', key: 'new_var_name', width: 150 },
              { title: t('settings.storage.project'), dataIndex: 'project_code', key: 'project_code', width: 120 },
              { title: t('settings.variables.kioAction'), dataIndex: 'action', key: 'action', width: 110, render: (value) => <Tag>{String(value)}</Tag> },
              { title: t('settings.variables.kioReason'), dataIndex: 'reason', key: 'reason', ellipsis: true },
            ]}
            dataSource={result.items.slice(0, 80)}
            scroll={{ y: 260 }}
          />
        </div>
      ) : null}
    </Modal>
  )
}

function VariableFormSection({ title, description, defaultOpen = false, children }: { title: string; description?: string; defaultOpen?: boolean; children: ReactNode }) {
  return (
    <details className="settings-variable-form-section" open={defaultOpen}>
      <summary>
        <strong>{title}</strong>
        {description ? <span>{description}</span> : null}
      </summary>
      {children}
    </details>
  )
}

const variableTypeOptions = [
  { label: 'FLOAT', value: 'FLOAT' },
  { label: 'INT', value: 'INT' },
  { label: 'BOOL', value: 'BOOL' },
  { label: 'STRING', value: 'STRING' },
]
