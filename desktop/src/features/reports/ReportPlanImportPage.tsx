import { useMemo, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Alert, Button, Checkbox, Empty, Input, Space, Statistic, Table, Tag, Typography, Upload, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { UploadProps } from 'antd'
import { CheckCircle2, FileSpreadsheet, UploadCloud } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { confirmMainReportPlanImport, parseMainReportPlanImport } from '@/features/edge-status/api'
import { env } from '@/shared/config/env'
import type { PlanImportIssue, PlanImportRow } from '@/shared/api/types'
import './reports.css'

function confidenceColor(value?: number) {
  if ((value ?? 0) >= 0.9) return 'green'
  if ((value ?? 0) >= 0.7) return 'gold'
  return 'red'
}

function issueText(issues?: PlanImportIssue[]) {
  if (!issues?.length) return ''
  return issues.map((issue) => `${issue.field}:${issue.code}`).join('; ')
}

export function ReportPlanImportPage() {
  const { t } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [edgeInstanceId, setEdgeInstanceId] = useState('')
  const [allowNeedsConfirmation, setAllowNeedsConfirmation] = useState(false)
  const isMainServer = env.runtimeRole === 'main_server'

  const parseMutation = useMutation({
    mutationFn: () => {
      if (!selectedFile) throw new Error(t('reportSettings.planImport.fileRequired'))
      return parseMainReportPlanImport(selectedFile, edgeInstanceId.trim() || undefined)
    },
    onSuccess: (draft) => {
      messageApi.success(t('reportSettings.planImport.parsed', { count: draft.rows.length }))
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.planImport.parseFailed')),
  })

  const draft = parseMutation.data
  const confirmableRows = useMemo(() => draft?.rows ?? [], [draft?.rows])

  const confirmMutation = useMutation({
    mutationFn: () =>
      confirmMainReportPlanImport({
        rows: confirmableRows,
        source_artifact_key: draft?.artifact.artifact_key,
        edge_instance_id: edgeInstanceId.trim() || undefined,
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
      return false
    },
    onRemove: () => {
      setSelectedFile(null)
      parseMutation.reset()
    },
  }

  const columns: ColumnsType<PlanImportRow> = [
    {
      title: t('reportSettings.planImport.columns.row'),
      dataIndex: 'row_number',
      width: 70,
    },
    {
      title: t('reportSettings.planImport.columns.plan'),
      dataIndex: 'test_no',
      width: 190,
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{record.test_no || record.factory_no || '-'}</Typography.Text>
          <Typography.Text type="secondary">{record.factory_no || record.customer_name || '-'}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.project'),
      dataIndex: 'project_code',
      width: 170,
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{record.project_match?.project_code || record.project_code || '-'}</Typography.Text>
          <Tag color={confidenceColor(record.project_match?.confidence)}>{Math.round((record.project_match?.confidence ?? 0) * 100)}%</Tag>
        </Space>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.projectGroup'),
      dataIndex: 'project_group',
      width: 120,
      render: (_value, record) => (
        <Typography.Text>{record.project_match?.project_group || record.project_group || '-'}</Typography.Text>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.variable'),
      dataIndex: 'variable_raw',
      width: 220,
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{record.variable_match?.display_name || record.variable_match?.var_name || record.variable_raw || '-'}</Typography.Text>
          <Typography.Text type="secondary">{record.variable_match?.var_id_text || record.var_id_text || '-'}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.limit'),
      dataIndex: 'limit_raw',
      width: 180,
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{record.limit.normalized || record.limit_raw || '-'}</Typography.Text>
          <Typography.Text type="secondary">
            L {record.limit.limit_l ?? '-'} / H {record.limit.limit_h ?? '-'} {record.limit.unit || record.unit || ''}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.template'),
      dataIndex: 'template_code',
      width: 170,
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{record.template_match?.template_code || record.template_code || '-'}</Typography.Text>
          {record.template_match?.version ? <Tag>v{record.template_match.version}</Tag> : null}
        </Space>
      ),
    },
    {
      title: t('reportSettings.planImport.columns.status'),
      dataIndex: 'issues',
      width: 170,
      render: (_value, record) => {
        const issues = issueText(record.issues)
        if (issues) return <Tag color="red">{issues}</Tag>
        if (record.needs_confirm || record.limit.needs_confirmation) return <Tag color="gold">{t('reportSettings.planImport.needsConfirmation')}</Tag>
        return <Tag color="green">{t('reportSettings.planImport.ready')}</Tag>
      },
    },
  ]

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
          <Input className="report-edge-filter" value={edgeInstanceId} placeholder={t('reportSettings.planImport.edge')} onChange={(event) => setEdgeInstanceId(event.target.value)} />
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

      {draft ? (
        <>
          <section className="report-summary-grid">
            <Statistic title={t('reportSettings.planImport.summary.total')} value={draft.summary.total_rows} />
            <Statistic title={t('reportSettings.planImport.summary.ready')} value={draft.summary.ready_rows} />
            <Statistic title={t('reportSettings.planImport.summary.issues')} value={draft.summary.rows_with_issues} />
            <Statistic title={t('reportSettings.planImport.summary.confirm')} value={draft.summary.needs_confirmation} />
          </section>
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
              description={t('reportSettings.planImport.resultDesc', {
                standards: confirmMutation.data.created_standards,
                plans: confirmMutation.data.created_plans,
                status: confirmMutation.data.plan_creation_status,
              })}
            />
          ) : null}
          <section className="report-panel report-list-panel report-import-table">
            <Table<PlanImportRow>
              rowKey="row_number"
              size="small"
              columns={columns}
              dataSource={draft.rows}
              pagination={{ pageSize: 12, size: 'small' }}
              scroll={{ x: 1300 }}
            />
          </section>
        </>
      ) : (
        <section className="report-panel report-empty-panel">
          <Empty description={t('reportSettings.planImport.empty')} />
        </section>
      )}
    </div>
  )
}
