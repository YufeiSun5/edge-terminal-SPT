package models

import (
	"testing"
	"time"
)

func TestTableNames(t *testing.T) {
	names := map[string]string{
		GatewayConfig{}.TableName():             "sys_gateways",
		Project{}.TableName():                   "sys_projects",
		TagConfig{}.TableName():                 "sys_tags",
		StorageRoute{}.TableName():              "sys_storage_routes",
		HistoryData{}.TableName():               "rt_history_data",
		SysUser{}.TableName():                   "sys_users",
		SysServiceClient{}.TableName():          "sys_service_clients",
		SysSSOTicket{}.TableName():              "sys_sso_tickets",
		SysAuditLog{}.TableName():               "sys_audit_logs",
		DetectionTask{}.TableName():             "sys_detection_tasks",
		DetectionStandard{}.TableName():         "sys_detection_standards",
		DetectionStandardFavorite{}.TableName(): "sys_detection_standard_favorites",
		DetectionStandardRecent{}.TableName():   "sys_detection_standard_recents",
		DetectionStandardItem{}.TableName():     "sys_detection_standard_items",
		DetectionRunStandardItem{}.TableName():  "detection_run_standard_items",
		DetectionRunStorageRoute{}.TableName():  "detection_run_storage_routes",
		DetectionRunEvent{}.TableName():         "detection_run_events",
		DetectionRunSummary{}.TableName():       "detection_run_summaries",
		DetectionLimitAlarm{}.TableName():       "detection_limit_alarms",
		SysNotification{}.TableName():           "sys_notifications",
		SysNotificationRecipient{}.TableName():  "sys_notification_recipients",
		TaskRule{}.TableName():                  "sys_task_rules",
		TaskFlow{}.TableName():                  "sys_task_flows",
		TaskFlowVar{}.TableName():               "sys_task_flow_vars",
		TaskFlowRun{}.TableName():               "task_flow_runs",
		TaskFlowSQLLog{}.TableName():            "task_flow_sql_logs",
	}
	for got, want := range names {
		if got != want {
			t.Fatalf("table name got=%s want=%s", got, want)
		}
	}
}

func TestActiveTaskAllowsStore(t *testing.T) {
	withoutStandard := ActiveTask{}
	if !withoutStandard.AllowsStore(1) {
		t.Fatal("task without standard should allow legacy storage")
	}
	standardID := uint(7)
	withStandard := ActiveTask{
		StandardID: &standardID,
		StandardItems: map[int64]DetectionRunStandardItem{
			1: {VarID: 1, StoreEnabled: true},
			2: {VarID: 2, StoreEnabled: false},
		},
	}
	if !withStandard.AllowsStore(1) || withStandard.AllowsStore(2) || withStandard.AllowsStore(3) {
		t.Fatal("standard storage filter failed")
	}
	customRun := ActiveTask{
		StandardItems: map[int64]DetectionRunStandardItem{
			1: {VarID: 1, StoreEnabled: true},
			2: {VarID: 2, StoreEnabled: false},
		},
	}
	if !customRun.AllowsStore(1) || customRun.AllowsStore(2) || customRun.AllowsStore(3) {
		t.Fatal("custom run storage filter failed")
	}
}

func TestTagDefaultsAndNumericUpdates(t *testing.T) {
	tag := NewTag(TagConfig{
		VarID:       1,
		GatewayID:   1,
		VarName:     "temp",
		DataType:    "FLOAT",
		ScaleFactor: 2,
		OffsetVal:   1,
	})
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	if old, changed, first := tag.UpdateNumeric(10, now, 1); old != 0 || changed || !first {
		t.Fatalf("unexpected first numeric update old=%v changed=%v first=%v", old, changed, first)
	}
	if snap := tag.Snapshot(); snap.Value != 21 || snap.Quality != 1 || snap.LastUpdate != now {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if old, changed, first := tag.UpdateNumeric(10.1, now.Add(time.Second), 0); old != 21 || first || !changed {
		t.Fatalf("unexpected deadband update old=%v changed=%v first=%v", old, changed, first)
	}
	if _, changed, _ := tag.UpdateNumeric(11, now.Add(2*time.Second), 0); !changed {
		t.Fatal("expected numeric change outside deadband")
	}
}

func TestTagNumericCleaningFilters(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	suspicious := 999.0
	threshold := 5.0
	tag := NewTag(TagConfig{
		VarID:             10,
		GatewayID:         1,
		VarName:           "temp",
		DataType:          "FLOAT",
		ScaleFactor:       1,
		SuspiciousValue:   &suspicious,
		DebounceThreshold: &threshold,
		DebounceMS:        100,
		Deadband:          0.2,
	})

	if _, _, first := tag.UpdateNumeric(20, now, 1); !first {
		t.Fatal("expected first accepted value")
	}
	if _, changed, first := tag.UpdateNumeric(999, now.Add(time.Second), 1); changed || first || tag.Snapshot().Value != 20 {
		t.Fatalf("suspicious value should not replace stable snapshot: %+v", tag.Snapshot())
	}
	if _, changed, first := tag.UpdateNumeric(20.1, now.Add(2*time.Second), 1); changed || first || tag.Snapshot().Value != 20 {
		t.Fatalf("runtime deadband should keep stable value: %+v", tag.Snapshot())
	}
	if snap := tag.Snapshot(); snap.LastUpdate != now.Add(2*time.Second) {
		t.Fatalf("runtime deadband should refresh timestamp, got %s", snap.LastUpdate)
	}
	if _, changed, first := tag.UpdateNumeric(40, now.Add(3*time.Second), 1); changed || first || tag.Snapshot().Value != 20 {
		t.Fatalf("debounce pending should keep stable value: %+v", tag.Snapshot())
	}
	if _, changed, first := tag.UpdateNumeric(40, now.Add(3*time.Second+120*time.Millisecond), 1); !changed || first || tag.Snapshot().Value != 40 {
		t.Fatalf("debounce elapsed should accept value changed=%v first=%v snap=%+v", changed, first, tag.Snapshot())
	}
}

func TestTagFirstFrameUpdatesRuntimeOnly(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tag := NewTag(TagConfig{
		VarID:       11,
		GatewayID:   1,
		VarName:     "counter",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		CreatedAt:   now,
	})
	if _, changed, first := tag.UpdateNumeric(7, now, 1); changed || !first {
		t.Fatalf("first frame should initialize runtime without change-store signal changed=%v first=%v", changed, first)
	}
	if snap := tag.Snapshot(); snap.Value != 7 {
		t.Fatalf("first frame should initialize stable value: %+v", snap)
	}
}

func TestTagStringCycleAndStoreTask(t *testing.T) {
	ProjectID := uint(12)
	tag := NewTag(TagConfig{
		VarID:       2,
		GatewayID:   1,
		ProjectID:   &ProjectID,
		ProjectCode: "AC-01",
		VarName:     "status",
		DataType:    "STRING",
	})
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	if old, changed, first := tag.UpdateString("run", now, 1); old != "" || changed || !first {
		t.Fatalf("unexpected first string update old=%q changed=%v first=%v", old, changed, first)
	}
	if !tag.Snapshot().IsString || tag.Snapshot().StrValue != "run" {
		t.Fatalf("unexpected string snapshot: %+v", tag.Snapshot())
	}
	active := ActiveTask{
		ID:          3,
		TestNo:      "T-1",
		ProjectID:   ProjectID,
		ProjectCode: "AC-01",
		Mode:        "standard",
		StorageRoutes: map[int64][]DetectionRunStorageRoute{
			2: {{
				ID:            9,
				TaskID:        3,
				ProjectID:     ProjectID,
				VarID:         2,
				StorageTarget: StorageTargetWideTable,
				StorageTable:  "rt_project_12_data",
				ColumnName:    "status",
				ColumnType:    "TEXT",
				TriggerMode:   StoreTriggerOnCycle,
				CycleMS:       10000,
			}},
		},
	}
	task := tag.StoreTaskForTrigger(1, "topic", active, now, StoreTriggerOnCycle, false, false)
	if task.TaskID != 3 || task.StrValue != "run" || !task.IsString {
		t.Fatalf("unexpected store task: %+v", task)
	}
	tag.MarkStorageRoutesStored(task.StorageRoutes, now)
	if task := tag.StoreTaskForTrigger(1, "topic", active, now.Add(5*time.Second), StoreTriggerOnCycle, false, false); task != nil {
		t.Fatalf("cycle route should not be due yet: %+v", task)
	}
	if task := tag.StoreTaskForTrigger(1, "topic", active, now.Add(11*time.Second), StoreTriggerOnCycle, false, false); task == nil {
		t.Fatal("expected cycle route due after route cycle_ms")
	}
}

func TestDataTypeHelpers(t *testing.T) {
	if !IsStringDataType("varchar") || IsStringDataType("float") {
		t.Fatal("string data type helper failed")
	}
}
