package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestRealtimeWSHandlerClientMessages(t *testing.T) {
	service := services.NewRealtimeWSService(pipeline.NewTagManager(), pipeline.NewTaskManager())
	handler := NewRealtimeWSHandler(service, nil, nil)
	current := services.DefaultRealtimeSubscription()
	principal := auth.Principal{AuthType: "user", UserID: 1, Role: auth.RoleAdmin}

	next, responses := handler.handleClientMessage(current, wsClientMessage{
		Type:       "subscribe",
		RequestID:  "req-sub",
		Topics:     []string{"variables", "notifications"},
		SourceType: models.TagSourceMQTT,
		GatewayID:  intPtrForHandlerTest(1),
		VarIDs:     flexibleInt64List{10},
	}, principal)
	if len(responses) != 1 || responses[0].Type != services.WSTypeSubscriptionUpdated || !next.Wants("realtime.variables") || !next.Wants("notifications") || next.Wants("detection.runs") || next.SourceType != models.TagSourceMQTT || !next.VarIDs[10] {
		t.Fatalf("unexpected subscribe result next=%+v responses=%+v", next, responses)
	}

	_, responses = handler.handleClientMessage(next, wsClientMessage{Type: "command.write_variable", RequestID: "req-cmd", CommandID: "cmd-1"}, principal)
	if len(responses) != 1 || responses[0].Type != services.WSTypeError || responses[0].Error.Code != "unsupported_command" {
		t.Fatalf("unexpected command error: %+v", responses)
	}
	_, responses = handler.handleClientMessage(next, wsClientMessage{Type: "bad", RequestID: "req-bad"}, principal)
	if len(responses) != 1 || responses[0].Error.Code != "unsupported_message" {
		t.Fatalf("unexpected unsupported error: %+v", responses)
	}
	_, responses = handler.handleClientMessage(next, wsClientMessage{Type: "subscribe", RequestID: "req-empty", Topics: []string{"bad"}}, principal)
	if len(responses) != 1 || responses[0].Error.Code != "invalid_subscription" {
		t.Fatalf("unexpected invalid subscription error: %+v", responses)
	}
	_, responses = handler.handleClientMessage(next, wsClientMessage{Type: "ping"}, principal)
	if len(responses) != 1 || responses[0].Type != services.WSTypeHeartbeat {
		t.Fatalf("unexpected ping response: %+v", responses)
	}
}

func TestRealtimeWSHandlerSnapshotIntervalInjection(t *testing.T) {
	handler := NewRealtimeWSHandler(services.NewRealtimeWSService(pipeline.NewTagManager(), pipeline.NewTaskManager()), nil, nil)
	if handler.interval != defaultWSSnapshotInterval {
		t.Fatalf("unexpected default interval: %s", handler.interval)
	}
	handler.WithSnapshotInterval(120 * time.Millisecond)
	if handler.interval != minimumWSSnapshotInterval {
		t.Fatalf("expected lower clamp, got %s", handler.interval)
	}
	handler.WithSnapshotInterval(6 * time.Second)
	if handler.interval != maximumWSSnapshotInterval {
		t.Fatalf("expected upper clamp, got %s", handler.interval)
	}
	handler.WithSnapshotInterval(750 * time.Millisecond)
	if handler.interval != 750*time.Millisecond {
		t.Fatalf("expected injected interval, got %s", handler.interval)
	}
}

func TestRealtimeWSHandlerWriteVariableErrorIncludesPartialResult(t *testing.T) {
	projectID := uint(7)
	tags := pipeline.NewTagManager()
	tags.Load([]models.TagConfig{{
		VarID:         7101,
		GatewayID:     1,
		SourceType:    models.TagSourceMQTT,
		SourcePath:    "pid_sp",
		RawName:       "pid_sp",
		ProjectID:     &projectID,
		ProjectCode:   "AC-PID",
		VarName:       "pid_sp",
		JSONPath:      "pid_sp",
		DataType:      "FLOAT",
		ScaleFactor:   1,
		RWMode:        models.RWModeReadWrite,
		Writable:      true,
		WriteSourceID: 1,
		WritePath:     "pid_sp",
		WriteDataType: "FLOAT",
		Enabled:       true,
		Discovered:    true,
	}})
	broker := &partialResultKIOBroker{waitErr: errors.New("ack timeout")}
	handler := NewRealtimeWSHandler(
		services.NewRealtimeWSService(tags, pipeline.NewTaskManager()),
		nil,
		nil,
		services.NewVariableWriteService(nil, tags, services.NewKIOWriteService(broker), nil),
	)
	_, responses := handler.handleClientMessage(services.DefaultRealtimeSubscription(), wsClientMessage{
		Type:      "command.write_variable",
		RequestID: "req-pid-write",
		CommandID: "cmd-pid-write",
		Payload:   []byte(`{"var_id":"7101","value":12.5,"wait_ack":true,"ack_timeout_sec":1}`),
	}, auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeError {
		t.Fatalf("unexpected write error response: %+v", responses)
	}
	payload, ok := responses[0].Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected error payload map, got %#v", responses[0].Payload)
	}
	result, ok := payload["result"].(services.VariableWriteResult)
	if !ok {
		t.Fatalf("expected partial variable write result, got %#v", payload["result"])
	}
	if result.VarIDText != "7101" || result.KIO == nil || !result.KIO.BrokerAccepted || result.KIO.Status != "ack_timeout_or_unmatched" {
		t.Fatalf("unexpected partial result: %+v", result)
	}
}

func TestRealtimeWSHandlerSubscriptionAcceptsStringVarIDs(t *testing.T) {
	var msg wsClientMessage
	if err := json.Unmarshal([]byte(`{"type":"subscribe","request_id":"req-big","topics":["variables"],"var_ids":["9212397624135540849",10]}`), &msg); err != nil {
		t.Fatalf("expected string var_ids to decode: %v", err)
	}
	service := services.NewRealtimeWSService(pipeline.NewTagManager(), pipeline.NewTaskManager())
	handler := NewRealtimeWSHandler(service, nil, nil)
	next, responses := handler.handleClientMessage(services.DefaultRealtimeSubscription(), msg, auth.Principal{AuthType: "user", UserID: 1, Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeSubscriptionUpdated || !next.VarIDs[9212397624135540849] || !next.VarIDs[10] {
		t.Fatalf("unexpected subscribe response next=%+v responses=%+v", next, responses)
	}
	payload, ok := responses[0].Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected subscription payload map, got %#v", responses[0].Payload)
	}
	texts, ok := payload["var_id_texts"].([]string)
	if !ok || len(texts) != 2 || texts[1] != "9212397624135540849" {
		t.Fatalf("expected exact var_id_texts, got %#v", payload["var_id_texts"])
	}

	if err := json.Unmarshal([]byte(`{"type":"subscribe","var_ids":["not-an-id"]}`), &msg); err == nil {
		t.Fatal("expected invalid string var_id to fail")
	}
}

func TestRealtimeWSHandlerDetectionCommands(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	Project := createHandlerProject(t, repo)
	tasks := pipeline.NewTaskManager()
	handler := NewRealtimeWSHandler(
		services.NewRealtimeWSService(pipeline.NewTagManager(), tasks),
		services.NewDetectionRunsService(repo, tasks),
		repo,
	)
	principal := auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin}
	current := services.DefaultRealtimeSubscription()

	_, responses := handler.handleClientMessage(current, wsClientMessage{
		Type:      "command.detection.start",
		RequestID: "req-start",
		CommandID: "cmd-start",
		Payload:   []byte(`{"project_id":` + itoaHandler(uint64(Project.ID)) + `,"factory_no":"F-WS-H-1","test_no":"WS-H-1","mode":"standard"}`),
	}, principal)
	if len(responses) != 1 || responses[0].Type != services.WSTypeCommandAck {
		t.Fatalf("unexpected start response: %+v", responses)
	}
	var task models.DetectionTask
	if err := db.First(&task, "test_no = ?", "WS-H-1").Error; err != nil {
		t.Fatal(err)
	}
	_, responses = handler.handleClientMessage(current, wsClientMessage{
		Type:      "command.detection.stop",
		RequestID: "req-stop",
		CommandID: "cmd-stop",
		Payload:   []byte(`{"task_id":` + itoaHandler(uint64(task.ID)) + `,"reason":"done"}`),
	}, principal)
	if len(responses) != 1 || responses[0].Type != services.WSTypeCommandAck {
		t.Fatalf("unexpected stop response: %+v", responses)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("target_type = ? AND result = ?", "ws_command", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("expected 2 ws audit rows, got %d", auditCount)
	}

	_, responses = handler.handleClientMessage(current, wsClientMessage{
		Type:      "command.detection.start",
		RequestID: "req-forbidden",
		CommandID: "cmd-forbidden",
		Payload:   []byte(`{"project_id":` + itoaHandler(uint64(Project.ID)) + `,"factory_no":"F-WS-H-2","test_no":"WS-H-2","mode":"standard"}`),
	}, auth.Principal{AuthType: "user", UserID: 2, Role: auth.RoleGuest})
	if len(responses) != 1 || responses[0].Error == nil || responses[0].Error.Code != "forbidden" {
		t.Fatalf("unexpected forbidden response: %+v", responses)
	}
}

func TestRealtimeWSHandlerWriteVirtualVariableCommand(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	project := models.Project{ProjectCode: "AC-WS", Name: "WS Project", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	projectID := project.ID
	varID := int64(9212397624135540849)
	tags := pipeline.NewTagManager()
	tags.Load([]models.TagConfig{{
		VarID:       varID,
		SourceType:  models.TagSourceVirtual,
		GatewayID:   0,
		SourceTopic: "virtual",
		SourcePath:  "task_request",
		RawName:     "task_request",
		VarName:     "task_request",
		JSONPath:    "task_request",
		DataType:    "STRING",
		ProjectID:   &projectID,
		ProjectCode: project.ProjectCode,
		Enabled:     true,
	}})
	tasks := pipeline.NewTaskManager()
	channels := pipeline.NewChannels()
	flows := pipeline.NewTaskFlowExecutor(repo, tags, tasks, channels)
	flows.Start(1)
	flows.Load([]models.TaskFlow{{
		ID:              901,
		ProjectID:       projectID,
		FlowCode:        "ws-write-variable-trigger",
		Name:            "WS Write Variable Trigger",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: `task_params.command === "start"`,
		ActionType:      models.TaskFlowActionJavaScript,
		ActionScript:    `log.info("command=" + context["param.command"]); ({command: context["param.command"]});`,
		TimeoutMS:       3000,
		Vars: []models.TaskFlowVar{{
			FlowID:    901,
			ProjectID: projectID,
			VarID:     varID,
			Role:      models.TaskFlowVarRoleWatch,
		}},
	}})
	handler := NewRealtimeWSHandler(
		services.NewRealtimeWSService(tags, tasks),
		nil,
		repo,
		services.NewVariableWriteService(repo, tags, nil, flows),
	)
	_, responses := handler.handleClientMessage(services.DefaultRealtimeSubscription(), wsClientMessage{
		Type:      "command.write_variable",
		RequestID: "req-write",
		CommandID: "cmd-write",
		Payload:   []byte(`{"var_id":"9212397624135540849","value":"{\"command\":\"start\"}"}`),
	}, auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeCommandAck {
		t.Fatalf("unexpected write response: %+v", responses)
	}
	payload, ok := responses[0].Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected command payload map, got %#v", responses[0].Payload)
	}
	result, ok := payload["result"].(services.VariableWriteResult)
	if !ok || result.VarIDText != "9212397624135540849" {
		t.Fatalf("expected exact write result var_id_text, got %#v", payload["result"])
	}
	_, responses = handler.handleClientMessage(services.DefaultRealtimeSubscription(), wsClientMessage{
		Type:      "command.write_variable",
		RequestID: "req-write-project",
		CommandID: "cmd-write-project",
		Payload:   []byte(`{"project_id":` + itoaHandler(uint64(projectID)) + `,"var_name":"task_request","value":"{\"command\":\"start\",\"source\":\"project_id\"}"}`),
	}, auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeCommandAck {
		t.Fatalf("unexpected project+var_name write response: %+v", responses)
	}
	_, responses = handler.handleClientMessage(services.DefaultRealtimeSubscription(), wsClientMessage{
		Type:      "command.write_variable",
		RequestID: "req-write-project-code",
		CommandID: "cmd-write-project-code",
		Payload:   []byte(`{"project_code":"AC-WS","var_name":"task_request","value":"{\"command\":\"start\",\"source\":\"project_code\"}"}`),
	}, auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeCommandAck {
		t.Fatalf("unexpected project_code+var_name write response: %+v", responses)
	}
	tag, ok := tags.Get(varID)
	if !ok || tag.RuntimeState().StrValue != `{"command":"start","source":"project_code"}` {
		t.Fatalf("expected virtual variable write, ok=%v tag=%+v", ok, tag)
	}
	var run models.TaskFlowRun
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := db.First(&run, "flow_id = ?", 901).Error; err == nil && run.Status == models.TaskFlowStatusSuccess {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if run.Status != models.TaskFlowStatusSuccess || !strings.Contains(run.ScriptLogs, "command=start") || !strings.Contains(run.InputSnapshot, "trigger_params") {
		t.Fatalf("expected ws variable write to trigger task flow, got %+v", run)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND result = ?", "ws.command.write_variable", "success").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 3 {
		t.Fatalf("expected three ws variable write audit rows, got %d", auditCount)
	}
}

func TestRealtimeWSHandlerWriteVariableByNameErrorsOnDuplicate(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	projectID := uint(7)
	tags := pipeline.NewTagManager()
	tags.Load([]models.TagConfig{
		{VarID: 101, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", SourcePath: "dup_a", RawName: "dup_a", VarName: "dup", JSONPath: "dup_a", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-DUP", Enabled: true},
		{VarID: 102, SourceType: models.TagSourceVirtual, GatewayID: 0, SourceTopic: "virtual", SourcePath: "dup_b", RawName: "dup_b", VarName: "dup", JSONPath: "dup_b", DataType: "FLOAT", ProjectID: &projectID, ProjectCode: "AC-DUP", Enabled: true},
	})
	handler := NewRealtimeWSHandler(
		services.NewRealtimeWSService(tags, pipeline.NewTaskManager()),
		nil,
		repo,
		services.NewVariableWriteService(repo, tags, nil, nil),
	)
	_, responses := handler.handleClientMessage(services.DefaultRealtimeSubscription(), wsClientMessage{
		Type:      "command.write_variable",
		RequestID: "req-dup",
		CommandID: "cmd-dup",
		Payload:   []byte(`{"project_id":7,"var_name":"dup","value":1}`),
	}, auth.Principal{AuthType: "user", UserID: 1, Username: "admin", Role: auth.RoleAdmin})
	if len(responses) != 1 || responses[0].Type != services.WSTypeError || responses[0].Error.Code != "ambiguous_variable" {
		t.Fatalf("unexpected duplicate response: %+v", responses)
	}
	var auditCount int64
	if err := db.Model(&models.SysAuditLog{}).Where("action = ? AND result = ?", "ws.command.write_variable", "failed").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected failed ws audit row, got %d", auditCount)
	}
}

func TestRealtimeWSHandlerServiceTokenEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	serviceToken := "edge-ws-token"
	if err := repo.UpsertServiceClient(models.SysServiceClient{
		ClientID:   "main-server",
		SecretHash: auth.HashOpaqueToken(serviceToken),
		Scopes:     auth.NormalizeScopes([]string{auth.ScopeServiceRealtimeRead}),
		Enabled:    true,
	}); err != nil {
		t.Fatal(err)
	}
	projectID := uint(1)
	tags := pipeline.NewTagManager()
	tags.Load([]models.TagConfig{{
		VarID:       9101,
		SourceType:  models.TagSourceMQTT,
		GatewayID:   1,
		SourceTopic: "topic/a",
		SourcePath:  "$.temp",
		RawName:     "raw_temp",
		VarName:     "temp",
		JSONPath:    "$.temp",
		DataType:    "FLOAT",
		ProjectID:   &projectID,
		ProjectCode: "AC-WS-SVC",
		Enabled:     true,
	}})
	if tag, ok := tags.Get(9101); ok {
		tag.UpdateNumeric(26.5, time.Now(), 192)
	}

	router := gin.New()
	group := router.Group("/api/v1")
	authService := auth.NewService(repo, auth.NewJWTManager("test-secret", time.Hour), auth.Options{EdgeInstanceID: "edge-test"})
	NewRealtimeWSHandler(services.NewRealtimeWSService(tags, pipeline.NewTaskManager()), nil, repo).Register(group, authService)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	badURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/edge-control/ws?topic=realtime.variables&project_id=1"
	if conn, resp, err := websocket.DefaultDialer.Dial(badURL, nil); err == nil {
		_ = conn.Close()
		t.Fatal("expected websocket auth failure without service token")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected websocket 401 without service token, resp=%v err=%v", resp, err)
	}

	headers := http.Header{"Authorization": []string{"Bearer " + serviceToken}}
	conn, _, err := websocket.DefaultDialer.Dial(badURL, headers)
	if err != nil {
		t.Fatalf("service websocket dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	ready := readHandlerWSMessageOfType(t, conn, services.WSTypeConnectionReady)
	if ready["type"] != services.WSTypeConnectionReady {
		t.Fatalf("unexpected ready message: %#v", ready)
	}
	snapshot := readHandlerWSMessageOfType(t, conn, services.WSTypeRealtimeVariablesSnapshot)
	if !strings.Contains(mustJSONForHandlerTest(t, snapshot), `"var_id_text":"9101"`) {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestSubscriptionFromQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/ws?topic=variables&source_type=mqtt&gateway_id=2&project_id=3&var_id=10,11", nil)
	ctx.Request = req

	sub := subscriptionFromQuery(ctx)
	if !sub.Wants("realtime.variables") || sub.Wants("detection.runs") || sub.SourceType != models.TagSourceMQTT || sub.GatewayID == nil || *sub.GatewayID != 2 || sub.ProjectID == nil || *sub.ProjectID != 3 || !sub.VarIDs[10] || !sub.VarIDs[11] {
		t.Fatalf("unexpected subscription: %+v", sub)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/ws?topic=bad", nil)
	sub = subscriptionFromQuery(ctx)
	if !sub.Wants("realtime.variables") || !sub.Wants("detection.runs") || !sub.Wants("notifications") {
		t.Fatalf("expected default subscription fallback: %+v", sub)
	}
}

func readHandlerWSMessageOfType(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			continue
		}
		if msg["type"] == want {
			return msg
		}
	}
	t.Fatalf("websocket message type %s not received", want)
	return nil
}

func mustJSONForHandlerTest(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func intPtrForHandlerTest(value int) *int {
	return &value
}

type partialResultKIOBroker struct {
	waitErr error
}

func (b *partialResultKIOBroker) Config(gatewayID int) (models.GatewayConfig, bool) {
	if gatewayID != 1 {
		return models.GatewayConfig{}, false
	}
	return models.GatewayConfig{
		ID:               1,
		QOS:              1,
		KIOClientID:      "PID",
		KIOWriter:        "writer",
		SetDataTopic:     "setdata_PID",
		WriteResultTopic: "setdata_result_PID",
	}, true
}

func (b *partialResultKIOBroker) Publish(context.Context, int, string, []byte, byte, bool) error {
	return nil
}

func (b *partialResultKIOBroker) Subscribe(context.Context, int, string, byte) error {
	return nil
}

func (b *partialResultKIOBroker) PublishAndWaitKIOAck(context.Context, int, string, []byte, byte, bool, int64) (*kio.WriteAck, bool, error) {
	if b.waitErr != nil {
		return nil, true, b.waitErr
	}
	return &kio.WriteAck{QID: 1, ProcessStep: 100, Result: "ok", Success: true}, true, nil
}
