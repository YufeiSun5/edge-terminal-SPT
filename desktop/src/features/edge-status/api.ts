import { deleteJson, getJson, patchJson, postJson, putJson } from "@/shared/api/http";
import type {
  ActiveDetectionRun,
  AuditLogListParams,
  AuditLogListResponse,
  BulkRemapKioProjectsPayload,
  BulkRemapKioProjectsResult,
  ChannelStats,
  CommandAcceptedResponse,
  Device,
  DevicePatchPayload,
  DevicePayload,
  DetectionRun,
  DetectionRunEventsResponse,
  DetectionRunListParams,
  DetectionRunListResponse,
  DetectionRunNotePayload,
  DetectionRunNotesResponse,
  DetectionRunStartPayload,
  DetectionRunStopPayload,
  DetectionRunSummary,
  DetectionStandard,
  DetectionStandardItemPayload,
  DetectionStandardListParams,
  DetectionStandardPayload,
  DatabaseConfig,
  DatabaseConfigPayload,
  DatabaseConfigTestResult,
  GatewayConfig,
  GatewayConfigPayload,
  GatewayStatusMap,
  HealthResponse,
  NotificationListParams,
  NotificationListResponse,
  NotificationUnreadCount,
  RealtimeVariableListParams,
  TagSnapshot,
  ReportTemplate,
  ReportTemplateListParams,
  ReportTemplatePayload,
  StorageRoute,
  StorageRouteListParams,
  StorageRoutePayload,
  TaskFlow,
  TaskFlowListParams,
  TaskFlowModule,
  TaskFlowPayload,
  TaskFlowRun,
  TaskFlowRunAccepted,
  TaskFlowRunListParams,
  TaskFlowRunListResponse,
  TaskFlowSqlLog,
  TaskFlowTemplate,
  VariableAssignmentPayload,
  VariableConfig,
  VariableCreatePayload,
  VariableListParams,
  VariablePatchPayload,
} from "@/shared/api/types";

export function getHealth() {
  return getJson<HealthResponse>("/health");
}

export function getChannels() {
  return getJson<ChannelStats>("/api/v1/runtime/channels");
}

export function getDatabaseConfig() {
  return getJson<DatabaseConfig>("/api/v1/system/database-config");
}

export function updateDatabaseConfig(payload: DatabaseConfigPayload) {
  return patchJson<DatabaseConfig, DatabaseConfigPayload>(
    "/api/v1/system/database-config",
    payload,
  );
}

export function testDatabaseConfig(payload: DatabaseConfigPayload) {
  return postJson<DatabaseConfigTestResult, DatabaseConfigPayload>(
    "/api/v1/system/database-config/test",
    payload,
  );
}

export function getAuditLogs(params: AuditLogListParams = {}) {
  const query = new URLSearchParams();
  if (params.actor_type) query.set("actor_type", params.actor_type);
  if (params.actor_id) query.set("actor_id", params.actor_id);
  if (params.action) query.set("action", params.action);
  if (params.target_type) query.set("target_type", params.target_type);
  if (params.target_id) query.set("target_id", params.target_id);
  if (params.result) query.set("result", params.result);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  if (params.created_from) query.set("created_from", params.created_from);
  if (params.created_to) query.set("created_to", params.created_to);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<AuditLogListResponse>(`/api/v1/audit-logs${suffix ? `?${suffix}` : ""}`);
}

export function getNotifications(params: NotificationListParams = {}) {
  const query = new URLSearchParams();
  if (params.unread !== undefined) query.set("unread", String(params.unread));
  if (params.type) query.set("type", params.type);
  if (params.level) query.set("level", params.level);
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<NotificationListResponse>(`/api/v1/notifications${suffix ? `?${suffix}` : ""}`);
}

export function getNotificationUnreadCount() {
  return getJson<NotificationUnreadCount>("/api/v1/notifications/unread-count");
}

export function markNotificationRead(notificationId: number) {
  return postJson<{ ok: true }, Record<string, never>>(`/api/v1/notifications/${notificationId}/read`, {});
}

export function markAllNotificationsRead() {
  return postJson<{ updated: number }, Record<string, never>>("/api/v1/notifications/read-all", {});
}

export function getGateways() {
  return getJson<GatewayStatusMap>("/api/v1/gateways");
}

export function getGatewayConfigs() {
  return getJson<GatewayConfig[]>("/api/v1/gateway-configs");
}

export function getDevices() {
  return getJson<Device[]>("/api/v1/projects").then((items) => items.map(withDeviceAliases));
}

export function createDevice(payload: DevicePayload) {
  return postJson<Device, Record<string, unknown>>("/api/v1/projects", withProjectPayload(payload)).then(withDeviceAliases);
}

export function updateDevice(deviceId: number, payload: DevicePatchPayload) {
  return patchJson<Device, DevicePatchPayload>(`/api/v1/projects/${deviceId}`, payload).then(withDeviceAliases);
}

export function createGatewayConfig(payload: GatewayConfigPayload) {
  return postJson<GatewayConfig, GatewayConfigPayload>(
    "/api/v1/gateway-configs",
    payload,
  );
}

export function updateGatewayConfig(
  gatewayId: number,
  payload: GatewayConfigPayload,
) {
  return patchJson<GatewayConfig, GatewayConfigPayload>(
    `/api/v1/gateway-configs/${gatewayId}`,
    payload,
  );
}

export function deleteGatewayConfig(gatewayId: number) {
  return deleteJson<{ status: string }>(`/api/v1/gateway-configs/${gatewayId}`);
}

export function discoverGatewayVariables(gatewayId: number) {
  return postJson<CommandAcceptedResponse, Record<string, never>>(
    `/api/v1/gateway-configs/${gatewayId}/discover`,
    {},
  );
}

export function getVariables(params: VariableListParams = {}) {
  const query = new URLSearchParams();
  if (params.gateway_id !== undefined)
    query.set("gateway_id", String(params.gateway_id));
  if (params.enabled !== undefined)
    query.set("enabled", String(params.enabled));
  if (params.discovered !== undefined)
    query.set("discovered", String(params.discovered));
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<VariableConfig[]>(
    `/api/v1/variables${suffix ? `?${suffix}` : ""}`,
  ).then((items) => items.map(withVariableAliases));
}

export function createVariable(payload: VariableCreatePayload) {
  return postJson<VariableConfig, Record<string, unknown>>(
    "/api/v1/variables",
    withProjectPayload(withoutVariableStoragePayload(payload)),
  ).then(withVariableAliases);
}

export function updateVariable(
  variableId: number,
  payload: VariablePatchPayload,
) {
  return patchJson<VariableConfig, Record<string, unknown>>(
    `/api/v1/variables/${variableId}`,
    withoutVariableStoragePayload(payload),
  ).then(withVariableAliases);
}

export function assignVariable(
  variableId: number,
  payload: VariableAssignmentPayload,
) {
  return patchJson<{ status: string }, Record<string, unknown>>(
    `/api/v1/variables/${variableId}/assignment`,
    withProjectPayload(payload),
  );
}

export function deleteVariable(variableId: number) {
  return deleteJson<{ status: string }>(`/api/v1/variables/${variableId}`);
}

export function bulkRemapKioProjects(payload: BulkRemapKioProjectsPayload = {}) {
  return postJson<BulkRemapKioProjectsResult, BulkRemapKioProjectsPayload>(
    "/api/v1/variables/bulk-remap/kio-projects",
    payload,
  );
}

export function getStorageRoutes(params: StorageRouteListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  else if (params.device_id !== undefined) query.set("project_id", String(params.device_id));
  if (params.var_id !== undefined) query.set("var_id", String(params.var_id));
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  const suffix = query.toString();
  return getJson<StorageRoute[]>(`/api/v1/storage-routes${suffix ? `?${suffix}` : ""}`)
    .then((items) => items.map(withStorageRouteAliases));
}

export function createStorageRoute(payload: StorageRoutePayload) {
  return postJson<StorageRoute, Record<string, unknown>>("/api/v1/storage-routes", withProjectPayload(payload))
    .then(withStorageRouteAliases);
}

export function updateStorageRoute(routeId: number, payload: StorageRoutePayload) {
  return patchJson<StorageRoute, Record<string, unknown>>(`/api/v1/storage-routes/${routeId}`, withoutProjectAliases(payload))
    .then(withStorageRouteAliases);
}

export function deleteStorageRoute(routeId: number) {
  return deleteJson<{ status: string }>(`/api/v1/storage-routes/${routeId}`);
}

export function getRealtimeVariables(params: RealtimeVariableListParams = {}) {
  const query = new URLSearchParams();
  if (params.source_type) query.set("source_type", params.source_type);
  if (params.gateway_id !== undefined) query.set("gateway_id", String(params.gateway_id));
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  else if (params.device_id !== undefined) query.set("project_id", String(params.device_id));
  if (params.var_id !== undefined) {
    const varIds = Array.isArray(params.var_id) ? params.var_id : [params.var_id];
    for (const varId of varIds) query.append("var_id", String(varId));
  }
  const suffix = query.toString();
  return getJson<TagSnapshot[]>(`/api/v1/realtime/variables${suffix ? `?${suffix}` : ""}`).then((items) => items.map(withTagSnapshotAliases));
}

export function getActiveDetectionRuns() {
  return getJson<ActiveDetectionRun[]>("/api/v1/detection-runs/active").then((items) => items.map(withRunAliases));
}

export function getDetectionRuns(params: DetectionRunListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  else if (params.device_id !== undefined) query.set("project_id", String(params.device_id));
  if (params.project_code) query.set("project_code", params.project_code);
  if (params.status) query.set("status", params.status);
  if (params.test_no) query.set("test_no", params.test_no);
  if (params.start) query.set("start", params.start);
  if (params.end) query.set("end", params.end);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  const suffix = query.toString();
  return getJson<DetectionRunListResponse>(`/api/v1/detection-runs${suffix ? `?${suffix}` : ""}`)
    .then((response) => ({ ...response, items: response.items.map(withRunAliases) }));
}

export function getDetectionRun(runId: number) {
  return getJson<DetectionRun>(`/api/v1/detection-runs/${runId}`).then(withRunAliases);
}

export function getCurrentDetectionRun(projectId: number) {
  const query = new URLSearchParams({ project_id: String(projectId) });
  return getJson<DetectionRun>(`/api/v1/detection-runs/current?${query.toString()}`).then(withRunAliases);
}

export function getDetectionRunSummary(runId: number) {
  return getJson<DetectionRunSummary>(`/api/v1/detection-runs/${runId}/summary`).then(withRunAliases);
}

export function getDetectionRunEvents(runId: number, limit = 200) {
  return getJson<DetectionRunEventsResponse>(`/api/v1/detection-runs/${runId}/events?limit=${limit}`)
    .then((response) => ({ ...response, items: response.items.map(withRunAliases) }));
}

export function startDetectionRun(payload: DetectionRunStartPayload) {
  return postJson<DetectionRun, Record<string, unknown>>("/api/v1/detection-runs", withProjectPayload(payload))
    .then(withRunAliases);
}

export function stopDetectionRun(runId: number, payload: DetectionRunStopPayload = {}) {
  return postJson<DetectionRun, DetectionRunStopPayload>(`/api/v1/detection-runs/${runId}/stop`, payload);
}

export function abnormalStopDetectionRun(runId: number, payload: Required<DetectionRunStopPayload>) {
  return postJson<DetectionRun, Required<DetectionRunStopPayload>>(`/api/v1/detection-runs/${runId}/abnormal-stop`, payload);
}

export function getDetectionRunNotes(runId: number, limit = 200) {
  return getJson<DetectionRunNotesResponse>(`/api/v1/detection-runs/${runId}/notes?limit=${limit}`);
}

export function addDetectionRunNote(runId: number, payload: DetectionRunNotePayload) {
  return postJson<DetectionRunNotesResponse["items"][number], DetectionRunNotePayload>(`/api/v1/detection-runs/${runId}/notes`, payload);
}

export function getDetectionStandards(params: DetectionStandardListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  else if (params.device_id !== undefined) query.set("project_id", String(params.device_id));
  if (params.project_code) query.set("project_code", params.project_code);
  else if (params.device_code) query.set("project_code", params.device_code);
  if (params.mode) query.set("mode", params.mode);
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<DetectionStandard[]>(`/api/v1/detection-standards${suffix ? `?${suffix}` : ""}`)
    .then((items) => items.map(withStandardAliases));
}

export function getDetectionStandard(standardId: number) {
  return getJson<DetectionStandard>(`/api/v1/detection-standards/${standardId}`).then(withStandardAliases);
}

export function createDetectionStandard(payload: DetectionStandardPayload) {
  return postJson<DetectionStandard, Record<string, unknown>>("/api/v1/detection-standards", withProjectPayload(payload))
    .then(withStandardAliases);
}

export function updateDetectionStandard(standardId: number, payload: DetectionStandardPayload) {
  return patchJson<DetectionStandard, Record<string, unknown>>(
    `/api/v1/detection-standards/${standardId}`,
    withProjectPayload(payload),
  ).then(withStandardAliases);
}

export function replaceDetectionStandardItems(
  standardId: number,
  items: DetectionStandardItemPayload[],
) {
  return putJson<DetectionStandard, { items: DetectionStandardItemPayload[] }>(
    `/api/v1/detection-standards/${standardId}/items`,
    { items },
  ).then(withStandardAliases);
}

export function deleteDetectionStandard(standardId: number) {
  return deleteJson<{ status: string }>(`/api/v1/detection-standards/${standardId}`);
}

export function getReportTemplates(params: ReportTemplateListParams = {}) {
  const query = new URLSearchParams();
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<ReportTemplate[]>(`/api/v1/report-templates${suffix ? `?${suffix}` : ""}`);
}

export function createReportTemplate(payload: ReportTemplatePayload) {
  return postJson<ReportTemplate, ReportTemplatePayload>("/api/v1/report-templates", payload);
}

export function updateReportTemplate(templateId: number, payload: ReportTemplatePayload) {
  return patchJson<ReportTemplate, ReportTemplatePayload>(`/api/v1/report-templates/${templateId}`, payload);
}

export function deleteReportTemplate(templateId: number) {
  return deleteJson<{ status: string }>(`/api/v1/report-templates/${templateId}`);
}

export function getTaskFlows(params: TaskFlowListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.trigger_type) query.set("trigger_type", params.trigger_type);
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  const suffix = query.toString();
  return getJson<TaskFlow[]>(`/api/v1/task-flows${suffix ? `?${suffix}` : ""}`);
}

export function getTaskModules() {
  return getJson<TaskFlowModule[]>("/api/v1/task-modules");
}

export function getTaskFlowTemplates() {
  return getJson<TaskFlowTemplate[]>("/api/v1/task-flow-templates");
}

export function createTaskFlow(payload: TaskFlowPayload) {
  return postJson<TaskFlow, TaskFlowPayload>("/api/v1/task-flows", payload);
}

export function updateTaskFlow(flowId: number, payload: TaskFlowPayload) {
  return patchJson<TaskFlow, TaskFlowPayload>(`/api/v1/task-flows/${flowId}`, payload);
}

export function runTaskFlow(flowId: number) {
  return postJson<TaskFlowRunAccepted, Record<string, never>>(`/api/v1/task-flows/${flowId}/run`, {});
}

export function getTaskFlowRuns(params: TaskFlowRunListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.flow_id !== undefined) query.set("flow_id", String(params.flow_id));
  if (params.trigger_type) query.set("trigger_type", params.trigger_type);
  if (params.trigger_var_id !== undefined) query.set("trigger_var_id", String(params.trigger_var_id));
  if (params.origin_flow_id !== undefined) query.set("origin_flow_id", String(params.origin_flow_id));
  if (params.status) query.set("status", params.status);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<TaskFlowRunListResponse>(`/api/v1/task-flow-runs${suffix ? `?${suffix}` : ""}`);
}

export function getTaskFlowRun(flowRunId: number) {
  return getJson<TaskFlowRun>(`/api/v1/task-flow-runs/${flowRunId}`);
}

export function getTaskFlowSqlLogs(flowRunId: number, limit = 100) {
  const query = new URLSearchParams({ limit: String(limit) });
  return getJson<TaskFlowSqlLog[]>(`/api/v1/task-flow-runs/${flowRunId}/sql-logs?${query.toString()}`);
}

function withDeviceAliases<T extends { project_code?: string; device_code?: string }>(item: T): T {
  return {
    ...item,
    device_code: item.device_code || item.project_code || "",
  };
}

function withVariableAliases<T extends VariableConfig>(item: T): T {
  return {
    ...item,
    device_id: item.device_id ?? item.project_id,
    device_code: item.device_code || item.project_code || "",
  };
}

function withTagSnapshotAliases<T extends TagSnapshot>(item: T): T {
  return {
    ...item,
    device_id: item.device_id ?? item.project_id,
    device_code: item.device_code || item.project_code || "",
  };
}

function withStorageRouteAliases<T extends { project_id?: number; device_id?: number }>(item: T): T {
  return {
    ...item,
    device_id: item.device_id ?? item.project_id,
  };
}

function withRunAliases<T extends { project_id?: number; project_code?: string; device_id?: number; device_code?: string; storage_routes?: Array<{ project_id?: number; device_id?: number }> }>(item: T): T {
  return {
    ...item,
    device_id: item.device_id ?? item.project_id,
    device_code: item.device_code || item.project_code || "",
    storage_routes: item.storage_routes?.map(withStorageRouteAliases),
  };
}

function withStandardAliases<T extends DetectionStandard>(item: T): T {
  return {
    ...item,
    device_id: item.device_id ?? item.project_id,
    device_code: item.device_code || item.project_code || "",
  };
}

function withProjectPayload<T extends Record<string, unknown>>(payload: T): Record<string, unknown> {
  const next: Record<string, unknown> = { ...payload };
  if (next.device_id !== undefined && next.project_id === undefined) next.project_id = next.device_id;
  if (next.device_code !== undefined && next.project_code === undefined) next.project_code = next.device_code;
  if (next.device_code !== undefined) delete next.device_code;
  if (next.device_id !== undefined) delete next.device_id;
  return next;
}

function withoutProjectAliases<T extends Record<string, unknown>>(payload: T): Record<string, unknown> {
  const next: Record<string, unknown> = { ...payload };
  delete next.device_id;
  delete next.device_code;
  return next;
}

function withoutVariableStoragePayload(payload: Record<string, unknown>): Record<string, unknown> {
  const next = withoutProjectAliases(payload);
  for (const key of [
    "store_mode",
    "store_trigger",
    "store_cycle_sec",
    "store_deadband",
    "storage_name",
    "storage_target",
    "storage_table",
    "storage_value_column",
    "storage_key_column",
    "storage_time_column",
    "form_field_key",
    "query_alias",
    "startup_snapshot_enable",
  ]) {
    delete next[key];
  }
  return next;
}
