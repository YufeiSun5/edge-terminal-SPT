package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
)

func TestStationViewEffectiveFiltersCurrentProjectVariables(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	projectA := models.Project{ProjectCode: "P-A", Name: "Station A", DisplayName: "工位A", Enabled: true}
	projectB := models.Project{ProjectCode: "P-B", Name: "Station B", DisplayName: "工位B", Enabled: true}
	if err := db.Create(&projectA).Error; err != nil {
		t.Fatalf("create project A: %v", err)
	}
	if err := db.Create(&projectB).Error; err != nil {
		t.Fatalf("create project B: %v", err)
	}
	if err := db.Create(&[]models.TagConfig{
		stationViewTestTag(1001, projectA.ID, projectA.ProjectCode, "temp", "温度"),
		stationViewTestTag(1002, projectA.ID, projectA.ProjectCode, "pressure", "压力"),
		stationViewTestTag(2001, projectB.ID, projectB.ProjectCode, "temp", "温度B"),
	}).Error; err != nil {
		t.Fatalf("create tags: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-01")
	rec := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(projectA.ID), 10), nil, handler.effective)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response models.StationViewEffectiveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Project.ID != projectA.ID {
		t.Fatalf("expected project A, got %+v", response.Project)
	}
	if response.Template.TemplateCode != "STATION-DEFAULT" {
		t.Fatalf("expected auto-seeded default template, got %+v", response.Template)
	}
	if len(response.Regions) != 2 {
		t.Fatalf("expected default left/right regions, got %+v", response.Regions)
	}
	if got := response.WSSubscription.VarIDs; len(got) != 2 || got[0] != "1001" || got[1] != "1002" {
		t.Fatalf("expected only project A variables in ws subscription, got %#v", got)
	}
	for _, item := range response.Items {
		for _, binding := range item.ResolvedBindings {
			if binding.VarID == 2001 {
				t.Fatalf("station view leaked another project variable: %+v", binding)
			}
		}
	}
}

func TestStationViewEffectiveReturnsEmptyBindingsForEmptyProject(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-EMPTY", Name: "Empty Station", DisplayName: "空工位", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-01")
	rec := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response models.StationViewEffectiveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.WSSubscription.VarIDs; len(got) != 0 {
		t.Fatalf("expected empty ws subscription for empty project, got %#v", got)
	}
	for _, item := range response.Items {
		if len(item.ResolvedBindings) != 0 {
			t.Fatalf("expected empty bindings for empty project item=%+v", item)
		}
	}
	if len(response.Warnings) == 0 {
		t.Fatalf("expected warning for missing current run")
	}
}

func TestStationViewEffectiveUsesCurrentRunDetectionItems(t *testing.T) {
	testStationViewEffectiveUsesCurrentRunDetectionItems(t, models.DetectionStatusRunning)
}

func TestStationViewEffectiveUsesPausedRunDetectionItems(t *testing.T) {
	testStationViewEffectiveUsesCurrentRunDetectionItems(t, models.DetectionStatusPaused)
}

func testStationViewEffectiveUsesCurrentRunDetectionItems(t *testing.T, status string) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-A", Name: "Station A", DisplayName: "工位A", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	tag := stationViewTestTag(1001, project.ID, project.ProjectCode, "temp", "温度")
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	now := time.Now()
	task := models.DetectionTask{
		TestNo:      "RUN-001",
		ProjectID:   project.ID,
		ProjectCode: project.ProjectCode,
		Mode:        "manual",
		Status:      status,
		StartedAt:   &now,
	}
	if status == models.DetectionStatusPaused {
		task.PauseStartedAt = &now
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	limitH := 30.0
	if err := db.Create(&models.DetectionRunStandardItem{
		TaskID:         task.ID,
		TestNo:         task.TestNo,
		StandardID:     1,
		StandardItemID: 1,
		VarID:          1001,
		VarName:        "temp",
		DisplayName:    "检测温度",
		CheckEnabled:   true,
		AlarmEnabled:   true,
		StoreEnabled:   true,
		CheckMethod:    models.CheckMethodNumericRange,
		LimitH:         &limitH,
		QualityPolicy:  models.QualityPolicyIgnoreBad,
		Unit:           "C",
		DecimalPlaces:  1,
		SortOrder:      3,
	}).Error; err != nil {
		t.Fatalf("create run standard item: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-01")
	rec := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response models.StationViewEffectiveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.HTTPCompanion.CurrentRunRequired {
		t.Fatalf("expected current run companion requirement")
	}
	var detectionBinding *models.StationViewResolvedBinding
	for _, item := range response.Items {
		if item.BindingType != models.StationViewBindingDetectionItems {
			continue
		}
		if len(item.ResolvedBindings) > 0 {
			detectionBinding = &item.ResolvedBindings[0]
			break
		}
	}
	if detectionBinding == nil {
		t.Fatalf("expected detection item binding, got %+v", response.Items)
	}
	if detectionBinding.VarID != 1001 || detectionBinding.DisplayName != "检测温度" || detectionBinding.LimitH == nil || *detectionBinding.LimitH != limitH {
		t.Fatalf("unexpected detection binding: %+v", detectionBinding)
	}
}

func TestStationViewEffectiveRejectsForeignEdgeProject(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-FOREIGN", EdgeInstanceID: "edge-02", Name: "Foreign Station", DisplayName: "其他边缘工位", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-01")
	rec := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected foreign edge project to be hidden, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func stationViewTestTag(varID int64, projectID uint, projectCode string, varName string, displayName string) models.TagConfig {
	return models.TagConfig{
		VarID:         varID,
		GatewayID:     1,
		SourcePath:    "station/" + projectCode + "/" + varName,
		SourceType:    "mqtt",
		RawName:       varName,
		ProjectID:     &projectID,
		ProjectCode:   projectCode,
		VarGroup:      "environment",
		VarName:       varName,
		DisplayName:   displayName,
		JSONPath:      "$." + varName,
		DataType:      "FLOAT",
		Unit:          "C",
		DecimalPlaces: 1,
		RWMode:        "R",
		Discovered:    true,
		Enabled:       true,
	}
}
