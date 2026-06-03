package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type TaskFlow struct {
	ID                 uint64        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID          uint          `gorm:"column:project_id" json:"project_id"`
	FlowCode           string        `gorm:"column:flow_code" json:"flow_code"`
	Name               string        `gorm:"column:name" json:"name"`
	Enabled            bool          `gorm:"column:enabled" json:"enabled"`
	TriggerType        string        `gorm:"column:trigger_type" json:"trigger_type"`
	ConditionScript    string        `gorm:"column:condition_script" json:"condition_script"`
	ActionType         string        `gorm:"column:action_type" json:"action_type"`
	ActionScript       string        `gorm:"column:action_script" json:"action_script"`
	ActionPayload      string        `gorm:"column:action_payload" json:"action_payload"`
	StepsJSON          string        `gorm:"column:steps_json" json:"steps_json"`
	TimeoutMS          int           `gorm:"column:timeout_ms" json:"timeout_ms"`
	CooldownMS         int           `gorm:"column:cooldown_ms" json:"cooldown_ms"`
	HoldMS             int           `gorm:"column:hold_ms" json:"hold_ms"`
	ScheduleIntervalMS int           `gorm:"column:schedule_interval_ms" json:"schedule_interval_ms"`
	Priority           int           `gorm:"column:priority" json:"priority"`
	Remark             string        `gorm:"column:remark" json:"remark"`
	Vars               []TaskFlowVar `gorm:"foreignKey:FlowID;references:ID" json:"vars,omitempty"`
	CreatedAt          time.Time     `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time     `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskFlow) TableName() string { return "sys_task_flows" }

type TaskFlowVar struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FlowID    uint64    `gorm:"column:flow_id" json:"flow_id"`
	ProjectID uint      `gorm:"column:project_id" json:"project_id"`
	VarID     int64     `gorm:"column:var_id" json:"var_id"`
	VarName   string    `gorm:"column:var_name" json:"var_name"`
	Role      string    `gorm:"column:role" json:"role"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TaskFlowVar) TableName() string { return "sys_task_flow_vars" }

func (v TaskFlowVar) MarshalJSON() ([]byte, error) {
	type alias TaskFlowVar
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(v),
		VarIDText: strconv.FormatInt(v.VarID, 10),
	})
}

type TaskFlowFilter struct {
	ProjectID   *uint
	TriggerType string
	Enabled     *bool
}

func (q *StationViewQuery) ListTaskFlows(filter TaskFlowFilter, edgeInstanceID string) ([]TaskFlow, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, err
		}
	}
	stmt := q.db.Preload("Vars").
		Model(&TaskFlow{}).
		Joins("LEFT JOIN sys_projects p ON p.id = sys_task_flows.project_id")
	if edgeInstanceID = strings.TrimSpace(edgeInstanceID); edgeInstanceID != "" {
		stmt = stmt.Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("sys_task_flows.project_id = ?", *filter.ProjectID)
	}
	if strings.TrimSpace(filter.TriggerType) != "" {
		stmt = stmt.Where("sys_task_flows.trigger_type = ?", strings.TrimSpace(filter.TriggerType))
	}
	if filter.Enabled != nil {
		stmt = stmt.Where("sys_task_flows.enabled = ?", *filter.Enabled)
	}
	var flows []TaskFlow
	err := stmt.Order("sys_task_flows.project_id asc, sys_task_flows.priority desc, sys_task_flows.id asc").Find(&flows).Error
	return flows, err
}

func (q *StationViewQuery) GetTaskFlow(id uint64, edgeInstanceID string) (TaskFlow, error) {
	var flow TaskFlow
	if err := q.db.Preload("Vars").First(&flow, "id = ?", id).Error; err != nil {
		return flow, err
	}
	if flow.ProjectID != 0 {
		if _, err := q.projectForEdge(flow.ProjectID, edgeInstanceID); err != nil {
			return TaskFlow{}, err
		}
	}
	return flow, nil
}
