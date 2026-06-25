package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

func TestRuntimeDraftServiceSaveReadDelete(t *testing.T) {
	service := NewRuntimeDraftService(nil)

	draft, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(0),
		TTLSec:           int(time.Hour / time.Second),
		Data: map[string]any{
			"standard_id": 123,
			"items":       []any{map[string]any{"var_id": 1, "var_name": "T1"}},
		},
	})
	if err != nil {
		t.Fatalf("put draft: %v", err)
	}
	if draft.Revision != 1 {
		t.Fatalf("revision = %d, want 1", draft.Revision)
	}

	got, err := service.Get(RuntimeDraftNamespaceStationDetectionPreload, RuntimeDraftScopeProject, "1")
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if got.Revision != draft.Revision || string(got.Data) != string(draft.Data) {
		t.Fatalf("got %#v, want %#v", got, draft)
	}

	if err := service.Clear(RuntimeDraftClearInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(draft.Revision),
	}); err != nil {
		t.Fatalf("clear draft: %v", err)
	}
	if _, err := service.Get(RuntimeDraftNamespaceStationDetectionPreload, RuntimeDraftScopeProject, "1"); !errors.Is(err, ErrRuntimeDraftNotFound) {
		t.Fatalf("get after clear error = %v, want not_found", err)
	}
}

func TestRuntimeDraftServiceRevisionConflict(t *testing.T) {
	service := NewRuntimeDraftService(nil)
	if _, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(0),
		Data:             map[string]any{"ok": true},
	}); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(0),
		Data:             map[string]any{"ok": false},
	}); !errors.Is(err, ErrRuntimeDraftRevisionConflict) {
		t.Fatalf("second put error = %v, want revision conflict", err)
	}
}

func TestRuntimeDraftServiceTooLargeAndExpired(t *testing.T) {
	service := NewRuntimeDraftService(nil)
	if _, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(0),
		Data:             strings.Repeat("x", 512*1024+1),
	}); !errors.Is(err, ErrRuntimeDraftTooLarge) {
		t.Fatalf("large put error = %v, want too large", err)
	}

	if _, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "2",
		ExpectedRevision: int64Ptr(0),
		TTLSec:           1,
		Data:             map[string]any{"ok": true},
	}); err != nil {
		t.Fatalf("create expiring draft: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := service.Get(RuntimeDraftNamespaceStationDetectionPreload, RuntimeDraftScopeProject, "2"); !errors.Is(err, ErrRuntimeDraftNotFound) {
		t.Fatalf("expired get error = %v, want not_found", err)
	}
}

func TestRuntimeDraftServiceRejectsProjectRunning(t *testing.T) {
	tasks := pipeline.NewTaskManager()
	tasks.SetActive(models.DetectionTask{ID: 7, ProjectID: 1})
	service := NewRuntimeDraftService(tasks)

	if _, err := service.Put(RuntimeDraftPutInput{
		Namespace:        RuntimeDraftNamespaceStationDetectionPreload,
		ScopeType:        RuntimeDraftScopeProject,
		ScopeID:          "1",
		ExpectedRevision: int64Ptr(0),
		Data:             map[string]any{"ok": true},
	}); !errors.Is(err, ErrRuntimeDraftProjectRunning) {
		t.Fatalf("running put error = %v, want project_running", err)
	}
}

func TestDetectionRuntimeDraftItemsPreferVarIDText(t *testing.T) {
	items, err := detectionRuntimeDraftItems([]map[string]any{
		{
			"var_id":      float64(7943629295557820000),
			"var_id_text": "7943629295557820374",
			"var_name":    "reheater_outlet_temperature",
		},
	})
	if err != nil {
		t.Fatalf("convert items: %v", err)
	}
	if got, want := items[0].VarID, int64(7943629295557820374); got != want {
		t.Fatalf("VarID = %d, want %d", got, want)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
