import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, Button, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { Braces, Edit3, Play, Plus, Save, ScrollText, Trash2, Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { queryClient } from '@/app/queryClient'
import {
  createTaskFlow,
  getDevices,
  getTaskFlows,
  getVariables,
  runTaskFlow,
  updateTaskFlow,
} from '@/features/edge-status/api'
import type { Device, TaskFlow, TaskFlowPayload, VariableConfig } from '@/shared/api/types'
import '@/features/settings/settings.css'
import './task-flows.css'

type TaskFlowFormValues = TaskFlowPayload
type TaskFlowFilter = 'all' | number

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

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  if (language === 'en') return variable.display_name_en || variable.display_name || variable.raw_name || variable.var_name
  if (language === 'ja') return variable.display_name_ja || variable.display_name || variable.raw_name || variable.var_name
  return variable.display_name || variable.raw_name || variable.var_name
}

export function TaskFlowsPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [projectFilter, setProjectFilter] = useState<TaskFlowFilter>('all')
  const [triggerFilter, setTriggerFilter] = useState<string | undefined>()
  const [flowModalOpen, setFlowModalOpen] = useState(false)
  const [editingFlow, setEditingFlow] = useState<TaskFlow | undefined>()
  const [form] = Form.useForm<TaskFlowFormValues>()
  const formProjectId = Form.useWatch('project_id', form)

  const devicesQuery = useQuery({
    queryKey: ['task-flows', 'projects'],
    queryFn: getDevices,
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

  const devices = useMemo(() => devicesQuery.data ?? [], [devicesQuery.data])
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const flows = useMemo(() => flowsQuery.data ?? [], [flowsQuery.data])
  const enabledFlows = flows.filter((item) => item.enabled)
  const variableById = useMemo(() => new Map(variables.map((variable) => [variable.var_id, variable])), [variables])
  const deviceById = useMemo(() => new Map(devices.map((device) => [device.id, device])), [devices])
  const projectVariables = useMemo(() => {
    return variables.filter((variable) => !formProjectId || variable.device_id === formProjectId)
  }, [formProjectId, variables])

  const displayDeviceName = (device?: Device) => {
    if (!device) return '-'
    if (i18n.resolvedLanguage === 'en') return device.display_name_en || device.display_name || device.name || device.device_code
    if (i18n.resolvedLanguage === 'ja') return device.display_name_ja || device.display_name || device.name || device.device_code
    return device.display_name || device.name || device.device_code
  }

  function normalizePayload(values: TaskFlowFormValues): TaskFlowPayload {
    return {
      ...values,
      enabled: values.enabled ?? true,
      timeout_ms: values.timeout_ms ?? 3000,
      cooldown_ms: values.cooldown_ms ?? 0,
      hold_ms: values.hold_ms ?? 0,
      priority: values.priority ?? 0,
      vars: (values.vars ?? [])
        .filter((item) => item.var_id)
        .map((item) => {
          const variable = variableById.get(Number(item.var_id))
          return {
            var_id: Number(item.var_id),
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
    onSuccess: () => messageApi.success(t('taskFlows.messages.queued')),
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

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
        priority: flow.priority,
        remark: flow.remark,
        vars: flow.vars?.map((item) => ({ var_id: item.var_id, var_name: item.var_name, role: item.role })) ?? [],
      })
    } else {
      const project = projectFilter === 'all' ? devices[0] : deviceById.get(projectFilter)
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
        priority: 0,
        remark: '',
        vars: [],
      })
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
      render: (value: number) => displayDeviceName(deviceById.get(value)),
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
          {devices.map((device) => (
            <button
              className={projectFilter === device.id ? 'task-project-card active' : 'task-project-card'}
              key={device.id}
              onClick={() => setProjectFilter(device.id)}
            >
              <strong>{displayDeviceName(device)}</strong>
              <span>{flows.filter((flow) => flow.project_id === device.id).length}</span>
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
              <strong>{t('taskFlows.summary.pending')}</strong>
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
                  children: <Alert type="info" showIcon message={t('taskFlows.logs.pendingRuns')} />,
                },
                {
                  key: 'sql',
                  label: t('taskFlows.logs.sql'),
                  children: <Alert type="info" showIcon message={t('taskFlows.logs.pendingSql')} />,
                },
              ]}
            />
          </section>
        </main>
      </div>

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
                options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
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
              <Select options={actionOptions} />
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
                            value: variable.var_id,
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
