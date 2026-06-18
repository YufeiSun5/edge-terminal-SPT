import { useMemo, useState } from 'react'
import type { Key } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Empty, Tree, Typography, message } from 'antd'
import type { DataNode } from 'antd/es/tree'
import { Download, FileArchive } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { saveAs } from 'file-saver'
import {
  downloadMainServerPackage,
  getMainReportJobs,
} from '@/features/edge-status/api'
import { env } from '@/shared/config/env'

function collectLeafKeys(nodes: DataNode[]) {
  const keys: string[] = []
  const visit = (node: DataNode) => {
    if (node.children?.length) {
      node.children.forEach(visit)
      return
    }
    keys.push(String(node.key))
  }
  nodes.forEach(visit)
  return keys
}

function resolveCheckedLeafKeys(nodes: DataNode[], checked: Key[]) {
  const checkedSet = new Set(checked.map(String))
  const leafKeys = new Set<string>()
  const visit = (node: DataNode, parentChecked: boolean) => {
    const selected = parentChecked || checkedSet.has(String(node.key))
    if (node.children?.length) {
      node.children.forEach((child) => visit(child, selected))
      return
    }
    if (selected) leafKeys.add(String(node.key))
  }
  nodes.forEach((node) => visit(node, false))
  return Array.from(leafKeys)
}

export function DataDownloadTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  const [checkedKeys, setCheckedKeys] = useState<Key[]>([])
  const [messageApi, contextHolder] = message.useMessage()
  const [downloading, setDownloading] = useState<'selected' | 'all' | null>(null)

  const jobsQuery = useQuery({
    queryKey: ['history', 'run', 'download-report-jobs', taskId],
    queryFn: () => getMainReportJobs({ task_id: taskId, limit: 100 }),
    enabled: env.runtimeRole === 'main_server',
    retry: false,
  })

  const reportJobs = useMemo(
    () => (jobsQuery.data?.items ?? []).filter((job) => job.status === 'succeeded' || job.status === 'success'),
    [jobsQuery.data?.items],
  )

  const treeData = useMemo<DataNode[]>(() => [
    {
      title: t('history.detail.downloads.data'),
      key: 'data',
      children: [
        { title: t('history.detail.downloads.rawSamples'), key: 'data-raw' },
        { title: t('history.detail.downloads.filteredData'), key: 'data-filtered' },
        { title: t('history.detail.downloads.chartImage'), key: 'data-chart' },
        { title: t('history.detail.downloads.detailedTable'), key: 'data-table' },
      ],
    },
    {
      title: t('history.detail.downloads.reports'),
      key: 'reports',
      children: reportJobs.length > 0
        ? reportJobs.map((job) => ({
            title: `${job.report_name || job.template_code || t('history.detail.downloads.reportArtifact')} (${job.template_code || '-'})`,
            key: `report-job-${job.id}`,
          }))
        : [{ title: t('history.detail.downloads.noReportArtifacts'), key: 'reports-empty', disabled: true }],
    },
    {
      title: t('history.detail.downloads.alarms'),
      key: 'alarms',
      children: [
        { title: t('history.detail.downloads.alarmRecords'), key: 'alarms-records' },
        { title: t('history.detail.downloads.eventLog'), key: 'alarms-events' },
      ],
    },
    {
      title: t('history.detail.downloads.config'),
      key: 'config',
      children: [
        { title: t('history.detail.downloads.standardSnapshot'), key: 'config-standard' },
        { title: t('history.detail.downloads.limitSnapshot'), key: 'config-limits' },
        { title: t('history.detail.downloads.reportRequestSnapshot'), key: 'config-reports' },
        { title: t('history.detail.downloads.storageRouteSnapshot'), key: 'config-routes' },
      ],
    },
  ], [reportJobs, t])

  const allLeafKeys = useMemo(() => collectLeafKeys(treeData).filter((key) => key !== 'reports-empty'), [treeData])
  const selectedLeafKeys = useMemo(() => resolveCheckedLeafKeys(treeData, checkedKeys), [checkedKeys, treeData])

  async function handleDownload(mode: 'selected' | 'all') {
    const keys = (mode === 'all' ? allLeafKeys : selectedLeafKeys).filter((key) => key !== 'reports-empty')
    if (keys.length === 0) {
      messageApi.warning(t('history.detail.downloads.selectAtLeastOne'))
      return
    }
    setDownloading(mode)
    try {
      const artifact = await downloadMainServerPackage({ task_id: taskId, keys })
      saveAs(artifact.blob, artifact.filename)
      messageApi.success(t('history.detail.downloads.packageStarted'))
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('history.detail.downloads.packageFailed'))
    } finally {
      setDownloading(null)
    }
  }

  if (env.runtimeRole !== 'main_server') {
    return (
      <div className="history-tab-content history-remote-only-tab">
        <Alert
          type="info"
          showIcon
          title={t('history.detail.downloads.remoteTitle')}
          description={t('history.detail.downloads.remoteDescription')}
        />
        <Empty description={t('history.detail.downloads.remoteEmpty')} />
      </div>
    )
  }

  return (
    <div className="history-tab-content history-download-tab">
      {contextHolder}
      <div>
        <div className="history-panel-header history-download-header">
          <div>
            <Typography.Title level={4}>{t('history.detail.downloads.title')}</Typography.Title>
            <Typography.Text type="secondary">
              {t('history.detail.downloads.selectedCount', { count: selectedLeafKeys.length })}
            </Typography.Text>
          </div>
          <div className="history-download-actions">
            <Button
              icon={<FileArchive size={16} />}
              loading={downloading === 'selected'}
              onClick={() => void handleDownload('selected')}
            >
              {t('history.detail.downloads.downloadSelected')}
            </Button>
            <Button
              type="primary"
              icon={<Download size={16} />}
              loading={downloading === 'all'}
              onClick={() => void handleDownload('all')}
            >
              {t('history.detail.downloads.downloadAll')}
            </Button>
          </div>
        </div>
        <div className="history-panel-body">
          <Alert
            className="history-report-alert"
            type="info"
            showIcon
            title={t('history.detail.downloads.packageScope')}
          />
          <Tree
            checkable
            defaultExpandAll
            checkedKeys={checkedKeys}
            treeData={treeData}
            className="history-download-tree"
            onCheck={(keys) => setCheckedKeys(Array.isArray(keys) ? keys : keys.checked)}
          />
        </div>
      </div>
    </div>
  )
}
