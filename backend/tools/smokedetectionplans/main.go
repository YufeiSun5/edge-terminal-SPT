package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

type loginResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

type listResponse struct {
	Items []models.DetectionPlan `json:"items"`
	Total int64                  `json:"total"`
}

type startResponse struct {
	Plan models.DetectionPlan `json:"plan"`
	Task models.DetectionTask `json:"task"`
}

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:18082", "edge backend base URL")
	configPath := flag.String("config", envDefault("EDGE_CONFIG", "configs/config.json"), "edge backend config path")
	user := flag.String("user", "admin", "login username")
	pass := flag.String("pass", "Admin@12345", "login password")
	flag.Parse()

	if err := run(*baseURL, *configPath, *user, *pass); err != nil {
		fmt.Fprintf(os.Stderr, "smokedetectionplans failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("smokedetectionplans passed")
}

func run(baseURL, configPath, user, pass string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	stamp := time.Now().Format("20060102150405")
	if err := runMissingConfigSmoke(db, baseURL, tokenLogin{user: user, pass: pass}, stamp); err != nil {
		return err
	}
	if err := runSuccessfulStartSmoke(db, cfg, baseURL, tokenLogin{user: user, pass: pass}, stamp); err != nil {
		return err
	}
	return nil
}

type tokenLogin struct {
	user string
	pass string
}

func runMissingConfigSmoke(db *gorm.DB, baseURL string, loginInput tokenLogin, stamp string) error {
	var project models.Project
	if err := db.Order("id ASC").First(&project).Error; err != nil {
		return fmt.Errorf("load first project: %w", err)
	}
	planNo := "SMOKE-PLAN-MISSING-" + stamp
	sourceSystem := "codex-smoke"
	externalPlanID := "missing-" + stamp

	cleanup := func() {
		_ = db.Where("plan_no = ? OR (source_system = ? AND external_plan_id = ?)", planNo, sourceSystem, externalPlanID).Delete(&models.DetectionPlan{}).Error
	}
	cleanup()
	defer cleanup()

	plan := models.DetectionPlan{
		PlanNo:          planNo,
		SourceSystem:    sourceSystem,
		ExternalPlanID:  externalPlanID,
		ExternalOrderNo: "SMOKE-ORDER-" + stamp,
		FactoryNo:       "SMOKE-FACTORY-" + stamp,
		CustomerName:    "Codex Smoke",
		DeviceModel:     "Smoke Model",
		TestItemCode:    "SMOKE",
		TestItemName:    "Smoke Missing Config",
		TestSequence:    1,
		Mode:            "standard",
		StandardCode:    "SMOKE_MISSING_STANDARD_" + stamp,
		Status:          models.DetectionPlanStatusPending,
		SyncScope:       "global",
		UpdatedByNode:   "codex-smoke",
		UpdatedByUser:   "codex-smoke",
	}
	if err := db.Create(&plan).Error; err != nil {
		return fmt.Errorf("insert smoke plan: %w", err)
	}

	token, err := login(baseURL, loginInput.user, loginInput.pass)
	if err != nil {
		return err
	}
	list, err := getPlans(baseURL, token, plan.FactoryNo)
	if err != nil {
		return err
	}
	if len(list.Items) != 1 || list.Items[0].PlanNo != planNo || list.Items[0].Status != models.DetectionPlanStatusPending {
		return fmt.Errorf("unexpected plan list result: total=%d items=%d", list.Total, len(list.Items))
	}

	status, body, err := postJSON(baseURL+"/api/v1/detection-plans/"+fmt.Sprint(plan.ID)+"/start", token, map[string]any{
		"project_id":    project.ID,
		"operator_note": "codex smoke missing config",
	})
	if err != nil {
		return err
	}
	if status < 400 {
		return fmt.Errorf("start should fail for missing standard, status=%d body=%s", status, body)
	}

	var after models.DetectionPlan
	if err := db.First(&after, plan.ID).Error; err != nil {
		return fmt.Errorf("reload smoke plan: %w", err)
	}
	if after.Status != models.DetectionPlanStatusPending {
		return fmt.Errorf("plan should return to pending, got %s", after.Status)
	}
	if !strings.Contains(after.ErrorMessage, "config_not_ready") {
		return fmt.Errorf("plan error_message should mention config_not_ready, got %q", after.ErrorMessage)
	}
	if after.StartedTaskID != nil {
		return fmt.Errorf("missing-config smoke should not create started_task_id")
	}
	return nil
}

func runSuccessfulStartSmoke(db *gorm.DB, cfg *config.Config, baseURL string, loginInput tokenLogin, stamp string) error {
	sourceSystem := "codex-smoke"
	projectCode := "UNIT-PLAN-" + stamp
	standardCode := "UNIT_PLAN_STD_" + stamp
	planNo := "SMOKE-PLAN-START-" + stamp
	externalPlanID := "start-" + stamp

	cleanup := func() {
		var tasks []models.DetectionTask
		_ = db.Where("test_no = ?", planNo).Find(&tasks).Error
		for _, task := range tasks {
			_ = cleanupTask(db, task.ID)
		}
		_ = db.Where("plan_no = ? OR (source_system = ? AND external_plan_id = ?)", planNo, sourceSystem, externalPlanID).Delete(&models.DetectionPlan{}).Error
		var standard models.DetectionStandard
		if err := db.First(&standard, "standard_code = ?", standardCode).Error; err == nil {
			_ = db.Where("standard_id = ?", standard.ID).Delete(&models.DetectionStandardRecent{}).Error
			_ = db.Where("standard_id = ?", standard.ID).Delete(&models.DetectionStandardFavorite{}).Error
			_ = db.Where("standard_id = ?", standard.ID).Delete(&models.DetectionStandardItem{}).Error
			_ = db.Delete(&models.DetectionStandard{}, standard.ID).Error
		}
		var project models.Project
		if err := db.First(&project, "project_code = ?", projectCode).Error; err == nil {
			_ = db.Model(&models.Project{}).Where("id = ?", project.ID).Update("current_task_id", nil).Error
			_ = db.Delete(&models.Project{}, project.ID).Error
		}
	}
	cleanup()
	defer cleanup()

	project := models.Project{
		ProjectCode:    projectCode,
		SiteNo:         "SMOKE",
		EdgeInstanceID: strings.TrimSpace(cfg.Auth.EdgeInstanceID),
		Name:           "Codex Smoke Project",
		DisplayName:    "Codex Smoke Project",
		Enabled:        true,
		Blocked:        false,
		Placeholder:    true,
	}
	if err := db.Create(&project).Error; err != nil {
		return fmt.Errorf("create success smoke project: %w", err)
	}

	standard := models.DetectionStandard{
		StandardCode:   standardCode,
		Name:           "Codex Smoke Standard",
		DisplayName:    "Codex Smoke Standard",
		ProjectID:      &project.ID,
		ProjectCode:    project.ProjectCode,
		Mode:           "standard",
		Version:        1,
		Enabled:        true,
		SyncScope:      "global",
		EdgeInstanceID: strings.TrimSpace(cfg.Auth.EdgeInstanceID),
		UpdatedByNode:  "codex-smoke",
		UpdatedByUser:  "codex-smoke",
	}
	repo := database.NewRepository(db)
	if err := repo.CreateDetectionStandard(&standard, nil); err != nil {
		return fmt.Errorf("create success smoke standard: %w", err)
	}

	plan := models.DetectionPlan{
		PlanNo:          planNo,
		SourceSystem:    sourceSystem,
		ExternalPlanID:  externalPlanID,
		ExternalOrderNo: "SMOKE-ORDER-START-" + stamp,
		FactoryNo:       "SMOKE-FACTORY-START-" + stamp,
		CustomerName:    "Codex Smoke",
		DeviceModel:     "Smoke Model",
		TestItemCode:    "SMOKE-START",
		TestItemName:    "Smoke Successful Start",
		TestSequence:    2,
		Mode:            "standard",
		StandardCode:    standard.StandardCode,
		Status:          models.DetectionPlanStatusPending,
		SyncScope:       "global",
		UpdatedByNode:   "codex-smoke",
		UpdatedByUser:   "codex-smoke",
	}
	if err := db.Create(&plan).Error; err != nil {
		return fmt.Errorf("insert success smoke plan: %w", err)
	}

	token, err := login(baseURL, loginInput.user, loginInput.pass)
	if err != nil {
		return err
	}
	list, err := getPlans(baseURL, token, plan.FactoryNo)
	if err != nil {
		return err
	}
	if len(list.Items) != 1 || list.Items[0].PlanNo != planNo {
		return fmt.Errorf("successful smoke plan not visible through API: total=%d items=%d", list.Total, len(list.Items))
	}

	status, body, err := postJSON(baseURL+"/api/v1/detection-plans/"+fmt.Sprint(plan.ID)+"/start", token, map[string]any{
		"project_id":    project.ID,
		"operator_note": "codex smoke successful start",
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("successful plan start failed status=%d body=%s", status, body)
	}
	var started startResponse
	if err := json.Unmarshal([]byte(body), &started); err != nil {
		return fmt.Errorf("decode successful start response: %w", err)
	}
	if started.Plan.Status != models.DetectionPlanStatusStarted || started.Plan.StartedTaskID == nil || *started.Plan.StartedTaskID == 0 {
		return fmt.Errorf("start response did not mark plan started: %+v", started.Plan)
	}
	if started.Task.ID == 0 || started.Task.Status != models.DetectionStatusRunning || started.Task.TestNo != planNo {
		return fmt.Errorf("start response did not include running task: %+v", started.Task)
	}

	status, body, err = postJSON(baseURL+"/api/v1/detection-runs/"+fmt.Sprint(started.Task.ID)+"/stop", token, map[string]any{
		"reason": "codex smoke cleanup",
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("stop smoke task failed status=%d body=%s", status, body)
	}

	var after models.DetectionPlan
	if err := db.First(&after, plan.ID).Error; err != nil {
		return fmt.Errorf("reload started smoke plan: %w", err)
	}
	if after.Status != models.DetectionPlanStatusStarted || after.StartedTaskID == nil || *after.StartedTaskID != started.Task.ID {
		return fmt.Errorf("plan did not persist started task link: %+v", after)
	}
	return nil
}

func cleanupTask(db *gorm.DB, taskID uint) error {
	if taskID == 0 {
		return nil
	}
	_ = db.Model(&models.Project{}).Where("current_task_id = ?", taskID).Update("current_task_id", nil).Error
	tables := []any{
		&models.DetectionRunReportRequest{},
		&models.DetectionRunReport{},
		&models.DetectionRunNote{},
		&models.DetectionRunEvent{},
		&models.DetectionRunSummary{},
		&models.DetectionRunFeature{},
		&models.DetectionLimitAlarm{},
		&models.DetectionRunStorageRoute{},
		&models.DetectionRunStandardItem{},
	}
	for _, table := range tables {
		if err := db.Where("task_id = ?", taskID).Delete(table).Error; err != nil {
			return err
		}
	}
	return db.Delete(&models.DetectionTask{}, taskID).Error
}

func login(baseURL, user, pass string) (string, error) {
	status, body, err := postJSON(baseURL+"/api/v1/auth/login", "", map[string]string{"username": user, "password": pass})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("login failed status=%d body=%s", status, body)
	}
	var parsed loginResponse
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return "", err
	}
	token := firstNonEmpty(parsed.Token, parsed.AccessToken)
	if token == "" {
		return "", fmt.Errorf("login response did not include token")
	}
	return token, nil
}

func getPlans(baseURL, token, factoryNo string) (listResponse, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/detection-plans?status=pending&factory_no="+factoryNo+"&limit=5", nil)
	if err != nil {
		return listResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return listResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return listResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return listResponse{}, fmt.Errorf("list plans failed status=%d body=%s", resp.StatusCode, string(raw))
	}
	var parsed listResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return listResponse{}, err
	}
	return parsed, nil
}

func postJSON(url, token string, payload any) (int, string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
