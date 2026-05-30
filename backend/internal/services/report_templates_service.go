package services

import (
	"fmt"
	"strings"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
)

type ReportTemplatesService struct {
	repo *database.Repository
}

type CreateReportTemplateInput struct {
	TemplateCode string
	Name         string
	DisplayName  string
	FileRef      string
	FileKind     string
	Version      int
	Enabled      *bool
	Remark       string
}

func NewReportTemplatesService(repo *database.Repository) *ReportTemplatesService {
	return &ReportTemplatesService{repo: repo}
}

func (s *ReportTemplatesService) List(filter database.ReportTemplateFilter) ([]models.ReportTemplate, error) {
	return s.repo.ListReportTemplates(filter)
}

func (s *ReportTemplatesService) Create(input CreateReportTemplateInput) (models.ReportTemplate, error) {
	code := strings.TrimSpace(input.TemplateCode)
	name := strings.TrimSpace(input.Name)
	fileRef := strings.TrimSpace(input.FileRef)
	if code == "" {
		return models.ReportTemplate{}, fmt.Errorf("template_code is required")
	}
	if name == "" {
		return models.ReportTemplate{}, fmt.Errorf("name is required")
	}
	if fileRef == "" {
		return models.ReportTemplate{}, fmt.Errorf("file_ref is required")
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	template := models.ReportTemplate{
		TemplateCode: code,
		Name:         name,
		DisplayName:  input.DisplayName,
		FileRef:      fileRef,
		FileKind:     firstNonEmpty(strings.TrimSpace(input.FileKind), "xlsx"),
		Version:      input.Version,
		Enabled:      enabled,
		Remark:       input.Remark,
	}
	if err := s.repo.CreateReportTemplate(&template); err != nil {
		return models.ReportTemplate{}, err
	}
	return template, nil
}

func (s *ReportTemplatesService) Update(id uint, updates map[string]interface{}) (models.ReportTemplate, error) {
	return s.repo.UpdateReportTemplate(id, updates)
}

func (s *ReportTemplatesService) Delete(id uint) error {
	return s.repo.DeleteReportTemplate(id)
}
