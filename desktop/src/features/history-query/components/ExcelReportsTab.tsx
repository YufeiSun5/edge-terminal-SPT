import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Empty, Input, Modal, Select, Spin, Tag, Typography, message } from 'antd'
import { FileSpreadsheet, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  downloadMainReportArtifact,
  getDetectionRun,
  getDetectionRunReportRequests,
  getMainReportJobEvents,
  getMainReportJobs,
  regenerateMainReportJob,
  retryMainReportJob,
} from '@/features/edge-status/api'
import { createLuckysheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
import { env } from '@/shared/config/env'
import type {
  DetectionRunReport,
  DetectionRunReportRequest,
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
  if (status === 'success' || status === 'succeeded' || status === 'ready' || status === 'generated') return 'green'
  if (status === 'running' || status === 'generating') return 'blue'
  if (status === 'waiting' || status === 'waiting_for_sync') return 'gold'
  if (status === 'failed' || status === 'error') return 'red'
  return 'default'
}

function statusLabel(status: string, t: TFunction) {
  const labels: Record<string, string> = {
    pending: t('history.detail.reports.status.pending'),
    waiting: t('history.detail.reports.status.waiting'),
    waiting_for_sync: t('history.detail.reports.status.waiting'),
    running: t('history.detail.reports.status.running'),
    generating: t('history.detail.reports.status.running'),
    success: t('history.detail.reports.status.success'),
    succeeded: t('history.detail.reports.status.success'),
    ready: t('history.detail.reports.status.ready'),
    generated: t('history.detail.reports.status.success'),
    failed: t('history.detail.reports.status.failed'),
    error: t('history.detail.reports.status.failed'),
  }
  return labels[status] ?? (status || t('history.detail.reports.status.unknown'))
}

function isSucceeded(status?: string) {
  return status === 'success' || status === 'succeeded' || status === 'ready' || status === 'generated'
}

function createReportItems(
  requests: DetectionRunReportRequest[],
  reports: DetectionRunReport[],
  jobs: MainReportJob[],
  t: TFunction,
): ReportListItem[] {
  const jobByRequestId = new Map<number, MainReportJob>()
  for (const job of jobs) {
    if (!jobByRequestId.has(job.request_id)) {
      jobByRequestId.set(job.request_id, job)
    }
  }
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

function prettyJSONString(value: unknown) {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
  return JSON.stringify(value ?? {}, null, 2)
}

function editableParamsJSON(report?: ReportListItem) {
  if (report?.job?.params_override_json) {
    return prettyJSONString(report.job.params_override_json)
  }
  return prettyJSONString(report?.request?.params ?? {})
}

function ReportArtifactPreview({ job }: { job?: MainReportJob }) {
  const { t } = useTranslation()
  const adapterRef = useRef<ReturnType<typeof createLuckysheetAdapter> | null>(null)
  const containerId = `history-report-preview-${useId().replace(/:/g, '')}`
  const [previewError, setPreviewError] = useState<string | null>(null)
  const artifactQuery = useQuery({
    queryKey: ['history', 'run', 'report-artifact-preview', job?.id],
    queryFn: () => downloadMainReportArtifact(job!.id),
    enabled: Boolean(job?.id) && isSucceeded(job?.status),
    retry: false,
  })

  useEffect(() => {
    const adapter = createLuckysheetAdapter()
    adapterRef.current = adapter
    let disposed = false

    async function waitForContainer() {
      for (let attempt = 0; attempt < 10; attempt += 1) {
        if (disposed) return false
        if (document.getElementById(containerId)) return true
        await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()))
      }
      return Boolean(document.getElementById(containerId))
    }

    async function mountPreview() {
      setPreviewError(null)
      const hasContainer = await waitForContainer()
      if (disposed) return
      if (!hasContainer) {
        setPreviewError(t('history.detail.reports.previewFailed'))
        return
      }

      await adapter.mount({
        containerId,
        data: [{ name: job?.report_name || job?.template_code || 'Report', index: '0', status: 1, order: 0 }],
        readonly: true,
        toolbar: false,
        sheetbar: true,
      })

      if (disposed || !artifactQuery.data || !job) return
      const file = new File([artifactQuery.data.blob], artifactQuery.data.filename, { type: artifactQuery.data.contentType })
      await adapter.importFile(file)
    }

    void mountPreview()
      .catch((error) => {
        if (disposed) return
        setPreviewError(error instanceof Error ? error.message : t('history.detail.reports.previewFailed'))
      })

    return () => {
      disposed = true
      adapter.unmount()
      adapterRef.current = null
    }
  }, [artifactQuery.data, containerId, job, t])

  if (!job) {
    return <Empty description={t('history.detail.reports.noGeneratedArtifact')} />
  }

  if (!isSucceeded(job.status)) {
    return <Empty description={t('history.detail.reports.waitingForGeneratedArtifact')} />
  }

  return (
    <div className="history-report-luckysheet">
      {artifactQuery.isError || previewError ? (
        <Alert
          type="warning"
          showIcon
          message={artifactQuery.error instanceof Error ? artifactQuery.error.message : previewError || t('history.detail.reports.previewFailed')}
        />
      ) : null}
      <div id={containerId} className="report-luckysheet-host" />
    </div>
  )
}

export function ExcelReportsTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedReportKey, setSelectedReportKey] = useState<string | null>(null)
  const [paramsModalOpen, setParamsModalOpen] = useState(false)
  const [paramsDraft, setParamsDraft] = useState('{}')
  const [paramsReason, setParamsReason] = useState('')
  const [selectedGenerationJobId, setSelectedGenerationJobId] = useState<number | null>(null)
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

  const selectedReportGenerations = useMemo(() => {
    const requestId = selectedReport?.request?.id
    if (!requestId) return []
    return (jobsQuery.data?.items ?? []).filter((job) => job.request_id === requestId)
  }, [jobsQuery.data?.items, selectedReport?.request?.id])

  const selectedGenerationJob = useMemo(() => {
    if (selectedGenerationJobId) {
      const matched = selectedReportGenerations.find((job) => job.id === selectedGenerationJobId)
      if (matched) return matched
    }
    return selectedReport?.job
  }, [selectedGenerationJobId, selectedReport?.job, selectedReportGenerations])

  const selectedReportWithGeneration = useMemo<ReportListItem | undefined>(() => {
    if (!selectedReport) return undefined
    if (!selectedGenerationJob || selectedGenerationJob.id === selectedReport.job?.id) return selectedReport
    return {
      ...selectedReport,
      job: selectedGenerationJob,
      status: selectedGenerationJob.status || selectedReport.status,
      updatedAt: selectedGenerationJob.updated_at || selectedReport.updatedAt,
      templateCode: selectedGenerationJob.template_code || selectedReport.templateCode,
      templateVersion: selectedGenerationJob.template_version || selectedReport.templateVersion,
    }
  }, [selectedGenerationJob, selectedReport])

  const eventsQuery = useQuery({
    queryKey: ['history', 'run', 'report-job-events', selectedGenerationJob?.id],
    queryFn: () => getMainReportJobEvents(selectedGenerationJob!.id, 50),
    enabled: isMainServer && reportGenerationEnabled && Boolean(selectedGenerationJob?.id),
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

  const regenerateMutation = useMutation({
    mutationFn: ({ jobId, params, reason }: { jobId: number; params: Record<string, unknown>; reason: string }) =>
      regenerateMainReportJob(jobId, { params, reason }),
    onSuccess: () => {
      message.success(t('history.detail.reports.regenerateSubmitted'))
      setParamsModalOpen(false)
      refreshReports()
    },
    onError: (error) => {
      message.error(error instanceof Error ? error.message : t('history.detail.reports.regenerateFailed'))
    },
  })

  const openParamsModal = () => {
    setParamsDraft(editableParamsJSON(selectedReportWithGeneration))
    setParamsReason('')
    setParamsModalOpen(true)
  }

  const submitRegenerate = () => {
    if (!selectedGenerationJob?.id) return
    let params: Record<string, unknown>
    try {
      const parsed = JSON.parse(paramsDraft)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        message.error(t('history.detail.reports.paramsMustBeObject'))
        return
      }
      params = parsed as Record<string, unknown>
    } catch {
      message.error(t('history.detail.reports.invalidParamsJson'))
      return
    }
    regenerateMutation.mutate({ jobId: selectedGenerationJob.id, params, reason: paramsReason })
  }

  const latestEvent = (eventsQuery.data?.items ?? [])[0] as MainReportJobEvent | undefined

  const loading = runQuery.isFetching || requestsQuery.isFetching || jobsQuery.isFetching

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
                  onClick={() => {
                    setSelectedReportKey(item.key)
                    setSelectedGenerationJobId(null)
                  }}
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
        {!selectedReportWithGeneration ? (
          <Empty description={t('history.detail.reports.selectReport')} />
        ) : (
          <div className="history-report-preview">
            <div className="history-report-topbar">
              <div className="history-report-summary">
                <FileSpreadsheet size={18} className="history-muted-icon" />
                <div className="history-report-summary-main">
                  <div className="history-report-summary-title">
                    <Typography.Text strong ellipsis>{selectedReportWithGeneration.name}</Typography.Text>
                    <Tag color={statusColor(selectedReportWithGeneration.status)}>{statusLabel(selectedReportWithGeneration.status, t)}</Tag>
                  </div>
                  <div className="history-report-summary-meta">
                    <span>{selectedReportWithGeneration.templateCode}{selectedReportWithGeneration.templateVersion ? ` v${selectedReportWithGeneration.templateVersion}` : ''}</span>
                    <span>{t('history.detail.reports.lastUpdated')}: {compactDate(selectedReportWithGeneration.updatedAt)}</span>
                    {latestEvent ? <span>{latestEvent.event_type}: {compactDate(latestEvent.created_at)}</span> : null}
                  </div>
                </div>
              </div>
              <div className="history-report-actions">
                {selectedReportGenerations.length > 1 ? (
                  <Select
                    size="small"
                    className="history-report-generation-select"
                    value={selectedGenerationJob?.id}
                    aria-label={t('history.detail.reports.generationHistory')}
                    options={selectedReportGenerations.map((job, index) => ({
                      value: job.id,
                      label: `${index === 0 ? t('history.detail.reports.latestGeneration') : t('history.detail.reports.generation')} #${job.id} · ${statusLabel(job.status, t)} · ${compactDate(job.created_at)}`,
                    }))}
                    onChange={(value) => setSelectedGenerationJobId(value)}
                  />
                ) : null}
                <Button size="small" icon={<RefreshCw size={14} />} onClick={refreshReports} loading={loading}>
                  {t('actions.refresh')}
                </Button>
                {selectedGenerationJob?.status === 'failed' ? (
                  <Button
                    size="small"
                    danger
                    icon={<RefreshCw size={14} />}
                    loading={retryMutation.isPending}
                    onClick={() => retryMutation.mutate(selectedGenerationJob.id)}
                  >
                    {t('history.detail.reports.retryGenerate')}
                  </Button>
                ) : null}
                {selectedGenerationJob ? (
                  <Button
                    size="small"
                    icon={<RefreshCw size={14} />}
                    loading={regenerateMutation.isPending}
                    onClick={openParamsModal}
                  >
                    {t('history.detail.reports.regenerateWithParams')}
                  </Button>
                ) : null}
              </div>
            </div>

            <div className="history-report-main">
              <ReportArtifactPreview job={selectedGenerationJob} />
            </div>
            <Modal
              title={t('history.detail.reports.regenerateWithParams')}
              open={paramsModalOpen}
              onCancel={() => setParamsModalOpen(false)}
              onOk={submitRegenerate}
              confirmLoading={regenerateMutation.isPending}
              okText={t('history.detail.reports.submitRegenerate')}
              destroyOnHidden
            >
              <Typography.Paragraph type="secondary">
                {t('history.detail.reports.regenerateParamsHint')}
              </Typography.Paragraph>
              <Input
                value={paramsReason}
                onChange={(event) => setParamsReason(event.target.value)}
                placeholder={t('history.detail.reports.regenerateReason')}
              />
              <Input.TextArea
                className="history-report-params-editor"
                value={paramsDraft}
                onChange={(event) => setParamsDraft(event.target.value)}
                rows={14}
                spellCheck={false}
              />
            </Modal>
          </div>
        )}
      </div>
    </div>
  )
}
