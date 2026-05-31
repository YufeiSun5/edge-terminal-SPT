package services

import (
	"testing"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

func TestRealtimeWSServiceMessagesAndFilters(t *testing.T) {
	tags := pipeline.NewTagManager()
	ProjectID := uint(2)
	tags.Load([]models.TagConfig{
		{VarID: 1, GatewayID: 1, SourceType: models.TagSourceMQTT, SourcePath: "temp", RawName: "temp", ProjectID: &ProjectID, ProjectCode: "AC-1", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", ScaleFactor: 1, Enabled: true},
		{VarID: 2, GatewayID: 2, SourceType: models.TagSourceVirtual, SourcePath: "status", RawName: "status", VarName: "status", JSONPath: "status", DataType: "INT", ScaleFactor: 1, Enabled: true},
	})
	tasks := pipeline.NewTaskManager()
	tasks.SetActive(models.DetectionTask{ID: 3, TestNo: "T-3", ProjectID: ProjectID, ProjectCode: "AC-1", Mode: "standard"})
	service := NewRealtimeWSService(tags, tasks)
	fixedNow := time.Date(2026, 5, 29, 15, 45, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	sub := DefaultRealtimeSubscription()
	sub.SourceType = models.TagSourceMQTT
	sub.GatewayID = intPtrForServiceTest(1)
	sub.ProjectID = &ProjectID
	sub.VarIDs = map[int64]bool{1: true}

	if !sub.Wants("realtime.variables") || !sub.Wants("detection.runs") || sub.Wants("bad") {
		t.Fatalf("unexpected subscription topics: %+v", sub)
	}
	if got := service.FilteredSnapshots(sub); len(got) != 1 || got[0].VarID != 1 {
		t.Fatalf("unexpected filtered snapshots: %+v", got)
	}
	if got := service.FilteredActiveTasks(sub); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("unexpected active tasks: %+v", got)
	}
	if msg := service.ReadyMessage("req-1", sub); msg.Type != WSTypeConnectionReady || msg.RequestID != "req-1" || msg.At != fixedNow {
		t.Fatalf("unexpected ready message: %+v", msg)
	}
	if msg := service.SubscriptionMessage("req-2", sub); msg.Type != WSTypeSubscriptionUpdated {
		t.Fatalf("unexpected subscription message: %+v", msg)
	}
	if msg := service.VariableSnapshotMessage(sub); msg.Type != WSTypeRealtimeVariablesSnapshot {
		t.Fatalf("unexpected variable message: %+v", msg)
	}
	if msg := service.DetectionRunsMessage(sub); msg.Type != WSTypeDetectionRunsSnapshot {
		t.Fatalf("unexpected run message: %+v", msg)
	}
	notification := &models.RuntimeNotification{Type: models.NotificationDetectionResultOK, ProjectID: ProjectID, TaskID: 3}
	if !service.NotificationMatches(sub, notification) {
		t.Fatalf("expected notification to match subscription")
	}
	if msg := service.NotificationMessage(notification); msg.Type != WSTypeNotificationEvent {
		t.Fatalf("unexpected notification message: %+v", msg)
	}
	if msg := service.HeartbeatMessage(); msg.Type != WSTypeHeartbeat {
		t.Fatalf("unexpected heartbeat: %+v", msg)
	}
	if msg := service.ErrorMessage("req-3", "cmd-3", "read_only", "no writes"); msg.Type != WSTypeError || msg.Error.Code != "read_only" {
		t.Fatalf("unexpected error: %+v", msg)
	}
	payload := sub.ResponsePayload()
	if len(payload["topics"].([]string)) != 3 || len(payload["var_ids"].([]int64)) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if texts := payload["var_id_texts"].([]string); len(texts) != 1 || texts[0] != "1" {
		t.Fatalf("unexpected exact var id payload: %+v", payload)
	}
	if NormalizeWSTopic("variables") != "realtime.variables" || NormalizeWSTopic("runs") != "detection.runs" || NormalizeWSTopic("notifications") != "notifications" || NormalizeWSTopic("bad") != "" {
		t.Fatal("unexpected topic normalization")
	}
}

func intPtrForServiceTest(value int) *int {
	return &value
}
