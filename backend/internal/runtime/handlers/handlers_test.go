package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/mqttx"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestVariablesHandlerCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	handler := NewVariablesHandler(services.NewVariablesService(repo, tags))
	Project := createHandlerProject(t, repo)

	resp := callHandler(t, http.MethodPost, "/variables", map[string]any{
		"source_type":            "virtual",
		"project_id":             Project.ID,
		"project_code":           Project.ProjectCode,
		"var_name":               "virtual_bool",
		"data_type":              "BOOL",
		"default_alarm_enabled":  true,
		"default_limit_deadband": 0.2,
	}, handler.create)
	if resp.Code != http.StatusOK || tags.Count() != 1 {
		t.Fatalf("create status=%d body=%s tags=%d", resp.Code, resp.Body.String(), tags.Count())
	}
	resp = callHandler(t, http.MethodPost, "/variables", map[string]any{"source_type": "manual", "var_name": "bad", "data_type": "FLOAT"}, handler.create)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", resp.Code)
	}

	listResp := callHandler(t, http.MethodGet, "/variables?source_type=virtual", nil, handler.list)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list variables status=%d", listResp.Code)
	}
	realtimeResp := callHandler(t, http.MethodGet, "/realtime/variables", nil, handler.realtime)
	if realtimeResp.Code != http.StatusOK {
		t.Fatalf("realtime status=%d", realtimeResp.Code)
	}
	assignResp := callHandlerWithParams(t, http.MethodPatch, "/variables/1/assignment", map[string]any{"project_id": Project.ID, "enabled": true}, gin.Params{{Key: "variable_id", Value: "1"}}, handler.assign)
	if assignResp.Code != http.StatusOK {
		t.Fatalf("assign status=%d body=%s", assignResp.Code, assignResp.Body.String())
	}
	patchResp := callHandlerWithParams(t, http.MethodPatch, "/variables/1", map[string]any{"display_name": "Updated", "decimal_places": 1, "default_limit_h": 10, "apply_to_running": true}, gin.Params{{Key: "variable_id", Value: "1"}}, handler.patch)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchResp.Code, patchResp.Body.String())
	}
	deleteResp := callHandlerWithParams(t, http.MethodDelete, "/variables/1", nil, gin.Params{{Key: "variable_id", Value: "1"}}, handler.delete)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status=%d", deleteResp.Code)
	}
	missingPatchResp := callHandlerWithParams(t, http.MethodPatch, "/variables/404", map[string]any{"display_name": "missing"}, gin.Params{{Key: "variable_id", Value: "404"}}, handler.patch)
	if missingPatchResp.Code != http.StatusNotFound {
		t.Fatalf("missing variable patch should be 404, got %d body=%s", missingPatchResp.Code, missingPatchResp.Body.String())
	}
	missingAssignResp := callHandlerWithParams(t, http.MethodPatch, "/variables/404/assignment", map[string]any{"project_id": Project.ID, "enabled": true}, gin.Params{{Key: "variable_id", Value: "404"}}, handler.assign)
	if missingAssignResp.Code != http.StatusNotFound {
		t.Fatalf("missing variable assignment should be 404, got %d body=%s", missingAssignResp.Code, missingAssignResp.Body.String())
	}
	missingDeleteResp := callHandlerWithParams(t, http.MethodDelete, "/variables/404", nil, gin.Params{{Key: "variable_id", Value: "404"}}, handler.delete)
	if missingDeleteResp.Code != http.StatusNotFound {
		t.Fatalf("missing variable delete should be 404, got %d body=%s", missingDeleteResp.Code, missingDeleteResp.Body.String())
	}
}

func TestVariablesHandlerRealtimeFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	projectOne := uint(1)
	projectTwo := uint(2)
	tags := pipeline.NewTagManager()
	tags.Load([]models.TagConfig{
		{VarID: 10, GatewayID: 1, SourceType: models.TagSourceMQTT, SourceTopic: "topic-a", SourcePath: "temp", RawName: "temp", ProjectID: &projectOne, ProjectCode: "AC-01", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ScaleFactor: 1, Enabled: true},
		{VarID: 11, GatewayID: 1, SourceType: models.TagSourceMQTT, SourceTopic: "topic-a", SourcePath: "humidity", RawName: "humidity", ProjectID: &projectTwo, ProjectCode: "AC-02", VarName: "humidity", JSONPath: "humidity", DataType: "FLOAT", ScaleFactor: 1, Enabled: true},
		{VarID: 12, GatewayID: 2, SourceType: models.TagSourceVirtual, SourceTopic: "virtual", SourcePath: "flag", RawName: "flag", ProjectID: &projectOne, ProjectCode: "AC-01", VarName: "flag", JSONPath: "flag", DataType: "BOOL", ScaleFactor: 1, Enabled: true},
	})
	handler := NewVariablesHandler(services.NewVariablesService(nil, tags))

	resp := callHandler(t, http.MethodGet, "/realtime/variables?project_id=1", nil, handler.realtime)
	if resp.Code != http.StatusOK {
		t.Fatalf("project realtime status=%d body=%s", resp.Code, resp.Body.String())
	}
	var projectItems []models.TagSnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &projectItems); err != nil {
		t.Fatal(err)
	}
	if len(projectItems) != 2 {
		t.Fatalf("expected two project snapshots, got %+v", projectItems)
	}

	resp = callHandler(t, http.MethodGet, "/realtime/variables?var_id=11", nil, handler.realtime)
	var singleItems []models.TagSnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &singleItems); err != nil {
		t.Fatal(err)
	}
	if len(singleItems) != 1 || singleItems[0].VarID != 11 {
		t.Fatalf("expected single var snapshot, got %+v", singleItems)
	}

	resp = callHandler(t, http.MethodGet, "/realtime/variables?var_id=10,11&source_type=mqtt", nil, handler.realtime)
	var multiItems []models.TagSnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &multiItems); err != nil {
		t.Fatal(err)
	}
	if len(multiItems) != 2 || multiItems[0].VarID != 10 || multiItems[1].VarID != 11 {
		t.Fatalf("expected ordered multi var snapshots, got %+v", multiItems)
	}

	resp = callHandler(t, http.MethodGet, "/realtime/variables?device_id=2", nil, handler.realtime)
	var aliasItems []models.TagSnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &aliasItems); err != nil {
		t.Fatal(err)
	}
	if len(aliasItems) != 1 || aliasItems[0].ProjectID == nil || *aliasItems[0].ProjectID != 2 {
		t.Fatalf("expected device_id alias to filter project, got %+v", aliasItems)
	}

	resp = callHandler(t, http.MethodGet, "/realtime/variables?var_id=bad", nil, handler.realtime)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid var_id 400, got %d", resp.Code)
	}
}

func TestVariablesHandlerBulkRemapKIOProjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	handler := NewVariablesHandler(services.NewVariablesService(repo, tags))
	fixtures := []models.TagConfig{
		{VarID: 1001, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台1_39").1`, SourceType: models.TagSourceMQTT, RawName: "台1_39", VarName: "台1_39", JSONPath: `Objs.#(N=="台1_39").1`, DataType: "FLOAT", ScaleFactor: 1, Discovered: true, Enabled: false},
		{VarID: 12042, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台12_42").1`, SourceType: models.TagSourceMQTT, RawName: "台12_42", VarName: "台12_42", JSONPath: `Objs.#(N=="台12_42").1`, DataType: "STRING", ScaleFactor: 1, Discovered: true, Enabled: false},
		{VarID: 13001, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台13_1").1`, SourceType: models.TagSourceMQTT, RawName: "台13_1", VarName: "台13_1", JSONPath: `Objs.#(N=="台13_1").1`, DataType: "INT", ScaleFactor: 1, Discovered: true, Enabled: false},
	}
	for i := range fixtures {
		if err := repo.CreateTag(&fixtures[i]); err != nil {
			t.Fatal(err)
		}
	}

	resp := callHandler(t, http.MethodPost, "/variables/bulk-remap/kio-projects", map[string]any{}, handler.bulkRemapKIOProjects)
	if resp.Code != http.StatusOK {
		t.Fatalf("bulk remap status=%d body=%s", resp.Code, resp.Body.String())
	}
	var result services.BulkRemapKIOProjectsResult
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.CreatedProjects != 12 || result.Matched != 2 || result.Updated != 2 || result.Skipped != 1 {
		t.Fatalf("unexpected bulk result: %+v", result)
	}
	tag, err := repo.GetTag(1001)
	if err != nil {
		t.Fatal(err)
	}
	if tag.ProjectID == nil || *tag.ProjectID != 1 || tag.ProjectCode != "AC-01" || tag.VarName != "kio_01_39" || tag.DisplayName != "台1_39" || !tag.Enabled {
		t.Fatalf("unexpected remapped tag: %+v", tag)
	}
	routes, err := repo.ListStorageRoutesByProject(*tag.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].VarID != tag.VarID || routes[0].ColumnName != "kio_01_39" || routes[0].Enabled {
		t.Fatalf("unexpected default storage route: %+v", routes)
	}
	if tags.Count() != 2 {
		t.Fatalf("expected runtime tag manager to reload assigned tags, got %d", tags.Count())
	}
}

func TestStorageRoutesHandlerCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	Project := createHandlerProject(t, repo)
	tag := models.TagConfig{
		VarID:       9212397624135540848,
		GatewayID:   1,
		SourcePath:  "temp",
		RawName:     "temp",
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	handler := NewStorageRoutesHandler(repo)
	storeOnStart := true
	enabled := true
	createResp := callHandler(t, http.MethodPost, "/storage-routes", map[string]any{
		"project_id":     Project.ID,
		"var_id":         strconv.FormatInt(tag.VarID, 10),
		"table_name":     "custom_temp_data",
		"column_name":    "temp_value",
		"trigger_mode":   models.StoreTriggerOnCycle,
		"cycle_ms":       3000,
		"store_on_start": storeOnStart,
		"enabled":        enabled,
	}, handler.create)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create storage route status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var route models.StorageRoute
	if err := json.Unmarshal(createResp.Body.Bytes(), &route); err != nil {
		t.Fatal(err)
	}
	if route.StorageTable != "custom_temp_data" || route.CycleMS != 3000 || !route.StoreOnStart || !route.Enabled {
		t.Fatalf("unexpected created route: %+v", route)
	}
	if !strings.Contains(createResp.Body.String(), `"var_id_text":"9212397624135540848"`) {
		t.Fatalf("expected storage route response to include exact var_id_text, body=%s", createResp.Body.String())
	}
	listResp := callHandler(t, http.MethodGet, "/storage-routes?project_id=1&enabled=true", nil, handler.list)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list storage routes status=%d", listResp.Code)
	}
	patchResp := callHandlerWithParams(t, http.MethodPatch, "/storage-routes/1", map[string]any{"cycle_ms": 5000, "deadband": 0.2}, gin.Params{{Key: "id", Value: strconv.FormatUint(route.ID, 10)}}, handler.patch)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch storage route status=%d body=%s", patchResp.Code, patchResp.Body.String())
	}
	missingPatchResp := callHandlerWithParams(t, http.MethodPatch, "/storage-routes/404", map[string]any{"enabled": true}, gin.Params{{Key: "id", Value: "404"}}, handler.patch)
	if missingPatchResp.Code != http.StatusNotFound {
		t.Fatalf("missing storage route patch should be 404, got %d body=%s", missingPatchResp.Code, missingPatchResp.Body.String())
	}
	deleteResp := callHandlerWithParams(t, http.MethodDelete, "/storage-routes/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(route.ID, 10)}}, handler.delete)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete storage route status=%d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	missingDeleteResp := callHandlerWithParams(t, http.MethodDelete, "/storage-routes/404", nil, gin.Params{{Key: "id", Value: "404"}}, handler.delete)
	if missingDeleteResp.Code != http.StatusNotFound {
		t.Fatalf("missing storage route delete should be 404, got %d body=%s", missingDeleteResp.Code, missingDeleteResp.Body.String())
	}
}

func TestAuditLogsHandlerListAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	if err := repo.CreateAuditLog(&models.SysAuditLog{ActorType: "user", ActorID: "1", Action: "storage_route.update", TargetType: "storage_route", TargetID: "1", Result: "success"}); err != nil {
		t.Fatal(err)
	}
	handler := NewAuditLogsHandler(repo)
	resp := callHandler(t, http.MethodGet, "/audit-logs?actor_type=user&action=storage_route.update&limit=500&offset=0&from=2026-01-01", nil, handler.list)
	if resp.Code != http.StatusOK {
		t.Fatalf("audit list status=%d body=%s", resp.Code, resp.Body.String())
	}
	badResp := callHandler(t, http.MethodGet, "/audit-logs?limit=bad", nil, handler.list)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad audit query, got %d", badResp.Code)
	}
	if normalizedAuditLimit(500) != 200 || normalizedAuditLimit(0) != 50 || normalizedAuditOffset(-1) != 0 {
		t.Fatal("audit normalization failed")
	}
}

func TestTaskFlowsHandlerCRUDAndManualRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	tags := pipeline.NewTagManager()
	tasks := pipeline.NewTaskManager()
	flows := pipeline.NewTaskFlowExecutor(repo, tags, tasks, channels)
	flows.Start(1)
	handler := NewTaskFlowsHandler(repo, flows)
	Project := createHandlerProject(t, repo)
	otherProject := models.Project{ProjectCode: "AC-H2", Name: "Project 2", Enabled: true}
	if err := repo.CreateProject(&otherProject); err != nil {
		t.Fatal(err)
	}

	modulesResp := callHandler(t, http.MethodGet, "/task-modules", nil, handler.modules)
	if modulesResp.Code != http.StatusOK {
		t.Fatalf("modules status=%d body=%s", modulesResp.Code, modulesResp.Body.String())
	}
	if body := modulesResp.Body.String(); !strings.Contains(body, "getMany([var_id])") || !strings.Contains(body, "trigger_var_id_text") || !strings.Contains(body, "默认 trigger=false") {
		t.Fatalf("expected JavaScript realtime runtime_api, body=%s", body)
	}
	if body := modulesResp.Body.String(); !strings.Contains(body, "process_params") || !strings.Contains(body, "plc_writes") || !strings.Contains(body, "进风口面积") {
		t.Fatalf("expected task module schema to expose process params and PLC writes, body=%s", body)
	}
	badCreateResp := callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"flow_code":    "missing-project",
		"name":         "Missing Project",
		"trigger_type": models.TaskFlowTriggerManual,
		"action_type":  models.TaskFlowActionJavaScript,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("missing project_id should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    " ",
		"name":         "Missing Flow Code",
		"trigger_type": models.TaskFlowTriggerManual,
		"action_type":  models.TaskFlowActionJavaScript,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("blank flow_code should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "missing-name",
		"name":         " ",
		"trigger_type": models.TaskFlowTriggerManual,
		"action_type":  models.TaskFlowActionJavaScript,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("blank name should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-trigger",
		"name":         "Bad Trigger",
		"trigger_type": "never",
		"action_type":  models.TaskFlowActionJavaScript,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid trigger_type should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-action",
		"name":         "Bad Action",
		"trigger_type": models.TaskFlowTriggerManual,
		"action_type":  "builtin.missing",
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid action_type should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-timing",
		"name":         "Bad Timing",
		"trigger_type": models.TaskFlowTriggerManual,
		"timeout_ms":   -1,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("negative timing should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-step",
		"name":         "Bad Step",
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[{"code":"bad","module":"builtin.missing"}]`,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid steps_json module should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "empty-steps",
		"name":         "Empty Steps",
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[]`,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("empty steps_json should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "duplicate-step-code",
		"name":         "Duplicate Step Code",
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[{"code":"same","module":"builtin.context_set","params":{"a":1}},{"code":"same","module":"builtin.context_set","params":{"b":2}}]`,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("duplicate steps_json code should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-param-source",
		"name":         "Bad Param Source",
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[{"code":"start","module":"builtin.start_detection_run","params":{"project_id":{"source":"trigger","key":"project_id"}}}]`,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid steps_json param source should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-event-key",
		"name":         "Bad Event Key",
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[{"code":"ctx","module":"builtin.context_set","params":{"project":{"source":"event","key":"project"}}}]`,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid steps_json event key should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	disabledEventFlow := false
	eventKeyCreateResp := callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "event-var-id-text",
		"name":         "Event Var ID Text",
		"enabled":      disabledEventFlow,
		"trigger_type": models.TaskFlowTriggerManual,
		"steps_json":   `[{"code":"ctx","module":"builtin.context_set","params":{"trigger_var_id_text":{"source":"event","key":"trigger_var_id_text"}}}]`,
	}, handler.create)
	if eventKeyCreateResp.Code != http.StatusOK {
		t.Fatalf("valid trigger_var_id_text event key should be accepted, got %d body=%s", eventKeyCreateResp.Code, eventKeyCreateResp.Body.String())
	}
	var eventFlow models.TaskFlow
	if err := json.Unmarshal(eventKeyCreateResp.Body.Bytes(), &eventFlow); err != nil {
		t.Fatal(err)
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-role",
		"name":         "Bad Role",
		"trigger_type": models.TaskFlowTriggerManual,
		"vars":         []map[string]any{{"var_id": 100, "role": "execute"}},
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid var role should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "bad-var-id",
		"name":         "Bad Var ID",
		"trigger_type": models.TaskFlowTriggerManual,
		"vars":         []map[string]any{{"var_id": 0, "role": models.TaskFlowVarRoleWatch}},
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid var_id should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "data-change-no-watch",
		"name":         "Data Change No Watch",
		"trigger_type": models.TaskFlowTriggerDataChange,
		"vars":         []map[string]any{{"var_id": 100, "role": models.TaskFlowVarRoleRead}},
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("data_change without watch var should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	badCreateResp = callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":   Project.ID,
		"flow_code":    "schedule-no-interval",
		"name":         "Schedule No Interval",
		"trigger_type": models.TaskFlowTriggerSchedule,
	}, handler.create)
	if badCreateResp.Code != http.StatusBadRequest {
		t.Fatalf("schedule without interval should be rejected, got %d body=%s", badCreateResp.Code, badCreateResp.Body.String())
	}
	createResp := callHandler(t, http.MethodPost, "/task-flows", map[string]any{
		"project_id":       Project.ID,
		"flow_code":        "manual-js",
		"name":             "Manual JS",
		"trigger_type":     models.TaskFlowTriggerManual,
		"action_type":      models.TaskFlowActionJavaScript,
		"action_script":    `log.info("manual"); ({ok:true});`,
		"timeout_ms":       3000,
		"priority":         3,
		"condition_script": "true",
		"vars":             []map[string]any{{"var_id": 100, "var_name": "start_flag", "role": models.TaskFlowVarRoleWatch}},
	}, handler.create)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create task flow status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var flow models.TaskFlow
	mustDecodeHandler(t, createResp, &flow)
	getFlowResp := callHandlerWithParams(t, http.MethodGet, "/task-flows/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.get)
	if getFlowResp.Code != http.StatusOK {
		t.Fatalf("get task flow status=%d body=%s", getFlowResp.Code, getFlowResp.Body.String())
	}
	var gotFlow models.TaskFlow
	mustDecodeHandler(t, getFlowResp, &gotFlow)
	if gotFlow.ID != flow.ID || len(gotFlow.Vars) != 1 {
		t.Fatalf("unexpected task flow detail: %+v", gotFlow)
	}
	listResp := callHandler(t, http.MethodGet, "/task-flows?enabled=true", nil, handler.list)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list task flows status=%d", listResp.Code)
	}
	badListResp := callHandler(t, http.MethodGet, "/task-flows?trigger_type=never", nil, handler.list)
	if badListResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid list trigger_type should be rejected, got %d body=%s", badListResp.Code, badListResp.Body.String())
	}
	templatesResp := callHandler(t, http.MethodGet, "/task-flow-templates", nil, handler.templates)
	templatesBody := templatesResp.Body.String()
	if templatesResp.Code != http.StatusOK ||
		!strings.Contains(templatesBody, models.TaskFlowActionBuiltinWriteVariable) ||
		!strings.Contains(templatesBody, "variable-request-start-fixed-duration-detection") ||
		!strings.Contains(templatesBody, "variable-request-start-qualified-hold-detection") ||
		!strings.Contains(templatesBody, models.TaskFlowActionBuiltinUpdateDetectionLimits) ||
		!strings.Contains(templatesBody, models.TaskFlowActionBuiltinRefreshFeatures) ||
		!strings.Contains(templatesBody, models.TaskFlowActionBuiltinRegisterReport) ||
		!strings.Contains(templatesBody, "operator_note") ||
		!strings.Contains(templatesBody, "report_template_id") {
		t.Fatalf("task flow templates status=%d body=%s", templatesResp.Code, templatesResp.Body.String())
	}
	if strings.Count(templatesBody, models.TaskFlowActionBuiltinWriteControlVariables) < 4 {
		t.Fatalf("start detection templates should include PLC write step before detection start, body=%s", templatesBody)
	}
	disabled := false
	patchResp := callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"project_id": otherProject.ID, "enabled": disabled, "priority": 7, "vars": []map[string]any{{"var_id": 101, "role": models.TaskFlowVarRoleRead}}}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch task flow status=%d body=%s", patchResp.Code, patchResp.Body.String())
	}
	mustDecodeHandler(t, patchResp, &flow)
	if flow.ProjectID != otherProject.ID || len(flow.Vars) != 1 || flow.Vars[0].ProjectID != otherProject.ID {
		t.Fatalf("patch should update task flow project and var bindings: %+v", flow)
	}
	badPatchResp := callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"trigger_type": models.TaskFlowTriggerDataChange}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("patch data_change without watch var should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"trigger_type": models.TaskFlowTriggerSchedule}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("patch schedule without interval should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"action_type": "builtin.missing"}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch action_type should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"project_id": 0}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch project_id should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"flow_code": " "}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("blank patch flow_code should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"name": " "}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("blank patch name should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"timeout_ms": 0}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch timeout_ms should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"hold_ms": -1}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("negative patch hold_ms should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{
		"steps_json": `[{"code":"control","module":"builtin.write_control_variables","params":{"items":{"source":"task_param","key":"plc_writes"}}}]`,
	}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch steps_json param source should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"steps_json": `[]`}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("empty patch steps_json should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{
		"steps_json": `[{"code":"same","module":"builtin.context_set","params":{"a":1}},{"code":"same","module":"builtin.context_set","params":{"b":2}}]`,
	}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("duplicate patch steps_json code should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{
		"steps_json": `[{"code":"ctx","module":"builtin.context_set","params":{"project":{"source":"event","key":"project"}}}]`,
	}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch steps_json event key should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	patchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{
		"steps_json": `[{"code":"ctx","module":"builtin.context_set","params":{"trigger_var_id_text":{"source":"event","key":"trigger_var_id_text"}}}]`,
	}, gin.Params{{Key: "id", Value: strconv.FormatUint(eventFlow.ID, 10)}}, handler.patch)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("valid patch trigger_var_id_text event key should be accepted, got %d body=%s", patchResp.Code, patchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"vars": []map[string]any{{"var_id": 101, "role": "execute"}}}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch var role should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	badPatchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"vars": []map[string]any{{"var_id": -1, "role": models.TaskFlowVarRoleWatch}}}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if badPatchResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid patch var_id should be rejected, got %d body=%s", badPatchResp.Code, badPatchResp.Body.String())
	}
	enabled := true
	patchResp = callHandlerWithParams(t, http.MethodPatch, "/task-flows/1", map[string]any{"enabled": enabled}, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.patch)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("enable task flow status=%d body=%s", patchResp.Code, patchResp.Body.String())
	}
	runResp := callHandlerWithParams(t, http.MethodPost, "/task-flows/1/run", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.run)
	if runResp.Code != http.StatusOK {
		t.Fatalf("manual run status=%d body=%s", runResp.Code, runResp.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	var run models.TaskFlowRun
	for time.Now().Before(deadline) {
		if err := db.First(&run, "flow_id = ?", flow.ID).Error; err == nil && run.Status == models.TaskFlowStatusSuccess {
			if !strings.Contains(run.ScriptLogs, "manual") || strings.Contains(run.InputSnapshot, "run_params") {
				t.Fatalf("manual run should not carry HTTP params: %+v", run)
			}
			runsResp := callHandler(t, http.MethodGet, "/task-flow-runs?status=success&limit=10", nil, handler.listRuns)
			if runsResp.Code != http.StatusOK {
				t.Fatalf("list task flow runs status=%d body=%s", runsResp.Code, runsResp.Body.String())
			}
			getRunResp := callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(run.ID, 10)}}, handler.getRun)
			if getRunResp.Code != http.StatusOK {
				t.Fatalf("get task flow run status=%d body=%s", getRunResp.Code, getRunResp.Body.String())
			}
			missingRunID := run.ID + 999
			missingRunResp := callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/999999", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(missingRunID, 10)}}, handler.getRun)
			if missingRunResp.Code != http.StatusNotFound {
				t.Fatalf("missing task flow run should be 404, got %d body=%s", missingRunResp.Code, missingRunResp.Body.String())
			}
			sqlLogsResp := callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/1/sql-logs", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(run.ID, 10)}}, handler.listRunSQLLogs)
			if sqlLogsResp.Code != http.StatusOK {
				t.Fatalf("list task flow sql logs status=%d body=%s", sqlLogsResp.Code, sqlLogsResp.Body.String())
			}
			missingSQLLogsResp := callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/999999/sql-logs", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(missingRunID, 10)}}, handler.listRunSQLLogs)
			if missingSQLLogsResp.Code != http.StatusNotFound {
				t.Fatalf("missing task flow run sql logs should be 404, got %d body=%s", missingSQLLogsResp.Code, missingSQLLogsResp.Body.String())
			}
			badSQLLogsLimitResp := callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/1/sql-logs?limit=0", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(run.ID, 10)}}, handler.listRunSQLLogs)
			if badSQLLogsLimitResp.Code != http.StatusBadRequest {
				t.Fatalf("zero sql logs limit should be rejected, got %d body=%s", badSQLLogsLimitResp.Code, badSQLLogsLimitResp.Body.String())
			}
			deleteResp := callHandlerWithParams(t, http.MethodDelete, "/task-flows/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.delete)
			if deleteResp.Code != http.StatusOK || !strings.Contains(deleteResp.Body.String(), "deleted") {
				t.Fatalf("delete task flow status=%d body=%s", deleteResp.Code, deleteResp.Body.String())
			}
			getDeletedResp := callHandlerWithParams(t, http.MethodGet, "/task-flows/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(flow.ID, 10)}}, handler.get)
			if getDeletedResp.Code == http.StatusOK {
				t.Fatalf("deleted task flow should not be readable: %s", getDeletedResp.Body.String())
			}
			getRunResp = callHandlerWithParams(t, http.MethodGet, "/task-flow-runs/1", nil, gin.Params{{Key: "id", Value: strconv.FormatUint(run.ID, 10)}}, handler.getRun)
			if getRunResp.Code != http.StatusOK {
				t.Fatalf("run history should remain after deleting task flow config status=%d body=%s", getRunResp.Code, getRunResp.Body.String())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected successful run, got %+v", run)
}

func TestTaskFlowsHandlerListRunsFiltersFailedAndTimeoutDistinctly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	handler := NewTaskFlowsHandler(repo, pipeline.NewTaskFlowExecutor(repo, pipeline.NewTagManager(), pipeline.NewTaskManager(), pipeline.NewChannels()))
	project := createHandlerProject(t, repo)
	flow := models.TaskFlow{
		ProjectID:       project.ID,
		FlowCode:        "status-filter",
		Name:            "Status Filter",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerManual,
		ActionType:      models.TaskFlowActionJavaScript,
		ConditionScript: "true",
	}
	if err := repo.CreateTaskFlow(&flow); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 5, 31, 1, 40, 0, 0, time.UTC)
	failedRun := models.TaskFlowRun{
		FlowID:       flow.ID,
		FlowCode:     flow.FlowCode,
		ProjectID:    project.ID,
		TriggerType:  models.TaskFlowTriggerDataChange,
		TriggerVarID: 9001,
		Status:       models.TaskFlowStatusFailed,
		StartedAt:    base,
		ErrorMessage: "plc ack timeout",
	}
	if err := repo.CreateTaskFlowRun(&failedRun); err != nil {
		t.Fatal(err)
	}
	timeoutRun := models.TaskFlowRun{
		FlowID:       flow.ID,
		FlowCode:     flow.FlowCode,
		ProjectID:    project.ID,
		TriggerType:  models.TaskFlowTriggerDataChange,
		TriggerVarID: 9001,
		Status:       models.TaskFlowStatusTimeout,
		StartedAt:    base.Add(time.Second),
		ErrorMessage: "timeout at <eval>:1:1(1)",
	}
	if err := repo.CreateTaskFlowRun(&timeoutRun); err != nil {
		t.Fatal(err)
	}

	failedResp := callHandler(t, http.MethodGet, "/task-flow-runs?status=failed&limit=10", nil, handler.listRuns)
	if failedResp.Code != http.StatusOK {
		t.Fatalf("failed filter status=%d body=%s", failedResp.Code, failedResp.Body.String())
	}
	var failedBody struct {
		Items []models.TaskFlowRun `json:"items"`
		Total int64                `json:"total"`
	}
	mustDecodeHandler(t, failedResp, &failedBody)
	if failedBody.Total != 1 || len(failedBody.Items) != 1 || failedBody.Items[0].ID != failedRun.ID || failedBody.Items[0].Status != models.TaskFlowStatusFailed {
		t.Fatalf("unexpected failed filter body: %+v", failedBody)
	}

	timeoutResp := callHandler(t, http.MethodGet, "/task-flow-runs?status=timeout&limit=10", nil, handler.listRuns)
	if timeoutResp.Code != http.StatusOK {
		t.Fatalf("timeout filter status=%d body=%s", timeoutResp.Code, timeoutResp.Body.String())
	}
	var timeoutBody struct {
		Items []models.TaskFlowRun `json:"items"`
		Total int64                `json:"total"`
	}
	mustDecodeHandler(t, timeoutResp, &timeoutBody)
	if timeoutBody.Total != 1 || len(timeoutBody.Items) != 1 || timeoutBody.Items[0].ID != timeoutRun.ID || timeoutBody.Items[0].Status != models.TaskFlowStatusTimeout {
		t.Fatalf("unexpected timeout filter body: %+v", timeoutBody)
	}

	badStatusResp := callHandler(t, http.MethodGet, "/task-flow-runs?status=done", nil, handler.listRuns)
	if badStatusResp.Code != http.StatusBadRequest {
		t.Fatalf("bad status query should be rejected, got %d body=%s", badStatusResp.Code, badStatusResp.Body.String())
	}
	badTriggerResp := callHandler(t, http.MethodGet, "/task-flow-runs?trigger_type=unknown", nil, handler.listRuns)
	if badTriggerResp.Code != http.StatusBadRequest {
		t.Fatalf("bad trigger_type query should be rejected, got %d body=%s", badTriggerResp.Code, badTriggerResp.Body.String())
	}
	badLimitZeroResp := callHandler(t, http.MethodGet, "/task-flow-runs?limit=0", nil, handler.listRuns)
	if badLimitZeroResp.Code != http.StatusBadRequest {
		t.Fatalf("zero limit query should be rejected, got %d body=%s", badLimitZeroResp.Code, badLimitZeroResp.Body.String())
	}
	badLimitNegativeResp := callHandler(t, http.MethodGet, "/task-flow-runs?limit=-1", nil, handler.listRuns)
	if badLimitNegativeResp.Code != http.StatusBadRequest {
		t.Fatalf("negative limit query should be rejected, got %d body=%s", badLimitNegativeResp.Code, badLimitNegativeResp.Body.String())
	}
	badOffsetResp := callHandler(t, http.MethodGet, "/task-flow-runs?offset=-1", nil, handler.listRuns)
	if badOffsetResp.Code != http.StatusBadRequest {
		t.Fatalf("negative offset query should be rejected, got %d body=%s", badOffsetResp.Code, badOffsetResp.Body.String())
	}
	badProjectResp := callHandler(t, http.MethodGet, "/task-flow-runs?project_id=0", nil, handler.listRuns)
	if badProjectResp.Code != http.StatusBadRequest {
		t.Fatalf("zero project_id query should be rejected, got %d body=%s", badProjectResp.Code, badProjectResp.Body.String())
	}
	badFlowResp := callHandler(t, http.MethodGet, "/task-flow-runs?flow_id=0", nil, handler.listRuns)
	if badFlowResp.Code != http.StatusBadRequest {
		t.Fatalf("zero flow_id query should be rejected, got %d body=%s", badFlowResp.Code, badFlowResp.Body.String())
	}
	badTriggerVarResp := callHandler(t, http.MethodGet, "/task-flow-runs?trigger_var_id=-1", nil, handler.listRuns)
	if badTriggerVarResp.Code != http.StatusBadRequest {
		t.Fatalf("negative trigger_var_id query should be rejected, got %d body=%s", badTriggerVarResp.Code, badTriggerVarResp.Body.String())
	}
	badOriginResp := callHandler(t, http.MethodGet, "/task-flow-runs?origin_flow_id=0", nil, handler.listRuns)
	if badOriginResp.Code != http.StatusBadRequest {
		t.Fatalf("zero origin_flow_id query should be rejected, got %d body=%s", badOriginResp.Code, badOriginResp.Body.String())
	}
	badTimeRangeResp := callHandler(t, http.MethodGet, "/task-flow-runs?from=2026-05-31T02:00:00Z&to=2026-05-31T01:00:00Z", nil, handler.listRuns)
	if badTimeRangeResp.Code != http.StatusBadRequest {
		t.Fatalf("reversed time range query should be rejected, got %d body=%s", badTimeRangeResp.Code, badTimeRangeResp.Body.String())
	}
}

func TestHandlerHelpers(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" || firstNonEmpty("", "") != "" {
		t.Fatal("firstNonEmpty failed")
	}
	name := "new"
	qos := byte(2)
	enabled := false
	updates := gatewayConfigUpdates(gatewayConfigPatchRequest{Name: &name, QOS: &qos, Enabled: &enabled})
	if updates["name"] != "new" || updates["qos"] != byte(2) || updates["enabled"] != false {
		t.Fatalf("unexpected gateway updates: %+v", updates)
	}
	display := "Temp"
	decimals := 3
	scale := 1.2
	tagUpdates := variableUpdates(variablePatchRequest{DisplayName: &display, DecimalPlaces: &decimals, ScaleFactor: &scale})
	if tagUpdates["display_name"] != "Temp" || tagUpdates["decimal_places"] != 3 || tagUpdates["scale_factor"] != 1.2 {
		t.Fatalf("unexpected variable updates: %+v", tagUpdates)
	}
	writable := true
	rwMode := "RW"
	writePath := "setpoint"
	debounceMS := 250
	defaultAlarm := true
	defaultLimitH := 30.0
	tagUpdates = variableUpdates(variablePatchRequest{Writable: &writable, RWMode: &rwMode, WritePath: &writePath, DebounceMS: &debounceMS, DefaultAlarmEnabled: &defaultAlarm, DefaultLimitH: &defaultLimitH})
	if tagUpdates["writable"] != true || tagUpdates["rw_mode"] != "RW" || tagUpdates["write_path"] != "setpoint" || tagUpdates["debounce_ms"] != 250 || tagUpdates["default_alarm_enabled"] != true || *tagUpdates["default_limit_h"].(*float64) != defaultLimitH {
		t.Fatalf("unexpected variable write updates: %+v", tagUpdates)
	}
	ProjectEN := "Project"
	ProjectPatchUpdates, err := ProjectUpdates(ProjectPatchRequest{DisplayNameEN: &ProjectEN})
	if err != nil || ProjectPatchUpdates["display_name_en"] != "Project" {
		t.Fatalf("unexpected Project updates: %+v err=%v", ProjectPatchUpdates, err)
	}
	emptyName := " "
	if _, err := ProjectUpdates(ProjectPatchRequest{Name: &emptyName}); err == nil {
		t.Fatal("expected empty Project name error")
	}
	std, err := detectionStandardFromCreate(detectionStandardCreateRequest{StandardCode: "STD", DisplayName: "标准"})
	if err != nil || std.Name != "标准" || std.Version != 1 || !std.Enabled {
		t.Fatalf("unexpected standard from create: %+v err=%v", std, err)
	}
	stdEN := "Standard"
	stdUpdates, err := detectionStandardUpdates(detectionStandardPatchRequest{DisplayNameEN: &stdEN})
	if err != nil || stdUpdates["display_name_en"] != "Standard" {
		t.Fatalf("unexpected standard updates: %+v err=%v", stdUpdates, err)
	}
	checkDisabled := false
	alarmDisabled := false
	checkOnStart := false
	stdItems, err := detectionStandardItemsFromRequests([]detectionStandardItemRequest{{VarID: 1, VarName: "temp", CheckEnabled: &checkDisabled, AlarmEnabled: &alarmDisabled, CheckCycleMS: 3000, CheckOnStart: &checkOnStart, LimitDeadband: 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stdItems) != 1 || stdItems[0].CheckEnabled || stdItems[0].AlarmEnabled || !stdItems[0].StoreEnabled || stdItems[0].CheckCycleMS != 3000 || stdItems[0].CheckOnStart || stdItems[0].DecimalPlaces != 2 || stdItems[0].CheckMethod != models.CheckMethodNumericRange || stdItems[0].QualityPolicy != models.QualityPolicyIgnoreBad || stdItems[0].LimitDeadband != 0.5 {
		t.Fatalf("unexpected standard items: %+v", stdItems)
	}
	if _, err := detectionStandardItemsFromRequests([]detectionStandardItemRequest{{VarID: 1, VarName: "temp", CheckMethod: "bad"}}); err == nil {
		t.Fatal("expected invalid check_method error")
	}
}

func TestHandlerRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	tags := pipeline.NewTagManager()
	tasks := pipeline.NewTaskManager()
	authService := auth.NewService(repo, auth.NewJWTManager("test-secret", time.Hour), auth.Options{EdgeInstanceID: "edge-test"})
	router := gin.New()
	group := router.Group("/api/v1")
	NewUsersHandler(repo).Register(group, authService)
	NewProjectsHandler(repo).Register(group, authService)
	NewVariablesHandler(services.NewVariablesService(repo, tags)).Register(group, authService)
	NewStorageRoutesHandler(repo).Register(group, authService)
	NewHistoryHandler(repo).Register(group, authService)
	NewDetectionStandardsHandler(repo).Register(group, authService)
	NewGatewaysHandler(repo, mqttx.NewManager(channels), channels).Register(group, authService)
	NewReportTemplatesHandler(services.NewReportTemplatesService(repo)).Register(group, authService)
	NewDetectionRunsHandler(services.NewDetectionRunsService(repo, tasks)).Register(group, authService)
	NewNotificationsHandler(repo).Register(group, authService)
	NewLimitAlarmsHandler(repo).Register(group, authService)
	cfg := config.Default()
	cfg.ConfigPath = filepath.Join(t.TempDir(), "config.json")
	NewSystemConfigHandler(services.NewSystemConfigService(cfg)).Register(group, authService)
	if len(router.Routes()) == 0 {
		t.Fatal("expected registered routes")
	}
}

func TestNotificationResponseKeepsPayloadAsObject(t *testing.T) {
	at := time.Date(2026, 5, 30, 19, 0, 0, 0, time.UTC)
	readAt := at.Add(time.Minute)
	responses := notificationResponses([]models.UserNotification{{
		ID:          1,
		EventUID:    "event-1",
		Type:        models.NotificationDetectionResultNG,
		Level:       models.NotificationLevelWarning,
		TargetType:  models.NotificationTargetProject,
		TargetID:    "2",
		ProjectID:   2,
		ProjectCode: "AC-02",
		VarID:       9212397624135540846,
		Message:     "NG",
		Payload:     `{"result_status":"NG"}`,
		OccurredAt:  at,
		CreatedAt:   at,
		ReadAt:      &readAt,
	}})
	if len(responses) != 1 || string(responses[0].Payload) != `{"result_status":"NG"}` || responses[0].ReadAt == nil {
		t.Fatalf("unexpected response: %+v", responses)
	}
	if responses[0].VarIDText != "9212397624135540846" {
		t.Fatalf("expected exact notification var_id_text, got %+v", responses[0])
	}
	body, err := json.Marshal(responses[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"payload":"`) {
		t.Fatalf("payload should be encoded as object, got %s", body)
	}
}

func TestNotificationsHandlerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	user := &models.SysUser{Username: "admin", PasswordHash: "hash", Role: "admin", Enabled: true, PermissionsVersion: 1}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	item, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:          "route-notification-1",
		Type:        models.NotificationDetectionResultNG,
		Level:       models.NotificationLevelWarning,
		ProjectID:   3,
		ProjectCode: "AC-03",
		Message:     "NG",
		Payload:     map[string]any{"result_status": models.DetectionSummaryStatusNG},
		OccurredAt:  time.Date(2026, 5, 30, 19, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	jwt := auth.NewJWTManager("test-secret", time.Hour)
	authService := auth.NewService(repo, jwt, auth.Options{EdgeInstanceID: "edge-test"})
	token, _, err := jwt.Sign(auth.UserTokenSubject{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		PermissionsVersion: user.PermissionsVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(authService.RequireUser())
	NewNotificationsHandler(repo).Register(group, authService)

	listResp := callRouterWithToken(t, router, http.MethodGet, "/api/v1/notifications?unread=true&project_id=3&limit=5", nil, token)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Items []struct {
			ID      uint64         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	mustDecodeHandler(t, listResp, &listBody)
	if listBody.Total != 1 || len(listBody.Items) != 1 || listBody.Items[0].ID != item.ID || listBody.Items[0].Payload["result_status"] != models.DetectionSummaryStatusNG {
		t.Fatalf("unexpected list body: %+v", listBody)
	}

	countResp := callRouterWithToken(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, token)
	if countResp.Code != http.StatusOK || !strings.Contains(countResp.Body.String(), `"unread":1`) {
		t.Fatalf("unread status=%d body=%s", countResp.Code, countResp.Body.String())
	}
	readResp := callRouterWithToken(t, router, http.MethodPost, "/api/v1/notifications/"+strconv.FormatUint(item.ID, 10)+"/read", map[string]any{}, token)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readResp.Code, readResp.Body.String())
	}
	missingResp := callRouterWithToken(t, router, http.MethodPost, "/api/v1/notifications/99999/read", map[string]any{}, token)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missingResp.Code, missingResp.Body.String())
	}
	readAllResp := callRouterWithToken(t, router, http.MethodPost, "/api/v1/notifications/read-all", map[string]any{}, token)
	if readAllResp.Code != http.StatusOK {
		t.Fatalf("read all status=%d body=%s", readAllResp.Code, readAllResp.Body.String())
	}
	badResp := callRouterWithToken(t, router, http.MethodGet, "/api/v1/notifications?unread=maybe", nil, token)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("bad query status=%d body=%s", badResp.Code, badResp.Body.String())
	}
}

func TestLimitAlarmsHandlerListFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	handler := NewLimitAlarmsHandler(repo)
	Project := createHandlerProject(t, repo)
	now := time.Date(2026, 5, 30, 20, 0, 0, 0, time.UTC)
	startValue := 12.0
	limitValue := 10.0
	if err := repo.CreateDetectionLimitAlarm(&models.DetectionLimitAlarm{
		Scope:         models.AlarmScopeDefault,
		TaskID:        0,
		ProjectID:     Project.ID,
		ProjectCode:   Project.ProjectCode,
		VarID:         9212397624135540845,
		VarName:       "temperature",
		CheckMethod:   models.CheckMethodNumericRange,
		AlarmType:     "above_h",
		AlarmLevel:    "H",
		Status:        models.DetectionAlarmStatusActive,
		StartValue:    &startValue,
		PeakValue:     &startValue,
		LimitValue:    &limitValue,
		Quality:       1,
		FirstSeenAt:   now,
		LastSeenAt:    now,
		LimitDeadband: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateDetectionLimitAlarm(&models.DetectionLimitAlarm{
		Scope:       models.AlarmScopeDetection,
		TaskID:      7,
		TestNo:      "T-007",
		ProjectID:   Project.ID,
		ProjectCode: Project.ProjectCode,
		VarID:       1002,
		VarName:     "pressure",
		CheckMethod: models.CheckMethodNumericRange,
		AlarmType:   "below_l",
		AlarmLevel:  "L",
		Status:      models.DetectionAlarmStatusClosed,
		StartValue:  &startValue,
		PeakValue:   &startValue,
		LimitValue:  &limitValue,
		Quality:     1,
		FirstSeenAt: now.Add(time.Minute),
		LastSeenAt:  now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	resp := callHandler(t, http.MethodGet, "/limit-alarms?scope=default&project_id="+strconv.FormatUint(uint64(Project.ID), 10)+"&status=active&limit=5", nil, handler.list)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Items []models.DetectionLimitAlarm `json:"items"`
		Total int64                        `json:"total"`
		Limit int                          `json:"limit"`
	}
	mustDecodeHandler(t, resp, &body)
	if body.Total != 1 || body.Limit != 5 || len(body.Items) != 1 || body.Items[0].Scope != models.AlarmScopeDefault || body.Items[0].TaskID != 0 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if !strings.Contains(resp.Body.String(), `"var_id_text":"9212397624135540845"`) {
		t.Fatalf("expected exact limit alarm var_id_text, body=%s", resp.Body.String())
	}
	closedResp := callHandler(t, http.MethodGet, "/limit-alarms?scope=detection&status=closed&limit=5", nil, handler.list)
	if closedResp.Code != http.StatusOK {
		t.Fatalf("closed status=%d body=%s", closedResp.Code, closedResp.Body.String())
	}
	var closedBody struct {
		Items []models.DetectionLimitAlarm `json:"items"`
		Total int64                        `json:"total"`
	}
	mustDecodeHandler(t, closedResp, &closedBody)
	if closedBody.Total != 1 || len(closedBody.Items) != 1 || closedBody.Items[0].Status != models.DetectionAlarmStatusClosed {
		t.Fatalf("expected closed alias to return recovered alarm: %+v", closedBody)
	}
	badResp := callHandler(t, http.MethodGet, "/limit-alarms?scope=bad", nil, handler.list)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%s", badResp.Code, badResp.Body.String())
	}
	badStatusResp := callHandler(t, http.MethodGet, "/limit-alarms?status=done", nil, handler.list)
	if badStatusResp.Code != http.StatusBadRequest {
		t.Fatalf("bad status query code=%d body=%s", badStatusResp.Code, badStatusResp.Body.String())
	}
}

func TestParseTagFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/variables?gateway_id=2&enabled=true&discovered=false&keyword=temp", nil)
	ctx.Request = req
	filter, err := parseTagFilter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if *filter.GatewayID != 2 || *filter.Enabled != true || *filter.Discovered != false || filter.Keyword != "temp" {
		t.Fatalf("unexpected filter: %+v", filter)
	}

	for _, rawURL := range []string{"/variables?gateway_id=bad", "/variables?enabled=bad", "/variables?discovered=bad"} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
		if _, err := parseTagFilter(ctx); err == nil {
			t.Fatalf("expected error for %s", rawURL)
		}
	}
}

func TestReportTemplatesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	handler := NewReportTemplatesHandler(services.NewReportTemplatesService(repo))

	resp := callHandler(t, http.MethodPost, "/report-templates", map[string]any{
		"template_code": "RPT-H",
		"name":          "Report",
		"file_ref":      "templates/report.xlsx",
	}, handler.create)
	if resp.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var template models.ReportTemplate
	mustDecodeHandler(t, resp, &template)

	resp = callHandler(t, http.MethodGet, "/report-templates?enabled=true&keyword=RPT", nil, handler.list)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodPatch, "/report-templates/1", map[string]any{"remark": "updated"}, gin.Params{{Key: "id", Value: "1"}}, handler.patch)
	if resp.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodPatch, "/report-templates/404", map[string]any{"remark": "missing"}, gin.Params{{Key: "id", Value: "404"}}, handler.patch)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing report template patch should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodDelete, "/report-templates/1", nil, gin.Params{{Key: "id", Value: "1"}}, handler.delete)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodDelete, "/report-templates/404", nil, gin.Params{{Key: "id", Value: "404"}}, handler.delete)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing report template delete should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandler(t, http.MethodGet, "/report-templates?enabled=bad", nil, handler.list)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid enabled bad request, got %d", resp.Code)
	}
	resp = callHandlerWithParams(t, http.MethodPatch, "/report-templates/bad", map[string]any{}, gin.Params{{Key: "id", Value: "bad"}}, handler.patch)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid id bad request, got %d", resp.Code)
	}
	_ = template
}

func TestUsersProjectsHistoryAndStandardsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	users := NewUsersHandler(repo)
	Projects := NewProjectsHandler(repo)
	history := NewHistoryHandler(repo)
	standards := NewDetectionStandardsHandler(repo)

	resp := callHandler(t, http.MethodPost, "/users", map[string]any{"username": "operator", "password": "Operator@12345", "role": "guest"}, users.create)
	if resp.Code != http.StatusOK {
		t.Fatalf("create user status=%d body=%s", resp.Code, resp.Body.String())
	}
	var user map[string]any
	mustDecodeHandler(t, resp, &user)
	userID := itoaHandler(uint64(user["id"].(float64)))
	if resp = callHandler(t, http.MethodGet, "/users", nil, users.list); resp.Code != http.StatusOK {
		t.Fatalf("list users status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodPatch, "/users/1", map[string]any{"role": "developer"}, gin.Params{{Key: "id", Value: userID}}, users.patch); resp.Code != http.StatusOK {
		t.Fatalf("patch user status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodPost, "/users/1/reset-password", map[string]any{"password": "Operator@67890"}, gin.Params{{Key: "id", Value: userID}}, users.resetPassword); resp.Code != http.StatusOK {
		t.Fatalf("reset password status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodDelete, "/users/1", nil, gin.Params{{Key: "id", Value: userID}}, users.delete); resp.Code != http.StatusOK {
		t.Fatalf("delete user status=%d", resp.Code)
	}
	if resp = callHandler(t, http.MethodPost, "/users", map[string]any{"username": "bad", "password": "x", "role": "bad"}, users.create); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad role, got %d", resp.Code)
	}

	resp = callHandler(t, http.MethodPost, "/projects", map[string]any{
		"project_code":    "AC-H2",
		"display_name":    "设备",
		"display_name_en": "Project",
	}, Projects.create)
	if resp.Code != http.StatusOK {
		t.Fatalf("create Project status=%d body=%s", resp.Code, resp.Body.String())
	}
	var Project models.Project
	mustDecodeHandler(t, resp, &Project)
	if resp = callHandler(t, http.MethodGet, "/projects", nil, Projects.list); resp.Code != http.StatusOK {
		t.Fatalf("list Projects status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodPatch, "/projects/1", map[string]any{"display_name_ja": "設備"}, gin.Params{{Key: "id", Value: itoaHandler(uint64(Project.ID))}}, Projects.patch); resp.Code != http.StatusOK {
		t.Fatalf("patch Project status=%d", resp.Code)
	}
	memberUser := &models.SysUser{Username: "project-member", PasswordHash: "hash", Role: "operator", Enabled: true}
	if err := repo.CreateUser(memberUser); err != nil {
		t.Fatal(err)
	}
	if resp = callHandlerWithParams(t, http.MethodPut, "/projects/1/members", map[string]any{
		"members": []map[string]any{{"user_id": memberUser.ID, "member_role": "operator", "notify_enabled": true}},
	}, gin.Params{{Key: "id", Value: itoaHandler(uint64(Project.ID))}}, Projects.replaceMembers); resp.Code != http.StatusOK {
		t.Fatalf("replace Project members status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"username":"project-member"`) {
		t.Fatalf("expected project member response, body=%s", resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodPut, "/projects/1/members", map[string]any{
		"members": []map[string]any{},
	}, gin.Params{{Key: "id", Value: itoaHandler(uint64(Project.ID))}}, Projects.replaceMembers); resp.Code != http.StatusOK {
		t.Fatalf("clear Project members status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty project members array, body=%s", resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodGet, "/projects/1/members", nil, gin.Params{{Key: "id", Value: itoaHandler(uint64(Project.ID))}}, Projects.listMembers); resp.Code != http.StatusOK {
		t.Fatalf("list Project members status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty project members list array, body=%s", resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodPut, "/projects/1/members", map[string]any{
		"members": []map[string]any{{"user_id": memberUser.ID}, {"user_id": memberUser.ID}},
	}, gin.Params{{Key: "id", Value: itoaHandler(uint64(Project.ID))}}, Projects.replaceMembers); resp.Code != http.StatusBadRequest {
		t.Fatalf("duplicate Project member should be 400, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodGet, "/projects/404/members", nil, gin.Params{{Key: "id", Value: "404"}}, Projects.listMembers); resp.Code != http.StatusNotFound {
		t.Fatalf("missing Project members should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodPatch, "/projects/404", map[string]any{"display_name": "missing"}, gin.Params{{Key: "id", Value: "404"}}, Projects.patch); resp.Code != http.StatusNotFound {
		t.Fatalf("missing Project patch should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandler(t, http.MethodPost, "/projects", map[string]any{"project_code": "bad"}, Projects.create); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing name bad request, got %d", resp.Code)
	}

	value := 21.5
	if err := db.Create(&models.HistoryData{
		GatewayID:   1,
		ProjectID:   Project.ID,
		TaskID:      7,
		TestNo:      "H-1",
		VarID:       9212397624135540844,
		VarName:     "temp",
		ProjectCode: Project.ProjectCode,
		Value:       &value,
		Quality:     1,
		SourceTime:  time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if resp = callHandler(t, http.MethodGet, "/history/data?task_id=7&limit=10", nil, history.data); resp.Code != http.StatusOK {
		t.Fatalf("history data status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"var_id_text":"9212397624135540844"`) {
		t.Fatalf("expected exact history var_id_text, body=%s", resp.Body.String())
	}
	if resp = callHandler(t, http.MethodGet, "/history/data?limit=0", nil, history.data); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid history limit, got %d", resp.Code)
	}

	resp = callHandler(t, http.MethodPost, "/detection-standards", map[string]any{
		"standard_code": "STD-H",
		"display_name":  "标准",
		"project_id":    Project.ID,
		"items": []map[string]any{{
			"var_id":   "9212397624135540847",
			"var_name": "temp",
		}},
	}, standards.create)
	if resp.Code != http.StatusOK {
		t.Fatalf("create standard status=%d body=%s", resp.Code, resp.Body.String())
	}
	var standard models.DetectionStandard
	mustDecodeHandler(t, resp, &standard)
	if !strings.Contains(resp.Body.String(), `"var_id_text":"9212397624135540847"`) {
		t.Fatalf("expected detection standard response to include exact var_id_text, body=%s", resp.Body.String())
	}
	if resp = callHandler(t, http.MethodGet, "/detection-standards?enabled=true", nil, standards.list); resp.Code != http.StatusOK {
		t.Fatalf("list standards status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodGet, "/detection-standards/1", nil, gin.Params{{Key: "id", Value: itoaHandler(uint64(standard.ID))}}, standards.get); resp.Code != http.StatusOK {
		t.Fatalf("get standard status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodPatch, "/detection-standards/1", map[string]any{"remark": "updated"}, gin.Params{{Key: "id", Value: itoaHandler(uint64(standard.ID))}}, standards.patch); resp.Code != http.StatusOK {
		t.Fatalf("patch standard status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodPatch, "/detection-standards/404", map[string]any{"remark": "missing"}, gin.Params{{Key: "id", Value: "404"}}, standards.patch); resp.Code != http.StatusNotFound {
		t.Fatalf("expected missing standard patch to return 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodPut, "/detection-standards/1/items", map[string]any{"items": []map[string]any{{"var_id": "9212397624135540847", "var_name": "temp", "store_enabled": false}}}, gin.Params{{Key: "id", Value: itoaHandler(uint64(standard.ID))}}, standards.replaceItems); resp.Code != http.StatusOK {
		t.Fatalf("replace items status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodPut, "/detection-standards/404/items", map[string]any{"items": []map[string]any{}}, gin.Params{{Key: "id", Value: "404"}}, standards.replaceItems); resp.Code != http.StatusNotFound {
		t.Fatalf("expected missing standard items replace to return 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandlerWithParams(t, http.MethodDelete, "/detection-standards/1", nil, gin.Params{{Key: "id", Value: itoaHandler(uint64(standard.ID))}}, standards.delete); resp.Code != http.StatusOK {
		t.Fatalf("delete standard status=%d", resp.Code)
	}
	if resp = callHandlerWithParams(t, http.MethodDelete, "/detection-standards/404", nil, gin.Params{{Key: "id", Value: "404"}}, standards.delete); resp.Code != http.StatusNotFound {
		t.Fatalf("missing standard delete should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp = callHandler(t, http.MethodGet, "/detection-standards?enabled=bad", nil, standards.list); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid enabled, got %d", resp.Code)
	}
}

func TestGatewaysHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	manager := mqttx.NewManager(channels)
	handler := NewGatewaysHandler(repo, manager, channels)

	if resp := callHandler(t, http.MethodGet, "/runtime/channels", nil, handler.runtimeChannels); resp.Code != http.StatusOK {
		t.Fatalf("runtime channels status=%d", resp.Code)
	}
	if resp := callHandler(t, http.MethodGet, "/gateways", nil, handler.status); resp.Code != http.StatusOK {
		t.Fatalf("gateway status=%d", resp.Code)
	}
	createBody := map[string]any{
		"id":          9,
		"name":        "gw",
		"broker":      "tcp://127.0.0.1:1883",
		"client_id":   "client",
		"topic":       "topic",
		"enabled":     false,
		"parser_type": "kingiot_kio",
	}
	if resp := callHandler(t, http.MethodPost, "/gateway-configs", createBody, handler.createConfig); resp.Code != http.StatusOK {
		t.Fatalf("create gateway status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := callHandler(t, http.MethodGet, "/gateway-configs", nil, handler.listConfigs); resp.Code != http.StatusOK {
		t.Fatalf("list gateways status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodGet, "/gateway-configs/9", nil, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.getConfig); resp.Code != http.StatusOK {
		t.Fatalf("get gateway status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodPatch, "/gateway-configs/9", map[string]any{"name": "gw2", "qos": 2}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.patchConfig); resp.Code != http.StatusOK {
		t.Fatalf("patch gateway status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := callHandlerWithParams(t, http.MethodPatch, "/gateway-configs/404", map[string]any{"name": "missing"}, gin.Params{{Key: "gateway_id", Value: "404"}}, handler.patchConfig); resp.Code != http.StatusNotFound {
		t.Fatalf("missing gateway patch should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateway-configs/9/discover", map[string]any{}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.discover); resp.Code != http.StatusBadRequest {
		t.Fatalf("discover without config status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateways/9/publish", map[string]any{"topic": "x", "payload": map[string]any{"a": 1}}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.publish); resp.Code != http.StatusBadGateway {
		t.Fatalf("publish status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateways/9/subscribe", map[string]any{"topic": "x"}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.subscribe); resp.Code != http.StatusBadGateway {
		t.Fatalf("subscribe status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateways/9/kio/write", map[string]any{"values": []map[string]any{{"name": "A", "value": 1}}}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.kioWrite); resp.Code != http.StatusBadRequest {
		t.Fatalf("kio write missing topic status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateways/9/kio/write", map[string]any{
		"client_id": "client",
		"writer":    "writer",
		"topic":     "topic",
		"wait_ack":  true,
		"values":    []map[string]any{{"name": "A", "value": 1}},
	}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.kioWrite); resp.Code != http.StatusBadGateway {
		t.Fatalf("kio write wait ack status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := callHandlerWithParams(t, http.MethodPost, "/gateways/9/kio/query-all", map[string]any{}, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.kioQueryAll); resp.Code != http.StatusBadRequest {
		t.Fatalf("query all missing topic status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodDelete, "/gateway-configs/9", nil, gin.Params{{Key: "gateway_id", Value: "9"}}, handler.deleteConfig); resp.Code != http.StatusOK {
		t.Fatalf("delete gateway status=%d", resp.Code)
	}
	if resp := callHandlerWithParams(t, http.MethodDelete, "/gateway-configs/404", nil, gin.Params{{Key: "gateway_id", Value: "404"}}, handler.deleteConfig); resp.Code != http.StatusNotFound {
		t.Fatalf("missing gateway delete should be 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if resp := callHandlerWithParams(t, http.MethodGet, "/gateway-configs/bad", nil, gin.Params{{Key: "gateway_id", Value: "bad"}}, handler.getConfig); resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad gateway id, got %d", resp.Code)
	}
}

func TestSystemConfigHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.ConfigPath = filepath.Join(t.TempDir(), "config.json")
	cfg.Database.Password = "secret"
	handler := NewSystemConfigHandler(services.NewSystemConfigService(cfg))

	resp := callHandler(t, http.MethodGet, "/system/database-config", nil, handler.getDatabaseConfig)
	if resp.Code != http.StatusOK {
		t.Fatalf("get config status=%d", resp.Code)
	}
	var view map[string]any
	mustDecodeHandler(t, resp, &view)
	if view["password_set"] != true {
		t.Fatalf("password should be marked as set: %+v", view)
	}
	if _, exists := view["password"]; exists {
		t.Fatalf("password should be redacted: %+v", view)
	}

	resp = callHandler(t, http.MethodPatch, "/system/database-config", map[string]any{"host": "db.local", "port": 3307}, handler.patchDatabaseConfig)
	if resp.Code != http.StatusOK {
		t.Fatalf("patch config status=%d body=%s", resp.Code, resp.Body.String())
	}
	mustDecodeHandler(t, resp, &view)
	if view["host"] != "db.local" || view["restart_required"] != true {
		t.Fatalf("unexpected patch response: %+v", view)
	}

	resp = callHandler(t, http.MethodPatch, "/system/database-config", map[string]any{"port": 70000}, handler.patchDatabaseConfig)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid port bad request, got %d", resp.Code)
	}
	resp = callHandler(t, http.MethodPost, "/system/database-config/test", map[string]any{"host": ""}, handler.testDatabaseConfig)
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected invalid test bad gateway, got %d", resp.Code)
	}
}

func TestDetectionRunsHandlerLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	handler := NewDetectionRunsHandler(services.NewDetectionRunsService(repo, pipeline.NewTaskManager()))
	Project := createHandlerProject(t, repo)
	tag := models.TagConfig{
		VarID:       501,
		GatewayID:   1,
		SourcePath:  "temp",
		RawName:     "temp",
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	route := models.StorageRoute{
		ProjectID:     Project.ID,
		VarID:         tag.VarID,
		RouteCode:     "temp-main",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  "rt_project_1_data",
		ColumnName:    "temp_value",
		ColumnType:    "DOUBLE",
		TriggerMode:   models.StoreTriggerOnCycle,
		CycleMS:       3000,
		StoreOnStart:  true,
		Enabled:       true,
	}
	if err := repo.CreateStorageRoute(&route); err != nil {
		t.Fatal(err)
	}

	resp := callHandler(t, http.MethodPost, "/detection-runs", map[string]any{
		"project_id":    Project.ID,
		"test_no":       "T-H-1",
		"mode":          "standard",
		"duration_sec":  30,
		"operator_note": "memo",
	}, handler.start)
	if resp.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", resp.Code, resp.Body.String())
	}
	var task models.DetectionTask
	mustDecodeHandler(t, resp, &task)

	resp = callHandler(t, http.MethodGet, "/detection-runs?status=running", nil, handler.list)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandler(t, http.MethodGet, "/detection-runs/active", nil, handler.active)
	if resp.Code != http.StatusOK {
		t.Fatalf("active status=%d", resp.Code)
	}
	resp = callHandler(t, http.MethodGet, "/detection-runs/current?project_id=1", nil, handler.current)
	if resp.Code != http.StatusOK {
		t.Fatalf("current status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandler(t, http.MethodGet, "/detection-runs?start=2026-05-29T00:00:00Z&end=2026-05-30T00:00:00Z&limit=10", nil, handler.list)
	if resp.Code != http.StatusOK {
		t.Fatalf("list with time status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1", nil, gin.Params{{Key: "id", Value: "1"}}, handler.get)
	if resp.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1/summary", nil, gin.Params{{Key: "id", Value: "1"}}, handler.summary)
	if resp.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1/events?limit=10", nil, gin.Params{{Key: "id", Value: "1"}}, handler.listEvents)
	if resp.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1/storage-routes", nil, gin.Params{{Key: "id", Value: "1"}}, handler.storageRoutes)
	if resp.Code != http.StatusOK {
		t.Fatalf("storage routes status=%d body=%s", resp.Code, resp.Body.String())
	}
	var routeList struct {
		Items []models.DetectionRunStorageRoute `json:"items"`
		Count int                               `json:"count"`
	}
	mustDecodeHandler(t, resp, &routeList)
	if routeList.Count != 1 || len(routeList.Items) != 1 || routeList.Items[0].ColumnName != "temp_value" {
		t.Fatalf("unexpected storage route snapshot: %+v", routeList)
	}
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/1/notes", map[string]any{"content": "note"}, gin.Params{{Key: "id", Value: "1"}}, handler.addNote)
	if resp.Code != http.StatusOK {
		t.Fatalf("add note status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1/notes", nil, gin.Params{{Key: "id", Value: "1"}}, handler.listNotes)
	if resp.Code != http.StatusOK {
		t.Fatalf("list notes status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/1/pause", map[string]any{"reason": "pause"}, gin.Params{{Key: "id", Value: "1"}}, handler.pause)
	if resp.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandler(t, http.MethodGet, "/detection-runs/current?project_id=1", nil, handler.current)
	if resp.Code != http.StatusOK {
		t.Fatalf("current paused status=%d body=%s", resp.Code, resp.Body.String())
	}
	var pausedCurrent models.DetectionTask
	mustDecodeHandler(t, resp, &pausedCurrent)
	if pausedCurrent.Status != models.DetectionStatusPaused {
		t.Fatalf("expected current paused task, got %+v", pausedCurrent)
	}
	resp = callHandler(t, http.MethodGet, "/detection-runs/current", nil, handler.current)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected current without project_id bad request, got %d", resp.Code)
	}
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/1/resume", nil, gin.Params{{Key: "id", Value: "1"}}, handler.resume)
	if resp.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resp.Code, resp.Body.String())
	}
	value := 19.5
	if err := db.Create(&models.HistoryData{GatewayID: 1, ProjectID: Project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 9212397624135540843, VarName: "temp", ProjectCode: Project.ProjectCode, Value: &value, Quality: 1, SourceTime: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/1/stop", map[string]any{"reason": "done"}, gin.Params{{Key: "id", Value: "1"}}, handler.stop)
	if resp.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodGet, "/detection-runs/1/features", nil, gin.Params{{Key: "id", Value: "1"}}, handler.features)
	if resp.Code != http.StatusOK {
		t.Fatalf("features status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"var_id_text":"9212397624135540843"`) {
		t.Fatalf("expected exact feature var_id_text, body=%s", resp.Body.String())
	}
	resp = callHandler(t, http.MethodPost, "/detection-runs", map[string]any{"project_id": Project.ID, "test_no": "T-H-2", "mode": "standard"}, handler.start)
	if resp.Code != http.StatusOK {
		t.Fatalf("second start status=%d body=%s", resp.Code, resp.Body.String())
	}
	mustDecodeHandler(t, resp, &task)
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/2/abnormal-stop", map[string]any{"reason": "alarm"}, gin.Params{{Key: "id", Value: "2"}}, handler.abnormalStop)
	if resp.Code != http.StatusOK {
		t.Fatalf("abnormal stop status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = callHandlerWithParams(t, http.MethodPost, "/detection-runs/bad/stop", nil, gin.Params{{Key: "id", Value: "bad"}}, handler.stop)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid id bad request, got %d", resp.Code)
	}
}

func callHandler(t *testing.T, method string, target string, body any, fn gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	return callHandlerWithParams(t, method, target, body, nil, fn)
}

func callHandlerWithParams(t *testing.T, method string, target string, body any, params gin.Params, fn gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	resp := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(resp)
	ctx.Request = httptest.NewRequest(method, target, reader)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	fn(ctx)
	return resp
}

func callRouterWithToken(t *testing.T, router http.Handler, method string, target string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(resp, req)
	return resp
}

func mustDecodeHandler(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, resp.Body.String())
	}
}

func newHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

func createHandlerProject(t *testing.T, repo *database.Repository) models.Project {
	t.Helper()
	Project := &models.Project{ProjectCode: "AC-H", Name: "Project", Enabled: true}
	if err := repo.CreateProject(Project); err != nil {
		t.Fatal(err)
	}
	return *Project
}

func itoaHandler(value uint64) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	return string(buf)
}
