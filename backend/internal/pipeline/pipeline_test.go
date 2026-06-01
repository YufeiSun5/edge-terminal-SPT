package pipeline

import (
	"bytes"
	"log"
	"strings"
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
	details := channels.DetailedStats()
	if len(details) != 5 {
		t.Fatalf("expected five detailed stats, got %+v", details)
	}
	if details[0].Name != "alarm" || details[0].Len != 1 || details[0].Cap != cap(channels.Alarm) {
		t.Fatalf("unexpected first detailed stat: %+v", details[0])
	}
	channels.RecordDrop("alarm")
	channels.RecordDrop("store")
	details = channels.DetailedStats()
	if details[0].Dropped != 1 || channels.DropCount("store") != 1 {
		t.Fatalf("unexpected drop counters: details=%+v store=%d", details[0], channels.DropCount("store"))
	}
	diagnostics := channels.DetailedStatsWithDiagnosis(0.0001)
	if len(diagnostics) != 5 || !diagnostics[0].Pressure || diagnostics[0].Impact == "" || diagnostics[0].NextAction == "" {
		t.Fatalf("expected actionable diagnostics, got %+v", diagnostics)
	}
	if pressure := channels.Pressure(0.0001); len(pressure) != 5 {
		t.Fatalf("expected all channels to be under pressure at low threshold, got %+v", pressure)
	} else if pressure[0].Impact == "" || pressure[0].NextAction == "" {
		t.Fatalf("expected pressure diagnostics, got %+v", pressure[0])
	}
	if pressure := channels.Pressure(1.0); len(pressure) != 0 {
		t.Fatalf("expected no channel to be full, got %+v", pressure)
	}
}

func TestRunRecoveringLogsPanic(t *testing.T) {
	resetWorkerRecoveryStatsForTest()
	var buffer bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buffer)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	}()

	RunRecovering("unit-worker", func() {
		panic("boom")
	})

	output := buffer.String()
	if !strings.Contains(output, "[worker] panic recovered name=unit-worker") || !strings.Contains(output, "boom") {
		t.Fatalf("expected panic recovery log, got %q", output)
	}
	stats := WorkerRecoveryStats()
	if len(stats) != 1 || stats[0].Name != "unit-worker" || stats[0].Panics != 1 || stats[0].Active != 0 || stats[0].Health != "stopped_after_panic" {
		t.Fatalf("unexpected worker recovery stats: %+v", stats)
	}
	if stats[0].Impact == "" || stats[0].NextAction == "" || !strings.Contains(stats[0].LastError, "boom") {
		t.Fatalf("expected actionable worker diagnostics: %+v", stats[0])
	}
}

func TestRunRecoveringTracksNormalExit(t *testing.T) {
	resetWorkerRecoveryStatsForTest()
	RunRecovering("short-worker", func() {})

	stats := WorkerRecoveryStats()
	if len(stats) != 1 || stats[0].Name != "short-worker" || stats[0].Starts != 1 || stats[0].Exits != 1 || stats[0].Panics != 0 || stats[0].Active != 0 {
		t.Fatalf("unexpected worker recovery stats: %+v", stats)
	}
	if stats[0].Health != "ok" || stats[0].NextAction != "No action needed." || stats[0].LastStartedAt == nil || stats[0].LastExitedAt == nil {
		t.Fatalf("expected normal worker diagnostics: %+v", stats[0])
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

func TestProcessMessageKIOPayloadUpdatesOnlyIndexedKnownTags(t *testing.T) {
	projectID := uint(10)
	tags := NewTagManager()
	tags.Load([]models.TagConfig{
		{
			VarID:         3901,
			GatewayID:     1,
			SourceTopic:   "datachange_S_KIO_Project",
			SourcePath:    `Objs.#(N=="台1_39").1`,
			SourceType:    models.TagSourceMQTT,
			RawName:       "台1_39",
			VarName:       "kio_01_39",
			JSONPath:      `Objs.#(N=="台1_39").1`,
			DataType:      "FLOAT",
			ProjectID:     &projectID,
			ProjectCode:   "AC-01",
			ScaleFactor:   2,
			OffsetVal:     1,
			Enabled:       true,
			DisplayName:   "台1 测点 39",
			DisplayNameEN: "AC-01 point 39",
			DisplayNameJA: "AC-01 ポイント 39",
		},
		{
			VarID:       4001,
			GatewayID:   1,
			SourceTopic: "datachange_S_KIO_Project",
			SourcePath:  `Objs.#(N=="台1_40").1`,
			SourceType:  models.TagSourceMQTT,
			RawName:     "台1_40",
			VarName:     "kio_01_40",
			JSONPath:    `Objs.#(N=="台1_40").1`,
			DataType:    "BOOL",
			ProjectID:   &projectID,
			ProjectCode: "AC-01",
			ScaleFactor: 1,
			Enabled:     true,
		},
		{
			VarID:       4201,
			GatewayID:   1,
			SourceTopic: "datachange_S_KIO_Project",
			SourcePath:  `Objs.#(N=="台1_42").1`,
			SourceType:  models.TagSourceMQTT,
			RawName:     "台1_42",
			VarName:     "kio_01_42",
			JSONPath:    `Objs.#(N=="台1_42").1`,
			DataType:    "STRING",
			ProjectID:   &projectID,
			ProjectCode: "AC-01",
			ScaleFactor: 1,
			Enabled:     true,
		},
		{
			VarID:       5001,
			GatewayID:   1,
			SourceTopic: "datachange_S_KIO_Project",
			SourcePath:  `Objs.#(N=="$ProjectControlOfS7-1200").1`,
			SourceType:  models.TagSourceMQTT,
			RawName:     "$ProjectControlOfS7-1200",
			VarName:     "$ProjectControlOfS7_1200",
			JSONPath:    `Objs.#(N=="$ProjectControlOfS7-1200").1`,
			DataType:    "INT",
			ProjectID:   &projectID,
			ProjectCode: "AC-01",
			ScaleFactor: 1,
			Enabled:     true,
		},
		{
			VarID:       9901,
			GatewayID:   1,
			SourceTopic: "other_topic",
			SourcePath:  `Objs.#(N=="台1_39").1`,
			SourceType:  models.TagSourceMQTT,
			RawName:     "台1_39",
			VarName:     "other_topic_should_not_update",
			JSONPath:    `Objs.#(N=="台1_39").1`,
			DataType:    "FLOAT",
			ProjectID:   &projectID,
			ProjectCode: "AC-01",
			ScaleFactor: 1,
			Enabled:     true,
		},
	})

	at := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	payload := []byte(`{
		"Writer":"IOServer",
		"PVs":{"1":7,"2":"2026-05-31 08:30:00.000 +0800","3":192},
		"Objs":[
			{"N":"台1_39","1":13.5,"3":192},
			{"N":"台1_40","1":true,"3":192},
			{"N":"台1_42","1":"ready","3":0},
			{"N":"$ProjectControlOfS7-1200"},
			{"N":"未知变量","1":999}
		]
	}`)

	processMessage(1, &models.MQTTMessage{
		GatewayID: 1,
		Topic:     "datachange_S_KIO_Project",
		Payload:   payload,
		Timestamp: at,
	}, NewChannels(), tags, NewTaskManager())

	floatTag, _ := tags.Get(3901)
	if snap := floatTag.Snapshot(); snap.Value != 28 || snap.Quality != 1 || !snap.LastUpdate.Equal(at) {
		t.Fatalf("expected scaled KIO float update through raw JSONPath: %+v", snap)
	}
	boolTag, _ := tags.Get(4001)
	if snap := boolTag.Snapshot(); snap.Value != 1 || snap.Quality != 1 {
		t.Fatalf("expected KIO bool update: %+v", snap)
	}
	stringTag, _ := tags.Get(4201)
	if snap := stringTag.Snapshot(); !snap.IsString || snap.StrValue != "ready" || snap.Quality != 0 {
		t.Fatalf("expected KIO string update with bad quality: %+v", snap)
	}
	controlTag, _ := tags.Get(5001)
	if snap := controlTag.Snapshot(); snap.Value != 7 || snap.Quality != 1 {
		t.Fatalf("expected PVs fallback update despite remapped var_name: %+v", snap)
	}
	otherTopicTag, _ := tags.Get(9901)
	if snap := otherTopicTag.Snapshot(); !snap.LastUpdate.IsZero() || snap.Value != 0 {
		t.Fatalf("topic index should not update tags from another topic: %+v", snap)
	}
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
	if recover.Action != models.DetectionAlarmActionRecover ||
		recover.Alarm.Status != models.DetectionAlarmStatusClosed ||
		recover.Alarm.ProjectID != ProjectID ||
		recover.Alarm.ProjectCode != "AC-01" ||
		recover.Alarm.TestNo != "T-7" ||
		recover.Alarm.VarName != "temp" ||
		recover.Alarm.AlarmLevel != "HH" ||
		recover.Alarm.LimitValue == nil ||
		*recover.Alarm.LimitValue != 20 ||
		recover.Alarm.RecoverValue == nil ||
		*recover.Alarm.RecoverValue != 18.5 {
		t.Fatalf("unexpected recover alarm event: %+v", recover)
	}
}

func TestProcessMessageEnqueuesDefaultLimitAlarmWithoutDetection(t *testing.T) {
	ProjectID := uint(10)
	limitH := 10.0
	limitHH := 20.0
	deadband := 1.0
	tags := NewTagManager()
	tags.Load([]models.TagConfig{
		{
			VarID:                  1,
			GatewayID:              1,
			SourceTopic:            "topic",
			VarName:                "temp",
			DisplayName:            "Temperature",
			JSONPath:               "temp",
			DataType:               "FLOAT",
			ProjectID:              &ProjectID,
			ProjectCode:            "AC-01",
			ScaleFactor:            1,
			Enabled:                true,
			DefaultAlarmEnabled:    true,
			DefaultLimitH:          &limitH,
			DefaultLimitHH:         &limitHH,
			DefaultLimitDeadband:   deadband,
			DefaultRecoverHoldMS:   100,
			DefaultViolationHoldMS: 0,
		},
	})
	tasks := NewTaskManager()
	channels := NewChannels()
	at := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":12}`), Timestamp: at}, channels, tags, tasks)
	enter := <-channels.Alarm
	if enter.Action != models.DetectionAlarmActionEnter || enter.Alarm.Scope != models.AlarmScopeDefault || enter.Alarm.TaskID != 0 || enter.Alarm.AlarmType != "above_h" || enter.Alarm.DisplayName != "Temperature" {
		t.Fatalf("unexpected default enter alarm event: %+v", enter)
	}

	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":22}`), Timestamp: at.Add(50 * time.Millisecond)}, channels, tags, tasks)
	levelChange := <-channels.Alarm
	if levelChange.Action != models.DetectionAlarmActionLevelChange || levelChange.Alarm.Scope != models.AlarmScopeDefault || levelChange.PreviousAlarmType != "above_h" || levelChange.Alarm.AlarmType != "above_hh" {
		t.Fatalf("unexpected default level change event: %+v", levelChange)
	}

	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":18.5}`), Timestamp: at.Add(100 * time.Millisecond)}, channels, tags, tasks)
	if len(channels.Alarm) != 0 {
		t.Fatalf("expected recover hold to delay default alarm recovery")
	}
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"temp":18.5}`), Timestamp: at.Add(250 * time.Millisecond)}, channels, tags, tasks)
	recover := <-channels.Alarm
	if recover.Action != models.DetectionAlarmActionRecover ||
		recover.Alarm.Scope != models.AlarmScopeDefault ||
		recover.Alarm.Status != models.DetectionAlarmStatusClosed ||
		recover.Alarm.ProjectID != ProjectID ||
		recover.Alarm.ProjectCode != "AC-01" ||
		recover.Alarm.VarName != "temp" ||
		recover.Alarm.DisplayName != "Temperature" ||
		recover.Alarm.AlarmLevel != "HH" ||
		recover.Alarm.LimitValue == nil ||
		*recover.Alarm.LimitValue != 20 ||
		recover.Alarm.RecoverValue == nil ||
		*recover.Alarm.RecoverValue != 18.5 {
		t.Fatalf("unexpected default recover alarm event: %+v", recover)
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
