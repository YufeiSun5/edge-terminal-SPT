package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spindle-main-server/backend/internal/auth"
	"spindle-main-server/backend/internal/config"
	"spindle-main-server/backend/internal/query"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStationViewEffectiveRouteValidationAndSyncNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-ROUTE", Name: "Route Project", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	invalid := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/station-view/effective", token, nil)
	router.ServeHTTP(invalid, req)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("missing project_id status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	notReady := httptest.NewRecorder()
	req = authedRequest(http.MethodGet, "/api/v1/station-view/effective?project_id=1&edge_instance_id=edge-a", token, nil)
	router.ServeHTTP(notReady, req)
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("sync not ready status=%d body=%s", notReady.Code, notReady.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(notReady.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "sync_not_ready" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestStationViewEffectiveRouteSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-ROUTE-OK", Name: "Route Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := query.TagConfig{VarID: 11, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", VarGroup: "air", DisplayName: "温度", Enabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	template := query.StationViewTemplate{TemplateUID: "tpl-route", TemplateCode: "TPL-ROUTE", Name: "Route Template", Version: 1, Status: query.StationViewStatusPublished}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.StationViewRegion{TemplateUID: template.TemplateUID, RegionKey: "left", RegionType: "metric_grid", SortOrder: 1, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.StationViewItem{TemplateUID: template.TemplateUID, RegionKey: "left", ItemUID: "route-temp", ItemType: "metric_card", BindingType: query.StationViewBindingVarName, BindingKey: "temp", SortOrder: 1, Visible: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.StationViewAssignment{TemplateUID: template.TemplateUID, TargetType: query.StationViewTargetProject, TargetKey: project.ProjectCode, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/station-view/effective?project_id=1", token, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("success status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response query.StationViewEffectiveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.EdgeInstanceID != "edge-a" || response.Template.TemplateCode != "TPL-ROUTE" || len(response.WSSubscription.VarIDs) != 1 || response.WSSubscription.VarIDs[0] != "11" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestProjectsAndCurrentRunRoutesReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-CURRENT", Name: "Current Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-OTHER", Name: "Other Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute)
	task := query.DetectionTask{TestNo: "RUN-CURRENT", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusRunning, StartedAt: &started, LimitCheckEnabled: true}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	otherTask := query.DetectionTask{TestNo: "RUN-OTHER", ProjectID: other.ID, ProjectCode: other.ProjectCode, Mode: "standard", Status: query.DetectionStatusPaused, StartedAt: &started, LimitCheckEnabled: true}
	if err := db.Create(&otherTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 42, VarName: "temp", DisplayName: "温度", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunStorageRoute{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, VarID: 42, RouteCode: "default", StorageTarget: "wide_table", StorageTable: "rt_project_1_data", ColumnName: "temp", ColumnType: "DOUBLE"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, ResultStatus: "running", StartedAt: task.StartedAt, LastRefreshedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	avg := 1.2
	if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 42, VarName: "temp", SampleCount: 2, AvgValue: &avg, FirstSampleTime: started, LastSampleTime: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunEvent{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, EventType: "started", EventLevel: "info", OccurredAt: started}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunReportRequest{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 42, VarName: "temp", VariablesJSON: `[{"var_id":"42"}]`, ParamsJSON: `{}`, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunNote{TaskID: task.ID, NoteType: "memo", Content: "operator memo", ActorType: "user", ActorID: "admin", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}

	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("synced project reads should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	token := loginForTest(t, router, "admin", "Admin@12345")
	projectsRec := httptest.NewRecorder()
	router.ServeHTTP(projectsRec, authedRequest(http.MethodGet, "/api/v1/projects", token, nil))
	if projectsRec.Code != http.StatusOK {
		t.Fatalf("projects status=%d body=%s", projectsRec.Code, projectsRec.Body.String())
	}
	var projects []query.Project
	if err := json.Unmarshal(projectsRec.Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ProjectCode != project.ProjectCode {
		t.Fatalf("projects should be filtered to configured edge, got %+v", projects)
	}

	currentRec := httptest.NewRecorder()
	router.ServeHTTP(currentRec, authedRequest(http.MethodGet, "/api/v1/detection-runs/current?project_id=1", token, nil))
	if currentRec.Code != http.StatusOK {
		t.Fatalf("current status=%d body=%s", currentRec.Code, currentRec.Body.String())
	}
	var current query.DetectionTask
	if err := json.Unmarshal(currentRec.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.ID != task.ID || len(current.StandardItems) != 1 || len(current.StorageRoutes) != 1 {
		t.Fatalf("unexpected current response: %+v", current)
	}
	activeRec := httptest.NewRecorder()
	router.ServeHTTP(activeRec, authedRequest(http.MethodGet, "/api/v1/detection-runs/active", token, nil))
	if activeRec.Code != http.StatusOK {
		t.Fatalf("active status=%d body=%s", activeRec.Code, activeRec.Body.String())
	}
	var active []query.DetectionTask
	if err := json.Unmarshal(activeRec.Body.Bytes(), &active); err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != task.ID {
		t.Fatalf("active runs should filter foreign edge task: %+v", active)
	}
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, authedRequest(http.MethodGet, "/api/v1/detection-runs?project_id=1&limit=10", token, nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listPayload struct {
		Items []query.DetectionTask `json:"items"`
		Count int                   `json:"count"`
		Limit int                   `json:"limit"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if listPayload.Count != 1 || listPayload.Limit != 10 || listPayload.Items[0].ID != task.ID {
		t.Fatalf("unexpected list payload: %+v", listPayload)
	}
	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, authedRequest(http.MethodGet, "/api/v1/detection-runs/1", token, nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail query.DetectionTask
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.ReportRequests) != 1 || len(detail.StorageRoutes) != 1 {
		t.Fatalf("detail should include report requests and routes: %+v", detail)
	}
	for _, path := range []string{
		"/api/v1/detection-runs/1/summary",
		"/api/v1/detection-runs/1/features",
		"/api/v1/detection-runs/1/events",
		"/api/v1/detection-runs/1/storage-routes",
		"/api/v1/detection-runs/1/report-requests",
		"/api/v1/detection-runs/1/notes",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, authedRequest(http.MethodGet, path, token, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	notesRec := httptest.NewRecorder()
	router.ServeHTTP(notesRec, authedRequest(http.MethodGet, "/api/v1/detection-runs/1/notes?limit=5", token, nil))
	if notesRec.Code != http.StatusOK || !strings.Contains(notesRec.Body.String(), `"content":"operator memo"`) {
		t.Fatalf("notes status=%d body=%s", notesRec.Code, notesRec.Body.String())
	}

	mismatchRec := httptest.NewRecorder()
	router.ServeHTTP(mismatchRec, authedRequest(http.MethodGet, "/api/v1/detection-runs/current?project_id=1&edge_instance_id=edge-b", token, nil))
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}
	notesMismatch := httptest.NewRecorder()
	router.ServeHTTP(notesMismatch, authedRequest(http.MethodGet, "/api/v1/detection-runs/1/notes?edge_instance_id=edge-b", token, nil))
	if notesMismatch.Code != http.StatusNotFound {
		t.Fatalf("notes edge mismatch status=%d body=%s", notesMismatch.Code, notesMismatch.Body.String())
	}
}

func TestProjectMembersReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}
	operator := auth.SysUser{Username: "operator", PasswordHash: passwordHash, Role: auth.RoleGuest, Enabled: true, PermissionsVersion: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-MEMBER", Name: "Member Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-MEMBER-OTHER", Name: "Other Member Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.SysProjectMember{ProjectID: project.ID, UserID: operator.ID, MemberRole: "owner", NotifyEnabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.SysProjectMember{ProjectID: other.ID, UserID: operator.ID, MemberRole: "member", NotifyEnabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(testConfig(), db)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/members", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("project members should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/members", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project members status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []query.ProjectMemberView `json:"items"`
		Count int                       `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 1 || payload.Items[0].Username != "operator" || payload.Items[0].MemberRole != "owner" || !payload.Items[0].NotifyEnabled {
		t.Fatalf("unexpected project members payload: %+v", payload)
	}

	mismatch := httptest.NewRecorder()
	mismatchReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/members?edge_instance_id=edge-b", nil)
	mismatchReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(mismatch, mismatchReq)
	if mismatch.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch should be 404 status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}

	writeRec := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/1/members", strings.NewReader(`{"members":[]}`))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"edge_control_required"`) {
		t.Fatalf("project member writes should stay blocked status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}
}

func TestGatewayConfigsReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.GatewayConfig{
		ID:               1,
		Name:             "gw-a",
		Broker:           "tcp://127.0.0.1:1883",
		ClientID:         "client-a",
		Username:         "mqtt-user",
		Password:         "secret-pass",
		Topic:            "edge/a",
		QOS:              1,
		ParserType:       "json",
		KIOClientID:      "kio-client",
		KIOWriter:        "writer-a",
		KIOWriteUsername: "kio-user",
		KIOWritePassword: "kio-secret",
		SetDataTopic:     "edge/a/set",
		WriteResultTopic: "edge/a/write-result",
		QueryAllTopic:    "edge/a/query-all",
		Enabled:          true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.GatewayConfig{
		ID:         2,
		Name:       "gw-b",
		Broker:     "tcp://127.0.0.1:1884",
		ClientID:   "client-b",
		Topic:      "edge/b",
		QOS:        0,
		ParserType: "json",
		Enabled:    false,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	proxyCalled := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer edge.Close()
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.QueryProxyEnabled = true
	router := NewRouter(cfg, db)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/gateway-configs", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("gateway configs should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gateway-configs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("gateway configs status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-pass") || strings.Contains(rec.Body.String(), "kio-secret") || strings.Contains(rec.Body.String(), `"password"`) || strings.Contains(rec.Body.String(), `"kio_write_password"`) {
		t.Fatalf("gateway config response leaked password fields: %s", rec.Body.String())
	}
	var gateways []query.GatewayConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &gateways); err != nil {
		t.Fatal(err)
	}
	if len(gateways) != 2 || gateways[0].ID != 1 || gateways[0].Name != "gw-a" || gateways[1].Enabled {
		t.Fatalf("unexpected gateway configs payload: %+v", gateways)
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/gateway-configs/1", nil)
	detailReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(detail, detailReq)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"name":"gw-a"`) {
		t.Fatalf("gateway config detail status=%d body=%s", detail.Code, detail.Body.String())
	}

	invalid := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/gateway-configs/bad", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(invalid, invalidReq)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"invalid_gateway_id"`) {
		t.Fatalf("invalid gateway id status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	missing := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/gateway-configs/404", nil)
	missingReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(missing, missingReq)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing gateway config status=%d body=%s", missing.Code, missing.Body.String())
	}

	runtime := httptest.NewRecorder()
	runtimeReq := httptest.NewRequest(http.MethodGet, "/api/v1/gateways", nil)
	runtimeReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(runtime, runtimeReq)
	if runtime.Code != http.StatusNotImplemented || !strings.Contains(runtime.Body.String(), `"code":"main_server_runtime_diagnostic_unsupported"`) {
		t.Fatalf("main-server gateway runtime should be explicit 501 status=%d body=%s", runtime.Code, runtime.Body.String())
	}

	writeRec := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/gateway-configs", strings.NewReader(`{"name":"gw-new"}`))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"edge_control_required"`) {
		t.Fatalf("gateway config writes should stay blocked status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}
	if proxyCalled {
		t.Fatal("gateway config routes should not fall through to the edge query proxy")
	}
}

func TestVariablesAndHistoryRoutesReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-HISTORY", Name: "History Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-HISTORY-OTHER", Name: "Other History Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	tag := query.TagConfig{
		VarID:       1001,
		GatewayID:   1,
		SourceType:  "mqtt",
		SourcePath:  "$.temp",
		RawName:     "raw_temp",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		VarGroup:    "air",
		VarName:     "temp",
		DisplayName: "温度",
		JSONPath:    "$.temp",
		DataType:    "FLOAT",
		Enabled:     true,
		Discovered:  true,
	}
	otherTag := query.TagConfig{VarID: 1002, GatewayID: 1, SourceType: "mqtt", SourcePath: "$.other", RawName: "raw_other", ProjectID: &other.ID, ProjectCode: other.ProjectCode, VarName: "temp", DataType: "FLOAT", Enabled: true, Discovered: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherTag).Error; err != nil {
		t.Fatal(err)
	}
	sampleTime := time.Now().Add(-time.Minute).Truncate(time.Second)
	value := 23.5
	if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: 7, TestNo: "RUN-HISTORY", VarID: tag.VarID, VarName: tag.VarName, ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: sampleTime}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: other.ID, TaskID: 8, TestNo: "RUN-OTHER", VarID: otherTag.VarID, VarName: otherTag.VarName, ProjectCode: other.ProjectCode, Value: &value, Quality: 1, SourceTime: sampleTime}).Error; err != nil {
		t.Fatal(err)
	}

	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")
	variablesRec := httptest.NewRecorder()
	router.ServeHTTP(variablesRec, authedRequest(http.MethodGet, "/api/v1/variables?project_id=1&keyword=temp", token, nil))
	if variablesRec.Code != http.StatusOK {
		t.Fatalf("variables status=%d body=%s", variablesRec.Code, variablesRec.Body.String())
	}
	var variables []query.TagConfig
	if err := json.Unmarshal(variablesRec.Body.Bytes(), &variables); err != nil {
		t.Fatal(err)
	}
	if len(variables) != 1 || variables[0].VarID != tag.VarID {
		t.Fatalf("unexpected variables: %+v", variables)
	}
	if !strings.Contains(variablesRec.Body.String(), `"var_id_text":"1001"`) {
		t.Fatalf("variables response should include var_id_text: %s", variablesRec.Body.String())
	}

	historyRec := httptest.NewRecorder()
	router.ServeHTTP(historyRec, authedRequest(http.MethodGet, "/api/v1/history/data?task_id=7&limit=10", token, nil))
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", historyRec.Code, historyRec.Body.String())
	}
	var historyPayload struct {
		Items []query.HistoryData `json:"items"`
		Count int                 `json:"count"`
		Limit int                 `json:"limit"`
	}
	if err := json.Unmarshal(historyRec.Body.Bytes(), &historyPayload); err != nil {
		t.Fatal(err)
	}
	if historyPayload.Count != 1 || historyPayload.Limit != 10 || historyPayload.Items[0].VarID != tag.VarID {
		t.Fatalf("unexpected history payload: %+v", historyPayload)
	}

	mismatchRec := httptest.NewRecorder()
	router.ServeHTTP(mismatchRec, authedRequest(http.MethodGet, "/api/v1/history/data?project_id=1&edge_instance_id=edge-b", token, nil))
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch history status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}
}

func TestHistoryRoutePrefersWideHistoryTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-WIDE", Name: "Wide Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).Truncate(time.Second)
	task := query.DetectionTask{TestNo: "RUN-WIDE", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: "stopped", StartedAt: &started}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunStorageRoute{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, VarID: 2001, RouteCode: "wide-temp", StorageTarget: query.StorageTargetWideTable, StorageTable: "rt_project_1_data", ColumnName: "temp_col", ColumnType: "DOUBLE", QueryAlias: "temp"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE rt_project_1_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER,
		test_no TEXT,
		project_id INTEGER,
		project_code TEXT,
		sample_time DATETIME,
		sample_bucket_ms INTEGER,
		temp_col REAL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO rt_project_1_data (task_id, test_no, project_id, project_code, sample_time, sample_bucket_ms, temp_col) VALUES (?, ?, ?, ?, ?, ?, ?)`, task.ID, task.TestNo, project.ID, project.ProjectCode, started, started.UnixMilli(), 31.2).Error; err != nil {
		t.Fatal(err)
	}

	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/history/data?task_id=1&limit=10", token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("wide history status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []query.HistoryData `json:"items"`
		Count int                 `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 1 || payload.Items[0].VarID != 2001 || payload.Items[0].Value == nil || *payload.Items[0].Value != 31.2 {
		t.Fatalf("unexpected wide history payload: %+v", payload)
	}
}

func TestLimitAlarmsAndTaskFlowRunsReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	project := query.Project{ProjectCode: "AC-RUNS", Name: "Runs Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-RUNS-OTHER", Name: "Other Runs Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	startValue := 45.0
	limitValue := 42.0
	alarm := query.DetectionLimitAlarm{
		Scope:         query.AlarmScopeDetection,
		TaskID:        22,
		TestNo:        "RUN-ALARM",
		ProjectID:     project.ID,
		ProjectCode:   project.ProjectCode,
		VarID:         3001,
		VarName:       "temp",
		DisplayName:   "温度",
		CheckMethod:   "numeric_range",
		AlarmType:     "above_h",
		AlarmLevel:    "H",
		Status:        query.DetectionAlarmStatusOpen,
		StartValue:    &startValue,
		PeakValue:     &startValue,
		LimitValue:    &limitValue,
		FirstSeenAt:   now.Add(-time.Minute),
		LastSeenAt:    now,
		LimitDeadband: 0.1,
		Quality:       1,
	}
	if err := db.Create(&alarm).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionLimitAlarm{Scope: query.AlarmScopeDetection, TaskID: 23, TestNo: "RUN-OTHER", ProjectID: other.ID, ProjectCode: other.ProjectCode, VarID: 3002, VarName: "temp", CheckMethod: "numeric_range", AlarmType: "above_h", AlarmLevel: "H", Status: query.DetectionAlarmStatusOpen, FirstSeenAt: now, LastSeenAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	run := query.TaskFlowRun{
		FlowID:        77,
		FlowCode:      "flow.start",
		ProjectID:     project.ID,
		TriggerType:   query.TaskFlowTriggerDataChange,
		TriggerVarID:  4001,
		OriginFlowID:  11,
		OriginRunID:   12,
		Depth:         1,
		Status:        query.TaskFlowStatusSuccess,
		StartedAt:     now.Add(-time.Minute),
		DurationMS:    12,
		InputSnapshot: `{}`,
		ResultJSON:    `{"ok":true}`,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.TaskFlowSQLLog{RunID: run.ID, FlowID: run.FlowID, SQLText: "SELECT 1", SQLArgs: "[]", AffectedRows: 1, DurationMS: 2, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.TaskFlowRun{FlowID: 88, FlowCode: "flow.other", ProjectID: other.ID, TriggerType: query.TaskFlowTriggerManual, Status: query.TaskFlowStatusSuccess, StartedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	ensureTestAdmin(t, db)
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")
	alarmsRec := httptest.NewRecorder()
	router.ServeHTTP(alarmsRec, authedRequest(http.MethodGet, "/api/v1/limit-alarms?scope=detection&project_id=1&status=active&limit=10", token, nil))
	if alarmsRec.Code != http.StatusOK {
		t.Fatalf("limit alarms status=%d body=%s", alarmsRec.Code, alarmsRec.Body.String())
	}
	var alarmsPayload struct {
		Items []query.DetectionLimitAlarm `json:"items"`
		Total int64                       `json:"total"`
		Limit int                         `json:"limit"`
	}
	if err := json.Unmarshal(alarmsRec.Body.Bytes(), &alarmsPayload); err != nil {
		t.Fatal(err)
	}
	if alarmsPayload.Total != 1 || len(alarmsPayload.Items) != 1 || alarmsPayload.Items[0].VarID != alarm.VarID || !strings.Contains(alarmsRec.Body.String(), `"var_id_text":"3001"`) {
		t.Fatalf("unexpected alarms payload: %s", alarmsRec.Body.String())
	}

	runsRec := httptest.NewRecorder()
	router.ServeHTTP(runsRec, authedRequest(http.MethodGet, "/api/v1/task-flow-runs?project_id=1&status=success&trigger_type=data_change&limit=10", token, nil))
	if runsRec.Code != http.StatusOK {
		t.Fatalf("task flow runs status=%d body=%s", runsRec.Code, runsRec.Body.String())
	}
	var runsPayload struct {
		Items []query.TaskFlowRun `json:"items"`
		Total int64               `json:"total"`
		Limit int                 `json:"limit"`
	}
	if err := json.Unmarshal(runsRec.Body.Bytes(), &runsPayload); err != nil {
		t.Fatal(err)
	}
	if runsPayload.Total != 1 || len(runsPayload.Items) != 1 || runsPayload.Items[0].ID != run.ID || !strings.Contains(runsRec.Body.String(), `"trigger_var_id_text":"4001"`) {
		t.Fatalf("unexpected runs payload: %s", runsRec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, authedRequest(http.MethodGet, "/api/v1/task-flow-runs/1", token, nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("task flow run detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	logsRec := httptest.NewRecorder()
	router.ServeHTTP(logsRec, authedRequest(http.MethodGet, "/api/v1/task-flow-runs/1/sql-logs?limit=10", token, nil))
	if logsRec.Code != http.StatusOK || !strings.Contains(logsRec.Body.String(), "SELECT 1") {
		t.Fatalf("task flow sql logs status=%d body=%s", logsRec.Code, logsRec.Body.String())
	}
	mismatchRec := httptest.NewRecorder()
	router.ServeHTTP(mismatchRec, authedRequest(http.MethodGet, "/api/v1/task-flow-runs/1?edge_instance_id=edge-b", token, nil))
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("task flow edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}
	badAlarmRec := httptest.NewRecorder()
	router.ServeHTTP(badAlarmRec, authedRequest(http.MethodGet, "/api/v1/limit-alarms?scope=bad", token, nil))
	if badAlarmRec.Code != http.StatusBadRequest {
		t.Fatalf("bad alarm scope status=%d body=%s", badAlarmRec.Code, badAlarmRec.Body.String())
	}
	badRunRec := httptest.NewRecorder()
	router.ServeHTTP(badRunRec, authedRequest(http.MethodGet, "/api/v1/task-flow-runs?status=done", token, nil))
	if badRunRec.Code != http.StatusBadRequest {
		t.Fatalf("bad run status=%d body=%s", badRunRec.Code, badRunRec.Body.String())
	}
}

func TestAuditLogsAndNotificationsReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	user := auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-NOTIFY", Name: "Notify Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-NOTIFY-OTHER", Name: "Other Notify Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	if err := db.Create(&query.AuditLog{ActorType: "user", ActorID: "admin", Action: "ws.command.write_variable", TargetType: "variable", TargetID: "5001", Result: "success", Detail: `{"command_id":"cmd-1"}`, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.AuditLog{ActorType: "user", ActorID: "admin", Action: "http.write", TargetType: "variables", TargetID: "5002", Result: "failed", Detail: `{}`, CreatedAt: now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	notification := query.SysNotification{
		EventUID:    "event-edge-a",
		Type:        "alarm.limit.enter",
		Level:       "warning",
		TargetType:  "project",
		TargetID:    project.ProjectCode,
		ProjectID:   project.ID,
		ProjectCode: project.ProjectCode,
		TaskID:      9,
		TestNo:      "RUN-NOTIFY",
		VarID:       5001,
		VarName:     "temp",
		DisplayName: "温度",
		Message:     "温度超限",
		Payload:     `{"ok":true}`,
		OccurredAt:  now,
		CreatedAt:   now,
	}
	globalNotification := query.SysNotification{EventUID: "event-global", Type: "detection.run_started", Level: "info", TargetType: "all", Message: "检测开始", Payload: `{}`, OccurredAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Minute)}
	otherNotification := query.SysNotification{EventUID: "event-edge-b", Type: "alarm.limit.enter", Level: "warning", TargetType: "project", ProjectID: other.ID, ProjectCode: other.ProjectCode, Message: "other", Payload: `{}`, OccurredAt: now, CreatedAt: now}
	expiredAt := now.Add(-time.Second)
	expiredNotification := query.SysNotification{EventUID: "event-expired", Type: "alarm.limit.enter", Level: "warning", TargetType: "project", ProjectID: project.ID, ProjectCode: project.ProjectCode, Message: "expired", Payload: `{}`, OccurredAt: now, ExpiresAt: &expiredAt, CreatedAt: now}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&globalNotification).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherNotification).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&expiredNotification).Error; err != nil {
		t.Fatal(err)
	}
	for _, notificationID := range []uint64{notification.ID, globalNotification.ID, otherNotification.ID, expiredNotification.ID} {
		if err := db.Create(&query.SysNotificationRecipient{NotificationID: notificationID, UserID: user.ID, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	auditRec := httptest.NewRecorder()
	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action=ws.command.write_variable&result=success&limit=10", nil)
	auditReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", auditRec.Code, auditRec.Body.String())
	}
	var auditPayload struct {
		Items []query.AuditLog `json:"items"`
		Total int64            `json:"total"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(auditRec.Body.Bytes(), &auditPayload); err != nil {
		t.Fatal(err)
	}
	if auditPayload.Total != 1 || auditPayload.Limit != 10 || auditPayload.Items[0].TargetID != "5001" {
		t.Fatalf("unexpected audit payload: %+v", auditPayload)
	}

	notificationsRec := httptest.NewRecorder()
	notificationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?unread=true&keyword=%E6%B8%A9%E5%BA%A6&limit=10", nil)
	notificationsReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(notificationsRec, notificationsReq)
	if notificationsRec.Code != http.StatusOK {
		t.Fatalf("notifications status=%d body=%s", notificationsRec.Code, notificationsRec.Body.String())
	}
	var notificationsPayload struct {
		Items []struct {
			EventUID string         `json:"event_uid"`
			Payload  map[string]any `json:"payload"`
		} `json:"items"`
		Total int64 `json:"total"`
		Limit int   `json:"limit"`
	}
	if err := json.Unmarshal(notificationsRec.Body.Bytes(), &notificationsPayload); err != nil {
		t.Fatal(err)
	}
	if notificationsPayload.Total != 1 || len(notificationsPayload.Items) != 1 || notificationsPayload.Items[0].EventUID != notification.EventUID {
		t.Fatalf("unexpected notifications payload: %s", notificationsRec.Body.String())
	}
	if !strings.Contains(notificationsRec.Body.String(), `"var_id_text":"5001"`) || !strings.Contains(notificationsRec.Body.String(), `"payload":{"ok":true}`) {
		t.Fatalf("notification should include var_id_text and JSON payload: %s", notificationsRec.Body.String())
	}

	countRec := httptest.NewRecorder()
	countReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	countReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(countRec, countReq)
	if countRec.Code != http.StatusOK || !strings.Contains(countRec.Body.String(), `"unread":2`) {
		t.Fatalf("unread count status=%d body=%s", countRec.Code, countRec.Body.String())
	}

	mismatchRec := httptest.NewRecorder()
	mismatchReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?project_id=1&edge_instance_id=edge-b", nil)
	mismatchReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("notification edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}

	writeRec := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	writeReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"main_server_notification_read_unsupported"`) {
		t.Fatalf("notification write diagnostic status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}

	badAuditRec := httptest.NewRecorder()
	badAuditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?limit=0", nil)
	badAuditReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badAuditRec, badAuditReq)
	if badAuditRec.Code != http.StatusBadRequest {
		t.Fatalf("bad audit query status=%d body=%s", badAuditRec.Code, badAuditRec.Body.String())
	}
}

func TestReportTemplatesReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	template := query.ReportTemplate{
		TemplateCode:     "PERF",
		Name:             "Performance Report",
		DisplayName:      "性能报表",
		FileRef:          "templates/perf.xlsx",
		FileKind:         "xlsx",
		Version:          3,
		ParamsSchemaJSON: `[{"key":"inlet_area_m2","type":"number"}]`,
		Enabled:          true,
		Remark:           "synced",
	}
	disabled := query.ReportTemplate{TemplateCode: "OLD", Name: "Old Report", FileRef: "templates/old.xlsx", FileKind: "xlsx", Version: 1, Enabled: false}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/report-templates?enabled=true&keyword=PERF", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("report templates status=%d body=%s", rec.Code, rec.Body.String())
	}
	var templates []query.ReportTemplate
	if err := json.Unmarshal(rec.Body.Bytes(), &templates); err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].TemplateCode != template.TemplateCode || templates[0].ParamsSchemaJSON == "" {
		t.Fatalf("unexpected report templates: %s", rec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/report-templates?enabled=bad", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || !strings.Contains(badRec.Body.String(), `"code":"invalid_query"`) {
		t.Fatalf("bad enabled status=%d body=%s", badRec.Code, badRec.Body.String())
	}
}

func TestStorageRoutesReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-STORE", Name: "Storage Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-STORE-OTHER", Name: "Other Storage Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	route := query.StorageRoute{
		ProjectID:     project.ID,
		VarID:         9007199254740993,
		RouteCode:     "temp-default",
		StorageTarget: query.StorageTargetWideTable,
		StorageTable:  "rt_project_1_data",
		ColumnName:    "temp_col",
		ColumnType:    "DOUBLE",
		FormFieldKey:  "temperature",
		QueryAlias:    "temp",
		TriggerMode:   "on_cycle",
		CycleMS:       3000,
		Deadband:      0.1,
		StoreOnStart:  true,
		Enabled:       true,
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.StorageRoute{ProjectID: other.ID, VarID: 8001, RouteCode: "other", StorageTarget: query.StorageTargetWideTable, StorageTable: "rt_project_2_data", ColumnName: "other_col", ColumnType: "DOUBLE", TriggerMode: "on_cycle", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage-routes?project_id=1&var_id=9007199254740993&enabled=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("storage routes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var routes []query.StorageRoute
	if err := json.Unmarshal(rec.Body.Bytes(), &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].RouteCode != route.RouteCode || routes[0].ColumnName != "temp_col" {
		t.Fatalf("unexpected storage routes: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"var_id_text":"9007199254740993"`) {
		t.Fatalf("storage route should include var_id_text: %s", rec.Body.String())
	}

	allRec := httptest.NewRecorder()
	allReq := httptest.NewRequest(http.MethodGet, "/api/v1/storage-routes", nil)
	allReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("all storage routes status=%d body=%s", allRec.Code, allRec.Body.String())
	}
	var edgeRoutes []query.StorageRoute
	if err := json.Unmarshal(allRec.Body.Bytes(), &edgeRoutes); err != nil {
		t.Fatal(err)
	}
	if len(edgeRoutes) != 1 || edgeRoutes[0].ProjectID != project.ID {
		t.Fatalf("storage routes should be filtered to configured edge: %s", allRec.Body.String())
	}

	mismatchRec := httptest.NewRecorder()
	mismatchReq := httptest.NewRequest(http.MethodGet, "/api/v1/storage-routes?project_id=1&edge_instance_id=edge-b", nil)
	mismatchReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/storage-routes?enabled=bad", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || !strings.Contains(badRec.Body.String(), `"code":"invalid_query"`) {
		t.Fatalf("bad enabled status=%d body=%s", badRec.Code, badRec.Body.String())
	}

	writeRec := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/storage-routes", strings.NewReader(`{"project_id":1}`))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"edge_control_required"`) {
		t.Fatalf("storage route write should be blocked on main server status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}
}

func TestTaskFlowsReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-FLOW", Name: "Flow Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-FLOW-OTHER", Name: "Other Flow Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	flow := query.TaskFlow{
		ProjectID:          project.ID,
		FlowCode:           "flow.start",
		Name:               "Start Detection",
		Enabled:            true,
		TriggerType:        query.TaskFlowTriggerDataChange,
		ConditionScript:    `task_params.command === "start_detection"`,
		ActionType:         "builtin.start_detection_run",
		StepsJSON:          `[{"code":"start","module":"builtin.start_detection_run","params":{"project_id":{"source":"trigger_param","key":"project_id"}}}]`,
		TimeoutMS:          3000,
		CooldownMS:         100,
		ScheduleIntervalMS: 0,
		Priority:           9,
		Remark:             "synced",
	}
	if err := db.Create(&flow).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.TaskFlowVar{FlowID: flow.ID, ProjectID: project.ID, VarID: 9007199254740993, VarName: "request", Role: "watch"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.TaskFlow{ProjectID: other.ID, FlowCode: "flow.other", Name: "Other Flow", Enabled: true, TriggerType: query.TaskFlowTriggerDataChange, ActionType: "javascript", Priority: 1}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows?project_id=1&trigger_type=data_change&enabled=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("task flows status=%d body=%s", rec.Code, rec.Body.String())
	}
	var flows []query.TaskFlow
	if err := json.Unmarshal(rec.Body.Bytes(), &flows); err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 || flows[0].FlowCode != flow.FlowCode || len(flows[0].Vars) != 1 {
		t.Fatalf("unexpected task flows: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"var_id_text":"9007199254740993"`) {
		t.Fatalf("task flow vars should include var_id_text: %s", rec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows/1", nil)
	detailReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK || !strings.Contains(detailRec.Body.String(), `"flow_code":"flow.start"`) {
		t.Fatalf("task flow detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}

	allRec := httptest.NewRecorder()
	allReq := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows", nil)
	allReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("all task flows status=%d body=%s", allRec.Code, allRec.Body.String())
	}
	var edgeFlows []query.TaskFlow
	if err := json.Unmarshal(allRec.Body.Bytes(), &edgeFlows); err != nil {
		t.Fatal(err)
	}
	if len(edgeFlows) != 1 || edgeFlows[0].ProjectID != project.ID {
		t.Fatalf("task flows should be filtered to configured edge: %s", allRec.Body.String())
	}

	mismatchRec := httptest.NewRecorder()
	mismatchReq := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows?project_id=1&edge_instance_id=edge-b", nil)
	mismatchReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows?trigger_type=bad", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || !strings.Contains(badRec.Body.String(), `"code":"invalid_query"`) {
		t.Fatalf("bad trigger_type status=%d body=%s", badRec.Code, badRec.Body.String())
	}

	runtimeRec := httptest.NewRecorder()
	runtimeReq := httptest.NewRequest(http.MethodGet, "/api/v1/task-flows/runtime", nil)
	runtimeReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(runtimeRec, runtimeReq)
	if runtimeRec.Code != http.StatusServiceUnavailable || !strings.Contains(runtimeRec.Body.String(), `"code":"edge_runtime_token_missing"`) {
		t.Fatalf("runtime diagnostic mirror should require edge token status=%d body=%s", runtimeRec.Code, runtimeRec.Body.String())
	}

	writeRec := httptest.NewRecorder()
	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/task-flows", strings.NewReader(`{"project_id":1}`))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"edge_control_required"`) {
		t.Fatalf("task flow write should be blocked on main server status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}
}

func TestDetectionStandardsReadSyncedTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-STD", Name: "Standard Project", EdgeInstanceID: "edge-a", Enabled: true}
	other := query.Project{ProjectCode: "AC-STD-OTHER", Name: "Other Standard Project", EdgeInstanceID: "edge-b", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	standard := query.DetectionStandard{
		StandardCode:     "STD-PERF",
		Name:             "Performance Standard",
		DisplayName:      "性能检测标准",
		DisplayNameEN:    "Performance Standard",
		DisplayNameJA:    "性能検査標準",
		ProjectID:        &project.ID,
		ProjectCode:      project.ProjectCode,
		Mode:             "standard",
		ReportTemplateID: nil,
		Version:          2,
		Enabled:          true,
		Remark:           "synced",
	}
	otherStandard := query.DetectionStandard{StandardCode: "STD-OTHER", Name: "Other Standard", ProjectID: &other.ID, ProjectCode: other.ProjectCode, Mode: "standard", Version: 1, Enabled: true}
	if err := db.Create(&standard).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherStandard).Error; err != nil {
		t.Fatal(err)
	}
	limitH := 42.5
	if err := db.Create(&query.DetectionStandardItem{
		StandardID:      standard.ID,
		VarID:           9007199254740993,
		VarName:         "temp",
		DisplayName:     "温度",
		CheckEnabled:    true,
		AlarmEnabled:    true,
		StoreEnabled:    false,
		CheckCycleMS:    3000,
		CheckOnStart:    true,
		CheckMethod:     "numeric_range",
		LimitH:          &limitH,
		ViolationHoldMS: 1000,
		RecoverHoldMS:   2000,
		QualityPolicy:   "ignore_bad",
		Unit:            "C",
		DecimalPlaces:   1,
		SortOrder:       1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionStandardFavorite{UserID: 1, StandardID: standard.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionStandardRecent{UserID: 1, StandardID: standard.ID, ProjectID: project.ID, LastUsedAt: time.Now(), UseCount: 3}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards?project_id=1&enabled=true&keyword=PERF", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("detection standards list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var standards []query.DetectionStandard
	if err := json.Unmarshal(listRec.Body.Bytes(), &standards); err != nil {
		t.Fatal(err)
	}
	if len(standards) != 1 || standards[0].StandardCode != standard.StandardCode {
		t.Fatalf("unexpected detection standards list: %s", listRec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards/1", nil)
	detailReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detection standard detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail query.DetectionStandard
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Items) != 1 || detail.Items[0].VarID != 9007199254740993 || !strings.Contains(detailRec.Body.String(), `"var_id_text":"9007199254740993"`) {
		t.Fatalf("detail should include standard items and var_id_text: %s", detailRec.Body.String())
	}

	favoritesRec := httptest.NewRecorder()
	favoritesReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards/favorites", nil)
	favoritesReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(favoritesRec, favoritesReq)
	if favoritesRec.Code != http.StatusOK || !strings.Contains(favoritesRec.Body.String(), `"count":1`) {
		t.Fatalf("favorites status=%d body=%s", favoritesRec.Code, favoritesRec.Body.String())
	}

	recentRec := httptest.NewRecorder()
	recentReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards/recent?project_id=1&limit=5", nil)
	recentReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recentRec, recentReq)
	if recentRec.Code != http.StatusOK || !strings.Contains(recentRec.Body.String(), `"limit":5`) || !strings.Contains(recentRec.Body.String(), `"count":1`) {
		t.Fatalf("recent status=%d body=%s", recentRec.Code, recentRec.Body.String())
	}

	mismatchRec := httptest.NewRecorder()
	mismatchReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards?project_id=1&edge_instance_id=edge-b", nil)
	mismatchReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusNotFound {
		t.Fatalf("edge mismatch status=%d body=%s", mismatchRec.Code, mismatchRec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/detection-standards?enabled=bad", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || !strings.Contains(badRec.Body.String(), `"code":"invalid_query"`) {
		t.Fatalf("bad enabled status=%d body=%s", badRec.Code, badRec.Body.String())
	}
}

func TestMainServerSyncDiagnosticsReportsMissingTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-DIAG", Name: "Diagnostics Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/main-server/sync-diagnostics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Role           string `json:"role"`
		EdgeInstanceID string `json:"edge_instance_id"`
		Diagnostics    struct {
			OverallStatus string `json:"overall_status"`
			Tables        []struct {
				Name     string `json:"name"`
				Status   string `json:"status"`
				RowCount int64  `json:"row_count"`
			} `json:"tables"`
			MissingTables []string `json:"missing_tables"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Role != "main_server" || payload.EdgeInstanceID != "edge-a" || payload.Diagnostics.OverallStatus != "ok" {
		t.Fatalf("unexpected diagnostics payload: %+v", payload)
	}
	projectsFound := false
	reportTemplatesFound := false
	detectionStandardsFound := false
	storageRoutesFound := false
	taskFlowsFound := false
	for _, table := range payload.Diagnostics.Tables {
		if table.Name == "sys_projects" {
			projectsFound = true
			if table.Status != "ok" || table.RowCount != 1 {
				t.Fatalf("unexpected sys_projects diagnostics: %+v", table)
			}
		}
		if table.Name == "sys_report_templates" {
			reportTemplatesFound = true
			if table.Status != "ok" {
				t.Fatalf("unexpected sys_report_templates diagnostics: %+v", table)
			}
		}
		if table.Name == "sys_detection_standards" {
			detectionStandardsFound = true
			if table.Status != "ok" {
				t.Fatalf("unexpected sys_detection_standards diagnostics: %+v", table)
			}
		}
		if table.Name == "sys_storage_routes" {
			storageRoutesFound = true
			if table.Status != "ok" {
				t.Fatalf("unexpected sys_storage_routes diagnostics: %+v", table)
			}
		}
		if table.Name == "sys_task_flows" {
			taskFlowsFound = true
			if table.Status != "ok" {
				t.Fatalf("unexpected sys_task_flows diagnostics: %+v", table)
			}
		}
	}
	if !projectsFound {
		t.Fatalf("sys_projects diagnostics missing: %+v", payload.Diagnostics.Tables)
	}
	if !reportTemplatesFound {
		t.Fatalf("sys_report_templates diagnostics missing: %+v", payload.Diagnostics.Tables)
	}
	if !detectionStandardsFound {
		t.Fatalf("sys_detection_standards diagnostics missing: %+v", payload.Diagnostics.Tables)
	}
	if !storageRoutesFound {
		t.Fatalf("sys_storage_routes diagnostics missing: %+v", payload.Diagnostics.Tables)
	}
	if !taskFlowsFound {
		t.Fatalf("sys_task_flows diagnostics missing: %+v", payload.Diagnostics.Tables)
	}

	if err := db.Migrator().DropTable(&query.SysNotification{}); err != nil {
		t.Fatal(err)
	}
	degradedRec := httptest.NewRecorder()
	degradedReq := httptest.NewRequest(http.MethodGet, "/api/v1/main-server/sync-diagnostics", nil)
	degradedReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(degradedRec, degradedReq)
	if degradedRec.Code != http.StatusOK {
		t.Fatalf("degraded diagnostics status=%d body=%s", degradedRec.Code, degradedRec.Body.String())
	}
	if err := json.Unmarshal(degradedRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Diagnostics.OverallStatus != "degraded" || !containsString(payload.Diagnostics.MissingTables, "sys_notifications") {
		t.Fatalf("expected missing sys_notifications diagnostics, got %+v", payload.Diagnostics)
	}
}

func TestMainServerReportReadinessChecksSyncedRunData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-REPORT", Name: "Report Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	task := query.DetectionTask{TestNo: "RUN-REPORT", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &ended, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunReportRequest{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF", TemplateVersion: 2, VarID: 6001, VarName: "temp", VariablesJSON: `[{"var_id":"6001"}]`, ParamsJSON: `{"area":1.2}`, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, ResultStatus: "ok", HistoryRows: 1, StartedAt: &ended, EndedAt: &ended, LastRefreshedAt: ended}).Error; err != nil {
		t.Fatal(err)
	}
	avg := 23.4
	if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 6001, VarName: "temp", SampleCount: 1, AvgValue: &avg, FirstSampleTime: ended, LastSampleTime: ended}).Error; err != nil {
		t.Fatal(err)
	}
	value := 23.4
	if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 6001, VarName: "temp", ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: ended}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionLimitAlarm{Scope: query.AlarmScopeDetection, TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 6001, VarName: "temp", CheckMethod: "numeric_range", AlarmType: "above_h", AlarmLevel: "H", Status: query.DetectionAlarmStatusClose, FirstSeenAt: ended, LastSeenAt: ended}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(testConfig(), db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/main-server/report-readiness?task_id=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("report readiness status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Readiness struct {
			OverallStatus string `json:"overall_status"`
			Counts        struct {
				ReportRequests int   `json:"report_requests"`
				HistoryRows    int64 `json:"history_rows"`
				AlarmRows      int64 `json:"alarm_rows"`
			} `json:"counts"`
			Requests []struct {
				RequestID      uint64   `json:"request_id"`
				RequiredVarIDs []string `json:"required_var_ids"`
				Ready          bool     `json:"ready"`
			} `json:"requests"`
		} `json:"readiness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Readiness.OverallStatus != query.ReportReadinessReady || payload.Readiness.Counts.ReportRequests != 1 || payload.Readiness.Counts.HistoryRows != 1 || payload.Readiness.Counts.AlarmRows != 1 {
		t.Fatalf("unexpected ready payload: %s", rec.Body.String())
	}
	if len(payload.Readiness.Requests) != 1 || !payload.Readiness.Requests[0].Ready || !containsString(payload.Readiness.Requests[0].RequiredVarIDs, "6001") {
		t.Fatalf("unexpected request readiness: %s", rec.Body.String())
	}

	waitingTask := query.DetectionTask{TestNo: "RUN-WAITING", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusRunning, StartedAt: &ended}
	if err := db.Create(&waitingTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunReportRequest{TaskID: waitingTask.ID, TestNo: waitingTask.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF", VarID: 6002, VarName: "humidity", VariablesJSON: `[{"var_id":"6002"}]`, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	waitingRec := httptest.NewRecorder()
	waitingReq := httptest.NewRequest(http.MethodGet, "/api/v1/main-server/report-readiness?task_id=2", nil)
	waitingReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(waitingRec, waitingReq)
	if waitingRec.Code != http.StatusOK {
		t.Fatalf("waiting readiness status=%d body=%s", waitingRec.Code, waitingRec.Body.String())
	}
	if !strings.Contains(waitingRec.Body.String(), `"overall_status":"waiting"`) || !strings.Contains(waitingRec.Body.String(), `"missing_history_var_ids":["6002"]`) {
		t.Fatalf("waiting readiness should expose missing synchronized data: %s", waitingRec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/main-server/report-readiness", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("missing task_id status=%d body=%s", badRec.Code, badRec.Body.String())
	}
}

func TestEdgeControlRouteForwardsWithServiceToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	t.Setenv("EDGE_TEST_TOKEN", "test-secret")
	var seenPath string
	var seenQuery string
	var seenAuth string
	var seenCommandID string
	var seenBody string
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenAuth = r.Header.Get("Authorization")
		seenCommandID = r.Header.Get("X-Command-ID")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		seenBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true,"status":"accepted"}`))
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_TEST_TOKEN"
	router := NewRouter(cfg, db)

	rec := httptest.NewRecorder()
	body := `{"command_id":"cmd-1","operator_username":"admin","payload":{"var_id":"42","value":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/edge-control/variables/write?trace=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("forward status=%d body=%s", rec.Code, rec.Body.String())
	}
	if seenPath != "/api/v1/edge-control/variables/write" || seenQuery != "trace=1" {
		t.Fatalf("unexpected forwarded target path=%s query=%s", seenPath, seenQuery)
	}
	if seenAuth != "Bearer test-secret" || seenCommandID != "cmd-1" || seenBody != body {
		t.Fatalf("unexpected forwarded request auth=%q command=%q body=%s", seenAuth, seenCommandID, seenBody)
	}
}

func TestEdgeControlRouteRequiresConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_TEST_TOKEN"
	router := NewRouter(cfg, db)

	rec := httptest.NewRecorder()
	body := `{"command_id":"cmd-missing","operator_username":"admin","payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/edge-control/detection/refresh-features", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge backend should not be called when service token is missing")
	}
	if !strings.Contains(rec.Body.String(), `"code":"edge_control_token_missing"`) || !strings.Contains(rec.Body.String(), `"service_token_ref":"EDGE_MISSING_TEST_TOKEN"`) {
		t.Fatalf("unexpected missing token body=%s", rec.Body.String())
	}
}

func TestRealtimeVariablesRouteForwardsToEdgeServiceEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDGE_REALTIME_TEST_TOKEN", "edge-realtime-secret")
	var seenPath string
	var seenQuery string
	var seenAuth string
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"var_id":9101,"var_id_text":"9101","project_id":1,"var_name":"temp","value":25.5,"quality":1}]`))
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_REALTIME_TEST_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime/variables?project_id=1&var_id=9101", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("realtime status=%d body=%s", rec.Code, rec.Body.String())
	}
	if seenPath != "/api/v1/edge-control/realtime/variables" || seenQuery != "project_id=1&var_id=9101" || seenAuth != "Bearer edge-realtime-secret" {
		t.Fatalf("unexpected edge realtime request path=%s query=%s auth=%q", seenPath, seenQuery, seenAuth)
	}
	if !strings.Contains(rec.Body.String(), `"var_id_text":"9101"`) || !strings.Contains(rec.Body.String(), `"value":25.5`) {
		t.Fatalf("unexpected realtime body=%s", rec.Body.String())
	}
}

func TestRealtimeVariablesRouteRequiresConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_REALTIME_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime/variables?project_id=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing realtime token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge backend should not be called when realtime service token is missing")
	}
	if !strings.Contains(rec.Body.String(), `"code":"edge_realtime_token_missing"`) || !strings.Contains(rec.Body.String(), `"service_token_ref":"EDGE_MISSING_REALTIME_TOKEN"`) {
		t.Fatalf("unexpected realtime missing token body=%s", rec.Body.String())
	}
}

func TestDetectionRunUserRoutesForwardThroughEdgeControlEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	ensureTestAdmin(t, db)
	type seenRequest struct {
		Path      string
		Auth      string
		CommandID string
		Body      map[string]any
	}
	var seen []seenRequest
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		body := map[string]any{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("edge body is not json: %s", raw)
		}
		seen = append(seen, seenRequest{
			Path:      r.URL.Path,
			Auth:      r.Header.Get("Authorization"),
			CommandID: r.Header.Get("X-Command-ID"),
			Body:      body,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","id":123}`))
	}))
	t.Cleanup(edge.Close)
	t.Setenv("EDGE_USER_CONTROL_TOKEN", "edge-secret")
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_USER_CONTROL_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	start := httptest.NewRecorder()
	startReq := authedRequest(http.MethodPost, "/api/v1/detection-runs", token, strings.NewReader(`{"project_id":1,"test_no":"T-1","command_id":"drop-me"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.Header.Set("X-Command-ID", "user-start-1")
	router.ServeHTTP(start, startReq)
	if start.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	if len(seen) != 1 || seen[0].Path != "/api/v1/edge-control/detection/start" || seen[0].Auth != "Bearer edge-secret" || seen[0].CommandID != "user-start-1" {
		t.Fatalf("unexpected start forward: %+v", seen)
	}
	if seen[0].Body["operator_username"] != "admin" || seen[0].Body["command_id"] != "user-start-1" {
		t.Fatalf("start envelope missing operator or command: %+v", seen[0].Body)
	}
	startPayload, ok := seen[0].Body["payload"].(map[string]any)
	if !ok || startPayload["test_no"] != "T-1" || startPayload["command_id"] != nil {
		t.Fatalf("unexpected start payload: %+v", seen[0].Body["payload"])
	}

	stop := httptest.NewRecorder()
	stopReq := authedRequest(http.MethodPost, "/api/v1/detection-runs/123/stop", token, strings.NewReader(`{"reason":"done"}`))
	stopReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(stop, stopReq)
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", stop.Code, stop.Body.String())
	}
	if len(seen) != 2 || seen[1].Path != "/api/v1/edge-control/detection/stop" || seen[1].CommandID == "" {
		t.Fatalf("unexpected stop forward: %+v", seen)
	}
	stopPayload, ok := seen[1].Body["payload"].(map[string]any)
	if !ok || stopPayload["reason"] != "done" || stopPayload["task_id"].(float64) != 123 {
		t.Fatalf("unexpected stop payload: %+v", seen[1].Body["payload"])
	}

	abnormal := httptest.NewRecorder()
	abnormalReq := authedRequest(http.MethodPost, "/api/v1/detection-runs/123/abnormal-stop", token, strings.NewReader(`{"reason":"fault"}`))
	abnormalReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(abnormal, abnormalReq)
	if abnormal.Code != http.StatusOK {
		t.Fatalf("abnormal stop status=%d body=%s", abnormal.Code, abnormal.Body.String())
	}
	if len(seen) != 3 || seen[2].Path != "/api/v1/edge-control/detection/abnormal-stop" {
		t.Fatalf("unexpected abnormal stop forward: %+v", seen)
	}

	pause := httptest.NewRecorder()
	pauseReq := authedRequest(http.MethodPost, "/api/v1/detection-runs/123/pause", token, strings.NewReader(`{"reason":"operator pause"}`))
	pauseReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(pause, pauseReq)
	if pause.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", pause.Code, pause.Body.String())
	}
	if len(seen) != 4 || seen[3].Path != "/api/v1/edge-control/detection/pause" {
		t.Fatalf("unexpected pause forward: %+v", seen)
	}
	pausePayload, ok := seen[3].Body["payload"].(map[string]any)
	if !ok || pausePayload["reason"] != "operator pause" || pausePayload["task_id"].(float64) != 123 {
		t.Fatalf("unexpected pause payload: %+v", seen[3].Body["payload"])
	}

	resume := httptest.NewRecorder()
	resumeReq := authedRequest(http.MethodPost, "/api/v1/detection-runs/123/resume", token, strings.NewReader(`{}`))
	resumeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resume, resumeReq)
	if resume.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resume.Code, resume.Body.String())
	}
	if len(seen) != 5 || seen[4].Path != "/api/v1/edge-control/detection/resume" {
		t.Fatalf("unexpected resume forward: %+v", seen)
	}
	resumePayload, ok := seen[4].Body["payload"].(map[string]any)
	if !ok || resumePayload["task_id"].(float64) != 123 {
		t.Fatalf("unexpected resume payload: %+v", seen[4].Body["payload"])
	}

	note := httptest.NewRecorder()
	noteReq := authedRequest(http.MethodPost, "/api/v1/detection-runs/123/notes", token, strings.NewReader(`{"content":"memo"}`))
	noteReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(note, noteReq)
	if note.Code != http.StatusNotImplemented || !strings.Contains(note.Body.String(), `"code":"main_server_detection_note_write_unsupported"`) {
		t.Fatalf("note write status=%d body=%s", note.Code, note.Body.String())
	}
	if len(seen) != 5 {
		t.Fatalf("note writes should not call edge control directly, seen=%+v", seen)
	}
}

func TestDetectionRunUserRoutesRequireConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	ensureTestAdmin(t, db)
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_USER_CONTROL_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/api/v1/detection-runs", token, strings.NewReader(`{"project_id":1,"test_no":"T-1"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge backend should not be called when detection control service token is missing")
	}
	if !strings.Contains(rec.Body.String(), `"code":"edge_control_token_missing"`) || !strings.Contains(rec.Body.String(), `"service_token_ref":"EDGE_MISSING_USER_CONTROL_TOKEN"`) {
		t.Fatalf("unexpected missing token body=%s", rec.Body.String())
	}
}

func TestRawWriteProxyIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.QueryProxyEnabled = true
	router := NewRouter(cfg, db)

	for _, path := range []string{"/api/v1/variables", "/api/v1/edge-proxy/api/v1/variables"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"unsafe":true}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"edge_control_required"`) {
			t.Fatalf("%s should return edge_control_required: %s", path, rec.Body.String())
		}
	}
	if called {
		t.Fatal("raw write proxy should not call edge backend")
	}
}

func TestQueryProxyIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.QueryProxyEnabled = true
	router := NewRouter(cfg, db)

	for _, path := range []string{"/api/v1/not-ported-yet", "/api/v1/edge-proxy/api/v1/not-ported-yet"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"main_server_query_route_not_implemented"`) {
			t.Fatalf("%s should return main_server_query_route_not_implemented: %s", path, rec.Body.String())
		}
	}
	if called {
		t.Fatal("query proxy should not call edge backend")
	}

	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/v1/main-server/status", nil))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status route status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}
	if !strings.Contains(statusRec.Body.String(), `"query_proxy_enabled":false`) {
		t.Fatalf("status should report query proxy disabled: %s", statusRec.Body.String())
	}
}

func TestAuthLoginMeAndUsersReadSyncedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 3}).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(testConfig(), db)

	loginRec := httptest.NewRecorder()
	loginBody := `{"username":"admin","password":"Admin@12345"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginPayload struct {
		AccessToken string   `json:"access_token"`
		Permissions []string `json:"permissions"`
		User        struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}
	if loginPayload.AccessToken == "" || loginPayload.User.Username != "admin" || !containsString(loginPayload.Permissions, auth.PermManageUsers) {
		t.Fatalf("unexpected login payload: %+v", loginPayload)
	}

	meRec := httptest.NewRecorder()
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), `"username":"admin"`) {
		t.Fatalf("me status=%d body=%s", meRec.Code, meRec.Body.String())
	}

	usersRec := httptest.NewRecorder()
	usersReq := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	usersReq.Header.Set("Authorization", "Bearer "+loginPayload.AccessToken)
	router.ServeHTTP(usersRec, usersReq)
	if usersRec.Code != http.StatusOK || !strings.Contains(usersRec.Body.String(), `"permissions"`) {
		t.Fatalf("users status=%d body=%s", usersRec.Code, usersRec.Body.String())
	}

	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"bad"}`))
	badReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d body=%s", badRec.Code, badRec.Body.String())
	}
}

func TestMainServerDatabaseConfigIsLocalReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.Database = config.DatabaseConfig{
		Host:     "10.1.2.3",
		Port:     3307,
		User:     "sync_user",
		Password: "secret",
		Name:     "main_sync",
	}
	cfg.Edge.QueryProxyEnabled = true
	router := NewRouter(cfg, db)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/system/database-config", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("database config should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	token := loginForTest(t, router, "admin", "Admin@12345")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/database-config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("database config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["host"] != "10.1.2.3" || payload["name"] != "main_sync" || payload["source"] != "main_server_config" || payload["read_only"] != true {
		t.Fatalf("unexpected database config payload: %+v", payload)
	}
	if payload["password"] != nil || payload["password_set"] != true || payload["auto_migrate"] != false || payload["restart_required"] != false {
		t.Fatalf("database config should be sanitized and read-only: %+v", payload)
	}

	for _, item := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPatch, path: "/api/v1/system/database-config", body: `{"host":"127.0.0.1"}`},
		{method: http.MethodPost, path: "/api/v1/system/database-config/test", body: `{"host":"127.0.0.1"}`},
	} {
		writeRec := httptest.NewRecorder()
		writeReq := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		writeReq.Header.Set("Authorization", "Bearer "+token)
		writeReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(writeRec, writeReq)
		if writeRec.Code != http.StatusNotImplemented || !strings.Contains(writeRec.Body.String(), `"code":"main_server_database_config_read_only"`) {
			t.Fatalf("%s %s status=%d body=%s", item.method, item.path, writeRec.Code, writeRec.Body.String())
		}
	}
}

func TestMainServerRuntimeDiagnosticsForwardToEdgeServiceEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDGE_RUNTIME_TEST_TOKEN", "edge-runtime-secret")
	seen := make(map[string]string)
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"path":"` + r.URL.Path + `"}`))
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_RUNTIME_TEST_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	expected := map[string]string{
		"/api/v1/runtime/channels":        "/api/v1/edge-control/runtime/channels",
		"/api/v1/runtime/channels/detail": "/api/v1/edge-control/runtime/channels/detail",
		"/api/v1/runtime/notifications":   "/api/v1/edge-control/runtime/notifications",
		"/api/v1/runtime/workers":         "/api/v1/edge-control/runtime/workers",
		"/api/v1/task-flows/runtime":      "/api/v1/edge-control/task-flows/runtime",
	}
	for path, edgePath := range expected {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), edgePath) {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if seen[edgePath] != "Bearer edge-runtime-secret" {
			t.Fatalf("%s was not forwarded with service token, seen=%q", edgePath, seen[edgePath])
		}
	}
}

func TestMainServerRuntimeDiagnosticsRequireConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_RUNTIME_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/workers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"edge_runtime_token_missing"`) {
		t.Fatalf("missing runtime token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge backend should not be called when runtime service token is missing")
	}
}

func TestMainServerRealtimeWebSocketIsExplicitlyUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	proxyCalled := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.QueryProxyEnabled = true
	router := NewRouter(cfg, db)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/ws?topic=realtime.variables", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("main-server ws should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	token := loginForTest(t, router, "admin", "Admin@12345")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws?topic=realtime.variables", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), `"code":"main_server_realtime_ws_unsupported"`) {
		t.Fatalf("main-server ws should be explicit 501 status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"path":"/api/v1/ws"`) || !strings.Contains(rec.Body.String(), "service-token websocket bridge") {
		t.Fatalf("main-server ws diagnostic should include path and next action: %s", rec.Body.String())
	}
	if proxyCalled {
		t.Fatal("main-server ws route should not fall through to the edge query proxy")
	}
}

func TestMainServerTaskMetadataRoutesForwardToEdgeServiceEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDGE_METADATA_TEST_TOKEN", "edge-metadata-secret")
	seen := make(map[string]string)
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"path":"` + r.URL.Path + `"}]`))
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_METADATA_TEST_TOKEN"
	router := NewRouter(cfg, db)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/task-modules", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("task metadata should require login status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	token := loginForTest(t, router, "admin", "Admin@12345")
	expected := map[string]string{
		"/api/v1/task-modules":        "/api/v1/edge-control/task-modules",
		"/api/v1/task-flow-templates": "/api/v1/edge-control/task-flow-templates",
	}
	for path, edgePath := range expected {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), edgePath) {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if seen[edgePath] != "Bearer edge-metadata-secret" {
			t.Fatalf("%s was not forwarded with service token, seen=%q", edgePath, seen[edgePath])
		}
	}
}

func TestMainServerTaskMetadataRoutesRequireConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_METADATA_TOKEN"
	router := NewRouter(cfg, db)
	token := loginForTest(t, router, "admin", "Admin@12345")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-modules", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"edge_metadata_token_missing"`) {
		t.Fatalf("missing metadata token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge backend should not be called when metadata service token is missing")
	}
}

func TestAuthSSOTicketVerifyUsesEdgeThenIssuesMainToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	passwordHash, err := auth.HashPassword("unused")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 5}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDGE_SSO_TEST_TOKEN", "edge-service-secret")
	var seenAuth string
	var seenBody string
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/sso-ticket/verify" {
			t.Fatalf("unexpected sso path=%s", r.URL.Path)
		}
		seenAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		seenBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"edge_instance_id":"edge-a","user":{"id":1,"username":"admin","role":"admin","permissions_version":5}}`))
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_SSO_TEST_TOKEN"
	router := NewRouter(cfg, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso-ticket/verify", strings.NewReader(`{"ticket":"ticket-1","edge_id":"edge-a"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sso verify status=%d body=%s", rec.Code, rec.Body.String())
	}
	if seenAuth != "Bearer edge-service-secret" || !strings.Contains(seenBody, `"ticket":"ticket-1"`) {
		t.Fatalf("unexpected edge sso request auth=%q body=%s", seenAuth, seenBody)
	}
	if !strings.Contains(rec.Body.String(), `"access_token"`) || !strings.Contains(rec.Body.String(), `"username":"admin"`) {
		t.Fatalf("unexpected sso payload: %s", rec.Body.String())
	}
}

func TestAuthSSOTicketVerifyRequiresConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newServerTestDB(t)
	called := false
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(edge.Close)
	cfg := testConfig()
	cfg.Edge.BaseURL = edge.URL
	cfg.Edge.ServiceTokenRef = "EDGE_MISSING_SSO_TOKEN"
	router := NewRouter(cfg, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso-ticket/verify", strings.NewReader(`{"ticket":"ticket-1","edge_id":"edge-a"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing sso token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("edge sso endpoint should not be called when service token is missing")
	}
	if !strings.Contains(rec.Body.String(), `"code":"edge_control_token_missing"`) {
		t.Fatalf("unexpected missing token body=%s", rec.Body.String())
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func loginForTest(t *testing.T, router http.Handler, username string, password string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	body := `{"username":"` + username + `","password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccessToken == "" {
		t.Fatal("login response missing access_token")
	}
	return payload.AccessToken
}

func ensureTestAdmin(t *testing.T, db *gorm.DB) {
	t.Helper()
	var existing auth.SysUser
	if err := db.Where("username = ?", "admin").First(&existing).Error; err == nil {
		return
	}
	passwordHash, err := auth.HashPassword("Admin@12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&auth.SysUser{Username: "admin", PasswordHash: passwordHash, Role: auth.RoleAdmin, Enabled: true, PermissionsVersion: 1}).Error; err != nil {
		t.Fatal(err)
	}
}

func authedRequest(method string, target string, token string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func newServerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&query.Project{},
		&query.GatewayConfig{},
		&query.SysProjectMember{},
		&query.TagConfig{},
		&query.StationViewTemplate{},
		&query.StationViewRegion{},
		&query.StationViewItem{},
		&query.StationViewAssignment{},
		&query.DetectionTask{},
		&query.DetectionStandard{},
		&query.DetectionStandardItem{},
		&query.DetectionStandardFavorite{},
		&query.DetectionStandardRecent{},
		&query.StorageRoute{},
		&query.TaskFlow{},
		&query.TaskFlowVar{},
		&query.DetectionRunStandardItem{},
		&query.DetectionRunStorageRoute{},
		&query.DetectionRunNote{},
		&query.DetectionRunReport{},
		&query.DetectionRunReportRequest{},
		&query.ReportTemplate{},
		&query.DetectionRunEvent{},
		&query.DetectionRunSummary{},
		&query.DetectionRunFeature{},
		&query.HistoryData{},
		&query.DetectionLimitAlarm{},
		&query.TaskFlowRun{},
		&query.TaskFlowSQLLog{},
		&query.AuditLog{},
		&query.SysNotification{},
		&query.SysNotificationRecipient{},
		&auth.SysUser{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{HTTPAddr: "127.0.0.1:0"},
		Edge: config.EdgeConfig{
			BaseURL:           "http://127.0.0.1:18080",
			EdgeInstanceID:    "edge-a",
			ServiceTokenRef:   "EDGE_MAIN_SERVICE_TOKEN",
			QueryProxyEnabled: false,
		},
		Auth: config.AuthConfig{JWTSecret: "test-main-secret", AccessTokenTTLSeconds: 1800},
	}
}
