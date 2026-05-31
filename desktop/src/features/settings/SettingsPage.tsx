import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, Button, Checkbox, Form, Input, InputNumber, Modal, Pagination, Popconfirm, Segmented, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { TableColumnsType } from 'antd'
import { useTranslation } from 'react-i18next'
import {
  Edit3,
  Cable,
  CircleAlert,
  Clipboard,
  Database,
  FolderTree,
  FolderOpen,
  KeyRound,
  Minimize2,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  ServerCog,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  UsersRound,
  UserRound,
} from 'lucide-react'
import { queryClient } from '@/app/queryClient'
import { createUser, deleteUser, getUsers, resetUserPassword, updateUser } from '@/features/auth/api'
import { useAuthStore } from '@/features/auth/authStore'
import type {
  AuditLogEntry,
  BulkRemapKioProjectsResult,
  BulkRemapKioProjectsResultItem,
  Device,
  GatewayConfig,
  GatewayConfigPayload,
  GatewayStatus,
  DatabaseConfigPayload,
  RoleName,
  ProjectMemberUpdate,
  DetectionStandard,
  DetectionStandardItemPayload,
  DetectionStandardPayload,
  StorageRoute,
  StorageRoutePayload,
  SystemUser,
  UserCreatePayload,
  UserPatchPayload,
  UserResetPasswordPayload,
  VariableAssignmentPayload,
  VariableConfig,
  VariableCreatePayload,
  VarIdentifier,
  VariablePatchPayload,
} from '@/shared/api/types'
import {
  getDesktopStatus,
  getSidecarStatus,
  openLogs,
  readLogs,
  restartSidecar,
  setAutostart,
  setMinimizeToTray,
  type SidecarState,
} from '@/shared/desktop/desktopBridge'
import {
  createDevice,
  createGatewayConfig,
  createVariable,
  discoverGatewayVariables,
  getAuditLogs,
  getDetectionStandard,
  getDetectionStandards,
  getDevices,
  getGatewayConfigs,
  getGateways,
  getProjectMembers,
  getVariables,
  getDatabaseConfig,
  createDetectionStandard,
  deleteDetectionStandard,
  createStorageRoute,
  deleteStorageRoute,
  replaceDetectionStandardItems,
  replaceProjectMembers,
  testDatabaseConfig,
  updateGatewayConfig,
  updateDatabaseConfig,
  assignVariable,
  bulkRemapKioProjects,
  getStorageRoutes,
  updateDetectionStandard,
  updateStorageRoute,
  updateVariable,
} from '@/features/edge-status/api'
import './settings.css'

type GatewayFormValues = GatewayConfigPayload
type UserFormValues = Partial<UserCreatePayload> & Pick<UserCreatePayload, 'username' | 'role'>
type PasswordFormValues = UserResetPasswordPayload
type VariableEditFormValues = VariablePatchPayload
type VariableAssignFormValues = Omit<VariableAssignmentPayload, 'device_id' | 'device_code'> & { project_id: number }
type VirtualVariableFormValues = Omit<VariableCreatePayload, 'device_id' | 'device_code'> & { project_id: number }
type DetectionStandardFormValues = DetectionStandardPayload
type StorageRouteFormValues = StorageRoutePayload
type DatabaseConfigFormValues = DatabaseConfigPayload
type KioProjectRemapFormValues = {
  project_count?: number
  project_code_prefix?: string
  project_display_prefix?: string
  project_en_prefix?: string
  project_ja_prefix?: string
  raw_project_prefix?: string
  var_group?: string
  var_name_prefix?: string
  remap_var_name?: boolean
  enable?: boolean
}
type VariableFilter = 'all' | 'known' | 'unknown' | number
type StorageRouteStatusFilter = 'all' | 'enabled' | 'disabled'
type SettingsModule = 'variables' | 'standards' | 'storage' | 'realtime' | 'history' | 'system' | 'users'
const UNASSIGNED_PAGE_SIZE = 48

type ProjectMemberDraft = ProjectMemberUpdate & {
  draft_key: string
}

type DatabaseTestFeedback = {
  ok: boolean
  message: string
}

const sidecarTagColor: Record<SidecarState, string> = {
  online: 'success',
  starting: 'processing',
  unhealthy: 'warning',
  stopped: 'default',
  missing: 'error',
  offline: 'error',
  failed: 'error',
  unavailable: 'default',
}

function gatewayStatusFor(config: GatewayConfig, statuses: Record<string, GatewayStatus> | undefined) {
  return statuses?.[String(config.id)] ?? Object.values(statuses ?? {}).find((item) => item.client_id === config.client_id)
}

function variableTitle(variable: Pick<VariableConfig, 'display_name' | 'display_name_en' | 'display_name_ja' | 'raw_name' | 'var_name'>, language?: string) {
  if (language === 'en') return variable.display_name_en || variable.display_name || variable.raw_name || variable.var_name
  if (language === 'ja') return variable.display_name_ja || variable.display_name || variable.raw_name || variable.var_name
  return variable.display_name || variable.raw_name || variable.var_name
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

function variableProjectId(variable: Pick<VariableConfig, 'project_id' | 'device_id'>) {
  return variable.project_id ?? variable.device_id
}

function variableProjectCode(variable: Pick<VariableConfig, 'project_code' | 'device_code'>) {
  return variable.project_code || variable.device_code
}

function standardProjectId(standard: Pick<DetectionStandard, 'project_id' | 'device_id'>) {
  return standard.project_id ?? standard.device_id
}

function standardProjectCode(standard: Pick<DetectionStandard, 'project_code' | 'device_code'>) {
  return standard.project_code || standard.device_code
}

function parseAuditDetail(detail: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(detail)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

function stringFromAuditDetail(value: unknown) {
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}

function standardItemTitle(item: DetectionStandardItemPayload, language?: string) {
  if (language === 'en') return item.display_name_en || item.display_name || item.var_name
  if (language === 'ja') return item.display_name_ja || item.display_name || item.var_name
  return item.display_name || item.var_name
}

function normalizeDatabasePayload(values: DatabaseConfigFormValues): DatabaseConfigPayload {
  const payload: DatabaseConfigPayload = {
    host: values.host?.trim(),
    port: values.port,
    user: values.user?.trim(),
    name: values.name?.trim(),
    auto_migrate: values.auto_migrate,
  }
  if (values.password !== undefined && values.password !== '') {
    payload.password = values.password
  }
  return payload
}

function normalizeVariableWritePayload<T extends VariableEditFormValues | VirtualVariableFormValues>(values: T) {
  return {
    ...values,
    writable: values.writable ?? false,
    rw_mode: values.rw_mode || 'R',
    write_requires_audit: values.write_requires_audit ?? true,
    default_alarm_enabled: values.default_alarm_enabled ?? false,
    default_limit_deadband: values.default_limit_deadband ?? 0,
    default_violation_hold_ms: values.default_violation_hold_ms ?? 0,
    default_recover_hold_ms: values.default_recover_hold_ms ?? 0,
  }
}

export function SettingsPage() {
  const { t, i18n } = useTranslation()
  const [messageApi, contextHolder] = message.useMessage()
  const currentUser = useAuthStore((state) => state.user)
  const hasPermission = useAuthStore((state) => state.hasPermission)
  const canManageUsers = hasPermission('manage_users')
  const canUseSystemSettings = hasPermission('system_settings')
  const [activeModule, setActiveModule] = useState<SettingsModule>('variables')
  const [variableFilter, setVariableFilter] = useState<VariableFilter>('all')
  const [variableKeyword, setVariableKeyword] = useState('')
  const [gatewayModalOpen, setGatewayModalOpen] = useState(false)
  const [editingGateway, setEditingGateway] = useState<GatewayConfig | undefined>()
  const [deviceModalOpen, setDeviceModalOpen] = useState(false)
  const [userModalOpen, setUserModalOpen] = useState(false)
  const [editingUser, setEditingUser] = useState<SystemUser | undefined>()
  const [passwordModalOpen, setPasswordModalOpen] = useState(false)
  const [passwordUser, setPasswordUser] = useState<SystemUser | undefined>()
  const [memberProjectId, setMemberProjectId] = useState<number | undefined>()
  const [memberDrafts, setMemberDrafts] = useState<ProjectMemberDraft[]>([])
  const [memberUserIdToAdd, setMemberUserIdToAdd] = useState<number | undefined>()
  const [variableModalOpen, setVariableModalOpen] = useState(false)
  const [selectedVariable, setSelectedVariable] = useState<VariableConfig | undefined>()
  const [selectedUnassignedIds, setSelectedUnassignedIds] = useState<VarIdentifier[]>([])
  const [unassignedPage, setUnassignedPage] = useState(1)
  const [batchAssignModalOpen, setBatchAssignModalOpen] = useState(false)
  const [virtualVariableModalOpen, setVirtualVariableModalOpen] = useState(false)
  const [kioRemapModalOpen, setKioRemapModalOpen] = useState(false)
  const [kioRemapResult, setKioRemapResult] = useState<BulkRemapKioProjectsResult | undefined>()
  const [standardModalOpen, setStandardModalOpen] = useState(false)
  const [editingStandard, setEditingStandard] = useState<DetectionStandard | undefined>()
  const [standardItems, setStandardItems] = useState<DetectionStandardItemPayload[]>([])
  const [standardVariableId, setStandardVariableId] = useState<VarIdentifier | undefined>()
  const [storageRouteModalOpen, setStorageRouteModalOpen] = useState(false)
  const [editingStorageRoute, setEditingStorageRoute] = useState<StorageRoute | undefined>()
  const [storageRouteSearch, setStorageRouteSearch] = useState('')
  const [storageRouteStatus, setStorageRouteStatus] = useState<StorageRouteStatusFilter>('all')
  const [runtimeLogModalOpen, setRuntimeLogModalOpen] = useState(false)
  const [databaseTestFeedback, setDatabaseTestFeedback] = useState<DatabaseTestFeedback | undefined>()
  const [databaseRestartRequired, setDatabaseRestartRequired] = useState(false)
  const [gatewayForm] = Form.useForm<GatewayFormValues>()
  const [deviceForm] = Form.useForm<Pick<Device, 'device_code' | 'name' | 'site_no' | 'model_name'>>()
  const [userForm] = Form.useForm<UserFormValues>()
  const [passwordForm] = Form.useForm<PasswordFormValues>()
  const [variableEditForm] = Form.useForm<VariableEditFormValues>()
  const [variableAssignForm] = Form.useForm<VariableAssignFormValues>()
  const [batchAssignForm] = Form.useForm<VariableAssignFormValues>()
  const [virtualVariableForm] = Form.useForm<VirtualVariableFormValues>()
  const [kioRemapForm] = Form.useForm<KioProjectRemapFormValues>()
  const [standardForm] = Form.useForm<DetectionStandardFormValues>()
  const [storageRouteForm] = Form.useForm<StorageRouteFormValues>()
  const [databaseConfigForm] = Form.useForm<DatabaseConfigFormValues>()

  const gatewaysQuery = useQuery({
    queryKey: ['settings', 'gateway-configs'],
    queryFn: getGatewayConfigs,
    refetchInterval: 30000,
    retry: false,
  })
  const gatewayStatusQuery = useQuery({
    queryKey: ['settings', 'gateway-status'],
    queryFn: getGateways,
    refetchInterval: 30000,
    retry: false,
  })
  const devicesQuery = useQuery({
    queryKey: ['settings', 'devices'],
    queryFn: getDevices,
    refetchInterval: 8000,
    retry: false,
  })
  const variablesQuery = useQuery({
    queryKey: ['settings', 'variables', variableKeyword],
    queryFn: () => getVariables({ keyword: variableKeyword || undefined }),
    refetchInterval: 8000,
    retry: false,
  })
  const standardsQuery = useQuery({
    queryKey: ['settings', 'detection-standards'],
    queryFn: () => getDetectionStandards(),
    refetchInterval: 15000,
    retry: false,
  })
  const storageRoutesQuery = useQuery({
    queryKey: ['settings', 'storage-routes'],
    queryFn: () => getStorageRoutes(),
    enabled: activeModule === 'storage',
    refetchInterval: 15000,
    retry: false,
  })
  const databaseConfigQuery = useQuery({
    queryKey: ['settings', 'database-config'],
    queryFn: getDatabaseConfig,
    enabled: canUseSystemSettings,
    retry: false,
  })

  useEffect(() => {
    if (!databaseConfigQuery.data) return
    databaseConfigForm.setFieldsValue({
      host: databaseConfigQuery.data.host,
      port: databaseConfigQuery.data.port,
      user: databaseConfigQuery.data.user,
      password: '',
      name: databaseConfigQuery.data.name,
      auto_migrate: databaseConfigQuery.data.auto_migrate,
    })
  }, [databaseConfigForm, databaseConfigQuery.data])

  const desktopStatusQuery = useQuery({
    queryKey: ['desktop', 'status'],
    queryFn: getDesktopStatus,
    refetchInterval: 5000,
  })
  const sidecarQuery = useQuery({
    queryKey: ['desktop', 'sidecar'],
    queryFn: getSidecarStatus,
    refetchInterval: 2500,
  })
  const usersQuery = useQuery({
    queryKey: ['settings', 'users'],
    queryFn: getUsers,
    enabled: canManageUsers,
    refetchInterval: 30000,
    retry: false,
  })
  const devices = useMemo(() => devicesQuery.data ?? [], [devicesQuery.data])
  const users = useMemo(() => usersQuery.data ?? [], [usersQuery.data])
  const activeMemberProjectId = memberProjectId ?? devices[0]?.id
  const projectMembersQuery = useQuery({
    queryKey: ['settings', 'project-members', activeMemberProjectId],
    queryFn: () => getProjectMembers(activeMemberProjectId ?? 0),
    enabled: canManageUsers && activeModule === 'users' && Boolean(activeMemberProjectId),
    retry: false,
  })

  useEffect(() => {
    if (!projectMembersQuery.data) return
    const timer = window.setTimeout(() => {
      const members = projectMembersQuery.data.items ?? []
      setMemberDrafts(members.map((member) => ({
        draft_key: String(member.user_id),
        user_id: member.user_id,
        member_role: member.member_role || 'member',
        notify_enabled: member.notify_enabled,
      })))
      setMemberUserIdToAdd(undefined)
    }, 0)
    return () => window.clearTimeout(timer)
  }, [projectMembersQuery.data])

  const gateways = gatewaysQuery.data ?? []
  const variables = useMemo(() => variablesQuery.data ?? [], [variablesQuery.data])
  const standards = standardsQuery.data ?? []
  const storageRoutes = useMemo(() => storageRoutesQuery.data ?? [], [storageRoutesQuery.data])
  const desktopStatus = desktopStatusQuery.data
  const sidecarStatus = sidecarQuery.data
  const sidecarState = sidecarStatus?.state ?? 'unavailable'
  const backendUnavailable =
    gatewaysQuery.isError ||
    gatewayStatusQuery.isError ||
    devicesQuery.isError ||
    variablesQuery.isError ||
    standardsQuery.isError ||
    storageRoutesQuery.isError ||
    (canManageUsers && usersQuery.isError)

  const displayDeviceName = (device: Device) => {
    if (i18n.resolvedLanguage === 'en') return device.display_name_en || device.display_name || device.name || device.device_code
    if (i18n.resolvedLanguage === 'ja') return device.display_name_ja || device.display_name || device.name || device.device_code
    return device.display_name || device.name || device.device_code
  }

  const filteredVariables = useMemo(() => {
    return variables.filter((variable) => {
      if (variableFilter === 'all') return true
      if (variableFilter === 'known') return Boolean(variableProjectId(variable))
      if (variableFilter === 'unknown') return !variableProjectId(variable)
      return variableProjectId(variable) === variableFilter
    })
  }, [variableFilter, variables])
  const isUnassignedView = variableFilter === 'unknown'
  const unassignedVariables = useMemo(
    () => filteredVariables.filter((variable) => !variableProjectId(variable)),
    [filteredVariables],
  )
  const selectedUnassignedVariables = useMemo(
    () => unassignedVariables.filter((variable) => selectedUnassignedIds.some((id) => sameVarId(id, variableWireId(variable)))),
    [selectedUnassignedIds, unassignedVariables],
  )
  const selectedUnassignedIdSet = useMemo(() => new Set(selectedUnassignedIds.map(varKey)), [selectedUnassignedIds])
  const unassignedPageCount = Math.max(1, Math.ceil(unassignedVariables.length / UNASSIGNED_PAGE_SIZE))
  const safeUnassignedPage = Math.min(unassignedPage, unassignedPageCount)
  const visibleUnassignedVariables = useMemo(() => {
    const start = (safeUnassignedPage - 1) * UNASSIGNED_PAGE_SIZE
    return unassignedVariables.slice(start, start + UNASSIGNED_PAGE_SIZE)
  }, [safeUnassignedPage, unassignedVariables])
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
  const assignedVariables = useMemo(() => variables.filter((variable) => variableProjectId(variable)), [variables])
  const variableById = useMemo(() => {
    const entries: Array<[string, VariableConfig]> = []
    variables.forEach((variable) => {
      entries.push([String(variable.var_id), variable])
      if (variable.var_id_text) entries.push([variable.var_id_text, variable])
    })
    return new Map(entries)
  }, [variables])
  const deviceById = useMemo(() => new Map(devices.map((device) => [device.id, device])), [devices])
  const userById = useMemo(() => new Map(users.map((user) => [user.id, user])), [users])
  const memberProject = activeMemberProjectId ? deviceById.get(activeMemberProjectId) : undefined
  const projectMemberRows = useMemo(() => memberDrafts.map((draft) => ({
    ...draft,
    user: userById.get(draft.user_id),
  })), [memberDrafts, userById])
  const availableMemberUsers = useMemo(
    () => users.filter((user) => !memberDrafts.some((member) => member.user_id === user.id)),
    [memberDrafts, users],
  )
  const filteredStorageRoutes = useMemo(() => {
    const keyword = storageRouteSearch.trim().toLowerCase()
    return storageRoutes.filter((route) => {
      if (storageRouteStatus === 'enabled' && !route.enabled) return false
      if (storageRouteStatus === 'disabled' && route.enabled) return false
      if (!keyword) return true
      const variable = variableById.get(route.var_id_text ?? String(route.var_id))
      const project = deviceById.get(route.project_id)
      const haystack = [
        route.route_code,
        route.storage_target,
        route.table_name,
        route.column_name,
        route.column_type,
        route.trigger_mode,
        route.query_alias,
        route.form_field_key,
        variable?.var_name,
        variable?.raw_name,
        variable?.display_name,
        variable?.display_name_en,
        variable?.display_name_ja,
        project?.device_code,
        project?.display_name,
        project?.display_name_en,
        project?.display_name_ja,
        project?.name,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return haystack.includes(keyword)
    })
  }, [deviceById, storageRouteSearch, storageRouteStatus, storageRoutes, variableById])

  const saveGatewayMutation = useMutation({
    mutationFn: (payload: GatewayFormValues) => {
      if (editingGateway) return updateGatewayConfig(editingGateway.id, payload)
      return createGatewayConfig(payload)
    },
    onSuccess: async () => {
      setGatewayModalOpen(false)
      setEditingGateway(undefined)
      messageApi.success(t('settings.messages.gatewaySaved'))
      await queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const createDeviceMutation = useMutation({
    mutationFn: (values: Pick<Device, 'device_code' | 'name' | 'site_no' | 'model_name'>) =>
      createDevice({ ...values, placeholder: false }),
    onSuccess: async () => {
      setDeviceModalOpen(false)
      deviceForm.resetFields()
      messageApi.success(t('settings.messages.deviceCreated'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'devices'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const toggleVariableMutation = useMutation({
    mutationFn: (variable: VariableConfig) => updateVariable(variableWireId(variable), { enabled: !variable.enabled }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveVariableMutation = useMutation({
    mutationFn: (values: VariableEditFormValues) => {
      if (!selectedVariable) throw new Error(t('settings.variables.noVariable'))
      const payload = normalizeVariableWritePayload(values)
      if (payload.writable && payload.rw_mode !== 'W' && payload.rw_mode !== 'RW') {
        throw new Error(t('settings.variables.writeModeRequired'))
      }
      if (payload.writable && !payload.write_path?.trim()) {
        throw new Error(t('settings.variables.writePathRequired'))
      }
      if (payload.writable && !payload.write_data_type?.trim()) {
        throw new Error(t('settings.variables.writeDataTypeRequired'))
      }
      return updateVariable(variableWireId(selectedVariable), payload)
    },
    onSuccess: async () => {
      setVariableModalOpen(false)
      setSelectedVariable(undefined)
      variableEditForm.resetFields()
      messageApi.success(t('settings.messages.variableSaved'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const createVirtualVariableMutation = useMutation({
    mutationFn: (values: VirtualVariableFormValues) => {
      const project = devices.find((item) => item.id === values.project_id)
      if (!project) throw new Error(t('settings.messages.selectVariableDevice'))
      const payload = normalizeVariableWritePayload(values)
      if (payload.writable && payload.rw_mode !== 'W' && payload.rw_mode !== 'RW') {
        throw new Error(t('settings.variables.writeModeRequired'))
      }
      if (payload.writable && !payload.write_path?.trim()) {
        throw new Error(t('settings.variables.writePathRequired'))
      }
      if (payload.writable && !payload.write_data_type?.trim()) {
        throw new Error(t('settings.variables.writeDataTypeRequired'))
      }
      return createVariable({
        ...payload,
        display_name_en: payload.display_name_en || payload.display_name,
        display_name_ja: payload.display_name_ja || payload.display_name,
        source_type: 'virtual',
        gateway_id: 0,
        source_topic: 'virtual',
        source_path: payload.var_name,
        raw_name: payload.var_name,
        json_path: payload.var_name,
        project_id: project.id,
        project_code: project.device_code,
        var_group: '',
        scale_factor: payload.scale_factor ?? 1,
        offset_val: payload.offset_val ?? 0,
        enabled: payload.enabled ?? true,
      })
    },
    onSuccess: async () => {
      setVirtualVariableModalOpen(false)
      virtualVariableForm.resetFields()
      messageApi.success(t('settings.messages.virtualVariableCreated'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const kioRemapMutation = useMutation({
    mutationFn: ({ values, dryRun }: { values: KioProjectRemapFormValues; dryRun: boolean }) =>
      bulkRemapKioProjects({
        ...values,
        project_count: values.project_count ?? 12,
        remap_var_name: values.remap_var_name ?? true,
        enable: values.enable ?? true,
        dry_run: dryRun,
      }),
    onSuccess: async (result) => {
      setKioRemapResult(result)
      messageApi.success(result.dry_run ? t('settings.variables.kioRemapDryRunDone') : t('settings.variables.kioRemapDone', { count: result.updated }))
      if (!result.dry_run) {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['settings', 'devices'] }),
          queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] }),
          queryClient.invalidateQueries({ queryKey: ['settings', 'storage-routes'] }),
        ])
      }
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const assignVariableMutation = useMutation({
    mutationFn: (values: VariableAssignFormValues) => {
      if (!selectedVariable) throw new Error(t('settings.variables.noVariable'))
      const project = devices.find((item) => item.id === values.project_id)
      if (!project) throw new Error(t('settings.messages.selectVariableDevice'))
      return assignVariable(variableWireId(selectedVariable), {
        project_id: project.id,
        project_code: project.device_code,
        var_group: values.var_group ?? '',
        enabled: values.enabled,
      })
    },
    onSuccess: async () => {
      setVariableModalOpen(false)
      setSelectedVariable(undefined)
      variableAssignForm.resetFields()
      messageApi.success(t('settings.messages.variableAssigned'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const batchAssignVariableMutation = useMutation({
    mutationFn: async (values: VariableAssignFormValues) => {
      const project = devices.find((item) => item.id === values.project_id)
      if (!project || selectedUnassignedVariables.length === 0) throw new Error(t('settings.messages.selectVariableDevice'))
      await Promise.all(
        selectedUnassignedVariables.map((variable) =>
          assignVariable(variableWireId(variable), {
            project_id: project.id,
            project_code: project.device_code,
            var_group: values.var_group ?? '',
            enabled: values.enabled,
          }),
        ),
      )
      return selectedUnassignedVariables.length
    },
    onSuccess: async (count) => {
      setBatchAssignModalOpen(false)
      setSelectedUnassignedIds([])
      batchAssignForm.resetFields()
      messageApi.success(t('settings.messages.variablesAssigned', { count }))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveStandardMutation = useMutation({
    mutationFn: async (values: DetectionStandardFormValues) => {
      const project = devices.find((item) => item.id === values.project_id)
      const payload: DetectionStandardPayload = {
        ...values,
        project_code: project?.device_code ?? values.project_code ?? '',
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
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const deleteStandardMutation = useMutation({
    mutationFn: (standard: DetectionStandard) => deleteDetectionStandard(standard.id),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.standardDeleted'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'detection-standards'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveStorageRouteMutation = useMutation({
    mutationFn: (values: StorageRouteFormValues) => {
      if (editingStorageRoute) return updateStorageRoute(editingStorageRoute.id, values)
      return createStorageRoute(values)
    },
    onSuccess: async () => {
      setStorageRouteModalOpen(false)
      setEditingStorageRoute(undefined)
      storageRouteForm.resetFields()
      messageApi.success(t('settings.messages.storageRouteSaved'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'storage-routes'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const deleteStorageRouteMutation = useMutation({
    mutationFn: (route: StorageRoute) => deleteStorageRoute(route.id),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.storageRouteDeleted'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'storage-routes'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const discoverMutation = useMutation({
    mutationFn: (gatewayId: number) => discoverGatewayVariables(gatewayId),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.discoverRequested'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'variables'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const testDatabaseMutation = useMutation({
    mutationFn: (values: DatabaseConfigFormValues) => testDatabaseConfig(normalizeDatabasePayload(values)),
    onSuccess: (result) => {
      if (result.ok) {
        setDatabaseTestFeedback({ ok: true, message: t('settings.historySource.testSuccess') })
        messageApi.success(t('settings.historySource.testSuccess'))
        return
      }
      const errorMessage = result.error || t('settings.historySource.testFailed')
      setDatabaseTestFeedback({ ok: false, message: errorMessage })
      messageApi.error(errorMessage)
    },
    onError: (error) => {
      const errorMessage = error instanceof Error ? error.message : t('settings.historySource.testFailed')
      setDatabaseTestFeedback({ ok: false, message: errorMessage })
      messageApi.error(errorMessage)
    },
  })

  const saveDatabaseMutation = useMutation({
    mutationFn: (values: DatabaseConfigFormValues) => updateDatabaseConfig(normalizeDatabasePayload(values)),
    onSuccess: async (config) => {
      setDatabaseRestartRequired(config.restart_required)
      messageApi.success(t('settings.historySource.saveSuccess'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'database-config'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveUserMutation = useMutation({
    mutationFn: (values: UserFormValues) => {
      if (editingUser) {
        const payload: UserPatchPayload = {
          username: values.username,
          role: values.role,
          enabled: values.enabled,
        }
        return updateUser(editingUser.id, payload)
      }
      return createUser({
        username: values.username,
        password: values.password ?? '',
        role: values.role,
        enabled: values.enabled ?? true,
      })
    },
    onSuccess: async () => {
      setUserModalOpen(false)
      setEditingUser(undefined)
      userForm.resetFields()
      messageApi.success(t('settings.messages.userSaved'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'users'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const resetPasswordMutation = useMutation({
    mutationFn: (values: PasswordFormValues) => {
      if (!passwordUser) throw new Error(t('settings.users.noSelection'))
      return resetUserPassword(passwordUser.id, values)
    },
    onSuccess: async () => {
      setPasswordModalOpen(false)
      setPasswordUser(undefined)
      passwordForm.resetFields()
      messageApi.success(t('settings.messages.passwordReset'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'users'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const toggleUserMutation = useMutation({
    mutationFn: (user: SystemUser) => updateUser(user.id, { enabled: !user.enabled }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings', 'users'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const deleteUserMutation = useMutation({
    mutationFn: (user: SystemUser) => deleteUser(user.id),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.userDeleted'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'users'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const saveProjectMembersMutation = useMutation({
    mutationFn: () => {
      if (!activeMemberProjectId) throw new Error(t('settings.projectMembers.selectProject'))
      return replaceProjectMembers(activeMemberProjectId, memberDrafts.map((member) => ({
        user_id: member.user_id,
        member_role: member.member_role || 'member',
        notify_enabled: member.notify_enabled ?? true,
      })))
    },
    onSuccess: async () => {
      messageApi.success(t('settings.messages.projectMembersSaved'))
      await queryClient.invalidateQueries({ queryKey: ['settings', 'project-members', activeMemberProjectId] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const setAutostartMutation = useMutation({
    mutationFn: (enabled: boolean) => setAutostart(enabled),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.desktopSaved'))
      await queryClient.invalidateQueries({ queryKey: ['desktop', 'status'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const setMinimizeToTrayMutation = useMutation({
    mutationFn: (enabled: boolean) => setMinimizeToTray(enabled),
    onSuccess: async () => {
      messageApi.success(t('settings.messages.desktopSaved'))
      await queryClient.invalidateQueries({ queryKey: ['desktop', 'status'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const restartSidecarMutation = useMutation({
    mutationFn: restartSidecar,
    onSuccess: async () => {
      messageApi.success(t('settings.messages.sidecarRestarted'))
      await queryClient.invalidateQueries({ queryKey: ['desktop', 'sidecar'] })
    },
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const openLogsMutation = useMutation({
    mutationFn: openLogs,
    onError: (error) => messageApi.error(error instanceof Error ? error.message : t('messages.noData')),
  })

  const runtimeLogsQuery = useQuery({
    queryKey: ['settings', 'runtime-logs'],
    queryFn: () => readLogs({ maxBytes: 64000 }),
    enabled: activeModule === 'system',
    retry: false,
  })

  const auditLogsQuery = useQuery({
    queryKey: ['settings', 'audit-logs'],
    queryFn: () => getAuditLogs({ limit: 80 }),
    enabled: activeModule === 'system' && canUseSystemSettings,
    refetchInterval: 30000,
    retry: false,
  })

  function openGatewayModal(gateway?: GatewayConfig) {
    setEditingGateway(gateway)
    gatewayForm.setFieldsValue(
      gateway ?? {
        name: '',
        broker: 'tcp://127.0.0.1:1883',
        client_id: 'edge-local-kio',
        topic: 'datachange_S_KIO_Project',
        qos: 2,
        parser_type: 'kingiot_kio',
        enabled: true,
      },
    )
    setGatewayModalOpen(true)
  }

  function openUserModal(user?: SystemUser) {
    setEditingUser(user)
    userForm.setFieldsValue(
      user
        ? { username: user.username, role: user.role as RoleName, enabled: user.enabled }
        : { username: '', role: 'guest', enabled: true, password: '' },
    )
    setUserModalOpen(true)
  }

  function openPasswordModal(user: SystemUser) {
    setPasswordUser(user)
    passwordForm.resetFields()
    setPasswordModalOpen(true)
  }

  function addProjectMember() {
    if (!memberUserIdToAdd || memberDrafts.some((member) => member.user_id === memberUserIdToAdd)) return
    setMemberDrafts((items) => [
      ...items,
      {
        draft_key: String(memberUserIdToAdd),
        user_id: memberUserIdToAdd,
        member_role: 'member',
        notify_enabled: true,
      },
    ])
    setMemberUserIdToAdd(undefined)
  }

  function patchProjectMember(userId: number, patch: Partial<ProjectMemberUpdate>) {
    setMemberDrafts((items) => items.map((member) => member.user_id === userId ? { ...member, ...patch } : member))
  }

  function removeProjectMember(userId: number) {
    setMemberDrafts((items) => items.filter((member) => member.user_id !== userId))
  }

  function openVariableModal(variable: VariableConfig) {
    setSelectedVariable(variable)
    if (variableProjectId(variable)) {
      variableEditForm.setFieldsValue({
        var_name: variable.var_name,
        display_name: variable.display_name,
        display_name_en: variable.display_name_en,
        display_name_ja: variable.display_name_ja,
        data_type: variable.data_type,
        unit: variable.unit,
        decimal_places: variable.decimal_places,
        scale_factor: variable.scale_factor,
        offset_val: variable.offset_val,
        rw_mode: variable.rw_mode || 'R',
        writable: variable.writable,
        write_source_id: variable.write_source_id,
        write_path: variable.write_path,
        write_data_type: variable.write_data_type,
        write_min: variable.write_min,
        write_max: variable.write_max,
        write_enum: variable.write_enum,
        write_requires_audit: variable.write_requires_audit,
        suspicious_value: variable.suspicious_value,
        debounce_threshold: variable.debounce_threshold,
        debounce_ms: variable.debounce_ms,
        deadband: variable.deadband,
        default_alarm_enabled: variable.default_alarm_enabled,
        default_limit_ll: variable.default_limit_ll,
        default_limit_l: variable.default_limit_l,
        default_limit_h: variable.default_limit_h,
        default_limit_hh: variable.default_limit_hh,
        default_limit_deadband: variable.default_limit_deadband,
        default_violation_hold_ms: variable.default_violation_hold_ms,
        default_recover_hold_ms: variable.default_recover_hold_ms,
        apply_to_running: false,
        var_group: variable.var_group,
        enabled: variable.enabled,
      })
    } else {
      variableAssignForm.setFieldsValue({
        project_id: undefined as unknown as number,
        project_code: '',
        var_group: '',
        enabled: true,
      })
    }
    setVariableModalOpen(true)
  }

  function toggleUnassignedSelection(variableId: VarIdentifier) {
    setSelectedUnassignedIds((ids) =>
      ids.some((id) => sameVarId(id, variableId)) ? ids.filter((id) => !sameVarId(id, variableId)) : [...ids, variableId],
    )
  }

  function setVariableFilterWithReset(filter: VariableFilter) {
    setVariableFilter(filter)
    setUnassignedPage(1)
  }

  function openBatchAssignModal() {
    batchAssignForm.setFieldsValue({
      project_id: undefined as unknown as number,
      project_code: '',
      var_group: '',
      enabled: true,
    })
    setBatchAssignModalOpen(true)
  }

  function openVirtualVariableModal() {
    virtualVariableForm.setFieldsValue({
      data_type: 'INT',
      decimal_places: 0,
      scale_factor: 1,
      offset_val: 0,
      rw_mode: 'R',
      writable: false,
      write_source_id: 0,
      write_requires_audit: true,
      debounce_ms: 0,
      deadband: 0,
      default_alarm_enabled: false,
      default_limit_deadband: 0,
      default_violation_hold_ms: 0,
      default_recover_hold_ms: 0,
      enabled: true,
    })
    setVirtualVariableModalOpen(true)
  }

  function openKioRemapModal() {
    kioRemapForm.setFieldsValue({
      project_count: 12,
      project_code_prefix: 'AC',
      project_display_prefix: '项目',
      project_en_prefix: 'Project ',
      project_ja_prefix: 'プロジェクト',
      raw_project_prefix: '台',
      var_group: 'KIO变量',
      var_name_prefix: 'kio',
      remap_var_name: true,
      enable: true,
    })
    setKioRemapResult(undefined)
    setKioRemapModalOpen(true)
  }

  async function submitKioRemap(dryRun: boolean) {
    const values = await kioRemapForm.validateFields()
    kioRemapMutation.mutate({ values, dryRun })
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
        mode: detail.mode,
        version: detail.version,
        enabled: detail.enabled,
        remark: detail.remark,
      })
      setStandardItems((detail.items ?? []).map((item) => ({
        var_id: item.var_id_text ?? item.var_id,
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

  function openStorageRouteModal(route?: StorageRoute) {
    setEditingStorageRoute(route)
    if (route) {
      storageRouteForm.setFieldsValue({ ...route, var_id: route.var_id_text ?? route.var_id })
    } else {
      const firstVariable = assignedVariables[0]
      const firstProjectId = firstVariable ? variableProjectId(firstVariable) : undefined
      const firstDevice = firstProjectId ? deviceById.get(firstProjectId) : devices[0]
      storageRouteForm.setFieldsValue({
        project_id: firstDevice?.id,
        var_id: firstVariable ? variableWireId(firstVariable) : undefined,
        route_code: `route-${Date.now().toString().slice(-6)}`,
        storage_target: 'wide_table',
        table_name: firstDevice ? `rt_project_${firstDevice.id}_data` : '',
        column_name: firstVariable?.var_name ?? '',
        column_type: 'DOUBLE',
        form_field_key: firstVariable?.var_name ?? '',
        query_alias: firstVariable?.var_name ?? '',
        trigger_mode: 'on_cycle',
        cycle_ms: 60000,
        deadband: 0,
        store_on_start: false,
        enabled: true,
      })
    }
    setStorageRouteModalOpen(true)
  }

  function addStandardItem(variableId?: VarIdentifier) {
    if (!variableId) return
    const variable = standardVariables.find((item) => sameVarId(variableWireId(item), variableId))
    if (!variable || standardItems.some((item) => item.var_name === variable.var_name)) return
    setStandardItems((items) => [
      ...items,
      {
        var_id: variableWireId(variable),
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

  function patchStandardItem(varId: VarIdentifier, patch: Partial<DetectionStandardItemPayload>) {
    setStandardItems((items) => items.map((item) => sameVarId(item.var_id, varId) ? { ...item, ...patch } : item))
  }

  function removeStandardItem(varId: VarIdentifier) {
    setStandardItems((items) => items.filter((item) => !sameVarId(item.var_id, varId)).map((item, index) => ({ ...item, sort_order: index + 1 })))
  }

  const roleOptions = [
    { label: t('settings.users.roles.guest'), value: 'guest' },
    { label: t('settings.users.roles.admin'), value: 'admin' },
    { label: t('settings.users.roles.developer'), value: 'developer' },
  ]

  const memberRoleOptions = [
    { label: t('settings.projectMembers.roles.owner'), value: 'owner' },
    { label: t('settings.projectMembers.roles.operator'), value: 'operator' },
    { label: t('settings.projectMembers.roles.member'), value: 'member' },
    { label: t('settings.projectMembers.roles.viewer'), value: 'viewer' },
  ]

  const variableColumns: TableColumnsType<VariableConfig> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      fixed: 'left',
      render: (_, record) => (
        <div className="settings-variable-name">
          <strong>{variableTitle(record, i18n.resolvedLanguage)}</strong>
          <span>{record.var_name}</span>
        </div>
      ),
    },
    { title: 'Var ID', dataIndex: 'var_id', key: 'var_id', width: 120 },
    { title: t('settings.variables.rawName'), dataIndex: 'raw_name', key: 'raw_name', width: 180 },
    { title: t('settings.variables.path'), dataIndex: 'source_path', key: 'source_path', width: 260 },
    { title: t('settings.variables.jsonPath'), dataIndex: 'json_path', key: 'json_path', width: 220 },
    { title: 'Gateway', dataIndex: 'gateway_id', key: 'gateway_id', width: 90 },
    {
      title: t('settings.variables.type'),
      dataIndex: 'data_type',
      key: 'data_type',
      width: 90,
      render: (value, record) => variableProjectId(record) ? value : <span className="settings-muted">{t('settings.variables.readonly')}</span>,
    },
    {
      title: t('settings.variables.unit'),
      dataIndex: 'unit',
      key: 'unit',
      width: 90,
      render: (value, record) => variableProjectId(record) ? value : <span className="settings-muted">-</span>,
    },
    {
      title: t('settings.variables.device'),
      dataIndex: 'project_code',
      key: 'project_code',
      width: 140,
      render: (_, record) => variableProjectId(record) ? variableProjectCode(record) : <Tag>{t('settings.variables.unassigned')}</Tag>,
    },
    {
      title: t('settings.variables.group'),
      dataIndex: 'var_group',
      key: 'var_group',
      width: 130,
      render: (value, record) => variableProjectId(record) ? value : <span className="settings-muted">-</span>,
    },
    {
      title: t('settings.variables.writeMode'),
      dataIndex: 'rw_mode',
      key: 'rw_mode',
      width: 120,
      render: (value, record) => variableProjectId(record) ? (
        <Tag color={record.writable ? 'processing' : 'default'}>{record.writable ? value : 'R'}</Tag>
      ) : <span className="settings-muted">-</span>,
    },
    {
      title: t('settings.variables.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      fixed: 'right',
      render: (_, record) => (
        variableProjectId(record) ? (
          <Switch
            size="small"
            checked={record.enabled}
            loading={toggleVariableMutation.isPending}
            onChange={() => toggleVariableMutation.mutate(record)}
          />
        ) : <span className="settings-muted">{t('settings.variables.afterAssign')}</span>
      ),
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 140,
      fixed: 'right',
      render: (_, record) => (
        <Button
          size="small"
          icon={variableProjectId(record) ? <Edit3 size={13} /> : <SlidersHorizontal size={13} />}
          onClick={() => openVariableModal(record)}
        >
          {variableProjectId(record) ? t('settings.variables.edit') : t('settings.variables.assign')}
        </Button>
      ),
    },
  ]

  const userColumns: TableColumnsType<SystemUser> = [
    {
      title: t('settings.users.username'),
      dataIndex: 'username',
      key: 'username',
      width: 190,
      render: (_, record) => (
        <div className="settings-user-name">
          <strong>{record.username}</strong>
          <span>ID {record.id}</span>
        </div>
      ),
    },
    {
      title: t('settings.users.role'),
      dataIndex: 'role',
      key: 'role',
      width: 150,
      render: (role: string) => (
        <Tag className="settings-role-tag" icon={<ShieldCheck size={12} />}>
          {t(`settings.users.roles.${role}`, { defaultValue: role })}
        </Tag>
      ),
    },
    {
      title: t('settings.users.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 110,
      render: (_, record) => (
        <Switch
          size="small"
          checked={record.enabled}
          disabled={record.id === currentUser?.id}
          loading={toggleUserMutation.isPending}
          onChange={() => toggleUserMutation.mutate(record)}
        />
      ),
    },
    {
      title: t('settings.users.permissionsVersion'),
      dataIndex: 'permissions_version',
      key: 'permissions_version',
      width: 130,
    },
    {
      title: t('settings.users.permissions'),
      dataIndex: 'permissions',
      key: 'permissions',
      width: 340,
      render: (permissions: string[]) => (
        <div className="settings-permission-tags">
          {permissions.map((permission) => (
            <Tag key={permission}>{permission}</Tag>
          ))}
        </div>
      ),
    },
    {
      title: t('settings.users.lastLogin'),
      dataIndex: 'last_login_at',
      key: 'last_login_at',
      width: 180,
      render: (value?: string | null) => value ? new Date(value).toLocaleString() : t('settings.users.neverLogin'),
    },
    {
      title: t('settings.users.createdAt'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 260,
      fixed: 'right',
      render: (_, record) => (
        <div className="settings-user-actions">
          <Button size="small" icon={<UserRound size={13} />} onClick={() => openUserModal(record)}>
            {t('settings.users.edit')}
          </Button>
          <Button size="small" icon={<KeyRound size={13} />} onClick={() => openPasswordModal(record)}>
            {t('settings.users.resetPassword')}
          </Button>
          <Popconfirm
            title={t('settings.users.deleteConfirm', { username: record.username })}
            okText={t('settings.users.delete')}
            cancelText={t('settings.actions.cancel')}
            disabled={record.id === currentUser?.id}
            onConfirm={() => deleteUserMutation.mutate(record)}
          >
            <Button
              danger
              size="small"
              icon={<Trash2 size={13} />}
              aria-label={t('settings.users.delete')}
              disabled={record.id === currentUser?.id}
              loading={deleteUserMutation.isPending}
            />
          </Popconfirm>
        </div>
      ),
    },
  ]

  const projectMemberColumns: TableColumnsType<ProjectMemberDraft & { user?: SystemUser }> = [
    {
      title: t('settings.projectMembers.user'),
      key: 'user',
      width: 220,
      render: (_, record) => (
        <div className="settings-user-name">
          <strong>{record.user?.username ?? `#${record.user_id}`}</strong>
          <span>{record.user ? t(`settings.users.roles.${record.user.role}`, { defaultValue: record.user.role }) : `ID ${record.user_id}`}</span>
        </div>
      ),
    },
    {
      title: t('settings.projectMembers.memberRole'),
      dataIndex: 'member_role',
      key: 'member_role',
      width: 190,
      render: (_, record) => (
        <Select
          size="small"
          value={record.member_role || 'member'}
          options={memberRoleOptions}
          onChange={(value) => patchProjectMember(record.user_id, { member_role: value })}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: t('settings.projectMembers.notifyEnabled'),
      dataIndex: 'notify_enabled',
      key: 'notify_enabled',
      width: 150,
      render: (_, record) => (
        <Switch
          size="small"
          checked={record.notify_enabled ?? true}
          onChange={(checked) => patchProjectMember(record.user_id, { notify_enabled: checked })}
        />
      ),
    },
    {
      title: t('settings.users.enabled'),
      key: 'user_enabled',
      width: 110,
      render: (_, record) => (
        <Tag color={record.user?.enabled ? 'success' : 'default'}>
          {record.user?.enabled ? t('status.online') : t('status.offline')}
        </Tag>
      ),
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 90,
      render: (_, record) => (
        <Button danger size="small" icon={<Trash2 size={13} />} aria-label={t('settings.users.delete')} onClick={() => removeProjectMember(record.user_id)} />
      ),
    },
  ]

  const auditLogColumns: TableColumnsType<AuditLogEntry> = [
    {
      title: t('settings.system.auditTime'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (value: string) => new Date(value).toLocaleString(),
    },
    {
      title: t('settings.system.auditActor'),
      key: 'actor',
      width: 150,
      render: (_, record) => (
        <div className="settings-user-name">
          <strong>{record.actor_type}</strong>
          <span>{record.actor_id || '-'}</span>
        </div>
      ),
    },
    {
      title: t('settings.system.auditAction'),
      dataIndex: 'action',
      key: 'action',
      width: 180,
      render: (value: string) => <Tag color={value.startsWith('auth.') ? 'geekblue' : 'processing'}>{value}</Tag>,
    },
    {
      title: t('settings.system.auditTarget'),
      key: 'target',
      width: 240,
      render: (_, record) => (
        <div className="settings-variable-name">
          <strong>{record.target_type || '-'}</strong>
          <span>{record.target_id || '-'}</span>
        </div>
      ),
    },
    {
      title: t('settings.system.auditResult'),
      dataIndex: 'result',
      key: 'result',
      width: 100,
      render: (value: string) => (
        <Tag color={value === 'success' ? 'success' : 'error'}>
          {t(`settings.system.auditResults.${value}`, { defaultValue: value })}
        </Tag>
      ),
    },
    {
      title: t('settings.system.auditRequest'),
      key: 'request',
      width: 260,
      render: (_, record) => {
        const detail = parseAuditDetail(record.detail)
        const requestId = stringFromAuditDetail(detail.request_id)
        const commandId = stringFromAuditDetail(detail.command_id)
        const status = stringFromAuditDetail(detail.status)
        return (
          <div className="settings-audit-detail">
            <span>{requestId ? `RID ${requestId}` : '-'}</span>
            {commandId ? <span>CMD {commandId}</span> : null}
            {status ? <span>HTTP {status}</span> : null}
          </div>
        )
      },
    },
    {
      title: t('settings.system.auditError'),
      key: 'error',
      width: 260,
      render: (_, record) => {
        const detail = parseAuditDetail(record.detail)
        return stringFromAuditDetail(detail.error) || '-'
      },
    },
  ]

  const standardColumns: TableColumnsType<DetectionStandard> = [
    {
      title: t('settings.standards.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 240,
      render: (_, record) => (
        <div className="settings-variable-name">
          <strong>{record.display_name || record.name || record.standard_code}</strong>
          <span>{record.standard_code}</span>
        </div>
      ),
    },
    {
      title: t('settings.variables.device'),
      dataIndex: 'project_code',
      key: 'project_code',
      width: 150,
      render: (_, record) => standardProjectId(record) ? standardProjectCode(record) : <Tag>{t('settings.standards.global')}</Tag>,
    },
    { title: t('settings.standards.mode'), dataIndex: 'mode', key: 'mode', width: 110 },
    { title: t('settings.standards.version'), dataIndex: 'version', key: 'version', width: 90 },
    {
      title: t('settings.standards.items'),
      dataIndex: 'items',
      key: 'items',
      width: 100,
      render: (items?: DetectionStandard['items']) => items?.length ?? '-',
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
      width: 170,
      fixed: 'right',
      render: (_, record) => (
        <div className="settings-user-actions">
          <Button size="small" icon={<Edit3 size={13} />} onClick={() => void openStandardModal(record)}>
            {t('settings.users.edit')}
          </Button>
          <Popconfirm
            title={t('settings.standards.deleteConfirm', { code: record.standard_code })}
            okText={t('settings.users.delete')}
            cancelText={t('settings.actions.cancel')}
            onConfirm={() => deleteStandardMutation.mutate(record)}
          >
            <Button danger size="small" icon={<Trash2 size={13} />} aria-label={t('settings.users.delete')} loading={deleteStandardMutation.isPending} />
          </Popconfirm>
        </div>
      ),
    },
  ]

  const storageRouteColumns: TableColumnsType<StorageRoute> = [
    {
      title: t('settings.storage.routeCode'),
      dataIndex: 'route_code',
      key: 'route_code',
      width: 180,
      render: (_, record) => {
        const variable = variableById.get(record.var_id_text ?? String(record.var_id))
        return (
          <div className="settings-variable-name">
            <strong>{record.route_code}</strong>
            <span>{variable ? variableTitle(variable, i18n.resolvedLanguage) : `#${record.var_id}`}</span>
          </div>
        )
      },
    },
    {
      title: t('settings.storage.project'),
      dataIndex: 'project_id',
      key: 'project_id',
      width: 160,
      render: (value: number) => {
        const device = deviceById.get(value)
        return device ? displayDeviceName(device) : value
      },
    },
    { title: t('settings.storage.target'), dataIndex: 'storage_target', key: 'storage_target', width: 130 },
    {
      title: t('settings.storage.tableColumn'),
      key: 'tableColumn',
      width: 240,
      render: (_, record) => (
        <div className="settings-variable-name">
          <strong>{record.table_name || '-'}</strong>
          <span>{record.column_name || '-'}</span>
        </div>
      ),
    },
    { title: t('settings.storage.columnType'), dataIndex: 'column_type', key: 'column_type', width: 120 },
    {
      title: t('settings.storage.triggerMode'),
      dataIndex: 'trigger_mode',
      key: 'trigger_mode',
      width: 130,
    },
    {
      title: t('settings.storage.cycleMs'),
      dataIndex: 'cycle_ms',
      key: 'cycle_ms',
      width: 110,
      render: (value: number) => value ? `${value} ms` : '-',
    },
    {
      title: t('settings.storage.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (enabled: boolean) => <Tag color={enabled ? 'success' : 'default'}>{enabled ? t('status.online') : t('status.offline')}</Tag>,
    },
    {
      title: t('settings.users.actions'),
      key: 'actions',
      width: 170,
      fixed: 'right',
      render: (_, record) => (
        <div className="settings-user-actions">
          <Button size="small" icon={<Edit3 size={13} />} onClick={() => openStorageRouteModal(record)}>
            {t('settings.users.edit')}
          </Button>
          <Popconfirm
            title={t('settings.storage.deleteConfirm', { code: record.route_code })}
            okText={t('settings.users.delete')}
            cancelText={t('settings.actions.cancel')}
            onConfirm={() => deleteStorageRouteMutation.mutate(record)}
          >
            <Button danger size="small" icon={<Trash2 size={13} />} aria-label={t('settings.users.delete')} loading={deleteStorageRouteMutation.isPending} />
          </Popconfirm>
        </div>
      ),
    },
  ]

  const standardItemColumns: TableColumnsType<DetectionStandardItemPayload> = [
    {
      title: t('settings.variables.name'),
      dataIndex: 'display_name',
      key: 'display_name',
      width: 220,
      render: (_, record) => (
        <div className="settings-variable-name">
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
      render: (_, record) => <Switch size="small" checked={record.check_enabled ?? true} onChange={(checked) => patchStandardItem(record.var_id, { check_enabled: checked })} />,
    },
    {
      title: t('settings.standards.store'),
      dataIndex: 'store_enabled',
      key: 'store_enabled',
      width: 90,
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
      render: (_, record) => (
        <Input
          size="small"
          value={record.target_value ?? ''}
          onChange={(event) => patchStandardItem(record.var_id, { target_value: event.target.value })}
        />
      ),
    },
    {
      title: 'LL',
      dataIndex: 'limit_ll',
      key: 'limit_ll',
      width: 96,
      render: (_, record) => <InputNumber size="small" value={record.limit_ll ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_ll: value })} />,
    },
    {
      title: 'L',
      dataIndex: 'limit_l',
      key: 'limit_l',
      width: 96,
      render: (_, record) => <InputNumber size="small" value={record.limit_l ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_l: value })} />,
    },
    {
      title: 'H',
      dataIndex: 'limit_h',
      key: 'limit_h',
      width: 96,
      render: (_, record) => <InputNumber size="small" value={record.limit_h ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_h: value })} />,
    },
    {
      title: 'HH',
      dataIndex: 'limit_hh',
      key: 'limit_hh',
      width: 96,
      render: (_, record) => <InputNumber size="small" value={record.limit_hh ?? null} onChange={(value) => patchStandardItem(record.var_id, { limit_hh: value })} />,
    },
    {
      title: t('settings.standards.limitDeadband'),
      dataIndex: 'limit_deadband',
      key: 'limit_deadband',
      width: 120,
      render: (_, record) => <InputNumber size="small" min={0} value={record.limit_deadband ?? 0} onChange={(value) => patchStandardItem(record.var_id, { limit_deadband: value ?? 0 })} />,
    },
    {
      title: t('settings.standards.violationHold'),
      dataIndex: 'violation_hold_ms',
      key: 'violation_hold_ms',
      width: 120,
      render: (_, record) => <InputNumber size="small" min={0} value={record.violation_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { violation_hold_ms: value ?? 0 })} />,
    },
    {
      title: t('settings.standards.recoverHold'),
      dataIndex: 'recover_hold_ms',
      key: 'recover_hold_ms',
      width: 120,
      render: (_, record) => <InputNumber size="small" min={0} value={record.recover_hold_ms ?? 0} onChange={(value) => patchStandardItem(record.var_id, { recover_hold_ms: value ?? 0 })} />,
    },
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
      render: (_, record) => (
        <Button danger size="small" icon={<Trash2 size={13} />} aria-label={t('settings.users.delete')} onClick={() => removeStandardItem(record.var_id)} />
      ),
    },
  ]

  return (
    <div className="settings-page">
      {contextHolder}
      <header className="settings-hero">
        <div>
          <span className="settings-eyebrow">{t('settings.eyebrow')}</span>
          <h1>{t('settings.title')}</h1>
        </div>
        <div className="settings-summary">
          <button className={activeModule === 'variables' ? 'active' : ''} onClick={() => setActiveModule('variables')}>
            <Settings2 size={17} />
            <strong>{variables.length}</strong>
            <span>{t('settings.variables.title')}</span>
          </button>
          <button className={activeModule === 'standards' ? 'active' : ''} onClick={() => setActiveModule('standards')}>
            <ShieldCheck size={17} />
            <strong>{standards.length}</strong>
            <span>{t('settings.standards.title')}</span>
          </button>
          <button className={activeModule === 'storage' ? 'active' : ''} onClick={() => setActiveModule('storage')}>
            <FolderTree size={17} />
            <strong>{storageRoutes.length}</strong>
            <span>{t('settings.storage.title')}</span>
          </button>
          <button className={activeModule === 'realtime' ? 'active' : ''} onClick={() => setActiveModule('realtime')}>
            <Cable size={17} />
            <strong>{gateways.length}</strong>
            <span>{t('settings.summary.gateways')}</span>
          </button>
          <button className={activeModule === 'history' ? 'active' : ''} onClick={() => setActiveModule('history')}>
            <Database size={17} />
            <strong>{t('settings.historySource.reserved')}</strong>
            <span>{t('settings.historySource.title')}</span>
          </button>
          <button className={activeModule === 'system' ? 'active' : ''} onClick={() => setActiveModule('system')}>
            <ServerCog size={17} />
            <strong>{Object.values(gatewayStatusQuery.data ?? {}).filter((item) => item.active).length}</strong>
            <span>{t('settings.system.title')}</span>
          </button>
          {canManageUsers ? (
            <button className={activeModule === 'users' ? 'active' : ''} onClick={() => setActiveModule('users')}>
              <UserRound size={17} />
              <strong>{users.length}</strong>
              <span>{t('settings.users.title')}</span>
            </button>
          ) : null}
        </div>
      </header>
      <div className="settings-center">
        <main className="settings-module-content">
          {activeModule === 'variables' ? (
            <div className="settings-variable-workbench">
              <section className="settings-panel settings-filter-panel">
                <div className="settings-panel-head">
                  <div>
                    <span className="settings-eyebrow">{t('settings.groups.subtitle')}</span>
                    <h2>{t('settings.groups.title')}</h2>
                  </div>
                  <Button size="small" icon={<Plus size={14} />} onClick={() => setDeviceModalOpen(true)}>
                    {t('settings.groups.create')}
                  </Button>
                </div>
                <div className="settings-device-list">
                  <FilterButton active={variableFilter === 'all'} label={t('settings.groups.allVariables')} count={variables.length} onClick={() => setVariableFilterWithReset('all')} />
                  <FilterButton active={variableFilter === 'known'} label={t('settings.variables.known')} count={variables.filter((item) => variableProjectId(item)).length} onClick={() => setVariableFilterWithReset('known')} />
                  <FilterButton active={variableFilter === 'unknown'} label={t('settings.variables.unknown')} count={variables.filter((item) => !variableProjectId(item)).length} onClick={() => setVariableFilterWithReset('unknown')} />
                  {devices.map((device) => (
                    <FilterButton
                      key={device.id}
                      active={variableFilter === device.id}
                      label={displayDeviceName(device)}
                      note={device.device_code}
                      count={variables.filter((item) => variableProjectId(item) === device.id).length}
                      onClick={() => setVariableFilterWithReset(device.id)}
                    />
                  ))}
                </div>
              </section>
              <section className="settings-panel settings-variable-table-panel">
                <div className="settings-panel-head">
                  <div>
                    <span className="settings-eyebrow">{t('settings.variables.title')}</span>
                    <h2>{t('settings.variables.allInfo')}</h2>
                  </div>
                  <div className="settings-head-actions">
                    <Button size="small" icon={<RotateCcw size={14} />} onClick={openKioRemapModal}>
                      {t('settings.variables.kioRemap')}
                    </Button>
                    <Button size="small" icon={<Plus size={14} />} onClick={openVirtualVariableModal}>
                      {t('settings.variables.createVirtual')}
                    </Button>
                    <Input
                      className="settings-search"
                      prefix={<Search size={14} />}
                      value={variableKeyword}
                      onChange={(event) => {
                        setVariableKeyword(event.target.value)
                        setUnassignedPage(1)
                      }}
                      placeholder={t('settings.variables.search')}
                    />
                  </div>
                </div>
                {isUnassignedView ? (
                  <div className="settings-unassigned-workspace">
                    <div className="settings-unassigned-toolbar">
                      <div>
                        <strong>{t('settings.variables.unassignedPool')}</strong>
                        <span>{t('settings.variables.unassignedPoolHint')}</span>
                      </div>
                      <strong className="settings-unassigned-count">{selectedUnassignedVariables.length} / {unassignedVariables.length}</strong>
                      <div className="settings-unassigned-actions">
                        <Button
                          size="small"
                          onClick={() => setSelectedUnassignedIds(unassignedVariables.map(variableWireId))}
                          disabled={unassignedVariables.length === 0}
                        >
                          {t('settings.variables.selectAll')}
                        </Button>
                        <Button size="small" onClick={() => setSelectedUnassignedIds([])} disabled={selectedUnassignedVariables.length === 0}>
                          {t('settings.variables.clearSelection')}
                        </Button>
                        <button
                          type="button"
                          className="settings-batch-assign-button"
                          disabled={selectedUnassignedVariables.length === 0}
                          onClick={openBatchAssignModal}
                        >
                          <Save size={14} aria-hidden="true" />
                          <span>{t('settings.variables.batchAssign', { count: selectedUnassignedVariables.length })}</span>
                        </button>
                      </div>
                    </div>
                    <div className="settings-unassigned-grid">
                      {visibleUnassignedVariables.map((variable) => {
                        const id = variableWireId(variable)
                        const checked = selectedUnassignedIdSet.has(id)
                        return (
                          <div
                            key={id}
                            role="button"
                            tabIndex={0}
                            className={checked ? 'settings-unassigned-card selected' : 'settings-unassigned-card'}
                            onClick={() => toggleUnassignedSelection(id)}
                            onKeyDown={(event) => {
                              if (event.key === 'Enter' || event.key === ' ') {
                                event.preventDefault()
                                toggleUnassignedSelection(id)
                              }
                            }}
                          >
                            <Checkbox checked={checked} onChange={() => toggleUnassignedSelection(id)} onClick={(event) => event.stopPropagation()} />
                            <div className="settings-unassigned-card-main">
                              <strong>{variable.raw_name || variable.var_name}</strong>
                              <span>{variable.source_path || variable.json_path || '-'}</span>
                            </div>
                            <div className="settings-unassigned-card-meta">
                              <Tag>G{variable.gateway_id}</Tag>
                              <Tag>{variable.data_type || t('settings.variables.readonly')}</Tag>
                            </div>
                          </div>
                        )
                      })}
                      {unassignedVariables.length === 0 ? (
                        <div className="settings-empty-card">{t('settings.variables.emptyUnassigned')}</div>
                      ) : null}
                    </div>
                    {unassignedVariables.length > UNASSIGNED_PAGE_SIZE ? (
                      <Pagination
                        className="settings-unassigned-pagination"
                        size="small"
                        current={safeUnassignedPage}
                        total={unassignedVariables.length}
                        pageSize={UNASSIGNED_PAGE_SIZE}
                        showSizeChanger={false}
                        showLessItems
                        onChange={setUnassignedPage}
                      />
                    ) : null}
                  </div>
                ) : (
                  <Table
                    size="small"
                    rowKey="var_id"
                    loading={variablesQuery.isFetching}
                    columns={variableColumns}
                    dataSource={filteredVariables}
                    scroll={{ x: 2000, y: 560 }}
                    pagination={{ pageSize: 50, showSizeChanger: true, size: 'small' }}
                  />
                )}
              </section>
            </div>
          ) : null}

          {activeModule === 'standards' ? (
            <section className="settings-panel settings-full-module">
              <div className="settings-panel-head">
                <div>
                  <span className="settings-eyebrow">{t('settings.standards.subtitle')}</span>
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
                scroll={{ x: 980, y: 560 }}
                pagination={{ pageSize: 20, size: 'small' }}
              />
            </section>
          ) : null}

          {activeModule === 'storage' ? (
            <section className="settings-panel settings-full-module">
              <div className="settings-panel-head">
                <div>
                  <span className="settings-eyebrow">{t('settings.storage.subtitle')}</span>
                  <h2>{t('settings.storage.title')}</h2>
                </div>
                <Button size="small" icon={<Plus size={14} />} onClick={() => openStorageRouteModal()}>
                  {t('settings.storage.create')}
                </Button>
              </div>
              <Alert
                className="settings-database-alert"
                type="info"
                showIcon
                message={t('settings.storage.routeHint')}
              />
              <div className="settings-storage-toolbar">
                <Input
                  allowClear
                  prefix={<Search size={14} />}
                  value={storageRouteSearch}
                  onChange={(event) => setStorageRouteSearch(event.target.value)}
                  placeholder={t('settings.storage.searchPlaceholder')}
                />
                <Segmented
                  size="small"
                  value={storageRouteStatus}
                  onChange={(value) => setStorageRouteStatus(value as StorageRouteStatusFilter)}
                  options={[
                    { label: t('settings.storage.statusAll'), value: 'all' },
                    { label: t('settings.storage.statusEnabled'), value: 'enabled' },
                    { label: t('settings.storage.statusDisabled'), value: 'disabled' },
                  ]}
                />
                <span>{t('settings.storage.filteredCount', { count: filteredStorageRoutes.length })}</span>
              </div>
              <Table
                size="small"
                rowKey="id"
                loading={storageRoutesQuery.isFetching}
                columns={storageRouteColumns}
                dataSource={filteredStorageRoutes}
                scroll={{ x: 1260, y: 520 }}
                pagination={{ pageSize: 20, showSizeChanger: true, size: 'small' }}
              />
            </section>
          ) : null}

          {activeModule === 'realtime' ? (
            <section className="settings-panel settings-full-module">
              <div className="settings-panel-head">
                <div>
                  <span className="settings-eyebrow">{t('settings.realtime.service')}</span>
                  <h2>{t('settings.gateway.title')}</h2>
                </div>
                <Button size="small" icon={<Plus size={14} />} onClick={() => openGatewayModal()}>
                  {t('settings.gateway.new')}
                </Button>
              </div>
              <div className="settings-gateway-inline-list">
                {gateways.map((gateway) => {
                  const status = gatewayStatusFor(gateway, gatewayStatusQuery.data)
                  return (
                    <button key={gateway.id} className="settings-gateway-inline-card" onClick={() => openGatewayModal(gateway)}>
                      <span className={status?.active ? 'settings-live-dot online' : 'settings-live-dot'} />
                      <strong>{gateway.name}</strong>
                      <em>{status?.active ? t('status.online') : t('status.offline')}</em>
                      <small>{gateway.broker}</small>
                    </button>
                  )
                })}
                {gateways.length === 0 ? <div className="settings-empty">{t('settings.gateway.empty')}</div> : null}
              </div>
            </section>
          ) : null}

          {activeModule === 'history' ? (
            <div className="settings-history-grid">
              <section className="settings-panel settings-history-card">
                <div className="settings-panel-head">
                  <div>
                    <span className="settings-eyebrow">{t('settings.historySource.mysql')}</span>
                    <h2>{t('settings.historySource.connection')}</h2>
                  </div>
                  <Tag color={databaseConfigQuery.data?.password_set ? 'success' : 'warning'}>
                    {databaseConfigQuery.data?.password_set
                      ? t('settings.historySource.passwordSet')
                      : t('settings.historySource.passwordMissing')}
                  </Tag>
                </div>
                <Form
                  form={databaseConfigForm}
                  layout="vertical"
                  onFinish={(values) => saveDatabaseMutation.mutate(values)}
                  className="settings-database-form"
                >
                  <div className="settings-form-grid">
                    <Form.Item
                      name="host"
                      label={t('settings.historySource.host')}
                      rules={[{ required: true, message: t('settings.validation.required') }]}
                    >
                      <Input prefix={<Database size={14} />} />
                    </Form.Item>
                    <Form.Item
                      name="port"
                      label={t('settings.historySource.port')}
                      rules={[{ required: true, message: t('settings.validation.required') }]}
                    >
                      <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item
                      name="user"
                      label={t('settings.historySource.user')}
                      rules={[{ required: true, message: t('settings.validation.required') }]}
                    >
                      <Input prefix={<UserRound size={14} />} />
                    </Form.Item>
                    <Form.Item name="password" label={t('settings.historySource.password')}>
                      <Input.Password
                        prefix={<KeyRound size={14} />}
                        placeholder={databaseConfigQuery.data?.password_set ? t('settings.gateway.passwordPlaceholder') : ''}
                      />
                    </Form.Item>
                    <Form.Item
                      name="name"
                      label={t('settings.historySource.database')}
                      rules={[{ required: true, message: t('settings.validation.required') }]}
                    >
                      <Input />
                    </Form.Item>
                    <Form.Item name="auto_migrate" valuePropName="checked" label={t('settings.historySource.autoMigrate')}>
                      <Switch />
                    </Form.Item>
                  </div>
                  <Alert
                    className="settings-database-alert"
                    type="info"
                    showIcon
                    message={t('settings.historySource.restartRequired')}
                    description={t('settings.historySource.restartRequiredDesc')}
                  />
                  {databaseTestFeedback ? (
                    <Alert
                      className="settings-database-alert"
                      type={databaseTestFeedback.ok ? 'success' : 'error'}
                      showIcon
                      message={databaseTestFeedback.ok ? t('settings.historySource.testSuccess') : t('settings.historySource.testFailed')}
                      description={databaseTestFeedback.message}
                    />
                  ) : null}
                  {databaseRestartRequired || databaseConfigQuery.data?.restart_required ? (
                    <Alert
                      className="settings-database-alert"
                      type="warning"
                      showIcon
                      message={t('settings.historySource.restartRequiredSaved')}
                      description={t('settings.historySource.restartRequiredDesc')}
                    />
                  ) : null}
                  <div className="settings-form-actions">
                    <Space wrap>
                      <Button
                        icon={<Database size={16} />}
                        loading={testDatabaseMutation.isPending}
                        onClick={() => {
                          databaseConfigForm
                            .validateFields()
                            .then((values) => testDatabaseMutation.mutate(values))
                            .catch(() => undefined)
                        }}
                      >
                        {t('settings.historySource.test')}
                      </Button>
                      <Button
                        type="primary"
                        htmlType="submit"
                        icon={<Save size={16} />}
                        loading={saveDatabaseMutation.isPending}
                      >
                        {t('settings.historySource.save')}
                      </Button>
                    </Space>
                  </div>
                </Form>
              </section>
              <section className="settings-panel settings-risk-card">
                <div className="settings-panel-head">
                  <div>
                    <span className="settings-eyebrow">{t('settings.historySource.integration')}</span>
                    <h2>{t('settings.historySource.notes')}</h2>
                  </div>
                  <CircleAlert size={18} />
                </div>
                <ul>
                  <li>{t('settings.historySource.noteExternal')}</li>
                  <li>{t('settings.historySource.noteNoRendererDb')}</li>
                  <li>{t('settings.historySource.noteFutureApi')}</li>
                </ul>
              </section>
            </div>
          ) : null}

          {activeModule === 'system' ? (
            <section className="settings-panel settings-full-module">
              <div className="settings-panel-head">
                <div>
                  <span className="settings-eyebrow">{t('settings.system.local')}</span>
                  <h2>{t('settings.system.title')}</h2>
                </div>
                <Tag color={sidecarTagColor[sidecarState]}>{t(`status.${sidecarState}`)}</Tag>
              </div>
              <div className="settings-system-grid">
                <div className="settings-system-card">
                  <div>
                    <span className="settings-system-icon"><ServerCog size={18} /></span>
                    <strong>{t('settings.system.sidecar')}</strong>
                    <p>{sidecarStatus?.pid ? `PID ${sidecarStatus.pid}` : sidecarStatus?.error || t('settings.system.sidecarDesc')}</p>
                  </div>
                  <Button
                    size="small"
                    icon={<RotateCcw size={14} />}
                    loading={restartSidecarMutation.isPending}
                    onClick={() => restartSidecarMutation.mutate()}
                  >
                    {t('settings.system.restartSidecar')}
                  </Button>
                </div>
                <SystemSwitchCard
                  icon={<RefreshCw size={18} />}
                  title={t('settings.system.startup')}
                  description={t('settings.system.startupDesc')}
                  checked={Boolean(desktopStatus?.autostart.openAtLogin)}
                  disabled={!desktopStatus?.trayAvailable}
                  loading={setAutostartMutation.isPending || desktopStatusQuery.isFetching}
                  onChange={(checked) => setAutostartMutation.mutate(checked)}
                />
                <SystemSwitchCard
                  icon={<Minimize2 size={18} />}
                  title={t('settings.system.minimizeToTray')}
                  description={t('settings.system.minimizeToTrayDesc')}
                  checked={Boolean(desktopStatus?.minimizeToTray)}
                  disabled={!desktopStatus?.trayAvailable}
                  loading={setMinimizeToTrayMutation.isPending || desktopStatusQuery.isFetching}
                  onChange={(checked) => setMinimizeToTrayMutation.mutate(checked)}
                />
                <div className="settings-system-card">
                  <div>
                    <span className="settings-system-icon"><ShieldCheck size={18} /></span>
                    <strong>{t('settings.system.watchdog')}</strong>
                    <p>{t('settings.system.watchdogDesc', { count: desktopStatus?.restartAttempts ?? sidecarStatus?.restartAttempts ?? 0 })}</p>
                  </div>
                  <Tag color={desktopStatus?.watchdogEnabled ? 'success' : 'default'}>
                    {desktopStatus?.watchdogEnabled ? t('status.online') : t('status.unavailable')}
                  </Tag>
                </div>
                <div className="settings-system-card">
                  <div>
                    <span className="settings-system-icon"><FolderOpen size={18} /></span>
                    <strong>{t('settings.system.logs')}</strong>
                    <p>{sidecarStatus?.logFile || t('settings.system.logsDesc')}</p>
                  </div>
                  <Space wrap>
                    <Button
                      size="small"
                      icon={<RefreshCw size={14} />}
                      loading={runtimeLogsQuery.isFetching}
                      onClick={() => runtimeLogsQuery.refetch()}
                    >
                      {t('settings.system.refreshLogs')}
                    </Button>
                    <Button
                      size="small"
                      icon={<FolderOpen size={14} />}
                      loading={openLogsMutation.isPending}
                      onClick={() => openLogsMutation.mutate()}
                    >
                      {t('actions.openLogs')}
                    </Button>
                  </Space>
                </div>
                <div className="settings-system-card settings-log-preview-card">
                  <div className="settings-log-preview-head">
                    <div>
                      <span className="settings-system-icon"><Clipboard size={18} /></span>
                      <strong>{t('settings.system.logPreview')}</strong>
                      <p>
                        {runtimeLogsQuery.data?.updatedAt
                          ? t('settings.system.logUpdatedAt', { time: new Date(runtimeLogsQuery.data.updatedAt).toLocaleString() })
                          : t('settings.system.logPreviewDesc')}
                      </p>
                    </div>
                    <Tag color={runtimeLogsQuery.data?.size ? 'processing' : 'default'}>
                      {runtimeLogsQuery.data?.size ? `${Math.ceil(runtimeLogsQuery.data.size / 1024)} KB` : t('status.unavailable')}
                    </Tag>
                  </div>
                  <div className="settings-log-preview-actions">
                    <Button
                      size="small"
                      icon={<Clipboard size={14} />}
                      loading={runtimeLogsQuery.isFetching}
                      onClick={() => setRuntimeLogModalOpen(true)}
                    >
                      {t('settings.system.logPreview')}
                    </Button>
                    <Button
                      size="small"
                      icon={<RefreshCw size={14} />}
                      loading={runtimeLogsQuery.isFetching}
                      onClick={() => runtimeLogsQuery.refetch()}
                    >
                      {t('settings.system.refreshLogs')}
                    </Button>
                  </div>
                </div>
                <div className="settings-system-card settings-audit-card">
                  <div className="settings-log-preview-head">
                    <div>
                      <span className="settings-system-icon"><ShieldCheck size={18} /></span>
                      <strong>{t('settings.system.auditLogs')}</strong>
                      <p>{t('settings.system.auditLogsDesc', { count: auditLogsQuery.data?.total ?? 0 })}</p>
                    </div>
                    <Button
                      size="small"
                      icon={<RefreshCw size={14} />}
                      loading={auditLogsQuery.isFetching}
                      onClick={() => auditLogsQuery.refetch()}
                    >
                      {t('settings.system.refreshAuditLogs')}
                    </Button>
                  </div>
                  <Table
                    size="small"
                    rowKey="id"
                    loading={auditLogsQuery.isFetching}
                    columns={auditLogColumns}
                    dataSource={auditLogsQuery.data?.items ?? []}
                    scroll={{ x: 1360, y: 360 }}
                    pagination={false}
                    locale={{ emptyText: t('settings.system.auditEmpty') }}
                  />
                </div>
                {!desktopStatus?.trayAvailable ? (
                  <Alert showIcon type="info" message={t('settings.system.browserPreview')} description={t('messages.bridgeUnavailable')} />
                ) : null}
              </div>
            </section>
          ) : null}

          {activeModule === 'users' && canManageUsers ? (
            <section className="settings-panel settings-full-module settings-users-module">
              <div className="settings-users-layout">
                <div className="settings-users-table-card">
                  <div className="settings-panel-head">
                    <div>
                      <span className="settings-eyebrow">{t('settings.users.subtitle')}</span>
                      <h2>{t('settings.users.title')}</h2>
                    </div>
                    <Button size="small" icon={<Plus size={14} />} onClick={() => openUserModal()}>
                      {t('settings.users.create')}
                    </Button>
                  </div>
                  <Table
                    size="small"
                    rowKey="id"
                    loading={usersQuery.isFetching}
                    columns={userColumns}
                    dataSource={users}
                    scroll={{ x: 1700, y: 410 }}
                    pagination={{ pageSize: 20, showSizeChanger: true, size: 'small' }}
                  />
                </div>
                <div className="settings-project-members-card">
                  <div className="settings-panel-head">
                    <div>
                      <span className="settings-eyebrow">{t('settings.projectMembers.subtitle')}</span>
                      <h2>{t('settings.projectMembers.title')}</h2>
                    </div>
                    <Tag icon={<UsersRound size={13} />}>{projectMembersQuery.data?.count ?? memberDrafts.length}</Tag>
                  </div>
                  <div className="settings-project-members-toolbar">
                    <Select
                      showSearch
                      value={activeMemberProjectId}
                      placeholder={t('settings.projectMembers.selectProject')}
                      optionFilterProp="label"
                      onChange={(value) => setMemberProjectId(value)}
                      options={devices.map((device) => ({
                        value: device.id,
                        label: `${displayDeviceName(device)} · ${device.device_code}`,
                      }))}
                    />
                    <Button size="small" icon={<RefreshCw size={14} />} loading={projectMembersQuery.isFetching} onClick={() => projectMembersQuery.refetch()}>
                      {t('actions.refresh')}
                    </Button>
                  </div>
                  <Alert
                    className="settings-database-alert"
                    type="info"
                    showIcon
                    message={t('settings.projectMembers.hint')}
                    description={memberProject ? `${displayDeviceName(memberProject)} · ${memberProject.device_code}` : t('settings.projectMembers.selectProject')}
                  />
                  <div className="settings-project-members-toolbar">
                    <Select
                      showSearch
                      allowClear
                      value={memberUserIdToAdd}
                      placeholder={t('settings.projectMembers.addUser')}
                      optionFilterProp="label"
                      onChange={(value) => setMemberUserIdToAdd(value)}
                      options={availableMemberUsers.map((user) => ({
                        value: user.id,
                        label: `${user.username} · ${t(`settings.users.roles.${user.role}`, { defaultValue: user.role })}`,
                      }))}
                    />
                    <Button size="small" icon={<Plus size={14} />} disabled={!memberUserIdToAdd} onClick={addProjectMember}>
                      {t('settings.projectMembers.add')}
                    </Button>
                    <Button
                      type="primary"
                      size="small"
                      icon={<Save size={14} />}
                      disabled={!activeMemberProjectId}
                      loading={saveProjectMembersMutation.isPending}
                      onClick={() => saveProjectMembersMutation.mutate()}
                    >
                      {t('settings.projectMembers.save')}
                    </Button>
                  </div>
                  <Table
                    size="small"
                    rowKey="draft_key"
                    loading={projectMembersQuery.isFetching}
                    columns={projectMemberColumns}
                    dataSource={projectMemberRows}
                    scroll={{ x: 760, y: 310 }}
                    pagination={false}
                    locale={{ emptyText: t('settings.projectMembers.empty') }}
                  />
                </div>
              </div>
            </section>
          ) : null}
        </main>
      </div>

      <Modal
        title={editingGateway ? editingGateway.name : t('settings.gateway.new')}
        open={gatewayModalOpen}
        width={920}
        onCancel={() => setGatewayModalOpen(false)}
        footer={null}
      >
        <Form form={gatewayForm} layout="vertical" onFinish={(values) => saveGatewayMutation.mutate(values)}>
          <div className="settings-form-grid modal-grid">
            <Form.Item name="name" label={t('settings.gateway.name')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="enabled" label={t('settings.gateway.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="broker" label={t('settings.gateway.broker')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="client_id" label={t('settings.gateway.clientId')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="username" label={t('settings.gateway.username')}>
              <Input />
            </Form.Item>
            <Form.Item name="password" label={t('settings.gateway.password')}>
              <Input.Password placeholder={t('settings.gateway.passwordPlaceholder')} />
            </Form.Item>
            <Form.Item name="topic" label={t('settings.gateway.topic')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="qos" label="QoS">
              <InputNumber min={0} max={2} />
            </Form.Item>
            <Form.Item name="parser_type" label={t('settings.gateway.parser')}>
              <Select options={[{ label: 'KingIOT / KIO', value: 'kingiot_kio' }, { label: 'Generic JSON', value: 'json' }]} />
            </Form.Item>
            <Form.Item name="kio_client_id" label={t('settings.gateway.kioClient')}>
              <Input />
            </Form.Item>
            <Form.Item name="kio_writer" label={t('settings.gateway.kioWriter')}>
              <Input />
            </Form.Item>
            <Form.Item name="kio_write_username" label={t('settings.gateway.kioWriteUser')}>
              <Input />
            </Form.Item>
            <Form.Item name="kio_write_password" label={t('settings.gateway.kioWritePassword')}>
              <Input.Password placeholder={t('settings.gateway.passwordPlaceholder')} />
            </Form.Item>
            <Form.Item name="setdata_topic" label={t('settings.gateway.setdataTopic')}>
              <Input />
            </Form.Item>
            <Form.Item name="write_result_topic" label={t('settings.gateway.writeResultTopic')}>
              <Input />
            </Form.Item>
            <Form.Item name="query_all_topic" label={t('settings.gateway.queryAllTopic')}>
              <Input />
            </Form.Item>
          </div>
          <div className="settings-form-actions">
            {editingGateway ? (
              <Button
                icon={<RefreshCw size={14} />}
                loading={discoverMutation.isPending}
                onClick={() => discoverMutation.mutate(editingGateway.id)}
              >
                {t('settings.gateway.discover')}
              </Button>
            ) : null}
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveGatewayMutation.isPending}>
              {t('settings.actions.saveGateway')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('settings.groups.create')}
        open={deviceModalOpen}
        width={560}
        onCancel={() => setDeviceModalOpen(false)}
        footer={null}
      >
        <Form form={deviceForm} layout="vertical" onFinish={(values) => createDeviceMutation.mutate(values)}>
          <Form.Item name="device_code" label={t('settings.groups.code')} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="name" label={t('settings.groups.name')} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="site_no" label={t('settings.groups.site')}>
            <Input />
          </Form.Item>
          <Form.Item name="model_name" label={t('settings.groups.model')}>
            <Input />
          </Form.Item>
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={createDeviceMutation.isPending}>
              {t('settings.groups.create')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('settings.variables.createVirtual')}
        open={virtualVariableModalOpen}
        width={920}
        onCancel={() => {
          setVirtualVariableModalOpen(false)
          virtualVariableForm.resetFields()
        }}
        footer={null}
      >
        <Form form={virtualVariableForm} layout="vertical" onFinish={(values) => createVirtualVariableMutation.mutate(values)}>
          <Alert className="settings-modal-alert" showIcon type="info" message={t('settings.variables.virtualHint')} />
          <div className="settings-form-grid modal-grid">
            <Form.Item name="project_id" label={t('settings.variables.selectDevice')} rules={[{ required: true }]}>
              <Select
                options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
              />
            </Form.Item>
            <Form.Item name="var_name" label={t('settings.variables.varName')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="display_name" label={t('settings.variables.displayName')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="display_name_en" label={t('settings.variables.displayNameEn')}>
              <Input />
            </Form.Item>
            <Form.Item name="display_name_ja" label={t('settings.variables.displayNameJa')}>
              <Input />
            </Form.Item>
            <Form.Item name="data_type" label={t('settings.variables.type')} rules={[{ required: true }]}>
              <Select options={[{ label: 'INT', value: 'INT' }, { label: 'FLOAT', value: 'FLOAT' }, { label: 'BOOL', value: 'BOOL' }, { label: 'STRING', value: 'STRING' }]} />
            </Form.Item>
            <Form.Item name="unit" label={t('settings.variables.unit')}>
              <Input />
            </Form.Item>
            <Form.Item name="decimal_places" label={t('settings.variables.decimalPlaces')}>
              <InputNumber min={0} max={8} />
            </Form.Item>
            <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          <VariableDefaultAlarmFields />
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={createVirtualVariableMutation.isPending}>
              {t('settings.variables.createVirtual')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('settings.variables.kioRemapTitle')}
        open={kioRemapModalOpen}
        width={1040}
        onCancel={() => {
          setKioRemapModalOpen(false)
          setKioRemapResult(undefined)
        }}
        footer={null}
      >
        <Alert className="settings-modal-alert" showIcon type="info" message={t('settings.variables.kioRemapHint')} />
        <Form form={kioRemapForm} layout="vertical">
          <div className="settings-form-grid modal-grid">
            <Form.Item name="project_count" label={t('settings.variables.kioProjectCount')} rules={[{ required: true }]}>
              <InputNumber min={1} max={99} />
            </Form.Item>
            <Form.Item name="project_code_prefix" label={t('settings.variables.kioProjectCodePrefix')}>
              <Input />
            </Form.Item>
            <Form.Item name="project_display_prefix" label={t('settings.variables.kioProjectDisplayPrefix')}>
              <Input />
            </Form.Item>
            <Form.Item name="raw_project_prefix" label={t('settings.variables.kioRawProjectPrefix')}>
              <Input />
            </Form.Item>
            <Form.Item name="var_group" label={t('settings.variables.group')}>
              <Input />
            </Form.Item>
            <Form.Item name="var_name_prefix" label={t('settings.variables.kioVarNamePrefix')}>
              <Input />
            </Form.Item>
            <Form.Item name="remap_var_name" label={t('settings.variables.kioRemapVarName')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="enable" label={t('settings.variables.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          <div className="settings-form-actions">
            <Button loading={kioRemapMutation.isPending} onClick={() => void submitKioRemap(true)}>
              {t('settings.variables.kioDryRun')}
            </Button>
            <Popconfirm
              title={t('settings.variables.kioExecuteConfirm')}
              onConfirm={() => void submitKioRemap(false)}
            >
              <Button type="primary" danger icon={<RotateCcw size={15} />} loading={kioRemapMutation.isPending}>
                {t('settings.variables.kioExecute')}
              </Button>
            </Popconfirm>
          </div>
        </Form>
        {kioRemapResult ? (
          <div className="settings-kio-remap-result">
            <div className="settings-kio-remap-summary">
              <Tag color={kioRemapResult.dry_run ? 'processing' : 'success'}>
                {kioRemapResult.dry_run ? t('settings.variables.kioDryRun') : t('settings.variables.kioExecuted')}
              </Tag>
              <span>{t('settings.variables.kioResultSummary', kioRemapResult)}</span>
            </div>
            <Table<BulkRemapKioProjectsResultItem>
              size="small"
              rowKey={(record) => `${record.var_id_text ?? record.var_id}-${record.action}-${record.raw_name}`}
              columns={[
                { title: t('settings.variables.rawName'), dataIndex: 'raw_name', key: 'raw_name', width: 140 },
                { title: t('settings.variables.varName'), dataIndex: 'new_var_name', key: 'new_var_name', width: 150 },
                { title: t('settings.storage.project'), dataIndex: 'project_code', key: 'project_code', width: 120 },
                { title: t('settings.variables.kioAction'), dataIndex: 'action', key: 'action', width: 110, render: (value) => <Tag>{String(value)}</Tag> },
                { title: t('settings.variables.kioReason'), dataIndex: 'reason', key: 'reason', ellipsis: true },
              ]}
              dataSource={kioRemapResult.items.slice(0, 80)}
              pagination={false}
              scroll={{ x: 760, y: 260 }}
            />
          </div>
        ) : null}
      </Modal>

      <Modal
        title={selectedVariable && variableProjectId(selectedVariable) ? t('settings.variables.edit') : t('settings.variables.assignTitle')}
        open={variableModalOpen}
        width={1120}
        onCancel={() => {
          setVariableModalOpen(false)
          setSelectedVariable(undefined)
        }}
        footer={null}
      >
        {selectedVariable ? (
          <div className="settings-variable-modal">
            <div className="settings-readonly-grid">
              <div>
                <span>{t('settings.variables.rawName')}</span>
                <strong>{selectedVariable.raw_name || '-'}</strong>
              </div>
              <div>
                <span>{t('settings.variables.path')}</span>
                <strong>{selectedVariable.source_path || '-'}</strong>
              </div>
              <div>
                <span>{t('settings.variables.jsonPath')}</span>
                <strong>{selectedVariable.json_path || '-'}</strong>
              </div>
              <div>
                <span>Gateway</span>
                <strong>{selectedVariable.gateway_id}</strong>
              </div>
            </div>

            {variableProjectId(selectedVariable) ? (
              <Form form={variableEditForm} layout="vertical" onFinish={(values) => saveVariableMutation.mutate(values)}>
                <VariableFormSection title={t('settings.variables.basicSection')} description={t('settings.variables.basicSectionHint')} defaultOpen>
                  <div className="settings-form-grid modal-grid">
                    <Form.Item name="var_name" label={t('settings.variables.varName')} rules={[{ required: true }]}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('settings.variables.displayName')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="display_name_en" label={t('settings.variables.displayNameEn')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="display_name_ja" label={t('settings.variables.displayNameJa')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="data_type" label={t('settings.variables.type')}>
                      <Select options={[{ label: 'FLOAT', value: 'FLOAT' }, { label: 'INT', value: 'INT' }, { label: 'BOOL', value: 'BOOL' }, { label: 'STRING', value: 'STRING' }]} />
                    </Form.Item>
                    <Form.Item name="unit" label={t('settings.variables.unit')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="decimal_places" label={t('settings.variables.decimalPlaces')}>
                      <InputNumber min={0} max={8} />
                    </Form.Item>
                    <Form.Item name="scale_factor" label={t('settings.variables.scaleFactor')}>
                      <InputNumber step={0.1} />
                    </Form.Item>
                    <Form.Item name="offset_val" label={t('settings.variables.offsetVal')}>
                      <InputNumber step={0.1} />
                    </Form.Item>
                    <Form.Item name="var_group" label={t('settings.variables.group')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </div>
                </VariableFormSection>
                <VariableFormSection title={t('settings.variables.writeSection')} description={t('settings.variables.writeSectionHint')}>
                  <div className="settings-form-grid modal-grid">
                    <Form.Item name="rw_mode" label={t('settings.variables.writeMode')}>
                      <Select options={[{ label: 'R', value: 'R' }, { label: 'W', value: 'W' }, { label: 'RW', value: 'RW' }]} />
                    </Form.Item>
                    <Form.Item name="writable" label={t('settings.variables.writable')} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item name="write_source_id" label={t('settings.variables.writeSourceId')}>
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item name="write_path" label={t('settings.variables.writePath')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="write_data_type" label={t('settings.variables.writeDataType')}>
                      <Select allowClear options={[{ label: 'FLOAT', value: 'FLOAT' }, { label: 'INT', value: 'INT' }, { label: 'BOOL', value: 'BOOL' }, { label: 'STRING', value: 'STRING' }]} />
                    </Form.Item>
                    <Form.Item name="write_min" label={t('settings.variables.writeMin')}>
                      <InputNumber step={0.1} />
                    </Form.Item>
                    <Form.Item name="write_max" label={t('settings.variables.writeMax')}>
                      <InputNumber step={0.1} />
                    </Form.Item>
                    <Form.Item name="write_enum" label={t('settings.variables.writeEnum')}>
                      <Input />
                    </Form.Item>
                    <Form.Item name="write_requires_audit" label={t('settings.variables.writeRequiresAudit')} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </div>
                </VariableFormSection>
                <VariableFormSection title={t('settings.variables.runtimeSection')} description={t('settings.variables.runtimeSectionHint')}>
                  <div className="settings-form-grid modal-grid">
                    <Form.Item name="suspicious_value" label={t('settings.variables.suspiciousValue')}>
                      <InputNumber step={0.1} />
                    </Form.Item>
                    <Form.Item name="debounce_threshold" label={t('settings.variables.debounceThreshold')}>
                      <InputNumber min={0} step={0.1} />
                    </Form.Item>
                    <Form.Item name="debounce_ms" label={t('settings.variables.debounceMs')}>
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item name="deadband" label={t('settings.variables.runtimeDeadband')}>
                      <InputNumber min={0} step={0.1} />
                    </Form.Item>
                  </div>
                </VariableFormSection>
                <VariableFormSection title={t('settings.variables.defaultAlarmSection')} description={t('settings.variables.defaultAlarmSectionHint')}>
                  <VariableDefaultAlarmFields includeApplyToRunning />
                </VariableFormSection>
                <div className="settings-form-actions">
                  <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveVariableMutation.isPending}>
                    {t('settings.variables.save')}
                  </Button>
                </div>
              </Form>
            ) : (
              <Form form={variableAssignForm} layout="vertical" onFinish={(values) => assignVariableMutation.mutate(values)}>
                <Alert className="settings-modal-alert" showIcon type="info" message={t('settings.variables.unassignedReadonly')} />
                <div className="settings-form-grid modal-grid">
                  <Form.Item name="project_id" label={t('settings.variables.selectDevice')} rules={[{ required: true }]}>
                    <Select
                      options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
                    />
                  </Form.Item>
                  <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
                    <Switch />
                  </Form.Item>
                </div>
                <div className="settings-form-actions">
                  <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={assignVariableMutation.isPending}>
                    {t('settings.variables.assign')}
                  </Button>
                </div>
              </Form>
            )}
          </div>
        ) : null}
      </Modal>

      <Modal
        title={t('settings.variables.batchAssignTitle', { count: selectedUnassignedVariables.length })}
        open={batchAssignModalOpen}
        width={720}
        onCancel={() => setBatchAssignModalOpen(false)}
        footer={null}
      >
        <Form form={batchAssignForm} layout="vertical" onFinish={(values) => batchAssignVariableMutation.mutate(values)}>
          <Alert
            className="settings-modal-alert"
            showIcon
            type="info"
            message={t('settings.variables.batchAssignHint', { count: selectedUnassignedVariables.length })}
          />
          <div className="settings-form-grid modal-grid">
            <Form.Item name="project_id" label={t('settings.variables.selectDevice')} rules={[{ required: true }]}>
              <Select
                options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
              />
            </Form.Item>
            <Form.Item name="enabled" label={t('settings.variables.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={batchAssignVariableMutation.isPending}>
              {t('settings.variables.batchAssign', { count: selectedUnassignedVariables.length })}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={editingStorageRoute ? t('settings.storage.edit') : t('settings.storage.create')}
        open={storageRouteModalOpen}
        width={920}
        onCancel={() => {
          setStorageRouteModalOpen(false)
          setEditingStorageRoute(undefined)
          storageRouteForm.resetFields()
        }}
        footer={null}
      >
        <Form
          form={storageRouteForm}
          layout="vertical"
          onFinish={(values) => saveStorageRouteMutation.mutate(values)}
          onValuesChange={(changed) => {
            if (!('var_id' in changed)) return
            const variable = variableById.get(varKey(changed.var_id))
            if (!variable) return
            storageRouteForm.setFieldsValue({
              project_id: variableProjectId(variable),
              column_name: variable.var_name,
              form_field_key: variable.var_name,
              query_alias: variable.var_name,
            })
          }}
        >
          <Alert className="settings-modal-alert" showIcon type="info" message={t('settings.storage.routeHint')} />
          <div className="settings-form-grid modal-grid">
            <Form.Item name="var_id" label={t('settings.storage.variable')} rules={[{ required: true }]}>
              <Select
                showSearch
                optionFilterProp="label"
                options={assignedVariables.map((variable) => ({
                  label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
                  value: variableWireId(variable),
                }))}
              />
            </Form.Item>
            <Form.Item name="project_id" label={t('settings.storage.project')} rules={[{ required: true }]}>
              <Select
                options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
              />
            </Form.Item>
            <Form.Item name="route_code" label={t('settings.storage.routeCode')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="storage_target" label={t('settings.storage.target')} rules={[{ required: true }]}>
              <Select
                options={[
                  { label: 'wide_table', value: 'wide_table' },
                  { label: 'none', value: 'none' },
                ]}
              />
            </Form.Item>
            <Form.Item name="table_name" label={t('settings.storage.table')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="column_name" label={t('settings.storage.column')} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="column_type" label={t('settings.storage.columnType')}>
              <Select
                options={[
                  { label: 'DOUBLE', value: 'DOUBLE' },
                  { label: 'BIGINT', value: 'BIGINT' },
                  { label: 'TEXT', value: 'TEXT' },
                  { label: 'TINYINT(1)', value: 'TINYINT(1)' },
                ]}
              />
            </Form.Item>
            <Form.Item name="trigger_mode" label={t('settings.storage.triggerMode')} rules={[{ required: true }]}>
              <Select
                options={[
                  { label: 'on_cycle', value: 'on_cycle' },
                  { label: 'on_change', value: 'on_change' },
                  { label: 'on_detection', value: 'on_detection' },
                  { label: 'on_start', value: 'on_start' },
                  { label: 'always', value: 'always' },
                ]}
              />
            </Form.Item>
            <Form.Item name="cycle_ms" label={t('settings.storage.cycleMs')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="deadband" label={t('settings.storage.deadband')}>
              <InputNumber min={0} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="form_field_key" label={t('settings.storage.formFieldKey')}>
              <Input />
            </Form.Item>
            <Form.Item name="query_alias" label={t('settings.storage.queryAlias')}>
              <Input />
            </Form.Item>
            <Form.Item name="store_on_start" label={t('settings.storage.storeOnStart')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="enabled" label={t('settings.storage.enabled')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveStorageRouteMutation.isPending}>
              {t('settings.storage.save')}
            </Button>
          </div>
        </Form>
      </Modal>

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
            <Form.Item name="project_id" label={t('settings.variables.selectDevice')}>
              <Select
                allowClear
                options={devices.map((device) => ({ label: `${displayDeviceName(device)} · ${device.device_code}`, value: device.id }))}
              />
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
              <span>{t('settings.standards.itemsHint')}</span>
            </div>
            <div className="settings-standard-item-picker">
              <Select
                showSearch
                allowClear
                value={standardVariableId}
                placeholder={t('settings.standards.addVariable')}
                optionFilterProp="label"
                onChange={setStandardVariableId}
                options={standardVariables.map((variable) => ({
                  label: `${variableTitle(variable, i18n.resolvedLanguage)} · ${variable.var_name}`,
                  value: variableWireId(variable),
                }))}
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
            columns={standardItemColumns}
            dataSource={standardItems}
            scroll={{ x: 980, y: 320 }}
            pagination={false}
          />
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveStandardMutation.isPending}>
              {t('settings.standards.save')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={editingUser ? t('settings.users.edit') : t('settings.users.create')}
        open={userModalOpen}
        width={560}
        onCancel={() => {
          setUserModalOpen(false)
          setEditingUser(undefined)
        }}
        footer={null}
      >
        <Form form={userForm} layout="vertical" onFinish={(values) => saveUserMutation.mutate(values)}>
          <Form.Item name="username" label={t('settings.users.username')} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          {!editingUser ? (
            <Form.Item name="password" label={t('settings.users.password')} rules={[{ required: true, min: 8 }]}>
              <Input.Password />
            </Form.Item>
          ) : null}
          <Form.Item name="role" label={t('settings.users.role')} rules={[{ required: true }]}>
            <Select options={roleOptions} />
          </Form.Item>
          <Form.Item name="enabled" label={t('settings.users.enabled')} valuePropName="checked">
            <Switch disabled={editingUser?.id === currentUser?.id} />
          </Form.Item>
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<Save size={15} />} loading={saveUserMutation.isPending}>
              {t('settings.users.save')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('settings.users.resetPassword')}
        open={passwordModalOpen}
        width={500}
        onCancel={() => {
          setPasswordModalOpen(false)
          setPasswordUser(undefined)
        }}
        footer={null}
      >
        <Form form={passwordForm} layout="vertical" onFinish={(values) => resetPasswordMutation.mutate(values)}>
          <Alert
            showIcon
            type="info"
            className="settings-modal-alert"
            message={passwordUser ? t('settings.users.resetTarget', { username: passwordUser.username }) : t('settings.users.noSelection')}
          />
          <Form.Item name="password" label={t('settings.users.password')} rules={[{ required: true, min: 8 }]}>
            <Input.Password />
          </Form.Item>
          <div className="settings-form-actions">
            <Button type="primary" htmlType="submit" icon={<KeyRound size={15} />} loading={resetPasswordMutation.isPending}>
              {t('settings.users.resetPassword')}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('settings.system.logPreview')}
        open={runtimeLogModalOpen}
        width="min(1120px, calc(100vw - 48px))"
        centered
        className="settings-log-modal"
        onCancel={() => setRuntimeLogModalOpen(false)}
        footer={[
          <Button
            key="refresh"
            icon={<RefreshCw size={14} />}
            loading={runtimeLogsQuery.isFetching}
            onClick={() => runtimeLogsQuery.refetch()}
          >
            {t('settings.system.refreshLogs')}
          </Button>,
          <Button
            key="open"
            icon={<FolderOpen size={14} />}
            loading={openLogsMutation.isPending}
            onClick={() => openLogsMutation.mutate()}
          >
            {t('actions.openLogs')}
          </Button>,
          <Button key="close" type="primary" onClick={() => setRuntimeLogModalOpen(false)}>
            {t('actions.cancel')}
          </Button>,
        ]}
      >
        <div className="settings-log-modal-meta">
          <span>
            {runtimeLogsQuery.data?.updatedAt
              ? t('settings.system.logUpdatedAt', { time: new Date(runtimeLogsQuery.data.updatedAt).toLocaleString() })
              : t('settings.system.logPreviewDesc')}
          </span>
          <Tag color={runtimeLogsQuery.data?.size ? 'processing' : 'default'}>
            {runtimeLogsQuery.data?.size ? `${Math.ceil(runtimeLogsQuery.data.size / 1024)} KB` : t('status.unavailable')}
          </Tag>
        </div>
        <pre className="settings-log-preview settings-log-preview-modal">
          {runtimeLogsQuery.data?.content || t('settings.system.logEmpty')}
        </pre>
      </Modal>

      {backendUnavailable ? (
        <Alert
          className="settings-floating-alert"
          type="error"
          showIcon
          message={t('settings.gateway.apiOffline')}
          description={t('settings.gateway.apiOfflineDesc')}
        />
      ) : null}
    </div>
  )
}

function FilterButton({
  active,
  label,
  note,
  count,
  onClick,
}: {
  active: boolean
  label: string
  note?: string
  count: number
  onClick: () => void
}) {
  return (
    <button className={active ? 'settings-device-item active' : 'settings-device-item'} onClick={onClick}>
      <FolderTree size={15} />
      <div>
        <strong>{label}</strong>
        <span>{note ?? count}</span>
      </div>
      <Tag>{count}</Tag>
    </button>
  )
}

function VariableFormSection({
  title,
  description,
  defaultOpen = false,
  children,
}: {
  title: string
  description: string
  defaultOpen?: boolean
  children: ReactNode
}) {
  return (
    <details className="settings-variable-form-section" open={defaultOpen}>
      <summary>
        <div>
          <strong>{title}</strong>
          <span>{description}</span>
        </div>
      </summary>
      {children}
    </details>
  )
}

function VariableDefaultAlarmFields({ includeApplyToRunning = false }: { includeApplyToRunning?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className="settings-default-alarm-fields">
      <Alert
        className="settings-modal-alert"
        showIcon
        type="info"
        message={t('settings.variables.defaultAlarmHint')}
      />
      <div className="settings-form-grid modal-grid">
        <Form.Item name="default_alarm_enabled" label={t('settings.variables.defaultAlarmEnabled')} valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="default_limit_ll" label={t('settings.variables.defaultLimitLL')}>
          <InputNumber step={0.1} />
        </Form.Item>
        <Form.Item name="default_limit_l" label={t('settings.variables.defaultLimitL')}>
          <InputNumber step={0.1} />
        </Form.Item>
        <Form.Item name="default_limit_h" label={t('settings.variables.defaultLimitH')}>
          <InputNumber step={0.1} />
        </Form.Item>
        <Form.Item name="default_limit_hh" label={t('settings.variables.defaultLimitHH')}>
          <InputNumber step={0.1} />
        </Form.Item>
        <Form.Item name="default_limit_deadband" label={t('settings.variables.defaultLimitDeadband')}>
          <InputNumber min={0} step={0.1} />
        </Form.Item>
        <Form.Item name="default_violation_hold_ms" label={t('settings.variables.defaultViolationHold')}>
          <InputNumber min={0} />
        </Form.Item>
        <Form.Item name="default_recover_hold_ms" label={t('settings.variables.defaultRecoverHold')}>
          <InputNumber min={0} />
        </Form.Item>
      </div>
      {includeApplyToRunning ? (
        <>
          <Form.Item name="apply_to_running" valuePropName="checked" className="settings-apply-running-field">
            <Checkbox>{t('settings.variables.applyToRunning')}</Checkbox>
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(prev, current) => prev.apply_to_running !== current.apply_to_running}>
            {({ getFieldValue }) =>
              getFieldValue('apply_to_running') ? (
                <Alert
                  className="settings-modal-alert"
                  showIcon
                  type="warning"
                  message={t('settings.variables.applyToRunningWarning')}
                />
              ) : null
            }
          </Form.Item>
        </>
      ) : null}
    </div>
  )
}

function SystemSwitchCard({
  icon,
  title,
  description,
  checked,
  disabled,
  loading,
  onChange,
}: {
  icon: ReactNode
  title: string
  description: string
  checked: boolean
  disabled?: boolean
  loading?: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <div className="settings-system-card">
      <div>
        <span className="settings-system-icon">{icon}</span>
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
      <Switch checked={checked} disabled={disabled} loading={loading} onChange={onChange} />
    </div>
  )
}
