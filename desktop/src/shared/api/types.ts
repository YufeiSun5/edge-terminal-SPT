export type ChannelStats = {
  logic: number
  discovery: number
  store: number
  alarm?: number
  notify?: number
}

export type RuntimeQueueDiagnostic = {
  name: string
  len: number
  cap: number
  usage: number
  dropped: number
  pressure: boolean
  impact: string
  next_action: string
}

export type RuntimeChannelDetailsResponse = {
  items: RuntimeQueueDiagnostic[]
  pressure: RuntimeQueueDiagnostic[]
  pressure_threshold: number
}

export type RuntimeNotificationStats = {
  subscribers: number
  buffered: number
  capacity: number
  usage: number
  pressure_threshold: number
  pressure: boolean
  impact: string
  next_action: string
  published: number
  delivered: number
  dropped: number
}

export type RuntimeWorkerStat = {
  name: string
  active: number
  starts: number
  exits: number
  panics: number
  health: 'ok' | 'degraded' | 'stopped_after_panic' | string
  impact: string
  next_action: string
  last_started_at?: string
  last_exited_at?: string
  last_panic_at?: string
  last_error?: string
}

export type RuntimeWorkersResponse = {
  items: RuntimeWorkerStat[]
}

export type TaskFlowRuntimeStats = {
  queue: RuntimeQueueDiagnostic
  guards: number
  submitted: number
  enqueued: number
  dropped: number
  pressure_threshold: number
  pressure: boolean
}

export type GatewayStatus = {
  active: boolean
  client_id: string
  broker: string
  main_topic: string
  write_result_topic?: string
  query_all_topic?: string
  subscribed_topics: string[]
  reconnects: number
  last_connected?: string
  last_disconnected?: string
  last_full_sync?: string
  last_error?: string
}

export type GatewayStatusMap = Record<string, GatewayStatus>

export type HealthResponse = {
  status: string
  role?: 'edge' | 'main_server' | string
  tags?: number
  channels?: ChannelStats
  gateways?: GatewayStatusMap
  database_ok?: boolean
  edge_base_url?: string
  time?: string
}

export type DatabaseConfig = {
  host: string
  port: number
  user: string
  name: string
  auto_migrate: boolean
  password_set: boolean
  restart_required: boolean
}

export type DatabaseConfigPayload = Partial<{
  host: string
  port: number
  user: string
  password: string
  name: string
  auto_migrate: boolean
}>

export type DatabaseConfigTestResult = {
  ok: boolean
  error?: string
}

export type AuditLogEntry = {
  id: number
  actor_type: string
  actor_id: string
  action: string
  target_type: string
  target_id: string
  result: string
  detail: string
  created_at: string
}

export type AuditLogListParams = {
  actor_type?: string
  actor_id?: string
  action?: string
  target_type?: string
  target_id?: string
  result?: string
  from?: string
  to?: string
  created_from?: string
  created_to?: string
  limit?: number
  offset?: number
}

export type AuditLogListResponse = {
  items: AuditLogEntry[]
  total: number
  limit: number
  offset: number
}

export type UserNotification = {
  id: number
  event_uid: string
  type: string
  level: 'info' | 'success' | 'warning' | 'error' | string
  target_type: 'all' | 'user' | 'role' | 'project' | string
  target_id: string
  project_id: number
  project_code?: string
  task_id?: number
  test_no?: string
  var_id?: VarIdentifier
  var_id_text?: string
  var_name?: string
  display_name?: string
  message: string
  payload: Record<string, unknown>
  occurred_at: string
  expires_at?: string
  created_at: string
  read_at?: string
}

export type NotificationListParams = {
  unread?: boolean
  type?: string
  level?: string
  project_id?: number
  from?: string
  to?: string
  keyword?: string
  limit?: number
  offset?: number
}

export type NotificationListResponse = {
  items: UserNotification[]
  total: number
  limit: number
  offset: number
}

export type NotificationUnreadCount = {
  unread: number
}

export type MainReportNotification = {
  id: number
  job_id: number
  event_id: number
  type: string
  level: 'info' | 'success' | 'warning' | 'error' | string
  title: string
  message: string
  payload: Record<string, unknown>
  read_at?: string
  unread: boolean
  created_at: string
}

export type MainReportNotificationListResponse = {
  items: MainReportNotification[]
  total: number
  limit: number
  offset: number
}

export type MainReportNotificationUnreadCount = {
  unread: number
}

export type VarIdentifier = number | string

export type LimitAlarmScope = 'default' | 'detection'

export type LimitAlarm = {
  id: number
  scope: LimitAlarmScope | string
  task_id: number
  test_no?: string
  project_id: number
  project_code?: string
  standard_id?: number
  standard_item_id: number
  run_standard_item_id: number
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  display_name?: string
  display_name_en?: string
  display_name_ja?: string
  check_method: string
  alarm_type: string
  alarm_level: string
  status: string
  start_value?: number
  peak_value?: number
  recover_value?: number
  limit_value?: number
  limit_deadband: number
  quality: number
  first_seen_at: string
  last_seen_at: string
  recovered_at?: string
  duration_ms: number
  message?: string
  created_at: string
  updated_at: string
}

export type LimitAlarmListParams = {
  scope?: LimitAlarmScope
  project_id?: number
  task_id?: number
  test_no?: string
  var_id?: VarIdentifier
  status?: string
  alarm_type?: string
  level?: string
  alarm_level?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export type LimitAlarmListResponse = {
  items: LimitAlarm[]
  total: number
  limit: number
  offset: number
}

export type GatewayConfig = {
  id: number
  name: string
  broker: string
  client_id: string
  username: string
  topic: string
  qos: number
  parser_type: string
  kio_client_id: string
  kio_writer: string
  kio_write_username: string
  setdata_topic: string
  write_result_topic: string
  query_all_topic: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export type GatewayConfigPayload = Partial<
  Omit<GatewayConfig, 'created_at' | 'updated_at'>
> & {
  password?: string
  kio_write_password?: string
}

export type VariableConfig = {
  var_id: VarIdentifier
  var_id_text?: string
  gateway_id: number
  source_topic: string
  source_path: string
  source_type: 'mqtt' | 'manual' | 'virtual' | string
  raw_name: string
  project_id?: number
  project_code: string
  var_group: string
  var_name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  json_path: string
  data_type: string
  unit: string
  decimal_places: number
  scale_factor: number
  offset_val: number
  store_mode?: number
  store_trigger?: 'always' | 'on_detection' | string
  store_cycle_sec?: number
  store_deadband?: number
  storage_name?: string
  storage_target?:
    | 'none'
    | 'history_eav'
    | 'detection_form'
    | 'report_field'
    | 'wide_table'
    | string
  storage_table?: string
  storage_value_column?: string
  storage_key_column?: string
  storage_time_column?: string
  form_field_key?: string
  query_alias?: string
  rw_mode: 'R' | 'W' | 'RW' | string
  writable: boolean
  write_source_id: number
  write_path: string
  write_data_type: string
  write_min?: number
  write_max?: number
  write_enum: string
  write_requires_audit: boolean
  suspicious_value?: number
  debounce_threshold?: number
  debounce_ms: number
  deadband: number
  startup_snapshot_enable?: boolean
  default_alarm_enabled: boolean
  default_limit_ll?: number
  default_limit_l?: number
  default_limit_h?: number
  default_limit_hh?: number
  default_limit_deadband: number
  default_violation_hold_ms: number
  default_recover_hold_ms: number
  discovered: boolean
  placeholder: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export type VariablePatchPayload = Partial<
  Pick<
    VariableConfig,
    | 'var_name'
    | 'display_name'
    | 'display_name_en'
    | 'display_name_ja'
    | 'data_type'
    | 'unit'
    | 'decimal_places'
    | 'scale_factor'
    | 'offset_val'
    | 'rw_mode'
    | 'writable'
    | 'write_source_id'
    | 'write_path'
    | 'write_data_type'
    | 'write_min'
    | 'write_max'
    | 'write_enum'
    | 'write_requires_audit'
    | 'suspicious_value'
    | 'debounce_threshold'
    | 'debounce_ms'
    | 'deadband'
    | 'default_alarm_enabled'
    | 'default_limit_ll'
    | 'default_limit_l'
    | 'default_limit_h'
    | 'default_limit_hh'
    | 'default_limit_deadband'
    | 'default_violation_hold_ms'
    | 'default_recover_hold_ms'
    | 'var_group'
    | 'enabled'
  >
> & {
  apply_to_running?: boolean
}

export type VariableCreatePayload = Partial<
  Pick<
    VariableConfig,
    | 'var_id'
    | 'source_type'
    | 'gateway_id'
    | 'source_topic'
    | 'source_path'
    | 'raw_name'
    | 'project_id'
    | 'project_code'
    | 'var_group'
    | 'display_name'
    | 'display_name_en'
    | 'display_name_ja'
    | 'json_path'
    | 'unit'
    | 'decimal_places'
    | 'scale_factor'
    | 'offset_val'
    | 'rw_mode'
    | 'writable'
    | 'write_source_id'
    | 'write_path'
    | 'write_data_type'
    | 'write_min'
    | 'write_max'
    | 'write_enum'
    | 'write_requires_audit'
    | 'suspicious_value'
    | 'debounce_threshold'
    | 'debounce_ms'
    | 'deadband'
    | 'default_alarm_enabled'
    | 'default_limit_ll'
    | 'default_limit_l'
    | 'default_limit_h'
    | 'default_limit_hh'
    | 'default_limit_deadband'
    | 'default_violation_hold_ms'
    | 'default_recover_hold_ms'
    | 'enabled'
  >
> & {
  var_name: string
  data_type: string
}

export type VariableListParams = {
  gateway_id?: number
  edge_instance_id?: string
  project_id?: number
  assigned?: boolean
  project_code?: string
  var_group?: string
  writable?: boolean
  enabled?: boolean
  discovered?: boolean
  source_type?: 'mqtt' | 'virtual' | 'manual' | string
  keyword?: string
  limit?: number
  offset?: number
}

export type VariableListResponse = {
  items: VariableConfig[]
  total: number
  limit: number
  offset: number
}

export type BulkRemapKioProjectsPayload = Partial<{
  project_count: number
  project_code_prefix: string
  project_display_prefix: string
  project_en_prefix: string
  project_ja_prefix: string
  raw_project_prefix: string
  var_group: string
  var_name_prefix: string
  remap_var_name: boolean
  enable: boolean
  dry_run: boolean
}>

export type BulkRemapKioProjectsResultItem = {
  var_id: VarIdentifier
  var_id_text?: string
  raw_name: string
  old_var_name: string
  new_var_name: string
  project_no: number
  project_id: number
  project_code: string
  action: string
  reason?: string
}

export type BulkRemapKioProjectsResult = {
  dry_run: boolean
  project_count: number
  created_projects: number
  updated_projects: number
  matched: number
  updated: number
  skipped: number
  items: BulkRemapKioProjectsResultItem[]
}

export type CommandAcceptedResponse = {
  gateway_id?: number
  topic?: string
  broker_accepted?: boolean
  status?: string
}

export type Project = {
  id: number
  project_code: string
  site_no: string
  edge_instance_id?: string
  name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  model_name: string
  project_group: string
  image_ref: string
  enabled: boolean
  blocked: boolean
  placeholder: boolean
  current_task_id?: number
  created_at: string
  updated_at: string
}

export type ProjectPayload = {
  project_code: string
  site_no?: string
  edge_instance_id?: string
  name?: string
  display_name?: string
  display_name_en?: string
  display_name_ja?: string
  model_name?: string
  project_group?: string
  image_ref?: string
  placeholder?: boolean
}

export type ProjectPatchPayload = Partial<{
  site_no: string
  edge_instance_id: string
  name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  model_name: string
  project_group: string
  image_ref: string
  enabled: boolean
  blocked: boolean
  placeholder: boolean
}>

export type VariableAssignmentPayload = {
  project_id?: number
  project_code?: string
  var_group: string
  enabled: boolean
}

export type TagSnapshot = {
  var_id: VarIdentifier
  var_id_text?: string
  gateway_id: number
  source_topic: string
  source_path: string
  project_id?: number
  project_code: string
  var_group: string
  var_name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  value: number
  str_value: string
  is_string: boolean
  quality: number
  last_update: string
  store_mode?: number
  store_trigger?: 'always' | 'on_detection' | string
  store_cycle_sec?: number
}

export type ActiveDetectionRun = {
  id: number
  test_no: string
  factory_no?: string
  project_id: number
  project_code: string
  mode: string
  standard_id?: number
  standard_code: string
  standard_version: number
  config_enabled?: boolean
  config_status?: string
  config_code?: string
  config_name?: string
  config_version?: number
  config_hash?: string
  config_revision?: number
}

export type DetectionRunStandardItem = DetectionStandardItem & {
  task_id: number
  test_no: string
  standard_item_id: number
  config_revision?: number
  effective_from?: string
  effective_to?: string
  variable_default_alarm_enabled: boolean
  variable_default_limit_ll?: number | null
  variable_default_limit_l?: number | null
  variable_default_limit_h?: number | null
  variable_default_limit_hh?: number | null
  variable_default_limit_deadband: number
  variable_default_violation_hold_ms: number
  variable_default_recover_hold_ms: number
  storage_name?: string
  storage_target:
    | 'none'
    | 'history_eav'
    | 'detection_form'
    | 'report_field'
    | 'wide_table'
    | string
  storage_table?: string
  storage_value_column: string
  storage_key_column: string
  storage_time_column: string
  form_field_key: string
  query_alias: string
}

export type DetectionRunStorageRoute = {
  id: number
  task_id: number
  test_no: string
  project_id: number
  project_code: string
  var_id: VarIdentifier
  var_id_text?: string
  route_id: number
  route_code: string
  storage_target:
    | 'none'
    | 'history_eav'
    | 'detection_form'
    | 'report_field'
    | 'wide_table'
    | string
  table_name: string
  column_name: string
  column_type: string
  form_field_key: string
  query_alias: string
  trigger_mode:
    | 'always'
    | 'on_detection'
    | 'on_start'
    | 'on_cycle'
    | 'on_change'
    | string
  cycle_ms: number
  deadband: number
  store_on_start: boolean
  created_at: string
}

export type DetectionRunStorageRoutesResponse = {
  items: DetectionRunStorageRoute[]
  count: number
}

export type StorageRoute = {
  id: number
  project_id: number
  var_id: VarIdentifier
  var_id_text?: string
  route_code: string
  storage_target: 'none' | 'wide_table' | string
  table_name: string
  column_name: string
  column_type: 'DOUBLE' | 'BIGINT' | 'TEXT' | 'TINYINT(1)' | string
  form_field_key: string
  query_alias: string
  trigger_mode:
    | 'always'
    | 'on_detection'
    | 'on_start'
    | 'on_cycle'
    | 'on_change'
    | string
  cycle_ms: number
  deadband: number
  store_on_start: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export type StorageRoutePayload = Partial<
  Pick<
    StorageRoute,
    | 'project_id'
    | 'var_id'
    | 'route_code'
    | 'storage_target'
    | 'table_name'
    | 'column_name'
    | 'column_type'
    | 'form_field_key'
    | 'query_alias'
    | 'trigger_mode'
    | 'cycle_ms'
    | 'deadband'
    | 'store_on_start'
    | 'enabled'
  >
>

export type StorageRouteListParams = {
  project_id?: number
  var_id?: VarIdentifier
  enabled?: boolean
}

export type DetectionRunNote = {
  id: number
  task_id: number
  note_type: string
  content: string
  actor_type: string
  actor_id: string
  created_at: string
}

export type DetectionRunReport = {
  id: number
  task_id: number
  template_id?: number
  template_code: string
  template_version: number
  file_ref: string
  file_name: string
  status: string
  generated_at?: string
  error_message: string
  created_at: string
  updated_at: string
}

export type DetectionRunReportRequestVariable = {
  var_id?: VarIdentifier
  var_name?: string
  report_name?: string
  ext_1?: string
  ext_2?: string
  ext_3?: string
}

export type DetectionRunReportRequestReportPayload = {
  template_id?: number
  template_code?: string
  template_version?: number
  report_name?: string
  variables?: DetectionRunReportRequestVariable[]
  var_ids?: VarIdentifier[]
  variable_names?: string[]
  params?: Record<string, unknown>
}

export type DetectionRunReportRequestPayload = {
  enabled?: boolean
  reports?: DetectionRunReportRequestReportPayload[]
  variables?: DetectionRunReportRequestVariable[]
  var_ids?: VarIdentifier[]
  variable_names?: string[]
  params?: Record<string, unknown>
  ext_1?: string
  ext_2?: string
  ext_3?: string
}

export type DetectionRunReportRequest = {
  id: number
  task_id: number
  test_no: string
  project_id: number
  project_code: string
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  report_name: string
  status: string
  variables?: DetectionRunReportRequestVariable[]
  params?: Record<string, unknown>
  ext_1: string
  ext_2: string
  ext_3: string
  created_at: string
  updated_at: string
}

export type DetectionRunReportRequestsResponse = {
  items: DetectionRunReportRequest[]
  count: number
}

export type DetectionRunEvent = {
  id: number
  task_id: number
  test_no: string
  project_id: number
  project_code: string
  event_type: string
  event_level: string
  message: string
  detail: string
  occurred_at: string
  created_at: string
}

export type DetectionRunSummary = {
  id: number
  task_id: number
  test_no: string
  project_id: number
  project_code: string
  result_status: 'running' | 'ok' | 'ng' | 'unknown' | string
  started_at?: string
  ended_at?: string
  duration_ms: number
  history_rows: number
  alarm_total: number
  alarm_active: number
  alarm_recovered: number
  alarm_above_h: number
  alarm_above_hh: number
  alarm_below_l: number
  alarm_below_ll: number
  first_alarm_at?: string
  last_alarm_at?: string
  last_refreshed_at: string
  created_at: string
  updated_at: string
}

export type DetectionRun = {
  id: number
  test_no: string
  factory_no: string
  customer_name: string
  device_model: string
  project_id: number
  project_code: string
  mode: string
  status: string
  standard_id?: number
  standard_code: string
  standard_version: number
  config_enabled: boolean
  config_status: string
  config_code: string
  config_name: string
  config_version: number
  config_hash: string
  current_config_revision: number
  started_at?: string
  ended_at?: string
  duration_sec: number
  expected_end_at?: string
  end_type: string
  stop_reason: string
  operator_note: string
  custom_config_json?: string
  template_ref: string
  report_template_id?: number
  report_template_code: string
  report_template_version: number
  created_at: string
  updated_at: string
  standard_items?: DetectionRunStandardItem[]
  storage_routes?: DetectionRunStorageRoute[]
  reports?: DetectionRunReport[]
  report_requests?: DetectionRunReportRequest[]
  recent_notes?: DetectionRunNote[]
}

export type DetectionRunListParams = {
  project_id?: number
  project_code?: string
  status?: string
  test_no?: string
  factory_no?: string
  start?: string
  end?: string
  limit?: number
}

export type DetectionRunListResponse = {
  items: DetectionRun[]
  count: number
  limit: number
}

export type DetectionPlan = {
  id: number
  plan_no: string
  source_system: string
  external_plan_id: string
  external_order_no: string
  factory_no: string
  customer_name: string
  device_model: string
  test_item_code: string
  test_item_name: string
  test_sequence: number
  mode: string
  standard_code: string
  status: 'pending' | 'starting' | 'started' | 'cancelled' | string
  owner_edge_instance_id: string
  owner_project_id?: number
  owner_project_code: string
  started_task_id?: number
  started_at?: string
  cancelled_at?: string
  error_message: string
  created_at: string
  updated_at: string
}

export type DetectionPlanListParams = {
  status?: string
  factory_no?: string
  keyword?: string
  limit?: number
  offset?: number
}

export type DetectionPlanListResponse = {
  items: DetectionPlan[]
  count: number
  total: number
  limit: number
  offset: number
}

export type DetectionPlanUpdatePayload = {
  plan_no: string
  source_system: string
  external_plan_id: string
  external_order_no?: string
  factory_no: string
  customer_name?: string
  device_model?: string
  test_item_code?: string
  test_item_name?: string
  test_sequence?: number
  mode?: string
  standard_code: string
}

export type DetectionPlanStartPayload = {
  project_id: number
  operator_note?: string
  request_var_id?: number | string
  request_var_name?: string
}

export type DetectionPlanStartResponse = {
  plan: DetectionPlan
  task?: DetectionRun
}

export type DetectionRunStartPayload = {
  project_id: number
  project_code?: string
  test_no?: string
  factory_no: string
  customer_name?: string
  device_model?: string
  mode?: string
  standard_id?: number
  config_enabled: boolean
  config_code?: string
  config_name?: string
  config_version?: number
  config_hash?: string
  duration_sec?: number
  operator_note?: string
  report_template_id?: number
  report_request?: DetectionRunReportRequestPayload
}

export type DetectionRunStopPayload = {
  reason?: string
}

export type DetectionRunNotePayload = {
  note_type?: string
  content: string
}

export type DetectionRunNotesResponse = {
  items: DetectionRunNote[]
  count: number
  limit: number
}

export type DetectionRunEventsResponse = {
  items: DetectionRunEvent[]
  count: number
  limit: number
}

export type DetectionStandardItem = {
  id: number
  standard_id: number
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  check_enabled: boolean
  alarm_enabled: boolean
  store_enabled: boolean
  check_cycle_ms: number
  check_on_start: boolean
  required: boolean
  check_method:
    | 'numeric_range'
    | 'bool_equals'
    | 'string_equals'
    | 'regex'
    | string
  target_value: string
  limit_ll?: number | null
  limit_l?: number | null
  limit_h?: number | null
  limit_hh?: number | null
  limit_deadband: number
  violation_hold_ms: number
  recover_hold_ms: number
  quality_policy: 'ignore_bad' | 'record_invalid' | 'fail_on_bad' | string
  unit: string
  decimal_places: number
  sort_order: number
  created_at: string
  updated_at: string
}

export type DetectionStandard = {
  id: number
  standard_code: string
  name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  project_id?: number
  project_code: string
  project_group: string
  mode: string
  report_template_id?: number
  version: number
  config_hash: string
  enabled: boolean
  remark: string
  created_at: string
  updated_at: string
  items?: DetectionStandardItem[]
}

export type DetectionStandardItemPayload = {
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  display_name?: string
  display_name_en?: string
  display_name_ja?: string
  check_enabled?: boolean
  alarm_enabled?: boolean
  store_enabled?: boolean
  check_cycle_ms?: number
  check_on_start?: boolean
  required?: boolean
  check_method?:
    | 'numeric_range'
    | 'bool_equals'
    | 'string_equals'
    | 'regex'
    | string
  target_value?: string
  limit_ll?: number | null
  limit_l?: number | null
  limit_h?: number | null
  limit_hh?: number | null
  limit_deadband?: number
  violation_hold_ms?: number
  recover_hold_ms?: number
  quality_policy?: 'ignore_bad' | 'record_invalid' | 'fail_on_bad' | string
  unit?: string
  decimal_places?: number
  sort_order?: number
}

export type DetectionStandardPayload = Partial<
  Pick<
    DetectionStandard,
    | 'standard_code'
    | 'name'
    | 'display_name'
    | 'display_name_en'
    | 'display_name_ja'
    | 'project_id'
    | 'project_code'
    | 'project_group'
    | 'mode'
    | 'report_template_id'
    | 'version'
    | 'enabled'
    | 'remark'
  >
> & {
  items?: DetectionStandardItemPayload[]
}

export type DetectionStandardListParams = {
  project_id?: number
  project_code?: string
  project_group?: string
  mode?: string
  enabled?: boolean
  keyword?: string
}

export type RealtimeWebSocketTopic =
  | 'realtime.variables'
  | 'detection.runs'
  | 'notifications'

export type RealtimeWebSocketEnvelope<TPayload = unknown> = {
  type:
    | 'connection.ready'
    | 'subscription.updated'
    | 'realtime.variables.snapshot'
    | 'detection.runs.snapshot'
    | 'notification.event'
    | 'heartbeat'
    | 'command.ack'
    | 'error'
    | string
  request_id?: string
  command_id?: string
  at: string
  payload?: TPayload
  error?: {
    code: string
    message: string
  }
}

export type RealtimeWebSocketSubscription = {
  topics: RealtimeWebSocketTopic[]
  edge_instance_id?: string
  source_type?: 'mqtt' | 'virtual' | 'manual' | string
  gateway_id?: number
  project_id?: number
  var_ids?: VarIdentifier[]
  var_id_texts?: string[]
}

export type RealtimeVariableListParams = {
  edge_instance_id?: string
  source_type?: 'mqtt' | 'virtual' | 'manual' | string
  gateway_id?: number
  project_id?: number
  var_id?: VarIdentifier | VarIdentifier[]
}

export type StationViewProjectRef = {
  id: number
  project_code: string
  name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  model_name: string
}

export type StationViewTemplateRef = {
  template_uid: string
  template_code: string
  name: string
  display_name: string
  display_name_en: string
  display_name_ja: string
  version: number
  status: 'draft' | 'published' | 'disabled' | string
  owner_scope: string
  layout_json?: string
}

export type StationViewAssignment = {
  id: number
  template_uid: string
  target_type: 'global' | 'edge' | 'model' | 'project' | string
  target_key: string
  priority: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export type StationViewTemplateListItem = StationViewTemplateRef & {
  id: number
  assignments?: StationViewAssignment[]
  created_at?: string
  updated_at?: string
}

export type StationViewTemplatesResponse = {
  edge_instance_id?: string
  items: StationViewTemplateListItem[]
  count: number
}

export type StationViewDiagnostics = {
  status: string
  template_count: number
  published_templates: number
  region_count: number
  item_count: number
  assignment_count: number
  enabled_assignments: number
  default_template_ready: boolean
  warnings?: string[]
}

export type StationViewReloadResponse = {
  ok: boolean
  edge_instance_id?: string
  reload_mode: string
  diagnostics: StationViewDiagnostics
  effective?: StationViewEffectiveResponse
}

export type StationViewLayoutArea = 'card_pool' | 'list_layout'

export type StationViewRegion = {
  layout_area: StationViewLayoutArea | string
  layout_type: string
  layout_json?: string
  sort_order: number
}

export type StationViewResolvedBinding = {
  source: 'project_variable' | 'detection_item' | string
  var_id?: VarIdentifier
  var_id_text?: string
  var_name?: string
  var_group?: string
  display_name?: string
  display_name_en?: string
  display_name_ja?: string
  data_type?: string
  unit?: string
  decimal_places: number
  limit_ll?: number | null
  limit_l?: number | null
  limit_h?: number | null
  limit_hh?: number | null
  check_enabled?: boolean
  alarm_enabled?: boolean
  sort_order: number
}

export type StationViewItem = {
  item_uid: string
  layout_area: StationViewLayoutArea | string
  item_type: string
  binding_type:
    | 'var_name'
    | 'var_group'
    | 'detection_items'
    | 'alarm_summary'
    | 'run_state'
    | 'manual'
    | string
  binding_key: string
  binding_json?: string
  display_json?: string
  sort_order: number
  pinned: boolean
  visible: boolean
  resolved_bindings?: StationViewResolvedBinding[]
}

export type StationViewItemsResponse = {
  edge_instance_id?: string
  template_uid: string
  items: StationViewItem[]
  count: number
}

export type StationViewItemPayload = {
  item_uid: string
  layout_area: StationViewLayoutArea | string
  item_type: string
  binding_type:
    | 'var_name'
    | 'var_group'
    | 'detection_items'
    | 'alarm_summary'
    | 'run_state'
    | 'manual'
    | string
  binding_key: string
  binding_json?: string
  display_json?: string
  sort_order: number
  pinned?: boolean
  visible: boolean
}

export type StationViewItemsReplacePayload = {
  template_uid: string
  items: StationViewItemPayload[]
}

export type StationViewEffectiveResponse = {
  edge_instance_id: string
  project: StationViewProjectRef
  template: StationViewTemplateRef
  regions: StationViewRegion[]
  items: StationViewItem[]
  ws_subscription: RealtimeWebSocketSubscription
  http_companion: {
    current_run_required: boolean
    alarm_summary: boolean
  }
  warnings?: string[]
}

export type RealtimeVariablesSnapshotPayload = {
  items: TagSnapshot[]
}

export type DetectionRunsSnapshotPayload = {
  items: ActiveDetectionRun[]
}

export type RuntimeNotification = {
  id: string
  type: string
  level: 'info' | 'success' | 'warning' | 'error' | string
  target_type?: 'all' | 'user' | 'role' | 'project' | string
  target_id?: string
  project_id: number
  project_code?: string
  task_id?: number
  test_no?: string
  var_id?: VarIdentifier
  var_id_text?: string
  var_name?: string
  display_name?: string
  message: string
  payload?: Record<string, unknown>
  occurred_at: string
}

export type RealtimeWebSocketCommand<TPayload = unknown> = {
  type:
    | 'command.detection.start'
    | 'command.detection.stop'
    | 'command.detection.abnormal_stop'
    | 'command.write_variable'
    | string
  request_id: string
  command_id: string
  payload: TPayload
}

export type KIOWriteResult = {
  gateway_id: number
  topic: string
  ack_topic?: string
  qid: number
  broker_accepted: boolean
  project_confirmed?: boolean
  Project_confirmed?: boolean
  process_step?: number
  result?: string
  message?: string
  status:
    | 'published_unconfirmed'
    | 'confirmed'
    | 'rejected'
    | 'ack_timeout_or_unmatched'
    | string
}

export type VariableWriteResult = {
  var_id: VarIdentifier
  var_id_text: string
  var_name: string
  source_type: 'mqtt' | 'virtual' | 'manual' | string
  project_id?: number
  value?: unknown
  quality?: number
  updated_at?: string
  triggered?: number
  origin_flow_id?: number
  origin_run_id?: number
  depth?: number
  next_depth?: number
  max_depth?: number
  allow_reentrant?: boolean
  request_id?: string
  broker_accepted?: boolean
  project_confirmed?: boolean
  kio?: KIOWriteResult
}

export type HistoryDataItem = {
  id: number
  gateway_id: number
  topic: string
  project_id: number
  task_id: number
  test_no: string
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  project_code: string
  value?: number | null
  str_value?: string | null
  quality: number
  source_time: string
  created_at: string
}

export type HistoryDataResponse = {
  items: HistoryDataItem[]
  limit: number
  count: number
}

export type HistoryDataParams = {
  project_id?: number
  project_code?: string
  task_id?: number
  test_no?: string
  factory_no?: string
  start?: string
  end?: string
  limit?: number
}

export type ReportTemplate = {
  id: number
  template_code: string
  name: string
  display_name: string
  file_ref: string
  file_kind: string
  file_sha256?: string
  file_size?: number
  version: number
  params_schema_json?: string
  enabled: boolean
  remark: string
  created_at: string
  updated_at: string
}

export type ReportTemplatePayload = Partial<
  Pick<
    ReportTemplate,
    | 'template_code'
    | 'name'
    | 'display_name'
    | 'file_ref'
    | 'file_kind'
    | 'version'
    | 'enabled'
    | 'remark'
  >
>

export type ReportTemplateListParams = {
  enabled?: boolean
  keyword?: string
}

export type ReportTemplateListResponse = {
  items: ReportTemplate[]
  count: number
}

export type ReportTemplateUploadPayload = {
  file: File
  template_code: string
  name?: string
  display_name?: string
  version?: number
  params_schema_json?: string
  remark?: string
  enabled?: boolean
}

export type ReportTemplateUploadResponse = {
  template: ReportTemplate
  artifact: {
    key: string
    content_type: string
    size: number
    sha256: string
  }
}

export type ReportTemplateMappingPayload = {
  params_schema_json?: string
  mapping?: unknown
}

export type PlanImportIssue = {
  row_number?: number
  field: string
  code: string
  message: string
}

export type PlanImportLimit = {
  raw: string
  limit_l?: number
  limit_h?: number
  unit?: string
  mode?: string
  normalized?: string
  confidence: number
  needs_confirmation: boolean
  error?: string
}

export type PlanImportRow = {
  row_number: number
  project_code?: string
  project_name?: string
  project_group?: string
  project_match?: {
    project_id: number
    project_code: string
    project_group: string
    name: string
    edge_instance_id: string
    confidence: number
  }
  test_no?: string
  factory_no?: string
  customer_name?: string
  device_model?: string
  variable_raw?: string
  var_id_text?: string
  variable_match?: {
    var_id: number
    var_id_text: string
    var_name: string
    display_name: string
    unit?: string
    confidence: number
  }
  limit_raw?: string
  limit: PlanImportLimit
  setting_raw?: string
  check_enabled_raw?: string
  check_enabled: boolean
  formula_json?: string
  unit?: string
  template_code?: string
  template_match?: {
    template_id: number
    template_code: string
    version: number
    file_ref: string
    confidence: number
  }
  report_name?: string
  params?: Record<string, string>
  needs_confirm: boolean
  issues?: PlanImportIssue[]
  normalized_input?: Record<string, string>
}

export type PlanImportDraft = {
  artifact: {
    artifact_key: string
    content_type: string
    size: number
    sha256: string
  }
  source_file_name: string
  sheet_name: string
  rows: PlanImportRow[]
  summary: {
    total_rows: number
    ready_rows: number
    rows_with_issues: number
    project_matched_rows: number
    variable_matched_rows: number
    template_matched_rows: number
    limit_parsed_rows: number
    needs_confirmation: number
  }
  issues?: PlanImportIssue[]
  parsed_at: string
}

export type PlanImportCellMappingRow = {
  row_number?: number
  fields?: Record<string, string>
  values?: Record<string, string>
  params?: Record<string, string>
}

export type PlanImportCellMapping = {
  sheet?: string
  common?: Record<string, string>
  rows?: PlanImportCellMappingRow[]
}

export type PlanImportConfirmPayload = {
  rows: PlanImportRow[]
  source_artifact_key?: string
  edge_instance_id?: string
  allow_needs_confirmation: boolean
}

export type PlanImportConfirmResult = {
  standards: DetectionStandard[]
  plans: DetectionPlan[]
  issues?: PlanImportIssue[]
  created_standards: number
  created_plans: number
  plan_creation_status: string
  plan_creation_note: string
}

export type MainReportJobStatus =
  | 'pending'
  | 'waiting_for_sync'
  | 'generating'
  | 'succeeded'
  | 'failed'
  | 'waiting'
  | 'running'
  | 'success'
  | string

export type MainReportJob = {
  id: number
  job_key: string
  parent_job_id?: number
  generation_type?: string
  edge_instance_id: string
  task_id: number
  request_id: number
  test_no: string
  project_id: number
  project_code: string
  template_id?: number
  template_code: string
  template_version: number
  report_name: string
  status: MainReportJobStatus
  readiness_status: string
  attempts: number
  max_attempts: number
  next_run_at?: string
  locked_at?: string
  started_at?: string
  finished_at?: string
  last_checked_at?: string
  artifact_ref: string
  artifact_name: string
  params_override_json?: string
  error_message: string
  created_at: string
  updated_at: string
}

export type MainReportJobListParams = {
  status?: MainReportJobStatus
  task_id?: number
  edge_instance_id?: string
  limit?: number
  offset?: number
}

export type MainReportJobListResponse = {
  items: MainReportJob[]
  total: number
  limit: number
  offset: number
}

export type MainReportJobEvent = {
  id: number
  job_id: number
  event_type: string
  level: string
  message: string
  payload?: unknown
  created_at: string
}

export type MainReportJobEventsResponse = {
  items: MainReportJobEvent[]
  count: number
  limit: number
}

export type MainReportReadinessResponse = {
  role: string
  edge_instance_id: string
  sync_database: string
  readiness: Record<string, unknown>
}

export type MainReportEnqueuePayload = {
  task_id: number
  force?: boolean
  edge_instance_id?: string
}

export type MainReportEnqueueResponse = {
  jobs: MainReportJob[]
  readiness: Record<string, unknown>
  requests: unknown[]
}

export type MainReportRegeneratePayload = {
  params?: Record<string, unknown>
  params_json?: string
  reason?: string
}

export type MainReportArtifact = {
  blob: Blob
  filename: string
  contentType: string
}

export type TaskFlowVarRole = 'watch' | 'read' | 'write' | string

export type TaskFlowTriggerType =
  | 'data_change'
  | 'schedule'
  | 'project_start'
  | 'project_end'
  | 'manual'
  | string

export type TaskFlowActionType =
  | 'builtin.storage_snapshot'
  | 'javascript'
  | string

export type TaskFlowParamBinding = {
  source?: 'literal' | 'trigger_param' | 'event' | 'context' | string
  key?: string
  value?: unknown
  default?: unknown
  optional?: boolean
}

export type TaskFlowStep = {
  code?: string
  module: TaskFlowActionType
  params?: Record<string, unknown | TaskFlowParamBinding>
  script?: string
}

export type TaskFlowModule = {
  code: TaskFlowActionType
  name: string
  category: string
  trigger_types: string[]
  params_schema: Record<string, unknown>
}

export type TaskFlowTemplate = {
  template_code: string
  name: string
  category: string
  trigger_type: TaskFlowTriggerType
  description: string
  watch_vars?: Array<{ role: TaskFlowVarRole; description?: string }>
  steps: TaskFlowStep[]
  condition_script?: string
}

export type TaskFlowVar = {
  id?: number
  flow_id?: number
  project_id?: number
  var_id: VarIdentifier
  var_id_text?: string
  var_name: string
  role: TaskFlowVarRole
  created_at?: string
}

export type TaskFlow = {
  id: number
  project_id: number
  flow_code: string
  name: string
  enabled: boolean
  trigger_type: TaskFlowTriggerType
  condition_script: string
  action_type: TaskFlowActionType
  action_script: string
  action_payload: string
  steps_json: string
  timeout_ms: number
  cooldown_ms: number
  hold_ms: number
  schedule_interval_ms?: number
  priority: number
  remark: string
  vars?: TaskFlowVar[]
  created_at: string
  updated_at: string
}

export type TaskFlowPayload = Partial<
  Pick<
    TaskFlow,
    | 'project_id'
    | 'flow_code'
    | 'name'
    | 'enabled'
    | 'trigger_type'
    | 'condition_script'
    | 'action_type'
    | 'action_script'
    | 'action_payload'
    | 'steps_json'
    | 'timeout_ms'
    | 'cooldown_ms'
    | 'hold_ms'
    | 'schedule_interval_ms'
    | 'priority'
    | 'remark'
  >
> & {
  vars?: Array<Pick<TaskFlowVar, 'var_id' | 'var_name' | 'role'>>
}

export type TaskFlowListParams = {
  project_id?: number
  trigger_type?: string
  enabled?: boolean
}

export type TaskFlowRunAccepted = {
  status: 'queued' | string
}

export type TaskFlowRun = {
  id: number
  flow_id: number
  flow_code: string
  project_id: number
  trigger_type: string
  trigger_var_id: VarIdentifier
  trigger_var_id_text?: string
  origin_flow_id: number
  origin_run_id: number
  depth: number
  status: string
  started_at: string
  finished_at?: string
  duration_ms: number
  input_snapshot: string
  result_json: string
  error_message: string
  script_logs: string
  created_at: string
  updated_at: string
}

export type TaskFlowRunListParams = {
  project_id?: number
  flow_id?: number
  trigger_type?: string
  trigger_var_id?: VarIdentifier
  origin_flow_id?: number
  status?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export type TaskFlowRunListResponse = {
  items: TaskFlowRun[]
  total: number
  limit: number
  offset: number
}

export type TaskFlowSqlLog = {
  id: number
  run_id: number
  flow_id: number
  sql_text: string
  sql_args: string
  affected_rows: number
  duration_ms: number
  error_message: string
  created_at: string
}

export type LoginRequest = {
  username: string
  password: string
}

export type LoginResponse = {
  access_token: string
  token_type: string
  expires_in?: number
  user: {
    id?: number
    username: string
    role: string
    display_name?: string
    permissions_version?: number
  }
  permissions?: string[]
}

export type AuthMeResponse = {
  user: {
    id: number
    username: string
    role: string
    permissions_version: number
  }
  permissions: string[]
}

export type LogoutResponse = {
  status: string
}

export type SsoTicketResponse = {
  ticket: string
  expires_in: number
  edge_instance_id: string
  main_site_url: string
}

export type RoleName = 'guest' | 'admin' | 'developer'

export type SystemUser = {
  id: number
  username: string
  role: RoleName | string
  enabled: boolean
  permissions_version: number
  permissions: string[]
  last_login_at?: string | null
  created_at: string
  updated_at: string
}

export type UserCreatePayload = {
  username: string
  password: string
  role: RoleName
  enabled?: boolean
}

export type UserPatchPayload = Partial<{
  username: string
  role: RoleName
  enabled: boolean
}>

export type UserResetPasswordPayload = {
  password: string
}

export type ProjectMember = {
  id: number
  project_id: number
  user_id: number
  username: string
  user_role: string
  user_enabled: boolean
  member_role: string
  notify_enabled: boolean
  created_at: string
  updated_at: string
}

export type ProjectMemberUpdate = {
  user_id: number
  member_role?: string
  notify_enabled?: boolean
}

export type ProjectMembersResponse = {
  items: ProjectMember[]
  count: number
}
