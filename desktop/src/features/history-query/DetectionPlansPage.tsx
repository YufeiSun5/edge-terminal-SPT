import { useMemo, useState } from 'react'
import { Alert, Button, Form, Input, InputNumber, Modal, Select, Space, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Edit3, Search } from 'lucide-react'
import { getDetectionPlans, updateDetectionPlan } from '@/features/edge-status/api'
import type { DetectionPlan, DetectionPlanUpdatePayload } from '@/shared/api/types'
import { queryClient } from '@/app/queryClient'
import { env } from '@/shared/config/env'
import { formatHistoryTime } from './model'
import './history-query.css'

const planStatusColors: Record<string, string> = {
  pending: 'blue',
  starting: 'gold',
  started: 'green',
  cancelled: 'default',
}

export function DetectionPlansPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [messageApi, contextHolder] = message.useMessage()
  const [form] = Form.useForm<DetectionPlanUpdatePayload>()
  const [status, setStatus] = useState<string>('pending')
  const [keyword, setKeyword] = useState('')
  const [editingPlan, setEditingPlan] = useState<DetectionPlan | null>(null)
  const isMainServer = env.runtimeRole === 'main_server'

  const plansQuery = useQuery({
    queryKey: ['history', 'detection-plans', status, keyword],
    queryFn: () => getDetectionPlans({ status: status || undefined, keyword: keyword.trim() || undefined, limit: 300 }),
    refetchInterval: 30000,
    retry: false,
  })

  const plans = useMemo(() => plansQuery.data?.items ?? [], [plansQuery.data?.items])
  const updateMutation = useMutation({
    mutationFn: (values: DetectionPlanUpdatePayload) => {
      if (!editingPlan) throw new Error('missing plan')
      return updateDetectionPlan(editingPlan.id, values)
    },
    onSuccess: async () => {
      messageApi.success(t('history.plans.saveSuccess'))
      setEditingPlan(null)
      form.resetFields()
      await queryClient.invalidateQueries({ queryKey: ['history', 'detection-plans'] })
      await queryClient.invalidateQueries({ queryKey: ['station', 'detection-plans'] })
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('history.plans.saveFailed'))
    },
  })

  function openEditPlan(plan: DetectionPlan) {
    setEditingPlan(plan)
    form.setFieldsValue({
      plan_no: plan.plan_no,
      source_system: plan.source_system,
      external_plan_id: plan.external_plan_id,
      external_order_no: plan.external_order_no,
      factory_no: plan.factory_no,
      customer_name: plan.customer_name,
      device_model: plan.device_model,
      test_item_code: plan.test_item_code,
      test_item_name: plan.test_item_name,
      test_sequence: plan.test_sequence,
      mode: plan.mode || 'standard',
      standard_code: plan.standard_code,
    })
  }

  const columns: TableColumnsType<DetectionPlan> = [
    { title: t('history.plans.planNo'), dataIndex: 'plan_no', key: 'plan_no', render: (val) => val || '--' },
    { title: t('history.detail.factoryNo'), dataIndex: 'factory_no', key: 'factory_no', render: (val) => val || '--' },
    { title: t('history.plans.deviceModel'), dataIndex: 'device_model', key: 'device_model', render: (val) => val || '--' },
    { title: t('history.plans.testItem'), dataIndex: 'test_item_name', key: 'test_item_name', render: (_, record) => record.test_item_name || record.test_item_code || '--' },
    { title: t('history.plans.standardCode'), dataIndex: 'standard_code', key: 'standard_code', render: (val) => val || '--' },
    {
      title: t('history.plans.status'),
      dataIndex: 'status',
      key: 'status',
      render: (val) => <Tag color={planStatusColors[val] ?? 'default'}>{t(`history.plans.statuses.${val}`, { defaultValue: val })}</Tag>,
    },
    { title: t('history.plans.owner'), dataIndex: 'owner_edge_instance_id', key: 'owner_edge_instance_id', render: (val) => val || '--' },
    { title: t('history.filters.start'), dataIndex: 'started_at', key: 'started_at', render: (val) => formatHistoryTime(val) || '--' },
    {
      title: t('history.timeline.operation'),
      key: 'action',
      render: (_, record) => (
        <Space size={4}>
          {isMainServer && record.status === 'pending' ? (
            <Button type="link" size="small" icon={<Edit3 size={14} />} onClick={() => openEditPlan(record)}>
              {t('history.plans.edit')}
            </Button>
          ) : null}
          {record.started_task_id ? (
            <Button type="link" size="small" onClick={() => navigate(`/history/runs/${record.started_task_id}?tab=data`)}>
              {t('history.timeline.viewDetail')}
            </Button>
          ) : null}
        </Space>
      ),
    },
  ]

  return (
    <div className="history-page prototype-history-page history-dense-layout">
      {contextHolder}
      <main className="history-content">
        {plansQuery.isError ? (
          <Alert
            className="history-api-alert"
            type="warning"
            showIcon
            message={t('history.plans.apiUnavailable')}
            action={<Button size="small" onClick={() => void plansQuery.refetch()}>{t('actions.refresh')}</Button>}
          />
        ) : null}
        <div className="history-gantt-view glass-panel">
          <div className="history-gantt-toolbar">
            <Space wrap>
              <Select
                value={status}
                style={{ width: 150 }}
                onChange={setStatus}
                options={[
                  { value: 'pending', label: t('history.plans.statuses.pending') },
                  { value: 'starting', label: t('history.plans.statuses.starting') },
                  { value: 'started', label: t('history.plans.statuses.started') },
                  { value: 'cancelled', label: t('history.plans.statuses.cancelled') },
                  { value: '', label: t('history.plans.allStatuses') },
                ]}
              />
              <Input
                allowClear
                prefix={<Search size={14} />}
                placeholder={t('history.plans.keyword')}
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                style={{ width: 260 }}
              />
              <Button onClick={() => void plansQuery.refetch()}>{t('actions.refresh')}</Button>
            </Space>
          </div>
          <div className="history-panel-body" style={{ flex: 1, minHeight: 0, overflow: 'auto', padding: 16 }}>
            {!isMainServer ? (
              <Alert className="history-api-alert" type="info" showIcon message={t('history.plans.mainServerOnly')} />
            ) : null}
            <Table
              rowKey="id"
              dataSource={plans}
              columns={columns}
              loading={plansQuery.isFetching}
              size="small"
              pagination={{ pageSize: 20, size: 'small' }}
            />
          </div>
        </div>
      </main>
      <Modal
        title={t('history.plans.editTitle')}
        open={Boolean(editingPlan)}
        onCancel={() => {
          setEditingPlan(null)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        confirmLoading={updateMutation.isPending}
        okText={t('actions.save')}
        cancelText={t('actions.cancel')}
        width={760}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={(values) => updateMutation.mutate(values)}>
          <div className="settings-form-grid">
            <Form.Item name="plan_no" label={t('history.plans.planNo')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="factory_no" label={t('history.detail.factoryNo')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="source_system" label={t('history.plans.sourceSystem')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="external_plan_id" label={t('history.plans.externalPlanId')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="external_order_no" label={t('history.plans.externalOrderNo')}>
              <Input />
            </Form.Item>
            <Form.Item name="customer_name" label={t('history.plans.customerName')}>
              <Input />
            </Form.Item>
            <Form.Item name="device_model" label={t('history.plans.deviceModel')}>
              <Input />
            </Form.Item>
            <Form.Item name="standard_code" label={t('history.plans.standardCode')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="test_item_code" label={t('history.plans.testItemCode')}>
              <Input />
            </Form.Item>
            <Form.Item name="test_item_name" label={t('history.plans.testItem')}>
              <Input />
            </Form.Item>
            <Form.Item name="test_sequence" label={t('history.plans.testSequence')}>
              <InputNumber min={0} precision={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="mode" label={t('history.plans.mode')}>
              <Input />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  )
}
