package storage

import (
	"testing"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFlushAndWorker(t *testing.T) {
	db := newStorageTestDB(t)
	repo := database.NewRepository(db)
	flush("test", nil, repo)
	flush("test", []*models.StoreTask{{
		GatewayID:   1,
		Topic:       "topic",
		ProjectID:   1,
		TaskID:      1,
		TestNo:      "T-1",
		VarID:       100,
		VarName:     "temp",
		ProjectCode: "AC-01",
		Value:       23.5,
		Quality:     1,
		Timestamp:   time.Now(),
	}}, repo)

	channels := pipeline.NewChannels()
	bus := newStorageBus(repo, 1, 20*time.Millisecond)
	go bus.Run(channels.Store)
	channels.Store <- &models.StoreTask{
		GatewayID:   1,
		Topic:       "topic",
		ProjectID:   1,
		TaskID:      1,
		TestNo:      "T-1",
		VarID:       101,
		VarName:     "label",
		ProjectCode: "AC-01",
		StrValue:    "ok",
		IsString:    true,
		Quality:     1,
		Timestamp:   time.Now(),
	}
	close(channels.Store)
	time.Sleep(50 * time.Millisecond)

	var count int64
	if err := db.Model(&models.HistoryData{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("history count=%d", count)
	}
}

func TestSplitStoreTaskSkipsDuplicateHistoryRows(t *testing.T) {
	task := &models.StoreTask{
		ProjectID: 1,
		TaskID:    1,
		VarID:     100,
		Timestamp: time.Now(),
		StorageRoutes: []models.DetectionRunStorageRoute{
			{ProjectID: 1, VarID: 100, StorageTarget: models.StorageTargetWideTable, StorageTable: "rt_project_1_data", ColumnName: "temp"},
			{ProjectID: 1, VarID: 100, StorageTarget: models.StorageTargetWideTable, StorageTable: "rt_custom_data", ColumnName: "temp"},
		},
	}

	bucketed := splitStoreTaskByBucket(task)
	if len(bucketed) != 2 {
		t.Fatalf("bucketed len=%d", len(bucketed))
	}
	if bucketed[0].SkipHistoryRow {
		t.Fatal("first bucket should keep history row")
	}
	if !bucketed[1].SkipHistoryRow {
		t.Fatal("second bucket should skip duplicate history row")
	}
}

func TestStartWorkersDefaultBatch(t *testing.T) {
	db := newStorageTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	StartWorkers(1, 0, channels, repo)
	channels.Store <- &models.StoreTask{
		GatewayID: 1,
		Topic:     "topic",
		ProjectID: 1,
		TaskID:    1,
		TestNo:    "T-1",
		VarID:     100,
		VarName:   "temp",
		Value:     1,
		Quality:   1,
		Timestamp: time.Now(),
	}
	time.Sleep(250 * time.Millisecond)
	close(channels.Store)
}

func TestAlarmWorkerCreatesAndRecoversDetectionLimitAlarm(t *testing.T) {
	db := newStorageTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	go alarmWorker(1, channels, repo)

	startValue := 12.0
	limitValue := 10.0
	startedAt := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	channels.Alarm <- &models.DetectionLimitAlarmEvent{
		Action: models.DetectionAlarmActionEnter,
		Alarm: models.DetectionLimitAlarm{
			TaskID:        1,
			TestNo:        "T-1",
			ProjectID:     2,
			ProjectCode:   "AC-01",
			VarID:         100,
			VarName:       "temp",
			CheckMethod:   models.CheckMethodNumericRange,
			AlarmType:     "above_h",
			AlarmLevel:    "H",
			Status:        models.DetectionAlarmStatusActive,
			StartValue:    &startValue,
			PeakValue:     &startValue,
			LimitValue:    &limitValue,
			Quality:       1,
			FirstSeenAt:   startedAt,
			LastSeenAt:    startedAt,
			LimitDeadband: 1,
		},
	}
	time.Sleep(50 * time.Millisecond)

	recoverValue := 8.5
	recoveredAt := startedAt.Add(time.Second)
	channels.Alarm <- &models.DetectionLimitAlarmEvent{
		Action: models.DetectionAlarmActionRecover,
		Alarm: models.DetectionLimitAlarm{
			TaskID:       1,
			VarID:        100,
			AlarmType:    "above_h",
			Status:       models.DetectionAlarmStatusClosed,
			PeakValue:    &startValue,
			RecoverValue: &recoverValue,
			Quality:      1,
			LastSeenAt:   recoveredAt,
			RecoveredAt:  &recoveredAt,
			DurationMS:   1000,
		},
	}
	time.Sleep(50 * time.Millisecond)
	close(channels.Alarm)

	var alarm models.DetectionLimitAlarm
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := db.First(&alarm, "task_id = ? AND var_id = ?", 1, 100).Error; err == nil && alarm.Status == models.DetectionAlarmStatusClosed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if alarm.Status != models.DetectionAlarmStatusClosed || alarm.RecoverValue == nil || *alarm.RecoverValue != recoverValue || alarm.DurationMS != 1000 {
		t.Fatalf("unexpected recovered alarm: %+v", alarm)
	}
	notifications := drainAlarmNotifications(channels.Notify)
	if len(notifications) < 2 || notifications[0].Type != models.NotificationAlarmLimitEnter || notifications[1].Type != models.NotificationAlarmLimitRecover {
		t.Fatalf("expected enter and recover notifications, got %+v", notifications)
	}
}

func drainAlarmNotifications(ch <-chan *models.RuntimeNotification) []*models.RuntimeNotification {
	items := make([]*models.RuntimeNotification, 0)
	for {
		select {
		case item := <-ch:
			items = append(items, item)
		default:
			return items
		}
	}
}

func newStorageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}
