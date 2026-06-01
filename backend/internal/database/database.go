package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=5s&readTimeout=30s&writeTimeout=30s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Name,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.New(
			log.New(log.Writer(), "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	return db, nil
}

func TestConnection(ctx context.Context, cfg config.DatabaseConfig) error {
	db, err := Connect(cfg)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() {
		_ = sqlDB.Close()
	}()
	return sqlDB.PingContext(ctx)
}

func AutoMigrate(db *gorm.DB) error {
	if err := migrateProjectNaming(db); err != nil {
		return err
	}
	return db.AutoMigrate(
		&models.GatewayConfig{},
		&models.Project{},
		&models.SysProjectMember{},
		&models.TagConfig{},
		&models.StorageRoute{},
		&models.TaskRule{},
		&models.TaskFlow{},
		&models.TaskFlowVar{},
		&models.TaskFlowRun{},
		&models.TaskFlowSQLLog{},
		&models.DetectionStandard{},
		&models.DetectionStandardFavorite{},
		&models.DetectionStandardRecent{},
		&models.DetectionStandardItem{},
		&models.DetectionTask{},
		&models.DetectionRunStandardItem{},
		&models.DetectionRunStorageRoute{},
		&models.DetectionRunNote{},
		&models.DetectionRunEvent{},
		&models.DetectionRunSummary{},
		&models.DetectionRunFeature{},
		&models.DetectionLimitAlarm{},
		&models.ReportTemplate{},
		&models.DetectionRunReport{},
		&models.DetectionRunReportRequest{},
		&models.HistoryData{},
		&models.SysUser{},
		&models.SysServiceClient{},
		&models.SysSSOTicket{},
		&models.SysAuditLog{},
		&models.SysNotification{},
		&models.SysNotificationRecipient{},
	)
}

func migrateProjectNaming(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable("sys_devices") && !m.HasTable("sys_projects") {
		if err := m.RenameTable("sys_devices", "sys_projects"); err != nil {
			return err
		}
	}
	columnRenames := []struct {
		model any
		old   string
		new   string
	}{
		{&models.Project{}, "device_code", "project_code"},
		{&models.TagConfig{}, "device_id", "project_id"},
		{&models.TagConfig{}, "device_code", "project_code"},
		{&models.StorageRoute{}, "device_id", "project_id"},
		{&models.DetectionStandard{}, "device_id", "project_id"},
		{&models.DetectionStandard{}, "device_code", "project_code"},
		{&models.DetectionTask{}, "device_id", "project_id"},
		{&models.DetectionTask{}, "device_code", "project_code"},
		{&models.DetectionRunStorageRoute{}, "device_id", "project_id"},
		{&models.DetectionLimitAlarm{}, "device_id", "project_id"},
		{&models.DetectionLimitAlarm{}, "device_code", "project_code"},
		{&models.DetectionRunEvent{}, "device_id", "project_id"},
		{&models.DetectionRunEvent{}, "device_code", "project_code"},
		{&models.DetectionRunSummary{}, "device_id", "project_id"},
		{&models.DetectionRunSummary{}, "device_code", "project_code"},
		{&models.HistoryData{}, "device_id", "project_id"},
		{&models.HistoryData{}, "device_code", "project_code"},
	}
	for _, item := range columnRenames {
		if m.HasColumn(item.model, item.old) && !m.HasColumn(item.model, item.new) {
			if err := m.RenameColumn(item.model, item.old, item.new); err != nil {
				return err
			}
		}
	}
	if err := db.AutoMigrate(
		&models.Project{},
		&models.TagConfig{},
		&models.DetectionRunStandardItem{},
	); err != nil {
		return err
	}
	tagStorageColumns := []string{
		"store_mode",
		"store_trigger",
		"store_cycle_sec",
		"store_deadband",
		"storage_name",
		"storage_target",
		"storage_table",
		"storage_value_column",
		"storage_key_column",
		"storage_time_column",
		"form_field_key",
		"query_alias",
		"startup_snapshot_enable",
	}
	for _, column := range tagStorageColumns {
		if m.HasColumn(&models.TagConfig{}, column) {
			if err := m.DropColumn(&models.TagConfig{}, column); err != nil {
				return err
			}
		}
	}
	runItemStorageColumns := []string{
		"storage_name",
		"storage_target",
		"storage_table",
		"storage_value_column",
		"storage_key_column",
		"storage_time_column",
		"form_field_key",
		"query_alias",
	}
	for _, column := range runItemStorageColumns {
		if m.HasColumn(&models.DetectionRunStandardItem{}, column) {
			if err := m.DropColumn(&models.DetectionRunStandardItem{}, column); err != nil {
				return err
			}
		}
	}
	return nil
}
