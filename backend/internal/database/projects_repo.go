package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

func (r *Repository) ListProjects() ([]models.Project, error) {
	var Projects []models.Project
	err := r.db.Where("enabled = ?", true).Order("id asc").Find(&Projects).Error
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

func (r *Repository) ListProjectMembers(projectID uint) ([]models.ProjectMemberView, error) {
	if projectID == 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	var members []models.ProjectMemberView
	err := r.db.Table("sys_project_members AS pm").
		Select("pm.id, pm.project_id, pm.user_id, u.username, u.role AS user_role, u.enabled AS user_enabled, pm.member_role, pm.notify_enabled, pm.created_at, pm.updated_at").
		Joins("JOIN sys_users AS u ON u.id = pm.user_id").
		Where("pm.project_id = ?", projectID).
		Order("pm.id ASC").
		Scan(&members).Error
	if members == nil {
		members = []models.ProjectMemberView{}
	}
	return members, err
}

func (r *Repository) ReplaceProjectMembers(projectID uint, members []models.SysProjectMember) ([]models.ProjectMemberView, error) {
	if projectID == 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.First(&project, "id = ?", projectID).Error; err != nil {
			return err
		}
		seen := make(map[uint]struct{}, len(members))
		now := time.Now()
		cleaned := make([]models.SysProjectMember, 0, len(members))
		for _, member := range members {
			if member.UserID == 0 {
				return fmt.Errorf("user_id is required")
			}
			if _, ok := seen[member.UserID]; ok {
				return fmt.Errorf("duplicate project member user_id: %d", member.UserID)
			}
			seen[member.UserID] = struct{}{}
			var user models.SysUser
			if err := tx.First(&user, "id = ?", member.UserID).Error; err != nil {
				return err
			}
			role := strings.TrimSpace(member.MemberRole)
			if role == "" {
				role = "member"
			}
			cleaned = append(cleaned, models.SysProjectMember{
				ProjectID:     projectID,
				UserID:        member.UserID,
				MemberRole:    role,
				NotifyEnabled: member.NotifyEnabled,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		}
		if err := tx.Where("project_id = ?", projectID).Delete(&models.SysProjectMember{}).Error; err != nil {
			return err
		}
		if len(cleaned) == 0 {
			return nil
		}
		return tx.Select("ProjectID", "UserID", "MemberRole", "NotifyEnabled", "CreatedAt", "UpdatedAt").Create(&cleaned).Error
	})
	if err != nil {
		return nil, err
	}
	return r.ListProjectMembers(projectID)
}
