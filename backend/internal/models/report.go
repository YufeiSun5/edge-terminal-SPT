package models

import (
	"encoding/json"
	"strconv"
	"time"
)

type ReportTemplate struct {
	ID               uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateCode     string    `gorm:"column:template_code;size:64;uniqueIndex;not null" json:"template_code"`
	Name             string    `gorm:"column:name;size:128;not null" json:"name"`
	DisplayName      string    `gorm:"column:display_name;size:128" json:"display_name"`
	FileRef          string    `gorm:"column:file_ref;size:512;not null" json:"file_ref"`
	FileKind         string    `gorm:"column:file_kind;size:32;not null;default:xlsx" json:"file_kind"`
	Version          int       `gorm:"column:version;default:1;not null" json:"version"`
	ParamsSchemaJSON string    `gorm:"column:params_schema_json;type:text" json:"params_schema_json,omitempty"`
	Enabled          bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	Remark           string    `gorm:"column:remark;size:255" json:"remark"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ReportTemplate) TableName() string {
	return "sys_report_templates"
}

type DetectionRunReport struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          uint       `gorm:"column:task_id;index;not null" json:"task_id"`
	TemplateID      *uint      `gorm:"column:template_id;index" json:"template_id,omitempty"`
	TemplateCode    string     `gorm:"column:template_code;size:64" json:"template_code"`
	TemplateVersion int        `gorm:"column:template_version;default:0" json:"template_version"`
	FileRef         string     `gorm:"column:file_ref;size:512;not null" json:"file_ref"`
	FileName        string     `gorm:"column:file_name;size:255" json:"file_name"`
	Status          string     `gorm:"column:status;size:32;not null;default:pending" json:"status"`
	GeneratedAt     *time.Time `gorm:"column:generated_at" json:"generated_at,omitempty"`
	ErrorMessage    string     `gorm:"column:error_message;size:512" json:"error_message"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunReport) TableName() string {
	return "detection_run_reports"
}

type DetectionRunReportRequest struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID          uint      `gorm:"column:task_id;index;not null" json:"task_id"`
	TestNo          string    `gorm:"column:test_no;size:128;index" json:"test_no"`
	ProjectID       uint      `gorm:"column:project_id;index;not null" json:"project_id"`
	ProjectCode     string    `gorm:"column:project_code;size:64;index" json:"project_code"`
	TemplateID      *uint     `gorm:"column:template_id;index" json:"template_id,omitempty"`
	TemplateCode    string    `gorm:"column:template_code;size:64;index" json:"template_code"`
	TemplateVersion int       `gorm:"column:template_version;default:0" json:"template_version"`
	VarID           int64     `gorm:"column:var_id;index;not null;default:0" json:"var_id"`
	VarName         string    `gorm:"column:var_name;size:128;index;not null" json:"var_name"`
	DisplayName     string    `gorm:"column:display_name;size:128" json:"display_name"`
	DisplayNameEN   string    `gorm:"column:display_name_en;size:128" json:"display_name_en"`
	DisplayNameJA   string    `gorm:"column:display_name_ja;size:128" json:"display_name_ja"`
	ReportName      string    `gorm:"column:report_name;size:128" json:"report_name"`
	VariablesJSON   string    `gorm:"column:variables_json;type:text" json:"variables_json,omitempty"`
	ParamsJSON      string    `gorm:"column:params_json;type:text" json:"params_json,omitempty"`
	Status          string    `gorm:"column:status;size:32;not null;default:pending" json:"status"`
	Ext1            string    `gorm:"column:ext_1;size:255" json:"ext_1"`
	Ext2            string    `gorm:"column:ext_2;size:255" json:"ext_2"`
	Ext3            string    `gorm:"column:ext_3;size:255" json:"ext_3"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionRunReportRequest) TableName() string {
	return "detection_run_report_requests"
}

func (r DetectionRunReportRequest) MarshalJSON() ([]byte, error) {
	type alias DetectionRunReportRequest
	variables := rawJSONOrDefault(r.VariablesJSON, "[]")
	params := rawJSONOrDefault(r.ParamsJSON, "{}")
	return json.Marshal(struct {
		alias
		VarIDText string          `json:"var_id_text"`
		Variables json.RawMessage `json:"variables"`
		Params    json.RawMessage `json:"params"`
	}{
		alias:     alias(r),
		VarIDText: strconv.FormatInt(r.VarID, 10),
		Variables: variables,
		Params:    params,
	})
}

func rawJSONOrDefault(value string, fallback string) json.RawMessage {
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	return json.RawMessage(fallback)
}
