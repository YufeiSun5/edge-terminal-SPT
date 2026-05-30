import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import { Copy, Edit3, Plus, Save, ShieldCheck, Trash2 } from 'lucide-react'
import { queryClient } from '@/app/queryClient'
import {
  createDetectionStandard,
  deleteDetectionStandard,
  getDetectionStandard,
  getDetectionStandards,
  getDevices,
  getVariables,
  replaceDetectionStandardItems,
  updateDetectionStandard,
} from '@/features/edge-status/api'
import type { DetectionStandard, DetectionStandardItemPayload, DetectionStandardPayload, Device, VariableConfig } from '@/shared/api/types'
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

export function DetectionConfigPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [selectedStandardId, setSelectedStandardId] = useState<number | undefined>()
  const [editingStandard, setEditingStandard] = useState<DetectionStandard | undefined>()
  const [standardItems, setStandardItems] = useState<DetectionStandardItemPayload[]>([])
  const [standardVariableId, setStandardVariableId] = useState<number | undefined>()
  const [standardModalOpen, setStandardModalOpen] = useState(false)
  const [standardForm] = Form.useForm<DetectionStandardFormValues>()

  const standardsQuery = useQuery({
    queryKey: ['detection-config', 'standards'],
    queryFn: () => getDetectionStandards(),
    retry: false,
  })
  const devicesQuery = useQuery({
    queryKey: ['detection-config', 'devices'],
    queryFn: getDevices,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['detection-config', 'variables'],
    queryFn: () => getVariables(),
    retry: false,
  })

  const standards = useMemo(() => standardsQuery.data ?? [], [standardsQuery.data])
  const devices = useMemo(() => devicesQuery.data ?? [], [devicesQuery.data])
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
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

  const displayDeviceName = (device: Device) => {
    if (i18n.resolvedLanguage === 'en') return device.display_name_en || device.display_name || device.name || device.device_code
    if (i18n.resolvedLanguage === 'ja') return device.display_name_ja || device.display_name || device.name || device.device_code
    return device.display_name || device.name || device.device_code
  }

  const displayStandardName = (standard: DetectionStandard) => {
    if (i18n.resolvedLanguage === 'en') return standard.display_name_en || standard.display_name || standard.name || standard.standard_code
    if (i18n.resolvedLanguage === 'ja') return standard.display_name_ja || standard.display_name || standard.name || standard.standard_code
    return standard.display_name || standard.name || standard.standard_code
  }

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
        device_id: detail.device_id,
        device_code: detail.device_code,
        mode: detail.mode,
        version: detail.version,
        enabled: detail.enabled,
        remark: detail.remark,
      })
      setStandardItems((detail.items ?? []).map((item) => ({
        var_id: item.var_id,
        var_name: item.var_name,
        display_name: item.display_name,
        display_name_en: item.display_name_en,
        display_name_ja: item.display_name_ja,
        check_enabled: item.check_enabled,
        store_enabled: item.store_enabled,
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
      })))
    } else {
      standardForm.setFieldsValue({
        standard_code: `STD-${Date.now().toString().slice(-6)}`,
        mode: 'standard',
        version: 1,
        enabled: true,
        remark: '',
      })
      setStandardItems([])
    }
    setStandardModalOpen(true)
  }

  function addStandardItem(variableId?: number) {
    if (!variableId) return
    const variable = standardVariables.find((item) => item.var_id === variableId)
    if (!variable || standardItems.some((item) => item.var_name === variable.var_name)) return
    setStandardItems((items) => [
      ...items,
      {
        var_id: variable.var_id,
        var_name: variable.var_name,
        display_name: variable.display_name || variable.raw_name || variable.var_name,
        display_name_en: variable.display_name_en,
        display_name_ja: variable.display_name_ja,
        check_enabled: true,
        store_enabled: true,
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
        sort_order: standardItems.length + 1,
      },
    ])
    setStandardVariableId(undefined)
  }

  function patchStandardItem(varId: number, patch: Partial<DetectionStandardItemPayload>) {
    setStandardItems((items) => items.map((item) => item.var_id === varId ? { ...item, ...patch } : item))
  }

  function removeStandardItem(varId: number) {
    setStandardItems((items) => items.filter((item) => item.var_id !== varId).map((item, index) => ({ ...item, sort_order: index + 1 })))
  }

  const saveStandardMutation = useMutation({
    mutationFn: async (values: DetectionStandardFormValues) => {
      const device = devices.find((item) => item.id === values.device_id)
      const payload: DetectionStandardPayload = {
        ...values,
        device_code: device?.device_code ?? values.device_code ?? '',
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
      setStandardVariableId(undefined)
      standardForm.resetFields()
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
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const standardColumns: TableColumnsType<DetectionStandard> = [
    {
      title: t('settings.standards.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      render: (_, record) => (
        <button className="detection-link-button" onClick={() => setSelectedStandardId(record.id)}>
          <strong>{displayStandardName(record)}</strong>
          <span>{record.standard_code}</span>
        </button>
      ),
    },
    {
      title: t('settings.variables.device'),
      dataIndex: 'device_code',
      key: 'device_code',
      width: 130,
      render: (value, record) => record.device_id ? value : <Tag>{t('settings.standards.global')}</Tag>,
    },
    { title: t('settings.standards.mode'), dataIndex: 'mode', key: 'mode', width: 110 },
    { title: t('settings.standards.version'), dataIndex: 'version', key: 'version', width: 80 },
    {
      title: t('settings.standards.items'),
      dataIndex: 'items',
      key: 'items',
      width: 90,
      render: (items?: unknown[]) => items?.length ?? 0,
    },
    {
      title: t('settings.variables.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'success' : 'default'}>{enabled ? t('status.online') : t('status.offline')}</Tag>,
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 150,
      fixed: 'right',
      render: (_, record) => (
        <Space size={6}>
          <Button size="small" icon={<Edit3 size={13} />} onClick={() => void openStandardModal(record)} />
          <Popconfirm
            title={t('settings.standards.deleteConfirm', { code: record.standard_code })}
            okText={t('settings.users.delete')}
            cancelText={t('settings.actions.cancel')}
            onConfirm={() => deleteStandardMutation.mutate(record)}
          >
            <Button danger size="small" icon={<Trash2 size={13} />} loading={deleteStandardMutation.isPending} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const itemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      fixed: 'left',
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
      width: 90,
      render: (value: boolean) => <Tag color={value ? 'success' : 'default'}>{value ? t('settings.standards.check') : t('settings.standards.skip')}</Tag>,
    },
    {
      title: t('settings.standards.store'),
      dataIndex: 'store_enabled',
      key: 'store_enabled',
      width: 90,
      render: (value: boolean) => <Tag color={value ? 'processing' : 'default'}>{value ? t('settings.standards.store') : '-'}</Tag>,
    },
    { title: t('settings.standards.checkMethod'), dataIndex: 'check_method', key: 'check_method', width: 140 },
    { title: t('settings.standards.targetValue'), dataIndex: 'target_value', key: 'target_value', width: 120 },
    { title: 'LL', dataIndex: 'limit_ll', key: 'limit_ll', width: 90, render: (value) => value ?? '-' },
    { title: 'L', dataIndex: 'limit_l', key: 'limit_l', width: 90, render: (value) => value ?? '-' },
    { title: 'H', dataIndex: 'limit_h', key: 'limit_h', width: 90, render: (value) => value ?? '-' },
    { title: 'HH', dataIndex: 'limit_hh', key: 'limit_hh', width: 90, render: (value) => value ?? '-' },
    { title: t('settings.standards.limitDeadband'), dataIndex: 'limit_deadband', key: 'limit_deadband', width: 110 },
    { title: t('settings.standards.violationHold'), dataIndex: 'violation_hold_ms', key: 'violation_hold_ms', width: 130 },
    { title: t('settings.standards.recoverHold'), dataIndex: 'recover_hold_ms', key: 'recover_hold_ms', width: 130 },
    { title: t('settings.standards.qualityPolicy'), dataIndex: 'quality_policy', key: 'quality_policy', width: 140 },
    { title: t('settings.variables.unit'), dataIndex: 'unit', key: 'unit', width: 80 },
  ]

  const editableItemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      fixed: 'left',
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
            { label: 'numeric_range', value: 'numeric_range' },
            { label: 'bool_equals', value: 'bool_equals' },
            { label: 'string_equals', value: 'string_equals' },
            { label: 'regex', value: 'regex' },
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
            { label: 'ignore_bad', value: 'ignore_bad' },
            { label: 'record_invalid', value: 'record_invalid' },
            { label: 'fail_on_bad', value: 'fail_on_bad' },
          ]}
        />
      ),
    },
    { title: t('settings.variables.unit'), dataIndex: 'unit', key: 'unit', width: 80 },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 70,
      fixed: 'right',
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

      <header className="detection-config-hero">
        <div>
          <span className="settings-eyebrow">{t('detectionConfig.eyebrow')}</span>
          <h1>{t('detectionConfig.title')}</h1>
          <p>{t('detectionConfig.subtitle')}</p>
        </div>
        <div className="detection-config-actions">
          <Button icon={<Plus size={15} />} type="primary" onClick={() => void openStandardModal()}>
            {t('settings.standards.create')}
          </Button>
          {selectedStandardDetail ? (
            <Button icon={<Edit3 size={15} />} onClick={() => void openStandardModal(selectedStandardDetail)}>
              {t('settings.standards.edit')}
            </Button>
          ) : null}
        </div>
      </header>

      <div className="detection-config-grid">
        <aside className="detection-config-sidebar glass-panel">
          <div className="detection-panel-head">
            <div>
              <span className="settings-eyebrow">{t('detectionConfig.standardList')}</span>
              <h2>{standards.length}</h2>
            </div>
            <ShieldCheck size={18} />
          </div>
          <div className="detection-standard-list">
            {standards.map((standard) => (
              <button
                key={standard.id}
                className={selectedStandard?.id === standard.id ? 'detection-standard-card active' : 'detection-standard-card'}
                onClick={() => setSelectedStandardId(standard.id)}
              >
                <strong>{displayStandardName(standard)}</strong>
                <span>{standard.standard_code}</span>
                <em>{standard.items?.length ?? 0} {t('settings.standards.items')}</em>
              </button>
            ))}
            {standards.length === 0 ? <div className="detection-empty">{t('detectionConfig.noStandards')}</div> : null}
          </div>
          <div className="detection-legacy-panel">
            <div className="detection-panel-head compact">
              <div>
                <span className="settings-eyebrow">{t('detectionConfig.legacySource')}</span>
                <h2>{LEGACY_DETECTION_ITEMS.length}</h2>
              </div>
              <Copy size={16} />
            </div>
            <div className="detection-legacy-tags">
              {LEGACY_DETECTION_ITEMS.map((item) => <Tag key={item}>{item}</Tag>)}
            </div>
          </div>
        </aside>

        <main className="detection-config-main glass-panel">
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
            <div className="detection-summary-strip">
              <div>
                <span>{t('settings.variables.device')}</span>
                <strong>{selectedStandardDetail.device_id ? selectedStandardDetail.device_code : t('settings.standards.global')}</strong>
              </div>
              <div>
                <span>{t('settings.standards.items')}</span>
                <strong>{selectedStandardDetail.items?.length ?? 0}</strong>
              </div>
              <div>
                <span>{t('settings.standards.version')}</span>
                <strong>V{selectedStandardDetail.version}</strong>
              </div>
              <div>
                <span>{t('settings.standards.remark')}</span>
                <strong>{selectedStandardDetail.remark || '-'}</strong>
              </div>
            </div>
          ) : null}

          <Table
            className="detection-config-table"
            size="small"
            rowKey="var_id"
            loading={standardsQuery.isFetching || selectedStandardDetailQuery.isFetching}
            columns={itemColumns}
            dataSource={selectedStandardDetail?.items ?? []}
            scroll={{ x: 1550, y: 570 }}
            pagination={false}
          />
        </main>

        <section className="detection-config-standards glass-panel">
          <div className="detection-panel-head">
            <div>
              <span className="settings-eyebrow">{t('detectionConfig.allStandards')}</span>
              <h2>{t('settings.standards.title')}</h2>
            </div>
            <Button size="small" icon={<Plus size={14} />} onClick={() => void openStandardModal()}>
              {t('settings.standards.create')}
            </Button>
          </div>
          <Table
            size="small"
            rowKey="id"
            loading={standardsQuery.isFetching}
            columns={standardColumns}
            dataSource={standards}
            scroll={{ x: 980, y: 260 }}
            pagination={{ pageSize: 10, size: 'small' }}
          />
        </section>
      </div>

      <Modal
        title={editingStandard ? t('settings.standards.edit') : t('settings.standards.create')}
        open={standardModalOpen}
        width={1120}
        onCancel={() => {
          setStandardModalOpen(false)
          setEditingStandard(undefined)
          setStandardItems([])
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
            <Form.Item name="device_id" label={t('settings.variables.selectDevice')}>
              <Select allowClear options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))} />
            </Form.Item>
            <Form.Item name="mode" label={t('settings.standards.mode')}>
              <Input />
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
                options={standardVariables.map((variable) => ({ label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`, value: variable.var_id }))}
              />
              <Button size="small" icon={<Plus size={14} />} onClick={() => addStandardItem(standardVariableId)} disabled={!standardVariableId}>
                {t('settings.standards.add')}
              </Button>
            </div>
          </div>
          <Table
            className="settings-standard-items-table"
            size="small"
            rowKey="var_id"
            columns={editableItemColumns}
            dataSource={standardItems}
            scroll={{ x: 1650, y: 320 }}
            pagination={false}
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
