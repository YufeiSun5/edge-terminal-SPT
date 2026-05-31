package runtime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

func TestCORSMiddleware(t *testing.T) {
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodOptions, "/ok", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent || resp.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("unexpected cors options: code=%d headers=%v", resp.Code, resp.Header())
	}
	if isAllowedDesktopOrigin("http://example.com") || !isAllowedDesktopOrigin("http://localhost:3000") || isAllowedDesktopOrigin("null") {
		t.Fatal("origin allow-list failed")
	}
}

func TestKernelAuthAndBusinessRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	cfg.Auth.EdgeInstanceID = "edge-001"
	cfg.Auth.MainSiteURL = "https://main.example.com/sso/edge"
	cfg.Auth.ServiceClients = []config.ServiceClientSeed{
		{ClientID: "main", Token: "main-token", Scopes: []string{"service_sso_verify"}, Enabled: true},
	}
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.seedAuth(); err != nil {
		t.Fatal(err)
	}

	if resp := performKernelRequest(kernel, http.MethodGet, "/health", "", nil); resp.Code != http.StatusOK {
		t.Fatalf("health status=%d", resp.Code)
	}
	if resp := performKernelRequest(kernel, http.MethodGet, "/api/v1/variables", "", nil); resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized variables, got %d", resp.Code)
	}

	token := loginKernel(t, kernel, "admin", "Admin@12345")
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/auth/me", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/users", token, nil), http.StatusOK)
	createUserResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/users", token, map[string]any{
		"username": "operator",
		"password": "Operator@12345",
		"role":     "guest",
		"enabled":  true,
	})
	assertStatus(t, createUserResp, http.StatusOK)
	var createdUser map[string]any
	mustDecodeKernel(t, createUserResp, &createdUser)
	createdUserID := uint64(createdUser["id"].(float64))
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/users/"+itoa(createdUserID), token, map[string]any{"role": "developer"}), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/users/"+itoa(createdUserID)+"/reset-password", token, map[string]any{"password": "Operator@67890"}), http.StatusOK)
	operatorToken := loginKernel(t, kernel, "operator", "Operator@67890")
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/users", operatorToken, nil), http.StatusForbidden)
	assertStatus(t, performKernelRequest(kernel, http.MethodDelete, "/api/v1/users/"+itoa(createdUserID), token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/runtime/channels", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/gateways", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/realtime/variables", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/auth/logout", token, nil), http.StatusOK)

	createGatewayBody := map[string]any{
		"id":          7,
		"name":        "gw",
		"broker":      "tcp://127.0.0.1:1883",
		"client_id":   "client",
		"topic":       "topic",
		"enabled":     false,
		"parser_type": "kingiot_kio",
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateway-configs", token, createGatewayBody), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/gateway-configs", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/gateway-configs/7", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/gateway-configs/7", token, map[string]any{"name": "gw2", "qos": 2}), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateway-configs/7/discover", token, map[string]any{}), http.StatusBadRequest)
	assertStatusIn(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateways/7/publish", token, map[string]any{"topic": "x", "payload": map[string]any{"a": 1}}), http.StatusOK, http.StatusBadGateway)
	assertStatusIn(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateways/7/subscribe", token, map[string]any{"topic": "x"}), http.StatusOK, http.StatusBadGateway)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateways/7/kio/write", token, map[string]any{"values": []map[string]any{{"name": "A", "value": 1}}}), http.StatusBadRequest)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/gateways/7/kio/query-all", token, map[string]any{}), http.StatusBadRequest)
	assertStatus(t, performKernelRequest(kernel, http.MethodDelete, "/api/v1/gateway-configs/7", token, nil), http.StatusOK)

	ProjectResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/projects", token, map[string]any{
		"project_code":    "AC-T",
		"site_no":         "PLC-T",
		"display_name":    "测试项目",
		"display_name_en": "Test Project",
		"display_name_ja": "テストプロジェクト",
	})
	assertStatus(t, ProjectResp, http.StatusOK)
	var Project models.Project
	mustDecodeKernel(t, ProjectResp, &Project)
	if Project.Name != "测试项目" || Project.DisplayName != "测试项目" || Project.DisplayNameEN != "Test Project" || Project.DisplayNameJA != "テストプロジェクト" {
		t.Fatalf("unexpected Project i18n fields: %+v", Project)
	}
	patchProjectResp := performKernelRequest(kernel, http.MethodPatch, "/api/v1/projects/"+itoa(uint64(Project.ID)), token, map[string]any{
		"display_name_en": "Updated Project",
		"display_name_ja": "更新済みプロジェクト",
	})
	assertStatus(t, patchProjectResp, http.StatusOK)
	mustDecodeKernel(t, patchProjectResp, &Project)
	if Project.DisplayNameEN != "Updated Project" || Project.DisplayNameJA != "更新済みプロジェクト" {
		t.Fatalf("unexpected patched Project i18n fields: %+v", Project)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/projects", token, nil), http.StatusOK)

	tag := models.TagConfig{
		VarID:       100,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1, Discovered: true,
		Enabled: true,
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/variables?keyword=temp&enabled=true", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/variables/100", token, map[string]any{"display_name": "Temperature", "decimal_places": 1}), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/variables/100/assignment", token, map[string]any{"project_id": Project.ID, "var_group": "group", "enabled": true}), http.StatusOK)

	for _, dataType := range []string{"INT", "FLOAT", "BOOL", "STRING"} {
		createVariableResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/variables", token, map[string]any{
			"source_type":  "virtual",
			"project_id":   Project.ID,
			"project_code": Project.ProjectCode,
			"var_group":    "virtual",
			"var_name":     "virtual_" + dataType,
			"display_name": "Virtual " + dataType,
			"data_type":    dataType,
		})
		assertStatus(t, createVariableResp, http.StatusOK)
		var createdVariable models.TagConfig
		mustDecodeKernel(t, createVariableResp, &createdVariable)
		if createdVariable.SourceType != models.TagSourceVirtual || createdVariable.Discovered || !createdVariable.Placeholder || createdVariable.GatewayID != 0 {
			t.Fatalf("unexpected virtual variable: %+v", createdVariable)
		}
	}
	realtimeResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/realtime/variables", token, nil)
	assertStatus(t, realtimeResp, http.StatusOK)
	var realtimeItems []models.TagSnapshot
	mustDecodeKernel(t, realtimeResp, &realtimeItems)
	if len(realtimeItems) < 4 {
		t.Fatalf("expected virtual variables in realtime snapshots, got %d", len(realtimeItems))
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/variables", token, map[string]any{
		"source_type": "manual",
		"var_name":    "bad_manual",
		"data_type":   "FLOAT",
	}), http.StatusBadRequest)

	reportTemplateResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/report-templates", token, map[string]any{
		"template_code": "RPT-T",
		"name":          "Excel Report",
		"display_name":  "检测报表",
		"file_ref":      "reports/templates/rpt-t.xlsx",
		"version":       2,
	})
	assertStatus(t, reportTemplateResp, http.StatusOK)
	var reportTemplate models.ReportTemplate
	mustDecodeKernel(t, reportTemplateResp, &reportTemplate)
	if reportTemplate.ID == 0 || reportTemplate.FileKind != "xlsx" {
		t.Fatalf("unexpected report template: %+v", reportTemplate)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/report-templates?keyword=RPT-T", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/report-templates/"+itoa(uint64(reportTemplate.ID)), token, map[string]any{"remark": "updated"}), http.StatusOK)

	standardResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-standards", token, map[string]any{
		"standard_code":      "STD-T",
		"display_name":       "标准T",
		"display_name_en":    "Standard T",
		"project_id":         Project.ID,
		"project_code":       Project.ProjectCode,
		"mode":               "standard",
		"report_template_id": reportTemplate.ID,
		"items": []map[string]any{{
			"var_id":        100,
			"var_name":      "temp",
			"display_name":  "温度",
			"check_enabled": true,
			"store_enabled": true,
			"required":      true,
			"limit_l":       18.0,
			"limit_h":       30.0,
			"unit":          "C",
		}},
	})
	assertStatus(t, standardResp, http.StatusOK)
	var standard models.DetectionStandard
	mustDecodeKernel(t, standardResp, &standard)
	if standard.ID == 0 || len(standard.Items) != 1 {
		t.Fatalf("unexpected standard response: %+v", standard)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-standards?keyword=STD-T", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-standards/"+itoa(uint64(standard.ID)), token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPatch, "/api/v1/detection-standards/"+itoa(uint64(standard.ID)), token, map[string]any{"remark": "updated"}), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPut, "/api/v1/detection-standards/"+itoa(uint64(standard.ID))+"/items", token, map[string]any{
		"items": []map[string]any{{
			"var_id":        100,
			"var_name":      "temp",
			"store_enabled": false,
		}},
	}), http.StatusOK)
	historyValue := 23.5
	if err := db.Create(&models.HistoryData{
		GatewayID:   1,
		ProjectID:   Project.ID,
		TaskID:      1,
		TestNo:      "T-1",
		VarID:       100,
		VarName:     "temp",
		ProjectCode: Project.ProjectCode,
		Value:       &historyValue,
		Quality:     1,
		SourceTime:  time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}
	historyResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/history/data?project_id="+itoa(uint64(Project.ID))+"&start=2026-05-29T00:00:00Z&end=2026-05-29T02:00:00Z", token, nil)
	assertStatus(t, historyResp, http.StatusOK)
	var historyPayload map[string]any
	mustDecodeKernel(t, historyResp, &historyPayload)
	if historyPayload["count"].(float64) != 1 {
		t.Fatalf("history count=%v body=%s", historyPayload["count"], historyResp.Body.String())
	}

	runResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs", token, map[string]any{
		"project_id":    Project.ID,
		"test_no":       "T-1",
		"mode":          "standard",
		"standard_id":   standard.ID,
		"duration_sec":  3600,
		"operator_note": "startup memo",
	})
	assertStatus(t, runResp, http.StatusOK)
	var run models.DetectionTask
	mustDecodeKernel(t, runResp, &run)
	if run.StandardID == nil || *run.StandardID != standard.ID || len(run.StandardItems) != 1 {
		t.Fatalf("expected detection standard snapshot: %+v", run)
	}
	if run.OperatorNote != "startup memo" || run.DurationSec != 3600 || run.ExpectedEndAt == nil || run.ReportTemplateID == nil || *run.ReportTemplateID != reportTemplate.ID || run.ReportTemplateVersion != 2 {
		t.Fatalf("expected task metadata/template snapshot: %+v", run)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs", token, map[string]any{"project_id": Project.ID, "test_no": "T-duplicate", "mode": "standard"}), http.StatusConflict)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-runs/active", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-runs?project_id="+itoa(uint64(Project.ID))+"&status=running", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs/"+itoa(uint64(run.ID))+"/notes", token, map[string]any{"content": "memo 1"}), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-runs/"+itoa(uint64(run.ID))+"/notes", token, nil), http.StatusOK)
	detailResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-runs/"+itoa(uint64(run.ID)), token, nil)
	assertStatus(t, detailResp, http.StatusOK)
	var detail models.DetectionTask
	mustDecodeKernel(t, detailResp, &detail)
	if len(detail.RecentNotes) != 1 {
		t.Fatalf("expected recent notes on task detail: %+v", detail)
	}
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs/"+itoa(uint64(run.ID))+"/stop", token, map[string]any{"reason": "done"}), http.StatusOK)
	runResp = performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs", token, map[string]any{"project_id": Project.ID, "test_no": "T-2", "mode": "standard", "standard_id": standard.ID, "report_template_id": reportTemplate.ID})
	assertStatus(t, runResp, http.StatusOK)
	mustDecodeKernel(t, runResp, &run)
	assertStatus(t, performKernelRequest(kernel, http.MethodPost, "/api/v1/detection-runs/"+itoa(uint64(run.ID))+"/abnormal-stop", token, map[string]any{"reason": "alarm"}), http.StatusOK)
	historyByTaskResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/history/data?task_id=1", token, nil)
	assertStatus(t, historyByTaskResp, http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodDelete, "/api/v1/variables/100", token, nil), http.StatusOK)
	assertStatus(t, performKernelRequest(kernel, http.MethodDelete, "/api/v1/detection-standards/"+itoa(uint64(standard.ID)), token, nil), http.StatusConflict)
	assertStatus(t, performKernelRequest(kernel, http.MethodDelete, "/api/v1/report-templates/"+itoa(uint64(reportTemplate.ID)), token, nil), http.StatusConflict)

	ticketResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/auth/sso-ticket", token, nil)
	assertStatus(t, ticketResp, http.StatusOK)
	var ticketPayload map[string]any
	mustDecodeKernel(t, ticketResp, &ticketPayload)
	verifyResp := performKernelRequest(kernel, http.MethodPost, "/api/v1/auth/sso-ticket/verify", "main-token", map[string]any{
		"ticket":  ticketPayload["ticket"],
		"edge_id": "edge-001",
	})
	assertStatus(t, verifyResp, http.StatusOK)
}

func TestKernelRouteValidationFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.seedAuth(); err != nil {
		t.Fatal(err)
	}
	token := loginKernel(t, kernel, "admin", "Admin@12345")

	cases := []struct {
		method string
		path   string
		body   any
		status int
	}{
		{http.MethodGet, "/api/v1/gateway-configs/bad", nil, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/users", map[string]any{"username": "bad"}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/users", map[string]any{"username": "bad", "password": "Bad@12345", "role": "bad-role"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/users/bad", map[string]any{}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/users/1", map[string]any{"enabled": false}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/users/bad/reset-password", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/users/1/reset-password", map[string]any{}, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/users/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/users/1", nil, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateway-configs", map[string]any{"name": "missing"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/gateway-configs/bad", map[string]any{}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/gateway-configs/404", map[string]any{"name": "x"}, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/gateway-configs/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/gateway-configs/404", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/gateway-configs/bad/discover", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateway-configs/404/discover", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/bad/publish", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/1/publish", map[string]any{"topic": "x"}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/bad/subscribe", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/1/subscribe", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/bad/kio/write", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/1/kio/write", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/gateways/bad/kio/query-all", map[string]any{}, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/variables?gateway_id=bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/history/data?project_id=bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/history/data?start=bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/history/data?limit=0", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/detection-standards?project_id=bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/detection-standards?enabled=bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/detection-standards/bad", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/detection-standards/404", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/detection-standards/404/favorite", nil, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/detection-standards/404/favorite", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/detection-standards", map[string]any{"standard_code": "STD"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/detection-standards/bad", map[string]any{"remark": "x"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/detection-standards/404", map[string]any{"remark": "x"}, http.StatusNotFound},
		{http.MethodPut, "/api/v1/detection-standards/bad/items", map[string]any{"items": []any{}}, http.StatusBadRequest},
		{http.MethodPut, "/api/v1/detection-standards/404/items", map[string]any{"items": []any{}}, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/detection-standards/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/detection-standards/404", nil, http.StatusNotFound},
		{http.MethodGet, "/api/v1/report-templates?enabled=bad", nil, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/report-templates/bad", map[string]any{"remark": "x"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/report-templates/404", map[string]any{"remark": "x"}, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/report-templates/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/report-templates/404", nil, http.StatusNotFound},
		{http.MethodGet, "/api/v1/storage-routes?project_id=bad", nil, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/storage-routes/bad", map[string]any{"enabled": true}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/storage-routes/404", map[string]any{"enabled": true}, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/storage-routes/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/storage-routes/404", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects", map[string]any{"project_code": "AC"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/projects/bad", map[string]any{"display_name": "x"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/projects/404", map[string]any{"display_name": "x"}, http.StatusNotFound},
		{http.MethodPatch, "/api/v1/variables/bad", map[string]any{}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/variables/404", map[string]any{"display_name": "x"}, http.StatusNotFound},
		{http.MethodPatch, "/api/v1/variables/bad/assignment", map[string]any{}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/variables/404/assignment", map[string]any{"project_id": 1, "enabled": true}, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/variables/bad", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/variables/404", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/detection-runs", map[string]any{"project_id": 1}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/detection-runs/bad/stop", map[string]any{}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/detection-runs/404/stop", map[string]any{}, http.StatusNotFound},
	}
	for _, tc := range cases {
		resp := performKernelRequest(kernel, tc.method, tc.path, token, tc.body)
		if resp.Code != tc.status {
			t.Fatalf("%s %s status got=%d want=%d body=%s", tc.method, tc.path, resp.Code, tc.status, resp.Body.String())
		}
	}
}

func TestKernelRealtimeWebSocketReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.seedAuth(); err != nil {
		t.Fatal(err)
	}
	ProjectID := uint(9)
	kernel.tags.Load([]models.TagConfig{{
		VarID:       100,
		GatewayID:   1,
		SourceType:  models.TagSourceMQTT,
		SourcePath:  "temp",
		RawName:     "temp",
		ProjectID:   &ProjectID,
		ProjectCode: "AC-WS",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}})
	kernel.tasks.SetActive(models.DetectionTask{ID: 7, TestNo: "WS-T-1", ProjectID: ProjectID, ProjectCode: "AC-WS", Mode: "standard"})
	token := loginKernel(t, kernel, "admin", "Admin@12345")

	server := httptest.NewServer(kernel.Router())
	defer server.Close()
	unauthorizedURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws?topic=realtime.variables"
	if conn, resp, err := websocket.DefaultDialer.Dial(unauthorizedURL, nil); err == nil {
		_ = conn.Close()
		t.Fatal("expected websocket auth failure without token")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected websocket 401 without token, resp=%v err=%v", resp, err)
	}
	conn := dialRuntimeWebSocket(t, server.URL, token)
	defer func() {
		_ = conn.Close()
	}()

	assertRuntimeWebSocketSnapshots(t, conn)
	if err := conn.WriteJSON(map[string]any{"type": "command.write_variable", "request_id": "req-ws", "command_id": "cmd-ws"}); err != nil {
		t.Fatal(err)
	}
	errorMessage := readWSMessageOfType(t, conn, services.WSTypeError)
	errorPayload, _ := errorMessage["error"].(map[string]any)
	if errorPayload["code"] != "invalid_payload" {
		t.Fatalf("expected invalid_payload error, got %+v", errorMessage)
	}
	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "reconnect")); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	reconnected := dialRuntimeWebSocket(t, server.URL, token)
	defer func() {
		_ = reconnected.Close()
	}()
	assertRuntimeWebSocketSnapshots(t, reconnected)
}

func TestKernelRealtimeWebSocketDetectionCommandsAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.seedAuth(); err != nil {
		t.Fatal(err)
	}
	Project := models.Project{ProjectCode: "AC-CMD", Name: "Command Project", DisplayName: "Command Project", Enabled: true}
	if err := db.Create(&Project).Error; err != nil {
		t.Fatal(err)
	}
	token := loginKernel(t, kernel, "admin", "Admin@12345")
	server := httptest.NewServer(kernel.Router())
	defer server.Close()
	conn := dialRuntimeWebSocket(t, server.URL, token)
	defer func() {
		_ = conn.Close()
	}()
	readWSMessageOfType(t, conn, services.WSTypeConnectionReady)

	if err := conn.WriteJSON(map[string]any{
		"type":       "command.detection.start",
		"request_id": "req-start",
		"command_id": "cmd-start",
		"payload": map[string]any{
			"project_id": Project.ID,
			"test_no":    "WS-CMD-1",
			"mode":       "standard",
		},
	}); err != nil {
		t.Fatal(err)
	}
	startAck := readWSMessageOfType(t, conn, services.WSTypeCommandAck)
	if startAck["command_id"] != "cmd-start" {
		t.Fatalf("unexpected start ack: %+v", startAck)
	}
	var task models.DetectionTask
	if err := db.First(&task, "test_no = ?", "WS-CMD-1").Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != models.DetectionStatusRunning {
		t.Fatalf("expected running task, got %+v", task)
	}

	if err := conn.WriteJSON(map[string]any{
		"type":       "command.detection.stop",
		"request_id": "req-stop",
		"command_id": "cmd-stop",
		"payload": map[string]any{
			"task_id": task.ID,
			"reason":  "ws stop",
		},
	}); err != nil {
		t.Fatal(err)
	}
	stopAck := readWSMessageOfType(t, conn, services.WSTypeCommandAck)
	if stopAck["command_id"] != "cmd-stop" {
		t.Fatalf("unexpected stop ack: %+v", stopAck)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND result = ?", "ws.command.detection.start", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected ws start audit, got %d", auditCount)
	}
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND result = ?", "ws.command.detection.stop", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected ws stop audit, got %d", auditCount)
	}

	if err := conn.WriteJSON(map[string]any{
		"type":       "command.detection.stop",
		"request_id": "req-missing-command",
		"payload":    map[string]any{"task_id": task.ID},
	}); err != nil {
		t.Fatal(err)
	}
	errMsg := readWSMessageOfType(t, conn, services.WSTypeError)
	errPayload, _ := errMsg["error"].(map[string]any)
	if errPayload["code"] != "command_id_required" {
		t.Fatalf("expected command_id_required, got %+v", errMsg)
	}
}

func dialRuntimeWebSocket(t *testing.T, serverURL string, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws?access_token=" + url.QueryEscape(token) + "&topic=realtime.variables&topic=detection.runs&var_id=100"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func assertRuntimeWebSocketSnapshots(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	readWSMessageOfType(t, conn, services.WSTypeConnectionReady)
	variables := readWSMessageOfType(t, conn, services.WSTypeRealtimeVariablesSnapshot)
	variablePayload, _ := variables["payload"].(map[string]any)
	variableItems, _ := variablePayload["items"].([]any)
	if len(variableItems) != 1 {
		t.Fatalf("expected filtered variable snapshot, got %+v", variables)
	}
	runs := readWSMessageOfType(t, conn, services.WSTypeDetectionRunsSnapshot)
	runsPayload, _ := runs["payload"].(map[string]any)
	runItems, _ := runsPayload["items"].([]any)
	if len(runItems) != 1 {
		t.Fatalf("expected active run snapshot, got %+v", runs)
	}
}

func TestKernelWriteAuditMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.seedAuth(); err != nil {
		t.Fatal(err)
	}
	token := loginKernel(t, kernel, "admin", "Admin@12345")

	resp := performKernelRequestWithHeaders(kernel, http.MethodPost, "/api/v1/projects", token, map[string]any{
		"project_code": "AUDIT-AC",
		"site_no":      "AUDIT-PLC",
		"display_name": "审计项目",
	}, map[string]string{
		"X-Request-ID": "req-audit-1",
		"X-Command-ID": "cmd-audit-1",
	})
	assertStatus(t, resp, http.StatusOK)
	if resp.Header().Get("X-Request-ID") != "req-audit-1" {
		t.Fatalf("missing response request id: %s", resp.Header().Get("X-Request-ID"))
	}

	var entry models.SysAuditLog
	if err := db.Where("action = ? AND target_id = ?", "http.post", "/api/v1/projects").Last(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.ActorType != "user" || entry.ActorID == "" || entry.Result != "success" || entry.TargetType != "http_endpoint" {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(entry.Detail), &detail); err != nil {
		t.Fatalf("invalid audit detail: %v detail=%s", err, entry.Detail)
	}
	if detail["request_id"] != "req-audit-1" || detail["command_id"] != "cmd-audit-1" || detail["route"] != "/api/v1/projects" || detail["status"].(float64) != http.StatusOK {
		t.Fatalf("unexpected audit detail: %+v", detail)
	}
	if _, ok := detail["password"]; ok {
		t.Fatalf("audit detail must not include sensitive request body: %+v", detail)
	}

	resp = performKernelRequest(kernel, http.MethodPost, "/api/v1/users", token, map[string]any{"username": "bad"})
	assertStatus(t, resp, http.StatusBadRequest)
	entry = models.SysAuditLog{}
	if err := db.Where("action = ? AND target_id = ? AND result = ?", "http.post", "/api/v1/users", "failed").Last(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.Detail == "" {
		t.Fatal("expected failed write audit detail")
	}

	listResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/audit-logs?limit=10&action=http.post&result=success", token, nil)
	assertStatus(t, listResp, http.StatusOK)
	var listPayload struct {
		Items  []models.SysAuditLog `json:"items"`
		Total  int64                `json:"total"`
		Limit  int                  `json:"limit"`
		Offset int                  `json:"offset"`
	}
	mustDecodeKernel(t, listResp, &listPayload)
	if listPayload.Total == 0 || len(listPayload.Items) == 0 || listPayload.Limit != 10 {
		t.Fatalf("unexpected audit log list: %+v body=%s", listPayload, listResp.Body.String())
	}
	if listPayload.Items[0].Detail == "" || strings.Contains(listPayload.Items[0].Detail, "Admin@12345") {
		t.Fatalf("audit list returned invalid detail: %+v", listPayload.Items[0])
	}

	badLimitResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/audit-logs?limit=0", token, nil)
	assertStatus(t, badLimitResp, http.StatusBadRequest)
	badTimeResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/audit-logs?from=bad-time", token, nil)
	assertStatus(t, badTimeResp, http.StatusBadRequest)
}

func TestKernelStartSeedsAndLoadsRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.App.LogicWorkers = 1
	cfg.App.StoreWorkers = 1
	cfg.App.HistoryBatch = 1
	cfg.Gateways = []config.GatewaySeed{{
		ID:         1,
		Name:       "disabled",
		Broker:     "tcp://127.0.0.1:1883",
		ClientID:   "client",
		Topic:      "topic",
		ParserType: "kingiot_kio",
		Enabled:    false,
	}}
	db := newRuntimeTestDB(t)
	kernel := NewKernel(cfg, db)
	if err := kernel.Start(); err != nil {
		t.Fatal(err)
	}
	kernel.Stop()
}

func TestKernelStartRecoversRunningDetectionTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Auth.JWTSecret = "test-secret"
	cfg.App.LogicWorkers = 1
	cfg.App.StoreWorkers = 1
	cfg.App.HistoryBatch = 1
	cfg.Gateways = nil
	db := newRuntimeTestDB(t)
	repo := database.NewRepository(db)
	project := &models.Project{ProjectCode: "AC-REC", Name: "Recovered Project", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	tag := models.TagConfig{
		VarID:       7001,
		GatewayID:   1,
		SourceType:  models.TagSourceMQTT,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	route, err := repo.EnsureDefaultStorageRouteForTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateStorageRoute(route.ID, map[string]interface{}{
		"enabled":        true,
		"trigger_mode":   models.StoreTriggerOnDetection,
		"store_on_start": true,
	}); err != nil {
		t.Fatal(err)
	}
	limitH := 30.0
	standard := &models.DetectionStandard{StandardCode: "STD-REC", Name: "Recovered Standard", ProjectID: &project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:        tag.VarID,
		VarName:      tag.VarName,
		CheckEnabled: true,
		AlarmEnabled: true,
		StoreEnabled: true,
		CheckOnStart: true,
		LimitH:       &limitH,
	}}); err != nil {
		t.Fatal(err)
	}
	task, err := repo.StartDetectionTaskWithOptions(database.StartDetectionOptions{
		ProjectID:  project.ID,
		TestNo:     "RECOVER-RUNNING",
		Mode:       "standard",
		StandardID: &standard.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	kernel := NewKernel(cfg, db)
	if err := kernel.Start(); err != nil {
		t.Fatal(err)
	}
	defer kernel.Stop()

	active, ok := kernel.tasks.ActiveForProject(project.ID)
	if !ok {
		t.Fatal("expected running detection task to be restored into runtime TaskManager")
	}
	if active.ID != task.ID || active.TestNo != task.TestNo || active.ProjectID != project.ID {
		t.Fatalf("unexpected recovered active task: %+v want_task=%+v", active, task)
	}
	if !active.AllowsStore(tag.VarID) {
		t.Fatalf("expected recovered standard item to allow store for var_id=%d: %+v", tag.VarID, active)
	}
	routes := active.RoutesForStore(tag.VarID)
	if len(routes) != 1 || routes[0].TaskID != task.ID || routes[0].RouteID != route.ID {
		t.Fatalf("expected recovered storage route snapshot, got %+v", routes)
	}

	token := loginKernel(t, kernel, "admin", "Admin@12345")
	activeResp := performKernelRequest(kernel, http.MethodGet, "/api/v1/detection-runs/active", token, nil)
	assertStatus(t, activeResp, http.StatusOK)
	var activePayload []models.ActiveTask
	mustDecodeKernel(t, activeResp, &activePayload)
	if len(activePayload) != 1 || activePayload[0].ID != task.ID {
		t.Fatalf("expected active API to expose recovered task, got %+v", activePayload)
	}
}

func newRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func loginKernel(t *testing.T, kernel *Kernel, username string, password string) string {
	t.Helper()
	resp := performKernelRequest(kernel, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": username, "password": password})
	assertStatus(t, resp, http.StatusOK)
	var payload map[string]any
	mustDecodeKernel(t, resp, &payload)
	token, _ := payload["access_token"].(string)
	if token == "" {
		t.Fatalf("missing access token: %s", resp.Body.String())
	}
	return token
}

func performKernelRequest(kernel *Kernel, method string, path string, bearer string, body any) *httptest.ResponseRecorder {
	return performKernelRequestWithHeaders(kernel, method, path, bearer, body, nil)
}

func performKernelRequestWithHeaders(kernel *Kernel, method string, path string, bearer string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp := httptest.NewRecorder()
	kernel.Router().ServeHTTP(resp, req)
	return resp
}

func assertStatus(t *testing.T, resp *httptest.ResponseRecorder, want int) {
	t.Helper()
	if resp.Code != want {
		t.Fatalf("status got=%d want=%d body=%s", resp.Code, want, resp.Body.String())
	}
}

func assertStatusIn(t *testing.T, resp *httptest.ResponseRecorder, wants ...int) {
	t.Helper()
	for _, want := range wants {
		if resp.Code == want {
			return
		}
	}
	t.Fatalf("status got=%d want one of=%v body=%s", resp.Code, wants, resp.Body.String())
}

func mustDecodeKernel(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, resp.Body.String())
	}
}

func readWSMessageOfType(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg["type"] == want {
			return msg
		}
	}
	t.Fatalf("websocket message type %s not received", want)
	return nil
}

func itoa(value uint64) string {
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
