package query

import (
	"strings"
	"time"
)

type ReportTemplate struct {
	ID               uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TemplateCode     string    `gorm:"column:template_code" json:"template_code"`
	Name             string    `gorm:"column:name" json:"name"`
	DisplayName      string    `gorm:"column:display_name" json:"display_name"`
	FileRef          string    `gorm:"column:file_ref" json:"file_ref"`
	FileKind         string    `gorm:"column:file_kind" json:"file_kind"`
	Version          int       `gorm:"column:version" json:"version"`
	ParamsSchemaJSON string    `gorm:"column:params_schema_json" json:"params_schema_json,omitempty"`
	Enabled          bool      `gorm:"column:enabled" json:"enabled"`
	Remark           string    `gorm:"column:remark" json:"remark"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ReportTemplate) TableName() string { return "sys_report_templates" }

type ReportTemplateFilter struct {
	Enabled *bool
	Keyword string
}

func (q *StationViewQuery) ListReportTemplates(filter ReportTemplateFilter) ([]ReportTemplate, error) {
	stmt := q.db.Model(&ReportTemplate{})
	if filter.Enabled != nil {
		stmt = stmt.Where("enabled = ?", *filter.Enabled)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		stmt = stmt.Where("template_code LIKE ? OR name LIKE ? OR display_name LIKE ?", like, like, like)
	}
	var templates []ReportTemplate
	err := stmt.Order("id asc").Find(&templates).Error
	return templates, err
}
