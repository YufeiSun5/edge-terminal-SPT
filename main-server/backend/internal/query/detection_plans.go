package query

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrDetectionPlanNotEditable = errors.New("detection plan is not editable")
var ErrDetectionPlanInvalid = errors.New("detection plan invalid")

type DetectionPlan struct {
	ID                  uint       `gorm:"column:id;primaryKey" json:"id"`
	PlanNo              string     `gorm:"column:plan_no" json:"plan_no"`
	SourceSystem        string     `gorm:"column:source_system" json:"source_system"`
	ExternalPlanID      string     `gorm:"column:external_plan_id" json:"external_plan_id"`
	ExternalOrderNo     string     `gorm:"column:external_order_no" json:"external_order_no"`
	FactoryNo           string     `gorm:"column:factory_no" json:"factory_no"`
	CustomerName        string     `gorm:"column:customer_name" json:"customer_name"`
	DeviceModel         string     `gorm:"column:device_model" json:"device_model"`
	TestItemCode        string     `gorm:"column:test_item_code" json:"test_item_code"`
	TestItemName        string     `gorm:"column:test_item_name" json:"test_item_name"`
	TestSequence        int        `gorm:"column:test_sequence" json:"test_sequence"`
	Mode                string     `gorm:"column:mode" json:"mode"`
	StandardCode        string     `gorm:"column:standard_code" json:"standard_code"`
	ReportRequestJSON   string     `gorm:"column:report_request_json;type:text" json:"report_request_json,omitempty"`
	Status              string     `gorm:"column:status" json:"status"`
	OwnerEdgeInstanceID string     `gorm:"column:owner_edge_instance_id" json:"owner_edge_instance_id"`
	OwnerProjectID      *uint      `gorm:"column:owner_project_id" json:"owner_project_id,omitempty"`
	OwnerProjectCode    string     `gorm:"column:owner_project_code" json:"owner_project_code"`
	StartedTaskID       *uint      `gorm:"column:started_task_id" json:"started_task_id,omitempty"`
	StartedAt           *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`
	ErrorMessage        string     `gorm:"column:error_message" json:"error_message"`
	SyncScope           string     `gorm:"column:sync_scope" json:"sync_scope"`
	EdgeInstanceID      string     `gorm:"column:edge_instance_id" json:"edge_instance_id"`
	UpdatedByNode       string     `gorm:"column:updated_by_node" json:"updated_by_node"`
	UpdatedByUser       string     `gorm:"column:updated_by_user" json:"updated_by_user"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionPlan) TableName() string { return "sys_detection_plans" }

type DetectionPlanFilter struct {
	Status    string
	FactoryNo string
	Keyword   string
	Limit     int
	Offset    int
}

type DetectionPlanUpdate struct {
	PlanNo          string
	SourceSystem    string
	ExternalPlanID  string
	ExternalOrderNo string
	FactoryNo       string
	CustomerName    string
	DeviceModel     string
	TestItemCode    string
	TestItemName    string
	TestSequence    int
	Mode            string
	StandardCode    string
	UpdatedByUser   string
}

type DetectionPlanCreate struct {
	PlanNo            string
	SourceSystem      string
	ExternalPlanID    string
	ExternalOrderNo   string
	FactoryNo         string
	CustomerName      string
	DeviceModel       string
	TestItemCode      string
	TestItemName      string
	TestSequence      int
	Mode              string
	StandardCode      string
	ReportRequestJSON string
	SyncScope         string
	EdgeInstanceID    string
	UpdatedByUser     string
}

func (q *StationViewQuery) ListDetectionPlans(filter DetectionPlanFilter) ([]DetectionPlan, int64, int, int, error) {
	limit := normalizedDetectionPlanLimit(filter.Limit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	stmt := q.db.Model(&DetectionPlan{})
	stmt = applyDetectionPlanFilter(stmt, filter)
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, limit, offset, err
	}
	var plans []DetectionPlan
	err := stmt.Order("factory_no asc, test_sequence asc, id asc").Limit(limit).Offset(offset).Find(&plans).Error
	return plans, total, limit, offset, err
}

func (q *StationViewQuery) GetDetectionPlan(id uint) (DetectionPlan, error) {
	var plan DetectionPlan
	err := q.db.First(&plan, "id = ?", id).Error
	return plan, err
}

func (q *StationViewQuery) CreateDetectionPlan(input DetectionPlanCreate, meta SyncWriteMeta) (DetectionPlan, error) {
	now := time.Now()
	plan := DetectionPlan{
		PlanNo:            strings.TrimSpace(input.PlanNo),
		SourceSystem:      valueOrDefault(strings.TrimSpace(input.SourceSystem), "main-server"),
		ExternalPlanID:    strings.TrimSpace(input.ExternalPlanID),
		ExternalOrderNo:   strings.TrimSpace(input.ExternalOrderNo),
		FactoryNo:         strings.TrimSpace(input.FactoryNo),
		CustomerName:      strings.TrimSpace(input.CustomerName),
		DeviceModel:       strings.TrimSpace(input.DeviceModel),
		TestItemCode:      strings.TrimSpace(input.TestItemCode),
		TestItemName:      strings.TrimSpace(input.TestItemName),
		TestSequence:      input.TestSequence,
		Mode:              valueOrDefault(strings.TrimSpace(input.Mode), "standard"),
		StandardCode:      strings.TrimSpace(input.StandardCode),
		ReportRequestJSON: strings.TrimSpace(input.ReportRequestJSON),
		Status:            "pending",
		SyncScope:         valueOrDefault(strings.TrimSpace(input.SyncScope), valueOrDefault(strings.TrimSpace(meta.SyncScope), "global")),
		EdgeInstanceID:    valueOrDefault(strings.TrimSpace(input.EdgeInstanceID), strings.TrimSpace(meta.EdgeInstanceID)),
		UpdatedByNode:     normalizedUpdatedByNode(meta),
		UpdatedByUser:     valueOrDefault(strings.TrimSpace(input.UpdatedByUser), strings.TrimSpace(meta.UpdatedByUser)),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := validateDetectionPlanCreate(plan); err != nil {
		return DetectionPlan{}, err
	}
	err := q.db.Transaction(func(tx *gorm.DB) error {
		id, err := nextSyncID(tx, plan.TableName())
		if err != nil {
			return err
		}
		plan.ID = uint(id)
		return tx.Create(&plan).Error
	})
	if err != nil {
		return DetectionPlan{}, err
	}
	return q.GetDetectionPlan(plan.ID)
}

func (q *StationViewQuery) UpdatePendingDetectionPlan(id uint, input DetectionPlanUpdate) (DetectionPlan, error) {
	var plan DetectionPlan
	if err := q.db.First(&plan, "id = ?", id).Error; err != nil {
		return plan, err
	}
	if strings.TrimSpace(plan.Status) != "pending" {
		return plan, ErrDetectionPlanNotEditable
	}
	updates := map[string]any{
		"plan_no":           strings.TrimSpace(input.PlanNo),
		"source_system":     strings.TrimSpace(input.SourceSystem),
		"external_plan_id":  strings.TrimSpace(input.ExternalPlanID),
		"external_order_no": strings.TrimSpace(input.ExternalOrderNo),
		"factory_no":        strings.TrimSpace(input.FactoryNo),
		"customer_name":     strings.TrimSpace(input.CustomerName),
		"device_model":      strings.TrimSpace(input.DeviceModel),
		"test_item_code":    strings.TrimSpace(input.TestItemCode),
		"test_item_name":    strings.TrimSpace(input.TestItemName),
		"test_sequence":     input.TestSequence,
		"mode":              valueOrDefault(strings.TrimSpace(input.Mode), "standard"),
		"standard_code":     strings.TrimSpace(input.StandardCode),
		"updated_by_node":   "main-server",
		"updated_by_user":   strings.TrimSpace(input.UpdatedByUser),
		"updated_at":        time.Now(),
	}
	if err := validateDetectionPlanUpdate(updates); err != nil {
		return plan, err
	}
	result := q.db.Model(&DetectionPlan{}).Where("id = ? AND status = ?", id, "pending").Updates(updates)
	if result.Error != nil {
		return plan, result.Error
	}
	if result.RowsAffected == 0 {
		return plan, ErrDetectionPlanNotEditable
	}
	return q.GetDetectionPlan(id)
}

func applyDetectionPlanFilter(stmt *gorm.DB, filter DetectionPlanFilter) *gorm.DB {
	if strings.TrimSpace(filter.Status) != "" {
		stmt = stmt.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.FactoryNo) != "" {
		stmt = stmt.Where("factory_no = ?", strings.TrimSpace(filter.FactoryNo))
	}
	if strings.TrimSpace(filter.Keyword) != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		stmt = stmt.Where("(plan_no LIKE ? OR factory_no LIKE ? OR external_plan_id LIKE ? OR external_order_no LIKE ?)", keyword, keyword, keyword, keyword)
	}
	return stmt
}

func normalizedDetectionPlanLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func validateDetectionPlanUpdate(updates map[string]any) error {
	for _, field := range []string{"plan_no", "source_system", "external_plan_id", "factory_no", "standard_code"} {
		if strings.TrimSpace(updates[field].(string)) == "" {
			return fmt.Errorf("%w: %s is required", ErrDetectionPlanInvalid, field)
		}
	}
	return nil
}

func validateDetectionPlanCreate(plan DetectionPlan) error {
	for field, value := range map[string]string{
		"plan_no":          plan.PlanNo,
		"source_system":    plan.SourceSystem,
		"external_plan_id": plan.ExternalPlanID,
		"factory_no":       plan.FactoryNo,
		"standard_code":    plan.StandardCode,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrDetectionPlanInvalid, field)
		}
	}
	return nil
}

func valueOrDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
