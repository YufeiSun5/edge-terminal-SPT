package query

import "gorm.io/gorm"

func EnsureMainServerSchema(db *gorm.DB) error {
	return db.AutoMigrate(&MainNotificationRead{})
}
