package pipeline

import (
	"testing"
	"time"

	"spindle-edge/backend/internal/models"

	"github.com/tidwall/gjson"
)

func TestChannelsStats(t *testing.T) {
	channels := NewChannels()
	channels.Logic <- &models.MQTTMessage{}
	channels.Discovery <- &models.MQTTMessage{}
	channels.Store <- &models.StoreTask{}
	channels.Alarm <- &models.DetectionLimitAlarmEvent{}
	channels.Notify <- &models.RuntimeNotification{}
	stats := channels.Stats()
	if stats["logic"] != 1 || stats["discovery"] != 1 || stats["store"] != 1 || stats["alarm"] != 1 || stats["notify"] != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestTagManagerLoadUpsertAndSnapshots(t *testing.T) {
	manager := NewTagManager()
	ProjectID := uint(2)
	otherProjectID := uint(3)
	manager.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceTopic: "topic-a", VarName: "a", JSONPath: "a", DataType: "FLOAT", ProjectID: &ProjectID, Enabled: true, ScaleFactor: 1},
	})
	if manager.Count() != 1 {
		t.Fatal("expected one tag")
	}
	first, ok := manager.Get(1)
	if !ok {
		t.Fatal("missing tag")
	}
	manager.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceTopic: "topic-a", VarName: "a2", JSONPath: "a", DataType: "FLOAT", ProjectID: &ProjectID, Enabled: true, ScaleFactor: 1},
	})
	second, _ := manager.Get(1)
	if first != second || second.Config.VarName != "a2" {
		t.Fatal("expected existing tag to be reused and updated")
	}
	manager.Upsert([]models.TagConfig{
		{VarID: 2, GatewayID: 1, SourceTopic: "topic-b", VarName: "b", JSONPath: "b", DataType: "STRING", ProjectID: &otherProjectID, Enabled: true, ScaleFactor: 1},
		{VarID: 3, GatewayID: 1, VarName: "disabled", JSONPath: "d", DataType: "FLOAT", Enabled: false},
	})
	if manager.Count() != 2 || len(manager.All()) != 2 || len(manager.Snapshots()) != 2 {
		t.Fatalf("unexpected tag count: %d", manager.Count())
	}
	if got := manager.ForMessage(1, "topic-a"); len(got) != 1 || got[0].Config.VarID != 1 {
		t.Fatalf("expected topic-a index to return var 1: %+v", got)
	}
	if got := manager.ForMessage(1, "topic-b"); len(got) != 1 || got[0].Config.VarID != 2 {
		t.Fatalf("expected topic-b index to return var 2: %+v", got)
	}
	if got := manager.ForMessage(9, "missing"); len(got) != 2 {
		t.Fatalf("expected missing index fallback to all tags, got %d", len(got))
	}
	if got := manager.ForProject(ProjectID); len(got) != 1 || got[0].Config.VarID != 1 {
		t.Fatalf("expected Project index to return var 1: %+v", got)
	}
	if got := manager.SnapshotsForProject(ProjectID); len(got) != 1 || got[0].VarID != 1 {
		t.Fatalf("expected Project snapshots to return var 1 only after reload: %+v", got)
	}
	manager.Upsert([]models.TagConfig{
		{VarID: 4, GatewayID: 1, SourceTopic: "topic-c", VarName: "candidate", JSONPath: "c", DataType: "FLOAT", Enabled: true, ScaleFactor: 1},
	})
	if manager.Count() != 2 {
		t.Fatalf("expected unassigned enabled candidate to stay out of runtime map, got %d", manager.Count())
	}
	manager.Upsert([]models.TagConfig{
		{VarID: 2, GatewayID: 1, SourceTopic: "topic-b", VarName: "b", JSONPath: "b", DataType: "STRING", Enabled: false, ScaleFactor: 1},
	})
	if manager.Count() != 1 {
		t.Fatalf("expected disabled upsert to remove tag, got %d", manager.Count())
	}
	if got := manager.ForMessage(1, "topic-b"); len(got) != 1 || got[0].Config.VarID != 1 {
		t.Fatalf("expected removed topic index to fallback to remaining tags: %+v", got)
	}
}

func TestTaskManagerLifecycle(t *testing.T) {
	manager := NewTaskManager()
	manager.LoadTaskRules([]models.TaskRule{
		{ID: 1, RuleCode: "start", Enabled: true, TriggerVarID: 99, TriggerOperator: models.TaskRuleOperatorEQ, TriggerValue: "1", TriggerEdge: models.TaskRuleEdgeRising, ActionType: models.TaskRuleActionDetectionStart, Priority: 1},
	})
	matches := manager.EvaluateTaskRules(99, 0, 1, true, false)
	if len(matches) != 1 || matches[0].Rule.RuleCode != "start" {
		t.Fatalf("expected loaded task rule match: %+v", matches)
	}
	manager.Load([]models.DetectionTask{
		{ID: 1, TestNo: "T-1", ProjectID: 10, ProjectCode: "AC-01", Mode: "standard"},
	})
	if task, ok := manager.ActiveForProject(10); !ok || task.TestNo != "T-1" {
		t.Fatalf("unexpected loaded task: %+v ok=%v", task, ok)
	}
	manager.SetActive(models.DetectionTask{ID: 2, TestNo: "T-2", ProjectID: 11, ProjectCode: "AC-02", Mode: "fast"})
	if len(manager.AllActive()) != 2 {
		t.Fatal("expected two active tasks")
	}
	standardID := uint(9)
	manager.SetActive(models.DetectionTask{
		ID:          3,
		TestNo:      "T-3",
		ProjectID:   12,
		ProjectCode: "AC-03",
		Mode:        "standard",
		StandardID:  &standardID,
		StandardItems: []models.DetectionRunStandardItem{
			{VarID: 1, StoreEnabled: true},
			{VarID: 2, StoreEnabled: false},
		},
	})
	if task, ok := manager.ActiveForProject(12); !ok || !task.AllowsStore(1) || task.AllowsStore(2) {
		t.Fatalf("unexpected standard store filter: %+v ok=%v", task, ok)
	}
	manager.Clear(10)
	if _, ok := manager.ActiveForProject(10); ok {
		t.Fatal("expected task cleared")
	}
}

func TestProcessMessageUpdatesTagsAndStoresOnlyWhenAllowed(t *testing.T) {
	ProjectID := uint(10)
	tags := NewTagManager()
	tags.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, Enabled: true},
		{VarID: 2, GatewayID: 1, SourceTopic: "topic", VarName: "running", JSONPath: "running", DataType: "BOOL", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, Enabled: true},
		{VarID: 3, GatewayID: 1, SourceTopic: "topic", VarName: "label", JSONPath: "label", DataType: "STRING", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, Enabled: true},
		{VarID: 4, GatewayID: 1, SourceTopic: "other", VarName: "other", JSONPath: "other", DataType: "FLOAT", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, Enabled: true},
	})
	tasks := NewTaskManager()
	tasks.SetActive(models.DetectionTask{
		ID:          7,
		TestNo:      "T-7",
		ProjectID:   ProjectID,
		ProjectCode: "AC-01",
		Mode:        "standard",
		StorageRoutes: []models.DetectionRunStorageRoute{
			changeRoute(1, ProjectID, "temp"),
			changeRoute(2, ProjectID, "running"),
			changeRoute(3, ProjectID, "label"),
		},
	})
	channels := NewChannels()
	at := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	processMessage(1, &models.MQTTMessage{
		GatewayID: 1,
		Topic:     "topic",
		Payload:   []byte(`{"temp":23.5,"running":true,"label":"ok"}`),
		Timestamp: at,
	}, channels, tags, tasks)

	if len(channels.Store) != 0 {
		t.Fatalf("first frame should wait for on_start or later change routes, got %d", len(channels.Store))
	}
	processMessage(1, &models.MQTTMessage{
		GatewayID: 1,
		Topic:     "topic",
		Payload:   []byte(`{"temp":24.5,"running":false,"label":"ng"}`),
		Timestamp: at.Add(time.Second),
	}, channels, tags, tasks)
	if len(channels.Store) != 3 {
		t.Fatalf("expected three route-driven store tasks, got %d", len(channels.Store))
	}
	if tag, ok := tags.Get(4); !ok || !tag.Snapshot().LastUpdate.IsZero() {
		t.Fatalf("topic index should not update other topic tag: ok=%v snapshot=%+v", ok, tag.Snapshot())
	}
	snapshots := tags.Snapshots()
	var updated bool
	for _, snapshot := range snapshots {
		if snapshot.SourceTopic == "topic" && !snapshot.LastUpdate.IsZero() {
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("expected snapshots to be updated: %+v", snapshots)
	}
	processMessage(1, &models.MQTTMessage{Topic: "bad", Payload: []byte(`bad`)}, channels, tags, tasks)
}

func TestProcessMessageDoesNotStoreDebouncePending(t *testing.T) {
	ProjectID := uint(10)
	threshold := 5.0
	tags := NewTagManager()
	tags.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, DebounceThreshold: &threshold, DebounceMS: 100, Enabled: true},
	})
	tasks := NewTaskManager()
	tasks.SetActive(models.DetectionTask{
		ID:          7,
		TestNo:      "T-7",
		ProjectID:   ProjectID,
		ProjectCode: "AC-01",
		Mode:        "standard",
		StorageRoutes: []models.DetectionRunStorageRoute{
			{
				ID:            1,
				TaskID:        7,
				ProjectID:     ProjectID,
				VarID:         1,
				StorageTarget: models.StorageTargetWideTable,
				StorageTable:  "rt_project_10_data",
				ColumnName:    "temp",
				ColumnType:    "DOUBLE",
				TriggerMode:   models.StoreTriggerOnChange,
				Deadband:      0.1,
			},
		},
	})
	channels := NewChannels()
	at := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":20}`), Timestamp: at}, channels, tags, tasks)
	if len(channels.Store) != 0 {
		t.Fatalf("expected first frame not to store without on_start route, got %d", len(channels.Store))
	}
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":40}`), Timestamp: at.Add(time.Second)}, channels, tags, tasks)
	if len(channels.Store) != 0 {
		t.Fatalf("expected pending debounce not to store, got %d", len(channels.Store))
	}
	if snap := tags.Snapshots()[0]; snap.Value != 20 {
		t.Fatalf("expected pending debounce to keep stable value: %+v", snap)
	}
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":40}`), Timestamp: at.Add(time.Second + 120*time.Millisecond)}, channels, tags, tasks)
	if len(channels.Store) != 1 {
		t.Fatalf("expected debounced value to store after hold, got %d", len(channels.Store))
	}
}

func TestProcessMessageEnqueuesDetectionLimitAlarmEnterAndRecover(t *testing.T) {
	ProjectID := uint(10)
	tags := NewTagManager()
	tags.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1, Enabled: true},
	})
	limitH := 10.0
	limitHH := 20.0
	deadband := 1.0
	tasks := NewTaskManager()
	tasks.SetActive(models.DetectionTask{
		ID:          7,
		TestNo:      "T-7",
		ProjectID:   ProjectID,
		ProjectCode: "AC-01",
		Mode:        "standard",
		StandardID:  uintPtr(3),
		StandardItems: []models.DetectionRunStandardItem{{
			ID:             11,
			StandardID:     3,
			StandardItemID: 5,
			VarID:          1,
			VarName:        "temp",
			CheckEnabled:   true,
			AlarmEnabled:   true,
			CheckMethod:    models.CheckMethodNumericRange,
			LimitH:         &limitH,
			LimitHH:        &limitHH,
			LimitDeadband:  deadband,
		}},
	})
	channels := NewChannels()
	at := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":12}`), Timestamp: at}, channels, tags, tasks)
	enter := <-channels.Alarm
	if enter.Action != models.DetectionAlarmActionEnter || enter.Alarm.AlarmType != "above_h" || enter.Alarm.Status != models.DetectionAlarmStatusActive || enter.Alarm.LimitValue == nil || *enter.Alarm.LimitValue != 10 {
		t.Fatalf("unexpected enter alarm event: %+v", enter)
	}

	if muted := tasks.MuteActiveLimitAlarms(7); muted != 1 {
		t.Fatalf("expected one muted alarm, got %d", muted)
	}
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":22}`), Timestamp: at.Add(500 * time.Millisecond)}, channels, tags, tasks)
	levelChange := <-channels.Alarm
	if levelChange.Action != models.DetectionAlarmActionLevelChange || levelChange.PreviousAlarmType != "above_h" || levelChange.Alarm.AlarmType != "above_hh" {
		t.Fatalf("unexpected level change alarm event: %+v", levelChange)
	}

	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":19.5}`), Timestamp: at.Add(time.Second)}, channels, tags, tasks)
	if len(channels.Alarm) != 0 {
		t.Fatalf("expected deadband to keep alarm active")
	}
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":18.5}`), Timestamp: at.Add(2 * time.Second)}, channels, tags, tasks)
	recover := <-channels.Alarm
	if recover.Action != models.DetectionAlarmActionRecover || recover.Alarm.Status != models.DetectionAlarmStatusClosed || recover.Alarm.RecoverValue == nil || *recover.Alarm.RecoverValue != 18.5 {
		t.Fatalf("unexpected recover alarm event: %+v", recover)
	}
}

func TestBuildStoreTaskIfAllowed(t *testing.T) {
	ProjectID := uint(1)
	tag := models.NewTag(models.TagConfig{VarID: 1, VarName: "temp", DataType: "FLOAT", ProjectID: &ProjectID, ProjectCode: "AC-01", ScaleFactor: 1})
	tag.UpdateNumeric(20, time.Now(), 1)
	if task := buildStoreTaskIfAllowed(tag, NewTaskManager(), 1, "topic", time.Now()); task != nil {
		t.Fatalf("expected no task without active detection: %+v", task)
	}
	tasks := NewTaskManager()
	tasks.SetActive(models.DetectionTask{ID: 1, TestNo: "T-1", ProjectID: ProjectID, ProjectCode: "AC-01", Mode: "standard", StorageRoutes: []models.DetectionRunStorageRoute{changeRoute(1, ProjectID, "temp")}})
	if task := buildStoreTaskIfAllowed(tag, tasks, 1, "topic", time.Now()); task == nil || task.TaskID != 1 {
		t.Fatalf("expected active task: %+v", task)
	}
	standardID := uint(2)
	tasks.SetActive(models.DetectionTask{
		ID:          2,
		TestNo:      "T-2",
		ProjectID:   ProjectID,
		ProjectCode: "AC-01",
		Mode:        "standard",
		StandardID:  &standardID,
		StandardItems: []models.DetectionRunStandardItem{
			{VarID: 1, StoreEnabled: false},
		},
	})
	if task := buildStoreTaskIfAllowed(tag, tasks, 1, "topic", time.Now()); task != nil {
		t.Fatalf("expected standard to block storage: %+v", task)
	}
	if numericValue(gjson.Parse("true"), "BOOL") != 1 || numericValue(gjson.Parse("12"), "INT") != 12 {
		t.Fatal("numeric value conversion failed")
	}
}

func uintPtr(value uint) *uint {
	return &value
}

func changeRoute(varID int64, ProjectID uint, column string) models.DetectionRunStorageRoute {
	return models.DetectionRunStorageRoute{
		ID:            uint64(varID),
		TaskID:        7,
		ProjectID:     ProjectID,
		VarID:         varID,
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  "rt_project_10_data",
		ColumnName:    column,
		ColumnType:    "DOUBLE",
		TriggerMode:   models.StoreTriggerOnChange,
	}
}
