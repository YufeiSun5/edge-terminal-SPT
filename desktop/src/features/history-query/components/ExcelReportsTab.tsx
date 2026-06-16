import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Descriptions, Empty, Space, Spin, Table, Tag, Timeline, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Download, FileSpreadsheet, RefreshCw } from 'lucide-react'
import { saveAs } from 'file-saver'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  downloadMainReportArtifact,
  getDetectionRun,
  getDetectionRunReportRequests,
  getMainReportJobEvents,
  getMainReportJobs,
  retryMainReportJob,
} from '@/features/edge-status/api'
import { env } from '@/shared/config/env'
import type {
  DetectionRunReport,
  DetectionRunReportRequest,
  DetectionRunReportRequestVariable,
  MainReportJob,
  MainReportJobEvent,
} from '@/shared/api/types'

type ReportListItem = {
  key: string
  name: string
  templateCode: string
  templateVersion?: number
  status: string
  updatedAt?: string
  request?: DetectionRunReportRequest
  report?: DetectionRunReport
  job?: MainReportJob
}

function compactDate(value?: string) {
  if (!value) return '-'
  return value.replace('T', ' ').replace(/\.\d+.*$/, '')
}

function statusColor(status: string) {
  if (status === 'success' || status === 'ready' || status === 'generated') return 'green'
  if (status === 'running' || status === 'generating') return 'blue'
  if (status === 'waiting') return 'gold'
  if (status === 'failed' || status === 'error') return 'red'
  return 'default'
}

function statusLabel(status: string, t: TFunction) {
  const labels: Record<string, string> = {
    pending: t('history.detail.reports.status.pending'),
    waiting: t('history.detail.reports.status.waiting'),
    running: t('history.detail.reports.status.running'),
    generating: t('history.detail.reports.status.running'),
    success: t('history.detail.reports.status.success'),
    ready: t('history.detail.reports.status.ready'),
    generated: t('history.detail.reports.status.success'),
    failed: t('history.detail.reports.status.failed'),
    error: t('history.detail.reports.status.failed'),
  }
  return labels[status] ?? (status || t('history.detail.reports.status.unknown'))
}

function requestVariables(request?: DetectionRunReportRequest): DetectionRunReportRequestVariable[] {
  if (!request) return []
  if (request.variables?.length) return request.variables
  if (!request.var_id && !request.var_name) return []
  return [
    {
      var_id: request.var_id,
      var_name: request.var_name,
      report_name: request.report_name,
      ext_1: request.ext_1,
      ext_2: request.ext_2,
      ext_3: request.ext_3,
    },
  ]
}

function createReportItems(
  requests: DetectionRunReportRequest[],
  reports: DetectionRunReport[],
  jobs: MainReportJob[],
  t: TFunction,
): ReportListItem[] {
  const jobByRequestId = new Map(jobs.map((job) => [job.request_id, job]))
  const items: ReportListItem[] = requests.slice(0, 7).map((request) => {
    const job = jobByRequestId.get(request.id)
    return {
      key: `request-${request.id}`,
      name: job?.report_name || request.report_name || request.display_name || t('history.detail.reports.requestFallback', { id: request.id }),
      templateCode: job?.template_code || request.report_name || request.display_name || '-',
      templateVersion: job?.template_version,
      status: job?.status || request.status || 'pending',
      updatedAt: job?.updated_at || request.updated_at,
      request,
      job,
    }
  })

  const remaining = Math.max(0, 7 - items.length)
  const requestNames = new Set(items.map((item) => item.name))
  const fileItems: ReportListItem[] = reports
    .filter((report) => !requestNames.has(report.file_name || report.template_code))
    .slice(0, remaining)
    .map((report) => ({
      key: `report-${report.id}`,
      name: report.file_name || report.template_code || t('history.detail.reports.reportFallback', { id: report.id }),
      templateCode: report.template_code || '-',
      templateVersion: report.template_version,
      status: report.status || 'ready',
      updatedAt: report.generated_at || report.updated_at,
      report,
    }))

  return [...items, ...fileItems]
}

export function ExcelReportsTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedReportKey, setSelectedReportKey] = useState<string | null>(null)
  const reportGenerationEnabled = env.runtimeFeatures.reportGeneration
  const isMainServer = env.runtimeRole === 'main_server'

  const runQuery = useQuery({
    queryKey: ['history', 'run', taskId],
    queryFn: () => getDetectionRun(taskId),
    enabled: isMainServer,
    retry: false,
  })

  const requestsQuery = useQuery({
    queryKey: ['history', 'run', 'report-requests', taskId],
    queryFn: () => getDetectionRunReportRequests(taskId),
    enabled: isMainServer,
    retry: false,
  })

  const jobsQuery = useQuery({
    queryKey: ['history', 'run', 'report-jobs', taskId],
    queryFn: () => getMainReportJobs({ task_id: taskId, limit: 20 }),
    enabled: isMainServer && reportGenerationEnabled,
    retry: false,
  })

  const reportItems = useMemo(
    () =>
      createReportItems(
        requestsQuery.data?.items ?? runQuery.data?.report_requests ?? [],
        runQuery.data?.reports ?? [],
        jobsQuery.data?.items ?? [],
        t,
      ),
    [jobsQuery.data?.items, requestsQuery.data?.items, runQuery.data?.report_requests, runQuery.data?.reports, t],
  )

  const selectedReport = useMemo(
    () => reportItems.find((item) => item.key === selectedReportKey) ?? reportItems[0],
    [reportItems, selectedReportKey],
  )

  const eventsQuery = useQuery({
    queryKey: ['history', 'run', 'report-job-events', selectedReport?.job?.id],
    queryFn: () => getMainReportJobEvents(selectedReport!.job!.id, 50),
    enabled: isMainServer && reportGenerationEnabled && Boolean(selectedReport?.job?.id),
    retry: false,
  })

  const refreshReports = () => {
    void queryClient.invalidateQueries({ queryKey: ['history', 'run', taskId] })
    void queryClient.invalidateQueries({ queryKey: ['history', 'run', 'report-requests', taskId] })
    void queryClient.invalidateQueries({ queryKey: ['history', 'run', 'report-jobs', taskId] })
  }

  const retryMutation = useMutation({
    mutationFn: (jobId: number) => retryMainReportJob(jobId),
    onSuccess: () => {
      message.success(t('history.detail.reports.retrySubmitted'))
      refreshReports()
    },
    onError: (error) => {
      message.error(error instanceof Error ? error.message : t('history.detail.reports.retryFailed'))
    },
  })

  const downloadMutation = useMutation({
    mutationFn: (jobId: number) => downloadMainReportArtifact(jobId),
    onSuccess: (artifact) => {
      saveAs(artifact.blob, artifact.filename)
      message.success(t('history.detail.reports.downloadStarted'))
    },
    onError: (error) => {
      message.error(error instanceof Error ? error.message : t('history.detail.reports.downloadFailed'))
    },
  })

  const variableColumns: ColumnsType<DetectionRunReportRequestVariable> = [
    {
      title: t('history.detail.reports.variable'),
      dataIndex: 'var_name',
      render: (value: string | undefined, record) => value || record.var_id || '-',
    },
    {
      title: t('history.detail.reports.reportName'),
      dataIndex: 'report_name',
      render: (value: string | undefined) => value || '-',
    },
    {
      title: t('history.detail.reports.ext1'),
      dataIndex: 'ext_1',
      render: (value: string | undefined) => value || '-',
    },
    {
      title: t('history.detail.reports.ext2'),
      dataIndex: 'ext_2',
      render: (value: string | undefined) => value || '-',
    },
  ]

  const eventItems = (eventsQuery.data?.items ?? []).map((event: MainReportJobEvent) => ({
    content: (
      <Space orientation="vertical" size={0}>
        <Typography.Text>{event.message}</Typography.Text>
        <Typography.Text type="secondary">
          {compactDate(event.created_at)} · {event.event_type}
        </Typography.Text>
      </Space>
    ),
    color: event.level === 'error' ? 'red' : event.level === 'warn' ? 'orange' : 'blue',
  }))

  const loading = runQuery.isFetching || requestsQuery.isFetching || jobsQuery.isFetching
  const selectedVariables = requestVariables(selectedReport?.request)
  const canDownload = selectedReport?.job?.status === 'success'

  if (!isMainServer) {
    return (
      <div className="history-tab-content history-remote-only-tab">
        <Alert
          type="info"
          showIcon
          title={t('history.detail.reports.remoteTitle')}
          description={t('history.detail.reports.remoteDescription')}
        />
        <Empty description={t('history.detail.reports.remoteEmpty')} />
      </div>
    )
  }

  return (
    <div className="history-tab-content history-split-layout">
      <div className="history-split-left">
        <Typography.Title level={5} className="history-split-title">
          {t('history.detail.reports.title', { count: reportItems.length })}
        </Typography.Title>
        {loading && reportItems.length === 0 ? (
          <Spin className="history-inline-spin" />
        ) : (
          <div className="history-report-list">
            {reportItems.length > 0 ? (
              reportItems.map((item) => (
                <button
                  type="button"
                  key={item.key}
                  className={`history-report-item ${item.key === selectedReport?.key ? 'active' : ''}`}
                  onClick={() => setSelectedReportKey(item.key)}
                >
                  <FileSpreadsheet size={16} className="history-muted-icon" />
                  <div className="history-report-item-info">
                    <span className="history-report-name" title={item.name}>{item.name}</span>
                    <span className="history-report-meta">
                      {item.templateCode}
                      {item.templateVersion ? ` v${item.templateVersion}` : ''}
                    </span>
                  </div>
                  <Tag color={statusColor(item.status)}>{statusLabel(item.status, t)}</Tag>
                </button>
              ))
            ) : (
              <Empty description={t('history.detail.reports.emptyRequests')} />
            )}
          </div>
        )}
      </div>

      <div className="history-split-right">
        {!selectedReport ? (
          <Empty description={t('history.detail.reports.selectReport')} />
        ) : (
          <div className="history-report-preview">
            <div className="history-report-header">
              <FileSpreadsheet size={40} className="history-report-large-icon" />
              <Typography.Title level={4}>{selectedReport.name}</Typography.Title>
              <div className="history-report-tags">
                <Tag color="blue">{selectedReport.templateCode}</Tag>
                {selectedReport.templateVersion ? <Tag>v{selectedReport.templateVersion}</Tag> : null}
                <Tag color={statusColor(selectedReport.status)}>{statusLabel(selectedReport.status, t)}</Tag>
              </div>
              <p className="history-report-time">{t('history.detail.reports.lastUpdated')}: {compactDate(selectedReport.updatedAt)}</p>
            </div>

            <div className="history-report-actions">
              <Button icon={<RefreshCw size={16} />} onClick={refreshReports} loading={loading}>
                {t('actions.refresh')}
              </Button>
              {selectedReport.job?.status === 'failed' ? (
                <Button
                  danger
                  icon={<RefreshCw size={16} />}
                  loading={retryMutation.isPending}
                  onClick={() => retryMutation.mutate(selectedReport.job!.id)}
                >
                  {t('history.detail.reports.retryGenerate')}
                </Button>
              ) : null}
              <Button
                type="primary"
                icon={<Download size={16} />}
                disabled={!canDownload}
                loading={downloadMutation.isPending}
                onClick={() => selectedReport.job && downloadMutation.mutate(selectedReport.job.id)}
              >
                {t('history.detail.reports.downloadCurrent')}
              </Button>
            </div>

            <div className="history-report-detail">
              <Descriptions
                size="small"
                bordered
                column={2}
                items={[
                  { label: t('history.detail.reports.taskId'), children: taskId },
                  { label: t('history.detail.reports.testNo'), children: selectedReport.request?.test_no || selectedReport.job?.test_no || runQuery.data?.test_no || '-' },
                  { label: t('history.detail.reports.project'), children: selectedReport.request?.project_code || selectedReport.job?.project_code || runQuery.data?.project_code || '-' },
                  { label: t('history.detail.reports.requestId'), children: selectedReport.request?.id ?? '-' },
                  { label: 'Job ID', children: selectedReport.job?.id ?? '-' },
                  { label: t('history.detail.reports.artifact'), children: selectedReport.job?.artifact_name || selectedReport.report?.file_name || '-' },
                ]}
              />

              <Typography.Title level={5}>{t('history.detail.reports.variables')}</Typography.Title>
              <Table<DetectionRunReportRequestVariable>
                size="small"
                rowKey={(record, index) => `${record.var_id ?? record.var_name ?? 'var'}-${index}`}
                pagination={false}
                columns={variableColumns}
                dataSource={selectedVariables}
                locale={{ emptyText: t('history.detail.reports.emptyVariables') }}
              />

              {selectedReport.request?.params ? (
                <>
                  <Typography.Title level={5}>{t('history.detail.reports.requestParams')}</Typography.Title>
                  <pre className="history-report-json">{JSON.stringify(selectedReport.request.params, null, 2)}</pre>
                </>
              ) : null}

              {reportGenerationEnabled && selectedReport.job ? (
                <>
                  <Typography.Title level={5}>{t('history.detail.reports.generationEvents')}</Typography.Title>
                  {eventsQuery.isFetching ? (
                    <Spin />
                  ) : eventItems.length > 0 ? (
                    <Timeline items={eventItems} />
                  ) : (
                    <Empty description={t('history.detail.reports.emptyEvents')} />
                  )}
                </>
              ) : null}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
