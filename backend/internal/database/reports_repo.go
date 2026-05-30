package database

import (
	"fmt"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

func (r *Repository) CreateDetectionRunNote(note *models.DetectionRunNote) error {
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now()
	}
	return r.db.Create(note).Error
}

func (r *Repository) ListDetectionRunNotes(taskID uint, limit int) ([]models.DetectionRunNote, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var notes []models.DetectionRunNote
	err := r.db.Where("task_id = ?", taskID).Order("created_at desc, id desc").Limit(limit).Find(&notes).Error
	return notes, err
}

type ReportTemplateFilter struct {
	Enabled *bool
	Keyword string
}

func (r *Repository) ListReportTemplates(filter ReportTemplateFilter) ([]models.ReportTemplate, error) {
	query := r.db.Model(&models.ReportTemplate{})
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Keyword != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		query = query.Where("template_code LIKE ? OR name LIKE ? OR display_name LIKE ?", keyword, keyword, keyword)
	}
	var templates []models.ReportTemplate
	err := query.Order("id asc").Find(&templates).Error
	return templates, err
}

func (r *Repository) GetReportTemplate(id uint) (models.ReportTemplate, error) {
	return getReportTemplate(r.db, id)
}

func getReportTemplate(db *gorm.DB, id uint) (models.ReportTemplate, error) {
	var template models.ReportTemplate
	err := db.First(&template, "id = ?", id).Error
	return template, err
}

func (r *Repository) CreateReportTemplate(template *models.ReportTemplate) error {
	now := time.Now()
	template.CreatedAt = now
	template.UpdatedAt = now
	if template.FileKind == "" {
		template.FileKind = "xlsx"
	}
	if template.Version <= 0 {
		template.Version = 1
	}
	return r.db.Create(template).Error
}

func (r *Repository) UpdateReportTemplate(id uint, updates map[string]interface{}) (models.ReportTemplate, error) {
	if len(updates) == 0 {
		return r.GetReportTemplate(id)
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.ReportTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return models.ReportTemplate{}, err
	}
	return r.GetReportTemplate(id)
}

func (r *Repository) DeleteReportTemplate(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var refs int64
		for _, model := range []struct {
			name  string
			query *gorm.DB
		}{
			{"standards", tx.Model(&models.DetectionStandard{}).Where("report_template_id = ?", id)},
			{"tasks", tx.Model(&models.DetectionTask{}).Where("report_template_id = ?", id)},
			{"reports", tx.Model(&models.DetectionRunReport{}).Where("template_id = ?", id)},
		} {
			refs = 0
			if err := model.query.Count(&refs).Error; err != nil {
				return fmt.Errorf("%s reference check: %w", model.name, err)
			}
			if refs > 0 {
				return ErrReferenced
			}
		}
		return tx.Delete(&models.ReportTemplate{}, "id = ?", id).Error
	})
}

func (r *Repository) CreateDetectionRunReport(report *models.DetectionRunReport) error {
	now := time.Now()
	report.CreatedAt = now
	report.UpdatedAt = now
	return r.db.Create(report).Error
}

func (r *Repository) ListDetectionRunReports(taskID uint) ([]models.DetectionRunReport, error) {
	var reports []models.DetectionRunReport
	err := r.db.Where("task_id = ?", taskID).Order("created_at desc, id desc").Find(&reports).Error
	return reports, err
}
