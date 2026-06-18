package database

import (
	"fmt"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

type DetectionPlanFilter struct {
	Status    string
	FactoryNo string
	Keyword   string
	Limit     int
	Offset    int
}

func (r *Repository) ListDetectionPlans(filter DetectionPlanFilter) ([]models.DetectionPlan, int64, error) {
	query := r.db.Model(&models.DetectionPlan{})
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if factoryNo := strings.TrimSpace(filter.FactoryNo); factoryNo != "" {
		query = query.Where("factory_no = ?", factoryNo)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"plan_no LIKE ? OR external_plan_id LIKE ? OR external_order_no LIKE ? OR factory_no LIKE ? OR device_model LIKE ? OR test_item_code LIKE ? OR test_item_name LIKE ? OR standard_code LIKE ?",
			like, like, like, like, like, like, like, like,
		)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var plans []models.DetectionPlan
	err := query.Order("factory_no asc, test_sequence asc, id asc").Limit(limit).Offset(offset).Find(&plans).Error
	return plans, total, err
}

func (r *Repository) GetDetectionPlan(id uint) (models.DetectionPlan, error) {
	var plan models.DetectionPlan
	err := r.db.First(&plan, "id = ?", id).Error
	return plan, err
}

func (r *Repository) MarkDetectionPlanStarting(id uint) (models.DetectionPlan, error) {
	now := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.DetectionPlan{}).
			Where("id = ? AND status = ?", id, models.DetectionPlanStatusPending).
			Updates(map[string]interface{}{
				"status":        models.DetectionPlanStatusStarting,
				"error_message": "",
				"updated_at":    now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrDetectionPlanNotPending
		}
		return nil
	})
	if err != nil {
		return models.DetectionPlan{}, err
	}
	return r.GetDetectionPlan(id)
}

type DetectionPlanStartedUpdate struct {
	PlanID              uint
	TaskID              uint
	OwnerEdgeInstanceID string
	OwnerProjectID      uint
	OwnerProjectCode    string
}

func (r *Repository) MarkDetectionPlanStarted(input DetectionPlanStartedUpdate) (models.DetectionPlan, error) {
	if input.PlanID == 0 || input.TaskID == 0 {
		return models.DetectionPlan{}, fmt.Errorf("plan_id and task_id are required")
	}
	now := time.Now()
	result := r.db.Model(&models.DetectionPlan{}).
		Where("id = ? AND status = ?", input.PlanID, models.DetectionPlanStatusStarting).
		Updates(map[string]interface{}{
			"status":                 models.DetectionPlanStatusStarted,
			"started_task_id":        input.TaskID,
			"started_at":             now,
			"owner_edge_instance_id": strings.TrimSpace(input.OwnerEdgeInstanceID),
			"owner_project_id":       input.OwnerProjectID,
			"owner_project_code":     strings.TrimSpace(input.OwnerProjectCode),
			"error_message":          "",
			"updated_at":             now,
		})
	if result.Error != nil {
		return models.DetectionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.DetectionPlan{}, ErrDetectionPlanNotPending
	}
	return r.GetDetectionPlan(input.PlanID)
}

func (r *Repository) ResetDetectionPlanPending(id uint, message string) error {
	now := time.Now()
	result := r.db.Model(&models.DetectionPlan{}).
		Where("id = ? AND status = ?", id, models.DetectionPlanStatusStarting).
		Updates(map[string]interface{}{
			"status":        models.DetectionPlanStatusPending,
			"error_message": strings.TrimSpace(message),
			"updated_at":    now,
		})
	return result.Error
}

func (r *Repository) CancelDetectionPlan(id uint, reason string) (models.DetectionPlan, error) {
	now := time.Now()
	result := r.db.Model(&models.DetectionPlan{}).
		Where("id = ? AND status IN ?", id, []string{models.DetectionPlanStatusPending, models.DetectionPlanStatusStarting}).
		Updates(map[string]interface{}{
			"status":        models.DetectionPlanStatusCancelled,
			"cancelled_at":  now,
			"error_message": strings.TrimSpace(reason),
			"updated_at":    now,
		})
	if result.Error != nil {
		return models.DetectionPlan{}, result.Error
	}
	if result.RowsAffected == 0 {
		return models.DetectionPlan{}, ErrDetectionPlanNotPending
	}
	return r.GetDetectionPlan(id)
}
