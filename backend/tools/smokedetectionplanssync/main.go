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
	"spindle-edge/backend/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type dbConfigFile struct {
	Database config.DatabaseConfig `json:"database"`
}

type loginResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

type startResponse struct {
	Plan models.DetectionPlan `json:"plan"`
	Task models.DetectionTask `json:"task"`
}

func main() {
	mainConfig := flag.String("main-config", "", "main-server config JSON with the synchronized mirror database")
	edgeConfig := flag.String("edge-config", envDefault("EDGE_CONFIG", "configs/config.json"), "edge backend config JSON")
	edgeBaseURL := flag.String("edge-base-url", "http://127.0.0.1:18080", "edge backend base URL")
	projectCode := flag.String("project-code", "", "existing synchronized project_code to start on")
	standardCode := flag.String("standard-code", "", "existing synchronized detection standard_code")
	user := flag.String("user", "admin", "edge login username")
	pass := flag.String("pass", "Admin@12345", "edge login password")
	timeout := flag.Duration("timeout", 2*time.Minute, "maximum wait for each sync direction")
	interval := flag.Duration("interval", 2*time.Second, "poll interval while waiting for sync")
	keepPlan := flag.Bool("keep-plan", false, "keep the smoke plan row after the run")
	allowSameDB := flag.Bool("allow-same-db", false, "deprecated compatibility flag; same-database checks are allowed because local/dev environments may share one database")
	preflightOnly := flag.Bool("preflight-only", false, "check main/edge database visibility, project/config consistency, and edge login without inserting a plan or starting a task")
	confirmStart := flag.Bool("confirm-start", false, "required for the full smoke because it starts and stops a real detection task")
	flag.Parse()

	if err := run(syncSmokeOptions{
		MainConfigPath: *mainConfig,
		EdgeConfigPath: *edgeConfig,
		EdgeBaseURL:    *edgeBaseURL,
		ProjectCode:    *projectCode,
		StandardCode:   *standardCode,
		User:           *user,
		Pass:           *pass,
		Timeout:        *timeout,
		Interval:       *interval,
		KeepPlan:       *keepPlan,
		AllowSameDB:    *allowSameDB,
		PreflightOnly:  *preflightOnly,
		ConfirmStart:   *confirmStart,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "smokedetectionplanssync failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("smokedetectionplanssync passed")
}

type syncSmokeOptions struct {
	MainConfigPath string
	EdgeConfigPath string
	EdgeBaseURL    string
	ProjectCode    string
	StandardCode   string
	User           string
	Pass           string
	Timeout        time.Duration
	Interval       time.Duration
	KeepPlan       bool
	AllowSameDB    bool
	PreflightOnly  bool
	ConfirmStart   bool
}

func run(opts syncSmokeOptions) error {
	if strings.TrimSpace(opts.MainConfigPath) == "" {
		return fmt.Errorf("main-config is required")
	}
	if strings.TrimSpace(opts.ProjectCode) == "" {
		return fmt.Errorf("project-code is required")
	}
	if strings.TrimSpace(opts.StandardCode) == "" {
		return fmt.Errorf("standard-code is required")
	}
	if !opts.PreflightOnly && !opts.ConfirmStart {
		return fmt.Errorf("confirm-start is required for full sync smoke because it starts and stops a real detection task; run with -preflight-only first")
	}
	mainDB, mainCfg, err := connectConfigDB(opts.MainConfigPath)
	if err != nil {
		return fmt.Errorf("connect main db: %w", err)
	}
	defer closeDB(mainDB)
	edgeDB, edgeCfg, err := connectConfigDB(opts.EdgeConfigPath)
	if err != nil {
		return fmt.Errorf("connect edge db: %w", err)
	}
	defer closeDB(edgeDB)
	if sameDatabase(mainCfg.Database, edgeCfg.Database) && !opts.AllowSameDB {
		fmt.Printf("main and edge database configs point to the same database (%s:%d/%s); continuing as a local consistency check\n", mainCfg.Database.Host, mainCfg.Database.Port, mainCfg.Database.Name)
	}

	preflight, err := loadPreflight(mainDB, edgeDB, opts.ProjectCode, opts.StandardCode)
	if err != nil {
		return err
	}
	token, err := login(opts.EdgeBaseURL, opts.User, opts.Pass)
	if err != nil {
		return err
	}
	if opts.PreflightOnly {
		fmt.Printf("preflight ok project=%s project_group=%s main_project_id=%d edge_project_id=%d standard=%s standard_project_group=%s edge_standard_id=%d\n", opts.ProjectCode, preflight.EdgeProject.ProjectGroup, preflight.MainProject.ID, preflight.EdgeProject.ID, opts.StandardCode, preflight.EdgeStandard.ProjectGroup, preflight.EdgeStandard.ID)
		return nil
	}

	stamp := time.Now().Format("20060102150405")
	planNo := "SYNC-SMOKE-PLAN-" + stamp
	sourceSystem := "codex-sync-smoke"
	externalPlanID := "sync-" + stamp
	plan := models.DetectionPlan{
		PlanNo:          planNo,
		SourceSystem:    sourceSystem,
		ExternalPlanID:  externalPlanID,
		ExternalOrderNo: "SYNC-SMOKE-ORDER-" + stamp,
		FactoryNo:       "SYNC-SMOKE-FACTORY-" + stamp,
		CustomerName:    "Codex Sync Smoke",
		DeviceModel:     "Sync Smoke Model",
		TestItemCode:    "SYNC-SMOKE",
		TestItemName:    "Sync Smoke Start",
		TestSequence:    1,
		Mode:            firstNonEmpty(preflight.MainStandard.Mode, "standard"),
		StandardCode:    opts.StandardCode,
		Status:          models.DetectionPlanStatusPending,
		SyncScope:       "global",
		UpdatedByNode:   "codex-sync-smoke",
		UpdatedByUser:   "codex-sync-smoke",
	}
	cleanup := func() {
		if opts.KeepPlan {
			return
		}
		deletePlan(mainDB, planNo, sourceSystem, externalPlanID)
		deletePlan(edgeDB, planNo, sourceSystem, externalPlanID)
	}
	cleanup()
	defer cleanup()

	if err := mainDB.Create(&plan).Error; err != nil {
		return fmt.Errorf("insert smoke plan into main db: %w", err)
	}
	fmt.Printf("inserted main plan id=%d plan_no=%s\n", plan.ID, plan.PlanNo)

	edgePlan, err := waitForPlan(edgeDB, planNo, models.DetectionPlanStatusPending, opts.Timeout, opts.Interval)
	if err != nil {
		return fmt.Errorf("wait main->edge plan sync: %w", err)
	}
	fmt.Printf("edge plan visible id=%d status=%s\n", edgePlan.ID, edgePlan.Status)

	status, body, err := postJSON(opts.EdgeBaseURL+"/api/v1/detection-plans/"+fmt.Sprint(edgePlan.ID)+"/start", token, map[string]any{
		"project_id":    preflight.EdgeProject.ID,
		"operator_note": "codex sync smoke",
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("edge plan start failed status=%d body=%s", status, body)
	}
	var started startResponse
	if err := json.Unmarshal([]byte(body), &started); err != nil {
		return fmt.Errorf("decode edge start response: %w", err)
	}
	if started.Plan.Status != models.DetectionPlanStatusStarted || started.Plan.StartedTaskID == nil {
		return fmt.Errorf("edge start did not mark plan started: %+v", started.Plan)
	}
	fmt.Printf("edge plan started task_id=%d\n", *started.Plan.StartedTaskID)

	status, body, err = postJSON(opts.EdgeBaseURL+"/api/v1/detection-runs/"+fmt.Sprint(started.Task.ID)+"/stop", token, map[string]any{
		"reason": "codex sync smoke cleanup",
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("edge stop smoke task failed status=%d body=%s", status, body)
	}

	mainStarted, err := waitForPlan(mainDB, planNo, models.DetectionPlanStatusStarted, opts.Timeout, opts.Interval)
	if err != nil {
		return fmt.Errorf("wait edge->main plan started sync: %w", err)
	}
	if mainStarted.StartedTaskID == nil || *mainStarted.StartedTaskID == 0 {
		return fmt.Errorf("main plan started without started_task_id: %+v", mainStarted)
	}
	fmt.Printf("main plan updated status=%s started_task_id=%d\n", mainStarted.Status, *mainStarted.StartedTaskID)
	return nil
}

type preflightState struct {
	MainProject  models.Project
	MainStandard models.DetectionStandard
	EdgeProject  models.Project
	EdgeStandard models.DetectionStandard
}

func loadPreflight(mainDB *gorm.DB, edgeDB *gorm.DB, projectCode string, standardCode string) (preflightState, error) {
	var state preflightState
	if err := mainDB.First(&state.MainProject, "project_code = ?", projectCode).Error; err != nil {
		return state, fmt.Errorf("main db project %q not found; sync project/config first: %w", projectCode, err)
	}
	if err := mainDB.First(&state.MainStandard, "standard_code = ?", standardCode).Error; err != nil {
		return state, fmt.Errorf("main db standard %q not found; sync config first: %w", standardCode, err)
	}
	if !standardMatchesProject(state.MainStandard, state.MainProject) {
		return state, fmt.Errorf("main standard project mismatch standard_project_id=%s main_project_id=%d standard_project_group=%q main_project_group=%q", formatOptionalUint(state.MainStandard.ProjectID), state.MainProject.ID, state.MainStandard.ProjectGroup, state.MainProject.ProjectGroup)
	}
	if err := edgeDB.First(&state.EdgeProject, "project_code = ?", projectCode).Error; err != nil {
		return state, fmt.Errorf("edge db project %q not found after sync: %w", projectCode, err)
	}
	if strings.TrimSpace(state.MainProject.ProjectGroup) != strings.TrimSpace(state.EdgeProject.ProjectGroup) {
		return state, fmt.Errorf("project_group mismatch main=%q edge=%q for project_code=%s", state.MainProject.ProjectGroup, state.EdgeProject.ProjectGroup, projectCode)
	}
	if err := edgeDB.First(&state.EdgeStandard, "standard_code = ?", standardCode).Error; err != nil {
		return state, fmt.Errorf("edge db standard %q not found after sync: %w", standardCode, err)
	}
	if strings.TrimSpace(state.MainStandard.ProjectGroup) != strings.TrimSpace(state.EdgeStandard.ProjectGroup) {
		return state, fmt.Errorf("standard project_group mismatch main=%q edge=%q for standard_code=%s", state.MainStandard.ProjectGroup, state.EdgeStandard.ProjectGroup, standardCode)
	}
	if !standardMatchesProject(state.EdgeStandard, state.EdgeProject) {
		return state, fmt.Errorf("edge standard project mismatch standard_project_id=%s edge_project_id=%d standard_project_group=%q edge_project_group=%q", formatOptionalUint(state.EdgeStandard.ProjectID), state.EdgeProject.ID, state.EdgeStandard.ProjectGroup, state.EdgeProject.ProjectGroup)
	}
	return state, nil
}

func standardMatchesProject(standard models.DetectionStandard, project models.Project) bool {
	if standard.ProjectID != nil && *standard.ProjectID != project.ID {
		return false
	}
	if strings.TrimSpace(standard.ProjectCode) != "" && !strings.EqualFold(strings.TrimSpace(standard.ProjectCode), strings.TrimSpace(project.ProjectCode)) {
		return false
	}
	if strings.TrimSpace(standard.ProjectGroup) != "" && !strings.EqualFold(strings.TrimSpace(standard.ProjectGroup), strings.TrimSpace(project.ProjectGroup)) {
		return false
	}
	return true
}

func formatOptionalUint(value *uint) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprint(*value)
}

func connectConfigDB(path string) (*gorm.DB, dbConfigFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, dbConfigFile{}, err
	}
	var cfg dbConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, dbConfigFile{}, err
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.Name == "" {
		return nil, dbConfigFile{}, fmt.Errorf("database host, user, and name are required in %s", path)
	}
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=5s&readTimeout=30s&writeTimeout=30s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	return db, cfg, err
}

func sameDatabase(a config.DatabaseConfig, b config.DatabaseConfig) bool {
	return strings.EqualFold(strings.TrimSpace(a.Host), strings.TrimSpace(b.Host)) &&
		a.Port == b.Port &&
		strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name))
}

func closeDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func waitForPlan(db *gorm.DB, planNo string, status string, timeout time.Duration, interval time.Duration) (models.DetectionPlan, error) {
	deadline := time.Now().Add(timeout)
	var last models.DetectionPlan
	for {
		var plan models.DetectionPlan
		err := db.First(&plan, "plan_no = ?", planNo).Error
		if err == nil {
			last = plan
			if status == "" || plan.Status == status {
				return plan, nil
			}
		}
		if time.Now().After(deadline) {
			if last.ID != 0 {
				return models.DetectionPlan{}, fmt.Errorf("plan reached edge but status=%s, want %s", last.Status, status)
			}
			return models.DetectionPlan{}, fmt.Errorf("plan %s not visible before timeout", planNo)
		}
		time.Sleep(interval)
	}
}

func deletePlan(db *gorm.DB, planNo, sourceSystem, externalPlanID string) {
	_ = db.Where("plan_no = ? OR (source_system = ? AND external_plan_id = ?)", planNo, sourceSystem, externalPlanID).Delete(&models.DetectionPlan{}).Error
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
