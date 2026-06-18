import { useEffect, useId, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Checkbox, Empty, Form, Input, InputNumber, Space, Table, Tag, Tooltip, Typography, Upload, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { UploadProps } from 'antd'
import { Download, FileSpreadsheet, RefreshCw, Save, UploadCloud } from 'lucide-react'
import { saveAs } from 'file-saver'
import { useTranslation } from 'react-i18next'
import {
  downloadMainReportTemplateArtifact,
  getMainReportTemplates,
  updateMainReportTemplateMapping,
  uploadMainReportTemplate,
} from '@/features/edge-status/api'
import { createLuckysheetAdapter } from '@/features/spreadsheet/luckysheetAdapter'
import { env } from '@/shared/config/env'
import type { ReportTemplate } from '@/shared/api/types'
import './reports.css'

type UploadFormValues = {
  template_code: string
  name?: string
  display_name?: string
  version?: number
  enabled?: boolean
  remark?: string
}

function templateTitle(template: ReportTemplate) {
  return template.display_name || template.name || template.template_code
}

function fileToTemplateCode(file?: File) {
  if (!file) return ''
  return file.name.replace(/\.[^.]+$/, '').replace(/[^A-Za-z0-9_-]+/g, '_').toUpperCase()
}

function TemplatePreview({ template }: { template?: ReportTemplate }) {
  const { t } = useTranslation()
  const adapterRef = useRef<ReturnType<typeof createLuckysheetAdapter> | null>(null)
  const containerId = `report-template-preview-${useId().replace(/:/g, '')}`

  const artifactQuery = useQuery({
    queryKey: ['report-settings', 'templates', 'artifact-preview', template?.id],
    queryFn: () => downloadMainReportTemplateArtifact(template!.id),
    enabled: Boolean(template?.id),
    retry: false,
  })

  useEffect(() => {
    const adapter = createLuckysheetAdapter()
    adapterRef.current = adapter
    let disposed = false

    adapter
      .mount({
        containerId,
        data: [{ name: template ? templateTitle(template) : 'Template', index: '0', status: 1, order: 0 }],
        readonly: true,
        toolbar: false,
        sheetbar: true,
      })
      .then(async () => {
        if (disposed || !artifactQuery.data || !template) return
        const file = new File([artifactQuery.data.blob], artifactQuery.data.filename, { type: artifactQuery.data.contentType })
        await adapter.importFile(file)
      })
      .catch((error) => {
        if (!disposed) console.error(error)
      })

    return () => {
      disposed = true
      adapter.unmount()
      adapterRef.current = null
    }
  }, [artifactQuery.data, containerId, template])

  return (
    <section className="report-luckysheet-panel">
      <div className="report-subtitle">{template ? templateTitle(template) : t('reportSettings.templates.preview')}</div>
      {artifactQuery.isError ? (
        <Alert type="warning" showIcon message={artifactQuery.error instanceof Error ? artifactQuery.error.message : t('reportSettings.templates.previewFailed')} />
      ) : null}
      <div id={containerId} className="report-luckysheet-host" />
    </section>
  )
}

export function ReportTemplateManagementPage() {
  const { t } = useTranslation()
  const [form] = Form.useForm<UploadFormValues>()
  const [messageApi, contextHolder] = message.useMessage()
  const queryClient = useQueryClient()
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null)
  const [mappingDraft, setMappingDraft] = useState<{ templateId?: number; text: string }>({ text: '' })
  const isMainServer = env.runtimeRole === 'main_server'

  const templatesQuery = useQuery({
    queryKey: ['report-settings', 'templates'],
    queryFn: () => getMainReportTemplates({}),
    enabled: isMainServer,
  })
  const templates = templatesQuery.data?.items ?? []
  const selectedTemplate = templates.find((template) => template.id === selectedTemplateId) ?? templates[0]
  const mappingText = mappingDraft.templateId === selectedTemplate?.id ? mappingDraft.text : selectedTemplate?.params_schema_json || ''

  const uploadProps: UploadProps = {
    accept: '.xlsx',
    maxCount: 1,
    beforeUpload: (file) => {
      const nextFile = file as File
      setSelectedFile(nextFile)
      if (!form.getFieldValue('template_code')) form.setFieldValue('template_code', fileToTemplateCode(nextFile))
      if (!form.getFieldValue('name')) form.setFieldValue('name', nextFile.name.replace(/\.[^.]+$/, ''))
      return false
    },
    onRemove: () => {
      setSelectedFile(null)
    },
  }

  const uploadMutation = useMutation({
    mutationFn: async (values: UploadFormValues) => {
      if (!selectedFile) throw new Error(t('reportSettings.templates.fileRequired'))
      return uploadMainReportTemplate({
        file: selectedFile,
        template_code: values.template_code,
        name: values.name,
        display_name: values.display_name,
        version: values.version,
        enabled: values.enabled ?? true,
        remark: values.remark,
      })
    },
    onSuccess: async (result) => {
      messageApi.success(t('reportSettings.templates.uploaded'))
      setSelectedTemplateId(result.template.id)
      setSelectedFile(null)
      form.resetFields()
      await queryClient.invalidateQueries({ queryKey: ['report-settings', 'templates'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.templates.uploadFailed')),
  })

  const mappingMutation = useMutation({
    mutationFn: async () => {
      if (!selectedTemplate) throw new Error(t('reportSettings.templates.selectTemplate'))
      const trimmed = mappingText.trim()
      if (trimmed) JSON.parse(trimmed)
      return updateMainReportTemplateMapping(selectedTemplate.id, { params_schema_json: trimmed })
    },
    onSuccess: async () => {
      messageApi.success(t('reportSettings.templates.mappingSaved'))
      await queryClient.invalidateQueries({ queryKey: ['report-settings', 'templates'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.templates.mappingFailed')),
  })

  const downloadMutation = useMutation({
    mutationFn: (templateId: number) => downloadMainReportTemplateArtifact(templateId),
    onSuccess: (artifact) => {
      saveAs(artifact.blob, artifact.filename)
      messageApi.success(t('reportSettings.templates.downloadStarted'))
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('reportSettings.templates.downloadFailed')),
  })

  const columns: ColumnsType<ReportTemplate> = [
    {
      title: t('reportSettings.templates.columns.template'),
      dataIndex: 'template_code',
      render: (_value, record) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{templateTitle(record)}</Typography.Text>
          <Typography.Text type="secondary">{record.template_code}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('reportSettings.templates.columns.version'),
      dataIndex: 'version',
      width: 86,
      render: (value: number) => <Tag>v{value}</Tag>,
    },
    {
      title: t('reportSettings.templates.columns.file'),
      dataIndex: 'file_ref',
      ellipsis: true,
    },
    {
      title: t('reportSettings.templates.columns.updatedAt'),
      dataIndex: 'updated_at',
      width: 170,
      render: (value: string) => value?.replace('T', ' ').replace(/\.\d+.*$/, '') || '-',
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
          <h1>{t('reportSettings.templates.title')}</h1>
          <p>{t('reportSettings.templates.subtitle')}</p>
        </div>
        <Button icon={<RefreshCw size={14} />} onClick={() => void queryClient.invalidateQueries({ queryKey: ['report-settings', 'templates'] })} loading={templatesQuery.isFetching}>
          {t('actions.refresh')}
        </Button>
      </header>

      <main className="report-layout report-settings-layout">
        <section className="report-panel report-list-panel">
          <div className="report-panel-header">
            <Space>
              <FileSpreadsheet size={16} />
              <strong>{t('reportSettings.templates.listTitle')}</strong>
            </Space>
            <Tag>{templates.length}</Tag>
          </div>
          <Table<ReportTemplate>
            rowKey="id"
            size="small"
            columns={columns}
            dataSource={templates}
            loading={templatesQuery.isLoading}
            pagination={{ pageSize: 10, size: 'small' }}
            rowClassName={(record) => (record.id === selectedTemplate?.id ? 'report-row-selected' : '')}
            onRow={(record) => ({ onClick: () => setSelectedTemplateId(record.id) })}
            locale={{ emptyText: <Empty description={t('reportSettings.templates.empty')} /> }}
          />

          <div className="report-upload-panel">
            <Typography.Title level={5}>{t('reportSettings.templates.uploadTitle')}</Typography.Title>
            <Form form={form} layout="vertical" initialValues={{ enabled: true }} onFinish={(values) => uploadMutation.mutate(values)}>
              <Upload {...uploadProps}>
                <Button icon={<UploadCloud size={14} />}>{t('reportSettings.templates.pickFile')}</Button>
              </Upload>
              <Form.Item name="template_code" label={t('reportSettings.templates.fields.templateCode')} rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="name" label={t('reportSettings.templates.fields.name')}>
                <Input />
              </Form.Item>
              <Form.Item name="display_name" label={t('reportSettings.templates.fields.displayName')}>
                <Input />
              </Form.Item>
              <Form.Item name="version" label={t('reportSettings.templates.fields.version')}>
                <InputNumber min={1} className="report-full-input" />
              </Form.Item>
              <Form.Item name="remark" label={t('reportSettings.templates.fields.remark')}>
                <Input />
              </Form.Item>
              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>{t('reportSettings.templates.fields.enabled')}</Checkbox>
              </Form.Item>
              <Button type="primary" htmlType="submit" icon={<UploadCloud size={14} />} loading={uploadMutation.isPending}>
                {t('reportSettings.templates.upload')}
              </Button>
            </Form>
          </div>
        </section>

        <section className="report-panel report-detail-panel">
          {!selectedTemplate ? (
            <Empty description={t('reportSettings.templates.selectTemplate')} />
          ) : (
            <>
              <div className="report-panel-header">
                <Space wrap>
                  <Typography.Title level={3}>{templateTitle(selectedTemplate)}</Typography.Title>
                  <Tag>{selectedTemplate.template_code}</Tag>
                  <Tag>v{selectedTemplate.version}</Tag>
                  <Tag color={selectedTemplate.enabled ? 'green' : 'default'}>{selectedTemplate.enabled ? t('reportSettings.templates.enabled') : t('reportSettings.templates.disabled')}</Tag>
                </Space>
                <Tooltip title={t('reportSettings.templates.downloadHint')}>
                  <Button icon={<Download size={14} />} loading={downloadMutation.isPending} onClick={() => downloadMutation.mutate(selectedTemplate.id)}>
                    {t('reportSettings.templates.download')}
                  </Button>
                </Tooltip>
              </div>
              <TemplatePreview template={selectedTemplate} />
              <section className="report-grid-one">
                <div className="report-subtitle">{t('reportSettings.templates.mappingTitle')}</div>
                <Input.TextArea
                  value={mappingText}
                  onChange={(event) => setMappingDraft({ templateId: selectedTemplate.id, text: event.target.value })}
                  rows={8}
                  placeholder={t('reportSettings.templates.mappingPlaceholder')}
                />
                <div className="report-actions-row">
                  <Button type="primary" icon={<Save size={14} />} loading={mappingMutation.isPending} onClick={() => mappingMutation.mutate()}>
                    {t('reportSettings.templates.saveMapping')}
                  </Button>
                </div>
              </section>
            </>
          )}
        </section>
      </main>
    </div>
  )
}
