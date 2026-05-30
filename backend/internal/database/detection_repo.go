package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) LoadActiveDetectionTasks() ([]models.DetectionTask, error) {
	var tasks []models.DetectionTask
	if err := r.db.Where("status = ?", "running").Order("started_at asc").Find(&tasks).Error; err != nil {
		return nil, err
	}
	if err := r.attachRunStandardItems(tasks); err != nil {
		return nil, err
	}
	if err := r.attachRunStorageRoutes(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) StartDetectionTask(ProjectID uint, testNo string, mode string, standardID *uint) (*models.DetectionTask, error) {
	return r.StartDetectionTaskWithOptions(StartDetectionOptions{
		ProjectID:  ProjectID,
		TestNo:     testNo,
		Mode:       mode,
		StandardID: standardID,
	})
}

func (r *Repository) StartDetectionTaskWithOptions(opts StartDetectionOptions) (*models.DetectionTask, error) {
	now := time.Now()
	limitCheckEnabled := true
	if opts.LimitCheckEnabled != nil {
		limitCheckEnabled = *opts.LimitCheckEnabled
	}
	endPolicy := strings.TrimSpace(opts.EndPolicy)
	if endPolicy == "" {
		endPolicy = models.DetectionEndPolicyManual
	}
	if !isValidDetectionEndPolicy(endPolicy) {
		return nil, fmt.Errorf("invalid end_policy")
	}
	if endPolicy == models.DetectionEndPolicyFixedDuration && opts.DurationSec <= 0 {
		return nil, fmt.Errorf("duration_sec is required for fixed_duration")
	}
	if endPolicy == models.DetectionEndPolicyQualifiedHold && opts.QualifiedHoldMS <= 0 {
		return nil, fmt.Errorf("qualified_hold_ms is required for qualified_hold")
	}
	var expectedEndAt *time.Time
	if opts.DurationSec > 0 {
		value := now.Add(time.Duration(opts.DurationSec) * time.Second)
		expectedEndAt = &value
	}
	customConfigJSON := customDetectionConfigJSON(opts)
	task := &models.DetectionTask{
		TestNo:            opts.TestNo,
		Mode:              opts.Mode,
		Status:            models.DetectionStatusRunning,
		StartedAt:         &now,
		LimitCheckEnabled: limitCheckEnabled,
		EndPolicy:         endPolicy,
		DurationSec:       opts.DurationSec,
		QualifiedHoldMS:   opts.QualifiedHoldMS,
		ExpectedEndAt:     expectedEndAt,
		OperatorNote:      opts.OperatorNote,
		CustomConfigJSON:  customConfigJSON,
	}
	var snapshotItems []models.DetectionRunStandardItem
	var runStorageRoutes []models.DetectionRunStorageRoute

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var Project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&Project, "id = ? AND enabled = ? AND blocked = ?", opts.ProjectID, true, false).Error; err != nil {
			return err
		}
		if Project.CurrentTaskID != nil {
			return ErrProjectAlreadyRunning
		}
		var running int64
		if err := tx.Model(&models.DetectionTask{}).
			Where("project_id = ? AND status = ?", opts.ProjectID, models.DetectionStatusRunning).
			Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrProjectAlreadyRunning
		}
		task.ProjectID = Project.ID
		task.ProjectCode = Project.ProjectCode

		if opts.StandardID != nil && len(opts.CustomItems) > 0 {
			return errors.New("standard_id and custom_items cannot both be set")
		}
		if opts.StandardID != nil {
			standard, items, err := loadDetectionStandardWithItems(tx, *opts.StandardID)
			if err != nil {
				return err
			}
			if !standard.Enabled {
				return errors.New("detection standard is disabled")
			}
			task.StandardID = &standard.ID
			task.StandardCode = standard.StandardCode
			task.StandardVer = standard.Version
			if opts.ReportTemplateID == nil && standard.ReportTemplateID != nil {
				opts.ReportTemplateID = standard.ReportTemplateID
			}
			tagByVarID, err := loadTagsByVarID(tx, items)
			if err != nil {
				return err
			}
			snapshotItems = makeRunStandardItems(task, standard, items, tagByVarID, now)
		} else if len(opts.CustomItems) > 0 {
			items := opts.CustomItems
			if err := validateDetectionStandardItems(items); err != nil {
				return err
			}
			tagByVarID, err := loadTagsByVarID(tx, items)
			if err != nil {
				return err
			}
			task.StandardCode = "custom"
			task.StandardVer = 1
			snapshotItems = makeRunStandardItems(task, models.DetectionStandard{
				ID:           0,
				StandardCode: "custom",
				Version:      1,
			}, items, tagByVarID, now)
		}
		if !limitCheckEnabled {
			for i := range snapshotItems {
				snapshotItems[i].CheckEnabled = false
				snapshotItems[i].AlarmEnabled = false
			}
		}
		if opts.ReportTemplateID != nil {
			template, err := getReportTemplate(tx, *opts.ReportTemplateID)
			if err != nil {
				return err
			}
			if !template.Enabled {
				return errors.New("report template is disabled")
			}
			task.ReportTemplateID = &template.ID
			task.ReportTemplateCode = template.TemplateCode
			task.ReportTemplateVersion = template.Version
			task.TemplateRef = template.FileRef
		}
		if err := tx.Select("*").Create(task).Error; err != nil {
			return err
		}
		if !limitCheckEnabled {
			if err := tx.Model(&models.DetectionTask{}).Where("id = ?", task.ID).Update("limit_check_enabled", false).Error; err != nil {
				return err
			}
			task.LimitCheckEnabled = false
		}
		for i := range snapshotItems {
			snapshotItems[i].TaskID = task.ID
		}
		if len(snapshotItems) > 0 {
			if err := tx.Select("*").Create(&snapshotItems).Error; err != nil {
				return err
			}
			if !limitCheckEnabled {
				if err := tx.Model(&models.DetectionRunStandardItem{}).Where("task_id = ?", task.ID).Updates(map[string]interface{}{"check_enabled": false, "alarm_enabled": false}).Error; err != nil {
					return err
				}
				for i := range snapshotItems {
					snapshotItems[i].CheckEnabled = false
					snapshotItems[i].AlarmEnabled = false
				}
			}
		}
		if opts.StandardID != nil {
			if err := upsertDetectionStandardRecent(tx, opts.StartedByUserID, *opts.StandardID, task.ProjectID, now); err != nil {
				return err
			}
		}
		var err error
		runStorageRoutes, err = freezeDetectionRunStorageRoutes(tx, task, snapshotItems, now)
		if err != nil {
			return err
		}
		if err := NewRepository(tx).EnsureProjectWideTable(task.ProjectID, runStorageRoutes); err != nil {
			return err
		}
		return tx.Model(&models.Project{}).
			Where("id = ?", Project.ID).
			Update("current_task_id", task.ID).Error
	})
	if err != nil {
		return nil, err
	}
	task.StandardItems = snapshotItems
	task.StorageRoutes = runStorageRoutes
	return task, nil
}

func (r *Repository) StopDetectionTask(taskID uint, reason string) (*models.DetectionTask, error) {
	return r.StopDetectionTaskWithEndType(taskID, reason, models.DetectionEndManualStop)
}

func (r *Repository) StopDetectionTaskWithEndType(taskID uint, reason string, endType string) (*models.DetectionTask, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ? AND status IN ?", taskID, []string{models.DetectionStatusRunning, models.DetectionStatusPaused}).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	pausedDurationMS := task.PausedDurationMS + currentPauseDurationMS(task, now)
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.DetectionTask{}).
			Where("id = ?", taskID).
			Updates(map[string]interface{}{
				"status":             models.DetectionStatusStopped,
				"ended_at":           &now,
				"end_type":           endType,
				"stop_reason":        reason,
				"pause_started_at":   nil,
				"paused_duration_ms": pausedDurationMS,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&models.Project{}).
			Where("id = ?", task.ProjectID).
			Update("current_task_id", gorm.Expr("NULL")).Error
	})
	if err != nil {
		return nil, err
	}
	task.Status = models.DetectionStatusStopped
	task.EndedAt = &now
	task.EndType = endType
	task.StopReason = reason
	task.PauseStartedAt = nil
	task.PausedDurationMS = pausedDurationMS
	return &task, nil
}

func (r *Repository) PauseDetectionTask(taskID uint, reason string) (*models.DetectionTask, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ? AND status = ?", taskID, models.DetectionStatusRunning).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status":           models.DetectionStatusPaused,
		"pause_started_at": &now,
		"updated_at":       now,
	}
	if strings.TrimSpace(reason) != "" {
		updates["stop_reason"] = reason
	}
	if err := r.db.Model(&models.DetectionTask{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
		return nil, err
	}
	task.Status = models.DetectionStatusPaused
	task.PauseStartedAt = &now
	if value, ok := updates["stop_reason"].(string); ok {
		task.StopReason = value
	}
	return &task, nil
}

func (r *Repository) ResumeDetectionTask(taskID uint) (*models.DetectionTask, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ? AND status = ?", taskID, models.DetectionStatusPaused).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	pauseDelta := currentPauseDurationMS(task, now)
	pausedDurationMS := task.PausedDurationMS + pauseDelta
	var expectedEndAt *time.Time
	if task.ExpectedEndAt != nil && pauseDelta > 0 {
		value := task.ExpectedEndAt.Add(time.Duration(pauseDelta) * time.Millisecond)
		expectedEndAt = &value
	} else {
		expectedEndAt = task.ExpectedEndAt
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.DetectionTask{}).
			Where("id = ? AND status = ?", taskID, models.DetectionStatusPaused).
			Updates(map[string]interface{}{
				"status":             models.DetectionStatusRunning,
				"stop_reason":        "",
				"pause_started_at":   nil,
				"paused_duration_ms": pausedDurationMS,
				"expected_end_at":    expectedEndAt,
				"updated_at":         now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&models.Project{}).
			Where("id = ? AND (current_task_id IS NULL OR current_task_id = ?)", task.ProjectID, task.ID).
			Update("current_task_id", task.ID).Error
	}); err != nil {
		return nil, err
	}
	task.Status = models.DetectionStatusRunning
	task.StopReason = ""
	task.PauseStartedAt = nil
	task.PausedDurationMS = pausedDurationMS
	task.ExpectedEndAt = expectedEndAt
	var items []models.DetectionRunStandardItem
	if err := r.db.Where("task_id = ?", task.ID).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return nil, err
	}
	task.StandardItems = items
	routes, err := r.ListRunStorageRoutes(task.ID)
	if err != nil {
		return nil, err
	}
	task.StorageRoutes = routes
	return &task, nil
}

func (r *Repository) ListDetectionTasks(filter DetectionTaskFilter) ([]models.DetectionTask, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	query := r.db.Model(&models.DetectionTask{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.TestNo != "" {
		query = query.Where("test_no = ?", filter.TestNo)
	}
	if filter.Start != nil {
		query = query.Where("started_at >= ?", *filter.Start)
	}
	if filter.End != nil {
		query = query.Where("started_at <= ?", *filter.End)
	}
	var tasks []models.DetectionTask
	if err := query.Order("started_at desc, id desc").Limit(limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	if err := r.attachRunStandardItems(tasks); err != nil {
		return nil, err
	}
	if err := r.attachRunStorageRoutes(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) GetCurrentDetectionTaskForProject(projectID uint) (models.DetectionTask, error) {
	var task models.DetectionTask
	if err := r.db.
		Where("project_id = ? AND status IN ?", projectID, []string{models.DetectionStatusRunning, models.DetectionStatusPaused}).
		Order("started_at desc, id desc").
		First(&task).Error; err != nil {
		return task, err
	}
	return r.GetDetectionTask(task.ID)
}

func currentPauseDurationMS(task models.DetectionTask, now time.Time) int64 {
	if task.Status != models.DetectionStatusPaused || task.PauseStartedAt == nil || now.Before(*task.PauseStartedAt) {
		return 0
	}
	return now.Sub(*task.PauseStartedAt).Milliseconds()
}

func (r *Repository) GetDetectionTask(id uint) (models.DetectionTask, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ?", id).Error; err != nil {
		return task, err
	}
	if err := r.attachRunStandardItems([]models.DetectionTask{task}); err != nil {
		return task, err
	}
	var items []models.DetectionRunStandardItem
	if err := r.db.Where("task_id = ?", id).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return task, err
	}
	task.StandardItems = items
	routes, err := r.ListRunStorageRoutes(id)
	if err != nil {
		return task, err
	}
	task.StorageRoutes = routes
	task.RecentNotes, _ = r.ListDetectionRunNotes(id, 5)
	task.Reports, _ = r.ListDetectionRunReports(id)
	return task, nil
}

func (r *Repository) UpdateDetectionRunStandardItem(taskID uint, varID int64, updates map[string]interface{}) (models.DetectionRunStandardItem, error) {
	var item models.DetectionRunStandardItem
	if len(updates) == 0 {
		err := r.db.First(&item, "task_id = ? AND var_id = ?", taskID, varID).Error
		return item, err
	}
	if err := r.db.Model(&models.DetectionRunStandardItem{}).
		Where("task_id = ? AND var_id = ?", taskID, varID).
		Updates(updates).Error; err != nil {
		return item, err
	}
	err := r.db.First(&item, "task_id = ? AND var_id = ?", taskID, varID).Error
	return item, err
}

func (r *Repository) CreateDetectionRunEvent(event *models.DetectionRunEvent) error {
	now := time.Now()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	event.CreatedAt = now
	return r.db.Create(event).Error
}

func (r *Repository) ListDetectionRunEvents(taskID uint, limit int) ([]models.DetectionRunEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var events []models.DetectionRunEvent
	err := r.db.Where("task_id = ?", taskID).
		Order("occurred_at asc, id asc").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *Repository) RefreshDetectionRunSummary(taskID uint) (models.DetectionRunSummary, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return models.DetectionRunSummary{}, err
	}

	now := time.Now()
	summary := models.DetectionRunSummary{
		TaskID:          task.ID,
		TestNo:          task.TestNo,
		ProjectID:       task.ProjectID,
		ProjectCode:     task.ProjectCode,
		ResultStatus:    models.DetectionSummaryStatusUnknown,
		StartedAt:       task.StartedAt,
		EndedAt:         task.EndedAt,
		LastRefreshedAt: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if task.StartedAt != nil {
		end := now
		if task.EndedAt != nil {
			end = *task.EndedAt
		}
		durationMS := end.Sub(*task.StartedAt).Milliseconds() - task.PausedDurationMS - currentPauseDurationMS(task, now)
		if durationMS < 0 {
			durationMS = 0
		}
		summary.DurationMS = durationMS
	}
	if task.Status == models.DetectionStatusRunning || task.Status == models.DetectionStatusPaused {
		summary.ResultStatus = models.DetectionSummaryStatusRunning
	}

	if err := r.db.Model(&models.HistoryData{}).Where("task_id = ?", task.ID).Count(&summary.HistoryRows).Error; err != nil {
		return models.DetectionRunSummary{}, err
	}
	if err := r.db.Model(&models.DetectionLimitAlarm{}).Where("task_id = ?", task.ID).Count(&summary.AlarmTotal).Error; err != nil {
		return models.DetectionRunSummary{}, err
	}
	summary.AlarmActive = r.countDetectionAlarms(task.ID, "status", models.DetectionAlarmStatusActive)
	summary.AlarmRecovered = r.countDetectionAlarms(task.ID, "status", models.DetectionAlarmStatusClosed)
	summary.AlarmAboveH = r.countDetectionAlarms(task.ID, "alarm_type", "above_h")
	summary.AlarmAboveHH = r.countDetectionAlarms(task.ID, "alarm_type", "above_hh")
	summary.AlarmBelowL = r.countDetectionAlarms(task.ID, "alarm_type", "below_l")
	summary.AlarmBelowLL = r.countDetectionAlarms(task.ID, "alarm_type", "below_ll")

	var firstAlarm models.DetectionLimitAlarm
	if err := r.db.Where("task_id = ?", task.ID).Order("first_seen_at asc, id asc").First(&firstAlarm).Error; err == nil {
		value := firstAlarm.FirstSeenAt
		summary.FirstAlarmAt = &value
	}
	var lastAlarm models.DetectionLimitAlarm
	if err := r.db.Where("task_id = ?", task.ID).Order("last_seen_at desc, id desc").First(&lastAlarm).Error; err == nil {
		value := lastAlarm.LastSeenAt
		summary.LastAlarmAt = &value
	}
	if task.Status != models.DetectionStatusRunning && task.Status != models.DetectionStatusPaused {
		if summary.AlarmTotal > 0 || summary.AlarmActive > 0 {
			summary.ResultStatus = models.DetectionSummaryStatusNG
		} else {
			summary.ResultStatus = models.DetectionSummaryStatusOK
		}
	}

	err := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"test_no",
			"project_id",
			"project_code",
			"result_status",
			"started_at",
			"ended_at",
			"duration_ms",
			"history_rows",
			"alarm_total",
			"alarm_active",
			"alarm_recovered",
			"alarm_above_h",
			"alarm_above_hh",
			"alarm_below_l",
			"alarm_below_ll",
			"first_alarm_at",
			"last_alarm_at",
			"last_refreshed_at",
			"updated_at",
		}),
	}).Create(&summary).Error
	if err != nil {
		return models.DetectionRunSummary{}, err
	}
	var saved models.DetectionRunSummary
	err = r.db.First(&saved, "task_id = ?", taskID).Error
	return saved, err
}

type detectionFeatureAggregate struct {
	VarID       int64
	VarName     string
	SampleCount int64
	AvgValue    *float64
	MinValue    *float64
	MaxValue    *float64
}

func (r *Repository) RefreshDetectionRunFeatures(taskID uint) ([]models.DetectionRunFeature, error) {
	var task models.DetectionTask
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, err
	}
	var aggregates []detectionFeatureAggregate
	if err := r.db.Model(&models.HistoryData{}).
		Select("var_id, var_name, COUNT(*) AS sample_count, AVG(value) AS avg_value, MIN(value) AS min_value, MAX(value) AS max_value").
		Where("task_id = ? AND value IS NOT NULL", taskID).
		Group("var_id, var_name").
		Order("var_id asc").
		Scan(&aggregates).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	features := make([]models.DetectionRunFeature, 0, len(aggregates))
	for _, aggregate := range aggregates {
		var firstSample models.HistoryData
		if err := r.db.Where("task_id = ? AND var_id = ? AND value IS NOT NULL", taskID, aggregate.VarID).
			Order("source_time asc, id asc").
			First(&firstSample).Error; err != nil {
			return nil, err
		}
		var lastSample models.HistoryData
		if err := r.db.Where("task_id = ? AND var_id = ? AND value IS NOT NULL", taskID, aggregate.VarID).
			Order("source_time desc, id desc").
			First(&lastSample).Error; err != nil {
			return nil, err
		}
		features = append(features, models.DetectionRunFeature{
			TaskID:          task.ID,
			TestNo:          task.TestNo,
			ProjectID:       task.ProjectID,
			ProjectCode:     task.ProjectCode,
			VarID:           aggregate.VarID,
			VarName:         aggregate.VarName,
			SampleCount:     aggregate.SampleCount,
			AvgValue:        aggregate.AvgValue,
			MinValue:        aggregate.MinValue,
			MaxValue:        aggregate.MaxValue,
			FirstSampleTime: firstSample.SourceTime,
			LastSampleTime:  lastSample.SourceTime,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if len(features) > 0 {
		if err := r.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "task_id"}, {Name: "var_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"test_no",
				"project_id",
				"project_code",
				"var_name",
				"sample_count",
				"avg_value",
				"min_value",
				"max_value",
				"first_sample_time",
				"last_sample_time",
				"updated_at",
			}),
		}).Create(&features).Error; err != nil {
			return nil, err
		}
	}
	return r.ListDetectionRunFeatures(taskID)
}

func (r *Repository) ListDetectionRunFeatures(taskID uint) ([]models.DetectionRunFeature, error) {
	var features []models.DetectionRunFeature
	err := r.db.Where("task_id = ?", taskID).Order("var_id asc").Find(&features).Error
	return features, err
}

func (r *Repository) countDetectionAlarms(taskID uint, column string, value string) int64 {
	var count int64
	if err := r.db.Model(&models.DetectionLimitAlarm{}).
		Where("task_id = ? AND "+column+" = ?", taskID, value).
		Count(&count).Error; err != nil {
		return 0
	}
	return count
}

func (r *Repository) UpdateRunningRunItemsVariableDefaults(varID int64, tag models.TagConfig) (int64, error) {
	runningTasks := r.db.Model(&models.DetectionTask{}).
		Select("id").
		Where("status = ?", models.DetectionStatusRunning)
	result := r.db.Model(&models.DetectionRunStandardItem{}).
		Where("var_id = ? AND task_id IN (?)", varID, runningTasks).
		Updates(map[string]interface{}{
			"variable_default_alarm_enabled":     tag.DefaultAlarmEnabled,
			"variable_default_limit_ll":          tag.DefaultLimitLL,
			"variable_default_limit_l":           tag.DefaultLimitL,
			"variable_default_limit_h":           tag.DefaultLimitH,
			"variable_default_limit_hh":          tag.DefaultLimitHH,
			"variable_default_limit_deadband":    tag.DefaultLimitDeadband,
			"variable_default_violation_hold_ms": tag.DefaultViolationHoldMS,
			"variable_default_recover_hold_ms":   tag.DefaultRecoverHoldMS,
		})
	return result.RowsAffected, result.Error
}

func (r *Repository) CreateDetectionLimitAlarm(alarm *models.DetectionLimitAlarm) error {
	now := time.Now()
	alarm.CreatedAt = now
	alarm.UpdatedAt = now
	return r.db.Create(alarm).Error
}

func (r *Repository) CreateDetectionLimitAlarms(alarms []models.DetectionLimitAlarm) error {
	if len(alarms) == 0 {
		return nil
	}
	now := time.Now()
	for i := range alarms {
		alarms[i].CreatedAt = now
		alarms[i].UpdatedAt = now
	}
	return r.db.CreateInBatches(&alarms, 500).Error
}

func (r *Repository) RecoverDetectionLimitAlarm(event *models.DetectionLimitAlarmEvent) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":        models.DetectionAlarmStatusClosed,
		"recover_value": event.Alarm.RecoverValue,
		"recovered_at":  event.Alarm.RecoveredAt,
		"last_seen_at":  event.Alarm.LastSeenAt,
		"duration_ms":   event.Alarm.DurationMS,
		"quality":       event.Alarm.Quality,
		"updated_at":    now,
	}
	if event.Alarm.PeakValue != nil {
		updates["peak_value"] = event.Alarm.PeakValue
	}
	return r.db.Model(&models.DetectionLimitAlarm{}).
		Where("task_id = ? AND var_id = ? AND alarm_type = ? AND status = ?",
			event.Alarm.TaskID, event.Alarm.VarID, event.Alarm.AlarmType, models.DetectionAlarmStatusActive).
		Updates(updates).Error
}

func (r *Repository) ChangeDetectionLimitAlarmLevel(event *models.DetectionLimitAlarmEvent) error {
	if event == nil {
		return nil
	}
	now := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		recoveredAt := event.Alarm.FirstSeenAt
		if recoveredAt.IsZero() {
			recoveredAt = now
		}
		recoverValue := event.Alarm.StartValue
		updates := map[string]interface{}{
			"status":        models.DetectionAlarmStatusClosed,
			"recover_value": recoverValue,
			"recovered_at":  recoveredAt,
			"last_seen_at":  recoveredAt,
			"updated_at":    now,
			"message":       "level_change",
		}
		if event.Alarm.PeakValue != nil {
			updates["peak_value"] = event.Alarm.PeakValue
		}
		if err := tx.Model(&models.DetectionLimitAlarm{}).
			Where("task_id = ? AND var_id = ? AND alarm_type = ? AND status = ?",
				event.Alarm.TaskID, event.Alarm.VarID, event.PreviousAlarmType, models.DetectionAlarmStatusActive).
			Updates(updates).Error; err != nil {
			return err
		}
		alarm := event.Alarm
		alarm.CreatedAt = now
		alarm.UpdatedAt = now
		return tx.Create(&alarm).Error
	})
}

func (r *Repository) ListDetectionStandards(filter DetectionStandardFilter) ([]models.DetectionStandard, error) {
	var standards []models.DetectionStandard
	query := r.db.Model(&models.DetectionStandard{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.ProjectCode != "" {
		query = query.Where("project_code = ?", filter.ProjectCode)
	}
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Keyword != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		query = query.Where(
			"standard_code LIKE ? OR name LIKE ? OR display_name LIKE ? OR display_name_en LIKE ? OR display_name_ja LIKE ?",
			keyword, keyword, keyword, keyword, keyword,
		)
	}
	err := query.Order("id asc").Find(&standards).Error
	return standards, err
}

func (r *Repository) GetDetectionStandard(id uint) (models.DetectionStandard, error) {
	standard, items, err := loadDetectionStandardWithItems(r.db, id)
	if err != nil {
		return models.DetectionStandard{}, err
	}
	standard.Items = items
	return standard, nil
}

func (r *Repository) ListFavoriteDetectionStandards(userID uint) ([]models.DetectionStandard, error) {
	var standards []models.DetectionStandard
	err := r.db.Model(&models.DetectionStandard{}).
		Joins("JOIN sys_detection_standard_favorites f ON f.standard_id = sys_detection_standards.id AND f.user_id = ?", userID).
		Order("f.updated_at desc, f.id desc").
		Find(&standards).Error
	return standards, err
}

func (r *Repository) SetDetectionStandardFavorite(userID uint, standardID uint, favorite bool) error {
	if userID == 0 {
		return fmt.Errorf("user_id is required")
	}
	if favorite {
		if err := r.db.First(&models.DetectionStandard{}, "id = ?", standardID).Error; err != nil {
			return err
		}
		now := time.Now()
		item := models.DetectionStandardFavorite{UserID: userID, StandardID: standardID, CreatedAt: now, UpdatedAt: now}
		return r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "standard_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"updated_at": now}),
		}).Create(&item).Error
	}
	return r.db.Delete(&models.DetectionStandardFavorite{}, "user_id = ? AND standard_id = ?", userID, standardID).Error
}

func (r *Repository) ListRecentDetectionStandards(userID uint, projectID *uint, limit int) ([]models.DetectionStandard, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query := r.db.Table("sys_detection_standards").
		Select("sys_detection_standards.*").
		Joins("JOIN sys_detection_standard_recents r ON r.standard_id = sys_detection_standards.id").
		Where("r.user_id IN ?", []uint{0, userID})
	if projectID != nil {
		query = query.Where("r.project_id = ?", *projectID)
	}
	var standards []models.DetectionStandard
	err := query.Order("r.last_used_at desc, r.id desc").Limit(limit).Find(&standards).Error
	return standards, err
}

func (r *Repository) CreateDetectionStandard(standard *models.DetectionStandard, items []models.DetectionStandardItem) error {
	now := time.Now()
	standard.CreatedAt = now
	standard.UpdatedAt = now
	if standard.Version == 0 {
		standard.Version = 1
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(standard).Error; err != nil {
			return err
		}
		for i := range items {
			applyDetectionStandardItemDefaults(&items[i])
			items[i].StandardID = standard.ID
			items[i].CreatedAt = now
			items[i].UpdatedAt = now
		}
		if len(items) > 0 {
			return tx.Create(&items).Error
		}
		return nil
	})
}

func (r *Repository) UpdateDetectionStandard(id uint, updates map[string]interface{}) (models.DetectionStandard, error) {
	if len(updates) == 0 {
		return r.GetDetectionStandard(id)
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.DetectionStandard{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return models.DetectionStandard{}, err
	}
	return r.GetDetectionStandard(id)
}

func (r *Repository) ReplaceDetectionStandardItems(standardID uint, items []models.DetectionStandardItem) (models.DetectionStandard, error) {
	now := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var standard models.DetectionStandard
		if err := tx.First(&standard, "id = ?", standardID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.DetectionStandardItem{}, "standard_id = ?", standardID).Error; err != nil {
			return err
		}
		for i := range items {
			applyDetectionStandardItemDefaults(&items[i])
			items[i].StandardID = standardID
			items[i].CreatedAt = now
			items[i].UpdatedAt = now
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.DetectionStandard{}).
			Where("id = ?", standardID).
			Updates(map[string]interface{}{
				"version":    gorm.Expr("version + 1"),
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return models.DetectionStandard{}, err
	}
	return r.GetDetectionStandard(standardID)
}

func (r *Repository) DeleteDetectionStandard(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var refs int64
		if err := tx.Model(&models.DetectionRunStandardItem{}).Where("standard_id = ?", id).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return ErrReferenced
		}
		if err := tx.Delete(&models.DetectionStandardItem{}, "standard_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&models.DetectionStandard{}, "id = ?", id).Error
	})
}

func loadDetectionStandardWithItems(db *gorm.DB, id uint) (models.DetectionStandard, []models.DetectionStandardItem, error) {
	var standard models.DetectionStandard
	if err := db.First(&standard, "id = ?", id).Error; err != nil {
		return standard, nil, err
	}
	var items []models.DetectionStandardItem
	if err := db.Where("standard_id = ?", id).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return standard, nil, err
	}
	return standard, items, nil
}

func loadTagsByVarID(db *gorm.DB, items []models.DetectionStandardItem) (map[int64]models.TagConfig, error) {
	varIDs := make([]int64, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item.VarID]; ok {
			continue
		}
		seen[item.VarID] = struct{}{}
		varIDs = append(varIDs, item.VarID)
	}
	if len(varIDs) == 0 {
		return map[int64]models.TagConfig{}, nil
	}
	var tags []models.TagConfig
	if err := db.Where("var_id IN ?", varIDs).Find(&tags).Error; err != nil {
		return nil, err
	}
	tagByVarID := make(map[int64]models.TagConfig, len(tags))
	for _, tag := range tags {
		applyTagPersistenceDefaults(&tag)
		tagByVarID[tag.VarID] = tag
	}
	return tagByVarID, nil
}

func makeRunStandardItems(task *models.DetectionTask, standard models.DetectionStandard, items []models.DetectionStandardItem, tagByVarID map[int64]models.TagConfig, now time.Time) []models.DetectionRunStandardItem {
	snapshotItems := make([]models.DetectionRunStandardItem, 0, len(items))
	for _, item := range items {
		applyDetectionStandardItemDefaults(&item)
		tag := tagByVarID[item.VarID]
		checkCycleMS := item.CheckCycleMS
		snapshotItems = append(snapshotItems, models.DetectionRunStandardItem{
			TaskID:                         task.ID,
			TestNo:                         task.TestNo,
			StandardID:                     standard.ID,
			StandardItemID:                 item.ID,
			VarID:                          item.VarID,
			VarName:                        item.VarName,
			DisplayName:                    item.DisplayName,
			DisplayNameEN:                  item.DisplayNameEN,
			DisplayNameJA:                  item.DisplayNameJA,
			CheckEnabled:                   item.CheckEnabled,
			AlarmEnabled:                   item.AlarmEnabled,
			StoreEnabled:                   item.StoreEnabled,
			CheckCycleMS:                   checkCycleMS,
			CheckOnStart:                   item.CheckOnStart,
			Required:                       item.Required,
			CheckMethod:                    item.CheckMethod,
			TargetValue:                    item.TargetValue,
			LimitLL:                        item.LimitLL,
			LimitL:                         item.LimitL,
			LimitH:                         item.LimitH,
			LimitHH:                        item.LimitHH,
			LimitDeadband:                  item.LimitDeadband,
			ViolationHoldMS:                item.ViolationHoldMS,
			RecoverHoldMS:                  item.RecoverHoldMS,
			QualityPolicy:                  item.QualityPolicy,
			VariableDefaultAlarmEnabled:    tag.DefaultAlarmEnabled,
			VariableDefaultLimitLL:         tag.DefaultLimitLL,
			VariableDefaultLimitL:          tag.DefaultLimitL,
			VariableDefaultLimitH:          tag.DefaultLimitH,
			VariableDefaultLimitHH:         tag.DefaultLimitHH,
			VariableDefaultLimitDeadband:   tag.DefaultLimitDeadband,
			VariableDefaultViolationHoldMS: tag.DefaultViolationHoldMS,
			VariableDefaultRecoverHoldMS:   tag.DefaultRecoverHoldMS,
			Unit:                           item.Unit,
			DecimalPlaces:                  item.DecimalPlaces,
			SortOrder:                      item.SortOrder,
			CreatedAt:                      now,
		})
	}
	return snapshotItems
}

func applyDetectionStandardItemDefaults(item *models.DetectionStandardItem) {
	if item.CheckMethod == "" {
		item.CheckMethod = models.CheckMethodNumericRange
	}
	if item.QualityPolicy == "" {
		item.QualityPolicy = models.QualityPolicyIgnoreBad
	}
	if item.DecimalPlaces == 0 {
		item.DecimalPlaces = 2
	}
}

func isValidDetectionEndPolicy(value string) bool {
	switch value {
	case models.DetectionEndPolicyManual, models.DetectionEndPolicyFixedDuration, models.DetectionEndPolicyQualifiedHold:
		return true
	default:
		return false
	}
}

func customDetectionConfigJSON(opts StartDetectionOptions) string {
	if len(opts.CustomItems) == 0 {
		return ""
	}
	items := make([]map[string]interface{}, 0, len(opts.CustomItems))
	for _, item := range opts.CustomItems {
		applyDetectionStandardItemDefaults(&item)
		compact := map[string]interface{}{
			"var_id":            item.VarID,
			"var_name":          item.VarName,
			"check_enabled":     item.CheckEnabled,
			"alarm_enabled":     item.AlarmEnabled,
			"store_enabled":     item.StoreEnabled,
			"check_on_start":    item.CheckOnStart,
			"check_method":      item.CheckMethod,
			"quality_policy":    item.QualityPolicy,
			"check_cycle_ms":    item.CheckCycleMS,
			"limit_deadband":    item.LimitDeadband,
			"violation_hold_ms": item.ViolationHoldMS,
			"recover_hold_ms":   item.RecoverHoldMS,
			"sort_order":        item.SortOrder,
		}
		setCompactString(compact, "display_name", item.DisplayName)
		setCompactString(compact, "display_name_en", item.DisplayNameEN)
		setCompactString(compact, "display_name_ja", item.DisplayNameJA)
		setCompactString(compact, "target_value", item.TargetValue)
		setCompactString(compact, "unit", item.Unit)
		setCompactFloat(compact, "limit_ll", item.LimitLL)
		setCompactFloat(compact, "limit_l", item.LimitL)
		setCompactFloat(compact, "limit_h", item.LimitH)
		setCompactFloat(compact, "limit_hh", item.LimitHH)
		items = append(items, compact)
	}
	out := map[string]interface{}{
		"source":       "custom_items",
		"item_count":   len(items),
		"custom_items": items,
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func setCompactString(out map[string]interface{}, key string, value string) {
	if strings.TrimSpace(value) != "" {
		out[key] = value
	}
}

func setCompactFloat(out map[string]interface{}, key string, value *float64) {
	if value != nil {
		out[key] = *value
	}
}

func validateDetectionStandardItems(items []models.DetectionStandardItem) error {
	seen := make(map[int64]struct{}, len(items))
	for i := range items {
		item := &items[i]
		applyDetectionStandardItemDefaults(item)
		if item.VarID == 0 {
			return fmt.Errorf("custom_items.var_id is required")
		}
		if strings.TrimSpace(item.VarName) == "" {
			return fmt.Errorf("custom_items.var_name is required")
		}
		if _, ok := seen[item.VarID]; ok {
			return fmt.Errorf("custom_items contains duplicate var_id %d", item.VarID)
		}
		seen[item.VarID] = struct{}{}
		if item.CheckCycleMS < 0 {
			return fmt.Errorf("check_cycle_ms must be non-negative")
		}
		if item.LimitDeadband < 0 {
			return fmt.Errorf("limit_deadband must be non-negative")
		}
		if item.ViolationHoldMS < 0 || item.RecoverHoldMS < 0 {
			return fmt.Errorf("hold times must be non-negative")
		}
		if err := validateLimitOrder(item.LimitLL, item.LimitL, item.LimitH, item.LimitHH); err != nil {
			return err
		}
		if !validCheckMethod(item.CheckMethod) {
			return fmt.Errorf("invalid check_method")
		}
		if !validQualityPolicy(item.QualityPolicy) {
			return fmt.Errorf("invalid quality_policy")
		}
	}
	return nil
}

func validateLimitOrder(ll *float64, l *float64, h *float64, hh *float64) error {
	if ll != nil && l != nil && *ll > *l {
		return fmt.Errorf("limit_ll must be less than or equal to limit_l")
	}
	if l != nil && h != nil && *l > *h {
		return fmt.Errorf("limit_l must be less than or equal to limit_h")
	}
	if h != nil && hh != nil && *h > *hh {
		return fmt.Errorf("limit_h must be less than or equal to limit_hh")
	}
	return nil
}

func validCheckMethod(value string) bool {
	switch value {
	case models.CheckMethodNumericRange, models.CheckMethodBoolEquals, models.CheckMethodStringEquals, models.CheckMethodRegex:
		return true
	default:
		return false
	}
}

func validQualityPolicy(value string) bool {
	switch value {
	case models.QualityPolicyIgnoreBad, models.QualityPolicyRecordInvalid, models.QualityPolicyFailOnBad:
		return true
	default:
		return false
	}
}

func upsertDetectionStandardRecent(tx *gorm.DB, userID uint, standardID uint, projectID uint, now time.Time) error {
	if standardID == 0 {
		return nil
	}
	recent := models.DetectionStandardRecent{
		UserID:     userID,
		StandardID: standardID,
		ProjectID:  projectID,
		LastUsedAt: now,
		UseCount:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "standard_id"}, {Name: "project_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"last_used_at": now,
			"use_count":    gorm.Expr("use_count + 1"),
			"updated_at":   now,
		}),
	}).Create(&recent).Error
}

func (r *Repository) attachRunStandardItems(tasks []models.DetectionTask) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]uint, 0, len(tasks))
	indexByID := make(map[uint]int, len(tasks))
	for i, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		indexByID[task.ID] = i
	}
	var items []models.DetectionRunStandardItem
	if err := r.db.Where("task_id IN ?", taskIDs).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		if idx, ok := indexByID[item.TaskID]; ok {
			tasks[idx].StandardItems = append(tasks[idx].StandardItems, item)
		}
	}
	return nil
}

func (r *Repository) attachRunStorageRoutes(tasks []models.DetectionTask) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]uint, 0, len(tasks))
	indexByID := make(map[uint]int, len(tasks))
	for i, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		indexByID[task.ID] = i
	}
	var routes []models.DetectionRunStorageRoute
	if err := r.db.Where("task_id IN ?", taskIDs).Order("var_id asc, id asc").Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if idx, ok := indexByID[route.TaskID]; ok {
			tasks[idx].StorageRoutes = append(tasks[idx].StorageRoutes, route)
		}
	}
	return nil
}
