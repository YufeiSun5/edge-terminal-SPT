import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, Button, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { Braces, Edit3, Play, Plus, Save, ScrollText, Send, Trash2, Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { queryClient } from '@/app/queryClient'
import {
  createTaskFlow,
  getProjects,
  getTaskFlowRuns,
  getTaskFlowSqlLogs,
  getTaskFlows,
  getTaskFlowTemplates,
  getTaskModules,
  getVariables,
  runTaskFlow,
  updateTaskFlow,
} from '@/features/edge-status/api'
import { sendRealtimeWebSocketCommand } from '@/features/realtime/realtimeClient'
import type {
  Project,
  TaskFlow,
  TaskFlowModule,
  TaskFlowPayload,
  TaskFlowRun,
  TaskFlowSqlLog,
  TaskFlowTemplate,
  VariableConfig,
} from '@/shared/api/types'
import { languageCode } from '@/shared/i18n/language'
import '@/features/settings/settings.css'
import './task-flows.css'

type TaskFlowFormValues = TaskFlowPayload
type TaskFlowFilter = 'all' | number
type WireVarID = string | number
type ScalarValueType = 'string' | 'number' | 'boolean'
type ProcessParamRow = { key?: string; value?: string | number | boolean; value_type?: ScalarValueType }
type PLCWriteRow = {
  var_id?: WireVarID
  value?: string | number | boolean
  value_from?: string
  value_type?: ScalarValueType
  wait_ack?: boolean
  ack_timeout_sec?: number
  settle_ms?: number
}
type TaskRequestFormValues = {
  project_id?: number
  request_var_id?: WireVarID
  command?: string
  task_id?: number
  test_no?: string
  standard_id?: number
  duration_sec?: number
  qualified_hold_ms?: number
  check_interval_ms?: number
  limit_check_enabled?: boolean
  enable_storage?: boolean
  enable_alarm?: boolean
  operator_note?: string
  report_template_id?: number
  report_var_ids?: WireVarID[]
  report_ext_1?: string
  report_ext_2?: string
  report_ext_3?: string
  end_type?: string
  reason?: string
  var_id?: WireVarID
  check_enabled?: boolean
  alarm_enabled?: boolean
  store_enabled?: boolean
  limit_ll?: number
  limit_l?: number
  limit_h?: number
  limit_hh?: number
  limit_deadband?: number
  check_cycle_ms?: number
  violation_hold_ms?: number
  recover_hold_ms?: number
  items?: string
  file_ref?: string
  file_name?: string
  status?: string
  template_id?: number
  template_code?: string
  template_version?: number
  method?: string
  url?: string
  headers?: string
  body?: string
  timeout_ms?: number
  custom_items?: string
  process_params?: ProcessParamRow[]
  plc_writes?: PLCWriteRow[]
}
type TaskModuleParamSchema = {
  type?: string
  required?: boolean
  default?: unknown
  options?: string[]
  source?: string[]
  description?: string
  item_schema?: Record<string, TaskModuleParamSchema>
  language?: string
}

const triggerOptions = [
  { label: 'data_change', value: 'data_change' },
  { label: 'manual', value: 'manual' },
  { label: 'schedule', value: 'schedule' },
  { label: 'project_start', value: 'project_start' },
  { label: 'project_end', value: 'project_end' },
]

const actionOptions = [
  { label: 'builtin.storage_snapshot', value: 'builtin.storage_snapshot' },
  { label: 'javascript', value: 'javascript' },
]

const roleOptions = [
  { label: 'watch', value: 'watch' },
  { label: 'read', value: 'read' },
  { label: 'write', value: 'write' },
]

const taskRequestCommandOptions = [
  { label: 'start_detection', value: 'start_detection' },
  { label: 'start_fixed_duration_detection', value: 'start_fixed_duration_detection' },
  { label: 'start_qualified_hold_detection', value: 'start_qualified_hold_detection' },
  { label: 'stop_detection', value: 'stop_detection' },
  { label: 'pause_detection', value: 'pause_detection' },
  { label: 'resume_detection', value: 'resume_detection' },
  { label: 'mute_detection_alarms', value: 'mute_detection_alarms' },
  { label: 'storage_snapshot', value: 'storage_snapshot' },
  { label: 'storage_prepare', value: 'storage_prepare' },
  { label: 'update_detection_limits', value: 'update_detection_limits' },
  { label: 'refresh_detection_features', value: 'refresh_detection_features' },
  { label: 'register_report', value: 'register_report' },
  { label: 'http_request', value: 'http_request' },
  { label: 'write_control_variables', value: 'write_control_variables' },
]

const taskRequestStartCommands = new Set(['start_detection', 'start_fixed_duration_detection', 'start_qualified_hold_detection'])
const taskRequestRunControlCommands = new Set(['stop_detection', 'pause_detection', 'resume_detection', 'mute_detection_alarms', 'refresh_detection_features'])

const scalarValueTypeOptions = [
  { label: 'string', value: 'string' },
  { label: 'number', value: 'number' },
  { label: 'boolean', value: 'boolean' },
]

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return variable.display_name_en || variable.var_name || variable.raw_name
  if (currentLanguage === 'ja') return variable.display_name_ja || variable.var_name || variable.raw_name
  return variable.display_name || variable.raw_name || variable.var_name
}

function variableWireId(variable: Pick<VariableConfig, 'var_id' | 'var_id_text'>): string {
  return variable.var_id_text ?? String(variable.var_id)
}

function variableKey(value?: WireVarID | null) {
  return value === undefined || value === null || value === '' ? '' : String(value)
}

function taskFlowVarKey(variable: Pick<NonNullable<TaskFlow['vars']>[number], 'var_id' | 'var_id_text'>) {
  return variable.var_id_text ?? String(variable.var_id)
}

function variableProjectId(variable: Pick<VariableConfig, 'project_id' | 'device_id'>) {
  return variable.project_id ?? variable.device_id
}

function projectCode(project?: Pick<Project, 'project_code' | 'device_code'>) {
  return project?.project_code || project?.device_code || ''
}

export function TaskFlowsPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [projectFilter, setProjectFilter] = useState<TaskFlowFilter>('all')
  const [triggerFilter, setTriggerFilter] = useState<string | undefined>()
  const [flowModalOpen, setFlowModalOpen] = useState(false)
  const [requestModalOpen, setRequestModalOpen] = useState(false)
  const [editingFlow, setEditingFlow] = useState<TaskFlow | undefined>()
  const [selectedRunId, setSelectedRunId] = useState<number | undefined>()
  const [moduleParams, setModuleParams] = useState<Record<string, unknown>>({})
  const [taskRequestPreview, setTaskRequestPreview] = useState('{}')
  const [taskRequestPreviewError, setTaskRequestPreviewError] = useState<string>()
  const [form] = Form.useForm<TaskFlowFormValues>()
  const [requestForm] = Form.useForm<TaskRequestFormValues>()
  const formProjectId = Form.useWatch('project_id', form)
  const requestProjectId = Form.useWatch('project_id', requestForm)
  const requestVariableId = Form.useWatch('request_var_id', requestForm)
  const requestCommand = Form.useWatch('command', requestForm)
  const selectedActionType = Form.useWatch('action_type', form)

  const projectsQuery = useQuery({
    queryKey: ['task-flows', 'projects'],
    queryFn: getProjects,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['task-flows', 'variables'],
    queryFn: () => getVariables(),
    retry: false,
  })
  const flowsQuery = useQuery({
    queryKey: ['task-flows', 'items', projectFilter, triggerFilter],
    queryFn: () => getTaskFlows({
      project_id: projectFilter === 'all' ? undefined : projectFilter,
      trigger_type: triggerFilter,
    }),
    retry: false,
    refetchInterval: 10000,
  })
  const modulesQuery = useQuery({
    queryKey: ['task-flows', 'modules'],
    queryFn: getTaskModules,
    retry: false,
  })
  const templatesQuery = useQuery({
    queryKey: ['task-flows', 'templates'],
    queryFn: getTaskFlowTemplates,
    retry: false,
  })
  const runsQuery = useQuery({
    queryKey: ['task-flows', 'runs', projectFilter],
    queryFn: () =>
      getTaskFlowRuns({
        project_id: projectFilter === 'all' ? undefined : projectFilter,
        limit: 80,
      }),
    retry: false,
    refetchInterval: 8000,
  })
  const sqlLogsQuery = useQuery({
    queryKey: ['task-flows', 'sql-logs', selectedRunId],
    queryFn: () => getTaskFlowSqlLogs(selectedRunId!, 80),
    enabled: selectedRunId !== undefined,
    retry: false,
  })

  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const flows = useMemo(() => flowsQuery.data ?? [], [flowsQuery.data])
  const modules = useMemo(() => modulesQuery.data ?? [], [modulesQuery.data])
  const templates = useMemo(() => templatesQuery.data ?? [], [templatesQuery.data])
  const runs = useMemo(() => runsQuery.data?.items ?? [], [runsQuery.data?.items])
  const sqlLogs = useMemo(() => sqlLogsQuery.data ?? [], [sqlLogsQuery.data])
  const enabledFlows = flows.filter((item) => item.enabled)
  const variableById = useMemo(() => {
    const entries: Array<[string, VariableConfig]> = []
    variables.forEach((variable) => {
      entries.push([String(variable.var_id), variable])
      if (variable.var_id_text) entries.push([variable.var_id_text, variable])
    })
    return new Map(entries)
  }, [variables])
  const projectById = useMemo(() => new Map(projects.map((project) => [project.id, project])), [projects])
  const projectVariables = useMemo(() => {
    return variables.filter((variable) => !formProjectId || variableProjectId(variable) === formProjectId)
  }, [formProjectId, variables])
  const requestProjectVariables = useMemo(() => {
    return variables.filter((variable) => !requestProjectId || variableProjectId(variable) === requestProjectId)
  }, [requestProjectId, variables])
  const requestStringVariables = useMemo(() => {
    return requestProjectVariables.filter((variable) => variable.source_type === 'virtual' && variable.data_type === 'STRING')
  }, [requestProjectVariables])
  const requestLimitVariables = useMemo(() => {
    return requestProjectVariables.filter((variable) => variable.data_type === 'INT' || variable.data_type === 'FLOAT' || variable.data_type === 'BOOL')
  }, [requestProjectVariables])
  const requestVariableWatchFlows = useMemo(() => {
    const selectedVarID = variableKey(requestVariableId)
    if (!selectedVarID) return []
    return flows.filter((flow) => flow.enabled && flow.vars?.some((item) => item.role === 'watch' && taskFlowVarKey(item) === selectedVarID))
  }, [flows, requestVariableId])
  const moduleActionOptions = useMemo(() => {
    const fromModules = modules.map((module) => ({ label: module.code, value: module.code }))
    const known = new Set(fromModules.map((item) => item.value))
    return [...fromModules, ...actionOptions.filter((item) => !known.has(item.value))]
  }, [modules])
  const selectedModule = useMemo(() => modules.find((module) => module.code === selectedActionType), [modules, selectedActionType])

  const displayProjectName = (project?: Project) => {
    if (!project) return '-'
    const code = projectCode(project)
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return project.display_name_en || code
    if (currentLanguage === 'ja') return project.display_name_ja || code
    return project.display_name || project.name || code
  }

  function normalizePayload(values: TaskFlowFormValues): TaskFlowPayload {
    const stepsJson = normalizeStepsJson(values.steps_json)
    return {
      ...values,
      enabled: values.enabled ?? true,
      timeout_ms: values.timeout_ms ?? 3000,
      cooldown_ms: values.cooldown_ms ?? 0,
      hold_ms: values.hold_ms ?? 0,
      schedule_interval_ms: values.trigger_type === 'schedule' ? (values.schedule_interval_ms ?? values.cooldown_ms ?? 60000) : values.schedule_interval_ms,
      priority: values.priority ?? 0,
      steps_json: stepsJson,
      vars: (values.vars ?? [])
        .filter((item) => item.var_id)
        .map((item) => {
          const varID = variableKey(item.var_id)
          const variable = variableById.get(varID)
          return {
            var_id: varID,
            var_name: item.var_name || variable?.var_name || variable?.raw_name || '',
            role: item.role || 'watch',
          }
        }),
    }
  }

  const saveFlowMutation = useMutation({
    mutationFn: (values: TaskFlowFormValues) => {
      const payload = normalizePayload(values)
      if (editingFlow) return updateTaskFlow(editingFlow.id, payload)
      return createTaskFlow(payload)
    },
    onSuccess: async () => {
      setFlowModalOpen(false)
      setEditingFlow(undefined)
      form.resetFields()
      messageApi.success(t('taskFlows.messages.saved'))
      await queryClient.invalidateQueries({ queryKey: ['task-flows'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const runFlowMutation = useMutation({
    mutationFn: (flow: TaskFlow) => runTaskFlow(flow.id),
    onSuccess: async () => {
      messageApi.success(t('taskFlows.messages.queued'))
      await queryClient.invalidateQueries({ queryKey: ['task-flows', 'runs'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const sendTaskRequestMutation = useMutation({
    mutationFn: async (values: TaskRequestFormValues) => {
      const requestVarID = variableKey(values.request_var_id)
      if (!requestVarID) throw new Error(t('taskFlows.request.errors.requestVariableRequired'))
      let requestPayload: Record<string, unknown>
      try {
        requestPayload = buildTaskRequestPayload(values)
      } catch {
        throw new Error(t('taskFlows.request.errors.invalidJson'))
      }
      const requestId = `task-req-${Date.now()}`
      return sendRealtimeWebSocketCommand(
        {
          type: 'command.write_variable',
          request_id: requestId,
          command_id: `${requestId}-write`,
          payload: {
            var_id: requestVarID,
            value: JSON.stringify(requestPayload),
            quality: 1,
            trigger: true,
          },
        },
        15000,
      )
    },
    onSuccess: async () => {
      messageApi.success(t('taskFlows.request.sent'))
      setRequestModalOpen(false)
      await queryClient.invalidateQueries({ queryKey: ['task-flows', 'runs'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('taskFlows.request.failed')),
  })

  function updateTaskRequestPreview(values: TaskRequestFormValues) {
    try {
      const payload = buildTaskRequestPayload(values)
      setTaskRequestPreview(JSON.stringify(payload, null, 2))
      setTaskRequestPreviewError(undefined)
    } catch {
      setTaskRequestPreviewError(t('taskFlows.request.errors.invalidJson'))
    }
  }

  function normalizeStepsJson(value: unknown) {
    if (!value) return ''
    if (typeof value !== 'string') return JSON.stringify(value, null, 2)
    const trimmed = value.trim()
    if (!trimmed) return ''
    const parsed = JSON.parse(trimmed) as unknown
    if (!Array.isArray(parsed) && (!parsed || typeof parsed !== 'object')) {
      throw new Error(t('taskFlows.messages.invalidStepsJson'))
    }
    return JSON.stringify(parsed, null, 2)
  }

  function syncModuleParamsToForm(moduleCode: string, params: Record<string, unknown>, existingStepsJson = form.getFieldValue('steps_json')) {
    const steps = parseTaskFlowSteps(existingStepsJson)
    const firstStep = steps[0] && typeof steps[0] === 'object' ? steps[0] : {}
    const nextSteps = [{ ...firstStep, module: moduleCode, params }, ...steps.slice(1)]
    form.setFieldsValue({
      action_payload: JSON.stringify(params, null, 2),
      steps_json: JSON.stringify(nextSteps, null, 2),
    })
  }

  function applyModuleParams(nextParams: Record<string, unknown>) {
    setModuleParams(nextParams)
    if (selectedActionType) syncModuleParamsToForm(selectedActionType, nextParams)
  }

  function resetModuleParamsForAction(actionType: string) {
    const module = modules.find((item) => item.code === actionType)
    const nextParams = module ? buildDefaultParams(module) : {}
    setModuleParams(nextParams)
    syncModuleParamsToForm(actionType, nextParams)
  }

  function openTemplateModal(template: TaskFlowTemplate) {
    const project = projectFilter === 'all' ? projects[0] : projectById.get(projectFilter)
    const firstStep = template.steps[0]
    setEditingFlow(undefined)
    form.setFieldsValue({
      project_id: project?.id,
      flow_code: `${template.template_code}-${Date.now().toString().slice(-5)}`,
      name: template.name,
      enabled: true,
      trigger_type: template.trigger_type,
      condition_script: template.condition_script || 'return true',
      action_type: firstStep?.module || 'builtin.storage_snapshot',
      action_script: firstStep?.script || '',
      action_payload: JSON.stringify(firstStep?.params ?? {}, null, 2),
      steps_json: JSON.stringify(template.steps ?? [], null, 2),
      timeout_ms: 30000,
      cooldown_ms: 0,
      hold_ms: 0,
      schedule_interval_ms: template.trigger_type === 'schedule' ? 60000 : undefined,
      priority: 0,
      remark: template.description,
      vars: [],
    })
    setModuleParams(firstStep?.params && typeof firstStep.params === 'object' ? firstStep.params : {})
    setFlowModalOpen(true)
  }

  function openTaskRequestModal() {
    const project = projectFilter === 'all' ? projects[0] : projectById.get(projectFilter)
    const requestVariable = variables.find(
      (variable) =>
        variable.source_type === 'virtual' &&
        variable.data_type === 'STRING' &&
        (!project || variableProjectId(variable) === project.id),
    )
    const initialValues: TaskRequestFormValues = {
      project_id: project?.id,
      request_var_id: requestVariable ? variableWireId(requestVariable) : undefined,
      command: 'start_detection',
      test_no: `RUN-${new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 14)}`,
      limit_check_enabled: true,
      enable_storage: true,
      enable_alarm: true,
      report_var_ids: [],
      process_params: [{ key: 'inlet_area_m2', value_type: 'number' }],
      plc_writes: [{ value_from: 'process_params.inlet_area_m2', value_type: 'number', wait_ack: true, ack_timeout_sec: 5 }],
    }
    requestForm.setFieldsValue(initialValues)
    updateTaskRequestPreview(initialValues)
    setRequestModalOpen(true)
  }

  function openFlowModal(flow?: TaskFlow) {
    setEditingFlow(flow)
    if (flow) {
      form.setFieldsValue({
        project_id: flow.project_id,
        flow_code: flow.flow_code,
        name: flow.name,
        enabled: flow.enabled,
        trigger_type: flow.trigger_type,
        condition_script: flow.condition_script,
        action_type: flow.action_type,
        action_script: flow.action_script,
        action_payload: flow.action_payload,
        timeout_ms: flow.timeout_ms,
        cooldown_ms: flow.cooldown_ms,
        hold_ms: flow.hold_ms,
        schedule_interval_ms: flow.schedule_interval_ms,
        priority: flow.priority,
        remark: flow.remark,
        steps_json: flow.steps_json,
        vars: flow.vars?.map((item) => ({ var_id: item.var_id_text ?? item.var_id, var_name: item.var_name, role: item.role })) ?? [],
      })
      setModuleParams(getInitialModuleParams(flow.steps_json, flow.action_payload))
    } else {
      const project = projectFilter === 'all' ? projects[0] : projectById.get(projectFilter)
      form.setFieldsValue({
        project_id: project?.id,
        flow_code: `flow-${Date.now().toString().slice(-6)}`,
        name: '',
        enabled: true,
        trigger_type: 'data_change',
        condition_script: 'return true',
        action_type: 'builtin.storage_snapshot',
        action_script: '',
        action_payload: '{}',
        timeout_ms: 3000,
        cooldown_ms: 0,
        hold_ms: 0,
        schedule_interval_ms: undefined,
        priority: 0,
        remark: '',
        steps_json: JSON.stringify([{ module: 'builtin.storage_snapshot', params: {} }], null, 2),
        vars: [],
      })
      setModuleParams({})
    }
    setFlowModalOpen(true)
  }

  const columns: TableColumnsType<TaskFlow> = [
    {
      title: t('taskFlows.columns.flow'),
      dataIndex: 'name',
      key: 'name',
      width: 260,
      render: (_, record) => (
        <div className="task-flow-name">
          <strong>{record.name || record.flow_code}</strong>
          <span>{record.flow_code}</span>
        </div>
      ),
    },
    {
      title: t('taskFlows.columns.project'),
      dataIndex: 'project_id',
      key: 'project_id',
      width: 170,
      render: (value: number) => displayProjectName(projectById.get(value)),
    },
    { title: t('taskFlows.columns.trigger'), dataIndex: 'trigger_type', key: 'trigger_type', width: 140 },
    { title: t('taskFlows.columns.action'), dataIndex: 'action_type', key: 'action_type', width: 190 },
    {
      title: t('taskFlows.columns.vars'),
      dataIndex: 'vars',
      key: 'vars',
      width: 140,
      render: (vars?: TaskFlow['vars']) => vars?.length ?? 0,
    },
    { title: t('taskFlows.columns.priority'), dataIndex: 'priority', key: 'priority', width: 90 },
    {
      title: t('taskFlows.columns.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'success' : 'default'}>{enabled ? t('status.online') : t('status.offline')}</Tag>,
    },
    {
      title: t('taskFlows.columns.actions'),
      key: 'actions',
      fixed: 'right',
      width: 220,
      render: (_, record) => (
        <Space size={6}>
          <Button size="small" icon={<Play size={13} />} loading={runFlowMutation.isPending} onClick={() => runFlowMutation.mutate(record)}>
            {t('taskFlows.actions.run')}
          </Button>
          <Button size="small" icon={<Edit3 size={13} />} onClick={() => openFlowModal(record)}>
            {t('taskFlows.actions.edit')}
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div className="task-flows-page">
      {contextHolder}
      <header className="task-flows-hero">
        <div>
          <span className="settings-eyebrow">{t('taskFlows.eyebrow')}</span>
          <h1>{t('taskFlows.title')}</h1>
          <p>{t('taskFlows.subtitle')}</p>
        </div>
        <div className="task-flows-actions">
          <Button icon={<Send size={15} />} onClick={openTaskRequestModal}>
            {t('taskFlows.request.open')}
          </Button>
          <Button icon={<Plus size={15} />} type="primary" onClick={() => openFlowModal()}>
            {t('taskFlows.actions.create')}
          </Button>
        </div>
      </header>

      <div className="task-flows-grid">
        <section className="task-flows-sidebar">
          <div className="task-flows-panel-head">
            <div>
              <span className="settings-eyebrow">{t('taskFlows.projects.subtitle')}</span>
              <h2>{t('taskFlows.projects.title')}</h2>
            </div>
          </div>
          <button className={projectFilter === 'all' ? 'task-project-card active' : 'task-project-card'} onClick={() => setProjectFilter('all')}>
            <strong>{t('taskFlows.projects.all')}</strong>
            <span>{flows.length}</span>
          </button>
          {projects.map((project) => (
            <button
              className={projectFilter === project.id ? 'task-project-card active' : 'task-project-card'}
              key={project.id}
              onClick={() => setProjectFilter(project.id)}
            >
              <strong>{displayProjectName(project)}</strong>
              <span>{flows.filter((flow) => flow.project_id === project.id).length}</span>
            </button>
          ))}
        </section>

        <main className="task-flows-main">
          <div className="task-flow-summary">
            <div>
              <Workflow size={18} />
              <span>{t('taskFlows.summary.total')}</span>
              <strong>{flows.length}</strong>
            </div>
            <div>
              <Play size={18} />
              <span>{t('taskFlows.summary.enabled')}</span>
              <strong>{enabledFlows.length}</strong>
            </div>
            <div>
              <Braces size={18} />
              <span>{t('taskFlows.summary.javascript')}</span>
              <strong>{flows.filter((flow) => flow.action_type === 'javascript').length}</strong>
            </div>
            <div>
              <ScrollText size={18} />
              <span>{t('taskFlows.summary.logs')}</span>
              <strong>{runsQuery.data?.total ?? runs.length}</strong>
            </div>
          </div>

          <section className="task-flows-table-card">
            <div className="task-flows-panel-head">
              <div>
                <span className="settings-eyebrow">{t('taskFlows.list.subtitle')}</span>
                <h2>{t('taskFlows.list.title')}</h2>
              </div>
              <Space wrap>
                <Select
                  className="task-flow-template-select"
                  loading={templatesQuery.isFetching}
                  placeholder={t('taskFlows.templates.placeholder')}
                  options={templates.map((template) => ({
                    label: `${template.name} · ${template.template_code}`,
                    value: template.template_code,
                  }))}
                  onChange={(templateCode) => {
                    const template = templates.find((item) => item.template_code === templateCode)
                    if (template) openTemplateModal(template)
                  }}
                />
                <Select
                  className="task-flow-trigger-filter"
                  allowClear
                  placeholder={t('taskFlows.fields.triggerType')}
                  options={triggerOptions}
                  value={triggerFilter}
                  onChange={setTriggerFilter}
                />
                <Button icon={<Plus size={14} />} onClick={() => openFlowModal()}>
                  {t('taskFlows.actions.create')}
                </Button>
              </Space>
            </div>
            <Table
              size="small"
              rowKey="id"
              loading={flowsQuery.isFetching}
              columns={columns}
              dataSource={flows}
              scroll={{ x: 1300, y: 560 }}
              pagination={{ pageSize: 20, showSizeChanger: true, size: 'small' }}
            />
          </section>

          <section className="task-flows-log-card">
            <Tabs
              size="small"
              items={[
                {
                  key: 'runs',
                  label: t('taskFlows.logs.runs'),
                  children: <TaskFlowRunsTable runs={runs} loading={runsQuery.isFetching} projects={projectById} onSelectRun={setSelectedRunId} />,
                },
                {
                  key: 'sql',
                  label: t('taskFlows.logs.sql'),
                  children: selectedRunId ? (
                    <TaskFlowSqlLogsTable logs={sqlLogs} loading={sqlLogsQuery.isFetching} />
                  ) : (
                    <Alert type="info" showIcon message={t('taskFlows.logs.selectRun')} />
                  ),
                },
              ]}
            />
          </section>
        </main>
      </div>

      <Modal
        title={t('taskFlows.request.title')}
        open={requestModalOpen}
        width={980}
        onCancel={() => setRequestModalOpen(false)}
        footer={null}
      >
        <Alert className="task-flow-request-alert" type="info" showIcon message={t('taskFlows.request.hint')} />
        <Form
          form={requestForm}
          layout="vertical"
          onFinish={(values) => sendTaskRequestMutation.mutate(values)}
          onValuesChange={(_, values) => updateTaskRequestPreview(values as TaskRequestFormValues)}
        >
          <div className="task-flow-form-grid">
            <Form.Item name="project_id" label={t('taskFlows.fields.project')} rules={[{ required: true }]}>
              <Select
                options={projects.map((project) => ({ label: `${displayProjectName(project)} · ${projectCode(project)}`, value: project.id }))}
                onChange={() => requestForm.setFieldValue('request_var_id', undefined)}
              />
            </Form.Item>
            <Form.Item name="request_var_id" label={t('taskFlows.request.requestVariable')} rules={[{ required: true }]}>
              <Select
                showSearch
                optionFilterProp="label"
                options={requestStringVariables.map((variable) => ({
                  label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
                  value: variableWireId(variable),
                }))}
              />
            </Form.Item>
            <div className="task-flow-form-wide">
              <Alert
                className="task-flow-request-alert"
                type={!requestVariableId ? 'info' : requestVariableWatchFlows.length > 0 ? 'success' : 'warning'}
                showIcon
                message={
                  !requestVariableId
                    ? t('taskFlows.request.watchFlowsEmpty')
                    : requestVariableWatchFlows.length > 0
                      ? t('taskFlows.request.watchFlowsFound', { count: requestVariableWatchFlows.length })
                      : t('taskFlows.request.watchFlowsNone')
                }
                description={
                  requestVariableWatchFlows.length > 0 ? (
                    <div className="task-flow-request-watch-list">
                      {requestVariableWatchFlows.map((flow) => (
                        <Tag key={flow.id} color="green">
                          {flow.name || flow.flow_code}
                        </Tag>
                      ))}
                    </div>
                  ) : undefined
                }
              />
            </div>
            <Form.Item name="command" label={t('taskFlows.request.command')} rules={[{ required: true }]}>
              <Select options={taskRequestCommandOptions} />
            </Form.Item>
            <div className="task-flow-form-wide task-flow-request-config-note">
              <strong>{t('taskFlows.request.configParams')}</strong>
              <span>{t('taskFlows.request.configParamsHint')}</span>
            </div>
            {taskRequestStartCommands.has(requestCommand ?? '') ? (
              <>
                <Form.Item name="test_no" label={t('station.run.testNo')}>
                  <Input />
                </Form.Item>
                <Form.Item name="standard_id" label="standard_id">
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="duration_sec" label="duration_sec">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="qualified_hold_ms" label="qualified_hold_ms">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="check_interval_ms" label="check_interval_ms">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="report_template_id" label="report_template_id">
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item className="task-flow-form-wide" name="report_var_ids" label={t('taskFlows.request.reportVariables')}>
                  <Select
                    allowClear
                    mode="multiple"
                    optionFilterProp="label"
                    options={requestProjectVariables.map((variable) => ({
                      label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
                      value: variableWireId(variable),
                    }))}
                  />
                </Form.Item>
                <Form.Item name="report_ext_1" label="report_request.ext_1">
                  <Input />
                </Form.Item>
                <Form.Item name="report_ext_2" label="report_request.ext_2">
                  <Input />
                </Form.Item>
                <Form.Item name="report_ext_3" label="report_request.ext_3">
                  <Input />
                </Form.Item>
                <Form.Item name="limit_check_enabled" label="limit_check_enabled" valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name="enable_storage" label="enable_storage" valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name="enable_alarm" label="enable_alarm" valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item className="task-flow-form-wide" name="operator_note" label={t('station.run.note')}>
                  <Input />
                </Form.Item>
                <Form.Item className="task-flow-form-wide" name="custom_items" label="custom_items JSON">
                  <Input.TextArea className="task-flow-code" rows={3} spellCheck={false} placeholder={t('taskFlows.params.jsonPlaceholder')} />
                </Form.Item>
              </>
            ) : null}
          </div>

          {taskRequestRunControlCommands.has(requestCommand ?? '') ? (
            <TaskRequestRunControlEditor command={requestCommand} />
          ) : null}
          {requestCommand === 'update_detection_limits' ? (
            <TaskRequestLimitEditor variables={requestLimitVariables} language={i18n.resolvedLanguage} />
          ) : null}
          {requestCommand === 'storage_prepare' ? <TaskRequestStoragePrepareEditor /> : null}
          {requestCommand === 'register_report' ? <TaskRequestReportEditor /> : null}
          {requestCommand === 'http_request' ? <TaskRequestHttpEditor /> : null}
          {taskRequestStartCommands.has(requestCommand ?? '') ? <TaskRequestProcessParamsEditor /> : null}
          {taskRequestStartCommands.has(requestCommand ?? '') || requestCommand === 'write_control_variables' ? (
            <TaskRequestPLCWritesEditor variables={requestProjectVariables} language={i18n.resolvedLanguage} />
          ) : null}

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.request.preview')}</strong>
              <span>{t('taskFlows.request.previewHint')}</span>
            </div>
            {taskRequestPreviewError ? <Alert className="task-flow-request-alert" type="error" showIcon message={taskRequestPreviewError} /> : null}
            <Input.TextArea className="task-flow-code task-flow-request-preview" readOnly rows={8} spellCheck={false} value={taskRequestPreview} />
          </div>

          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Send size={15} />} loading={sendTaskRequestMutation.isPending} disabled={Boolean(taskRequestPreviewError)}>
              {t('taskFlows.request.send')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={editingFlow ? t('taskFlows.actions.edit') : t('taskFlows.actions.create')}
        open={flowModalOpen}
        width={1080}
        onCancel={() => {
          setFlowModalOpen(false)
          setEditingFlow(undefined)
          form.resetFields()
        }}
        footer={null}
      >
        <Form form={form} layout="vertical" onFinish={(values) => saveFlowMutation.mutate(values)}>
          <div className="task-flow-form-grid">
            <Form.Item name="project_id" label={t('taskFlows.fields.project')} rules={[{ required: true }]}>
              <Select
                disabled={Boolean(editingFlow)}
                options={projects.map((project) => ({ label: `${displayProjectName(project)} · ${projectCode(project)}`, value: project.id }))}
              />
            </Form.Item>
            <Form.Item name="flow_code" label={t('taskFlows.fields.flowCode')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="name" label={t('taskFlows.fields.name')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="enabled" label={t('taskFlows.fields.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="trigger_type" label={t('taskFlows.fields.triggerType')} rules={[{ required: true }]}>
              <Select options={triggerOptions} />
            </Form.Item>
            <Form.Item name="action_type" label={t('taskFlows.fields.actionType')} rules={[{ required: true }]}>
              <Select loading={modulesQuery.isFetching} options={moduleActionOptions} onChange={resetModuleParamsForAction} />
            </Form.Item>
            <Form.Item name="priority" label={t('taskFlows.fields.priority')}>
              <InputNumber style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="timeout_ms" label={t('taskFlows.fields.timeoutMs')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="cooldown_ms" label={t('taskFlows.fields.cooldownMs')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="hold_ms" label={t('taskFlows.fields.holdMs')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="schedule_interval_ms" label={t('taskFlows.fields.scheduleIntervalMs')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item className="task-flow-form-wide" name="remark" label={t('taskFlows.fields.remark')}>
              <Input />
            </Form.Item>
          </div>

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.vars.title')}</strong>
              <span>{t('taskFlows.vars.hint')}</span>
            </div>
            <Form.List name="vars">
              {(fields, { add, remove }) => (
                <div className="task-flow-var-list">
                  {fields.map((field) => (
                    <div className="task-flow-var-row" key={field.key}>
                      <Form.Item {...field} name={[field.name, 'var_id']} rules={[{ required: true }]}>
                        <Select
                          showSearch
                          optionFilterProp="label"
                          placeholder={t('taskFlows.vars.variable')}
                          options={projectVariables.map((variable) => ({
                            label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
                            value: variableWireId(variable),
                          }))}
                        />
                      </Form.Item>
                      <Form.Item {...field} name={[field.name, 'role']}>
                        <Select options={roleOptions} />
                      </Form.Item>
                      <Button danger size="small" icon={<Trash2 size={13} />} onClick={() => remove(field.name)} />
                    </div>
                  ))}
                  <Button size="small" icon={<Plus size={14} />} onClick={() => add({ role: 'watch' })}>
                    {t('taskFlows.vars.add')}
                  </Button>
                </div>
              )}
            </Form.List>
          </div>

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.params.title')}</strong>
              <span>{t('taskFlows.params.hint')}</span>
            </div>
            <TaskFlowModuleParamsEditor
              module={selectedModule}
              params={moduleParams}
              projects={projects}
              variables={projectVariables}
              language={i18n.resolvedLanguage}
              onChange={applyModuleParams}
            />
          </div>

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.steps.title')}</strong>
              <span>{t('taskFlows.steps.hint')}</span>
            </div>
            <Form.Item name="steps_json">
              <Input.TextArea className="task-flow-code" rows={8} spellCheck={false} />
            </Form.Item>
          </div>

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.scripts.condition')}</strong>
              <span>{t('taskFlows.scripts.conditionHint')}</span>
            </div>
            <Form.Item name="condition_script">
              <Input.TextArea className="task-flow-code" rows={5} spellCheck={false} />
            </Form.Item>
          </div>

          <div className="task-flow-section">
            <div className="task-flow-section-head">
              <strong>{t('taskFlows.scripts.action')}</strong>
              <span>{t('taskFlows.scripts.actionHint')}</span>
            </div>
            <Form.Item name="action_script">
              <Input.TextArea className="task-flow-code" rows={7} spellCheck={false} />
            </Form.Item>
            <Form.Item name="action_payload" label={t('taskFlows.fields.actionPayload')}>
              <Input.TextArea className="task-flow-code" rows={3} spellCheck={false} />
            </Form.Item>
          </div>

          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveFlowMutation.isPending}>
              {t('taskFlows.actions.save')}
            </Button>
          </div>
        </Form>
      </Modal>
    </div>
  )
}

function TaskFlowModuleParamsEditor({
  language,
  module,
  onChange,
  params,
  projects,
  variables,
}: {
  language?: string
  module?: TaskFlowModule
  onChange: (params: Record<string, unknown>) => void
  params: Record<string, unknown>
  projects: Project[]
  variables: VariableConfig[]
}) {
  const { t } = useTranslation()
  const entries = getParamSchemaEntries(module)

  if (!module || entries.length === 0) {
    return <Alert type="info" showIcon message={t('taskFlows.params.empty')} />
  }

  function updateParam(name: string, value: unknown) {
    onChange({ ...params, [name]: value })
  }

  return (
    <div className="task-flow-param-grid">
      {entries.map(([name, schema]) => (
        <div className={isWideParam(schema) ? 'task-flow-param-field wide' : 'task-flow-param-field'} key={name}>
          <label>
            <span>
              {name}
              {schema.required ? <b>*</b> : null}
            </span>
            <small>{[schema.type, schema.source?.length ? `${t('taskFlows.params.source')}: ${schema.source.join('/')}` : undefined].filter(Boolean).join(' · ')}</small>
          </label>
          <TaskFlowParamInput
            language={language}
            name={name}
            projects={projects}
            schema={schema}
            value={params[name]}
            variables={variables}
            onChange={updateParam}
          />
          {schema.description ? <p>{schema.description}</p> : null}
        </div>
      ))}
    </div>
  )
}

function TaskRequestProcessParamsEditor() {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.processParams')}</strong>
        <span>{t('taskFlows.request.processParamsHint')}</span>
      </div>
      <Form.List name="process_params">
        {(fields, { add, remove }) => (
          <div className="task-flow-param-list">
            {fields.map((field) => (
              <div className="task-flow-request-row process" key={field.key}>
                <Form.Item {...field} name={[field.name, 'key']}>
                  <Input placeholder="inlet_area_m2" />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'value_type']}>
                  <Select options={scalarValueTypeOptions} />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'value']}>
                  <Input placeholder={t('taskFlows.request.value')} />
                </Form.Item>
                <Button danger size="small" icon={<Trash2 size={13} />} onClick={() => remove(field.name)} />
              </div>
            ))}
            <Button size="small" icon={<Plus size={14} />} onClick={() => add({ value_type: 'string' })}>
              {t('taskFlows.request.addProcessParam')}
            </Button>
          </div>
        )}
      </Form.List>
    </div>
  )
}

function TaskRequestRunControlEditor({ command }: { command?: string }) {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.runControl')}</strong>
        <span>{t('taskFlows.request.runControlHint')}</span>
      </div>
      <div className="task-flow-form-grid">
        <Form.Item name="task_id" label="task_id">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        {command === 'stop_detection' ? (
          <Form.Item name="end_type" label="end_type">
            <Select allowClear options={[{ label: 'manual_stop', value: 'manual_stop' }, { label: 'task_flow_stop', value: 'task_flow_stop' }]} />
          </Form.Item>
        ) : null}
        {command === 'pause_detection' || command === 'stop_detection' ? (
          <Form.Item className="task-flow-form-wide" name="reason" label="reason">
            <Input />
          </Form.Item>
        ) : null}
      </div>
    </div>
  )
}

function TaskRequestLimitEditor({
  language,
  variables,
}: {
  language?: string
  variables: VariableConfig[]
}) {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.limitAdjust')}</strong>
        <span>{t('taskFlows.request.limitAdjustHint')}</span>
      </div>
      <div className="task-flow-form-grid">
        <Form.Item name="task_id" label="task_id">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="var_id" label="var_id">
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            options={variables.map((variable) => ({
              label: `${variableTitle(variable, language)} · ${variable.var_name}`,
              value: variableWireId(variable),
            }))}
          />
        </Form.Item>
        <Form.Item name="check_enabled" label="check_enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="alarm_enabled" label="alarm_enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="store_enabled" label="store_enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="limit_ll" label="limit_ll">
          <InputNumber style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="limit_l" label="limit_l">
          <InputNumber style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="limit_h" label="limit_h">
          <InputNumber style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="limit_hh" label="limit_hh">
          <InputNumber style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="limit_deadband" label="limit_deadband">
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="check_cycle_ms" label="check_cycle_ms">
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="violation_hold_ms" label="violation_hold_ms">
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="recover_hold_ms" label="recover_hold_ms">
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item className="task-flow-form-wide" name="items" label="items JSON">
          <Input.TextArea className="task-flow-code" rows={3} spellCheck={false} placeholder={t('taskFlows.request.limitItemsPlaceholder')} />
        </Form.Item>
      </div>
    </div>
  )
}

function TaskRequestReportEditor() {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.reportRegister')}</strong>
        <span>{t('taskFlows.request.reportRegisterHint')}</span>
      </div>
      <div className="task-flow-form-grid">
        <Form.Item name="task_id" label="task_id">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="file_ref" label="file_ref">
          <Input />
        </Form.Item>
        <Form.Item name="file_name" label="file_name">
          <Input />
        </Form.Item>
        <Form.Item name="status" label="status">
          <Select allowClear options={[{ label: 'pending', value: 'pending' }, { label: 'generated', value: 'generated' }, { label: 'failed', value: 'failed' }]} />
        </Form.Item>
        <Form.Item name="template_id" label="template_id">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="template_code" label="template_code">
          <Input />
        </Form.Item>
        <Form.Item name="template_version" label="template_version">
          <InputNumber min={0} style={{ width: '100%' }} />
        </Form.Item>
      </div>
    </div>
  )
}

function TaskRequestStoragePrepareEditor() {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.storagePrepare')}</strong>
        <span>{t('taskFlows.request.storagePrepareHint')}</span>
      </div>
      <div className="task-flow-form-grid">
        <Form.Item name="task_id" label="task_id">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
      </div>
    </div>
  )
}

function TaskRequestHttpEditor() {
  const { t } = useTranslation()
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.httpRequest')}</strong>
        <span>{t('taskFlows.request.httpRequestHint')}</span>
      </div>
      <div className="task-flow-form-grid">
        <Form.Item name="method" label="method">
          <Select options={['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((method) => ({ label: method, value: method }))} />
        </Form.Item>
        <Form.Item className="task-flow-form-wide" name="url" label="url">
          <Input placeholder="http://127.0.0.1:18080/health" />
        </Form.Item>
        <Form.Item className="task-flow-form-wide" name="headers" label="headers JSON">
          <Input.TextArea className="task-flow-code" rows={3} spellCheck={false} placeholder='{"X-Request-Source":"task-flow"}' />
        </Form.Item>
        <Form.Item className="task-flow-form-wide" name="body" label="body">
          <Input.TextArea className="task-flow-code" rows={3} spellCheck={false} />
        </Form.Item>
        <Form.Item name="timeout_ms" label="timeout_ms">
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
      </div>
    </div>
  )
}

function TaskRequestPLCWritesEditor({
  language,
  variables,
}: {
  language?: string
  variables: VariableConfig[]
}) {
  const { t } = useTranslation()
  const writableVariables = variables.filter((variable) => variable.writable || variable.rw_mode === 'W' || variable.rw_mode === 'RW')
  return (
    <div className="task-flow-section">
      <div className="task-flow-section-head">
        <strong>{t('taskFlows.request.plcWrites')}</strong>
        <span>{t('taskFlows.request.plcWritesHint')}</span>
      </div>
      <Alert className="task-flow-request-alert" type="warning" showIcon message={t('taskFlows.request.plcWritesRequireVar')} />
      <Form.List name="plc_writes">
        {(fields, { add, remove }) => (
          <div className="task-flow-param-list">
            {fields.map((field) => (
              <div className="task-flow-request-row plc" key={field.key}>
                <Form.Item {...field} name={[field.name, 'var_id']}>
                  <Select
                    allowClear
                    showSearch
                    optionFilterProp="label"
                    placeholder={t('taskFlows.vars.variable')}
                    options={writableVariables.map((variable) => ({
                      label: `${variableTitle(variable, language)} · ${variable.var_name}`,
                      value: variableWireId(variable),
                    }))}
                  />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'value_from']}>
                  <Input placeholder="process_params.inlet_area_m2" />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'value_type']}>
                  <Select options={scalarValueTypeOptions} />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'value']}>
                  <Input placeholder={t('taskFlows.request.literalValue')} />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'wait_ack']} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'ack_timeout_sec']}>
                  <InputNumber min={0} placeholder="ack" />
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'settle_ms']}>
                  <InputNumber min={0} placeholder="settle" />
                </Form.Item>
                <Button danger size="small" icon={<Trash2 size={13} />} onClick={() => remove(field.name)} />
              </div>
            ))}
            <Button size="small" icon={<Plus size={14} />} onClick={() => add({ value_from: 'process_params.inlet_area_m2', value_type: 'number', wait_ack: true, ack_timeout_sec: 5 })}>
              {t('taskFlows.request.addPLCWrite')}
            </Button>
          </div>
        )}
      </Form.List>
    </div>
  )
}

function TaskFlowParamInput({
  language,
  name,
  onChange,
  projects,
  schema,
  value,
  variables,
}: {
  language?: string
  name: string
  onChange: (name: string, value: unknown) => void
  projects: Project[]
  schema: TaskModuleParamSchema
  value: unknown
  variables: VariableConfig[]
}) {
  const { t } = useTranslation()
  const type = schema.type ?? 'string'
  if (type === 'boolean') {
    return <Switch checked={Boolean(value)} onChange={(checked) => onChange(name, checked)} />
  }
  if (type === 'number' || type === 'detection_run' || type === 'report_template' || type === 'detection_standard') {
    return <InputNumber className="task-flow-param-control" value={toNumberValue(value)} onChange={(next) => onChange(name, next ?? undefined)} />
  }
  if (type === 'project') {
    return (
      <Select
        allowClear
        className="task-flow-param-control"
        showSearch
        optionFilterProp="label"
        value={toNumberValue(value)}
        options={projects.map((project) => ({
          label: `${project.display_name || project.name || projectCode(project)} · ${projectCode(project)}`,
          value: project.id,
        }))}
        onChange={(next) => onChange(name, next)}
      />
    )
  }
  if (type === 'variable') {
    return (
      <Select
        allowClear
        className="task-flow-param-control"
        showSearch
        optionFilterProp="label"
        value={value === undefined || value === null ? undefined : String(value)}
        options={variables.map((variable) => ({
          label: `${variableTitle(variable, language)} · ${variable.var_name}`,
          value: variableWireId(variable),
        }))}
        onChange={(next) => onChange(name, next)}
      />
    )
  }
  if (type === 'select') {
    return (
      <Select
        allowClear
        className="task-flow-param-control"
        value={typeof value === 'string' ? value : undefined}
        options={(schema.options ?? []).map((option) => ({ label: option, value: option }))}
        onChange={(next) => onChange(name, next)}
      />
    )
  }
  if (type === 'object' || type === 'array') {
    return (
      <JsonParamInput
        key={formatJsonDraft(value)}
        placeholder={t('taskFlows.params.jsonPlaceholder')}
        value={value}
        onChange={(next) => onChange(name, next)}
      />
    )
  }
  if (type === 'text' || type === 'code') {
    return (
      <Input.TextArea
        className="task-flow-code"
        rows={type === 'code' ? 6 : 3}
        spellCheck={false}
        value={typeof value === 'string' ? value : value === undefined ? '' : JSON.stringify(value, null, 2)}
        onChange={(event) => onChange(name, event.target.value)}
      />
    )
  }
  return (
    <Input
      className="task-flow-param-control"
      value={value === undefined || value === null ? '' : String(value)}
      onChange={(event) => onChange(name, event.target.value)}
    />
  )
}

function JsonParamInput({ onChange, placeholder, value }: { onChange: (value: unknown) => void; placeholder: string; value: unknown }) {
  return (
    <Input.TextArea
      className="task-flow-code"
      rows={5}
      spellCheck={false}
      placeholder={placeholder}
      defaultValue={formatJsonDraft(value)}
      onBlur={(event) => {
        const trimmed = event.target.value.trim()
        if (!trimmed) {
          onChange(undefined)
          return
        }
        try {
          onChange(JSON.parse(trimmed) as unknown)
        } catch {
          onChange(event.target.value)
        }
      }}
    />
  )
}

function getParamSchemaEntries(module?: TaskFlowModule): Array<[string, TaskModuleParamSchema]> {
  if (!module?.params_schema) return []
  return Object.entries(module.params_schema).map(([name, schema]) => [name, normalizeParamSchema(schema)])
}

function buildTaskRequestPayload(values: TaskRequestFormValues) {
  const payload: Record<string, unknown> = {
    command: values.command,
    project_id: values.project_id,
  }
  const command = values.command ?? ''

  if (taskRequestStartCommands.has(command)) {
    setIfPresent(payload, 'test_no', values.test_no)
    setIfPresent(payload, 'standard_id', values.standard_id)
    setIfPresent(payload, 'duration_sec', values.duration_sec)
    setIfPresent(payload, 'qualified_hold_ms', values.qualified_hold_ms)
    setIfPresent(payload, 'check_interval_ms', values.check_interval_ms)
    setIfPresent(payload, 'limit_check_enabled', values.limit_check_enabled)
    setIfPresent(payload, 'enable_storage', values.enable_storage)
    setIfPresent(payload, 'enable_alarm', values.enable_alarm)
    setIfPresent(payload, 'operator_note', values.operator_note)
    setIfPresent(payload, 'report_template_id', values.report_template_id)
    const reportVarIds = (values.report_var_ids ?? []).filter((item) => item !== undefined && item !== null && item !== '')
    const reportRequest: Record<string, unknown> = {}
    if (reportVarIds.length > 0) reportRequest.var_ids = reportVarIds
    setIfPresent(reportRequest, 'ext_1', values.report_ext_1?.trim())
    setIfPresent(reportRequest, 'ext_2', values.report_ext_2?.trim())
    setIfPresent(reportRequest, 'ext_3', values.report_ext_3?.trim())
    if (Object.keys(reportRequest).length > 0) payload.report_request = reportRequest
    const processParams = Object.fromEntries(
      (values.process_params ?? [])
        .filter((row) => row.key?.trim())
        .map((row) => [row.key!.trim(), normalizeScalarValue(row.value, row.value_type)]),
    )
    if (Object.keys(processParams).length > 0) payload.process_params = processParams
    const customItems = values.custom_items?.trim()
    if (customItems) {
      payload.custom_items = JSON.parse(customItems) as unknown
    }
  }

  if (taskRequestRunControlCommands.has(command)) {
    setIfPresent(payload, 'task_id', values.task_id)
    setIfPresent(payload, 'end_type', values.end_type)
    setIfPresent(payload, 'reason', values.reason)
  }

  if (command === 'update_detection_limits') {
    setIfPresent(payload, 'task_id', values.task_id)
    setIfPresent(payload, 'var_id', values.var_id)
    setIfPresent(payload, 'check_enabled', values.check_enabled)
    setIfPresent(payload, 'alarm_enabled', values.alarm_enabled)
    setIfPresent(payload, 'store_enabled', values.store_enabled)
    setIfPresent(payload, 'limit_ll', values.limit_ll)
    setIfPresent(payload, 'limit_l', values.limit_l)
    setIfPresent(payload, 'limit_h', values.limit_h)
    setIfPresent(payload, 'limit_hh', values.limit_hh)
    setIfPresent(payload, 'limit_deadband', values.limit_deadband)
    setIfPresent(payload, 'check_cycle_ms', values.check_cycle_ms)
    setIfPresent(payload, 'violation_hold_ms', values.violation_hold_ms)
    setIfPresent(payload, 'recover_hold_ms', values.recover_hold_ms)
    const items = values.items?.trim()
    if (items) payload.items = JSON.parse(items) as unknown
  }

  if (command === 'register_report') {
    setIfPresent(payload, 'task_id', values.task_id)
    setIfPresent(payload, 'file_ref', values.file_ref)
    setIfPresent(payload, 'file_name', values.file_name)
    setIfPresent(payload, 'status', values.status)
    setIfPresent(payload, 'template_id', values.template_id)
    setIfPresent(payload, 'template_code', values.template_code)
    setIfPresent(payload, 'template_version', values.template_version)
  }

  if (command === 'storage_prepare') {
    setIfPresent(payload, 'task_id', values.task_id)
  }

  if (command === 'http_request') {
    setIfPresent(payload, 'method', values.method)
    setIfPresent(payload, 'url', values.url)
    setIfPresent(payload, 'body', values.body)
    setIfPresent(payload, 'timeout_ms', values.timeout_ms)
    const headers = values.headers?.trim()
    if (headers) payload.headers = JSON.parse(headers) as unknown
  }

  if (taskRequestStartCommands.has(command) || command === 'write_control_variables') {
    const plcWrites = (values.plc_writes ?? [])
      .filter((row) => row.var_id)
      .map((row) => {
        const item: Record<string, unknown> = { var_id: row.var_id }
        if (row.value_from?.trim()) item.value_from = row.value_from.trim()
        else if (row.value !== undefined && row.value !== '') item.value = normalizeScalarValue(row.value, row.value_type)
        setIfPresent(item, 'wait_ack', row.wait_ack)
        setIfPresent(item, 'ack_timeout_sec', row.ack_timeout_sec)
        setIfPresent(item, 'settle_ms', row.settle_ms)
        return item
      })
    if (plcWrites.length > 0) payload.plc_writes = plcWrites
  }
  return payload
}

function setIfPresent(target: Record<string, unknown>, key: string, value: unknown) {
  if (value === undefined || value === null || value === '') return
  target[key] = value
}

function normalizeScalarValue(value: unknown, type: ScalarValueType = 'string') {
  if (type === 'number') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : undefined
  }
  if (type === 'boolean') {
    if (typeof value === 'boolean') return value
    return String(value).toLowerCase() === 'true' || String(value) === '1'
  }
  return value === undefined || value === null ? '' : String(value)
}

function normalizeParamSchema(schema: unknown): TaskModuleParamSchema {
  if (!schema || typeof schema !== 'object') return { type: 'string' }
  const value = schema as Record<string, unknown>
  return {
    type: typeof value.type === 'string' ? value.type : 'string',
    required: Boolean(value.required),
    default: value.default,
    options: Array.isArray(value.options) ? value.options.filter((item): item is string => typeof item === 'string') : undefined,
    source: Array.isArray(value.source) ? value.source.filter((item): item is string => typeof item === 'string') : undefined,
    description: typeof value.description === 'string' ? value.description : undefined,
    item_schema: value.item_schema && typeof value.item_schema === 'object' ? (value.item_schema as Record<string, TaskModuleParamSchema>) : undefined,
    language: typeof value.language === 'string' ? value.language : undefined,
  }
}

function buildDefaultParams(module: TaskFlowModule) {
  return Object.fromEntries(
    getParamSchemaEntries(module)
      .map(([name, schema]) => [name, getParamDefault(schema)] as const)
      .filter(([, value]) => value !== undefined),
  )
}

function getParamDefault(schema: TaskModuleParamSchema): unknown {
  if (schema.default !== undefined) return schema.default
  if (schema.type === 'boolean') return false
  if (schema.type === 'array') return []
  if (schema.type === 'object') return {}
  return undefined
}

function parseTaskFlowSteps(value: unknown): Array<Record<string, unknown>> {
  if (!value) return []
  try {
    const parsed = typeof value === 'string' ? JSON.parse(value) : value
    if (Array.isArray(parsed)) return parsed.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object')
    if (parsed && typeof parsed === 'object') return [parsed as Record<string, unknown>]
  } catch {
    return []
  }
  return []
}

function getInitialModuleParams(stepsJson: unknown, actionPayload: unknown) {
  const firstStep = parseTaskFlowSteps(stepsJson)[0]
  if (firstStep?.params && typeof firstStep.params === 'object' && !Array.isArray(firstStep.params)) {
    return firstStep.params as Record<string, unknown>
  }
  if (!actionPayload) return {}
  try {
    const parsed = typeof actionPayload === 'string' ? JSON.parse(actionPayload) : actionPayload
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

function isWideParam(schema: TaskModuleParamSchema) {
  return schema.type === 'object' || schema.type === 'array' || schema.type === 'text' || schema.type === 'code'
}

function formatJsonDraft(value: unknown) {
  if (value === undefined || value === null || value === '') return ''
  if (typeof value === 'string') return value
  return JSON.stringify(value, null, 2)
}

function toNumberValue(value: unknown) {
  if (typeof value === 'number') return value
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : undefined
  }
  return undefined
}

function TaskFlowRunsTable({
  loading,
  onSelectRun,
  projects,
  runs,
}: {
  loading: boolean
  onSelectRun: (runId: number) => void
  projects: Map<number, Project>
  runs: TaskFlowRun[]
}) {
  const { t } = useTranslation()
  const columns: TableColumnsType<TaskFlowRun> = [
    { title: t('taskFlows.logs.flow'), dataIndex: 'flow_code', key: 'flow_code', width: 180 },
    {
      title: t('taskFlows.logs.project'),
      dataIndex: 'project_id',
      key: 'project_id',
      width: 130,
      render: (value: number) => projectCode(projects.get(value)) || value,
    },
    { title: t('taskFlows.logs.trigger'), dataIndex: 'trigger_type', key: 'trigger_type', width: 130 },
    { title: t('taskFlows.logs.status'), dataIndex: 'status', key: 'status', width: 110, render: (value: string) => <Tag>{value}</Tag> },
    { title: t('taskFlows.logs.duration'), dataIndex: 'duration_ms', key: 'duration_ms', width: 110, render: (value: number) => `${value ?? 0} ms` },
    { title: t('taskFlows.logs.startedAt'), dataIndex: 'started_at', key: 'started_at', width: 180, render: (value: string) => formatTime(value) },
    {
      title: t('taskFlows.columns.actions'),
      key: 'actions',
      width: 110,
      render: (_, record) => (
        <Button size="small" onClick={() => onSelectRun(record.id)}>
          {t('taskFlows.logs.viewSql')}
        </Button>
      ),
    },
  ]
  return <Table size="small" rowKey="id" loading={loading} columns={columns} dataSource={runs} pagination={{ pageSize: 8, size: 'small' }} scroll={{ x: 950 }} />
}

function TaskFlowSqlLogsTable({ loading, logs }: { loading: boolean; logs: TaskFlowSqlLog[] }) {
  const { t } = useTranslation()
  const columns: TableColumnsType<TaskFlowSqlLog> = [
    { title: t('taskFlows.logs.sqlText'), dataIndex: 'sql_text', key: 'sql_text', width: 360, ellipsis: true },
    { title: t('taskFlows.logs.affectedRows'), dataIndex: 'affected_rows', key: 'affected_rows', width: 120 },
    { title: t('taskFlows.logs.duration'), dataIndex: 'duration_ms', key: 'duration_ms', width: 110, render: (value: number) => `${value ?? 0} ms` },
    { title: t('taskFlows.logs.error'), dataIndex: 'error_message', key: 'error_message', width: 220, ellipsis: true, render: (value: string) => value || '-' },
    { title: t('taskFlows.logs.createdAt'), dataIndex: 'created_at', key: 'created_at', width: 180, render: (value: string) => formatTime(value) },
  ]
  return <Table size="small" rowKey="id" loading={loading} columns={columns} dataSource={logs} pagination={{ pageSize: 8, size: 'small' }} scroll={{ x: 990 }} />
}

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
