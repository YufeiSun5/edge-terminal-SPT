import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Descriptions, Empty, Input, InputNumber, Select, Space, Spin, Table, Tag, Timeline, Tooltip, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Download, FileSpreadsheet, RefreshCw, RotateCcw, Search, Send } from 'lucide-react'
import { saveAs } from 'file-saver'
import { useTranslation } from 'react-i18next'
import {
  downloadMainReportArtifact,
  enqueueMainReportJob,
  getDetectionRunReportRequests,
  getMainReportJob,
  getMainReportJobEvents,
  getMainReportJobs,
  getMainReportReadiness,
  getReportTemplates,
  retryMainReportJob,
} from '@/features/edge-status/api'
import { createLuckysheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
import { env } from '@/shared/config/env'
import type { DetectionRunReportRequest, MainReportJob, MainReportJobEvent, MainReportJobStatus, ReportTemplate } from '@/shared/api/types'
import './reports.css'

const reportJobStatuses = ['all', 'pending', 'waiting', 'running', 'success', 'failed'] as const

function statusColor(status: string) {
  if (status === 'success' || status === 'ready') return 'green'
  if (status === 'failed') return 'red'
  if (status === 'running') return 'blue'
  if (status === 'waiting') return 'gold'
  if (status === 'pending') return 'default'
  return 'default'
}

function compactDate(value?: string) {
  if (!value) return '-'
  return value.replace('T', ' ').replace(/\.\d+.*$/, '')
}

function safeJSONString(value: unknown) {
  if (value === undefined || value === null || value === '') return '-'
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function readinessValue(readiness: Record<string, unknown> | undefined, key: string) {
  if (!readiness) return '-'
  const value = readiness[key]
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? String(value) : '-'
}

function templateTitle(template: ReportTemplate) {
  return template.display_name || template.name || template.template_code
}

function templatePreviewWorkbook(template?: ReportTemplate) {
  const rows = [
    ['Template', template?.template_code ?? ''],
    ['Name', template ? templateTitle(template) : ''],
    ['Version', template?.version ? `v${template.version}` : ''],
    ['File ref', template?.file_ref ?? ''],
  ]

  return [
    {
      name: template ? templateTitle(template) : 'Report Template',
      index: '0',
      status: 1,
      order: 0,
      celldata: rows.flatMap((row, r) =>
        row.map((value, c) => ({
          r,
          c,
          v: {
            v: value,
            m: value,
            ct: { fa: 'General', t: 'g' },
            bl: c === 0 ? 1 : 0,
          },
        })),
      ),
      config: {
        columnlen: { 0: 120, 1: 360 },
      },
    },
  ]
}

function ReportTemplateLuckysheet({ template }: { template?: ReportTemplate }) {
  const adapterRef = useRef<ReturnType<typeof createLuckysheetAdapter> | null>(null)

  useEffect(() => {
    const adapter = createLuckysheetAdapter()
    adapterRef.current = adapter
    let disposed = false

    adapter
      .mount({
        containerId: 'report-luckysheet-preview',
        data: templatePreviewWorkbook(template),
        readonly: true,
        toolbar: true,
        sheetbar: true,
      })
      .catch((error) => {
        if (!disposed) console.error(error)
      })

    return () => {
      disposed = true
      adapter.unmount()
      adapterRef.current = null
    }
  }, [template])

  return (
    <section className="report-luckysheet-panel">
      <div className="report-subtitle">{template ? templateTitle(template) : 'Luckysheet'}</div>
      <div id="report-luckysheet-preview" className="report-luckysheet-host" />
    </section>
  )
}

export function ReportsPage() {
  const { t } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<(typeof reportJobStatuses)[number]>('all')
  const [taskIdFilter, setTaskIdFilter] = useState<number | null>(null)
  const [edgeFilter, setEdgeFilter] = useState('')
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null)
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null)
  const [enqueueTaskId, setEnqueueTaskId] = useState<number | null>(null)
  const [enqueueEdge, setEnqueueEdge] = useState('')
  const reportGenerationEnabled = env.runtimeFeatures.reportGeneration

  const templatesQuery = useQuery({
    queryKey: ['reports', 'templates'],
    queryFn: () => getReportTemplates({ enabled: true }),
  })
  const jobsQuery = useQuery({
    queryKey: ['reports', 'jobs', statusFilter, taskIdFilter, edgeFilter],
    queryFn: () =>
      getMainReportJobs({
        status: statusFilter === 'all' ? undefined : statusFilter,
        task_id: taskIdFilter ?? undefined,
        edge_instance_id: edgeFilter.trim() || undefined,
        limit: 100,
      }),
    enabled: reportGenerationEnabled,
  })

  const jobs = useMemo(() => jobsQuery.data?.items ?? [], [jobsQuery.data?.items])
  const templates = useMemo(() => templatesQuery.data ?? [], [templatesQuery.data])
  const selectedJob = useMemo(() => jobs.find((job) => job.id === selectedJobId) ?? jobs[0], [jobs, selectedJobId])
  const selectedTemplate = useMemo(
    () => templates.find((template) => template.id === selectedTemplateId) ?? templates[0],
    [selectedTemplateId, templates],
  )

  const jobDetailQuery = useQuery({
    queryKey: ['reports', 'job', selectedJob?.id],
    queryFn: () => getMainReportJob(selectedJob!.id),
    enabled: reportGenerationEnabled && Boolean(selectedJob?.id),
  })
  const activeJob = jobDetailQuery.data ?? selectedJob
  const readinessQuery = useQuery({
    queryKey: ['reports', 'readiness', activeJob?.task_id, activeJob?.edge_instance_id],
    queryFn: () => getMainReportReadiness(activeJob!.task_id, activeJob?.edge_instance_id),
    enabled: reportGenerationEnabled && Boolean(activeJob?.task_id),
  })
  const eventsQuery = useQuery({
    queryKey: ['reports', 'events', activeJob?.id],
    queryFn: () => getMainReportJobEvents(activeJob!.id, 100),
    enabled: reportGenerationEnabled && Boolean(activeJob?.id),
  })
  const requestsQuery = useQuery({
    queryKey: ['reports', 'requests', activeJob?.task_id],
    queryFn: () => getDetectionRunReportRequests(activeJob!.task_id),
    enabled: reportGenerationEnabled && Boolean(activeJob?.task_id),
  })

  const refreshReports = () => {
    void queryClient.invalidateQueries({ queryKey: ['reports'] })
  }

  const enqueueMutation = useMutation({
    mutationFn: () =>
      enqueueMainReportJob({
        task_id: enqueueTaskId ?? 0,
        edge_instance_id: enqueueEdge.trim() || undefined,
      }),
    onSuccess: (result) => {
      messageApi.success(t('reports.actions.enqueueSuccess', { count: result.jobs.length }))
      setSelectedJobId(result.jobs[0]?.id ?? null)
      refreshReports()
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('reports.actions.enqueueFailed'))
    },
  })

  const retryMutation = useMutation({
    mutationFn: (jobId: number) => retryMainReportJob(jobId),
    onSuccess: (job) => {
      messageApi.success(t('reports.actions.retrySuccess'))
      setSelectedJobId(job.id)
      refreshReports()
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('reports.actions.retryFailed'))
    },
  })

  const downloadMutation = useMutation({
    mutationFn: (jobId: number) => downloadMainReportArtifact(jobId),
    onSuccess: (artifact) => {
      saveAs(artifact.blob, artifact.filename)
      messageApi.success(t('reports.actions.downloadSuccess'))
    },
    onError: (error) => {
      messageApi.error(error instanceof Error ? error.message : t('reports.actions.downloadFailed'))
    },
  })

  const templateColumns: ColumnsType<ReportTemplate> = [
    {
      title: t('reports.columns.template'),
      dataIndex: 'template_code',
      render: (_: string, record) => (
          <Space orientation="vertical" size={1}>
          <Typography.Text strong>{templateTitle(record)}</Typography.Text>
          <Typography.Text type="secondary">{record.template_code}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('reports.columns.version'),
      dataIndex: 'version',
      width: 86,
      render: (value: number) => <Tag>v{value}</Tag>,
    },
    {
      title: t('reports.columns.fileRef'),
      dataIndex: 'file_ref',
      ellipsis: true,
    },
  ]

  const jobColumns: ColumnsType<MainReportJob> = [
    {
      title: t('reports.columns.job'),
      dataIndex: 'id',
      width: 110,
      render: (value: number, record) => (
        <Button type="link" className="report-link-button" onClick={() => setSelectedJobId(value)}>
          #{value}
          <br />
          {record.test_no || `task-${record.task_id}`}
        </Button>
      ),
    },
    {
      title: t('reports.columns.status'),
      dataIndex: 'status',
      width: 110,
      render: (value: MainReportJobStatus) => <Tag color={statusColor(value)}>{value}</Tag>,
    },
    {
      title: t('reports.columns.reportName'),
      dataIndex: 'report_name',
      ellipsis: true,
      render: (value: string, record) => value || record.template_code || '-',
    },
    {
      title: t('reports.columns.project'),
      dataIndex: 'project_code',
      width: 130,
      ellipsis: true,
    },
    {
      title: t('reports.columns.updatedAt'),
      dataIndex: 'updated_at',
      width: 170,
      render: compactDate,
    },
  ]

  const requestColumns: ColumnsType<DetectionRunReportRequest> = [
    {
      title: t('reports.columns.request'),
      dataIndex: 'id',
      width: 90,
      render: (value: number) => `#${value}`,
    },
    {
      title: t('reports.columns.variable'),
      dataIndex: 'var_name',
      ellipsis: true,
      render: (_: string, record) => record.display_name || record.var_name || record.var_id_text || record.var_id,
    },
    {
      title: t('reports.columns.reportName'),
      dataIndex: 'report_name',
      ellipsis: true,
    },
    {
      title: t('reports.columns.status'),
      dataIndex: 'status',
      width: 110,
      render: (value: string) => <Tag color={statusColor(value)}>{value || '-'}</Tag>,
    },
  ]

  const eventItems = (eventsQuery.data?.items ?? []).map((event: MainReportJobEvent) => ({
    color: event.level === 'error' ? 'red' : event.level === 'warning' ? 'gold' : 'blue',
    content: (
      <Space orientation="vertical" size={2}>
        <Space wrap>
          <Tag color={statusColor(event.event_type)}>{event.event_type}</Tag>
          <Typography.Text strong>{event.message}</Typography.Text>
        </Space>
        <Typography.Text type="secondary">{compactDate(event.created_at)}</Typography.Text>
        {event.payload ? <pre className="report-json">{safeJSONString(event.payload)}</pre> : null}
      </Space>
    ),
  }))

  return (
    <div className="reports-page">
      {contextHolder}
      <header className="report-toolbar">
        <div>
          <span className="report-eyebrow">{t('reports.eyebrow')}</span>
          <h1>{t('reports.title')}</h1>
          <p>{t('reports.subtitle')}</p>
        </div>
        {reportGenerationEnabled ? (
          <Space wrap>
            <Select
              className="report-status-select"
              value={statusFilter}
              onChange={setStatusFilter}
              options={reportJobStatuses.map((status) => ({ value: status, label: t(`reports.status.${status}`) }))}
            />
            <InputNumber
              min={1}
              value={taskIdFilter}
              placeholder={t('reports.filters.taskId')}
              onChange={setTaskIdFilter}
            />
            <Input
              className="report-edge-filter"
              value={edgeFilter}
              placeholder={t('reports.filters.edge')}
              prefix={<Search size={14} />}
              onChange={(event) => setEdgeFilter(event.target.value)}
            />
            <Button icon={<RefreshCw size={14} />} onClick={refreshReports} loading={jobsQuery.isFetching || templatesQuery.isFetching}>
              {t('reports.actions.refresh')}
            </Button>
          </Space>
        ) : (
          <Button icon={<RefreshCw size={14} />} onClick={refreshReports} loading={templatesQuery.isFetching}>
            {t('reports.actions.refresh')}
          </Button>
        )}
      </header>

      {reportGenerationEnabled ? (
        <section className="report-enqueue-bar">
          <Space wrap>
            <Typography.Text strong>{t('reports.enqueue.title')}</Typography.Text>
            <InputNumber min={1} value={enqueueTaskId} placeholder={t('reports.enqueue.taskId')} onChange={setEnqueueTaskId} />
            <Input className="report-edge-filter" value={enqueueEdge} placeholder={t('reports.enqueue.edge')} onChange={(event) => setEnqueueEdge(event.target.value)} />
            <Tooltip title={enqueueTaskId ? undefined : t('reports.enqueue.taskRequired')}>
              <Button type="primary" icon={<Send size={14} />} disabled={!enqueueTaskId} loading={enqueueMutation.isPending} onClick={() => enqueueMutation.mutate()}>
                {t('reports.actions.enqueue')}
              </Button>
            </Tooltip>
          </Space>
        </section>
      ) : (
        <Alert className="report-alert" showIcon type="info" message={t('reports.edgeModeNotice')} />
      )}

      <main className={reportGenerationEnabled ? 'report-layout' : 'report-layout templates-only'}>
        {reportGenerationEnabled ? <section className="report-panel report-list-panel">
          <div className="report-panel-header">
            <Space>
              <FileSpreadsheet size={16} />
              <strong>{t('reports.jobs.title')}</strong>
            </Space>
            <Tag>{jobsQuery.data?.total ?? jobs.length}</Tag>
          </div>
          <Table<MainReportJob>
            rowKey="id"
            size="small"
            columns={jobColumns}
            dataSource={jobs}
            loading={jobsQuery.isLoading}
            pagination={{ pageSize: 12, size: 'small' }}
            rowClassName={(record) => (record.id === activeJob?.id ? 'report-row-selected' : '')}
            onRow={(record) => ({ onClick: () => setSelectedJobId(record.id) })}
            locale={{ emptyText: <Empty description={t('reports.jobs.empty')} /> }}
          />
        </section> : null}

        <section className="report-panel report-detail-panel">
          {!activeJob ? (
            <div className="report-template-workspace">
              <section className="report-template-list-panel">
                <div className="report-panel-header">
                  <Space>
                    <FileSpreadsheet size={16} />
                    <strong>{t('reports.templates.title')}</strong>
                  </Space>
                  <Tag>{templates.length}</Tag>
                </div>
                <div className="report-template-list">
                  {templatesQuery.isLoading ? (
                    <Spin />
                  ) : templates.length ? (
                    templates.map((template) => (
                      <button
                        className={template.id === selectedTemplate?.id ? 'report-template-item active' : 'report-template-item'}
                        key={template.id}
                        onClick={() => setSelectedTemplateId(template.id)}
                      >
                        <span>
                          <strong>{templateTitle(template)}</strong>
                          <small>{template.template_code}</small>
                          <em>{template.file_ref}</em>
                        </span>
                        <Tag>v{template.version}</Tag>
                      </button>
                    ))
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('reports.templates.empty')} />
                  )}
                </div>
              </section>
              <ReportTemplateLuckysheet template={selectedTemplate} />
            </div>
          ) : (
            <>
              <div className="report-panel-header">
                <Space wrap>
                  <Tag color={statusColor(activeJob.status)}>{activeJob.status}</Tag>
                  <Typography.Title level={3}>#{activeJob.id} {activeJob.report_name || activeJob.template_code || activeJob.test_no}</Typography.Title>
                </Space>
                <Space wrap>
                  <Button
                    icon={<RotateCcw size={14} />}
                    disabled={!['failed', 'waiting'].includes(activeJob.status)}
                    loading={retryMutation.isPending}
                    onClick={() => retryMutation.mutate(activeJob.id)}
                  >
                    {t('reports.actions.retry')}
                  </Button>
                  <Button
                    type="primary"
                    icon={<Download size={14} />}
                    disabled={activeJob.status !== 'success'}
                    loading={downloadMutation.isPending}
                    onClick={() => downloadMutation.mutate(activeJob.id)}
                  >
                    {t('reports.actions.download')}
                  </Button>
                </Space>
              </div>

              {activeJob.error_message ? <Alert type="error" showIcon message={activeJob.error_message} className="report-alert" /> : null}

              <Descriptions size="small" column={3} bordered>
                <Descriptions.Item label={t('reports.fields.task')}>{activeJob.task_id}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.request')}>{activeJob.request_id}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.edge')}>{activeJob.edge_instance_id}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.project')}>{activeJob.project_code || activeJob.project_id}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.template')}>{activeJob.template_code || '-'}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.artifact')}>{activeJob.artifact_name || '-'}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.readiness')}>{activeJob.readiness_status || '-'}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.attempts')}>{activeJob.attempts}/{activeJob.max_attempts}</Descriptions.Item>
                <Descriptions.Item label={t('reports.fields.finishedAt')}>{compactDate(activeJob.finished_at)}</Descriptions.Item>
              </Descriptions>

              <div className="report-grid">
                <section>
                  <div className="report-subtitle">{t('reports.readiness.title')}</div>
                  {readinessQuery.isError ? (
                    <Alert type="warning" showIcon message={readinessQuery.error instanceof Error ? readinessQuery.error.message : t('reports.readiness.failed')} />
                  ) : (
                    <div className="report-readiness-summary">
                      <Tag color={statusColor(readinessValue(readinessQuery.data?.readiness, 'overall_status'))}>
                        {readinessValue(readinessQuery.data?.readiness, 'overall_status')}
                      </Tag>
                      <span>{t('reports.readiness.syncDatabase')}: {readinessQuery.data?.sync_database ?? '-'}</span>
                      <pre className="report-json">{safeJSONString(readinessQuery.data?.readiness)}</pre>
                    </div>
                  )}
                </section>

                <section>
                  <div className="report-subtitle">{t('reports.events.title')}</div>
                  {eventItems.length ? <Timeline items={eventItems} /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('reports.events.empty')} />}
                </section>
              </div>

              <div className="report-grid">
                <section>
                  <div className="report-subtitle">{t('reports.requests.title')}</div>
                  <Table<DetectionRunReportRequest>
                    rowKey="id"
                    size="small"
                    columns={requestColumns}
                    dataSource={requestsQuery.data?.items ?? []}
                    loading={requestsQuery.isLoading}
                    pagination={false}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('reports.requests.empty')} /> }}
                  />
                </section>

                <section>
                  <div className="report-subtitle">{t('reports.templates.title')}</div>
                  <Table<ReportTemplate>
                    rowKey="id"
                    size="small"
                    columns={templateColumns}
                    dataSource={templates}
                    loading={templatesQuery.isLoading}
                    pagination={false}
                    rowClassName={(record) => (record.id === selectedTemplate?.id ? 'report-row-selected' : '')}
                    onRow={(record) => ({ onClick: () => setSelectedTemplateId(record.id) })}
                    locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('reports.templates.empty')} /> }}
                  />
                </section>
              </div>

              <ReportTemplateLuckysheet template={selectedTemplate} />
            </>
          )}
        </section>
      </main>
    </div>
  )
}
