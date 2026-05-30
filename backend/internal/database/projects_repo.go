package database

import (
	"errors"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

func (r *Repository) ListProjects() ([]models.Project, error) {
	var Projects []models.Project
	err := r.db.Order("id asc").Find(&Projects).Error
	return Projects, err
}

func (r *Repository) GetProject(id uint) (models.Project, error) {
	var Project models.Project
	err := r.db.First(&Project, "id = ?", id).Error
	return Project, err
}

func (r *Repository) CreateProject(Project *models.Project) error {
	return r.db.Create(Project).Error
}

func (r *Repository) GetProjectByCode(projectCode string) (models.Project, error) {
	var Project models.Project
	err := r.db.First(&Project, "project_code = ?", projectCode).Error
	return Project, err
}

func (r *Repository) EnsureProjectByCode(Project models.Project) (models.Project, bool, bool, error) {
	if existing, err := r.GetProjectByCode(Project.ProjectCode); err == nil {
		updates := make(map[string]interface{})
		if existing.SiteNo == "" && Project.SiteNo != "" {
			updates["site_no"] = Project.SiteNo
		}
		if existing.DisplayName == "" && Project.DisplayName != "" {
			updates["display_name"] = Project.DisplayName
		}
		if existing.DisplayNameEN == "" && Project.DisplayNameEN != "" {
			updates["display_name_en"] = Project.DisplayNameEN
		}
		if existing.DisplayNameJA == "" && Project.DisplayNameJA != "" {
			updates["display_name_ja"] = Project.DisplayNameJA
		}
		if len(updates) == 0 {
			return existing, false, false, nil
		}
		updated, updateErr := r.UpdateProject(existing.ID, updates)
		return updated, false, true, updateErr
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Project{}, false, false, err
	}
	if !Project.Enabled {
		Project.Enabled = true
	}
	if err := r.CreateProject(&Project); err != nil {
		return models.Project{}, false, false, err
	}
	return Project, true, false, nil
}

func (r *Repository) UpdateProject(id uint, updates map[string]interface{}) (models.Project, error) {
	if len(updates) == 0 {
		return r.GetProject(id)
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.Project{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return models.Project{}, err
	}
	return r.GetProject(id)
}

func (r *Repository) EnsureProjectDisplayNameFallbacks() error {
	return r.db.Model(&models.Project{}).
		Where("(display_name = '' OR display_name IS NULL) AND name <> ''").
		Update("display_name", gorm.Expr("name")).Error
}
