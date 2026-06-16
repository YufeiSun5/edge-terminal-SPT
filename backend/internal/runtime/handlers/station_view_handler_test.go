package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	seedStationViewTestTemplate(t, db, "tpl-filter", projectA.ProjectCode, []models.StationViewItem{
		{ItemUID: "card-temp", LayoutArea: models.StationViewLayoutAreaCardPool, RegionKey: models.StationViewLayoutAreaCardPool, ItemType: "metric_card", BindingType: models.StationViewBindingVarName, BindingKey: "temp", SortOrder: 10, Visible: true},
		{ItemUID: "list-pressure", LayoutArea: models.StationViewLayoutAreaListLayout, RegionKey: models.StationViewLayoutAreaListLayout, ItemType: "inspection_row", BindingType: models.StationViewBindingVarName, BindingKey: "pressure", SortOrder: 20, Visible: true},
	})

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
	if response.Template.TemplateCode != "TPL-FILTER" {
		t.Fatalf("expected explicit station template, got %+v", response.Template)
	}
	if len(response.Regions) != 2 {
		t.Fatalf("expected default card/list layouts, got %+v", response.Regions)
	}
	if len(response.Items) == 0 || response.Items[0].LayoutArea != models.StationViewLayoutAreaCardPool || !response.Items[0].Visible {
		t.Fatalf("expected effective items to expose card-pool layout area and visible flag, got %+v", response.Items)
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
	seedStationViewTestTemplate(t, db, "tpl-empty", project.ProjectCode, []models.StationViewItem{
		{ItemUID: "card-all", LayoutArea: models.StationViewLayoutAreaCardPool, RegionKey: models.StationViewLayoutAreaCardPool, ItemType: "metric_card", BindingType: models.StationViewBindingVarGroup, SortOrder: 10, Visible: true},
	})

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
	if len(response.Warnings) != 0 {
		t.Fatalf("expected no warning when configured items do not require a current run, got %+v", response.Warnings)
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
	seedStationViewTestTemplate(t, db, "tpl-run-items", project.ProjectCode, []models.StationViewItem{
		{ItemUID: "list-run-items", LayoutArea: models.StationViewLayoutAreaListLayout, RegionKey: models.StationViewLayoutAreaListLayout, ItemType: "inspection_row", BindingType: models.StationViewBindingDetectionItems, SortOrder: 10, Visible: true},
	})

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

func TestStationViewEffectiveReturnsConflictForAmbiguousTemplate(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-CONFLICT", EdgeInstanceID: "edge-a", ModelName: "MODEL-CONFLICT", Name: "Conflict Station", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	templates := []models.StationViewTemplate{
		{TemplateUID: "tpl-edge-conflict", TemplateCode: "TPL-EDGE-CONFLICT", Name: "Edge template", Version: 1, Status: models.StationViewStatusPublished},
		{TemplateUID: "tpl-model-conflict", TemplateCode: "TPL-MODEL-CONFLICT", Name: "Model template", Version: 1, Status: models.StationViewStatusPublished},
	}
	assignments := []models.StationViewAssignment{
		{TemplateUID: "tpl-edge-conflict", TargetType: models.StationViewTargetEdge, TargetKey: "edge-a", Priority: 0, Enabled: true},
		{TemplateUID: "tpl-model-conflict", TargetType: models.StationViewTargetModel, TargetKey: "MODEL-CONFLICT", Priority: 100, Enabled: true},
	}
	if err := db.Create(&templates).Error; err != nil {
		t.Fatalf("create templates: %v", err)
	}
	if err := db.Create(&assignments).Error; err != nil {
		t.Fatalf("create assignments: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-a")
	rec := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body == "" || !strings.Contains(body, `"code":"station_view_template_conflict"`) {
		t.Fatalf("expected conflict code, body=%s", body)
	}
}

func TestStationViewTemplatesReloadAndAssignmentPatch(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-RELOAD", EdgeInstanceID: "edge-a", Name: "Reload Station", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	tag := stationViewTestTag(3001, project.ID, project.ProjectCode, "temp", "温度")
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	startedAt := time.Now().Add(-time.Minute)
	pausedAt := time.Now().Add(-30 * time.Second)
	task := models.DetectionTask{TestNo: "RUN-RELOAD", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "manual", Status: models.DetectionStatusRunning, StartedAt: &startedAt}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	pausedTask := models.DetectionTask{TestNo: "RUN-RELOAD-PAUSED", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "manual", Status: models.DetectionStatusPaused, StartedAt: &startedAt, PauseStartedAt: &pausedAt, PausedDurationMS: 1200}
	if err := db.Create(&pausedTask).Error; err != nil {
		t.Fatalf("create paused task: %v", err)
	}
	limitH := 35.0
	if err := db.Create(&models.DetectionRunStandardItem{
		TaskID:        task.ID,
		TestNo:        task.TestNo,
		VarID:         3001,
		VarName:       "temp",
		DisplayName:   "检测温度",
		CheckEnabled:  true,
		AlarmEnabled:  true,
		LimitH:        &limitH,
		DecimalPlaces: 1,
		SortOrder:     1,
	}).Error; err != nil {
		t.Fatalf("create run item: %v", err)
	}
	pausedLimitL := 12.0
	if err := db.Create(&models.DetectionRunStandardItem{
		TaskID:        pausedTask.ID,
		TestNo:        pausedTask.TestNo,
		VarID:         3001,
		VarName:       "temp",
		DisplayName:   "暂停检测温度",
		CheckEnabled:  true,
		AlarmEnabled:  true,
		LimitL:        &pausedLimitL,
		DecimalPlaces: 1,
		SortOrder:     1,
	}).Error; err != nil {
		t.Fatalf("create paused run item: %v", err)
	}
	template := models.StationViewTemplate{TemplateUID: "tpl-reload", TemplateCode: "TPL-RELOAD", Name: "Reload Template", Version: 2, Status: models.StationViewStatusPublished}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	assignment := models.StationViewAssignment{TemplateUID: template.TemplateUID, TargetType: models.StationViewTargetProject, TargetKey: project.ProjectCode, Enabled: true, Priority: 10}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	if err := db.Create(&models.StationViewRegion{TemplateUID: template.TemplateUID, RegionKey: models.StationViewLayoutAreaCardPool, LayoutArea: models.StationViewLayoutAreaCardPool, RegionType: "metric_grid", Enabled: true}).Error; err != nil {
		t.Fatalf("create region: %v", err)
	}
	if err := db.Create(&models.StationViewItem{TemplateUID: template.TemplateUID, RegionKey: models.StationViewLayoutAreaCardPool, LayoutArea: models.StationViewLayoutAreaCardPool, ItemUID: "reload-temp", ItemType: "metric_card", BindingType: models.StationViewBindingVarName, BindingKey: "temp", Visible: true}).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-a")
	list := callHandler(t, http.MethodGet, "/api/v1/station-view/templates", nil, handler.templates)
	if list.Code != http.StatusOK {
		t.Fatalf("expected templates 200, got %d body=%s", list.Code, list.Body.String())
	}
	if body := list.Body.String(); !strings.Contains(body, `"template_code":"TPL-RELOAD"`) || !strings.Contains(body, `"version":2`) || !strings.Contains(body, `"target_key":"P-RELOAD"`) {
		t.Fatalf("template list should include version and assignment, body=%s", body)
	}

	var tasksBefore int64
	if err := db.Model(&models.DetectionTask{}).Count(&tasksBefore).Error; err != nil {
		t.Fatalf("count tasks before reload: %v", err)
	}
	reload := callHandler(t, http.MethodPost, "/api/v1/station-view/reload", map[string]any{"project_id": project.ID}, handler.reload)
	if reload.Code != http.StatusOK {
		t.Fatalf("expected reload 200, got %d body=%s", reload.Code, reload.Body.String())
	}
	var reloadResponse models.StationViewReloadResponse
	if err := json.Unmarshal(reload.Body.Bytes(), &reloadResponse); err != nil {
		t.Fatalf("decode reload: %v", err)
	}
	if reloadResponse.ReloadMode != "validate_and_seed" || reloadResponse.Effective == nil || reloadResponse.Effective.Template.TemplateCode != "TPL-RELOAD" {
		t.Fatalf("unexpected reload response: %+v", reloadResponse)
	}
	var tasksAfter int64
	if err := db.Model(&models.DetectionTask{}).Count(&tasksAfter).Error; err != nil {
		t.Fatalf("count tasks after reload: %v", err)
	}
	if tasksAfter != tasksBefore {
		t.Fatalf("reload must not start or create detection tasks before=%d after=%d", tasksBefore, tasksAfter)
	}
	var runningAfter models.DetectionTask
	if err := db.First(&runningAfter, task.ID).Error; err != nil {
		t.Fatalf("read running task after reload: %v", err)
	}
	if runningAfter.Status != models.DetectionStatusRunning || runningAfter.EndedAt != nil || runningAfter.EndType != "" || runningAfter.StopReason != "" {
		t.Fatalf("reload must not stop or finish running task: %+v", runningAfter)
	}
	var pausedAfter models.DetectionTask
	if err := db.First(&pausedAfter, pausedTask.ID).Error; err != nil {
		t.Fatalf("read paused task after reload: %v", err)
	}
	if pausedAfter.Status != models.DetectionStatusPaused || pausedAfter.PauseStartedAt == nil || pausedAfter.PausedDurationMS != pausedTask.PausedDurationMS {
		t.Fatalf("reload must not resume or alter paused task: %+v", pausedAfter)
	}
	var tagAfter models.TagConfig
	if err := db.First(&tagAfter, "var_id = ?", tag.VarID).Error; err != nil {
		t.Fatalf("read tag after reload: %v", err)
	}
	if tagAfter.VarName != tag.VarName || tagAfter.DisplayName != tag.DisplayName || tagAfter.Enabled != tag.Enabled || tagAfter.ProjectCode != tag.ProjectCode {
		t.Fatalf("reload must not write variable configuration: before=%+v after=%+v", tag, tagAfter)
	}
	var unchanged models.DetectionRunStandardItem
	if err := db.First(&unchanged, "task_id = ? AND var_id = ?", task.ID, int64(3001)).Error; err != nil {
		t.Fatalf("reload changed running snapshot unexpectedly: %v", err)
	}
	if unchanged.LimitH == nil || *unchanged.LimitH != limitH {
		t.Fatalf("reload must not mutate running limit snapshot: %+v", unchanged)
	}
	var pausedUnchanged models.DetectionRunStandardItem
	if err := db.First(&pausedUnchanged, "task_id = ? AND var_id = ?", pausedTask.ID, int64(3001)).Error; err != nil {
		t.Fatalf("reload changed paused snapshot unexpectedly: %v", err)
	}
	if pausedUnchanged.LimitL == nil || *pausedUnchanged.LimitL != pausedLimitL {
		t.Fatalf("reload must not mutate paused limit snapshot: %+v", pausedUnchanged)
	}

	templatePatch := callHandlerWithParams(t, http.MethodPatch, "/api/v1/station-view/templates/1", map[string]any{"status": "disabled", "version": 4, "owner_scope": "main_server"}, gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(template.ID), 10)}}, handler.patchTemplate)
	if templatePatch.Code != http.StatusOK {
		t.Fatalf("expected template patch 200, got %d body=%s", templatePatch.Code, templatePatch.Body.String())
	}
	if body := templatePatch.Body.String(); !strings.Contains(body, `"status":"disabled"`) || !strings.Contains(body, `"version":4`) || !strings.Contains(body, `"owner_scope":"main_server"`) {
		t.Fatalf("unexpected template patch body=%s", body)
	}
	badTemplatePatch := callHandlerWithParams(t, http.MethodPatch, "/api/v1/station-view/templates/1", map[string]any{"status": "bad"}, gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(template.ID), 10)}}, handler.patchTemplate)
	if badTemplatePatch.Code != http.StatusBadRequest {
		t.Fatalf("expected bad template patch 400, got %d body=%s", badTemplatePatch.Code, badTemplatePatch.Body.String())
	}
	filteredList := callHandler(t, http.MethodGet, "/api/v1/station-view/templates?status=disabled&keyword=TPL-RELOAD", nil, handler.templates)
	if filteredList.Code != http.StatusOK || !strings.Contains(filteredList.Body.String(), `"template_code":"TPL-RELOAD"`) {
		t.Fatalf("expected filtered templates to include disabled template, status=%d body=%s", filteredList.Code, filteredList.Body.String())
	}

	// Re-publish before assignment patch so the project template can be selected again.
	templatePatch = callHandlerWithParams(t, http.MethodPatch, "/api/v1/station-view/templates/1", map[string]any{"status": models.StationViewStatusPublished}, gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(template.ID), 10)}}, handler.patchTemplate)
	if templatePatch.Code != http.StatusOK {
		t.Fatalf("expected template republish 200, got %d body=%s", templatePatch.Code, templatePatch.Body.String())
	}

	patch := callHandlerWithParams(t, http.MethodPatch, "/api/v1/station-view/assignments/1", map[string]any{"enabled": false}, gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(assignment.ID), 10)}}, handler.patchAssignment)
	if patch.Code != http.StatusOK {
		t.Fatalf("expected assignment patch 200, got %d body=%s", patch.Code, patch.Body.String())
	}
	badAssignmentPatch := callHandlerWithParams(t, http.MethodPatch, "/api/v1/station-view/assignments/1", map[string]any{"target_type": "bad"}, gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(assignment.ID), 10)}}, handler.patchAssignment)
	if badAssignmentPatch.Code != http.StatusBadRequest {
		t.Fatalf("expected bad assignment patch 400, got %d body=%s", badAssignmentPatch.Code, badAssignmentPatch.Body.String())
	}
	effective := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if effective.Code != http.StatusOK {
		t.Fatalf("expected fallback effective 200, got %d body=%s", effective.Code, effective.Body.String())
	}
	var effectiveResponse models.StationViewEffectiveResponse
	if err := json.Unmarshal(effective.Body.Bytes(), &effectiveResponse); err != nil {
		t.Fatalf("decode effective: %v", err)
	}
	if effectiveResponse.Template.TemplateCode == "TPL-RELOAD" {
		t.Fatalf("disabled assignment should not remain effective: %+v", effectiveResponse.Template)
	}
}

func TestStationViewItemsListAndReplace(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "P-ITEMS", EdgeInstanceID: "edge-a", Name: "Items Station", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&[]models.TagConfig{
		stationViewTestTag(4001, project.ID, project.ProjectCode, "temp", "温度"),
		stationViewTestTag(4002, project.ID, project.ProjectCode, "humidity", "湿度"),
	}).Error; err != nil {
		t.Fatalf("create tags: %v", err)
	}
	template := models.StationViewTemplate{TemplateUID: "tpl-items", TemplateCode: "TPL-ITEMS", Name: "Items Template", Version: 1, Status: models.StationViewStatusPublished}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	if err := db.Create(&models.StationViewAssignment{TemplateUID: template.TemplateUID, TargetType: models.StationViewTargetProject, TargetKey: project.ProjectCode, Enabled: true}).Error; err != nil {
		t.Fatalf("create assignment: %v", err)
	}

	handler := NewStationViewHandler(repo, "edge-a")
	replace := callHandler(t, http.MethodPut, "/api/v1/station-view/items", map[string]any{
		"template_uid": template.TemplateUID,
		"items": []map[string]any{
			{"item_uid": "card-temp", "layout_area": models.StationViewLayoutAreaCardPool, "item_type": "metric_card", "binding_type": models.StationViewBindingVarName, "binding_key": "temp", "sort_order": 10, "visible": true},
			{"item_uid": "list-humidity", "layout_area": models.StationViewLayoutAreaListLayout, "item_type": "inspection_row", "binding_type": models.StationViewBindingVarName, "binding_key": "humidity", "sort_order": 20, "pinned": true, "visible": true},
			{"item_uid": "hidden-temp", "layout_area": models.StationViewLayoutAreaCardPool, "item_type": "metric_card", "binding_type": models.StationViewBindingVarName, "binding_key": "temp", "sort_order": 30, "visible": false},
		},
	}, handler.replaceItems)
	if replace.Code != http.StatusOK {
		t.Fatalf("expected replace 200, got %d body=%s", replace.Code, replace.Body.String())
	}
	if body := replace.Body.String(); !strings.Contains(body, `"layout_area":"card_pool"`) || !strings.Contains(body, `"layout_area":"list_layout"`) || !strings.Contains(body, `"pinned":true`) || !strings.Contains(body, `"visible":false`) {
		t.Fatalf("replace should return layout areas and visible flag, body=%s", body)
	}

	listByTemplate := callHandler(t, http.MethodGet, "/api/v1/station-view/items?template_uid=tpl-items", nil, handler.items)
	if listByTemplate.Code != http.StatusOK || !strings.Contains(listByTemplate.Body.String(), `"item_uid":"hidden-temp"`) {
		t.Fatalf("template item list should include hidden config items, status=%d body=%s", listByTemplate.Code, listByTemplate.Body.String())
	}
	listByProject := callHandler(t, http.MethodGet, "/api/v1/station-view/items?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.items)
	if listByProject.Code != http.StatusOK || !strings.Contains(listByProject.Body.String(), `"template_uid":"tpl-items"`) {
		t.Fatalf("project item list should resolve current template, status=%d body=%s", listByProject.Code, listByProject.Body.String())
	}

	effective := callHandler(t, http.MethodGet, "/api/v1/station-view/effective?project_id="+strconv.FormatUint(uint64(project.ID), 10), nil, handler.effective)
	if effective.Code != http.StatusOK {
		t.Fatalf("expected effective 200, got %d body=%s", effective.Code, effective.Body.String())
	}
	var response models.StationViewEffectiveResponse
	if err := json.Unmarshal(effective.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode effective: %v", err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("effective should only include visible items, got %+v", response.Items)
	}
	if response.Items[0].LayoutArea != models.StationViewLayoutAreaCardPool || response.Items[1].LayoutArea != models.StationViewLayoutAreaListLayout {
		t.Fatalf("unexpected effective layout areas: %+v", response.Items)
	}
	if !response.Items[1].Pinned {
		t.Fatalf("expected pinned flag to round-trip through effective response: %+v", response.Items)
	}
	if got := response.WSSubscription.VarIDs; len(got) != 2 || got[0] != "4001" || got[1] != "4002" {
		t.Fatalf("unexpected effective ws ids: %+v", got)
	}

	badReplace := callHandler(t, http.MethodPut, "/api/v1/station-view/items", map[string]any{
		"template_uid": template.TemplateUID,
		"items": []map[string]any{
			{"item_uid": "bad", "layout_area": "left", "item_type": "metric_card", "binding_type": models.StationViewBindingVarName, "binding_key": "temp", "visible": true},
		},
	}, handler.replaceItems)
	if badReplace.Code != http.StatusBadRequest || !strings.Contains(badReplace.Body.String(), "invalid station view layout_area") {
		t.Fatalf("expected invalid layout_area 400, got %d body=%s", badReplace.Code, badReplace.Body.String())
	}
}

func TestStationViewPatchValidation(t *testing.T) {
	badVersion := 0
	if _, err := stationViewTemplateUpdates(stationViewTemplatePatchRequest{Version: &badVersion}); err == nil {
		t.Fatal("expected invalid template version")
	}
	blank := " "
	if _, err := stationViewTemplateUpdates(stationViewTemplatePatchRequest{OwnerScope: &blank}); err == nil {
		t.Fatal("expected invalid owner_scope")
	}
	status := models.StationViewStatusPublished
	version := 2
	ownerScope := "edge"
	templateUpdates, err := stationViewTemplateUpdates(stationViewTemplatePatchRequest{Status: &status, Version: &version, OwnerScope: &ownerScope})
	if err != nil || templateUpdates["status"] != status || templateUpdates["version"] != version || templateUpdates["owner_scope"] != ownerScope {
		t.Fatalf("unexpected template updates=%+v err=%v", templateUpdates, err)
	}

	if _, err := stationViewAssignmentUpdates(stationViewAssignmentPatchRequest{TemplateUID: &blank}); err == nil {
		t.Fatal("expected invalid template_uid")
	}
	if _, err := stationViewAssignmentUpdates(stationViewAssignmentPatchRequest{TargetKey: &blank}); err == nil {
		t.Fatal("expected invalid target_key")
	}
	priority := 9
	enabled := true
	templateUID := "tpl"
	targetType := models.StationViewTargetProject
	targetKey := "P-1"
	assignmentUpdates, err := stationViewAssignmentUpdates(stationViewAssignmentPatchRequest{
		TemplateUID: &templateUID,
		TargetType:  &targetType,
		TargetKey:   &targetKey,
		Priority:    &priority,
		Enabled:     &enabled,
	})
	if err != nil || assignmentUpdates["template_uid"] != templateUID || assignmentUpdates["target_type"] != targetType || assignmentUpdates["target_key"] != targetKey || assignmentUpdates["priority"] != priority || assignmentUpdates["enabled"] != enabled {
		t.Fatalf("unexpected assignment updates=%+v err=%v", assignmentUpdates, err)
	}
}

func seedStationViewTestTemplate(t *testing.T, db *gorm.DB, templateUID string, projectCode string, items []models.StationViewItem) {
	t.Helper()
	template := models.StationViewTemplate{
		TemplateUID:  templateUID,
		TemplateCode: strings.ToUpper(templateUID),
		Name:         templateUID,
		Version:      1,
		Status:       models.StationViewStatusPublished,
	}
	regions := []models.StationViewRegion{
		{TemplateUID: templateUID, RegionKey: models.StationViewLayoutAreaCardPool, LayoutArea: models.StationViewLayoutAreaCardPool, RegionType: "metric_grid", SortOrder: 10, Enabled: true},
		{TemplateUID: templateUID, RegionKey: models.StationViewLayoutAreaListLayout, LayoutArea: models.StationViewLayoutAreaListLayout, RegionType: "inspection_table", SortOrder: 20, Enabled: true},
	}
	assignment := models.StationViewAssignment{
		TemplateUID: templateUID,
		TargetType:  models.StationViewTargetProject,
		TargetKey:   projectCode,
		Priority:    10,
		Enabled:     true,
	}
	for index := range items {
		items[index].TemplateUID = templateUID
	}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("create station view template: %v", err)
	}
	if err := db.Create(&regions).Error; err != nil {
		t.Fatalf("create station view regions: %v", err)
	}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatalf("create station view assignment: %v", err)
	}
	if len(items) > 0 {
		if err := db.Create(&items).Error; err != nil {
			t.Fatalf("create station view items: %v", err)
		}
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
