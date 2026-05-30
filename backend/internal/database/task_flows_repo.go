package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

type TaskFlowFilter struct {
	ProjectID   *uint
	TriggerType string
	Enabled     *bool
}

type TaskFlowRunFilter struct {
	ProjectID    *uint
	FlowID       *uint64
	FlowCode     string
	Status       string
	TriggerType  string
	TriggerVarID *int64
	OriginFlowID *uint64
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}

func (r *Repository) LoadEnabledTaskFlows() ([]models.TaskFlow, error) {
	return r.ListTaskFlows(TaskFlowFilter{Enabled: boolPtr(true)})
}

func (r *Repository) ListTaskFlows(filter TaskFlowFilter) ([]models.TaskFlow, error) {
	var flows []models.TaskFlow
	query := r.db.Preload("Vars").Model(&models.TaskFlow{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.TriggerType != "" {
		query = query.Where("trigger_type = ?", filter.TriggerType)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	err := query.Order("project_id asc, priority desc, id asc").Find(&flows).Error
	return flows, err
}

func (r *Repository) GetTaskFlow(id uint64) (models.TaskFlow, error) {
	var flow models.TaskFlow
	err := r.db.Preload("Vars").First(&flow, "id = ?", id).Error
	return flow, err
}

func (r *Repository) CreateTaskFlow(flow *models.TaskFlow) error {
	normalizeTaskFlow(flow)
	now := time.Now()
	flow.CreatedAt = now
	flow.UpdatedAt = now
	for i := range flow.Vars {
		flow.Vars[i].ProjectID = flow.ProjectID
		flow.Vars[i].CreatedAt = now
		if strings.TrimSpace(flow.Vars[i].Role) == "" {
			flow.Vars[i].Role = models.TaskFlowVarRoleWatch
		}
	}
	return r.db.Create(flow).Error
}

func (r *Repository) UpdateTaskFlow(id uint64, updates map[string]any, vars []models.TaskFlowVar, replaceVars bool) (models.TaskFlow, error) {
	var flow models.TaskFlow
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&flow, "id = ?", id).Error; err != nil {
			return err
		}
		updates["updated_at"] = time.Now()
		if err := tx.Model(&models.TaskFlow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if replaceVars {
			if err := tx.Delete(&models.TaskFlowVar{}, "flow_id = ?", id).Error; err != nil {
				return err
			}
			now := time.Now()
			projectID := flow.ProjectID
			if rawProjectID, ok := updates["project_id"]; ok {
				switch value := rawProjectID.(type) {
				case uint:
					projectID = value
				case int:
					if value > 0 {
						projectID = uint(value)
					}
				case uint64:
					projectID = uint(value)
				}
			}
			for i := range vars {
				vars[i].FlowID = id
				vars[i].ProjectID = projectID
				vars[i].CreatedAt = now
				if strings.TrimSpace(vars[i].Role) == "" {
					vars[i].Role = models.TaskFlowVarRoleWatch
				}
			}
			if len(vars) > 0 {
				if err := tx.Create(&vars).Error; err != nil {
					return err
				}
			}
		}
		return tx.Preload("Vars").First(&flow, "id = ?", id).Error
	})
	return flow, err
}

func (r *Repository) DeleteTaskFlow(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var flow models.TaskFlow
		if err := tx.First(&flow, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.TaskFlowVar{}, "flow_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&models.TaskFlow{}, "id = ?", id).Error
	})
}

func (r *Repository) CreateTaskFlowRun(run *models.TaskFlowRun) error {
	now := time.Now()
	run.CreatedAt = now
	run.UpdatedAt = now
	return r.db.Create(run).Error
}

func (r *Repository) FinishTaskFlowRun(id uint64, status string, result string, errMessage string, logs string, finishedAt time.Time) error {
	var run models.TaskFlowRun
	if err := r.db.First(&run, "id = ?", id).Error; err != nil {
		return err
	}
	duration := finishedAt.Sub(run.StartedAt).Milliseconds()
	return r.db.Model(&models.TaskFlowRun{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"finished_at":   finishedAt,
		"duration_ms":   duration,
		"result_json":   result,
		"error_message": errMessage,
		"script_logs":   logs,
		"updated_at":    finishedAt,
	}).Error
}

func (r *Repository) ListTaskFlowRuns(filter TaskFlowRunFilter) ([]models.TaskFlowRun, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.db.Model(&models.TaskFlowRun{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.FlowID != nil {
		query = query.Where("flow_id = ?", *filter.FlowID)
	}
	if filter.FlowCode != "" {
		query = query.Where("flow_code = ?", filter.FlowCode)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.TriggerType != "" {
		query = query.Where("trigger_type = ?", filter.TriggerType)
	}
	if filter.TriggerVarID != nil {
		query = query.Where("trigger_var_id = ?", *filter.TriggerVarID)
	}
	if filter.OriginFlowID != nil {
		query = query.Where("origin_flow_id = ?", *filter.OriginFlowID)
	}
	if filter.From != nil {
		query = query.Where("started_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("started_at <= ?", *filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []models.TaskFlowRun
	err := query.Order("started_at desc, id desc").Limit(limit).Offset(offset).Find(&runs).Error
	return runs, total, err
}

func (r *Repository) GetTaskFlowRun(id uint64) (models.TaskFlowRun, error) {
	var run models.TaskFlowRun
	err := r.db.First(&run, "id = ?", id).Error
	return run, err
}

func (r *Repository) ListTaskFlowSQLLogs(runID uint64, limit int) ([]models.TaskFlowSQLLog, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var logs []models.TaskFlowSQLLog
	err := r.db.Where("run_id = ?", runID).Order("created_at asc, id asc").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *Repository) ExecTaskFlowSQL(ctx context.Context, runID uint64, flowID uint64, sqlText string, args []any) (int64, error) {
	start := time.Now()
	result := r.db.WithContext(ctx).Exec(sqlText, args...)
	affected := result.RowsAffected
	errMessage := ""
	if result.Error != nil {
		errMessage = result.Error.Error()
	}
	_ = r.createTaskFlowSQLLog(runID, flowID, sqlText, args, affected, time.Since(start), errMessage)
	return affected, result.Error
}

func (r *Repository) QueryTaskFlowSQL(ctx context.Context, runID uint64, flowID uint64, sqlText string, args []any, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	start := time.Now()
	rows, err := r.db.WithContext(ctx).Raw(sqlText, args...).Rows()
	errMessage := ""
	if err != nil {
		errMessage = err.Error()
		_ = r.createTaskFlowSQLLog(runID, flowID, sqlText, args, 0, time.Since(start), errMessage)
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	result, scanErr := scanRowsToMaps(rows, limit)
	if scanErr != nil {
		errMessage = scanErr.Error()
	}
	_ = r.createTaskFlowSQLLog(runID, flowID, sqlText, args, int64(len(result)), time.Since(start), errMessage)
	return result, scanErr
}

func (r *Repository) createTaskFlowSQLLog(runID uint64, flowID uint64, sqlText string, args []any, affected int64, duration time.Duration, errMessage string) error {
	rawArgs, _ := json.Marshal(args)
	return r.db.Create(&models.TaskFlowSQLLog{
		RunID:        runID,
		FlowID:       flowID,
		SQLText:      sqlText,
		SQLArgs:      string(rawArgs),
		AffectedRows: affected,
		DurationMS:   duration.Milliseconds(),
		ErrorMessage: errMessage,
		CreatedAt:    time.Now(),
	}).Error
}

func scanRowsToMaps(rows *sql.Rows, limit int) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return out, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			switch v := values[i].(type) {
			case []byte:
				row[column] = string(v)
			default:
				row[column] = v
			}
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func normalizeTaskFlow(flow *models.TaskFlow) {
	flow.FlowCode = strings.TrimSpace(flow.FlowCode)
	flow.Name = strings.TrimSpace(flow.Name)
	flow.TriggerType = strings.ToLower(strings.TrimSpace(flow.TriggerType))
	if flow.TriggerType == "" {
		flow.TriggerType = models.TaskFlowTriggerDataChange
	}
	flow.ActionType = strings.ToLower(strings.TrimSpace(flow.ActionType))
	if flow.ActionType == "" {
		flow.ActionType = models.TaskFlowActionBuiltinStorageSnapshot
	}
	if flow.TimeoutMS <= 0 {
		flow.TimeoutMS = 3000
	}
	if flow.CooldownMS < 0 {
		flow.CooldownMS = 0
	}
	if flow.HoldMS < 0 {
		flow.HoldMS = 0
	}
	if flow.ScheduleIntervalMS < 0 {
		flow.ScheduleIntervalMS = 0
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func ValidateTaskFlowTrigger(value string) error {
	switch value {
	case models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd, models.TaskFlowTriggerManual:
		return nil
	default:
		return fmt.Errorf("invalid task flow trigger_type %q", value)
	}
}
