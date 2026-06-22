package services

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestVariablesServiceCreateAndReload(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	service := NewVariablesService(repo, tags)
	Project := createServiceProject(t, repo)

	for _, dataType := range []string{"INT", "FLOAT", "BOOL", "STRING"} {
		tag, err := service.Create(CreateVariableInput{
			SourceType:  "virtual",
			ProjectID:   &Project.ID,
			ProjectCode: Project.ProjectCode,
			VarName:     "v_" + dataType,
			DataType:    dataType,
		})
		if err != nil {
			t.Fatal(err)
		}
		if tag.SourceType != models.TagSourceVirtual || tag.Discovered || !tag.Placeholder || tag.GatewayID != 0 {
			t.Fatalf("unexpected virtual tag: %+v", tag)
		}
		if tag.Writable || tag.RWMode != models.RWModeRead || !tag.WriteRequiresAudit {
			t.Fatalf("unexpected variable defaults: %+v", tag)
		}
	}
	if tags.Count() != 4 {
		t.Fatalf("expected 4 realtime tags, got %d", tags.Count())
	}
	if snapshots := service.Snapshots(RealtimeVariableFilter{}); len(snapshots) != 4 {
		t.Fatalf("expected snapshots through service, got %d", len(snapshots))
	}
	if listed, err := service.List(database.TagFilter{SourceType: models.TagSourceVirtual}); err != nil || len(listed) != 4 {
		t.Fatalf("expected list through service len=%d err=%v", len(listed), err)
	}
	if _, err := service.Create(CreateVariableInput{SourceType: "manual", VarName: "bad", DataType: "FLOAT"}); err == nil {
		t.Fatal("expected manual source validation error")
	}
	if _, err := service.Create(CreateVariableInput{SourceType: "virtual", VarName: "bad", DataType: "OBJECT"}); err == nil {
		t.Fatal("expected data type validation error")
	}
	if _, err := service.Create(CreateVariableInput{SourceType: "virtual", VarName: "bad_write", DataType: "FLOAT", Writable: true, RWMode: models.RWModeReadWrite}); err == nil {
		t.Fatal("expected writable variable without write_path to fail")
	}

	writeMin := 0.0
	writeMax := 10.0
	tag, err := service.Create(CreateVariableInput{
		SourceType: "virtual",
		ProjectID:  &Project.ID,
		VarName:    "writable_float",
		DataType:   "FLOAT",
		RWMode:     models.RWModeReadWrite,
		Writable:   true,
		WritePath:  "setpoint",
		WriteMin:   &writeMin,
		WriteMax:   &writeMax,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tag.Writable || tag.WritePath != "setpoint" || tag.WriteDataType != "FLOAT" {
		t.Fatalf("unexpected writable variable: %+v", tag)
	}
	if _, err := service.Update(tag.VarID, map[string]interface{}{"rw_mode": models.RWModeRead}); err == nil {
		t.Fatal("expected rw_mode R on writable variable to fail")
	}
	unassignedEnabled := false
	unassigned, err := service.Create(CreateVariableInput{SourceType: "virtual", VarName: "assign_me", DataType: "STRING", Enabled: &unassignedEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Assign(unassigned.VarID, &Project.ID, "", "assigned", true); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.GetTag(unassigned.VarID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.ProjectID == nil || assigned.VarGroup != "assigned" || !assigned.Enabled {
		t.Fatalf("expected assigned tag, got %+v", assigned)
	}
	if err := service.Delete(unassigned.VarID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetTag(unassigned.VarID); err == nil {
		t.Fatal("expected deleted variable to be absent")
	}
}

func TestVariablesServiceListUsesLocalEdgeInstance(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	if err := repo.UpsertGatewaySeeds([]models.GatewayConfig{
		{ID: 1, Name: "local", EdgeInstanceID: "edge-local", Broker: "tcp://127.0.0.1:1883", ClientID: "local", Topic: "topic", Enabled: true},
		{ID: 2, Name: "other", EdgeInstanceID: "edge-other", Broker: "tcp://127.0.0.1:1884", ClientID: "other", Topic: "topic", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	localProject := &models.Project{ProjectCode: "AC-LOCAL", EdgeInstanceID: "edge-local", Name: "Local", Enabled: true}
	otherProject := &models.Project{ProjectCode: "AC-OTHER", EdgeInstanceID: "edge-other", Name: "Other", Enabled: true}
	for _, project := range []*models.Project{localProject, otherProject} {
		if err := repo.CreateProject(project); err != nil {
			t.Fatal(err)
		}
	}
	fixtures := []models.TagConfig{
		{VarID: 201, GatewayID: 1, SourcePath: "local-assigned", RawName: "local-assigned", ProjectID: &localProject.ID, ProjectCode: localProject.ProjectCode, VarName: "local_assigned", JSONPath: "local-assigned", DataType: "FLOAT", ScaleFactor: 1, Enabled: true},
		{VarID: 202, GatewayID: 2, SourcePath: "other-assigned", RawName: "other-assigned", ProjectID: &otherProject.ID, ProjectCode: otherProject.ProjectCode, VarName: "other_assigned", JSONPath: "other-assigned", DataType: "FLOAT", ScaleFactor: 1, Enabled: true},
	}
	for i := range fixtures {
		if err := repo.CreateTag(&fixtures[i]); err != nil {
			t.Fatal(err)
		}
	}

	service := NewVariablesService(repo, pipeline.NewTagManager(), "edge-local")
	listed, err := service.List(database.TagFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].VarID != 201 {
		t.Fatalf("local edge list should only include local project variables, got %+v", listed)
	}
	mismatch, err := service.List(database.TagFilter{ProjectID: &localProject.ID, EdgeInstanceID: "edge-other"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatch) != 0 {
		t.Fatalf("explicit non-local edge query should return no variables, got %+v", mismatch)
	}
}

func TestVariablesServiceBulkRemapKIOProjects(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	service := NewVariablesService(repo, tags)
	fixtures := []models.TagConfig{
		{VarID: 1001, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台1_39").1`, SourceType: models.TagSourceMQTT, RawName: "台1_39", VarName: "台1_39", JSONPath: `Objs.#(N=="台1_39").1`, DataType: "FLOAT", ScaleFactor: 1, Discovered: true, Enabled: false},
		{VarID: 2040, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台2_40").1`, SourceType: models.TagSourceMQTT, RawName: "台2_40", VarName: "台2_40", JSONPath: `Objs.#(N=="台2_40").1`, DataType: "BOOL", ScaleFactor: 1, Discovered: true, Enabled: false},
		{VarID: 13001, GatewayID: 1, SourceTopic: "datachange", SourcePath: `Objs.#(N=="台13_1").1`, SourceType: models.TagSourceMQTT, RawName: "台13_1", VarName: "台13_1", JSONPath: `Objs.#(N=="台13_1").1`, DataType: "INT", ScaleFactor: 1, Discovered: true, Enabled: false},
	}
	for i := range fixtures {
		if err := repo.CreateTag(&fixtures[i]); err != nil {
			t.Fatal(err)
		}
	}

	dryRun, err := service.BulkRemapKIOProjects(BulkRemapKIOProjectsInput{DryRun: true, ProjectCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.DryRun || dryRun.Matched != 2 || dryRun.Updated != 0 || dryRun.Skipped != 1 {
		t.Fatalf("unexpected dry run result: %+v", dryRun)
	}
	if dryRun.Items[0].VarIDText == "" {
		t.Fatalf("expected bulk remap result to include var_id_text: %+v", dryRun.Items[0])
	}
	if projects, err := repo.ListProjects(); err != nil || len(projects) != 0 {
		t.Fatalf("dry run should not create projects len=%d err=%v", len(projects), err)
	}

	existingProject := &models.Project{ProjectCode: "AC-01", SiteNo: "1", Name: "Existing AC 1", Enabled: true}
	if err := repo.CreateProject(existingProject); err != nil {
		t.Fatal(err)
	}
	assignedTag, err := repo.UpdateTag(1001, map[string]interface{}{"project_id": &existingProject.ID, "project_code": existingProject.ProjectCode})
	if err != nil {
		t.Fatal(err)
	}
	oldRoute, err := repo.EnsureDefaultStorageRouteForTag(assignedTag)
	if err != nil {
		t.Fatal(err)
	}
	if oldRoute == nil || oldRoute.ColumnName == "mapped_01_39" {
		t.Fatalf("expected old default storage route before remap, got %+v", oldRoute)
	}

	remapVarName := true
	enable := true
	result, err := service.BulkRemapKIOProjects(BulkRemapKIOProjectsInput{
		ProjectCount:  2,
		VarNamePrefix: "mapped",
		RemapVarName:  &remapVarName,
		Enable:        &enable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedProjects != 1 || result.Matched != 2 || result.Updated != 2 || result.Skipped != 1 {
		t.Fatalf("unexpected remap result: %+v", result)
	}
	tag, err := repo.GetTag(1001)
	if err != nil {
		t.Fatal(err)
	}
	if tag.ProjectID == nil || *tag.ProjectID != 1 || tag.ProjectCode != "AC-01" || tag.VarName != "mapped_01_39" || tag.DisplayNameEN != "Project 1 Var 39" || !tag.Enabled {
		t.Fatalf("unexpected remapped tag: %+v", tag)
	}
	if routes, err := repo.ListStorageRoutesByProject(*tag.ProjectID); err != nil || len(routes) != 1 || routes[0].ColumnName != "mapped_01_39" {
		t.Fatalf("expected default storage route, routes=%+v err=%v", routes, err)
	}
	if tags.Count() != 2 {
		t.Fatalf("expected two runtime tags after remap, got %d", tags.Count())
	}

	remapVarName = false
	enable = false
	second, err := service.BulkRemapKIOProjects(BulkRemapKIOProjectsInput{ProjectCount: 2, RemapVarName: &remapVarName, Enable: &enable})
	if err != nil {
		t.Fatal(err)
	}
	if second.CreatedProjects != 0 || second.UpdatedProjects != 0 || second.Updated != 2 {
		t.Fatalf("unexpected second remap result: %+v", second)
	}
	disabled, err := repo.GetTag(1001)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.VarName != "mapped_01_39" || disabled.Enabled {
		t.Fatalf("expected var_name preserved and disabled, got %+v", disabled)
	}
}

func TestVariablesServiceBulkRemapKIOProjectsRejectsOutsideTestRange(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	service := NewVariablesService(repo, pipeline.NewTagManager())

	if _, err := service.BulkRemapKIOProjects(BulkRemapKIOProjectsInput{ProjectCount: 9}); err == nil {
		t.Fatal("expected project_count above AC-01..AC-08 test range to be rejected")
	}
	if projects, err := repo.ListProjects(); err != nil || len(projects) != 0 {
		t.Fatalf("rejected bulk remap should not create projects len=%d err=%v", len(projects), err)
	}
}

func TestVariablesServiceBulkRemapKIOProjectsCustomPrefixes(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	service := NewVariablesService(repo, tags)
	project := &models.Project{ProjectCode: "LINE-01", SiteNo: "", Name: "Line One", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	tag := models.TagConfig{
		VarID:       501,
		GatewayID:   1,
		SourceTopic: "datachange",
		SourcePath:  `Objs.#(N=="站1_7").1`,
		SourceType:  models.TagSourceMQTT,
		RawName:     "站1_7",
		VarName:     "keep_name",
		JSONPath:    `Objs.#(N=="站1_7").1`,
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Discovered:  true,
		Enabled:     false,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	remapVarName := false
	enable := false
	result, err := service.BulkRemapKIOProjects(BulkRemapKIOProjectsInput{
		ProjectCount:         1,
		ProjectCodePrefix:    "LINE",
		ProjectDisplayPrefix: "产线",
		ProjectENPrefix:      "Line",
		ProjectJAPrefix:      "ライン",
		RawProjectPrefix:     "站",
		VarGroup:             "Custom",
		VarNamePrefix:        "ignored",
		RemapVarName:         &remapVarName,
		Enable:               &enable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedProjects != 0 || result.UpdatedProjects != 1 || result.Updated != 1 || result.Items[0].NewVarName != "keep_name" {
		t.Fatalf("unexpected custom remap result: %+v", result)
	}
	updatedProject, err := repo.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedProject.SiteNo != "1" || updatedProject.DisplayName != "产线1" || updatedProject.DisplayNameEN != "Line 1" || updatedProject.DisplayNameJA != "ライン1" {
		t.Fatalf("expected display fallbacks to be updated, got %+v", updatedProject)
	}
	updatedTag, err := repo.GetTag(501)
	if err != nil {
		t.Fatal(err)
	}
	if updatedTag.VarName != "keep_name" || updatedTag.VarGroup != "Custom" || updatedTag.Enabled {
		t.Fatalf("unexpected custom remapped tag: %+v", updatedTag)
	}
	if tags.Count() != 0 {
		t.Fatalf("disabled remap should not load runtime tags, got %d", tags.Count())
	}
}

func TestVariableWriteServiceVirtualAndKIOPaths(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	projectID := uint(1)
	writeMin := 0.0
	writeMax := 10.0
	tags.Load([]models.TagConfig{
		{VarID: 10, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", SourcePath: "request", RawName: "request", VarName: "request", JSONPath: "request", DataType: "STRING", ProjectID: &projectID, ProjectCode: "P1", Enabled: true},
		{VarID: 11, SourceType: models.TagSourceMQTT, GatewayID: 2, SourceTopic: "topic", SourcePath: "setpoint", RawName: "setpoint", VarName: "setpoint", JSONPath: "setpoint", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "P1", Enabled: true, Writable: true, RWMode: models.RWModeReadWrite, WritePath: "SP", WriteDataType: "FLOAT", WriteMin: &writeMin, WriteMax: &writeMax},
	})
	broker := &fakeKIOBroker{config: models.GatewayConfig{ID: 2, QOS: 1, KIOClientID: "client", KIOWriter: "writer", SetDataTopic: "setdata"}}
	service := NewVariableWriteService(repo, tags, NewKIOWriteService(broker), nil)
	virtualResult, err := service.Write(context.Background(), VariableWriteInput{VarID: 10, Value: `{"command":"go"}`, Trigger: true})
	if err != nil {
		t.Fatal(err)
	}
	if virtualResult.VarIDText != "10" {
		t.Fatalf("expected virtual write var_id_text, got %+v", virtualResult)
	}
	tag, _ := tags.Get(10)
	if tag.RuntimeState().StrValue != `{"command":"go"}` {
		t.Fatalf("unexpected virtual value: %+v", tag.RuntimeState())
	}
	result, err := service.Write(context.Background(), VariableWriteInput{VarID: 11, Value: 5.5, WaitAck: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.VarIDText != "11" {
		t.Fatalf("expected kio write var_id_text, got %+v", result)
	}
	if !result.BrokerAccepted || !result.ProjectConfirmed || broker.publishedTopic != "setdata" || len(broker.values) != 1 || broker.values[0].Name != "SP" {
		t.Fatalf("unexpected kio write result=%+v broker=%+v", result, broker)
	}
	if _, err := service.Write(context.Background(), VariableWriteInput{VarID: 11, Value: 11}); err == nil {
		t.Fatal("expected write max validation error")
	}
}

func TestKIOWriteServiceValidationAndAckStatus(t *testing.T) {
	if _, err := NewKIOWriteService(nil).Write(context.Background(), KIOWriteInput{GatewayID: 1}); HTTPStatusForKIOError(err) != 502 {
		t.Fatalf("expected missing broker 502, got %v", err)
	}
	broker := &fakeKIOBroker{config: models.GatewayConfig{ID: 2, QOS: 1, KIOClientID: "client", KIOWriter: "writer"}}
	service := NewKIOWriteService(broker)
	if _, err := service.Write(context.Background(), KIOWriteInput{GatewayID: 404, Values: []kio.WriteValue{{Name: "SP", Value: 1}}}); HTTPStatusForKIOError(err) != 400 {
		t.Fatalf("expected gateway validation 400, got %v", err)
	}

	published, err := service.Write(context.Background(), KIOWriteInput{GatewayID: 2, Values: []kio.WriteValue{{Name: "SP", Value: 1.25}}})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "published_unconfirmed" || published.Topic != "setdata_client" || broker.publishedTopic != "setdata_client" {
		t.Fatalf("unexpected published result=%+v broker=%+v", published, broker)
	}

	broker.ack = &kio.WriteAck{QID: 99, ProcessStep: 80, Result: "ng", Success: false, Message: "rejected"}
	rejected, err := service.Write(context.Background(), KIOWriteInput{GatewayID: 2, QID: 99, Values: []kio.WriteValue{{Name: "SP", Value: 1.25}}, WaitAck: true})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" || rejected.ProjectConfirmed || rejected.AckTopic != "setdata_result_client_writer" {
		t.Fatalf("unexpected rejected result=%+v", rejected)
	}

	broker.waitErr = errors.New("timeout")
	broker.waitBrokerAccepted = true
	timedOut, err := service.Write(context.Background(), KIOWriteInput{GatewayID: 2, Values: []kio.WriteValue{{Name: "SP", Value: 1.25}}, WaitAck: true})
	if err == nil || HTTPStatusForKIOError(err) != 504 || timedOut.Status != "ack_timeout_or_unmatched" || !timedOut.BrokerAccepted {
		t.Fatalf("expected timeout result, got result=%+v err=%v", timedOut, err)
	}

	broker.waitErr = nil
	broker.waitBrokerAccepted = false
	broker.ack = nil
	broker.subscribeErr = errors.New("gateway is not connected")
	offline, err := service.Write(context.Background(), KIOWriteInput{GatewayID: 2, Values: []kio.WriteValue{{Name: "SP", Value: 1.25}}, WaitAck: true})
	if err == nil || HTTPStatusForKIOError(err) != 502 || offline.Status != "gateway_offline" || offline.BrokerAccepted {
		t.Fatalf("expected structured gateway offline result, got result=%+v err=%v", offline, err)
	}
}

type fakeKIOBroker struct {
	config             models.GatewayConfig
	publishedTopic     string
	values             []kio.WriteValue
	publishErr         error
	subscribeErr       error
	waitErr            error
	waitBrokerAccepted bool
	ack                *kio.WriteAck
}

func (b *fakeKIOBroker) Config(gatewayID int) (models.GatewayConfig, bool) {
	if gatewayID != b.config.ID {
		return models.GatewayConfig{}, false
	}
	return b.config, true
}

func (b *fakeKIOBroker) Publish(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool) error {
	if b.publishErr != nil {
		return b.publishErr
	}
	b.publishedTopic = topic
	var decoded kio.WritePayload
	if err := json.Unmarshal(payload, &decoded); err == nil {
		for _, obj := range decoded.Objs {
			name, _ := obj["N"].(string)
			item, ok := obj[kio.ValueKey]
			if !ok {
				continue
			}
			b.values = append(b.values, kio.WriteValue{Name: name, Value: item})
		}
	}
	return nil
}

func (b *fakeKIOBroker) Subscribe(ctx context.Context, gatewayID int, topic string, qos byte) error {
	return b.subscribeErr
}

func (b *fakeKIOBroker) PublishAndWaitKIOAck(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool, qid int64) (*kio.WriteAck, bool, error) {
	if err := b.Publish(ctx, gatewayID, topic, payload, qos, retain); err != nil {
		return nil, false, err
	}
	if b.waitErr != nil {
		return nil, b.waitBrokerAccepted, b.waitErr
	}
	if b.ack != nil {
		return b.ack, true, nil
	}
	return &kio.WriteAck{QID: qid, ProcessStep: 100, Result: "ok", Success: true}, true, nil
}

func TestDetectionRunsServiceLifecycleAndNotes(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	channels := pipeline.NewChannels()
	taskManager := pipeline.NewTaskManager()
	Project := createServiceProject(t, repo)
	flows := pipeline.NewTaskFlowExecutor(repo, pipeline.NewTagManager(), taskManager, channels)
	flows.Load([]models.TaskFlow{
		{ID: 5101, ProjectID: Project.ID, FlowCode: "svc-project-start", Name: "Service Project Start", Enabled: true, TriggerType: models.TaskFlowTriggerProjectStart, ActionType: models.TaskFlowActionJavaScript, ActionScript: `({task_id: task_params.task_id});`, TimeoutMS: 3000},
		{ID: 5102, ProjectID: Project.ID, FlowCode: "svc-project-end", Name: "Service Project End", Enabled: true, TriggerType: models.TaskFlowTriggerProjectEnd, ActionType: models.TaskFlowActionJavaScript, ActionScript: `({end_type: task_params.end_type});`, TimeoutMS: 3000},
	})
	flows.Start(1)
	service := NewDetectionRunsService(repo, taskManager, DetectionRunsRuntimeDeps{Channels: channels, Flows: flows})

	task, err := service.Start(database.StartDetectionOptions{ProjectID: Project.ID, TestNo: "T-SVC-1", FactoryNo: "F-T-SVC-1", Mode: "standard", DurationSec: 60, OperatorNote: "note"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(database.StartDetectionOptions{ProjectID: Project.ID, TestNo: "T-SVC-2", FactoryNo: "F-T-SVC-2", Mode: "standard"}); !errors.Is(err, database.ErrProjectAlreadyRunning) {
		t.Fatalf("expected duplicate running error, got %v", err)
	}
	if _, err := service.AddNote(AddNoteInput{TaskID: task.ID, Content: "memo", ActorType: "user", ActorID: "1"}); err != nil {
		t.Fatal(err)
	}
	if events, err := service.ListEvents(task.ID, 10); err != nil || len(events) != 1 || events[0].EventType != models.DetectionEventRunStarted {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	if summary, err := service.Summary(task.ID); err != nil || summary.ResultStatus != models.DetectionSummaryStatusRunning {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if len(service.Active()) != 1 {
		t.Fatal("expected one active task")
	}
	if tasks, err := service.List(database.DetectionTaskFilter{ProjectID: &Project.ID, Status: models.DetectionStatusRunning}); err != nil || len(tasks) != 1 {
		t.Fatalf("list tasks=%+v err=%v", tasks, err)
	}
	if notes, err := service.ListNotes(task.ID, 10); err != nil || len(notes) != 1 {
		t.Fatalf("notes len=%d err=%v", len(notes), err)
	}
	if detail, err := service.Get(task.ID); err != nil || detail.OperatorNote != "note" || len(detail.RecentNotes) != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	originalExpectedEnd := *task.ExpectedEndAt
	if _, err := service.Pause(task.ID, "pause for test"); err != nil {
		t.Fatal(err)
	}
	pausedSince := time.Now().Add(-5 * time.Second)
	if err := db.Model(&models.DetectionTask{}).Where("id = ?", task.ID).Update("pause_started_at", pausedSince).Error; err != nil {
		t.Fatal(err)
	}
	if len(service.Active()) != 0 {
		t.Fatal("paused task should be removed from runtime active map")
	}
	if summary, err := service.Summary(task.ID); err != nil || summary.ResultStatus != models.DetectionSummaryStatusRunning {
		t.Fatalf("paused summary=%+v err=%v", summary, err)
	}
	resumed, err := service.Resume(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.PausedDurationMS < 4900 || resumed.ExpectedEndAt == nil || resumed.ExpectedEndAt.Sub(originalExpectedEnd) < 4900*time.Millisecond {
		t.Fatalf("resume should exclude pause time from planned duration: %+v original_expected=%s", resumed, originalExpectedEnd)
	}
	if len(service.Active()) != 1 {
		t.Fatal("resumed task should return to runtime active map")
	}
	if err := repo.InsertHistoryBatch([]*models.StoreTask{{GatewayID: 1, Topic: "topic", ProjectID: Project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 1, VarName: "temp", ProjectCode: Project.ProjectCode, Value: 20, Quality: 1, Timestamp: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stop(task.ID, "done"); err != nil {
		t.Fatal(err)
	}
	waitServiceTaskFlowRunStatus(t, db, 5101, models.TaskFlowStatusSuccess, time.Second)
	waitServiceTaskFlowRunStatus(t, db, 5102, models.TaskFlowStatusSuccess, time.Second)
	if features, err := service.Features(task.ID); err != nil || len(features) != 1 || features[0].AvgValue == nil || *features[0].AvgValue != 20 {
		t.Fatalf("features=%+v err=%v", features, err)
	}
	if summary, err := service.Summary(task.ID); err != nil || summary.ResultStatus != models.DetectionSummaryStatusOK {
		t.Fatalf("stopped summary=%+v err=%v", summary, err)
	}
	notifications := drainServiceNotifications(channels.Notify)
	if !hasServiceNotification(notifications, models.NotificationDetectionRunStarted) || !hasServiceNotification(notifications, models.NotificationDetectionRunStopped) || !hasServiceNotification(notifications, models.NotificationDetectionResultOK) {
		t.Fatalf("expected lifecycle and ok notifications, got %+v", notifications)
	}
	task, err = service.Start(database.StartDetectionOptions{ProjectID: Project.ID, TestNo: "T-SVC-3", FactoryNo: "F-T-SVC-3", Mode: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AbnormalStop(task.ID, "alarm"); err != nil {
		t.Fatal(err)
	}
	if events, err := service.ListEvents(task.ID, 10); err != nil || len(events) != 2 || events[1].EventType != models.DetectionEventRunAbnormalStop {
		t.Fatalf("abnormal events=%+v err=%v", events, err)
	}
	if _, err := service.AbnormalStop(task.ID, ""); err == nil {
		t.Fatal("expected reason validation error")
	}
	if status := HTTPStatusForError(database.ErrProjectAlreadyRunning); status != 409 {
		t.Fatalf("unexpected status %d", status)
	}
	if status := HTTPStatusForError(gorm.ErrRecordNotFound); status != 404 {
		t.Fatalf("record not found should map to 404, got %d", status)
	}
	if status := HTTPStatusForError(database.ErrReferenced); status != 409 {
		t.Fatalf("referenced resource should map to 409, got %d", status)
	}
}

func waitServiceTaskFlowRunStatus(t *testing.T, db *gorm.DB, flowID uint64, status string, timeout time.Duration) models.TaskFlowRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var run models.TaskFlowRun
	for time.Now().Before(deadline) {
		if err := db.Order("id desc").First(&run, "flow_id = ?", flowID).Error; err == nil && run.Status == status {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := db.Order("id desc").First(&run, "flow_id = ?", flowID).Error; err != nil {
		t.Fatalf("expected task flow run flow_id=%d status=%s, err=%v", flowID, status, err)
	}
	t.Fatalf("expected task flow run flow_id=%d status=%s, got %+v", flowID, status, run)
	return run
}

func drainServiceNotifications(ch <-chan *models.RuntimeNotification) []*models.RuntimeNotification {
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

func hasServiceNotification(items []*models.RuntimeNotification, notificationType string) bool {
	for _, item := range items {
		if item.Type == notificationType {
			return true
		}
	}
	return false
}

func TestDetectionRunsServiceStartSnapshotsAndInitialAlarm(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	channels := pipeline.NewChannels()
	service := NewDetectionRunsService(repo, pipeline.NewTaskManager(), DetectionRunsRuntimeDeps{Tags: tags, Channels: channels})
	Project := createServiceProject(t, repo)

	tag := models.TagConfig{
		VarID:       900,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "temp",
		RawName:     "temp",
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1, Enabled: true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	route, err := repo.EnsureDefaultStorageRouteForTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.StorageRoute{}).Where("id = ?", route.ID).Updates(map[string]interface{}{
		"enabled":        true,
		"trigger_mode":   models.StoreTriggerOnStart,
		"store_on_start": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tags.Load([]models.TagConfig{tag})
	runtimeTag, ok := tags.Get(tag.VarID)
	if !ok {
		t.Fatal("expected runtime tag")
	}
	runtimeTag.UpdateNumeric(12, time.Now(), 1)

	limitH := 10.0
	standard := &models.DetectionStandard{StandardCode: "STD-SVC", Name: "Standard", ProjectID: &Project.ID, ProjectCode: Project.ProjectCode, Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:        tag.VarID,
		VarName:      tag.VarName,
		CheckEnabled: true,
		AlarmEnabled: true,
		StoreEnabled: true,
		CheckOnStart: true,
		LimitH:       &limitH,
	}}); err != nil {
		t.Fatal(err)
	}

	task, err := service.Start(database.StartDetectionOptions{ProjectID: Project.ID, TestNo: "T-SVC-SNAPSHOT", FactoryNo: "F-T-SVC-SNAPSHOT", Mode: "standard", StandardID: &standard.ID})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case store := <-channels.Store:
		if store.TaskID != task.ID || store.VarID != tag.VarID || store.Value != 12 {
			t.Fatalf("unexpected start snapshot store task: %+v", store)
		}
	case <-time.After(time.Second):
		t.Fatal("expected detection start snapshot store task")
	}
	select {
	case alarm := <-channels.Alarm:
		if alarm.Action != models.DetectionAlarmActionEnter || alarm.Alarm.TaskID != task.ID || alarm.Alarm.AlarmType != "above_h" || alarm.Alarm.Status != models.DetectionAlarmStatusActive {
			t.Fatalf("unexpected initial alarm event: %+v", alarm)
		}
	case <-time.After(time.Second):
		t.Fatal("expected detection start initial alarm event")
	}
}

func TestDetectionRunsServiceQualifiedHoldGuardStopsRun(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	tags := pipeline.NewTagManager()
	taskManager := pipeline.NewTaskManager()
	service := NewDetectionRunsService(repo, taskManager, DetectionRunsRuntimeDeps{Tags: tags})
	project := createServiceProject(t, repo)

	tag := models.TagConfig{
		VarID:       901,
		GatewayID:   1,
		SourceTopic: "topic",
		SourcePath:  "qualified_temp",
		RawName:     "qualified_temp",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		VarName:     "qualified_temp",
		JSONPath:    "qualified_temp",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	tags.Load([]models.TagConfig{tag})
	runtimeTag, ok := tags.Get(tag.VarID)
	if !ok {
		t.Fatal("expected runtime tag")
	}
	runtimeTag.UpdateNumeric(50, time.Now(), 1)

	limitL := 0.0
	limitH := 100.0
	standard := &models.DetectionStandard{StandardCode: "STD-SVC-HOLD", Name: "Hold Standard", ProjectID: &project.ID, ProjectCode: project.ProjectCode, Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:         tag.VarID,
		VarName:       tag.VarName,
		CheckEnabled:  true,
		AlarmEnabled:  true,
		CheckMethod:   models.CheckMethodNumericRange,
		QualityPolicy: models.QualityPolicyIgnoreBad,
		LimitL:        &limitL,
		LimitH:        &limitH,
	}}); err != nil {
		t.Fatal(err)
	}

	task, err := service.Start(database.StartDetectionOptions{ProjectID: project.ID, TestNo: "T-SVC-HOLD", FactoryNo: "F-T-SVC-HOLD", Mode: "standard", StandardID: &standard.ID, EndPolicy: models.DetectionEndPolicyQualifiedHold, QualifiedHoldMS: 50})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := repo.GetDetectionTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == models.DetectionStatusStopped {
			if got.EndType != models.DetectionEndQualifiedHold {
				t.Fatalf("expected qualified hold end type, got %+v", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected qualified hold guard to stop task %d", task.ID)
}

func TestReportTemplatesService(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	service := NewReportTemplatesService(repo)

	template, err := service.Create(CreateReportTemplateInput{TemplateCode: "RPT-SVC", Name: "Report", FileRef: "templates/report.xlsx", ParamsSchemaJSON: `[{"key":"inlet_area_m2","type":"number"}]`})
	if err != nil {
		t.Fatal(err)
	}
	if template.FileKind != "xlsx" || template.Version != 1 || !strings.Contains(template.ParamsSchemaJSON, "inlet_area_m2") {
		t.Fatalf("unexpected template defaults: %+v", template)
	}
	if templates, err := service.List(database.ReportTemplateFilter{Keyword: "RPT"}); err != nil || len(templates) != 1 {
		t.Fatalf("templates len=%d err=%v", len(templates), err)
	}
	if got, err := service.Update(template.ID, map[string]interface{}{"remark": "updated"}); err != nil || got.Remark != "updated" {
		t.Fatalf("update got=%+v err=%v", got, err)
	}
	if _, err := service.Update(template.ID, map[string]interface{}{"params_schema_json": `"bad"`}); err == nil {
		t.Fatal("expected params schema validation error")
	}
	if err := service.Delete(template.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(CreateReportTemplateInput{Name: "bad", FileRef: "x"}); err == nil {
		t.Fatal("expected code validation error")
	}
}

func TestSystemConfigServiceDatabaseConfig(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigPath = filepath.Join(t.TempDir(), "config.json")
	cfg.Database.Password = "secret"
	service := NewSystemConfigService(cfg)

	view := service.DatabaseConfig()
	if !view.PasswordSet || view.RestartRequired {
		t.Fatalf("unexpected initial view: %+v", view)
	}
	port := 4406
	host := "db.local"
	updated, err := service.UpdateDatabaseConfig(DatabaseConfigUpdate{Host: &host, Port: &port})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Host != host || updated.Port != port || !updated.PasswordSet || !updated.RestartRequired {
		t.Fatalf("unexpected updated view: %+v", updated)
	}
	badPort := 70000
	if _, err := service.UpdateDatabaseConfig(DatabaseConfigUpdate{Port: &badPort}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestDetectionPlansServiceStartsRunAndMarksPlan(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	project := createServiceProject(t, repo)
	if err := repo.CreateTag(&models.TagConfig{VarID: 1001, GatewayID: 1, SourceTopic: "topic", SourcePath: "supply_air", RawName: "supply_air", ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "supply_air", JSONPath: "supply_air", DataType: "FLOAT", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	standard := &models.DetectionStandard{StandardCode: "STD-PLAN", Name: "Plan Standard", ProjectID: &project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{
		{VarID: 1001, VarName: "supply_air", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true, CheckOnStart: true, CheckMethod: models.CheckMethodNumericRange, QualityPolicy: models.QualityPolicyIgnoreBad},
	}); err != nil {
		t.Fatal(err)
	}
	plan := &models.DetectionPlan{
		PlanNo:            "PLAN-SVC-1",
		SourceSystem:      "mes",
		ExternalPlanID:    "MES-SVC-1",
		FactoryNo:         "FAC-SVC-1",
		DeviceModel:       "MODEL-A",
		Mode:              "standard",
		StandardCode:      standard.StandardCode,
		ReportRequestJSON: `{"enabled":true,"reports":[{"template_code":"PLAN-SVC-TPL","report_name":"计划报表","variables":[{"var_name":"supply_air"}],"params":{"operator":"tester","end_policy":"qualified_hold","qualified_hold_minutes":"10"}}]}`,
		Status:            models.DetectionPlanStatusPending,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	tags := pipeline.NewTagManager()
	tasks := pipeline.NewTaskManager()
	flows := pipeline.NewTaskFlowExecutor(repo, tags, tasks, pipeline.NewChannels())
	requestVarID := int64(990001)
	tags.Load([]models.TagConfig{{
		VarID:       requestVarID,
		SourceType:  models.TagSourceVirtual,
		VarName:     "task_request",
		RawName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
	}})
	flowSteps, err := json.Marshal([]map[string]any{{
		"code":   "start",
		"module": models.TaskFlowActionBuiltinStartDetectionRun,
		"params": map[string]any{
			"project_id":        map[string]any{"source": "trigger_param", "key": "project_id"},
			"factory_no":        map[string]any{"source": "trigger_param", "key": "factory_no", "optional": true},
			"customer_name":     map[string]any{"source": "trigger_param", "key": "customer_name", "optional": true},
			"device_model":      map[string]any{"source": "trigger_param", "key": "device_model", "optional": true},
			"test_no":           map[string]any{"source": "trigger_param", "key": "test_no"},
			"standard_id":       map[string]any{"source": "trigger_param", "key": "standard_id"},
			"config_enabled":    map[string]any{"source": "trigger_param", "key": "config_enabled"},
			"config_code":       map[string]any{"source": "trigger_param", "key": "config_code"},
			"config_name":       map[string]any{"source": "trigger_param", "key": "config_name"},
			"config_version":    map[string]any{"source": "trigger_param", "key": "config_version"},
			"config_hash":       map[string]any{"source": "trigger_param", "key": "config_hash"},
			"process_params":    map[string]any{"source": "trigger_param", "key": "process_params", "optional": true},
			"report_request":    map[string]any{"source": "trigger_param", "key": "report_request", "optional": true},
			"end_policy":        map[string]any{"source": "trigger_param", "key": "end_policy", "optional": true},
			"qualified_hold_ms": map[string]any{"source": "trigger_param", "key": "qualified_hold_ms", "optional": true},
			"operator_note":     map[string]any{"source": "trigger_param", "key": "operator_note", "optional": true},
			"enable_storage":    true,
			"enable_alarm":      true,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	flows.Load([]models.TaskFlow{{
		ID:              910001,
		ProjectID:       project.ID,
		FlowCode:        "plan-request-start-detection",
		Name:            "Plan Request Start Detection",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start_detection"`,
		StepsJSON:       string(flowSteps),
		TimeoutMS:       3000,
		Vars:            []models.TaskFlowVar{{FlowID: 910001, ProjectID: project.ID, VarID: requestVarID, VarName: "task_request", Role: models.TaskFlowVarRoleWatch}},
	}})
	flows.Start(1)
	variables := NewVariableWriteService(repo, tags, nil, flows)
	detection := NewDetectionRunsService(repo, tasks)
	plans := NewDetectionPlansService(repo, detection, "edge-local", variables)
	result, err := plans.Start(StartDetectionPlanInput{PlanID: plan.ID, ProjectID: project.ID, RequestVarName: "task_request", WaitTaskTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.FactoryNo != plan.FactoryNo || result.Task.StandardID == nil || *result.Task.StandardID != standard.ID {
		t.Fatalf("unexpected started task: %+v", result.Task)
	}
	if result.Task.EndPolicy != models.DetectionEndPolicyQualifiedHold || result.Task.QualifiedHoldMS != 10*60*1000 {
		t.Fatalf("expected imported plan to start a 10 minute qualified-hold task, got policy=%s hold_ms=%d", result.Task.EndPolicy, result.Task.QualifiedHoldMS)
	}
	if result.Plan.Status != models.DetectionPlanStatusStarted || result.Plan.StartedTaskID == nil || *result.Plan.StartedTaskID != result.Task.ID {
		t.Fatalf("unexpected started plan: %+v", result.Plan)
	}
	if len(result.Task.ReportRequests) != 1 || result.Task.ReportRequests[0].ReportName != "计划报表" || result.Task.ReportRequests[0].TemplateCode != "PLAN-SVC-TPL" || !strings.Contains(result.Task.ReportRequests[0].ParamsJSON, `"operator":"tester"`) {
		t.Fatalf("expected plan report request snapshot to be frozen, got %+v", result.Task.ReportRequests)
	}
	if _, err := plans.Start(StartDetectionPlanInput{PlanID: plan.ID, ProjectID: project.ID}); !errors.Is(err, database.ErrDetectionPlanNotPending) {
		t.Fatalf("expected not pending on duplicate start, got %v", err)
	}
}

func TestNotificationDispatcherPersistsAndPublishes(t *testing.T) {
	db := newServiceTestDB(t)
	repo := database.NewRepository(db)
	user := &models.SysUser{Username: "notify-user", PasswordHash: "hash", Role: "operator", Enabled: true}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	hub := NewNotificationHub(nil)
	sub, cancel := hub.Subscribe(1)
	defer cancel()
	dispatcher := NewNotificationDispatcher(repo, hub)
	dispatcher.Dispatch(&models.RuntimeNotification{
		ID:         "dispatcher-event-1",
		Type:       models.NotificationDetectionRunStarted,
		Level:      models.NotificationLevelInfo,
		ProjectID:  1,
		Message:    "started",
		OccurredAt: time.Date(2026, 5, 30, 19, 10, 0, 0, time.UTC),
	})
	if got := waitNotification(t, sub); got.ID != "dispatcher-event-1" {
		t.Fatalf("unexpected published notification: %+v", got)
	}
	unread, err := repo.CountUnreadNotifications(user.ID)
	if err != nil || unread != 1 {
		t.Fatalf("unread=%d err=%v", unread, err)
	}
	dispatcher.Dispatch(nil)
	NewNotificationDispatcher(nil, hub).Dispatch(models.NewRuntimeNotification(models.NotificationDetectionResultOK, models.NotificationLevelSuccess, "ok", time.Now()))
	if got := waitNotification(t, sub); got.Type != models.NotificationDetectionResultOK {
		t.Fatalf("unexpected hub-only notification: %+v", got)
	}
}

func newServiceTestDB(t *testing.T) *gorm.DB {
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

func createServiceProject(t *testing.T, repo *database.Repository) models.Project {
	t.Helper()
	Project := &models.Project{ProjectCode: "AC-SVC", Name: "Project", Enabled: true}
	if err := repo.CreateProject(Project); err != nil {
		t.Fatal(err)
	}
	return *Project
}
