//go:build perf_tools

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/protocol/kio"

	"gorm.io/gorm"
)

type result struct {
	ProjectID   uint   `json:"project_id"`
	ProjectCode string `json:"project_code"`
	StandardID  uint   `json:"standard_id"`
	TaskID      uint   `json:"task_id,omitempty"`
	TestNo      string `json:"test_no,omitempty"`
	Variables   int    `json:"variables"`
}

type statsResult struct {
	TaskID           uint             `json:"task_id"`
	TestNo           string           `json:"test_no"`
	AlarmTotal       int64            `json:"alarm_total"`
	AlarmByStatus    map[string]int64 `json:"alarm_by_status"`
	AlarmByType      map[string]int64 `json:"alarm_by_type"`
	HistoryRows      int64            `json:"history_rows"`
	RunStandardItems int64            `json:"run_standard_items"`
}

func main() {
	cfgPath := flag.String("config", "configs/config.json", "backend config path")
	vars := flag.Int("vars", 520, "number of perf variables")
	start := flag.Bool("start", true, "start a running detection task")
	reuse := flag.Bool("reuse", false, "reuse existing perf project, variables and standard, and only create a new task")
	statsTestNo := flag.String("stats-test-no", "", "print perf stats for a test number instead of creating data")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db); err != nil {
			log.Fatalf("auto migrate: %v", err)
		}
	}
	if *statsTestNo != "" {
		printStats(db, *statsTestNo)
		return
	}
	repo := database.NewRepository(db)
	if *reuse {
		printResult(startReuseTask(db, repo, *start))
		return
	}
	if err := cleanupPerfData(db); err != nil {
		log.Fatalf("cleanup perf data: %v", err)
	}

	now := time.Now()
	project := models.Project{
		ProjectCode:   "PERF-520",
		Name:          "Perf 520 Variables",
		DisplayName:   "Perf 520 Variables",
		DisplayNameEN: "Perf 520 Variables",
		DisplayNameJA: "Perf 520 Variables",
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(&project).Error; err != nil {
		log.Fatalf("create project: %v", err)
	}

	tags := make([]models.TagConfig, 0, *vars)
	routes := make([]models.StorageRoute, 0, *vars)
	items := make([]models.DetectionStandardItem, 0, *vars)
	limitL := -50.0
	limitH := 50.0
	for i := 1; i <= *vars; i++ {
		name := fmt.Sprintf("perf_%04d", i)
		tag := models.TagConfig{
			VarID:              int64(900000 + i),
			GatewayID:          1,
			SourceTopic:        "datachange_S_KIO_Project",
			SourcePath:         kio.PathFor(name, kio.ValueKey),
			SourceType:         models.TagSourceManual,
			RawName:            name,
			ProjectID:          &project.ID,
			ProjectCode:        project.ProjectCode,
			VarGroup:           "perf",
			VarName:            name,
			DisplayName:        name,
			DisplayNameEN:      name,
			DisplayNameJA:      name,
			JSONPath:           kio.PathFor(name, kio.ValueKey),
			DataType:           "FLOAT",
			DecimalPlaces:      2,
			ScaleFactor:        1,
			RWMode:             models.RWModeRead,
			WriteRequiresAudit: true,
			Discovered:         false,
			Placeholder:        false,
			Enabled:            true,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		tags = append(tags, tag)
		routes = append(routes, models.StorageRoute{
			ProjectID:     project.ID,
			VarID:         tag.VarID,
			RouteCode:     fmt.Sprintf("perf_520_%04d", i),
			StorageTarget: models.StorageTargetWideTable,
			StorageTable:  database.ProjectWideTableName(project.ID),
			ColumnName:    database.NormalizeStorageColumnName(name, name, tag.VarID),
			ColumnType:    database.StorageColumnTypeForDataType(tag.DataType),
			FormFieldKey:  name,
			QueryAlias:    name,
			TriggerMode:   models.StoreTriggerOnCycle,
			CycleMS:       1000,
			Deadband:      0,
			StoreOnStart:  true,
			Enabled:       true,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		items = append(items, models.DetectionStandardItem{
			VarID:           tag.VarID,
			VarName:         tag.VarName,
			DisplayName:     tag.DisplayName,
			DisplayNameEN:   tag.DisplayNameEN,
			DisplayNameJA:   tag.DisplayNameJA,
			CheckEnabled:    true,
			AlarmEnabled:    true,
			StoreEnabled:    true,
			CheckCycleMS:    1000,
			CheckOnStart:    true,
			CheckMethod:     models.CheckMethodNumericRange,
			LimitL:          &limitL,
			LimitH:          &limitH,
			LimitDeadband:   5,
			QualityPolicy:   models.QualityPolicyIgnoreBad,
			DecimalPlaces:   2,
			SortOrder:       i,
			ViolationHoldMS: 0,
			RecoverHoldMS:   0,
		})
	}
	if err := db.CreateInBatches(&tags, 200).Error; err != nil {
		log.Fatalf("create tags: %v", err)
	}
	if err := db.CreateInBatches(&routes, 200).Error; err != nil {
		log.Fatalf("create storage routes: %v", err)
	}
	standard := &models.DetectionStandard{
		StandardCode: "PERF-520-STD",
		Name:         "Perf 520 Standard",
		DisplayName:  "Perf 520 Standard",
		ProjectID:    &project.ID,
		ProjectCode:  project.ProjectCode,
		Mode:         "standard",
		Enabled:      true,
	}
	if err := repo.CreateDetectionStandard(standard, items); err != nil {
		log.Fatalf("create standard: %v", err)
	}

	out := result{ProjectID: project.ID, ProjectCode: project.ProjectCode, StandardID: standard.ID, Variables: *vars}
	if *start {
		testNo := fmt.Sprintf("PERF-520-%s", now.Format("20060102-150405"))
		task, err := repo.StartDetectionTaskWithOptions(database.StartDetectionOptions{
			ProjectID:   project.ID,
			TestNo:      testNo,
			Mode:        "standard",
			StandardID:  &standard.ID,
			DurationSec: 3600,
		})
		if err != nil {
			log.Fatalf("start detection task: %v", err)
		}
		out.TaskID = task.ID
		out.TestNo = task.TestNo
	}

	printResult(out)
}

func printResult(out result) {
	raw, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(raw))
}

func startReuseTask(db *gorm.DB, repo *database.Repository, start bool) result {
	var project models.Project
	if err := db.First(&project, "project_code = ?", "PERF-520").Error; err != nil {
		log.Fatalf("load perf project: %v", err)
	}
	var standard models.DetectionStandard
	if err := db.First(&standard, "standard_code = ?", "PERF-520-STD").Error; err != nil {
		log.Fatalf("load perf standard: %v", err)
	}
	if err := db.Model(&models.DetectionTask{}).
		Where("project_id = ? AND status = ?", project.ID, models.DetectionStatusRunning).
		Updates(map[string]interface{}{
			"status":   models.DetectionStatusStopped,
			"ended_at": time.Now(),
			"end_type": "perf_replaced",
		}).Error; err != nil {
		log.Fatalf("stop previous perf tasks: %v", err)
	}
	if err := db.Model(&models.Project{}).Where("id = ?", project.ID).Update("current_task_id", gorm.Expr("NULL")).Error; err != nil {
		log.Fatalf("clear current task: %v", err)
	}

	out := result{ProjectID: project.ID, ProjectCode: project.ProjectCode, StandardID: standard.ID, Variables: 520}
	if !start {
		return out
	}
	testNo := fmt.Sprintf("PERF-520-%s", time.Now().Format("20060102-150405"))
	task, err := repo.StartDetectionTaskWithOptions(database.StartDetectionOptions{
		ProjectID:   project.ID,
		TestNo:      testNo,
		Mode:        "standard",
		StandardID:  &standard.ID,
		DurationSec: 3600,
	})
	if err != nil {
		log.Fatalf("start detection task: %v", err)
	}
	out.TaskID = task.ID
	out.TestNo = task.TestNo
	return out
}

func printStats(db *gorm.DB, testNo string) {
	var task models.DetectionTask
	if err := db.First(&task, "test_no = ?", testNo).Error; err != nil {
		log.Fatalf("load task: %v", err)
	}
	out := statsResult{
		TaskID:        task.ID,
		TestNo:        task.TestNo,
		AlarmByStatus: make(map[string]int64),
		AlarmByType:   make(map[string]int64),
	}
	if err := db.Model(&models.DetectionLimitAlarm{}).Where("task_id = ?", task.ID).Count(&out.AlarmTotal).Error; err != nil {
		log.Fatalf("count alarms: %v", err)
	}
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := db.Model(&models.DetectionLimitAlarm{}).
		Select("status, COUNT(*) as count").
		Where("task_id = ?", task.ID).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		log.Fatalf("count alarms by status: %v", err)
	}
	for _, row := range statusRows {
		out.AlarmByStatus[row.Status] = row.Count
	}
	var typeRows []struct {
		AlarmType string
		Count     int64
	}
	if err := db.Model(&models.DetectionLimitAlarm{}).
		Select("alarm_type, COUNT(*) as count").
		Where("task_id = ?", task.ID).
		Group("alarm_type").
		Scan(&typeRows).Error; err != nil {
		log.Fatalf("count alarms by type: %v", err)
	}
	for _, row := range typeRows {
		out.AlarmByType[row.AlarmType] = row.Count
	}
	if err := db.Model(&models.HistoryData{}).Where("task_id = ?", task.ID).Count(&out.HistoryRows).Error; err != nil {
		log.Fatalf("count history: %v", err)
	}
	if err := db.Model(&models.DetectionRunStandardItem{}).Where("task_id = ?", task.ID).Count(&out.RunStandardItems).Error; err != nil {
		log.Fatalf("count run standard items: %v", err)
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(raw))
}

func cleanupPerfData(db *gorm.DB) error {
	var taskIDs []uint
	if err := db.Model(&models.DetectionTask{}).
		Where("test_no LIKE ?", "PERF-520-%").
		Pluck("id", &taskIDs).Error; err != nil {
		return err
	}
	if len(taskIDs) > 0 {
		if err := db.Delete(&models.DetectionLimitAlarm{}, "task_id IN ?", taskIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.HistoryData{}, "task_id IN ?", taskIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.DetectionRunStandardItem{}, "task_id IN ?", taskIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.DetectionRunNote{}, "task_id IN ?", taskIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.DetectionRunReport{}, "task_id IN ?", taskIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.DetectionTask{}, "id IN ?", taskIDs).Error; err != nil {
			return err
		}
	}
	var standardIDs []uint
	if err := db.Model(&models.DetectionStandard{}).
		Where("standard_code = ?", "PERF-520-STD").
		Pluck("id", &standardIDs).Error; err != nil {
		return err
	}
	if len(standardIDs) > 0 {
		if err := db.Delete(&models.DetectionStandardItem{}, "standard_id IN ?", standardIDs).Error; err != nil {
			return err
		}
		if err := db.Delete(&models.DetectionStandard{}, "id IN ?", standardIDs).Error; err != nil {
			return err
		}
	}
	if err := db.Delete(&models.StorageRoute{}, "var_id BETWEEN ? AND ?", 900001, 900000+10000).Error; err != nil {
		return err
	}
	if err := db.Delete(&models.StorageRoute{}, "query_alias LIKE ?", "perf_%").Error; err != nil {
		return err
	}
	if err := db.Model(&models.Project{}).
		Where("project_code = ?", "PERF-520").
		Update("current_task_id", gorm.Expr("NULL")).Error; err != nil {
		return err
	}
	if err := db.Delete(&models.TagConfig{}, "var_id BETWEEN ? AND ?", 900001, 900000+10000).Error; err != nil {
		return err
	}
	if err := db.Delete(&models.TagConfig{}, "var_name LIKE ?", "perf_%").Error; err != nil {
		return err
	}
	return db.Delete(&models.Project{}, "project_code = ?", "PERF-520").Error
}
