package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestTaskFlowBuiltinStorageSnapshotFromDataChange(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(1)
	tags.Load([]models.TagConfig{
		{VarID: 100, GatewayID: 1, SourceTopic: "topic", VarName: "start_flag", JSONPath: "start_flag", DataType: "INT", ProjectID: &projectID, ProjectCode: "AC-01", Enabled: true, ScaleFactor: 1},
		{VarID: 101, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-01", Enabled: true, ScaleFactor: 1},
	})
	route := models.DetectionRunStorageRoute{
		TaskID:        1,
		TestNo:        "T-FLOW",
		ProjectID:     projectID,
		VarID:         101,
		RouteID:       1,
		RouteCode:     "temp-task",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  database.ProjectWideTableName(projectID),
		ColumnName:    "temp",
		ColumnType:    "DOUBLE",
		TriggerMode:   models.StoreTriggerOnDetection,
	}
	if err := repo.EnsureProjectWideTable(projectID, []models.DetectionRunStorageRoute{route}); err != nil {
		t.Fatal(err)
	}
	tasks.SetActive(models.DetectionTask{
		ID:            1,
		TestNo:        "T-FLOW",
		ProjectID:     projectID,
		ProjectCode:   "AC-01",
		Mode:          "standard",
		StorageRoutes: []models.DetectionRunStorageRoute{route},
	})
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Load([]models.TaskFlow{{
		ID:              1,
		ProjectID:       projectID,
		FlowCode:        "store-on-flag",
		Name:            "Store on flag",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: "realtime.get(100).value === 1",
		ActionType:      models.TaskFlowActionBuiltinStorageSnapshot,
		TimeoutMS:       3000,
		Priority:        10,
		Vars: []models.TaskFlowVar{
			{FlowID: 1, ProjectID: projectID, VarID: 100, Role: models.TaskFlowVarRoleWatch},
			{FlowID: 1, ProjectID: projectID, VarID: 101, Role: models.TaskFlowVarRoleRead},
		},
	}})
	executor.Start(1)

	at := time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"start_flag":0,"temp":21.5}`), Timestamp: at}, channels, tags, tasks, executor)
	latencyStart := time.Now()
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"start_flag":1,"temp":22.5}`), Timestamp: at.Add(time.Second)}, channels, tags, tasks, executor)

	var stored *models.StoreTask
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case stored = <-channels.Store:
			goto gotTask
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
gotTask:
	if stored == nil || stored.VarID != 101 || stored.Value != 22.5 || len(stored.StorageRoutes) != 1 {
		t.Fatalf("expected task-triggered storage for temp, got %+v", stored)
	}
	t.Logf("task flow builtin storage enqueue latency=%s", time.Since(latencyStart))
	if err := repo.InsertHistoryBatch([]*models.StoreTask{stored}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.HistoryData{}).Where("task_id = ? AND var_id = ?", 1, 101).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("history count=%d", count)
	}
}

func TestTaskFlowJavaScriptAndSQLRun(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(2)
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Start(1)
	flow := models.TaskFlow{
		ID:           2,
		ProjectID:    projectID,
		FlowCode:     "js-sql",
		Name:         "JS SQL",
		Enabled:      true,
		TriggerType:  models.TaskFlowTriggerManual,
		ActionType:   models.TaskFlowActionJavaScript,
		ActionScript: `const rows = db.query("SELECT 1 AS ok", []); log.info("rows=" + rows.length); ({ok: rows[0].ok});`,
		TimeoutMS:    3000,
	}
	if !executor.Submit(flow, TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: projectID}) {
		t.Fatal("submit failed")
	}
	deadline := time.Now().Add(time.Second)
	var run models.TaskFlowRun
	for time.Now().Before(deadline) {
		if err := db.First(&run, "flow_id = ?", flow.ID).Error; err == nil && run.Status == models.TaskFlowStatusSuccess {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if run.Status != models.TaskFlowStatusSuccess || !strings.Contains(run.ScriptLogs, "rows=1") {
		t.Fatalf("unexpected JS run: %+v", run)
	}
	var logs int64
	if err := db.Model(&models.TaskFlowSQLLog{}).Where("run_id = ?", run.ID).Count(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("sql logs=%d", logs)
	}
}

func TestTaskFlowScheduleScannerRunsScheduledFlow(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	executor := NewTaskFlowExecutor(repo, NewTagManager(), NewTaskManager(), NewChannels())
	executor.Load([]models.TaskFlow{{
		ID:                 220,
		ProjectID:          2,
		FlowCode:           "scheduled-js",
		Name:               "Scheduled JS",
		Enabled:            true,
		TriggerType:        models.TaskFlowTriggerSchedule,
		ActionType:         models.TaskFlowActionJavaScript,
		ActionScript:       `log.info("scheduled " + trigger.type); ({trigger_type: trigger.type});`,
		TimeoutMS:          3000,
		ScheduleIntervalMS: 20,
	}})
	executor.Start(1)
	executor.StartScheduleScanner(5 * time.Millisecond)

	run := waitForTaskFlowRunStatus(t, db, 220, models.TaskFlowStatusSuccess, time.Second)
	if run.TriggerType != models.TaskFlowTriggerSchedule || !strings.Contains(run.ScriptLogs, "scheduled schedule") {
		t.Fatalf("unexpected scheduled run: %+v", run)
	}
}

func TestTaskFlowJavaScriptRealtimeMultiVariableAndAuditedWrite(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(12)
	tags.Load([]models.TagConfig{
		{VarID: 1200, GatewayID: 1, SourceTopic: "topic", VarName: "start_flag", JSONPath: "start_flag", DataType: "INT", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
		{VarID: 1201, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
		{VarID: 1202, GatewayID: 1, SourceTopic: "topic", VarName: "pressure_ok", JSONPath: "pressure_ok", DataType: "BOOL", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
		{VarID: 1203, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", VarName: "js_status", JSONPath: "js_status", DataType: "STRING", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
	})
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Load([]models.TaskFlow{{
		ID:          120,
		ProjectID:   projectID,
		FlowCode:    "multi-var-js",
		Name:        "Multi Var JS",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerDataChange,
		ConditionScript: `
			const vars = realtime.getMany([1201, 1202]);
			const temp = realtime.getByName("temp");
			const projectVars = realtime.project();
			temp.value > 30 && vars.length === 2 && vars[1].value === 1 && projectVars.length === 4
		`,
		ActionType: models.TaskFlowActionJavaScript,
		ActionScript: `
			const write = realtime.write(1203, "qualified", {request_id: "js-write-1203"});
			log.info("write=" + write.ok + ", triggered=" + write.triggered);
			({ok: write.ok, triggered: write.triggered, project_count: realtime.project().length});
		`,
		TimeoutMS: 3000,
		Vars: []models.TaskFlowVar{
			{FlowID: 120, ProjectID: projectID, VarID: 1200, Role: models.TaskFlowVarRoleWatch},
			{FlowID: 120, ProjectID: projectID, VarID: 1201, Role: models.TaskFlowVarRoleRead},
			{FlowID: 120, ProjectID: projectID, VarID: 1202, Role: models.TaskFlowVarRoleRead},
			{FlowID: 120, ProjectID: projectID, VarID: 1203, Role: models.TaskFlowVarRoleWrite},
		},
	}})
	executor.Start(1)

	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: []byte(`{"start_flag":1,"temp":33.5,"pressure_ok":true}`), Timestamp: time.Now()}, channels, tags, tasks, executor)
	run := waitForTaskFlowRunStatus(t, db, 120, models.TaskFlowStatusSuccess, time.Second)
	if !strings.Contains(run.ScriptLogs, "write=true, triggered=0") {
		t.Fatalf("expected audited JS realtime write log, got %+v", run)
	}
	statusTag, ok := tags.Get(1203)
	if !ok || statusTag.RuntimeState().StrValue != "qualified" {
		t.Fatalf("expected JS status write, tag=%+v ok=%v", statusTag, ok)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", "1203", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one JS realtime write audit, got %d", auditCount)
	}
}

func TestTaskFlowJavaScriptRealtimeAPIAcceptsStringVarIDs(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(12)
	sourceVarID := int64(9212397624135540848)
	statusVarID := int64(9212397624135540849)
	tags.Load([]models.TagConfig{
		{VarID: sourceVarID, GatewayID: 1, SourceTopic: "topic", VarName: "precise_temp", JSONPath: "precise_temp", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
		{VarID: statusVarID, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", VarName: "precise_status", JSONPath: "precise_status", DataType: "STRING", ProjectID: &projectID, ProjectCode: "AC-12", Enabled: true, ScaleFactor: 1},
	})
	sourceTag, ok := tags.Get(sourceVarID)
	if !ok {
		t.Fatal("expected source tag")
	}
	sourceTag.UpdateNumeric(42.5, time.Now(), 1)
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	flow := models.TaskFlow{
		ID:          121,
		ProjectID:   projectID,
		FlowCode:    "js-string-var-id",
		Name:        "JS String Var ID",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerDataChange,
		ActionType:  models.TaskFlowActionJavaScript,
		ActionScript: `
			const point = realtime.get("9212397624135540848");
			const many = realtime.getMany(["9212397624135540848"]);
			const write = realtime.write("9212397624135540849", "qualified", {request_id: "js-write-big"});
			({
				point_text: point.var_id_text,
				many_text: many[0].var_id_text,
				write_text: write.var_id_text,
				trigger_text: trigger.var_id_text,
				context_text: context["trigger_var_id_text"]
			});
		`,
		TimeoutMS: 3000,
	}
	result := executor.runFlow(flow, TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: projectID, TriggerVarID: sourceVarID, TriggerValue: 1, At: time.Now()}, 121001)
	if result.Status != models.TaskFlowStatusSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	contextMap := result.Result["context"].(map[string]any)
	for _, key := range []string{"default.point_text", "default.many_text", "default.trigger_text", "default.context_text"} {
		if contextMap[key] != "9212397624135540848" {
			t.Fatalf("expected %s to keep exact source var id, context=%+v", key, contextMap)
		}
	}
	if contextMap["default.write_text"] != "9212397624135540849" {
		t.Fatalf("expected write result to keep exact var id, context=%+v", contextMap)
	}
	statusTag, ok := tags.Get(statusVarID)
	if !ok || statusTag.RuntimeState().StrValue != "qualified" {
		t.Fatalf("expected string-id realtime.write to update virtual tag, ok=%v tag=%+v", ok, statusTag)
	}
}

func TestTaskFlowStepsUseTriggerVariableParamsAndContext(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(3)
	tags.Load([]models.TagConfig{
		{VarID: 300, GatewayID: 1, SourceTopic: "topic", VarName: "task_request", JSONPath: "task_request", DataType: "STRING", ProjectID: &projectID, ProjectCode: "AC-03", Enabled: true, ScaleFactor: 1},
		{VarID: 301, GatewayID: 1, SourceTopic: "topic", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-03", Enabled: true, ScaleFactor: 1},
	})
	tag, ok := tags.Get(301)
	if !ok {
		t.Fatal("tag not loaded")
	}
	tag.UpdateNumeric(25.5, time.Now(), 1)
	route := models.DetectionRunStorageRoute{
		TaskID:        30,
		TestNo:        "T-STEPS",
		ProjectID:     projectID,
		VarID:         301,
		RouteID:       30,
		RouteCode:     "temp-step",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  database.ProjectWideTableName(projectID),
		ColumnName:    "temp_step",
		ColumnType:    "DOUBLE",
		TriggerMode:   models.StoreTriggerOnDetection,
	}
	if err := repo.EnsureProjectWideTable(projectID, []models.DetectionRunStorageRoute{route}); err != nil {
		t.Fatal(err)
	}
	tasks.SetActive(models.DetectionTask{
		ID:            30,
		TestNo:        "T-STEPS",
		ProjectID:     projectID,
		ProjectCode:   "AC-03",
		Mode:          "standard",
		StorageRoutes: []models.DetectionRunStorageRoute{route},
	})
	steps := []map[string]any{
		{
			"code":   "remember",
			"module": models.TaskFlowActionBuiltinContextSet,
			"params": map[string]any{
				"duration_sec": map[string]any{"source": "trigger_param", "key": "duration_sec"},
			},
		},
		{
			"code":   "store",
			"module": models.TaskFlowActionBuiltinStorageSnapshot,
			"params": map[string]any{
				"project_id": map[string]any{"source": "trigger_param", "key": "project_id"},
			},
		},
		{
			"code":   "script",
			"module": models.TaskFlowActionJavaScript,
			"script": `log.info("duration=" + context.duration_sec); ({duration: context.duration_sec});`,
		},
	}
	stepsJSON, _ := json.Marshal(steps)
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Start(1)
	flow := models.TaskFlow{
		ID:              3,
		ProjectID:       projectID,
		FlowCode:        "variable-steps",
		Name:            "Variable Steps",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "store"`,
		StepsJSON:       string(stepsJSON),
		TimeoutMS:       3000,
		Vars: []models.TaskFlowVar{
			{FlowID: 3, ProjectID: projectID, VarID: 300, Role: models.TaskFlowVarRoleWatch},
			{FlowID: 3, ProjectID: projectID, VarID: 301, Role: models.TaskFlowVarRoleRead},
		},
	}
	executor.Load([]models.TaskFlow{flow})
	request, _ := json.Marshal(map[string]any{"command": "store", "project_id": projectID, "duration_sec": 7200})
	payload, _ := json.Marshal(map[string]any{"task_request": string(request)})
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: payload, Timestamp: time.Now()}, channels, tags, tasks, executor)
	var stored *models.StoreTask
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case stored = <-channels.Store:
			goto gotTask
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
gotTask:
	if stored == nil || stored.VarID != 301 || stored.Value != 25.5 {
		t.Fatalf("expected step-triggered storage, got %+v", stored)
	}
	var run models.TaskFlowRun
	runDeadline := time.Now().Add(time.Second)
	for time.Now().Before(runDeadline) {
		if err := db.First(&run, "flow_id = ?", flow.ID).Error; err == nil && run.Status == models.TaskFlowStatusSuccess {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if run.Status != models.TaskFlowStatusSuccess || !strings.Contains(run.ScriptLogs, "duration=7200") || !strings.Contains(run.InputSnapshot, "trigger_params") {
		t.Fatalf("unexpected stepped run: %+v", run)
	}
}

func TestTaskFlowParamBindingsSupportOptionalAndDefault(t *testing.T) {
	ctx := newTaskFlowRunContext(
		models.TaskFlow{ID: 8, ProjectID: 8, FlowCode: "bindings"},
		TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: 8, TriggerValue: `{"project_id":8}`},
		8001,
	)
	params, err := resolveTaskFlowParams(map[string]any{
		"project_id":      map[string]any{"source": "trigger_param", "key": "project_id"},
		"optional_note":   map[string]any{"source": "trigger_param", "key": "note", "optional": true},
		"enable_alarm":    map[string]any{"source": "trigger_param", "key": "enable_alarm", "default": true},
		"context_missing": map[string]any{"source": "context", "key": "missing", "optional": true},
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if params["project_id"] != float64(8) || params["optional_note"] != nil || params["context_missing"] != nil || params["enable_alarm"] != true {
		t.Fatalf("unexpected resolved params: %+v", params)
	}
	if _, err := resolveTaskFlowParams(map[string]any{
		"required": map[string]any{"source": "trigger_param", "key": "missing"},
	}, ctx); err == nil {
		t.Fatal("expected missing required trigger_param to fail")
	}
}

func TestTaskFlowStepsAcceptSingleObjectJSON(t *testing.T) {
	steps, err := taskFlowSteps(models.TaskFlow{
		FlowCode:  "single-step",
		StepsJSON: `{"code":"write","module":"builtin.write_variable","params":{"var_id":1,"value":2}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Code != "write" || steps[0].Module != models.TaskFlowActionBuiltinWriteVariable {
		t.Fatalf("unexpected single-object steps: %+v", steps)
	}
}

func TestTaskFlowStepsRejectDuplicateCode(t *testing.T) {
	_, err := taskFlowSteps(models.TaskFlow{
		FlowCode:  "duplicate-step-code",
		StepsJSON: `[{"code":"same","module":"builtin.context_set","params":{"a":1}},{"code":" same ","module":"builtin.context_set","params":{"b":2}}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate task flow step code: same") {
		t.Fatalf("expected duplicate step code error, got %v", err)
	}
}

func TestTaskFlowDetectionModulesCoverStorageAlarmEndPauseAndFeatures(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(4)
	project := &models.Project{ProjectCode: "AC-04", Name: "Project 4", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	projectID = project.ID
	limitH := 30.0
	tempTag := models.TagConfig{
		VarID:       401,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &projectID,
		ProjectCode: "AC-04",
		Enabled:     true,
		ScaleFactor: 1,
	}
	if err := repo.CreateTag(&tempTag); err != nil {
		t.Fatal(err)
	}
	route, err := repo.EnsureDefaultStorageRouteForTag(tempTag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateStorageRoute(route.ID, map[string]interface{}{
		"enabled":        true,
		"trigger_mode":   models.StoreTriggerOnDetection,
		"store_on_start": true,
	}); err != nil {
		t.Fatal(err)
	}
	standard := &models.DetectionStandard{StandardCode: "STD-FLOW", Name: "Flow Standard", ProjectID: &projectID, ProjectCode: "AC-04", Mode: "task_flow", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:        tempTag.VarID,
		VarName:      "temp",
		CheckEnabled: true,
		AlarmEnabled: true,
		StoreEnabled: true,
		CheckOnStart: true,
		LimitH:       &limitH,
	}}); err != nil {
		t.Fatal(err)
	}
	tags.Load([]models.TagConfig{tempTag})
	temp, ok := tags.Get(tempTag.VarID)
	if !ok {
		t.Fatal("temp tag not loaded")
	}
	temp.UpdateNumeric(35, time.Now(), 1)
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)

	startResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          40,
		ProjectID:   projectID,
		FlowCode:    "start-detection",
		Name:        "Start Detection",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "start",
			"module": models.TaskFlowActionBuiltinStartDetectionRun,
			"params": map[string]any{
				"project_id":            projectID,
				"test_no":               "TF-START",
				"mode":                  "task_flow",
				"standard_id":           standard.ID,
				"duration_sec":          60,
				"enable_storage":        true,
				"enable_alarm":          true,
				"auto_stop_on_duration": false,
			},
		}}),
		TimeoutMS: 3000,
	})
	taskID := uintFromAny(startResult.Result["context"].(map[string]any)["task_id"])
	if taskID == 0 {
		t.Fatalf("expected start module to expose task id: %+v", startResult.Result)
	}
	if _, ok := tasks.ActiveForProject(projectID); !ok {
		t.Fatal("expected started detection task to become runtime active")
	}
	storeTask := receiveStoreTask(t, channels)
	if storeTask.VarID != tempTag.VarID || storeTask.Value != 35 {
		t.Fatalf("unexpected start snapshot: %+v", storeTask)
	}
	if err := repo.InsertHistoryBatch([]*models.StoreTask{storeTask}); err != nil {
		t.Fatal(err)
	}
	alarmEvent := receiveAlarmEvent(t, channels)
	if alarmEvent.Action != models.DetectionAlarmActionEnter || alarmEvent.Alarm.AlarmType != "above_h" {
		t.Fatalf("unexpected start alarm: %+v", alarmEvent)
	}
	if err := repo.CreateDetectionLimitAlarms([]models.DetectionLimitAlarm{alarmEvent.Alarm}); err != nil {
		t.Fatal(err)
	}
	muteResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          47,
		ProjectID:   projectID,
		FlowCode:    "mute-alarms",
		Name:        "Mute Alarms",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "mute",
			"module": models.TaskFlowActionBuiltinMuteDetectionAlarms,
			"params": map[string]any{"task_id": taskID},
		}}),
		TimeoutMS: 3000,
	})
	if muted := int(toFloat64(muteResult.Result["context"].(map[string]any)["mute.muted"])); muted != 1 {
		t.Fatalf("expected one muted alarm, got result=%+v", muteResult.Result)
	}
	runDetectionFlow(t, executor, models.TaskFlow{
		ID:          48,
		ProjectID:   projectID,
		FlowCode:    "prepare-storage",
		Name:        "Prepare Storage",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "prepare",
			"module": models.TaskFlowActionBuiltinStoragePrepare,
			"params": map[string]any{"task_id": taskID},
		}}),
		TimeoutMS: 3000,
	})
	newLimitH := 40.0
	runDetectionFlow(t, executor, models.TaskFlow{
		ID:          49,
		ProjectID:   projectID,
		FlowCode:    "update-limits",
		Name:        "Update Limits",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "limits",
			"module": models.TaskFlowActionBuiltinUpdateDetectionLimits,
			"params": map[string]any{"task_id": taskID, "var_id": tempTag.VarID, "limit_h": newLimitH, "alarm_enabled": true, "check_cycle_ms": 1000},
		}}),
		TimeoutMS: 3000,
	})
	updatedItem, err := repo.UpdateDetectionRunStandardItem(taskID, tempTag.VarID, nil)
	if err != nil || updatedItem.LimitH == nil || *updatedItem.LimitH != newLimitH || updatedItem.CheckCycleMS != 1000 {
		t.Fatalf("expected updated runtime limit item, got %+v err=%v", updatedItem, err)
	}

	runDetectionFlow(t, executor, models.TaskFlow{
		ID:          41,
		ProjectID:   projectID,
		FlowCode:    "pause-resume-stop",
		Name:        "Pause Resume Stop",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{
			{"code": "pause", "module": models.TaskFlowActionBuiltinPauseDetectionRun, "params": map[string]any{"task_id": taskID, "reason": "test pause"}},
			{"code": "resume", "module": models.TaskFlowActionBuiltinResumeDetectionRun, "params": map[string]any{"task_id": taskID}},
			{"code": "stop", "module": models.TaskFlowActionBuiltinStopDetectionRun, "params": map[string]any{"task_id": taskID, "end_type": models.DetectionEndManualStop, "reason": "manual done"}},
			{"code": "features", "module": models.TaskFlowActionBuiltinRefreshFeatures, "params": map[string]any{"task_id": taskID}},
		}),
		TimeoutMS: 3000,
	})
	stopped, err := repo.GetDetectionTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != models.DetectionStatusStopped || stopped.EndType != models.DetectionEndManualStop {
		t.Fatalf("expected manual stopped task, got %+v", stopped)
	}
	features, err := repo.ListDetectionRunFeatures(taskID)
	if err != nil || len(features) != 1 || features[0].AvgValue == nil || *features[0].AvgValue != 35 {
		t.Fatalf("unexpected features: %+v err=%v", features, err)
	}

	temp.UpdateNumeric(36, time.Now().Add(time.Second), 1)
	noIOResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          42,
		ProjectID:   projectID,
		FlowCode:    "start-no-io",
		Name:        "Start Without IO",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "start",
			"module": models.TaskFlowActionBuiltinStartDetectionRun,
			"params": map[string]any{
				"project_id":     projectID,
				"test_no":        "TF-NO-IO",
				"standard_id":    standard.ID,
				"enable_storage": false,
				"enable_alarm":   false,
			},
		}}),
		TimeoutMS: 3000,
	})
	noIOTaskID := uintFromAny(noIOResult.Result["context"].(map[string]any)["task_id"])
	assertNoStoreOrAlarm(t, channels)
	if _, err := executor.stopDetectionRun(noIOTaskID, "cleanup", models.DetectionEndTaskFlowStop); err != nil {
		t.Fatal(err)
	}

	temp.UpdateNumeric(25, time.Now().Add(2*time.Second), 1)
	qualifiedStart := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          43,
		ProjectID:   projectID,
		FlowCode:    "qualified-start",
		Name:        "Qualified Start",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "start",
			"module": models.TaskFlowActionBuiltinStartDetectionRun,
			"params": map[string]any{"project_id": projectID, "test_no": "TF-QUALIFIED", "standard_id": standard.ID, "enable_storage": false, "enable_alarm": true},
		}}),
		TimeoutMS: 3000,
	})
	qualifiedTaskID := uintFromAny(qualifiedStart.Result["context"].(map[string]any)["task_id"])
	runDetectionFlow(t, executor, models.TaskFlow{
		ID:          44,
		ProjectID:   projectID,
		FlowCode:    "qualified-stop",
		Name:        "Qualified Stop",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "qualified",
			"module": models.TaskFlowActionBuiltinQualifiedHoldGuard,
			"params": map[string]any{"task_id": qualifiedTaskID, "qualified_hold_ms": 0},
		}}),
		TimeoutMS: 3000,
	})
	qualifiedStopped, err := repo.GetDetectionTask(qualifiedTaskID)
	if err != nil || qualifiedStopped.EndType != models.DetectionEndQualifiedHold {
		t.Fatalf("expected qualified hold stop, got %+v err=%v", qualifiedStopped, err)
	}

	durationStart := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          45,
		ProjectID:   projectID,
		FlowCode:    "duration-start",
		Name:        "Duration Start",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "start",
			"module": models.TaskFlowActionBuiltinStartDetectionRun,
			"params": map[string]any{"project_id": projectID, "test_no": "TF-DURATION", "standard_id": standard.ID, "duration_sec": 1, "enable_storage": false, "enable_alarm": false},
		}}),
		TimeoutMS: 3000,
	})
	durationTaskID := uintFromAny(durationStart.Result["context"].(map[string]any)["task_id"])
	past := time.Now().Add(-time.Second)
	if err := db.Model(&models.DetectionTask{}).Where("id = ?", durationTaskID).Update("expected_end_at", past).Error; err != nil {
		t.Fatal(err)
	}
	runDetectionFlow(t, executor, models.TaskFlow{
		ID:          46,
		ProjectID:   projectID,
		FlowCode:    "duration-stop",
		Name:        "Duration Stop",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "duration",
			"module": models.TaskFlowActionBuiltinFixedDurationGuard,
			"params": map[string]any{"task_id": durationTaskID},
		}}),
		TimeoutMS: 3000,
	})
	durationStopped, err := repo.GetDetectionTask(durationTaskID)
	if err != nil || durationStopped.EndType != models.DetectionEndFixedDuration {
		t.Fatalf("expected fixed duration stop, got %+v err=%v", durationStopped, err)
	}
}

func TestTaskFlowStringVirtualPayloadStartsCustomDetection(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-09", Name: "Project 9", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	requestTag := models.TagConfig{
		VarID:       901,
		SourceType:  models.TagSourceVirtual,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "task_request",
		RawName:     "task_request",
		VarName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	tempTag := models.TagConfig{
		VarID:       902,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	if err := repo.CreateTag(&requestTag); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTag(&tempTag); err != nil {
		t.Fatal(err)
	}
	route, err := repo.EnsureDefaultStorageRouteForTag(tempTag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateStorageRoute(route.ID, map[string]interface{}{"enabled": true, "trigger_mode": models.StoreTriggerOnDetection, "store_on_start": true}); err != nil {
		t.Fatal(err)
	}
	tags.Load([]models.TagConfig{requestTag, tempTag})
	temp, ok := tags.Get(tempTag.VarID)
	if !ok {
		t.Fatal("temp tag not loaded")
	}
	temp.UpdateNumeric(45, time.Now(), 1)

	steps := mustStepsJSON(t, []map[string]any{{
		"code":   "start",
		"module": models.TaskFlowActionBuiltinStartDetectionRun,
		"params": map[string]any{
			"project_id":          map[string]any{"source": "trigger_param", "key": "project_id"},
			"test_no":             map[string]any{"source": "trigger_param", "key": "test_no"},
			"custom_items":        map[string]any{"source": "trigger_param", "key": "custom_items"},
			"process_params":      map[string]any{"source": "trigger_param", "key": "process_params", "optional": true},
			"plc_writes":          map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true},
			"limit_check_enabled": map[string]any{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
			"end_policy":          map[string]any{"source": "trigger_param", "key": "end_policy"},
			"duration_sec":        map[string]any{"source": "trigger_param", "key": "duration_sec"},
			"enable_storage":      true,
			"enable_alarm":        true,
		},
	}})
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	flow := models.TaskFlow{
		ID:              90,
		ProjectID:       project.ID,
		FlowCode:        "request-custom-detection",
		Name:            "Request Custom Detection",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start_custom_detection"`,
		StepsJSON:       steps,
		TimeoutMS:       3000,
		Vars:            []models.TaskFlowVar{{FlowID: 90, ProjectID: project.ID, VarID: requestTag.VarID, Role: models.TaskFlowVarRoleWatch}},
	}
	executor.Load([]models.TaskFlow{flow})
	executor.Start(1)

	limitH := 30.0
	request, _ := json.Marshal(map[string]any{
		"command":             "start_custom_detection",
		"project_id":          project.ID,
		"test_no":             "TF-CUSTOM-STRING",
		"limit_check_enabled": true,
		"end_policy":          models.DetectionEndPolicyFixedDuration,
		"duration_sec":        60,
		"process_params":      map[string]any{"inlet_area_m2": 1.25},
		"plc_writes":          []map[string]any{{"var_id": 3901, "value_from": "process_params.inlet_area_m2"}},
		"custom_items": []map[string]any{{
			"var_id":         tempTag.VarID,
			"var_name":       tempTag.VarName,
			"check_enabled":  true,
			"alarm_enabled":  true,
			"store_enabled":  true,
			"check_on_start": true,
			"limit_h":        limitH,
		}},
	})
	payload, _ := json.Marshal(map[string]any{"task_request": string(request)})
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: payload, Timestamp: time.Now()}, channels, tags, tasks, executor)

	waitForTaskFlowRunStatus(t, db, flow.ID, models.TaskFlowStatusSuccess, time.Second)
	var task models.DetectionTask
	if err := db.First(&task, "test_no = ?", "TF-CUSTOM-STRING").Error; err != nil {
		t.Fatal(err)
	}
	if task.StandardID != nil || task.StandardCode != "custom" || !task.LimitCheckEnabled || task.EndPolicy != models.DetectionEndPolicyFixedDuration || task.CustomConfigJSON == "" {
		t.Fatalf("unexpected custom task: %+v", task)
	}
	if !strings.Contains(task.CustomConfigJSON, `"process_params"`) || !strings.Contains(task.CustomConfigJSON, `"inlet_area_m2"`) || !strings.Contains(task.CustomConfigJSON, `"plc_writes"`) {
		t.Fatalf("expected process params and plc writes frozen, custom_config_json=%s", task.CustomConfigJSON)
	}
	item, err := repo.UpdateDetectionRunStandardItem(task.ID, tempTag.VarID, nil)
	if err != nil || item.StandardID != 0 || item.StandardItemID != 0 || item.LimitH == nil || *item.LimitH != limitH {
		t.Fatalf("unexpected custom run item: %+v err=%v", item, err)
	}
	storeTask := receiveStoreTask(t, channels)
	if storeTask.TaskID != task.ID || storeTask.VarID != tempTag.VarID {
		t.Fatalf("unexpected start store task: %+v", storeTask)
	}
	alarmEvent := receiveAlarmEvent(t, channels)
	if alarmEvent.Alarm.TaskID != task.ID || alarmEvent.Alarm.VarID != tempTag.VarID || alarmEvent.Alarm.AlarmType != "above_h" {
		t.Fatalf("unexpected custom start alarm: %+v", alarmEvent)
	}
}

func TestTaskFlowStringPayloadWritesPLCBeforeStartingDetection(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-15", Name: "Project 15", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	requestTag := models.TagConfig{
		VarID:       1501,
		SourceType:  models.TagSourceVirtual,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "task_request",
		RawName:     "task_request",
		VarName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	tempTag := models.TagConfig{
		VarID:       1502,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	controlTag := models.TagConfig{
		VarID:         1503,
		GatewayID:     1,
		SourceTopic:   "topic",
		SourcePath:    "plc.inlet_area",
		RawName:       "plc.inlet_area",
		VarName:       "plc_inlet_area",
		JSONPath:      "plc.inlet_area",
		DataType:      "FLOAT",
		ProjectID:     &project.ID,
		ProjectCode:   project.ProjectCode,
		Enabled:       true,
		ScaleFactor:   1,
		RWMode:        "W",
		Writable:      true,
		WritePath:     "plc.inlet_area",
		WriteDataType: "FLOAT",
	}
	for _, cfg := range []*models.TagConfig{&requestTag, &tempTag, &controlTag} {
		if err := repo.CreateTag(cfg); err != nil {
			t.Fatal(err)
		}
	}
	tags.Load([]models.TagConfig{requestTag, tempTag, controlTag})

	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	var writes []TaskFlowVariableWriteInput
	executor.SetVariableWriter(func(_ context.Context, input TaskFlowVariableWriteInput) (map[string]any, error) {
		var existing int64
		if err := db.Model(&models.DetectionTask{}).Where("test_no = ?", "TF-PLC-FIRST").Count(&existing).Error; err != nil {
			t.Fatal(err)
		}
		if existing != 0 {
			t.Fatalf("PLC control write must run before detection start, existing tasks=%d", existing)
		}
		writes = append(writes, input)
		return map[string]any{
			"var_id":            input.VarID,
			"value":             input.Value,
			"wait_ack":          input.WaitAck,
			"ack_timeout_sec":   input.AckTimeoutSec,
			"request_id":        input.RequestID,
			"project_confirmed": true,
			"source_type":       models.TagSourceMQTT,
		}, nil
	})
	flow := models.TaskFlow{
		ID:              150,
		ProjectID:       project.ID,
		FlowCode:        "request-plc-then-detection",
		Name:            "Request PLC Then Detection",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start_detection"`,
		StepsJSON: mustStepsJSON(t, []map[string]any{
			{
				"code":   "control",
				"module": models.TaskFlowActionBuiltinWriteControlVariables,
				"params": map[string]any{
					"items": map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true},
				},
			},
			{
				"code":   "start",
				"module": models.TaskFlowActionBuiltinStartDetectionRun,
				"params": map[string]any{
					"project_id":          map[string]any{"source": "trigger_param", "key": "project_id"},
					"test_no":             map[string]any{"source": "trigger_param", "key": "test_no"},
					"custom_items":        map[string]any{"source": "trigger_param", "key": "custom_items"},
					"process_params":      map[string]any{"source": "trigger_param", "key": "process_params"},
					"plc_writes":          map[string]any{"source": "trigger_param", "key": "plc_writes"},
					"limit_check_enabled": map[string]any{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
					"end_policy":          map[string]any{"source": "trigger_param", "key": "end_policy", "default": models.DetectionEndPolicyManual},
					"enable_storage":      false,
					"enable_alarm":        false,
				},
			},
		}),
		TimeoutMS: 3000,
		Vars:      []models.TaskFlowVar{{FlowID: 150, ProjectID: project.ID, VarID: requestTag.VarID, Role: models.TaskFlowVarRoleWatch}},
	}
	executor.Load([]models.TaskFlow{flow})
	executor.Start(1)

	limitH := 50.0
	request, _ := json.Marshal(map[string]any{
		"command":             "start_detection",
		"project_id":          project.ID,
		"test_no":             "TF-PLC-FIRST",
		"process_params":      map[string]any{"inlet_area_m2": 1.25},
		"plc_writes":          []map[string]any{{"var_id": controlTag.VarID, "value_from": "process_params.inlet_area_m2", "wait_ack": true, "ack_timeout_sec": 9}},
		"limit_check_enabled": true,
		"custom_items": []map[string]any{{
			"var_id":         tempTag.VarID,
			"var_name":       tempTag.VarName,
			"check_enabled":  true,
			"alarm_enabled":  false,
			"store_enabled":  false,
			"check_on_start": true,
			"limit_h":        limitH,
		}},
	})
	payload, _ := json.Marshal(map[string]any{"task_request": string(request)})
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: payload, Timestamp: time.Now()}, channels, tags, tasks, executor)

	waitForTaskFlowRunStatus(t, db, flow.ID, models.TaskFlowStatusSuccess, time.Second)
	if len(writes) != 1 {
		t.Fatalf("expected one PLC write, got %d", len(writes))
	}
	if writes[0].VarID != controlTag.VarID || writes[0].Value != 1.25 || !writes[0].WaitAck || writes[0].AckTimeoutSec != 9 {
		t.Fatalf("unexpected PLC write input: %+v", writes[0])
	}
	var task models.DetectionTask
	if err := db.First(&task, "test_no = ?", "TF-PLC-FIRST").Error; err != nil {
		t.Fatal(err)
	}
	if task.CustomConfigJSON == "" || !strings.Contains(task.CustomConfigJSON, `"process_params"`) || !strings.Contains(task.CustomConfigJSON, `"plc_writes"`) || !strings.Contains(task.CustomConfigJSON, `"inlet_area_m2"`) {
		t.Fatalf("expected process and PLC parameters frozen, custom_config_json=%s", task.CustomConfigJSON)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", controlTag.VarID, "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one PLC write audit, got %d", auditCount)
	}
}

func TestTaskFlowStringPayloadStopsDetectionWhenPLCParamMissing(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-16", Name: "Project 16", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	requestTag := models.TagConfig{
		VarID:       1601,
		SourceType:  models.TagSourceVirtual,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "task_request",
		RawName:     "task_request",
		VarName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	tempTag := models.TagConfig{
		VarID:       1602,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	controlTag := models.TagConfig{
		VarID:         1603,
		GatewayID:     1,
		SourceTopic:   "topic",
		SourcePath:    "plc.inlet_area",
		RawName:       "plc.inlet_area",
		VarName:       "plc_inlet_area",
		JSONPath:      "plc.inlet_area",
		DataType:      "FLOAT",
		ProjectID:     &project.ID,
		ProjectCode:   project.ProjectCode,
		Enabled:       true,
		ScaleFactor:   1,
		RWMode:        "W",
		Writable:      true,
		WritePath:     "plc.inlet_area",
		WriteDataType: "FLOAT",
	}
	for _, cfg := range []*models.TagConfig{&requestTag, &tempTag, &controlTag} {
		if err := repo.CreateTag(cfg); err != nil {
			t.Fatal(err)
		}
	}
	tags.Load([]models.TagConfig{requestTag, tempTag, controlTag})

	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	var writerCalled bool
	executor.SetVariableWriter(func(_ context.Context, input TaskFlowVariableWriteInput) (map[string]any, error) {
		writerCalled = true
		return map[string]any{"var_id": input.VarID, "value": input.Value}, nil
	})
	flow := models.TaskFlow{
		ID:              160,
		ProjectID:       project.ID,
		FlowCode:        "request-plc-param-missing",
		Name:            "Request PLC Param Missing",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start_detection"`,
		StepsJSON: mustStepsJSON(t, []map[string]any{
			{
				"code":   "control",
				"module": models.TaskFlowActionBuiltinWriteControlVariables,
				"params": map[string]any{
					"items": map[string]any{"source": "trigger_param", "key": "plc_writes"},
				},
			},
			{
				"code":   "start",
				"module": models.TaskFlowActionBuiltinStartDetectionRun,
				"params": map[string]any{
					"project_id":     map[string]any{"source": "trigger_param", "key": "project_id"},
					"test_no":        map[string]any{"source": "trigger_param", "key": "test_no"},
					"custom_items":   map[string]any{"source": "trigger_param", "key": "custom_items"},
					"enable_storage": false,
					"enable_alarm":   false,
				},
			},
		}),
		TimeoutMS: 3000,
		Vars:      []models.TaskFlowVar{{FlowID: 160, ProjectID: project.ID, VarID: requestTag.VarID, Role: models.TaskFlowVarRoleWatch}},
	}
	executor.Load([]models.TaskFlow{flow})
	executor.Start(1)

	request, _ := json.Marshal(map[string]any{
		"command":    "start_detection",
		"project_id": project.ID,
		"test_no":    "TF-PLC-MISSING",
		"process_params": map[string]any{
			"inlet_area_m2": 1.25,
		},
		"plc_writes": []map[string]any{{
			"var_id":     controlTag.VarID,
			"value_from": "process_params.missing_inlet_area",
		}},
		"custom_items": []map[string]any{{
			"var_id":        tempTag.VarID,
			"var_name":      tempTag.VarName,
			"check_enabled": true,
			"store_enabled": false,
			"limit_h":       50,
		}},
	})
	payload, _ := json.Marshal(map[string]any{"task_request": string(request)})
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: payload, Timestamp: time.Now()}, channels, tags, tasks, executor)

	run := waitForTaskFlowRunStatus(t, db, flow.ID, models.TaskFlowStatusFailed, time.Second)
	if writerCalled {
		t.Fatalf("variable writer must not be called when value_from cannot be resolved")
	}
	if !strings.Contains(run.ErrorMessage, "missing_inlet_area") {
		t.Fatalf("expected missing value_from in error, got %q", run.ErrorMessage)
	}
	var taskCount int64
	if err := db.Model(&models.DetectionTask{}).Where("test_no = ?", "TF-PLC-MISSING").Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("detection task must not start after PLC parameter failure, count=%d", taskCount)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", controlTag.VarID, "failed").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one failed PLC write audit, got %d", auditCount)
	}
}

func TestTaskFlowStringPayloadStopsDetectionWhenPLCWriteFails(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-17", Name: "Project 17", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	requestTag := models.TagConfig{
		VarID:       1701,
		SourceType:  models.TagSourceVirtual,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "task_request",
		RawName:     "task_request",
		VarName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	tempTag := models.TagConfig{
		VarID:       1702,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	controlTag := models.TagConfig{
		VarID:         1703,
		GatewayID:     1,
		SourceTopic:   "topic",
		SourcePath:    "plc.inlet_area",
		RawName:       "plc.inlet_area",
		VarName:       "plc_inlet_area",
		JSONPath:      "plc.inlet_area",
		DataType:      "FLOAT",
		ProjectID:     &project.ID,
		ProjectCode:   project.ProjectCode,
		Enabled:       true,
		ScaleFactor:   1,
		RWMode:        "W",
		Writable:      true,
		WritePath:     "plc.inlet_area",
		WriteDataType: "FLOAT",
	}
	for _, cfg := range []*models.TagConfig{&requestTag, &tempTag, &controlTag} {
		if err := repo.CreateTag(cfg); err != nil {
			t.Fatal(err)
		}
	}
	tags.Load([]models.TagConfig{requestTag, tempTag, controlTag})

	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	var writes []TaskFlowVariableWriteInput
	executor.SetVariableWriter(func(_ context.Context, input TaskFlowVariableWriteInput) (map[string]any, error) {
		writes = append(writes, input)
		return nil, errors.New("plc write rejected")
	})
	flow := models.TaskFlow{
		ID:              161,
		ProjectID:       project.ID,
		FlowCode:        "request-plc-write-fails",
		Name:            "Request PLC Write Fails",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start_detection"`,
		StepsJSON: mustStepsJSON(t, []map[string]any{
			{
				"code":   "control",
				"module": models.TaskFlowActionBuiltinWriteControlVariables,
				"params": map[string]any{
					"items": map[string]any{"source": "trigger_param", "key": "plc_writes"},
				},
			},
			{
				"code":   "start",
				"module": models.TaskFlowActionBuiltinStartDetectionRun,
				"params": map[string]any{
					"project_id":     map[string]any{"source": "trigger_param", "key": "project_id"},
					"test_no":        map[string]any{"source": "trigger_param", "key": "test_no"},
					"custom_items":   map[string]any{"source": "trigger_param", "key": "custom_items"},
					"enable_storage": false,
					"enable_alarm":   false,
				},
			},
		}),
		TimeoutMS: 3000,
		Vars:      []models.TaskFlowVar{{FlowID: 161, ProjectID: project.ID, VarID: requestTag.VarID, Role: models.TaskFlowVarRoleWatch}},
	}
	executor.Load([]models.TaskFlow{flow})
	executor.Start(1)

	request, _ := json.Marshal(map[string]any{
		"command":        "start_detection",
		"project_id":     project.ID,
		"test_no":        "TF-PLC-WRITE-FAIL",
		"process_params": map[string]any{"inlet_area_m2": 1.25},
		"plc_writes": []map[string]any{{
			"var_id":     controlTag.VarID,
			"value_from": "process_params.inlet_area_m2",
			"wait_ack":   true,
		}},
		"custom_items": []map[string]any{{
			"var_id":        tempTag.VarID,
			"var_name":      tempTag.VarName,
			"check_enabled": true,
			"store_enabled": false,
			"limit_h":       50,
		}},
	})
	payload, _ := json.Marshal(map[string]any{"task_request": string(request)})
	processMessage(1, &models.MQTTMessage{GatewayID: 1, Topic: "topic", Payload: payload, Timestamp: time.Now()}, channels, tags, tasks, executor)

	run := waitForTaskFlowRunStatus(t, db, flow.ID, models.TaskFlowStatusFailed, time.Second)
	if len(writes) != 1 {
		t.Fatalf("expected one attempted PLC write, got %d", len(writes))
	}
	if writes[0].VarID != controlTag.VarID || writes[0].Value != 1.25 || !writes[0].WaitAck {
		t.Fatalf("unexpected write input: %+v", writes[0])
	}
	if !strings.Contains(run.ErrorMessage, "plc write rejected") {
		t.Fatalf("expected PLC writer error in task-flow run, got %q", run.ErrorMessage)
	}
	var taskCount int64
	if err := db.Model(&models.DetectionTask{}).Where("test_no = ?", "TF-PLC-WRITE-FAIL").Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("detection task must not start after PLC write failure, count=%d", taskCount)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", controlTag.VarID, "failed").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one failed PLC write audit, got %d", auditCount)
	}
}

func TestTaskFlowStatusForErrDoesNotClassifyBusinessTimeoutTextAsTimeout(t *testing.T) {
	if got := statusForErr(nil); got != models.TaskFlowStatusSuccess {
		t.Fatalf("nil error status = %s", got)
	}
	if got := statusForErr(errors.New("plc ack timeout")); got != models.TaskFlowStatusFailed {
		t.Fatalf("business error with timeout text status = %s", got)
	}
	if got := statusForErr(errors.New("timeout")); got != models.TaskFlowStatusTimeout {
		t.Fatalf("exact timeout status = %s", got)
	}
	if got := statusForErr(context.DeadlineExceeded); got != models.TaskFlowStatusTimeout {
		t.Fatalf("context deadline status = %s", got)
	}
}

func TestTaskFlowJavaScriptTimeoutStatus(t *testing.T) {
	executor := NewTaskFlowExecutor(database.NewRepository(newTaskFlowTestDB(t)), NewTagManager(), NewTaskManager(), NewChannels())
	flow := models.TaskFlow{
		ID:           162,
		ProjectID:    1,
		FlowCode:     "js-timeout",
		Name:         "JS Timeout",
		Enabled:      true,
		TriggerType:  models.TaskFlowTriggerManual,
		ActionType:   models.TaskFlowActionJavaScript,
		ActionScript: `while (true) {}`,
		TimeoutMS:    10,
	}

	started := time.Now()
	result := executor.runFlow(flow, TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: 1, At: started}, 16201)
	if result.Status != models.TaskFlowStatusTimeout {
		t.Fatalf("expected timeout status, got status=%s err=%v", result.Status, result.Err)
	}
	if result.Err == nil {
		t.Fatalf("expected timeout error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("javascript timeout took too long: %s", elapsed)
	}
}

func TestTaskFlowStringParamPayloadCapacityGuard(t *testing.T) {
	items := make([]map[string]any, 0, 520)
	for i := 0; i < 520; i++ {
		items = append(items, map[string]any{"var_id": i + 1, "var_name": "var", "limit_h": 100})
	}
	payload, _ := json.Marshal(map[string]any{"command": "start_custom_detection", "project_id": 1, "custom_items": items})
	params := taskFlowParamsFromTrigger(string(payload))
	if params["command"] != "start_custom_detection" || len(params["custom_items"].([]any)) != 520 {
		t.Fatalf("expected large custom_items payload to parse, got keys=%+v size=%d", params, len(payload))
	}
	oversize := `{"command":"start","padding":"` + strings.Repeat("x", taskFlowStringParamMaxBytes) + `"}`
	params = taskFlowParamsFromTrigger(oversize)
	if params["_error"] == nil {
		t.Fatalf("expected oversize payload error")
	}
}

func TestTaskFlowStringPayloadPerformanceBudget(t *testing.T) {
	items := make([]map[string]any, 0, 520)
	for i := 0; i < 520; i++ {
		items = append(items, map[string]any{
			"var_id":            i + 1,
			"var_name":          "kio_var",
			"display_name":      "变量",
			"check_enabled":     true,
			"alarm_enabled":     true,
			"store_enabled":     true,
			"check_on_start":    true,
			"limit_ll":          0,
			"limit_l":           10,
			"limit_h":           90,
			"limit_hh":          100,
			"limit_deadband":    0.5,
			"violation_hold_ms": 1000,
			"recover_hold_ms":   1000,
			"quality_policy":    models.QualityPolicyIgnoreBad,
			"check_cycle_ms":    3000,
		})
	}
	payload, _ := json.Marshal(map[string]any{
		"command":             "start_custom_detection",
		"project_id":          1,
		"test_no":             "PERF",
		"limit_check_enabled": true,
		"end_policy":          models.DetectionEndPolicyFixedDuration,
		"duration_sec":        600,
		"custom_items":        items,
	})
	if len(payload) >= taskFlowStringParamMaxBytes {
		t.Fatalf("payload too large bytes=%d limit=%d", len(payload), taskFlowStringParamMaxBytes)
	}
	start := time.Now()
	params := taskFlowParamsFromTrigger(string(payload))
	parseLatency := time.Since(start)
	if params["_error"] != nil || len(params["custom_items"].([]any)) != 520 {
		t.Fatalf("unexpected params parse result err=%v", params["_error"])
	}
	itemsStart := time.Now()
	converted, err := detectionStandardItemsFromTaskParams(params["custom_items"])
	convertLatency := time.Since(itemsStart)
	if err != nil || len(converted) != 520 {
		t.Fatalf("unexpected converted items len=%d err=%v", len(converted), err)
	}
	t.Logf("STRING custom detection payload bytes=%d limit=%d parse=%s convert=%s", len(payload), taskFlowStringParamMaxBytes, parseLatency, convertLatency)
	if parseLatency > 50*time.Millisecond || convertLatency > 50*time.Millisecond {
		t.Fatalf("payload handling latency too high parse=%s convert=%s", parseLatency, convertLatency)
	}
}

func TestTaskFlowAsyncEndGuards(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-05", Name: "Project 5", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	limitH := 30.0
	tagConfig := models.TagConfig{
		VarID:       501,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
		ScaleFactor: 1,
	}
	if err := repo.CreateTag(&tagConfig); err != nil {
		t.Fatal(err)
	}
	standard := &models.DetectionStandard{StandardCode: "STD-GUARD", Name: "Guard Standard", ProjectID: &project.ID, ProjectCode: project.ProjectCode, Mode: "task_flow", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:        tagConfig.VarID,
		VarName:      tagConfig.VarName,
		CheckEnabled: true,
		AlarmEnabled: true,
		StoreEnabled: true,
		LimitH:       &limitH,
	}}); err != nil {
		t.Fatal(err)
	}
	tags.Load([]models.TagConfig{tagConfig})
	tag, ok := tags.Get(tagConfig.VarID)
	if !ok {
		t.Fatal("expected tag")
	}
	tag.UpdateNumeric(25, time.Now(), 1)
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)

	durationTask, err := repo.StartDetectionTaskWithOptions(database.StartDetectionOptions{ProjectID: project.ID, TestNo: "TF-GUARD-DURATION", Mode: "task_flow", StandardID: &standard.ID, DurationSec: 1})
	if err != nil {
		t.Fatal(err)
	}
	tasks.SetActive(*durationTask)
	past := time.Now().Add(-time.Second)
	if err := db.Model(&models.DetectionTask{}).Where("id = ?", durationTask.ID).Update("expected_end_at", past).Error; err != nil {
		t.Fatal(err)
	}
	executor.startFixedDurationGuard(durationTask.ID)
	waitForDetectionEndType(t, repo, durationTask.ID, models.DetectionEndFixedDuration)

	qualifiedTask, err := repo.StartDetectionTaskWithOptions(database.StartDetectionOptions{ProjectID: project.ID, TestNo: "TF-GUARD-QUALIFIED", Mode: "task_flow", StandardID: &standard.ID})
	if err != nil {
		t.Fatal(err)
	}
	tasks.SetActive(*qualifiedTask)
	if !executor.startQualifiedHoldGuard(qualifiedTask.ID, 10*time.Millisecond, 5*time.Millisecond) {
		t.Fatal("expected qualified guard to start")
	}
	if executor.startQualifiedHoldGuard(qualifiedTask.ID, 10*time.Millisecond, 5*time.Millisecond) {
		t.Fatal("duplicate qualified guard should be ignored")
	}
	waitForDetectionEndType(t, repo, qualifiedTask.ID, models.DetectionEndQualifiedHold)
}

func TestTaskFlowBuiltinWriteVariableTriggersAndBlocksSelfRecursion(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(6)
	tags.Load([]models.TagConfig{
		{VarID: 610, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", SourcePath: "task_request", RawName: "task_request", VarName: "task_request", JSONPath: "task_request", DataType: "STRING", ProjectID: &projectID, ProjectCode: "AC-06", Enabled: true, ScaleFactor: 1},
		{VarID: 611, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", SourcePath: "self_request", RawName: "self_request", VarName: "self_request", JSONPath: "self_request", DataType: "STRING", ProjectID: &projectID, ProjectCode: "AC-06", Enabled: true, ScaleFactor: 1},
	})
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Load([]models.TaskFlow{{
		ID:              61,
		ProjectID:       projectID,
		FlowCode:        "watch-write-request",
		Name:            "Watch Write Request",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start"`,
		ActionType:      models.TaskFlowActionBuiltinContextSet,
		ActionPayload:   `{"handled":true}`,
		TimeoutMS:       3000,
		Vars:            []models.TaskFlowVar{{FlowID: 61, ProjectID: projectID, VarID: 610, Role: models.TaskFlowVarRoleWatch}},
	}})
	executor.Start(1)

	steps := mustStepsJSON(t, []map[string]any{{
		"code":   "write",
		"module": models.TaskFlowActionBuiltinWriteVariable,
		"params": map[string]any{
			"var_id":     610,
			"value":      `{"command":"start"}`,
			"trigger":    true,
			"max_depth":  1,
			"request_id": "req-610",
		},
	}})
	if !executor.Submit(models.TaskFlow{
		ID:          60,
		ProjectID:   projectID,
		FlowCode:    "write-request",
		Name:        "Write Request",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON:   steps,
		TimeoutMS:   3000,
	}, TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: projectID, RequestID: "req-610"}) {
		t.Fatal("submit write flow failed")
	}
	waitForTaskFlowRunStatus(t, db, 61, models.TaskFlowStatusSuccess, time.Second)
	tag, ok := tags.Get(610)
	if !ok || tag.RuntimeState().StrValue != `{"command":"start"}` {
		t.Fatalf("expected task_request write, tag=%+v ok=%v", tag, ok)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", "610", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount == 0 {
		t.Fatal("expected write audit")
	}

	executor.Load([]models.TaskFlow{{
		ID:          62,
		ProjectID:   projectID,
		FlowCode:    "self-write",
		Name:        "Self Write",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerDataChange,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "self",
			"module": models.TaskFlowActionBuiltinWriteVariable,
			"params": map[string]any{"var_id": 611, "value": `{"loop":true}`, "trigger": true, "max_depth": 3},
		}}),
		TimeoutMS: 3000,
		Vars:      []models.TaskFlowVar{{FlowID: 62, ProjectID: projectID, VarID: 611, Role: models.TaskFlowVarRoleWatch}},
	}})
	if !executor.Submit(models.TaskFlow{
		ID:          62,
		ProjectID:   projectID,
		FlowCode:    "self-write",
		Name:        "Self Write",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerDataChange,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "self",
			"module": models.TaskFlowActionBuiltinWriteVariable,
			"params": map[string]any{"var_id": 611, "value": `{"loop":true}`, "trigger": true, "max_depth": 3},
		}}),
		TimeoutMS: 3000,
	}, TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: projectID, TriggerVarID: 611, TriggerValue: `{"loop":false}`}) {
		t.Fatal("submit self flow failed")
	}
	waitForTaskFlowRunStatus(t, db, 62, models.TaskFlowStatusSuccess, time.Second)
	time.Sleep(50 * time.Millisecond)
	var selfRuns int64
	if err := db.Model(&models.TaskFlowRun{}).Where("flow_id = ?", 62).Count(&selfRuns).Error; err != nil {
		t.Fatal(err)
	}
	if selfRuns != 1 {
		t.Fatalf("expected self recursion to be blocked, runs=%d", selfRuns)
	}
}

func TestTaskFlowWriteVariableRejectsPhysicalVariables(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(13)
	tags.Load([]models.TagConfig{{
		VarID:         1301,
		SourceType:    models.TagSourceMQTT,
		GatewayID:     1,
		SourceTopic:   "topic",
		SourcePath:    "physical",
		RawName:       "physical",
		VarName:       "physical",
		JSONPath:      "physical",
		DataType:      "FLOAT",
		ProjectID:     &projectID,
		ProjectCode:   "AC-13",
		Enabled:       true,
		ScaleFactor:   1,
		Writable:      true,
		RWMode:        models.RWModeReadWrite,
		WritePath:     "physical",
		WriteDataType: "FLOAT",
	}})
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	flow := models.TaskFlow{
		ID:          130,
		ProjectID:   projectID,
		FlowCode:    "physical-write",
		Name:        "Physical Write",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "write",
			"module": models.TaskFlowActionBuiltinWriteVariable,
			"params": map[string]any{"var_id": 1301, "value": 12.3},
		}}),
		TimeoutMS: 3000,
	}
	executor.Start(1)
	if !executor.Submit(flow, TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: projectID}) {
		t.Fatal("submit failed")
	}
	run := waitForTaskFlowRunStatus(t, db, flow.ID, models.TaskFlowStatusFailed, time.Second)
	if !strings.Contains(run.ErrorMessage, "not writable") {
		t.Fatalf("expected physical write rejection, run=%+v", run)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", "1301", "failed").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected failed physical write audit, got %d", auditCount)
	}
}

func TestTaskFlowWriteControlVariablesResolvesProcessParams(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	executor := NewTaskFlowExecutor(repo, NewTagManager(), NewTaskManager(), NewChannels())
	var writes []TaskFlowVariableWriteInput
	executor.SetVariableWriter(func(_ context.Context, input TaskFlowVariableWriteInput) (map[string]any, error) {
		writes = append(writes, input)
		return map[string]any{
			"var_id":      input.VarID,
			"value":       input.Value,
			"wait_ack":    input.WaitAck,
			"request_id":  input.RequestID,
			"source_type": models.TagSourceMQTT,
		}, nil
	})
	flow := models.TaskFlow{
		ID:          131,
		ProjectID:   13,
		FlowCode:    "control-write",
		Name:        "Control Write",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerDataChange,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "write_plc",
			"module": models.TaskFlowActionBuiltinWriteControlVariables,
			"params": map[string]any{
				"items": []map[string]any{{
					"var_id":          "9212397624135540848",
					"value_from":      "process_params.inlet_area_m2",
					"wait_ack":        true,
					"ack_timeout_sec": 7,
				}},
			},
		}}),
		TimeoutMS: 3000,
	}
	result := executor.runFlow(flow, TaskFlowEvent{
		TriggerType:  models.TaskFlowTriggerDataChange,
		ProjectID:    13,
		TriggerValue: map[string]any{"process_params": map[string]any{"inlet_area_m2": 1.25}},
		RequestID:    "req-control",
	}, 13101)
	if result.Err != nil || result.Status == models.TaskFlowStatusFailed {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(writes) != 1 {
		t.Fatalf("expected one write, got %d", len(writes))
	}
	if writes[0].VarID != 9212397624135540848 || writes[0].Value != 1.25 || !writes[0].WaitAck || writes[0].AckTimeoutSec != 7 || writes[0].RequestID != "req-control" {
		t.Fatalf("unexpected write input: %+v", writes[0])
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "task_flow.write_variable", "9212397624135540848", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected success audit, got %d", auditCount)
	}
}

func TestTaskFlowDetectionLifecycleTriggersProjectFlows(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	project := &models.Project{ProjectCode: "AC-14", Name: "Project 14", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	executor.Load([]models.TaskFlow{
		{
			ID:              141,
			ProjectID:       project.ID,
			FlowCode:        "on-project-start",
			Name:            "On Project Start",
			Enabled:         true,
			TriggerType:     models.TaskFlowTriggerProjectStart,
			ConditionScript: `task_params.task_id > 0 && task_params.project_id === project.id`,
			ActionType:      models.TaskFlowActionJavaScript,
			ActionScript:    `log.info("project_start task=" + task_params.task_id); ({task_id: task_params.task_id});`,
			TimeoutMS:       3000,
		},
		{
			ID:              142,
			ProjectID:       project.ID,
			FlowCode:        "on-project-end",
			Name:            "On Project End",
			Enabled:         true,
			TriggerType:     models.TaskFlowTriggerProjectEnd,
			ConditionScript: `task_params.end_type === "task_flow_stop"`,
			ActionType:      models.TaskFlowActionJavaScript,
			ActionScript:    `log.info("project_end task=" + task_params.task_id + ", end=" + task_params.end_type); ({end_type: task_params.end_type});`,
			TimeoutMS:       3000,
		},
	})
	executor.Start(1)
	startStopFlow := models.TaskFlow{
		ID:          140,
		ProjectID:   project.ID,
		FlowCode:    "start-stop-lifecycle",
		Name:        "Start Stop Lifecycle",
		Enabled:     true,
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{
			{
				"code":   "start",
				"module": models.TaskFlowActionBuiltinStartDetectionRun,
				"params": map[string]any{
					"project_id": project.ID,
					"test_no":    "TF-LIFECYCLE",
					"mode":       "task_flow",
				},
			},
			{
				"code":   "stop",
				"module": models.TaskFlowActionBuiltinStopDetectionRun,
				"params": map[string]any{
					"task_id":  map[string]any{"source": "context", "key": "task_id"},
					"end_type": models.DetectionEndTaskFlowStop,
					"reason":   "lifecycle smoke",
				},
			},
		}),
		TimeoutMS: 3000,
	}
	if !executor.Submit(startStopFlow, TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: project.ID}) {
		t.Fatal("submit lifecycle flow failed")
	}
	waitForTaskFlowRunStatus(t, db, 140, models.TaskFlowStatusSuccess, time.Second)
	startRun := waitForTaskFlowRunStatus(t, db, 141, models.TaskFlowStatusSuccess, time.Second)
	endRun := waitForTaskFlowRunStatus(t, db, 142, models.TaskFlowStatusSuccess, time.Second)
	if !strings.Contains(startRun.ScriptLogs, "project_start task=") || !strings.Contains(endRun.ScriptLogs, "project_end task=") {
		t.Fatalf("expected lifecycle script logs, start=%+v end=%+v", startRun, endRun)
	}
}

func TestTaskFlowExternalActionModules(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	tags := NewTagManager()
	tasks := NewTaskManager()
	projectID := uint(7)
	task := models.DetectionTask{ID: 70, TestNo: "T-REPORT", ProjectID: projectID, ProjectCode: "AC-07", Mode: "task_flow", Status: models.DetectionStatusRunning}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	executor := NewTaskFlowExecutor(repo, tags, tasks, channels)
	reportResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          70,
		ProjectID:   projectID,
		FlowCode:    "register-report",
		Name:        "Register Report",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "report",
			"module": models.TaskFlowActionBuiltinRegisterReport,
			"params": map[string]any{"task_id": task.ID, "file_ref": "reports/out.xlsx", "file_name": "out.xlsx"},
		}}),
		TimeoutMS: 3000,
	})
	if reportResult.Result["context"].(map[string]any)["report.task_id"] == nil {
		t.Fatalf("expected report context: %+v", reportResult.Result)
	}
	var reports int64
	if err := db.Model(&models.DetectionRunReport{}).Where("task_id = ?", task.ID).Count(&reports).Error; err != nil || reports != 1 {
		t.Fatalf("reports=%d err=%v", reports, err)
	}
	featureResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          72,
		ProjectID:   projectID,
		FlowCode:    "refresh-features",
		Name:        "Refresh Features",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "features",
			"module": models.TaskFlowActionBuiltinRefreshFeatures,
			"params": map[string]any{"task_id": task.ID},
		}}),
		TimeoutMS: 3000,
	})
	if featureResult.Result["context"].(map[string]any)["features.feature_count"] == nil {
		t.Fatalf("expected feature refresh context: %+v", featureResult.Result)
	}
	var featureEvents int64
	if err := db.Model(&models.DetectionRunEvent{}).Where("task_id = ? AND event_type = ?", task.ID, models.DetectionEventFeaturesUpdated).Count(&featureEvents).Error; err != nil || featureEvents != 1 {
		t.Fatalf("feature events=%d err=%v", featureEvents, err)
	}
	featureNotification := <-channels.Notify
	if featureNotification.Type != models.NotificationDetectionFeatures || featureNotification.TaskID != task.ID {
		t.Fatalf("unexpected feature notification: %+v", featureNotification)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	httpResult := runDetectionFlow(t, executor, models.TaskFlow{
		ID:          71,
		ProjectID:   projectID,
		FlowCode:    "http-action",
		Name:        "HTTP Action",
		TriggerType: models.TaskFlowTriggerManual,
		StepsJSON: mustStepsJSON(t, []map[string]any{{
			"code":   "http",
			"module": models.TaskFlowActionBuiltinHTTPRequest,
			"params": map[string]any{"method": "POST", "url": server.URL, "body": `{"ping":true}`},
		}}),
		TimeoutMS: 3000,
	})
	step := httpResult.Result["steps"].([]map[string]any)[0]["result"].(map[string]any)
	if step["status_code"] != http.StatusAccepted {
		t.Fatalf("unexpected http result: %+v", httpResult.Result)
	}
}

func TestTaskFlowDetectionNotifications(t *testing.T) {
	db := newTaskFlowTestDB(t)
	repo := database.NewRepository(db)
	channels := NewChannels()
	executor := NewTaskFlowExecutor(repo, NewTagManager(), NewTaskManager(), channels)
	startedAt := time.Date(2026, 5, 30, 18, 10, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Minute)
	task := models.DetectionTask{
		ID:          77,
		TestNo:      "T-NOTIFY",
		ProjectID:   8,
		ProjectCode: "AC-08",
		Status:      models.DetectionStatusStopped,
		StartedAt:   &startedAt,
		EndedAt:     &endedAt,
		EndType:     models.DetectionEndTaskFlowStop,
	}
	executor.recordDetectionRunEvent(task, models.DetectionEventRunStopped, models.NotificationLevelInfo, "stopped")
	eventNotification := <-channels.Notify
	if eventNotification.Type != models.NotificationDetectionRunStopped || eventNotification.TaskID != task.ID || eventNotification.OccurredAt != endedAt {
		t.Fatalf("unexpected event notification: %+v", eventNotification)
	}

	executor.publishDetectionResult(task, models.DetectionRunSummary{
		TaskID:       task.ID,
		TestNo:       task.TestNo,
		ProjectID:    task.ProjectID,
		ProjectCode:  task.ProjectCode,
		ResultStatus: models.DetectionSummaryStatusNG,
		AlarmTotal:   2,
	})
	resultNotification := <-channels.Notify
	if resultNotification.Type != models.NotificationDetectionResultNG || resultNotification.Level != models.NotificationLevelWarning || resultNotification.Payload["alarm_total"] != int64(2) {
		t.Fatalf("unexpected result notification: %+v", resultNotification)
	}
}

func newTaskFlowTestDB(t *testing.T) *gorm.DB {
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

func runDetectionFlow(t *testing.T, executor *TaskFlowExecutor, flow models.TaskFlow) taskFlowResult {
	t.Helper()
	result := executor.runFlow(flow, TaskFlowEvent{TriggerType: flow.TriggerType, ProjectID: flow.ProjectID, At: time.Now()}, uint64(flow.ID)+1000)
	if result.Err != nil || result.Status != models.TaskFlowStatusSuccess {
		t.Fatalf("flow %s failed status=%s err=%v result=%+v logs=%v", flow.FlowCode, result.Status, result.Err, result.Result, result.Logs)
	}
	return result
}

func mustStepsJSON(t *testing.T, steps []map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func receiveStoreTask(t *testing.T, channels *Channels) *models.StoreTask {
	t.Helper()
	select {
	case task := <-channels.Store:
		return task
	case <-time.After(time.Second):
		t.Fatal("expected store task")
		return nil
	}
}

func receiveAlarmEvent(t *testing.T, channels *Channels) *models.DetectionLimitAlarmEvent {
	t.Helper()
	select {
	case event := <-channels.Alarm:
		return event
	case <-time.After(time.Second):
		t.Fatal("expected alarm event")
		return nil
	}
}

func assertNoStoreOrAlarm(t *testing.T, channels *Channels) {
	t.Helper()
	select {
	case task := <-channels.Store:
		t.Fatalf("unexpected store task: %+v", task)
	default:
	}
	select {
	case event := <-channels.Alarm:
		t.Fatalf("unexpected alarm event: %+v", event)
	default:
	}
}

func waitForDetectionEndType(t *testing.T, repo *database.Repository, taskID uint, endType string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		task, err := repo.GetDetectionTask(taskID)
		if err == nil && task.Status == models.DetectionStatusStopped && task.EndType == endType {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	task, err := repo.GetDetectionTask(taskID)
	t.Fatalf("expected end_type=%s, got task=%+v err=%v", endType, task, err)
}

func waitForTaskFlowRunStatus(t *testing.T, db *gorm.DB, flowID uint64, status string, timeout time.Duration) models.TaskFlowRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var run models.TaskFlowRun
	for time.Now().Before(deadline) {
		if err := db.Order("id desc").First(&run, "flow_id = ?", flowID).Error; err == nil && run.Status == status {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected task flow run flow_id=%d status=%s, got %+v", flowID, status, run)
	return run
}
