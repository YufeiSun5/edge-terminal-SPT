package database

import (
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) UpsertGatewaySeeds(gateways []models.GatewayConfig) error {
	if len(gateways) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&gateways).Error
}

func (r *Repository) LoadGateways(edgeInstanceID string) ([]models.GatewayConfig, error) {
	var gateways []models.GatewayConfig
	query := r.db.Where("enabled = ?", true)
	if edgeInstanceID != "" {
		query = query.Where("(edge_instance_id = ? OR edge_instance_id = '' OR edge_instance_id IS NULL)", edgeInstanceID)
	}
	err := query.Order("id asc").Find(&gateways).Error
	return gateways, err
}

func (r *Repository) ListGatewayConfigs() ([]models.GatewayConfig, error) {
	var gateways []models.GatewayConfig
	err := r.db.Order("id asc").Find(&gateways).Error
	return gateways, err
}

func (r *Repository) GetGatewayConfig(id int) (models.GatewayConfig, error) {
	var gateway models.GatewayConfig
	err := r.db.First(&gateway, "id = ?", id).Error
	return gateway, err
}

func (r *Repository) CreateGatewayConfig(gateway *models.GatewayConfig) error {
	if gateway.ID == 0 {
		var nextID int
		if err := r.db.Model(&models.GatewayConfig{}).
			Select("COALESCE(MAX(id), 0) + 1").
			Scan(&nextID).Error; err != nil {
			return err
		}
		gateway.ID = nextID
	}
	now := time.Now()
	gateway.CreatedAt = now
	gateway.UpdatedAt = now
	return r.db.Create(gateway).Error
}

func (r *Repository) UpdateGatewayConfig(id int, updates map[string]interface{}) (models.GatewayConfig, error) {
	if len(updates) == 0 {
		return r.GetGatewayConfig(id)
	}
	updates["updated_at"] = time.Now()
	if err := r.db.Model(&models.GatewayConfig{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return models.GatewayConfig{}, err
	}
	return r.GetGatewayConfig(id)
}

func (r *Repository) DeleteGatewayConfig(id int) error {
	result := r.db.Delete(&models.GatewayConfig{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
