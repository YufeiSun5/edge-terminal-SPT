package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
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
		Payload:   []byte(`{"project_id":` + itoaHandler(uint64(Project.ID)) + `,"test_no":"WS-H-1","mode":"standard"}`),
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
		Payload:   []byte(`{"project_id":` + itoaHandler(uint64(Project.ID)) + `,"test_no":"WS-H-2","mode":"standard"}`),
	}, auth.Principal{AuthType: "user", UserID: 2, Role: auth.RoleGuest})
	if len(responses) != 1 || responses[0].Error == nil || responses[0].Error.Code != "forbidden" {
		t.Fatalf("unexpected forbidden response: %+v", responses)
	}
}

func TestRealtimeWSHandlerWriteVirtualVariableCommand(t *testing.T) {
	db := newHandlerTestDB(t)
	repo := database.NewRepository(db)
	projectID := uint(9)
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
		ProjectCode: "AC-WS",
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
	tag, ok := tags.Get(varID)
	if !ok || tag.RuntimeState().StrValue != `{"command":"start"}` {
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
	if auditCount != 1 {
		t.Fatalf("expected one ws variable write audit row, got %d", auditCount)
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

func intPtrForHandlerTest(value int) *int {
	return &value
}
