package database

import (
	"errors"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) LoadTags(edgeInstanceID string) ([]models.TagConfig, error) {
	var tags []models.TagConfig
	query := r.db.Model(&models.TagConfig{}).
		Joins("JOIN sys_projects p ON p.id = sys_tags.project_id").
		Where("sys_tags.enabled = ? AND sys_tags.project_id IS NOT NULL", true)
	if edgeInstanceID != "" {
		query = query.Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	err := query.Order("sys_tags.var_id asc").Find(&tags).Error
	return tags, err
}

type TagFilter struct {
	GatewayID   *int
	ProjectID   *uint
	Enabled     *bool
	Discovered  *bool
	Writable    *bool
	SourceType  string
	ProjectCode string
	VarGroup    string
	Keyword     string
}

func (r *Repository) ListTags(filter TagFilter) ([]models.TagConfig, error) {
	var tags []models.TagConfig
	query := r.db.Model(&models.TagConfig{})
	if filter.GatewayID != nil {
		query = query.Where("gateway_id = ?", *filter.GatewayID)
	}
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Discovered != nil {
		query = query.Where("discovered = ?", *filter.Discovered)
	}
	if filter.Writable != nil {
		query = query.Where("writable = ?", *filter.Writable)
	}
	if filter.SourceType != "" {
		query = query.Where("source_type = ?", filter.SourceType)
	}
	if filter.ProjectCode != "" {
		query = query.Where("project_code = ?", strings.TrimSpace(filter.ProjectCode))
	}
	if filter.VarGroup != "" {
		query = query.Where("var_group = ?", strings.TrimSpace(filter.VarGroup))
	}
	if filter.Keyword != "" {
		keyword := "%" + strings.TrimSpace(filter.Keyword) + "%"
		query = query.Where(
			"raw_name LIKE ? OR var_name LIKE ? OR display_name LIKE ? OR display_name_en LIKE ? OR display_name_ja LIKE ? OR source_path LIKE ?",
			keyword, keyword, keyword, keyword, keyword, keyword,
		)
	}
	err := query.Order("gateway_id asc, source_path asc").Find(&tags).Error
	return tags, err
}

func (r *Repository) CreateTag(tag *models.TagConfig) error {
	now := time.Now()
	if tag.VarID == 0 {
		var nextID int64
		if err := r.db.Model(&models.TagConfig{}).
			Select("COALESCE(MAX(var_id), 0) + 1").
			Scan(&nextID).Error; err != nil {
			return err
		}
		tag.VarID = nextID
	}
	if tag.SourceType == "" {
		tag.SourceType = models.TagSourceMQTT
	}
	if tag.ScaleFactor == 0 {
		tag.ScaleFactor = 1
	}
	applyTagPersistenceDefaults(tag)
	tag.CreatedAt = now
	tag.UpdatedAt = now
	discovered := tag.Discovered
	placeholder := tag.Placeholder
	sourceType := tag.SourceType
	if err := r.db.Select("*").Create(tag).Error; err != nil {
		return err
	}
	tag.Discovered = discovered
	tag.Placeholder = placeholder
	tag.SourceType = sourceType
	if err := r.db.Model(&models.TagConfig{}).
		Where("var_id = ?", tag.VarID).
		Updates(map[string]interface{}{
			"discovered":  discovered,
			"placeholder": placeholder,
			"source_type": sourceType,
		}).Error; err != nil {
		return err
	}
	_, err := r.EnsureDefaultStorageRouteForTag(*tag)
	return err
}

func (r *Repository) GetTag(varID int64) (models.TagConfig, error) {
	var tag models.TagConfig
	err := r.db.First(&tag, "var_id = ?", varID).Error
	return tag, err
}

func (r *Repository) UpdateTag(varID int64, updates map[string]interface{}) (models.TagConfig, error) {
	if len(updates) == 0 {
		var tag models.TagConfig
		err := r.db.First(&tag, "var_id = ?", varID).Error
		return tag, err
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.TagConfig{}).
		Where("var_id = ?", varID).
		Updates(updates).Error; err != nil {
		return models.TagConfig{}, err
	}
	var tag models.TagConfig
	err := r.db.First(&tag, "var_id = ?", varID).Error
	return tag, err
}

func (r *Repository) AssignTag(varID int64, ProjectID *uint, ProjectCode string, group string, enabled bool) error {
	resolvedProjectCode := ""
	if ProjectID != nil {
		var Project models.Project
		if err := r.db.First(&Project, "id = ?", *ProjectID).Error; err != nil {
			return err
		}
		if err := r.ensureTagProjectGatewayEdge(varID, Project); err != nil {
			return err
		}
		resolvedProjectCode = Project.ProjectCode
	} else {
		resolvedProjectCode = ProjectCode
	}

	result := r.db.Model(&models.TagConfig{}).
		Where("var_id = ?", varID).
		Updates(map[string]interface{}{
			"project_id":   ProjectID,
			"project_code": resolvedProjectCode,
			"var_group":    group,
			"enabled":      enabled,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	if ProjectID == nil || !enabled {
		return nil
	}
	tag, err := r.GetTag(varID)
	if err != nil {
		return err
	}
	_, err = r.EnsureDefaultStorageRouteForTag(tag)
	return err
}

func (r *Repository) EnsureTagProjectGatewayEdge(varID int64, projectID uint) error {
	var project models.Project
	if err := r.db.First(&project, "id = ?", projectID).Error; err != nil {
		return err
	}
	return r.ensureTagProjectGatewayEdge(varID, project)
}

func (r *Repository) ensureTagProjectGatewayEdge(varID int64, project models.Project) error {
	var tag models.TagConfig
	if err := r.db.First(&tag, "var_id = ?", varID).Error; err != nil {
		return err
	}
	if tag.GatewayID <= 0 {
		return nil
	}
	var gateway models.GatewayConfig
	if err := r.db.First(&gateway, "id = ?", tag.GatewayID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	projectEdge := strings.TrimSpace(project.EdgeInstanceID)
	gatewayEdge := strings.TrimSpace(gateway.EdgeInstanceID)
	if projectEdge != "" && gatewayEdge != "" && projectEdge != gatewayEdge {
		return ErrEdgeInstanceMismatch
	}
	return nil
}

func (r *Repository) DeleteTag(varID int64) error {
	result := r.db.Delete(&models.TagConfig{}, "var_id = ?", varID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) UpsertDiscoveredTags(tags []models.TagConfig) error {
	if len(tags) == 0 {
		return nil
	}
	for i := range tags {
		tags[i].Discovered = true
		tags[i].Placeholder = false
		tags[i].Enabled = false
		tags[i].Writable = false
		tags[i].RWMode = models.RWModeRead
		tags[i].WriteRequiresAudit = true
		applyTagPersistenceDefaults(&tags[i])
	}
	if err := r.db.Select("*").Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "gateway_id"},
			{Name: "source_path"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"source_topic",
			"source_type",
			"raw_name",
			"data_type",
			"updated_at",
		}),
	}).Create(&tags).Error; err != nil {
		return err
	}
	for _, tag := range tags {
		if err := r.db.Model(&models.TagConfig{}).
			Where("gateway_id = ? AND source_path = ? AND project_id IS NULL AND discovered = ?", tag.GatewayID, tag.SourcePath, true).
			Updates(map[string]interface{}{
				"enabled":     false,
				"writable":    false,
				"rw_mode":     models.RWModeRead,
				"placeholder": false,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyTagPersistenceDefaults(tag *models.TagConfig) {
	if tag.RWMode == "" {
		tag.RWMode = models.RWModeRead
	}
	if tag.WriteDataType == "" && tag.Writable {
		tag.WriteDataType = tag.DataType
	}
}
