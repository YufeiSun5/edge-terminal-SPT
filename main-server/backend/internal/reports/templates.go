package reports

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"spindle-main-server/backend/internal/query"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

var (
	ErrInvalidReportTemplate = errors.New("invalid report template")
	ErrTemplateNotFound      = errors.New("report template not found")
)

type TemplateUploadInput struct {
	TemplateCode     string
	Name             string
	DisplayName      string
	Version          int
	ParamsSchemaJSON string
	Remark           string
	Enabled          bool
}

func (s *Service) UploadTemplate(ctx context.Context, input TemplateUploadInput, raw []byte, originalName string) (query.ReportTemplate, ArtifactMeta, error) {
	code := strings.TrimSpace(input.TemplateCode)
	if code == "" {
		return query.ReportTemplate{}, ArtifactMeta{}, fmt.Errorf("%w: template_code is required", ErrInvalidReportTemplate)
	}
	if len(raw) == 0 {
		return query.ReportTemplate{}, ArtifactMeta{}, fmt.Errorf("%w: file is required", ErrInvalidReportTemplate)
	}
	if err := validateTemplateWorkbook(raw); err != nil {
		return query.ReportTemplate{}, ArtifactMeta{}, err
	}

	var existing query.ReportTemplate
	hasExisting := s.db.First(&existing, "template_code = ?", code).Error == nil
	version := input.Version
	if version <= 0 {
		version = 1
		if hasExisting && existing.Version > 0 {
			version = existing.Version + 1
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = code
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = name
	}
	fileName := "source.xlsx"
	if ext := strings.ToLower(filepath.Ext(originalName)); ext == ".xlsx" {
		fileName = safeArtifactName(strings.TrimSuffix(filepath.Base(originalName), ext)) + ext
	}
	sha := sha256Hex(raw)
	key := filepath.ToSlash(filepath.Join("templates", safeArtifactName(code), fmt.Sprintf("v%d", version), sha, fileName))
	meta, err := s.store.Put(ctx, key, raw, reportXLSXContentType)
	if err != nil {
		return query.ReportTemplate{}, ArtifactMeta{}, err
	}

	now := time.Now()
	updates := map[string]any{
		"template_code":      code,
		"name":               name,
		"display_name":       displayName,
		"file_ref":           meta.Key,
		"file_kind":          "xlsx",
		"file_sha256":        meta.SHA256,
		"file_size":          meta.Size,
		"version":            version,
		"params_schema_json": strings.TrimSpace(input.ParamsSchemaJSON),
		"enabled":            input.Enabled,
		"remark":             strings.TrimSpace(input.Remark),
		"updated_at":         now,
	}
	if hasExisting {
		if err := s.db.Model(&query.ReportTemplate{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return query.ReportTemplate{}, ArtifactMeta{}, err
		}
		if err := s.db.First(&existing, "id = ?", existing.ID).Error; err != nil {
			return query.ReportTemplate{}, ArtifactMeta{}, err
		}
		return existing, meta, nil
	}

	template := query.ReportTemplate{
		TemplateCode:     code,
		Name:             name,
		DisplayName:      displayName,
		FileRef:          meta.Key,
		FileKind:         "xlsx",
		FileSHA256:       meta.SHA256,
		FileSize:         meta.Size,
		Version:          version,
		ParamsSchemaJSON: strings.TrimSpace(input.ParamsSchemaJSON),
		Enabled:          input.Enabled,
		Remark:           strings.TrimSpace(input.Remark),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.db.Create(&template).Error; err != nil {
		return query.ReportTemplate{}, ArtifactMeta{}, err
	}
	return template, meta, nil
}

func (s *Service) UpdateTemplateMapping(id uint, paramsSchemaJSON string) (query.ReportTemplate, error) {
	var template query.ReportTemplate
	if err := s.db.First(&template, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return query.ReportTemplate{}, ErrTemplateNotFound
		}
		return query.ReportTemplate{}, err
	}
	if err := s.db.Model(&query.ReportTemplate{}).Where("id = ?", id).Updates(map[string]any{
		"params_schema_json": strings.TrimSpace(paramsSchemaJSON),
		"updated_at":         time.Now(),
	}).Error; err != nil {
		return query.ReportTemplate{}, err
	}
	if err := s.db.First(&template, "id = ?", id).Error; err != nil {
		return query.ReportTemplate{}, err
	}
	return template, nil
}

func (s *Service) TemplateArtifact(id uint) (string, string, string, error) {
	var template query.ReportTemplate
	if err := s.db.First(&template, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", "", ErrTemplateNotFound
		}
		return "", "", "", err
	}
	ref := strings.TrimSpace(template.FileRef)
	if ref == "" {
		return "", "", "", ErrArtifactUnavailable
	}
	path, err := s.store.Path(ref)
	if err != nil || !s.store.Exists(context.Background(), ref) {
		legacyPath, legacyErr := s.legacyArtifactPath(ref)
		if legacyErr != nil {
			return "", "", "", ErrArtifactUnavailable
		}
		path = legacyPath
	}
	name := fmt.Sprintf("%s-v%d.xlsx", safeArtifactName(template.TemplateCode), template.Version)
	return path, name, reportXLSXContentType, nil
}

func validateTemplateWorkbook(raw []byte) error {
	file, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: xlsx cannot be opened", ErrInvalidReportTemplate)
	}
	return file.Close()
}
