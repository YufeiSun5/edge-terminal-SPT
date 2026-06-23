import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import { Copy, ListPlus, Edit3, Plus, Save, SlidersHorizontal, Trash2 } from 'lucide-react'
import { queryClient } from '@/app/queryClient'
import {
  createDetectionStandard,
  deleteDetectionStandard,
  getDetectionStandard,
  getDetectionStandards,
  getProjects,
  getReportTemplates,
  getVariables,
  replaceDetectionStandardItems,
  updateDetectionStandard,
} from '@/features/edge-status/api'
import type { DetectionStandard, DetectionStandardItemPayload, DetectionStandardPayload, Project, ReportTemplate, VarIdentifier, VariableConfig } from '@/shared/api/types'
import {
  extractInletAreaM2,
  normalizeDetectionItemName,
  normalizeDetectionStandardItems,
  sanitizeDetectionStandardItems,
  sortDetectionItems,
  standardDetectionItemOrder,
} from '@/shared/detection/detectionItemOrder'
import { detectionStandardScopeColor, detectionStandardScopeLabel } from '@/shared/detection/standardScope'
import { languageCode } from '@/shared/i18n/language'
import '@/features/settings/settings.css'
import './detection-config.css'

type DetectionStandardFormValues = DetectionStandardPayload
type CopyStandardFormValues = {
  standard_code: string
  name?: string
  display_name: string
}

export type DetectionConfigEditorVariant = 'page' | 'station-modal'

export type StationDetectionConfigDraft = {
  projectId: number
  standardId?: number
  configCode?: string
  configName?: string
  configVersion?: number
  configHash?: string
  items: DetectionStandardItemPayload[]
  processParams: {
    inlet_area_m2?: number
  }
  updatedAt: number
}

type DetectionConfigEditorProps = {
  variant?: DetectionConfigEditorVariant
  projectId?: number
  selectedProject?: Project
  initialStandardId?: number
  initialDraft?: StationDetectionConfigDraft
  running?: boolean
  standards?: DetectionStandard[]
  projects?: Project[]
  variables?: VariableConfig[]
  reportTemplates?: ReportTemplate[]
  onApplyDraft?: (draft: StationDetectionConfigDraft) => void
}

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return variable.display_name_en || variable.var_name || variable.raw_name
  if (currentLanguage === 'ja') return variable.display_name_ja || variable.var_name || variable.raw_name
  return variable.display_name || variable.raw_name || variable.var_name
}

function standardItemTitle(item: DetectionStandardItemPayload, language?: string) {
  const currentLanguage = languageCode(language)
  if (currentLanguage === 'en') return item.display_name_en || item.var_name
  if (currentLanguage === 'ja') return item.display_name_ja || item.var_name
  return item.display_name || item.var_name
}

function variableWireId(variable: Pick<VariableConfig, 'var_id' | 'var_id_text'>): string {
  return variable.var_id_text ?? String(variable.var_id)
}

function varKey(value?: VarIdentifier | null) {
  return value === undefined || value === null || value === '' ? '' : String(value)
}

function sameVarId(left?: VarIdentifier | null, right?: VarIdentifier | null) {
  return varKey(left) === varKey(right)
}

function standardProjectId(standard: Pick<DetectionStandard, 'project_id'>) {
  return standard.project_id
}

function standardProjectCode(standard: Pick<DetectionStandard, 'project_code'>) {
  return standard.project_code
}

function projectCode(project?: Pick<Project, 'project_code'>) {
  return project?.project_code || ''
}

function reportTemplateTitle(template?: Pick<ReportTemplate, 'template_code' | 'display_name' | 'name'>) {
  if (!template) return ''
  return template.display_name || template.name || template.template_code
}

function standardItemFromVariable(variable: VariableConfig, sortOrder: number): DetectionStandardItemPayload {
  return {
    var_id: variableWireId(variable),
    var_name: variable.var_name,
    display_name: variable.display_name || variable.raw_name || variable.var_name,
    display_name_en: variable.display_name_en,
    display_name_ja: variable.display_name_ja,
    check_enabled: true,
    alarm_enabled: true,
    store_enabled: true,
    check_cycle_ms: 0,
    check_on_start: true,
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
    sort_order: sortOrder,
  }
}

function standardItemNames(item: Pick<DetectionStandardItemPayload, 'display_name' | 'var_name'>) {
  return [item.display_name, item.var_name].map(normalizeDetectionItemName).filter(Boolean)
}

function variableNames(variable: VariableConfig) {
  return [variable.display_name, variable.raw_name, variable.var_name, variable.display_name_en, variable.display_name_ja]
    .map(normalizeDetectionItemName)
    .filter(Boolean)
}

function hasMatchingName(names: string[], targetName: string) {
  const normalizedTarget = normalizeDetectionItemName(targetName)
  return names.some((name) => name === normalizedTarget || name.includes(normalizedTarget) || normalizedTarget.includes(name))
}

function defaultVariableMatchScore(variable: VariableConfig, targetName: string, scopeProjectId?: number) {
  const names = variableNames(variable)
  const normalizedTarget = normalizeDetectionItemName(targetName)
  const exactNameScore = names.some((name) => name === normalizedTarget) ? 100 : 0
  const displayScore = normalizeDetectionItemName(variable.display_name || variable.raw_name || variable.var_name) === normalizedTarget ? 20 : 0
  const projectScore = scopeProjectId
    ? variable.project_id === scopeProjectId
      ? 200
      : variable.project_id
        ? -50
        : 0
    : 0
  return projectScore + exactNameScore + displayScore
}

function nextCopyCode(baseCode: string, standards: DetectionStandard[]) {
  const usedCodes = new Set(standards.map((standard) => standard.standard_code))
  for (let index = 2; index < 1000; index += 1) {
    const candidate = `${baseCode}_COPY${index}`
    if (!usedCodes.has(candidate)) return candidate
  }
  return `${baseCode}_COPY_${Date.now().toString().slice(-6)}`
}

export function DetectionConfigEditor({
  variant = 'page',
  projectId,
  selectedProject,
  initialStandardId,
  initialDraft,
  running = false,
  standards: providedStandards,
  projects: providedProjects,
  variables: providedVariables,
  reportTemplates: providedReportTemplates,
  onApplyDraft,
}: DetectionConfigEditorProps) {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const [selectedStandardId, setSelectedStandardId] = useState<number | undefined>(
    () => initialDraft?.standardId ?? initialStandardId,
  )
  const [editingStandard, setEditingStandard] = useState<DetectionStandard | undefined>()
  const [standardItems, setStandardItems] = useState<DetectionStandardItemPayload[]>(() =>
    initialDraft ? sanitizeDetectionStandardItems(initialDraft.items) : [],
  )
  const [draftStandardId, setDraftStandardId] = useState<number | undefined>(
    () => initialDraft?.standardId,
  )
  const [standardVariableId, setStandardVariableId] = useState<VarIdentifier | undefined>()
  const [standardModalOpen, setStandardModalOpen] = useState(false)
  const [advancedMode, setAdvancedMode] = useState(false)
  const [copyModalOpen, setCopyModalOpen] = useState(false)
  const [copySourceStandard, setCopySourceStandard] = useState<DetectionStandard | undefined>()
  const [processParams, setProcessParams] = useState<{ inlet_area_m2?: number }>(
    () => initialDraft?.processParams ?? {},
  )
  const [standardForm] = Form.useForm<DetectionStandardFormValues>()
  const [copyForm] = Form.useForm<CopyStandardFormValues>()

  const standardsQuery = useQuery({
    queryKey: ['detection-config', 'standards'],
    queryFn: () => getDetectionStandards(),
    enabled: !providedStandards,
    retry: false,
  })
  const projectsQuery = useQuery({
    queryKey: ['detection-config', 'projects'],
    queryFn: getProjects,
    enabled: !providedProjects,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['detection-config', 'variables'],
    queryFn: () => getVariables(),
    enabled: !providedVariables,
    retry: false,
  })
  const reportTemplatesQuery = useQuery({
    queryKey: ['detection-config', 'report-templates'],
    queryFn: () => getReportTemplates({ enabled: true }),
    enabled: !providedReportTemplates,
    retry: false,
  })

  const isStationModal = variant === 'station-modal'
  const standards = useMemo(() => providedStandards ?? standardsQuery.data ?? [], [providedStandards, standardsQuery.data])
  const projects = useMemo(() => providedProjects ?? projectsQuery.data ?? [], [providedProjects, projectsQuery.data])
  const variables = useMemo(() => providedVariables ?? variablesQuery.data ?? [], [providedVariables, variablesQuery.data])
  const reportTemplates = useMemo(() => providedReportTemplates ?? reportTemplatesQuery.data ?? [], [providedReportTemplates, reportTemplatesQuery.data])
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
    return sortDetectionItems(
      Array.from(byName.values()).map((variable) => ({
        ...variable,
        display_name: variableTitle(variable, i18n.resolvedLanguage),
      })),
    )
  }, [i18n.resolvedLanguage, variables])
  const selectedStandard = standards.find((item) => item.id === selectedStandardId) ?? standards[0]
  const selectedStandardDetailQuery = useQuery({
    queryKey: ['detection-config', 'standard-detail', selectedStandard?.id],
    queryFn: () => getDetectionStandard(selectedStandard?.id as number),
    enabled: Boolean(selectedStandard?.id) && (!providedStandards || !selectedStandard?.items?.length),
    retry: false,
  })
  const selectedStandardDetail = selectedStandardDetailQuery.data ?? selectedStandard

  const selectedStandardItems = useMemo(() => {
    if (!selectedStandardDetail) return []
    if (draftStandardId === selectedStandardDetail.id) return standardItems
    return normalizeDetectionStandardItems(selectedStandardDetail.items)
  }, [draftStandardId, selectedStandardDetail, standardItems])
  const checkedItemCount = useMemo(() => selectedStandardItems.filter((item) => item.check_enabled ?? true).length, [selectedStandardItems])
  const selectedReportTemplateName = reportTemplateTitle(reportTemplates.find((template) => template.id === selectedStandardDetail?.report_template_id))
  const selectedStandardDirty = Boolean(selectedStandardDetail && draftStandardId === selectedStandardDetail.id)

  const displayProjectName = useCallback((project: Project) => {
    const code = projectCode(project)
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return project.display_name_en || code
    if (currentLanguage === 'ja') return project.display_name_ja || code
    return project.display_name || project.name || code
  }, [i18n.resolvedLanguage])

  const displayStandardName = useCallback((standard: DetectionStandard) => {
    const currentLanguage = languageCode(i18n.resolvedLanguage)
    if (currentLanguage === 'en') return standard.display_name_en || standard.standard_code
    if (currentLanguage === 'ja') return standard.display_name_ja || standard.standard_code
    return standard.display_name || standard.name || standard.standard_code
  }, [i18n.resolvedLanguage])

  const standardOptions = useMemo(() => standards.map((standard) => ({
    label: `${displayStandardName(standard)} · ${standard.standard_code}`,
    value: standard.id,
  })), [displayStandardName, standards])
  const projectOptions = useMemo(() => projects.map((project) => ({
    label: `${displayProjectName(project)} · ${projectCode(project)}`,
    value: project.id,
  })), [displayProjectName, projects])
  const reportTemplateOptions = useMemo(() => reportTemplates.map((template) => ({
    label: `${reportTemplateTitle(template)} · ${template.template_code}`,
    value: template.id,
  })), [reportTemplates])

  const standardVariableOptions = useMemo(() => standardVariables.map((variable) => ({
    label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
    value: variableWireId(variable),
  })), [i18n.resolvedLanguage, standardVariables])

  function selectStandard(standardId?: number) {
    setSelectedStandardId(standardId)
    const nextStandard = standards.find((standard) => standard.id === standardId)
    const matchingDraft = isStationModal && initialDraft?.standardId === standardId ? initialDraft : undefined
    if (matchingDraft) {
      setStandardItems(sanitizeDetectionStandardItems(matchingDraft.items))
      setDraftStandardId(standardId)
      setProcessParams(matchingDraft.processParams)
      return
    }
    setDraftStandardId(undefined)
    setStandardItems([])
    setProcessParams({
      inlet_area_m2: extractInletAreaM2(nextStandard?.items),
    })
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
        project_id: standardProjectId(detail),
        project_code: standardProjectCode(detail),
        project_group: detail.project_group,
        mode: detail.mode,
        report_template_id: detail.report_template_id,
        version: detail.version,
        enabled: detail.enabled,
        remark: detail.remark,
      })
      setStandardItems(normalizeDetectionStandardItems(detail.items))
      setDraftStandardId(detail.id)
    } else {
      standardForm.setFieldsValue({
        standard_code: `STD-${Date.now().toString().slice(-6)}`,
        project_id: undefined,
        project_code: '',
        project_group: '',
        mode: 'standard',
        version: 1,
        enabled: true,
        report_template_id: undefined,
        remark: '',
      })
      setStandardItems([])
      setDraftStandardId(undefined)
    }
    setStandardModalOpen(true)
  }

  function addStandardItem(variableId?: VarIdentifier) {
    if (!variableId) return
    const variable = standardVariables.find((item) => sameVarId(variableWireId(item), variableId))
    const currentItems = standardModalOpen ? standardItems : selectedStandardItems
    if (!variable || currentItems.some((item) => item.var_name === variable.var_name)) return
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) =>
      sanitizeDetectionStandardItems([
        ...(standardModalOpen ? items : currentItems),
        standardItemFromVariable(variable, currentItems.length + 1),
      ]),
    )
    setStandardVariableId(undefined)
  }

  function addDefaultStandardItems() {
    if (running) return
    if (!standardModalOpen && !selectedStandardDetail) return
    const currentItems = standardModalOpen ? standardItems : selectedStandardItems
    const existingVarIds = new Set(currentItems.map((item) => varKey(item.var_id)).filter(Boolean))
    const existingNames = currentItems.flatMap(standardItemNames)
    const scopeProjectId = standardModalOpen
      ? Number(standardForm.getFieldValue('project_id')) || undefined
      : standardProjectId(selectedStandardDetail as DetectionStandard) ?? projectId
    const additions: DetectionStandardItemPayload[] = []
    const missingNames: string[] = []

    standardDetectionItemOrder.forEach((itemName) => {
      if (hasMatchingName(existingNames, itemName)) return
      const candidates = variables
        .filter((variable) => !existingVarIds.has(varKey(variableWireId(variable))))
        .filter((variable) => hasMatchingName(variableNames(variable), itemName))
        .sort((left, right) => defaultVariableMatchScore(right, itemName, scopeProjectId) - defaultVariableMatchScore(left, itemName, scopeProjectId))
      const variable = candidates[0]
      if (!variable) {
        missingNames.push(itemName)
        return
      }
      existingVarIds.add(varKey(variableWireId(variable)))
      existingNames.push(...variableNames(variable))
      additions.push(standardItemFromVariable(variable, currentItems.length + additions.length + 1))
    })

    if (additions.length === 0) {
      if (missingNames.length > 0) {
        messageApi.warning(t('detectionConfig.defaultItemsMissing', { names: missingNames.join('、') }))
      } else {
        messageApi.info(t('detectionConfig.defaultItemsNoChange'))
      }
      return
    }

    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems(sanitizeDetectionStandardItems([...currentItems, ...additions]))
    setStandardVariableId(undefined)
    if (missingNames.length > 0) {
      messageApi.warning(t('detectionConfig.defaultItemsAddedWithMissing', { count: additions.length, names: missingNames.join('、') }))
    } else {
      messageApi.success(t('detectionConfig.defaultItemsAdded', { count: additions.length }))
    }
  }

  function openCopyStandardModal() {
    if (!selectedStandardDetail) return
    const baseName = displayStandardName(selectedStandardDetail)
    setCopySourceStandard(selectedStandardDetail)
    copyForm.setFieldsValue({
      standard_code: nextCopyCode(selectedStandardDetail.standard_code, standards),
      display_name: `${baseName}_副本`,
      name: `${selectedStandardDetail.name || baseName}_副本`,
    })
    setCopyModalOpen(true)
  }

  function patchStandardItem(varId: VarIdentifier, patch: Partial<DetectionStandardItemPayload>) {
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) => {
      const currentItems = standardModalOpen ? items : selectedStandardItems
      return sanitizeDetectionStandardItems(currentItems.map((item) => sameVarId(item.var_id, varId) ? { ...item, ...patch } : item))
    })
  }

  function removeStandardItem(varId: VarIdentifier) {
    if (!standardModalOpen && selectedStandardDetail) {
      setDraftStandardId(selectedStandardDetail.id)
    }
    setStandardItems((items) => {
      const currentItems = standardModalOpen ? items : selectedStandardItems
      return sanitizeDetectionStandardItems(currentItems.filter((item) => !sameVarId(item.var_id, varId)))
    })
  }

  function toggleAllCheckEnabled() {
    if (running) return
    if (!selectedStandardDetail) return
    if (!standardModalOpen) setDraftStandardId(selectedStandardDetail.id)
    const currentItems = standardModalOpen ? standardItems : selectedStandardItems
    const allEnabled = currentItems.every((item) => item.check_enabled ?? true)
    setStandardItems(sanitizeDetectionStandardItems(currentItems.map((item) => ({ ...item, check_enabled: !allEnabled }))))
  }

  function applyStationDraft() {
    if (!isStationModal || !projectId || !selectedStandardDetail || !onApplyDraft) return
    onApplyDraft({
      projectId,
      standardId: selectedStandardDetail.id,
      configCode: selectedStandardDetail.standard_code,
      configName: displayStandardName(selectedStandardDetail),
      configVersion: selectedStandardDetail.version,
      configHash: selectedStandardDetail.config_hash,
      items: sanitizeDetectionStandardItems(selectedStandardItems),
      processParams,
      updatedAt: Date.now(),
    })
  }

  const saveStandardMutation = useMutation({
    mutationFn: async (values: DetectionStandardFormValues) => {
      const project = projects.find((item) => item.id === values.project_id)
      const payloadItems = sanitizeDetectionStandardItems(standardItems)
      const displayName = values.display_name?.trim() || values.name?.trim() || values.standard_code
      const payload: DetectionStandardPayload = {
        ...values,
        standard_code: values.standard_code?.trim() || `STD-${Date.now().toString().slice(-6)}`,
        name: values.name?.trim() || displayName,
        display_name: displayName,
        project_code: projectCode(project) || values.project_code || '',
        project_group: values.project_group || project?.project_group || '',
        mode: values.mode || 'standard',
        version: editingStandard?.version ?? 1,
        enabled: values.enabled ?? true,
        items: payloadItems,
      }
      if (editingStandard) {
        const updated = await updateDetectionStandard(editingStandard.id, payload)
        return replaceDetectionStandardItems(updated.id, payloadItems)
      }
      return createDetectionStandard(payload)
    },
    onSuccess: async () => {
      setStandardModalOpen(false)
      setEditingStandard(undefined)
      setStandardItems([])
      setDraftStandardId(undefined)
      setStandardVariableId(undefined)
      standardForm.resetFields()
      messageApi.success(t('settings.messages.standardSaved'))
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
      await queryClient.invalidateQueries({ queryKey: ['station', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveCurrentStandardMutation = useMutation({
    mutationFn: async () => {
      if (!selectedStandardDetail) throw new Error(t('detectionConfig.noSelection'))
      const payloadItems = sanitizeDetectionStandardItems(selectedStandardItems)
      const payload: DetectionStandardPayload = {
        standard_code: selectedStandardDetail.standard_code,
        name: selectedStandardDetail.name,
        display_name: selectedStandardDetail.display_name,
        display_name_en: selectedStandardDetail.display_name_en,
        display_name_ja: selectedStandardDetail.display_name_ja,
        project_id: standardProjectId(selectedStandardDetail),
        project_code: standardProjectCode(selectedStandardDetail),
        project_group: selectedStandardDetail.project_group,
        mode: selectedStandardDetail.mode || 'standard',
        report_template_id: selectedStandardDetail.report_template_id,
        version: selectedStandardDetail.version ?? 1,
        enabled: selectedStandardDetail.enabled ?? true,
        remark: selectedStandardDetail.remark,
        items: payloadItems,
      }
      await updateDetectionStandard(selectedStandardDetail.id, payload)
      return replaceDetectionStandardItems(selectedStandardDetail.id, payloadItems)
    },
    onSuccess: async () => {
      messageApi.success(t('settings.messages.standardSaved'))
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
      await queryClient.invalidateQueries({ queryKey: ['station', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const copyStandardMutation = useMutation({
    mutationFn: async (values: CopyStandardFormValues) => {
      const source = copySourceStandard ?? selectedStandardDetail
      if (!source) throw new Error(t('detectionConfig.noSelection'))
      const detail = await getDetectionStandard(source.id)
      const sourceItems = selectedStandardDetail?.id === source.id && draftStandardId === source.id
        ? selectedStandardItems
        : normalizeDetectionStandardItems(detail.items)
      const payload: DetectionStandardPayload = {
        standard_code: values.standard_code.trim(),
        name: values.name?.trim() || values.display_name.trim(),
        display_name: values.display_name.trim(),
        display_name_en: detail.display_name_en,
        display_name_ja: detail.display_name_ja,
        project_id: standardProjectId(detail),
        project_code: standardProjectCode(detail),
        project_group: detail.project_group,
        mode: detail.mode || 'standard',
        report_template_id: detail.report_template_id,
        version: 1,
        enabled: detail.enabled ?? true,
        remark: detail.remark,
        items: sanitizeDetectionStandardItems(sourceItems),
      }
      return createDetectionStandard(payload)
    },
    onSuccess: async (created) => {
      setCopyModalOpen(false)
      setCopySourceStandard(undefined)
      copyForm.resetFields()
      setSelectedStandardId(created.id)
      setDraftStandardId(undefined)
      setStandardItems([])
      messageApi.success(t('detectionConfig.copiedStandard'))
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
      await queryClient.invalidateQueries({ queryKey: ['station', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const deleteStandardMutation = useMutation({
    mutationFn: (standard: DetectionStandard) => deleteDetectionStandard(standard.id),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.standardDeleted'))
      setSelectedStandardId(undefined)
      setDraftStandardId(undefined)
      await queryClient.invalidateQueries({ queryKey: ['detection-config', 'standards'] })
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
      await queryClient.invalidateQueries({ queryKey: ['station', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const compactItemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 260,
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
      width: 110,
      render: (_, record) => <Switch size="small" disabled={running} checked={record.check_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { check_enabled: checked })} />,
    },
    { title: t('detectionConfig.lowerLimit'), dataIndex: 'limit_l', key: 'limit_l', width: 150, render: (_, record) => <InputNumber size="small" disabled={running} value={record.limit_l ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_l: value })} /> },
    { title: t('detectionConfig.upperLimit'), dataIndex: 'limit_h', key: 'limit_h', width: 150, render: (_, record) => <InputNumber size="small" disabled={running} value={record.limit_h ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_h: value })} /> },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 80,
      render: (_, record) => <Button danger size="small" disabled={running} icon={<Trash2 size={13} />} onClick={() => removeStandardItem(record.var_id)} />,
    },
  ]
  const advancedItemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    compactItemColumns[0],
    compactItemColumns[1],
    {
      title: t('settings.standards.store'),
      dataIndex: 'store_enabled',
      key: 'store_enabled',
      width: 80,
      render: (_, record) => <Switch size="small" disabled={running} checked={record.store_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { store_enabled: checked })} />,
    },
    {
      title: t('settings.standards.alarm'),
      dataIndex: 'alarm_enabled',
      key: 'alarm_enabled',
      width: 80,
      render: (_, record) => <Switch size="small" disabled={running} checked={record.alarm_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { alarm_enabled: checked })} />,
    },
    {
      title: t('settings.standards.checkOnStart'),
      dataIndex: 'check_on_start',
      key: 'check_on_start',
      width: 110,
      render: (_, record) => <Switch size="small" disabled={running} checked={record.check_on_start ?? true} onChange={(checked) => patchStandardItem(record.var_id, { check_on_start: checked })} />,
    },
    {
      title: t('settings.standards.checkCycle'),
      dataIndex: 'check_cycle_ms',
      key: 'check_cycle_ms',
      width: 130,
      render: (_, record) => <InputNumber size="small" disabled={running} min={0} precision={0} value={record.check_cycle_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { check_cycle_ms: value ?? 0 })} />,
    },
    {
      title: t('settings.standards.checkMethod'),
      dataIndex: 'check_method',
      key: 'check_method',
      width: 150,
      render: (_, record) => (
        <Select
          size="small"
          disabled={running}
          value={record.check_method ?? 'numeric_range'}
          onChange={(value) => patchStandardItem(record.var_id, { check_method: value })}
          options={[
            { label: t('settings.standards.checkMethods.numericRange'), value: 'numeric_range' },
            { label: t('settings.standards.checkMethods.boolEquals'), value: 'bool_equals' },
            { label: t('settings.standards.checkMethods.stringEquals'), value: 'string_equals' },
            { label: t('settings.standards.checkMethods.regex'), value: 'regex' },
          ]}
        />
      ),
    },
    {
      title: t('settings.standards.targetValue'),
      dataIndex: 'target_value',
      key: 'target_value',
      width: 130,
      render: (_, record) => <Input size="small" disabled={running} value={record.target_value ?? ''} onChange={(event) => patchStandardItem(record.var_id, { target_value: event.target.value })} />,
    },
    { title: 'LL', dataIndex: 'limit_ll', key: 'limit_ll', width: 96, render: (_, record) => <InputNumber size="small" disabled={running} value={record.limit_ll ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_ll: value })} /> },
    compactItemColumns[2],
    compactItemColumns[3],
    { title: 'HH', dataIndex: 'limit_hh', key: 'limit_hh', width: 96, render: (_, record) => <InputNumber size="small" disabled={running} value={record.limit_hh ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_hh: value })} /> },
    { title: t('settings.standards.limitDeadband'), dataIndex: 'limit_deadband', key: 'limit_deadband', width: 120, render: (_, record) => <InputNumber size="small" disabled={running} min={0} value={record.limit_deadband ?? 0} onChange={(value) => patchStandardItem(record.var_id, { limit_deadband: value ?? 0 })} /> },
    { title: t('settings.standards.violationHold'), dataIndex: 'violation_hold_ms', key: 'violation_hold_ms', width: 130, render: (_, record) => <InputNumber size="small" disabled={running} min={0} value={record.violation_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { violation_hold_ms: value ?? 0 })} /> },
    { title: t('settings.standards.recoverHold'), dataIndex: 'recover_hold_ms', key: 'recover_hold_ms', width: 130, render: (_, record) => <InputNumber size="small" disabled={running} min={0} value={record.recover_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { recover_hold_ms: value ?? 0 })} /> },
    {
      title: t('settings.standards.qualityPolicy'),
      dataIndex: 'quality_policy',
      key: 'quality_policy',
      width: 150,
      render: (_, record) => (
        <Select
          size="small"
          disabled={running}
          value={record.quality_policy ?? 'ignore_bad'}
          onChange={(value) => patchStandardItem(record.var_id, { quality_policy: value })}
          options={[
            { label: t('settings.standards.qualityPolicies.ignoreBad'), value: 'ignore_bad' },
            { label: t('settings.standards.qualityPolicies.recordInvalid'), value: 'record_invalid' },
            { label: t('settings.standards.qualityPolicies.failOnBad'), value: 'fail_on_bad' },
          ]}
        />
      ),
    },
    { title: t('settings.variables.unit'), dataIndex: 'unit', key: 'unit', width: 80 },
    compactItemColumns[4],
  ]
  const showAdvancedMode = !isStationModal && advancedMode
  const visibleItemColumns = showAdvancedMode ? advancedItemColumns : compactItemColumns
  const standardActionControls = selectedStandardDetail ? (
    <>
      <Button icon={<Save size={15} />} type={isStationModal ? 'default' : 'primary'} disabled={running} loading={saveCurrentStandardMutation.isPending} onClick={() => saveCurrentStandardMutation.mutate()}>
        {t('settings.standards.save')}
      </Button>
      <Button icon={<SlidersHorizontal size={15} />} disabled={running} onClick={toggleAllCheckEnabled}>
        {selectedStandardItems.every((item) => item.check_enabled ?? true) ? t('detectionConfig.disableAll') : t('detectionConfig.enableAll')}
      </Button>
      {isStationModal ? (
        <Button type="primary" disabled={running || !projectId} onClick={applyStationDraft}>
          {t('detectionConfig.applyToStation')}
        </Button>
      ) : (
        <>
          <Button className="detection-copy-action" type="primary" icon={<Copy size={15} />} onClick={openCopyStandardModal}>
            {t('detectionConfig.copyStandard')}
          </Button>
          <Popconfirm
            title={t('settings.standards.deleteConfirm', { code: selectedStandardDetail.standard_code })}
            okText={t('settings.users.delete')}
            cancelText={t('settings.actions.cancel')}
            onConfirm={() => deleteStandardMutation.mutate(selectedStandardDetail)}
          >
            <Button danger icon={<Trash2 size={15} />} loading={deleteStandardMutation.isPending}>
              {t('settings.users.delete')}
            </Button>
          </Popconfirm>
        </>
      )}
    </>
  ) : null

  return (
    <div className={`detection-config-page ${isStationModal ? 'detection-config-page--station-modal' : ''}`}>
      {contextHolder}
      {!isStationModal ? (
        <div className="history-ambient-background" aria-hidden="true">
          <div className="history-orb history-orb-1" />
          <div className="history-orb history-orb-2" />
          <div className="history-orb history-orb-3" />
          <div className="history-noise" />
        </div>
      ) : null}

      <section className="detection-config-workspace glass-panel">
        <header className="detection-config-hero">
          <div className="detection-standard-toolbar">
            <span className="settings-eyebrow">{t('detectionConfig.title')}</span>
            <Select
              className="detection-standard-select"
              showSearch
              value={selectedStandard?.id}
              loading={standardsQuery.isFetching}
              optionFilterProp="label"
              placeholder={t('detectionConfig.selectStandard')}
              onChange={selectStandard}
              options={standardOptions}
            />
            {!isStationModal ? (
              <label className="detection-advanced-toggle">
                <span>{t('detectionConfig.advancedMode')}</span>
                <Switch size="small" checked={advancedMode} onChange={setAdvancedMode} />
              </label>
            ) : null}
            {!isStationModal ? (
              <Button icon={<Plus size={15} />} onClick={() => void openStandardModal()}>
              {t('settings.standards.create')}
              </Button>
            ) : null}
          </div>
          <div className="detection-config-actions">
            {!isStationModal ? standardActionControls : null}
          </div>
        </header>

        <div className="detection-config-grid">
          <aside className="detection-config-sidebar">
            {isStationModal ? (
              <div className="detection-station-overview">
                {selectedStandardDetail ? (
                  <>
                    <div className="detection-station-card">
                      <span className="settings-eyebrow">{selectedStandardDetail.standard_code}</span>
                      <h2>{displayStandardName(selectedStandardDetail)}</h2>
                      {selectedProject ? <p>{displayProjectName(selectedProject)}</p> : null}
                      <div className="detection-station-tags">
                        <Tag color={detectionStandardScopeColor(selectedStandardDetail)}>
                          {detectionStandardScopeLabel(selectedStandardDetail, t)}
                        </Tag>
                        <Tag color={selectedStandardDetail.enabled ? 'success' : 'default'}>{selectedStandardDetail.enabled ? t('status.online') : t('status.offline')}</Tag>
                        <Tag>{selectedStandardDetail.mode || 'standard'}</Tag>
                        <Tag>V{selectedStandardDetail.version}</Tag>
                      </div>
                      {selectedStandardDetail.remark ? <p className="detection-station-remark">{selectedStandardDetail.remark}</p> : null}
                    </div>
                    <div className="detection-station-actions">
                      {standardActionControls}
                    </div>
                    <div className="detection-summary-strip detection-summary-strip--station">
                      <div className="detection-summary-item">
                        <span>{t('settings.standards.items')}</span>
                        <strong>{selectedStandardItems.length}</strong>
                      </div>
                      <div className="detection-summary-item">
                        <span>{t('settings.standards.check')}</span>
                        <strong>{checkedItemCount}</strong>
                      </div>
                    </div>
                    <div className="detection-inline-meta detection-inline-meta--station">
                      <Tag>{selectedReportTemplateName || t('settings.standards.reportTemplate')}</Tag>
                    </div>
                    <div className="detection-process-params detection-process-params--station">
                      <div>
                        <strong>{t('detectionConfig.processParams')}</strong>
                        <span>{t('detectionConfig.processParamsHint')}</span>
                      </div>
                      <label>
                        <span>{t('detectionConfig.inletArea')}</span>
                        <InputNumber
                          min={0}
                          precision={3}
                          step={0.001}
                          disabled={running}
                          value={processParams.inlet_area_m2}
                          addonAfter="m²"
                          onChange={(value) =>
                            setProcessParams((current) => ({
                              ...current,
                              inlet_area_m2: typeof value === 'number' ? value : undefined,
                            }))
                          }
                        />
                      </label>
                    </div>
                  </>
                ) : (
                  <div className="detection-empty">{t('detectionConfig.selectStandard')}</div>
                )}
              </div>
            ) : (
              <>
                <div className="detection-sidebar-head">
                  <strong>{t('detectionConfig.standardList')}</strong>
                  <Tag>{standards.length}</Tag>
                </div>
                <div className="detection-standard-list">
                  {standards.length > 0 ? standards.map((standard) => (
                    <button
                      className={`detection-standard-card ${standard.id === selectedStandard?.id ? 'active' : ''}`}
                      key={standard.id}
                      type="button"
                      onClick={() => selectStandard(standard.id)}
                    >
                      <strong>{displayStandardName(standard)}</strong>
                      <span>{standard.standard_code} · {standard.mode || 'standard'}</span>
                      <em>{detectionStandardScopeLabel(standard, t)}</em>
                    </button>
                  )) : (
                    <div className="detection-empty">{t('detectionConfig.noStandards')}</div>
                  )}
                </div>
              </>
            )}
          </aside>

          <main className="detection-config-main">
            {!isStationModal ? (
              <>
                <div className="detection-panel-head">
                  <div>
                    <span className="settings-eyebrow">{selectedStandardDetail ? selectedStandardDetail.standard_code : t('detectionConfig.noSelection')}</span>
                    <h2>{selectedStandardDetail ? displayStandardName(selectedStandardDetail) : t('detectionConfig.selectStandard')}</h2>
                  </div>
                  {selectedStandardDetail ? (
                    <Space>
                      {showAdvancedMode ? (
                        <Button size="small" type="text" icon={<Edit3 size={13} />} onClick={() => void openStandardModal(selectedStandardDetail)}>
                          {t('detectionConfig.standardInfo')}
                        </Button>
                      ) : null}
                      {selectedStandardDirty ? <Tag color="processing">{t('settings.standards.save')}</Tag> : null}
                      <Tag color={detectionStandardScopeColor(selectedStandardDetail)}>
                        {detectionStandardScopeLabel(selectedStandardDetail, t)}
                      </Tag>
                      <Tag color={selectedStandardDetail.enabled ? 'success' : 'default'}>{selectedStandardDetail.enabled ? t('status.online') : t('status.offline')}</Tag>
                      <Tag>{selectedStandardDetail.mode || 'standard'}</Tag>
                    </Space>
                  ) : null}
                </div>

                {selectedStandardDetail ? (
                  <>
                    <div className="detection-summary-strip">
                      <div className="detection-summary-item">
                        <span>{t('settings.standards.items')}</span>
                        <strong>{selectedStandardItems.length}</strong>
                      </div>
                      <div className="detection-summary-item">
                        <span>{t('settings.standards.check')}</span>
                        <strong>{checkedItemCount}</strong>
                      </div>
                    </div>

                    <div className="detection-inline-meta">
                      <Tag color={detectionStandardScopeColor(selectedStandardDetail)}>
                        {detectionStandardScopeLabel(selectedStandardDetail, t)}
                      </Tag>
                      <Tag>V{selectedStandardDetail.version}</Tag>
                      <Tag>{selectedReportTemplateName || t('settings.standards.reportTemplate')}</Tag>
                      {selectedStandardDetail.remark ? <Tag>{selectedStandardDetail.remark}</Tag> : null}
                    </div>
                  </>
                ) : null}
              </>
            ) : null}

            <div className="detection-table-toolbar">
              <div className="settings-standard-item-picker">
                <Select
                  showSearch
                  allowClear
                  value={standardVariableId}
                  placeholder={t('settings.standards.addVariable')}
                  optionFilterProp="label"
                  onChange={setStandardVariableId}
                  options={standardVariableOptions}
                  disabled={running}
                />
                <Button size="small" icon={<Plus size={14} />} onClick={() => addStandardItem(standardVariableId)} disabled={running || !standardVariableId || !selectedStandardDetail}>
                  {t('settings.standards.add')}
                </Button>
                <Button size="small" icon={<ListPlus size={14} />} onClick={addDefaultStandardItems} disabled={running || !selectedStandardDetail || standardVariables.length === 0}>
                  {t('detectionConfig.addDefaultItems')}
                </Button>
              </div>
            </div>

            <Table
              className="detection-config-table settings-standard-items-table"
              size="small"
              virtual
              rowKey={(record) => varKey(record.var_id)}
              loading={standardsQuery.isFetching || selectedStandardDetailQuery.isFetching}
              columns={visibleItemColumns}
              dataSource={selectedStandardDetail ? selectedStandardItems : []}
              scroll={{ x: showAdvancedMode ? 1850 : 760, y: 520 }}
              pagination={{
                defaultPageSize: 30,
                pageSizeOptions: [20, 30, 50, 100],
                showSizeChanger: true,
                showQuickJumper: true,
                showTotal: (total) => `${total} ${t('settings.standards.items')}`,
                size: 'small',
              }}
            />
          </main>
        </div>
      </section>

      <Modal
        title={editingStandard ? t('settings.standards.edit') : t('settings.standards.create')}
        open={standardModalOpen}
        width={1120}
        onCancel={() => {
          setStandardModalOpen(false)
          setEditingStandard(undefined)
          setStandardItems([])
          setDraftStandardId(undefined)
          setStandardVariableId(undefined)
          standardForm.resetFields()
        }}
        footer={null}
      >
        <Form form={standardForm} layout="vertical" onFinish={(values) => saveStandardMutation.mutate(values)}>
          <div className="settings-form-grid modal-grid">
            <Form.Item name="display_name" label={t('settings.standards.displayName')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
          </div>
          {showAdvancedMode ? (
            <div className="settings-form-grid modal-grid">
              <Form.Item name="standard_code" label={t('settings.standards.code')} rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="name" label={t('settings.standards.internalName')}>
                <Input />
              </Form.Item>
              <Form.Item name="project_id" label={t('detectionConfig.projectScopeOptional')}>
                <Select
                  allowClear
                  options={projectOptions}
                  onChange={(projectId) => {
                    const project = projects.find((item) => item.id === projectId)
                    standardForm.setFieldValue('project_group', project?.project_group || '')
                  }}
                />
              </Form.Item>
              <Form.Item name="project_group" label={t('settings.groups.projectGroup')}>
                <Input />
              </Form.Item>
              <Form.Item name="mode" label={t('settings.standards.mode')}>
                <Input />
              </Form.Item>
              <Form.Item name="report_template_id" label={t('settings.standards.reportTemplate')}>
                <Select
                  allowClear
                  loading={reportTemplatesQuery.isFetching}
                  options={reportTemplateOptions}
                />
              </Form.Item>
              <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item className="settings-form-wide" name="remark" label={t('settings.standards.remark')}>
                <Input.TextArea rows={2} />
              </Form.Item>
            </div>
          ) : null}
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
                options={standardVariableOptions}
              />
              <Button size="small" icon={<Plus size={14} />} onClick={() => addStandardItem(standardVariableId)} disabled={!standardVariableId}>
                {t('settings.standards.add')}
              </Button>
              <Button size="small" icon={<ListPlus size={14} />} onClick={addDefaultStandardItems} disabled={standardVariables.length === 0}>
                {t('detectionConfig.addDefaultItems')}
              </Button>
            </div>
          </div>
          <Table
            className="settings-standard-items-table"
            size="small"
            virtual
            rowKey={(record) => varKey(record.var_id)}
            columns={visibleItemColumns}
            dataSource={standardItems}
            scroll={{ x: showAdvancedMode ? 1850 : 760, y: 320 }}
            pagination={{
              defaultPageSize: 20,
              pageSizeOptions: [20, 30, 50, 100],
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total) => `${total} ${t('settings.standards.items')}`,
              size: 'small',
            }}
          />
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveStandardMutation.isPending}>
              {t('settings.standards.save')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('detectionConfig.copyStandard')}
        open={copyModalOpen}
        okText={t('detectionConfig.copyStandard')}
        cancelText={t('settings.actions.cancel')}
        confirmLoading={copyStandardMutation.isPending}
        onOk={() => copyForm.submit()}
        onCancel={() => {
          setCopyModalOpen(false)
          setCopySourceStandard(undefined)
          copyForm.resetFields()
        }}
      >
        <Form form={copyForm} layout="vertical" onFinish={(values) => copyStandardMutation.mutate(values)}>
          <Form.Item name="display_name" label={t('settings.standards.displayName')} rules={[{ required: true, message: t('detectionConfig.copyNameRequired') }]}>
            <Input />
          </Form.Item>
          {showAdvancedMode ? (
            <>
              <Form.Item
                name="standard_code"
                label={t('settings.standards.code')}
                rules={[
                  { required: true, message: t('detectionConfig.copyCodeRequired') },
                  {
                    validator: (_, value) => {
                      const code = String(value ?? '').trim()
                      if (!code) return Promise.resolve()
                      if (standards.some((standard) => standard.standard_code === code)) {
                        return Promise.reject(new Error(t('detectionConfig.copyCodeExists')))
                      }
                      return Promise.resolve()
                    },
                  },
                ]}
              >
                <Input />
              </Form.Item>
              <Form.Item name="name" label={t('settings.standards.internalName')}>
                <Input />
              </Form.Item>
            </>
          ) : null}
        </Form>
      </Modal>
    </div>
  )
}

export function DetectionConfigPage() {
  return <DetectionConfigEditor variant="page" />
}
