package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	var reportRequests []models.DetectionRunReportRequest

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
		reportRequests, err = buildDetectionRunReportRequests(tx, task, opts.ReportRequest, snapshotItems)
		if err != nil {
			return err
		}
		if len(reportRequests) > 0 {
			if err := NewRepository(tx).CreateDetectionRunReportRequests(reportRequests); err != nil {
				return err
			}
		}
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
	task.ReportRequests = reportRequests
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
	task.ReportRequests, _ = r.ListDetectionRunReportRequests(id)
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
	alarm.Scope = normalizedAlarmScope(alarm.Scope)
	alarm.CreatedAt = now
	alarm.UpdatedAt = now
	return r.db.Create(alarm).Error
}

func (r *Repository) ListLimitAlarms(filter LimitAlarmFilter) ([]models.DetectionLimitAlarm, int64, error) {
	query := r.db.Model(&models.DetectionLimitAlarm{})
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.TaskID != nil {
		query = query.Where("task_id = ?", *filter.TaskID)
	}
	if filter.TestNo != "" {
		query = query.Where("test_no = ?", filter.TestNo)
	}
	if filter.VarID != nil {
		query = query.Where("var_id = ?", *filter.VarID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.AlarmType != "" {
		query = query.Where("alarm_type = ?", filter.AlarmType)
	}
	if filter.AlarmLevel != "" {
		query = query.Where("alarm_level = ?", filter.AlarmLevel)
	}
	if filter.From != nil {
		query = query.Where("first_seen_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("first_seen_at <= ?", *filter.To)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := normalizedLimitAlarmLimit(filter.Limit)
	offset := normalizedLimitAlarmOffset(filter.Offset)
	var alarms []models.DetectionLimitAlarm
	if err := query.Order("last_seen_at DESC, id DESC").Limit(limit).Offset(offset).Find(&alarms).Error; err != nil {
		return nil, 0, err
	}
	return alarms, total, nil
}

func (r *Repository) CreateDetectionLimitAlarms(alarms []models.DetectionLimitAlarm) error {
	if len(alarms) == 0 {
		return nil
	}
	now := time.Now()
	for i := range alarms {
		alarms[i].Scope = normalizedAlarmScope(alarms[i].Scope)
		alarms[i].CreatedAt = now
		alarms[i].UpdatedAt = now
	}
	return r.db.CreateInBatches(&alarms, 500).Error
}

func (r *Repository) RecoverDetectionLimitAlarm(event *models.DetectionLimitAlarmEvent) error {
	now := time.Now()
	scope := normalizedAlarmScope(event.Alarm.Scope)
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
		Where("scope = ? AND task_id = ? AND var_id = ? AND alarm_type = ? AND status = ?",
			scope, event.Alarm.TaskID, event.Alarm.VarID, event.Alarm.AlarmType, models.DetectionAlarmStatusActive).
		Updates(updates).Error
}

func (r *Repository) ChangeDetectionLimitAlarmLevel(event *models.DetectionLimitAlarmEvent) error {
	if event == nil {
		return nil
	}
	now := time.Now()
	scope := normalizedAlarmScope(event.Alarm.Scope)
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
			Where("scope = ? AND task_id = ? AND var_id = ? AND alarm_type = ? AND status = ?",
				scope, event.Alarm.TaskID, event.Alarm.VarID, event.PreviousAlarmType, models.DetectionAlarmStatusActive).
			Updates(updates).Error; err != nil {
			return err
		}
		alarm := event.Alarm
		alarm.Scope = scope
		alarm.CreatedAt = now
		alarm.UpdatedAt = now
		return tx.Create(&alarm).Error
	})
}

func normalizedAlarmScope(scope string) string {
	if scope == models.AlarmScopeDefault {
		return models.AlarmScopeDefault
	}
	return models.AlarmScopeDetection
}

func normalizedLimitAlarmLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func normalizedLimitAlarmOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
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
	if err := r.db.First(&models.DetectionStandard{}, "id = ?", standardID).Error; err != nil {
		return err
	}
	if favorite {
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
	if err := validateDetectionStandardItems(items); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(standard).Error; err != nil {
			return err
		}
		if err := hydrateDetectionStandardItemDisplayFields(tx, items); err != nil {
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
	if err := validateDetectionStandardItems(items); err != nil {
		return models.DetectionStandard{}, err
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var standard models.DetectionStandard
		if err := tx.First(&standard, "id = ?", standardID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.DetectionStandardItem{}, "standard_id = ?", standardID).Error; err != nil {
			return err
		}
		if err := hydrateDetectionStandardItemDisplayFields(tx, items); err != nil {
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
		if err := tx.First(&models.DetectionStandard{}, "id = ?", id).Error; err != nil {
			return err
		}
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

func hydrateDetectionStandardItemDisplayFields(db *gorm.DB, items []models.DetectionStandardItem) error {
	varIDs := make([]int64, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.VarID == 0 {
			continue
		}
		if _, ok := seen[item.VarID]; ok {
			continue
		}
		seen[item.VarID] = struct{}{}
		varIDs = append(varIDs, item.VarID)
	}
	if len(varIDs) == 0 {
		return nil
	}
	var tags []models.TagConfig
	if err := db.Where("var_id IN ?", varIDs).Find(&tags).Error; err != nil {
		return err
	}
	tagByVarID := make(map[int64]models.TagConfig, len(tags))
	for _, tag := range tags {
		tagByVarID[tag.VarID] = tag
	}
	for i := range items {
		tag, ok := tagByVarID[items[i].VarID]
		if !ok {
			continue
		}
		fillDetectionStandardItemDisplayFields(&items[i], tag)
	}
	return nil
}

func fillDetectionStandardItemDisplayFields(item *models.DetectionStandardItem, tag models.TagConfig) {
	if strings.TrimSpace(item.DisplayName) == "" {
		item.DisplayName = tag.DisplayName
	}
	if strings.TrimSpace(item.DisplayNameEN) == "" {
		item.DisplayNameEN = tag.DisplayNameEN
	}
	if strings.TrimSpace(item.DisplayNameJA) == "" {
		item.DisplayNameJA = tag.DisplayNameJA
	}
	if strings.TrimSpace(item.Unit) == "" {
		item.Unit = tag.Unit
	}
	if item.DecimalPlaces == 0 && tag.DecimalPlaces > 0 {
		item.DecimalPlaces = tag.DecimalPlaces
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

type reportRequestSpec struct {
	Enabled         *bool
	Reports         []reportRequestReportSpec
	TemplateID      *uint
	TemplateCode    string
	TemplateVersion int
	ReportName      string
	Status          string
	ParamsJSON      string
	Ext1            string
	Ext2            string
	Ext3            string
}

type reportRequestReportSpec struct {
	TemplateID      *uint
	TemplateCode    string
	TemplateVersion int
	ReportName      string
	Status          string
	Variables       []reportRequestVariableSpec
	ParamsJSON      string
	Ext1            string
	Ext2            string
	Ext3            string
}

type reportRequestVariableSpec struct {
	VarID         int64
	VarName       string
	DisplayName   string
	DisplayNameEN string
	DisplayNameJA string
	ReportName    string
	Status        string
	Ext1          string
	Ext2          string
	Ext3          string
}

func buildDetectionRunReportRequests(db *gorm.DB, task *models.DetectionTask, raw any, snapshotItems []models.DetectionRunStandardItem) ([]models.DetectionRunReportRequest, error) {
	spec, err := parseReportRequestSpec(raw)
	if err != nil {
		return nil, err
	}
	if spec.Enabled != nil && !*spec.Enabled {
		return nil, nil
	}
	if len(spec.Reports) == 0 {
		return nil, nil
	}
	byID := make(map[int64]models.DetectionRunStandardItem, len(snapshotItems))
	byName := make(map[string]models.DetectionRunStandardItem, len(snapshotItems))
	for _, item := range snapshotItems {
		byID[item.VarID] = item
		if strings.TrimSpace(item.VarName) != "" {
			byName[item.VarName] = item
		}
	}
	allVariables := make([]reportRequestVariableSpec, 0)
	for _, report := range spec.Reports {
		allVariables = append(allVariables, report.Variables...)
	}
	tagByID, tagByName, err := loadReportRequestTags(db, task.ProjectID, allVariables)
	if err != nil {
		return nil, err
	}
	out := make([]models.DetectionRunReportRequest, 0, len(spec.Reports))
	for _, report := range spec.Reports {
		if len(report.Variables) == 0 {
			return nil, fmt.Errorf("report_request reports require at least one variable")
		}
		variables := make([]reportRequestVariableSpec, 0, len(report.Variables))
		seen := make(map[string]struct{}, len(report.Variables))
		for _, variable := range report.Variables {
			resolved, err := resolveReportRequestVariable(variable, byID, byName, tagByID, tagByName)
			if err != nil {
				return nil, err
			}
			key := reportRequestDedupeKey(resolved)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			variables = append(variables, resolved)
		}
		if len(variables) == 0 {
			return nil, fmt.Errorf("report_request reports require at least one variable")
		}
		templateID, templateCode, templateVersion, err := resolveReportRequestTemplate(db, task, report)
		if err != nil {
			return nil, err
		}
		variablesJSON, err := reportVariablesJSON(variables)
		if err != nil {
			return nil, err
		}
		paramsJSON := firstNonEmpty(report.ParamsJSON, "{}")
		if _, err := normalizeJSONObject(paramsJSON, "report_request.params"); err != nil {
			return nil, err
		}
		primary := variables[0]
		status := firstNonEmpty(primary.Status, report.Status, spec.Status, "pending")
		out = append(out, models.DetectionRunReportRequest{
			TaskID:          task.ID,
			TestNo:          task.TestNo,
			ProjectID:       task.ProjectID,
			ProjectCode:     task.ProjectCode,
			TemplateID:      templateID,
			TemplateCode:    templateCode,
			TemplateVersion: templateVersion,
			VarID:           primary.VarID,
			VarName:         primary.VarName,
			DisplayName:     primary.DisplayName,
			DisplayNameEN:   primary.DisplayNameEN,
			DisplayNameJA:   primary.DisplayNameJA,
			ReportName:      firstNonEmpty(primary.ReportName, report.ReportName, spec.ReportName),
			VariablesJSON:   variablesJSON,
			ParamsJSON:      paramsJSON,
			Status:          status,
			Ext1:            firstNonEmpty(primary.Ext1, report.Ext1, spec.Ext1),
			Ext2:            firstNonEmpty(primary.Ext2, report.Ext2, spec.Ext2),
			Ext3:            firstNonEmpty(primary.Ext3, report.Ext3, spec.Ext3),
		})
	}
	return out, nil
}

func parseReportRequestSpec(raw any) (reportRequestSpec, error) {
	var spec reportRequestSpec
	if raw == nil {
		return spec, nil
	}
	var decoded map[string]any
	switch typed := raw.(type) {
	case map[string]any:
		decoded = typed
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return spec, nil
		}
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return spec, fmt.Errorf("report_request is invalid JSON")
		}
	default:
		buf, err := json.Marshal(raw)
		if err != nil {
			return spec, fmt.Errorf("report_request is invalid")
		}
		if err := json.Unmarshal(buf, &decoded); err != nil {
			return spec, fmt.Errorf("report_request is invalid")
		}
	}
	if decoded == nil {
		return spec, nil
	}
	if rawEnabled, ok := decoded["enabled"]; ok {
		value := boolFromReportAny(rawEnabled, true)
		spec.Enabled = &value
	}
	spec.TemplateID = uintPtrFromReportAny(firstMapValue(decoded, "template_id", "report_template_id"))
	spec.TemplateCode = reportString(firstMapValue(decoded, "template_code", "report_template_code"))
	spec.TemplateVersion = int(reportInt64(firstMapValue(decoded, "template_version", "report_template_version")))
	spec.ReportName = reportString(decoded["report_name"])
	spec.Status = reportString(decoded["status"])
	paramsJSON, err := reportParamsJSON(firstMapValue(decoded, "params", "params_json"))
	if err != nil {
		return spec, err
	}
	spec.ParamsJSON = paramsJSON
	spec.Ext1 = reportString(firstMapValue(decoded, "ext_1", "ext1"))
	spec.Ext2 = reportString(firstMapValue(decoded, "ext_2", "ext2"))
	spec.Ext3 = reportString(firstMapValue(decoded, "ext_3", "ext3"))
	reports, err := reportRequestsFromAny(firstMapValue(decoded, "reports", "report_requests"), spec)
	if err != nil {
		return spec, err
	}
	spec.Reports = append(spec.Reports, reports...)
	if len(spec.Reports) == 0 {
		variables := make([]reportRequestVariableSpec, 0)
		variables = append(variables, reportVariablesFromAny(decoded["variables"])...)
		variables = append(variables, reportVariablesFromIDs(decoded["var_ids"])...)
		variables = append(variables, reportVariablesFromNames(decoded["variable_names"])...)
		if len(variables) == 0 && (decoded["var_id"] != nil || decoded["var_name"] != nil) {
			variables = append(variables, reportVariableFromMap(decoded))
		}
		for _, variable := range variables {
			spec.Reports = append(spec.Reports, reportRequestReportSpec{
				TemplateID:      spec.TemplateID,
				TemplateCode:    spec.TemplateCode,
				TemplateVersion: spec.TemplateVersion,
				ReportName:      spec.ReportName,
				Status:          spec.Status,
				Variables:       []reportRequestVariableSpec{variable},
				ParamsJSON:      firstNonEmpty(spec.ParamsJSON, "{}"),
				Ext1:            spec.Ext1,
				Ext2:            spec.Ext2,
				Ext3:            spec.Ext3,
			})
		}
	}
	return spec, nil
}

func reportRequestsFromAny(raw any, parent reportRequestSpec) ([]reportRequestReportSpec, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]reportRequestReportSpec, 0, len(items))
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("report_request.reports must contain objects")
		}
		paramsJSON, err := reportParamsJSON(firstMapValue(value, "params", "params_json"))
		if err != nil {
			return nil, err
		}
		variables := make([]reportRequestVariableSpec, 0)
		variables = append(variables, reportVariablesFromAny(value["variables"])...)
		variables = append(variables, reportVariablesFromIDs(value["var_ids"])...)
		variables = append(variables, reportVariablesFromNames(value["variable_names"])...)
		if len(variables) == 0 && (value["var_id"] != nil || value["var_name"] != nil) {
			variables = append(variables, reportVariableFromMap(value))
		}
		out = append(out, reportRequestReportSpec{
			TemplateID:      firstUintPtr(uintPtrFromReportAny(firstMapValue(value, "template_id", "report_template_id")), parent.TemplateID),
			TemplateCode:    firstNonEmpty(reportString(firstMapValue(value, "template_code", "report_template_code")), parent.TemplateCode),
			TemplateVersion: firstPositiveInt(int(reportInt64(firstMapValue(value, "template_version", "report_template_version"))), parent.TemplateVersion),
			ReportName:      firstNonEmpty(reportString(value["report_name"]), parent.ReportName),
			Status:          firstNonEmpty(reportString(value["status"]), parent.Status),
			Variables:       variables,
			ParamsJSON:      firstNonEmpty(paramsJSON, parent.ParamsJSON, "{}"),
			Ext1:            firstNonEmpty(reportString(firstMapValue(value, "ext_1", "ext1")), parent.Ext1),
			Ext2:            firstNonEmpty(reportString(firstMapValue(value, "ext_2", "ext2")), parent.Ext2),
			Ext3:            firstNonEmpty(reportString(firstMapValue(value, "ext_3", "ext3")), parent.Ext3),
		})
	}
	return out, nil
}

func reportVariablesFromAny(raw any) []reportRequestVariableSpec {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]reportRequestVariableSpec, 0, len(items))
	for _, item := range items {
		if value, ok := item.(map[string]any); ok {
			out = append(out, reportVariableFromMap(value))
		} else {
			text := reportString(item)
			if text != "" {
				out = append(out, reportRequestVariableSpec{VarName: text})
			}
		}
	}
	return out
}

func reportVariablesFromIDs(raw any) []reportRequestVariableSpec {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]reportRequestVariableSpec, 0, len(items))
	for _, item := range items {
		if id := reportInt64(item); id != 0 {
			out = append(out, reportRequestVariableSpec{VarID: id})
		}
	}
	return out
}

func reportVariablesFromNames(raw any) []reportRequestVariableSpec {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]reportRequestVariableSpec, 0, len(items))
	for _, item := range items {
		if name := reportString(item); name != "" {
			out = append(out, reportRequestVariableSpec{VarName: name})
		}
	}
	return out
}

func reportVariableFromMap(value map[string]any) reportRequestVariableSpec {
	return reportRequestVariableSpec{
		VarID:         reportInt64(firstMapValue(value, "var_id", "var_id_text")),
		VarName:       reportString(value["var_name"]),
		DisplayName:   reportString(value["display_name"]),
		DisplayNameEN: reportString(value["display_name_en"]),
		DisplayNameJA: reportString(value["display_name_ja"]),
		ReportName:    reportString(value["report_name"]),
		Status:        reportString(value["status"]),
		Ext1:          reportString(firstMapValue(value, "ext_1", "ext1")),
		Ext2:          reportString(firstMapValue(value, "ext_2", "ext2")),
		Ext3:          reportString(firstMapValue(value, "ext_3", "ext3")),
	}
}

func resolveReportRequestVariable(variable reportRequestVariableSpec, byID map[int64]models.DetectionRunStandardItem, byName map[string]models.DetectionRunStandardItem, tagByID map[int64]models.TagConfig, tagByName map[string]models.TagConfig) (reportRequestVariableSpec, error) {
	if variable.VarID == 0 && strings.TrimSpace(variable.VarName) == "" {
		return variable, fmt.Errorf("report_request variables require var_id or var_name")
	}
	if variable.VarID == 0 {
		if item, ok := byName[variable.VarName]; ok {
			variable.VarID = item.VarID
		} else if tag, ok := tagByName[variable.VarName]; ok {
			variable.VarID = tag.VarID
		}
	}
	if item, ok := byID[variable.VarID]; ok {
		fillReportRequestVariableFromRunItem(&variable, item)
	}
	if tag, ok := tagByID[variable.VarID]; ok {
		fillReportRequestVariableFromTag(&variable, tag)
	} else if tag, ok := tagByName[variable.VarName]; ok {
		fillReportRequestVariableFromTag(&variable, tag)
	}
	variable.VarName = strings.TrimSpace(variable.VarName)
	if variable.VarName == "" {
		return variable, fmt.Errorf("report_request variable name is required for var_id %d", variable.VarID)
	}
	return variable, nil
}

func resolveReportRequestTemplate(db *gorm.DB, task *models.DetectionTask, report reportRequestReportSpec) (*uint, string, int, error) {
	if report.TemplateID != nil && *report.TemplateID > 0 {
		template, err := getReportTemplate(db, *report.TemplateID)
		if err != nil {
			return nil, "", 0, err
		}
		return &template.ID, template.TemplateCode, template.Version, nil
	}
	if strings.TrimSpace(report.TemplateCode) != "" {
		template, err := getReportTemplateByCode(db, report.TemplateCode)
		if err == nil {
			return &template.ID, template.TemplateCode, template.Version, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", 0, err
		}
		return nil, strings.TrimSpace(report.TemplateCode), report.TemplateVersion, nil
	}
	if task.ReportTemplateID != nil {
		return task.ReportTemplateID, task.ReportTemplateCode, task.ReportTemplateVersion, nil
	}
	return nil, "", 0, nil
}

func reportVariablesJSON(variables []reportRequestVariableSpec) (string, error) {
	items := make([]map[string]any, 0, len(variables))
	for _, variable := range variables {
		item := map[string]any{
			"var_id":      variable.VarID,
			"var_id_text": strconv.FormatInt(variable.VarID, 10),
			"var_name":    variable.VarName,
		}
		setCompactString(item, "display_name", variable.DisplayName)
		setCompactString(item, "display_name_en", variable.DisplayNameEN)
		setCompactString(item, "display_name_ja", variable.DisplayNameJA)
		items = append(items, item)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func loadReportRequestTags(db *gorm.DB, projectID uint, variables []reportRequestVariableSpec) (map[int64]models.TagConfig, map[string]models.TagConfig, error) {
	ids := make([]int64, 0, len(variables))
	names := make([]string, 0, len(variables))
	seenIDs := map[int64]struct{}{}
	seenNames := map[string]struct{}{}
	for _, variable := range variables {
		if variable.VarID != 0 {
			if _, ok := seenIDs[variable.VarID]; !ok {
				seenIDs[variable.VarID] = struct{}{}
				ids = append(ids, variable.VarID)
			}
		}
		name := strings.TrimSpace(variable.VarName)
		if name != "" {
			if _, ok := seenNames[name]; !ok {
				seenNames[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	if len(ids) == 0 && len(names) == 0 {
		return map[int64]models.TagConfig{}, map[string]models.TagConfig{}, nil
	}
	query := db.Model(&models.TagConfig{})
	switch {
	case len(ids) > 0 && len(names) > 0:
		query = query.Where("var_id IN ? OR (project_id = ? AND var_name IN ?)", ids, projectID, names)
	case len(ids) > 0:
		query = query.Where("var_id IN ?", ids)
	default:
		query = query.Where("project_id = ? AND var_name IN ?", projectID, names)
	}
	var tags []models.TagConfig
	if err := query.Find(&tags).Error; err != nil {
		return nil, nil, err
	}
	byID := make(map[int64]models.TagConfig, len(tags))
	byName := make(map[string]models.TagConfig, len(tags))
	for _, tag := range tags {
		byID[tag.VarID] = tag
		if strings.TrimSpace(tag.VarName) != "" {
			byName[tag.VarName] = tag
		}
	}
	return byID, byName, nil
}

func fillReportRequestVariableFromRunItem(variable *reportRequestVariableSpec, item models.DetectionRunStandardItem) {
	if variable.VarID == 0 {
		variable.VarID = item.VarID
	}
	if strings.TrimSpace(variable.VarName) == "" {
		variable.VarName = item.VarName
	}
	if strings.TrimSpace(variable.DisplayName) == "" {
		variable.DisplayName = item.DisplayName
	}
	if strings.TrimSpace(variable.DisplayNameEN) == "" {
		variable.DisplayNameEN = item.DisplayNameEN
	}
	if strings.TrimSpace(variable.DisplayNameJA) == "" {
		variable.DisplayNameJA = item.DisplayNameJA
	}
}

func fillReportRequestVariableFromTag(variable *reportRequestVariableSpec, tag models.TagConfig) {
	if variable.VarID == 0 {
		variable.VarID = tag.VarID
	}
	if strings.TrimSpace(variable.VarName) == "" {
		variable.VarName = tag.VarName
	}
	if strings.TrimSpace(variable.DisplayName) == "" {
		variable.DisplayName = tag.DisplayName
	}
	if strings.TrimSpace(variable.DisplayNameEN) == "" {
		variable.DisplayNameEN = tag.DisplayNameEN
	}
	if strings.TrimSpace(variable.DisplayNameJA) == "" {
		variable.DisplayNameJA = tag.DisplayNameJA
	}
}

func reportRequestDedupeKey(variable reportRequestVariableSpec) string {
	if variable.VarID != 0 {
		return fmt.Sprintf("id:%d", variable.VarID)
	}
	return "name:" + variable.VarName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstMapValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func boolFromReportAny(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	default:
		return reportInt64(value) != 0
	}
}

func reportString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func reportInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case uint:
		return int64(typed)
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(typed)
	case uint32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return value
		}
		return 0
	case string:
		value, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return value
		}
		return 0
	default:
		return 0
	}
}

func uintPtrFromReportAny(value any) *uint {
	parsed := reportInt64(value)
	if parsed <= 0 {
		return nil
	}
	out := uint(parsed)
	return &out
}

func firstUintPtr(values ...*uint) *uint {
	for _, value := range values {
		if value != nil && *value > 0 {
			return value
		}
	}
	return nil
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func reportParamsJSON(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	switch typed := raw.(type) {
	case string:
		return normalizeJSONObject(typed, "report_request.params")
	default:
		buf, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("report_request.params is invalid")
		}
		return normalizeJSONObject(string(buf), "report_request.params")
	}
}

func normalizeJSONObject(value string, field string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", fmt.Errorf("%s is invalid JSON", field)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "", fmt.Errorf("%s must be a JSON object", field)
	}
	raw, _ := json.Marshal(decoded)
	return string(raw), nil
}

func customDetectionConfigJSON(opts StartDetectionOptions) string {
	if len(opts.CustomItems) == 0 && opts.ProcessParams == nil && opts.PLCWrites == nil && opts.ReportRequest == nil {
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
		"source": "task_params",
	}
	if len(items) > 0 {
		out["item_count"] = len(items)
		out["custom_items"] = items
	}
	if opts.ProcessParams != nil {
		out["process_params"] = opts.ProcessParams
	}
	if opts.PLCWrites != nil {
		out["plc_writes"] = opts.PLCWrites
	}
	if opts.ReportRequest != nil {
		out["report_request"] = opts.ReportRequest
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
