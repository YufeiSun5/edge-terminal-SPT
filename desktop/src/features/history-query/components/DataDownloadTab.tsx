import { Alert, Button, Empty, Tree, Typography } from 'antd'
import { Download, FileArchive } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { env } from '@/shared/config/env'

export function DataDownloadTab({ taskId }: { taskId: number }) {
  const { t } = useTranslation()
  void taskId
  const treeData = [
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
      children: [
        { title: t('history.detail.downloads.performanceReport'), key: 'report-1' },
        { title: t('history.detail.downloads.longRunDaily'), key: 'report-2' },
      ],
    },
    {
      title: t('history.detail.downloads.alarms'),
      key: 'alarms',
      children: [
        { title: t('history.detail.downloads.alarmRecords'), key: 'alarms-records' },
        { title: t('history.detail.downloads.alarmRecovery'), key: 'alarms-recovery' },
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
  ]

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
      <div>
        <div className="history-panel-header history-download-header">
          <Typography.Title level={4}>{t('history.detail.downloads.title')}</Typography.Title>
          <div className="history-download-actions">
            <Button icon={<FileArchive size={16} />}>
              {t('history.detail.downloads.downloadSelected')}
            </Button>
            <Button type="primary" icon={<Download size={16} />}>
              {t('history.detail.downloads.downloadAll')}
            </Button>
          </div>
        </div>
        <div className="history-panel-body">
          <Tree
            checkable
            defaultExpandAll
            treeData={treeData}
            className="history-download-tree"
          />
        </div>
      </div>
    </div>
  )
}
