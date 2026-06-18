package models

import "time"

type DetectionPlan struct {
	ID                  uint       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PlanNo              string     `gorm:"column:plan_no;size:128;uniqueIndex;not null" json:"plan_no"`
	SourceSystem        string     `gorm:"column:source_system;size:64;uniqueIndex:uk_detection_plans_source_external;not null" json:"source_system"`
	ExternalPlanID      string     `gorm:"column:external_plan_id;size:128;uniqueIndex:uk_detection_plans_source_external;not null" json:"external_plan_id"`
	ExternalOrderNo     string     `gorm:"column:external_order_no;size:128;index" json:"external_order_no"`
	FactoryNo           string     `gorm:"column:factory_no;size:128;index;not null" json:"factory_no"`
	CustomerName        string     `gorm:"column:customer_name;size:128" json:"customer_name"`
	DeviceModel         string     `gorm:"column:device_model;size:128" json:"device_model"`
	TestItemCode        string     `gorm:"column:test_item_code;size:64" json:"test_item_code"`
	TestItemName        string     `gorm:"column:test_item_name;size:128" json:"test_item_name"`
	TestSequence        int        `gorm:"column:test_sequence;default:0" json:"test_sequence"`
	Mode                string     `gorm:"column:mode;size:64;default:standard" json:"mode"`
	StandardCode        string     `gorm:"column:standard_code;size:64;index;not null" json:"standard_code"`
	ReportRequestJSON   string     `gorm:"column:report_request_json;type:text" json:"report_request_json,omitempty"`
	Status              string     `gorm:"column:status;size:32;index;not null;default:pending" json:"status"`
	OwnerEdgeInstanceID string     `gorm:"column:owner_edge_instance_id;size:64;index" json:"owner_edge_instance_id"`
	OwnerProjectID      *uint      `gorm:"column:owner_project_id;index" json:"owner_project_id,omitempty"`
	OwnerProjectCode    string     `gorm:"column:owner_project_code;size:64" json:"owner_project_code"`
	StartedTaskID       *uint      `gorm:"column:started_task_id;index" json:"started_task_id,omitempty"`
	StartedAt           *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`
	ErrorMessage        string     `gorm:"column:error_message;size:512" json:"error_message"`
	SyncScope           string     `gorm:"column:sync_scope;size:32;default:global;index" json:"sync_scope"`
	EdgeInstanceID      string     `gorm:"column:edge_instance_id;size:64;index" json:"edge_instance_id"`
	UpdatedByNode       string     `gorm:"column:updated_by_node;size:64;index" json:"updated_by_node"`
	UpdatedByUser       string     `gorm:"column:updated_by_user;size:128" json:"updated_by_user"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionPlan) TableName() string {
	return "sys_detection_plans"
}
