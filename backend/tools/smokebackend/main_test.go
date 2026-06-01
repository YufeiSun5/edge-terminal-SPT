//go:build smoke_tools

package main

import "testing"

func TestEnsureNoNewRuntimeDropsAllowsStableCounters(t *testing.T) {
	before := runtimeDropSnapshot{
		Channels:             map[string]runtimeChannelSnapshot{"logic": {Dropped: 2}, "store": {Dropped: 0}},
		TaskFlowDropped:      4,
		TaskFlowQueue:        runtimeQueueSnapshot{Len: 0, Cap: 1000, Pressure: false},
		NotificationDropped:  3,
		NotificationPressure: false,
		Workers:              map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 0, Health: "ok"}},
	}
	after := runtimeDropSnapshot{
		Channels:             map[string]runtimeChannelSnapshot{"logic": {Dropped: 2}, "store": {Dropped: 0}},
		TaskFlowDropped:      4,
		TaskFlowQueue:        runtimeQueueSnapshot{Len: 1, Cap: 1000, Pressure: false},
		NotificationDropped:  3,
		NotificationPressure: false,
		Workers:              map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 0, Health: "ok"}},
	}
	if err := ensureNoNewRuntimeDrops(before, after); err != nil {
		t.Fatalf("expected stable counters to pass: %v", err)
	}
}

func TestEnsureNoNewRuntimeDropsRejectsChannelGrowth(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}}
	after := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 1}}}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected channel drop growth to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsChannelPressure(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}}
	after := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0, Len: 900, Cap: 1000, Pressure: true}}}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected channel pressure to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsTaskFlowGrowth(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}, TaskFlowDropped: 1}
	after := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}, TaskFlowDropped: 2}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected task-flow drop growth to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsTaskFlowPressure(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}}
	after := runtimeDropSnapshot{
		Channels:      map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		TaskFlowQueue: runtimeQueueSnapshot{Len: 900, Cap: 1000, Pressure: true},
	}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected task-flow pressure to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsNotificationGrowth(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}, NotificationDropped: 1}
	after := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}, NotificationDropped: 2}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected notification drop growth to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsNotificationPressure(t *testing.T) {
	before := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}}
	after := runtimeDropSnapshot{Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}}, NotificationPressure: true}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected notification pressure to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsWorkerPanicGrowth(t *testing.T) {
	before := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 0, Health: "ok"}},
	}
	after := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 1, Health: "degraded"}},
	}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected worker panic growth to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsUnhealthyWorker(t *testing.T) {
	before := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 1, Health: "degraded"}},
	}
	after := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 1, Health: "degraded"}},
	}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected unhealthy worker to fail")
	}
}

func TestEnsureNoNewRuntimeDropsRejectsInactiveCoreWorker(t *testing.T) {
	before := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 1, Panics: 0, Health: "ok"}},
	}
	after := runtimeDropSnapshot{
		Channels: map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}},
		Workers:  map[string]runtimeWorkerSnapshot{"logic": {Active: 0, Panics: 0, Health: "ok"}},
	}
	if err := ensureNoNewRuntimeDrops(before, after); err == nil {
		t.Fatal("expected inactive core worker to fail")
	}
}

func TestRequireExpectedNamesRejectsMissingCoreItem(t *testing.T) {
	if err := requireExpectedNames("runtime/workers", map[string]runtimeWorkerSnapshot{"logic": {Health: "ok"}}, []string{"logic", "storage-bus"}); err == nil {
		t.Fatal("expected missing core worker to fail")
	}
}

func TestRequireExpectedNamesAllowsAllCoreItems(t *testing.T) {
	values := map[string]runtimeChannelSnapshot{"logic": {Dropped: 0}, "store": {Dropped: 0}}
	if err := requireExpectedNames("runtime/channels/detail", values, []string{"logic", "store"}); err != nil {
		t.Fatalf("expected complete core items to pass: %v", err)
	}
}

func TestValidateRuntimeChannelItemsRequiresDroppedCounter(t *testing.T) {
	dropped := uint64(0)
	items := validRuntimeChannelItems(&dropped)
	items[0].Dropped = nil
	if _, err := validateRuntimeChannelItems(items); err == nil {
		t.Fatal("expected missing dropped counter to fail")
	}
}

func TestValidateRuntimeChannelItemsRequiresPressureDiagnostics(t *testing.T) {
	dropped := uint64(0)
	items := validRuntimeChannelItems(&dropped)
	items[0].Pressure = nil
	if _, err := validateRuntimeChannelItems(items); err == nil {
		t.Fatal("expected missing pressure diagnostics to fail")
	}

	items = validRuntimeChannelItems(&dropped)
	pressure := true
	items[0].Pressure = &pressure
	if _, err := validateRuntimeChannelItems(items); err == nil {
		t.Fatal("expected pressure state to fail")
	}
}

func TestValidateRuntimeChannelItemsRequiresCoreChannels(t *testing.T) {
	dropped := uint64(0)
	items := validRuntimeChannelItems(&dropped)
	items = items[:len(items)-1]
	if _, err := validateRuntimeChannelItems(items); err == nil {
		t.Fatal("expected missing core channel to fail")
	}
}

func TestValidateRuntimeChannelItemsRejectsDuplicateNames(t *testing.T) {
	dropped := uint64(0)
	items := validRuntimeChannelItems(&dropped)
	items[1].Name = items[0].Name
	if _, err := validateRuntimeChannelItems(items); err == nil {
		t.Fatal("expected duplicate channel name to fail")
	}
}

func TestValidateRuntimeChannelItemsAllowsCompleteCoreChannels(t *testing.T) {
	dropped := uint64(0)
	got, err := validateRuntimeChannelItems(validRuntimeChannelItems(&dropped))
	if err != nil {
		t.Fatalf("expected complete core channels to pass: %v", err)
	}
	if len(got) != len(expectedCoreChannels) {
		t.Fatalf("channel count=%d want=%d", len(got), len(expectedCoreChannels))
	}
}

func TestValidateRuntimeQueueItemRejectsMissingAndPressure(t *testing.T) {
	length := 1
	capacity := 10
	pressure := false
	if _, err := validateRuntimeQueueItem("task-flow", runtimeQueueItem{Name: "task_flow", Len: &length, Cap: &capacity, Pressure: nil}, "task_flow"); err == nil {
		t.Fatal("expected missing queue pressure field to fail")
	}
	pressure = true
	if _, err := validateRuntimeQueueItem("task-flow", runtimeQueueItem{Name: "task_flow", Len: &length, Cap: &capacity, Pressure: &pressure}, "task_flow"); err == nil {
		t.Fatal("expected pressured queue to fail")
	}
	pressure = false
	if _, err := validateRuntimeQueueItem("task-flow", runtimeQueueItem{Name: "wrong", Len: &length, Cap: &capacity, Pressure: &pressure}, "task_flow"); err == nil {
		t.Fatal("expected queue name mismatch to fail")
	}
}

func TestRequireDroppedCounterRejectsMissingCounter(t *testing.T) {
	if _, err := requireDroppedCounter("runtime/notifications", nil); err == nil {
		t.Fatal("expected missing dropped counter to fail")
	}
}

func TestRequireDroppedCounterAllowsZero(t *testing.T) {
	dropped := uint64(0)
	got, err := requireDroppedCounter("task-flows/runtime", &dropped)
	if err != nil {
		t.Fatalf("expected zero dropped counter to pass: %v", err)
	}
	if got != 0 {
		t.Fatalf("dropped=%d want=0", got)
	}
}

func TestValidateRuntimeWorkerItemsRequiresLifecycleDiagnostics(t *testing.T) {
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	panics := uint64(0)
	items := validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[0].Starts = nil
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected missing starts counter to fail")
	}

	items = validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[0].Exits = nil
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected missing exits counter to fail")
	}

	items = validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[0].Active = nil
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected missing active counter to fail")
	}

	items = validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[0].Panics = nil
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected missing panics counter to fail")
	}
}

func TestValidateRuntimeWorkerItemsRejectsDuplicateNames(t *testing.T) {
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	panics := uint64(0)
	items := validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[1].Name = items[0].Name
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected duplicate worker name to fail")
	}
}

func TestValidateRuntimeWorkerItemsRejectsUnhealthySnapshot(t *testing.T) {
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	panics := uint64(1)
	items := validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	items[0].Health = "degraded"
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected unhealthy worker snapshot to fail")
	}
}

func TestValidateRuntimeWorkerItemsRejectsInactiveCoreAtSnapshot(t *testing.T) {
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	panics := uint64(0)
	items := validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	inactive := 0
	items[0].Active = &inactive
	if _, err := validateRuntimeWorkerItems(items); err == nil {
		t.Fatal("expected inactive core worker snapshot to fail")
	}
}

func TestValidateRuntimeWorkerItemsAllowsCompleteCoreWorkers(t *testing.T) {
	active := 1
	starts := uint64(1)
	exits := uint64(0)
	panics := uint64(0)
	items := validRuntimeWorkerItems(&active, &starts, &exits, &panics)
	got, err := validateRuntimeWorkerItems(items)
	if err != nil {
		t.Fatalf("expected complete workers to pass: %v", err)
	}
	if len(got) != len(expectedCoreWorkers) {
		t.Fatalf("worker count=%d want=%d", len(got), len(expectedCoreWorkers))
	}
}

func validRuntimeWorkerItems(active *int, starts, exits, panics *uint64) []runtimeWorkerItem {
	items := make([]runtimeWorkerItem, 0, len(expectedCoreWorkers))
	for _, name := range expectedCoreWorkers {
		items = append(items, runtimeWorkerItem{
			Name:   name,
			Active: active,
			Starts: starts,
			Exits:  exits,
			Panics: panics,
			Health: "ok",
		})
	}
	return items
}

func validRuntimeChannelItems(dropped *uint64) []runtimeChannelItem {
	items := make([]runtimeChannelItem, 0, len(expectedCoreChannels))
	length := 0
	capacity := 100
	pressure := false
	for _, name := range expectedCoreChannels {
		items = append(items, runtimeChannelItem{
			Name:     name,
			Len:      &length,
			Cap:      &capacity,
			Dropped:  dropped,
			Pressure: &pressure,
		})
	}
	return items
}
