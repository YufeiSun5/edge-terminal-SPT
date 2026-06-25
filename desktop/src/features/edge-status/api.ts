import { apiClient, deleteJson, getJson, patchJson, postJson, putJson, toApiError } from "@/shared/api/http";
import type {
  ActiveDetectionRun,
  AuditLogListParams,
  AuditLogListResponse,
  BulkRemapKioProjectsPayload,
  BulkRemapKioProjectsResult,
  ChannelStats,
  CommandAcceptedResponse,
  DetectionRun,
  DetectionRunEventsResponse,
  DetectionPlan,
  DetectionPlanListParams,
  DetectionPlanListResponse,
  DetectionPlanStartPayload,
  DetectionPlanStartResponse,
  DetectionPlanUpdatePayload,
  DetectionRunListParams,
  DetectionRunListResponse,
  DetectionRunNotePayload,
  DetectionRunNotesResponse,
  DetectionRunReportRequestsResponse,
  DetectionRunStartPayload,
  DetectionRunStopPayload,
  DetectionRunStorageRoutesResponse,
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
  LimitAlarmListParams,
  LimitAlarmListResponse,
  MainReportArtifact,
  MainReportEnqueuePayload,
  MainReportEnqueueResponse,
  MainReportJob,
  MainReportJobEventsResponse,
  MainReportJobListParams,
  MainReportJobListResponse,
  MainReportRegeneratePayload,
  MainReportReadinessResponse,
  NotificationListParams,
  NotificationListResponse,
  NotificationUnreadCount,
  MainReportNotificationListResponse,
  MainReportNotificationUnreadCount,
  Project,
  ProjectPatchPayload,
  ProjectPayload,
  ProjectMemberUpdate,
  ProjectMembersResponse,
  PlanImportCellMapping,
  PlanImportConfirmPayload,
  PlanImportConfirmResult,
  PlanImportDraft,
  RealtimeVariableListParams,
  StationViewEffectiveResponse,
  StationViewItemsReplacePayload,
  StationViewItemsResponse,
  StationViewReloadResponse,
  StationViewTemplatesResponse,
  TagSnapshot,
  ReportTemplate,
  ReportTemplateListParams,
  ReportTemplateListResponse,
  ReportTemplateMappingPayload,
  ReportTemplatePayload,
  ReportTemplateUploadPayload,
  ReportTemplateUploadResponse,
  RuntimeChannelDetailsResponse,
  RuntimeDraft,
  RuntimeDraftPutPayload,
  RuntimeNotificationStats,
  RuntimeWorkersResponse,
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
  TaskFlowRuntimeStats,
  TaskFlowSqlLog,
  TaskFlowTemplate,
  VariableAssignmentPayload,
  VariableConfig,
  VariableCreatePayload,
  VariableListResponse,
  VariableListParams,
  VariablePatchPayload,
  VarIdentifier,
} from "@/shared/api/types";

export function getHealth() {
  return getJson<HealthResponse>("/health");
}

export type MainServerStatus = {
  role: "main_server";
  query_source?: string;
  edge_control_target?: string;
  query_proxy_enabled?: boolean;
  report_service?: string;
}

export function getMainServerStatus() {
  return getJson<MainServerStatus>("/api/v1/main-server/status");
}

export function getChannels() {
  return getJson<ChannelStats>("/api/v1/runtime/channels");
}

export function getRuntimeWorkers() {
  return getJson<RuntimeWorkersResponse>("/api/v1/runtime/workers");
}

export function getRuntimeChannelDetails() {
  return getJson<RuntimeChannelDetailsResponse>("/api/v1/runtime/channels/detail");
}

export function getRuntimeNotifications() {
  return getJson<RuntimeNotificationStats>("/api/v1/runtime/notifications");
}

export function getTaskFlowRuntime() {
  return getJson<TaskFlowRuntimeStats>("/api/v1/task-flows/runtime");
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
	if (params.from) query.set("from", params.from);
	if (params.to) query.set("to", params.to);
	if (params.keyword) query.set("keyword", params.keyword);
	if (params.limit !== undefined) query.set("limit", String(params.limit));
	if (params.offset !== undefined) query.set("offset", String(params.offset));
	const suffix = query.toString();
	return getJson<NotificationListResponse>(`/api/v1/notifications${suffix ? `?${suffix}` : ""}`);
}

export function getNotificationUnreadCount(params: NotificationListParams = {}) {
	const query = new URLSearchParams();
	if (params.type) query.set("type", params.type);
	if (params.level) query.set("level", params.level);
	if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
	if (params.from) query.set("from", params.from);
	if (params.to) query.set("to", params.to);
	if (params.keyword) query.set("keyword", params.keyword);
	const suffix = query.toString();
	return getJson<NotificationUnreadCount>(`/api/v1/notifications/unread-count${suffix ? `?${suffix}` : ""}`);
}

export function markNotificationRead(notificationId: number) {
  return postJson<{ ok: true }, Record<string, never>>(`/api/v1/notifications/${notificationId}/read`, {});
}

export function markAllNotificationsRead(params: NotificationListParams = {}) {
	const query = new URLSearchParams();
	if (params.unread !== undefined) query.set("unread", String(params.unread));
	if (params.type) query.set("type", params.type);
	if (params.level) query.set("level", params.level);
	if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
	if (params.from) query.set("from", params.from);
	if (params.to) query.set("to", params.to);
	if (params.keyword) query.set("keyword", params.keyword);
	const suffix = query.toString();
	return postJson<{ updated: number }, Record<string, never>>(`/api/v1/notifications/read-all${suffix ? `?${suffix}` : ""}`, {});
}

type MainReportNotificationParams = NotificationListParams & {
	job_id?: number;
	event_type?: string | string[];
	dedupe?: 'job_event';
};

function appendReportNotificationQuery(query: URLSearchParams, params: MainReportNotificationParams) {
	if (params.unread !== undefined) query.set("unread", String(params.unread));
	if (params.level) query.set("level", params.level);
	if (params.limit !== undefined) query.set("limit", String(params.limit));
	if (params.offset !== undefined) query.set("offset", String(params.offset));
	if (params.job_id !== undefined) query.set("job_id", String(params.job_id));
	if (params.dedupe) query.set("dedupe", params.dedupe);
	const eventTypes = Array.isArray(params.event_type) ? params.event_type : params.event_type ? [params.event_type] : [];
	eventTypes.forEach((eventType) => query.append("event_type", eventType));
}

export function getMainReportNotifications(params: MainReportNotificationParams = {}) {
	const query = new URLSearchParams();
	appendReportNotificationQuery(query, params);
	const suffix = query.toString();
	return getJson<MainReportNotificationListResponse>(`/api/v1/main-server/report-notifications${suffix ? `?${suffix}` : ""}`);
}

export function getMainReportNotificationUnreadCount(params: MainReportNotificationParams = {}) {
	const query = new URLSearchParams();
	appendReportNotificationQuery(query, params);
	const suffix = query.toString();
	return getJson<MainReportNotificationUnreadCount>(`/api/v1/main-server/report-notifications/unread-count${suffix ? `?${suffix}` : ""}`);
}

export function markMainReportNotificationRead(notificationId: number) {
  return postJson<{ ok: true }, Record<string, never>>(`/api/v1/main-server/report-notifications/${notificationId}/read`, {});
}

export function markAllMainReportNotificationsRead(params: MainReportNotificationParams = {}) {
	const query = new URLSearchParams();
	appendReportNotificationQuery(query, params);
	const suffix = query.toString();
	return postJson<{ updated: number }, Record<string, never>>(`/api/v1/main-server/report-notifications/read-all${suffix ? `?${suffix}` : ""}`, {});
}

export function getLimitAlarms(params: LimitAlarmListParams = {}) {
  const query = new URLSearchParams();
  if (params.scope) query.set("scope", params.scope);
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.task_id !== undefined) query.set("task_id", String(params.task_id));
  if (params.test_no) query.set("test_no", params.test_no);
  if (params.var_id !== undefined) query.set("var_id", String(params.var_id));
  if (params.status) query.set("status", params.status);
  if (params.alarm_type) query.set("alarm_type", params.alarm_type);
  if (params.level) query.set("level", params.level);
  if (params.alarm_level) query.set("alarm_level", params.alarm_level);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<LimitAlarmListResponse>(`/api/v1/limit-alarms${suffix ? `?${suffix}` : ""}`);
}

export function getGateways() {
  return getJson<GatewayStatusMap>("/api/v1/gateways");
}

export function getGatewayConfigs() {
  return getJson<GatewayConfig[]>("/api/v1/gateway-configs");
}

export function getProjects() {
  return getJson<Project[]>("/api/v1/projects").then((items) => items.map(withProjectAliases));
}

export function getProjectMembers(projectId: number) {
  return getJson<ProjectMembersResponse>(`/api/v1/projects/${projectId}/members`);
}

export function replaceProjectMembers(projectId: number, members: ProjectMemberUpdate[]) {
  return putJson<ProjectMembersResponse, { members: ProjectMemberUpdate[] }>(
    `/api/v1/projects/${projectId}/members`,
    { members },
  );
}

export function createProject(payload: ProjectPayload) {
  return postJson<Project, Record<string, unknown>>("/api/v1/projects", withProjectPayload(payload)).then(withProjectAliases);
}

export function updateProject(projectId: number, payload: ProjectPatchPayload) {
  return patchJson<Project, ProjectPatchPayload>(`/api/v1/projects/${projectId}`, payload).then(withProjectAliases);
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
  if (params.edge_instance_id) query.set("edge_instance_id", params.edge_instance_id);
  if (params.gateway_id !== undefined)
    query.set("gateway_id", String(params.gateway_id));
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.assigned !== undefined) query.set("assigned", String(params.assigned));
  if (params.project_code) query.set("project_code", params.project_code);
  if (params.var_group) query.set("var_group", params.var_group);
  if (params.writable !== undefined) query.set("writable", String(params.writable));
  if (params.enabled !== undefined)
    query.set("enabled", String(params.enabled));
  if (params.discovered !== undefined)
    query.set("discovered", String(params.discovered));
  if (params.source_type) query.set("source_type", params.source_type);
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<VariableConfig[]>(
    `/api/v1/variables${suffix ? `?${suffix}` : ""}`,
  ).then((items) => items.map(withVariableAliases));
}

export function getVariablesPage(params: VariableListParams = {}) {
  const query = new URLSearchParams();
  if (params.edge_instance_id) query.set("edge_instance_id", params.edge_instance_id);
  if (params.gateway_id !== undefined) query.set("gateway_id", String(params.gateway_id));
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.assigned !== undefined) query.set("assigned", String(params.assigned));
  if (params.project_code) query.set("project_code", params.project_code);
  if (params.var_group) query.set("var_group", params.var_group);
  if (params.writable !== undefined) query.set("writable", String(params.writable));
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  if (params.discovered !== undefined) query.set("discovered", String(params.discovered));
  if (params.source_type) query.set("source_type", params.source_type);
  if (params.keyword) query.set("keyword", params.keyword);
  query.set("limit", String(params.limit ?? 100));
  query.set("offset", String(params.offset ?? 0));
  return getJson<VariableListResponse>(`/api/v1/variables?${query.toString()}`)
    .then((response) => ({ ...response, items: response.items.map(withVariableAliases) }));
}

export function createVariable(payload: VariableCreatePayload) {
  return postJson<VariableConfig, Record<string, unknown>>(
    "/api/v1/variables",
    withProjectPayload(withoutVariableStoragePayload(payload)),
  ).then(withVariableAliases);
}

export function updateVariable(
  variableId: VarIdentifier,
  payload: VariablePatchPayload,
) {
  return patchJson<VariableConfig, Record<string, unknown>>(
    `/api/v1/variables/${variableId}`,
    withoutVariableStoragePayload(payload),
  ).then(withVariableAliases);
}

export function assignVariable(
  variableId: VarIdentifier,
  payload: VariableAssignmentPayload,
) {
  return patchJson<{ status: string }, Record<string, unknown>>(
    `/api/v1/variables/${variableId}/assignment`,
    withProjectPayload(payload),
  );
}

export function deleteVariable(variableId: VarIdentifier) {
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
  if (params.edge_instance_id) query.set("edge_instance_id", params.edge_instance_id);
  if (params.source_type) query.set("source_type", params.source_type);
  if (params.gateway_id !== undefined) query.set("gateway_id", String(params.gateway_id));
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.var_id !== undefined) {
    const varIds = Array.isArray(params.var_id) ? params.var_id : [params.var_id];
    for (const varId of varIds) query.append("var_id", String(varId));
  }
  const suffix = query.toString();
  return getJson<TagSnapshot[]>(`/api/v1/realtime/variables${suffix ? `?${suffix}` : ""}`).then((items) => items.map(withTagSnapshotAliases));
}

export function getStationViewEffective(projectId: number, edgeInstanceId?: string) {
  const query = new URLSearchParams({ project_id: String(projectId) });
  if (edgeInstanceId) query.set("edge_instance_id", edgeInstanceId);
  return getJson<StationViewEffectiveResponse>(`/api/v1/station-view/effective?${query.toString()}`);
}

export function getStationViewTemplates(params: { status?: string; owner_scope?: string; keyword?: string } = {}) {
  const query = new URLSearchParams();
  if (params.status) query.set("status", params.status);
  if (params.owner_scope) query.set("owner_scope", params.owner_scope);
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<StationViewTemplatesResponse>(`/api/v1/station-view/templates${suffix ? `?${suffix}` : ""}`);
}

export function getStationViewItems(params: { template_uid?: string; project_id?: number }) {
  const query = new URLSearchParams();
  if (params.template_uid) query.set("template_uid", params.template_uid);
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  const suffix = query.toString();
  return getJson<StationViewItemsResponse>(`/api/v1/station-view/items${suffix ? `?${suffix}` : ""}`);
}

export function replaceStationViewItems(payload: StationViewItemsReplacePayload) {
  return putJson<StationViewItemsResponse, StationViewItemsReplacePayload>("/api/v1/station-view/items", payload);
}

export function reloadStationView(projectId?: number) {
  return postJson<StationViewReloadResponse, { project_id?: number }>(
    "/api/v1/station-view/reload",
    projectId ? { project_id: projectId } : {},
  );
}

export function getActiveDetectionRuns() {
  return getJson<ActiveDetectionRun[]>("/api/v1/detection-runs/active").then((items) => items.map(withRunAliases));
}

export function getDetectionRuns(params: DetectionRunListParams = {}) {
  const query = new URLSearchParams();
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.project_code) query.set("project_code", params.project_code);
  if (params.status) query.set("status", params.status);
  if (params.test_no) query.set("test_no", params.test_no);
  if (params.factory_no) query.set("factory_no", params.factory_no);
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

export function getDetectionPlans(params: DetectionPlanListParams = {}) {
  const query = new URLSearchParams();
  if (params.status) query.set("status", params.status);
  if (params.factory_no) query.set("factory_no", params.factory_no);
  if (params.keyword) query.set("keyword", params.keyword);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<DetectionPlanListResponse>(`/api/v1/detection-plans${suffix ? `?${suffix}` : ""}`);
}

export function updateDetectionPlan(planId: number, payload: DetectionPlanUpdatePayload) {
  return patchJson<DetectionPlan, DetectionPlanUpdatePayload>(`/api/v1/detection-plans/${planId}`, payload);
}

export function startDetectionPlan(planId: number, payload: DetectionPlanStartPayload) {
  return postJson<DetectionPlanStartResponse, DetectionPlanStartPayload>(`/api/v1/detection-plans/${planId}/start`, payload)
    .then((response) => ({ ...response, task: response.task ? withRunAliases(response.task) : undefined }));
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

export function getDetectionRunReportRequests(runId: number) {
  return getJson<DetectionRunReportRequestsResponse>(`/api/v1/detection-runs/${runId}/report-requests`);
}

export function getDetectionRunStorageRoutes(runId: number) {
  return getJson<DetectionRunStorageRoutesResponse>(`/api/v1/detection-runs/${runId}/storage-routes`)
    .then((response) => ({ ...response, items: response.items.map(withStorageRouteAliases) }));
}

export function getRuntimeDraft<TData = Record<string, unknown>>(
  namespace: string,
  params: { scope_type?: string; scope_id?: string; project_id?: number; edge_instance_id?: string } = {},
) {
  const query = new URLSearchParams();
  if (params.scope_type) query.set("scope_type", params.scope_type);
  if (params.scope_id) query.set("scope_id", params.scope_id);
  if (params.project_id !== undefined) query.set("project_id", String(params.project_id));
  if (params.edge_instance_id) query.set("edge_instance_id", params.edge_instance_id);
  const suffix = query.toString();
  return getJson<RuntimeDraft<TData>>(`/api/v1/runtime-drafts/${encodeURIComponent(namespace)}${suffix ? `?${suffix}` : ""}`);
}

export function putRuntimeDraft<TData = Record<string, unknown>>(namespace: string, payload: RuntimeDraftPutPayload<TData>) {
  return putJson<RuntimeDraft<TData>, RuntimeDraftPutPayload<TData>>(`/api/v1/runtime-drafts/${encodeURIComponent(namespace)}`, payload);
}

export function deleteRuntimeDraft(namespace: string, params: { scope_type: string; scope_id: string; expected_revision?: number }) {
  const query = new URLSearchParams({ scope_type: params.scope_type, scope_id: params.scope_id });
  if (params.expected_revision !== undefined) query.set("expected_revision", String(params.expected_revision));
  return deleteJson<{ status: string }>(`/api/v1/runtime-drafts/${encodeURIComponent(namespace)}?${query.toString()}`);
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
  if (params.project_code) query.set("project_code", params.project_code);
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

export function getMainReportTemplates(params: ReportTemplateListParams = {}) {
  const query = new URLSearchParams();
  if (params.enabled !== undefined) query.set("enabled", String(params.enabled));
  if (params.keyword) query.set("keyword", params.keyword);
  const suffix = query.toString();
  return getJson<ReportTemplateListResponse>(`/api/v1/main-server/report-templates${suffix ? `?${suffix}` : ""}`);
}

export async function uploadMainReportTemplate(payload: ReportTemplateUploadPayload) {
  try {
    const body = new FormData();
    body.append("file", payload.file);
    body.append("template_code", payload.template_code);
    if (payload.name) body.append("name", payload.name);
    if (payload.display_name) body.append("display_name", payload.display_name);
    if (payload.version !== undefined) body.append("version", String(payload.version));
    if (payload.params_schema_json) body.append("params_schema_json", payload.params_schema_json);
    if (payload.remark) body.append("remark", payload.remark);
    if (payload.enabled !== undefined) body.append("enabled", String(payload.enabled));
    const response = await apiClient.post<ReportTemplateUploadResponse>("/api/v1/main-server/report-templates/upload", body);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export function updateMainReportTemplateMapping(templateId: number, payload: ReportTemplateMappingPayload) {
  return patchJson<ReportTemplate, ReportTemplateMappingPayload>(`/api/v1/main-server/report-templates/${templateId}/mapping`, payload);
}

export async function downloadMainReportTemplateArtifact(templateId: number): Promise<MainReportArtifact> {
  try {
    const response = await apiClient.get<Blob>(`/api/v1/main-server/report-templates/${templateId}/artifact`, {
      responseType: "blob",
    });
    return {
      blob: response.data,
      filename: filenameFromContentDisposition(headerString(response.headers["content-disposition"]), `report-template-${templateId}.xlsx`),
      contentType: headerString(response.headers["content-type"]) ?? "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    };
  } catch (error) {
    throw toApiError(error);
  }
}

export async function parseMainReportPlanImport(file: File, edgeInstanceId?: string, mapping?: PlanImportCellMapping) {
  try {
    const body = new FormData();
    body.append("file", file);
    if (edgeInstanceId) body.append("edge_instance_id", edgeInstanceId);
    if (mapping && mapping.rows?.length) body.append("mapping_json", JSON.stringify(mapping));
    const response = await apiClient.post<PlanImportDraft>("/api/v1/main-server/report-plan-imports/parse", body);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export function confirmMainReportPlanImport(payload: PlanImportConfirmPayload) {
  return postJson<PlanImportConfirmResult, PlanImportConfirmPayload>("/api/v1/main-server/report-plan-imports/confirm", payload);
}

export function getMainReportReadiness(taskId: number, edgeInstanceId?: string) {
  const query = new URLSearchParams({ task_id: String(taskId) });
  if (edgeInstanceId) query.set("edge_instance_id", edgeInstanceId);
  return getJson<MainReportReadinessResponse>(`/api/v1/main-server/report-readiness?${query.toString()}`);
}

export function enqueueMainReportJob(payload: MainReportEnqueuePayload) {
  return postJson<MainReportEnqueueResponse, MainReportEnqueuePayload>("/api/v1/main-server/report-jobs/enqueue", payload);
}

export function getMainReportJobs(params: MainReportJobListParams = {}) {
  const query = new URLSearchParams();
  if (params.status) query.set("status", params.status);
  if (params.task_id !== undefined) query.set("task_id", String(params.task_id));
  if (params.edge_instance_id) query.set("edge_instance_id", params.edge_instance_id);
  if (params.limit !== undefined) query.set("limit", String(params.limit));
  if (params.offset !== undefined) query.set("offset", String(params.offset));
  const suffix = query.toString();
  return getJson<MainReportJobListResponse>(`/api/v1/main-server/report-jobs${suffix ? `?${suffix}` : ""}`);
}

export function getMainReportJob(jobId: number) {
  return getJson<MainReportJob>(`/api/v1/main-server/report-jobs/${jobId}`);
}

export function getMainReportJobEvents(jobId: number, limit = 100) {
  return getJson<MainReportJobEventsResponse>(`/api/v1/main-server/report-jobs/${jobId}/events?limit=${limit}`);
}

export function retryMainReportJob(jobId: number) {
  return postJson<MainReportJob, Record<string, never>>(`/api/v1/main-server/report-jobs/${jobId}/retry`, {});
}

export function regenerateMainReportJob(jobId: number, payload: MainReportRegeneratePayload) {
  return postJson<MainReportJob, MainReportRegeneratePayload>(`/api/v1/main-server/report-jobs/${jobId}/regenerate`, payload);
}

function filenameFromContentDisposition(value: string | undefined, fallback: string) {
  if (!value) return fallback;
  const utf8Match = value.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) return decodeURIComponent(utf8Match[1]);
  const plainMatch = value.match(/filename="?([^"]+)"?/i);
  return plainMatch?.[1] ? plainMatch[1] : fallback;
}

function headerString(value: unknown) {
  return typeof value === "string" ? value : undefined;
}

export async function downloadMainReportArtifact(jobId: number): Promise<MainReportArtifact> {
  try {
    const response = await apiClient.get<Blob>(`/api/v1/main-server/report-jobs/${jobId}/artifact`, {
      responseType: "blob",
    });
    return {
      blob: response.data,
      filename: filenameFromContentDisposition(headerString(response.headers["content-disposition"]), `report-job-${jobId}.xlsx`),
      contentType: headerString(response.headers["content-type"]) ?? "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    };
  } catch (error) {
    throw toApiError(error);
  }
}

export async function downloadMainServerPackage(payload: { task_id: number; keys: string[]; edge_instance_id?: string }): Promise<MainReportArtifact> {
  try {
    const response = await apiClient.post<Blob>("/api/v1/main-server/download-packages", payload, {
      responseType: "blob",
    });
    return {
      blob: response.data,
      filename: filenameFromContentDisposition(headerString(response.headers["content-disposition"]), `task-${payload.task_id}-download-package.zip`),
      contentType: headerString(response.headers["content-type"]) ?? "application/zip",
    };
  } catch (error) {
    throw toApiError(error);
  }
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

function withProjectAliases<T extends { project_code?: string }>(item: T): T {
  return {
    ...item,
    project_code: item.project_code || "",
  };
}

function withVariableAliases<T extends VariableConfig>(item: T): T {
  return item;
}

function withTagSnapshotAliases<T extends TagSnapshot>(item: T): T {
  return item;
}

function withStorageRouteAliases<T>(item: T): T {
  return item;
}

function withRunAliases<T>(item: T): T {
  const run = item as T & { storage_routes?: unknown[] };
  if (!Array.isArray(run.storage_routes)) return item;
  return {
    ...item,
    storage_routes: run.storage_routes.map(withStorageRouteAliases),
  };
}

function withStandardAliases<T extends DetectionStandard>(item: T): T {
  return item;
}

function withProjectPayload<T extends Record<string, unknown>>(payload: T): Record<string, unknown> {
  return { ...payload };
}

function withoutProjectAliases<T extends Record<string, unknown>>(payload: T): Record<string, unknown> {
  return { ...payload };
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
