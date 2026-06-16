package database

import (
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm/clause"
)

func (r *Repository) UpsertRuntimeSetting(key string, value string, remark string) error {
	now := time.Now()
	item := models.RuntimeSetting{
		Key:       key,
		Value:     value,
		Remark:    remark,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "setting_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"setting_value": value,
			"remark":        remark,
			"updated_at":    now,
		}),
	}).Create(&item).Error
}

func (r *Repository) CreateRuntimeSettingIfMissing(key string, value string, remark string) error {
	now := time.Now()
	item := models.RuntimeSetting{
		Key:       key,
		Value:     value,
		Remark:    remark,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "setting_key"}},
		DoNothing: true,
	}).Create(&item).Error
}

func (r *Repository) ListRuntimeSettings() (map[string]string, error) {
	var items []models.RuntimeSetting
	if err := r.db.Find(&items).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		out[item.Key] = item.Value
	}
	return out, nil
}
