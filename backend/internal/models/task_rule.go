package models

import "time"

const (
	TaskRuleEdgeAny     = "any"
	TaskRuleEdgeRising  = "rising"
	TaskRuleEdgeFalling = "falling"

	TaskRuleOperatorEQ = "eq"
	TaskRuleOperatorNE = "ne"
	TaskRuleOperatorGT = "gt"
	TaskRuleOperatorGE = "ge"
	TaskRuleOperatorLT = "lt"
	TaskRuleOperatorLE = "le"

	TaskRuleActionDetectionStart = "detection_start"
	TaskRuleActionDetectionStop  = "detection_stop"
	TaskRuleActionStorageEnable  = "storage_enable"
)

type TaskRule struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID       uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	RuleCode        string    `gorm:"column:rule_code;size:128;uniqueIndex;not null" json:"rule_code"`
	Name            string    `gorm:"column:name;size:128;not null" json:"name"`
	Enabled         bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	TriggerVarID    int64     `gorm:"column:trigger_var_id;index;not null" json:"trigger_var_id"`
	TriggerOperator string    `gorm:"column:trigger_operator;size:16;not null" json:"trigger_operator"`
	TriggerValue    string    `gorm:"column:trigger_value;size:255;not null" json:"trigger_value"`
	TriggerEdge     string    `gorm:"column:trigger_edge;size:16;default:any;not null" json:"trigger_edge"`
	ActionType      string    `gorm:"column:action_type;size:64;not null" json:"action_type"`
	ActionPayload   string    `gorm:"column:action_payload;type:text" json:"action_payload"`
	Priority        int       `gorm:"column:priority;default:0;index" json:"priority"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskRule) TableName() string {
	return "sys_task_rules"
}
