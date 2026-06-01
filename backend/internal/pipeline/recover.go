package pipeline

import (
	"fmt"
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorkerRecoveryStat struct {
	Name          string     `json:"name"`
	Active        int        `json:"active"`
	Starts        uint64     `json:"starts"`
	Exits         uint64     `json:"exits"`
	Panics        uint64     `json:"panics"`
	Health        string     `json:"health"`
	Impact        string     `json:"impact"`
	NextAction    string     `json:"next_action"`
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
	LastExitedAt  *time.Time `json:"last_exited_at,omitempty"`
	LastPanicAt   *time.Time `json:"last_panic_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type workerRecoveryRecord struct {
	name          string
	active        int
	starts        uint64
	exits         uint64
	panics        uint64
	lastStartedAt *time.Time
	lastExitedAt  *time.Time
	lastPanicAt   *time.Time
	lastError     string
}

var workerRecoveryRegistry = struct {
	sync.Mutex
	records map[string]*workerRecoveryRecord
}{records: make(map[string]*workerRecoveryRecord)}

func resetWorkerRecoveryStatsForTest() {
	workerRecoveryRegistry.Lock()
	defer workerRecoveryRegistry.Unlock()
	workerRecoveryRegistry.records = make(map[string]*workerRecoveryRecord)
}

func RunRecovering(name string, fn func()) {
	startedAt := time.Now()
	recordWorkerStart(name, startedAt)
	defer func() {
		exitedAt := time.Now()
		if recovered := recover(); recovered != nil {
			recordWorkerPanic(name, exitedAt, recovered)
			log.Printf("[worker] panic recovered name=%s err=%v stack=%s", name, recovered, string(debug.Stack()))
		}
		recordWorkerExit(name, exitedAt)
	}()
	fn()
}

func GoRecovering(name string, fn func()) {
	go RunRecovering(name, fn)
}

func WorkerRecoveryStats() []WorkerRecoveryStat {
	workerRecoveryRegistry.Lock()
	defer workerRecoveryRegistry.Unlock()
	stats := make([]WorkerRecoveryStat, 0, len(workerRecoveryRegistry.records))
	for _, record := range workerRecoveryRegistry.records {
		stat := WorkerRecoveryStat{
			Name:          record.name,
			Active:        record.active,
			Starts:        record.starts,
			Exits:         record.exits,
			Panics:        record.panics,
			LastStartedAt: cloneTime(record.lastStartedAt),
			LastExitedAt:  cloneTime(record.lastExitedAt),
			LastPanicAt:   cloneTime(record.lastPanicAt),
			LastError:     record.lastError,
		}
		stat.Health = workerHealth(stat)
		stat.Impact = workerImpact(stat.Name)
		stat.NextAction = workerNextAction(stat)
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})
	return stats
}

func recordWorkerStart(name string, at time.Time) {
	workerRecoveryRegistry.Lock()
	defer workerRecoveryRegistry.Unlock()
	record := workerRecoveryRecordFor(name)
	record.active++
	record.starts++
	record.lastStartedAt = &at
}

func recordWorkerExit(name string, at time.Time) {
	workerRecoveryRegistry.Lock()
	defer workerRecoveryRegistry.Unlock()
	record := workerRecoveryRecordFor(name)
	if record.active > 0 {
		record.active--
	}
	record.exits++
	record.lastExitedAt = &at
}

func recordWorkerPanic(name string, at time.Time, recovered any) {
	workerRecoveryRegistry.Lock()
	defer workerRecoveryRegistry.Unlock()
	record := workerRecoveryRecordFor(name)
	record.panics++
	record.lastPanicAt = &at
	record.lastError = truncateWorkerError(recovered)
}

func workerRecoveryRecordFor(name string) *workerRecoveryRecord {
	if name == "" {
		name = "unnamed"
	}
	record := workerRecoveryRegistry.records[name]
	if record == nil {
		record = &workerRecoveryRecord{name: name}
		workerRecoveryRegistry.records[name] = record
	}
	return record
}

func workerHealth(stat WorkerRecoveryStat) string {
	switch {
	case stat.Panics == 0:
		return "ok"
	case stat.Active == 0:
		return "stopped_after_panic"
	default:
		return "degraded"
	}
}

func workerImpact(name string) string {
	switch name {
	case "logic":
		return "Realtime cleaning, project maps, task triggers, alarms, storage dispatch, and WebSocket snapshots can stop or lag."
	case "cycle-scanner":
		return "Cycle-based storage and alarm checks can stop running."
	case "discovery":
		return "New variable discovery can stop; existing known-variable realtime handling can continue."
	case "storage-bus", "storage-bucket":
		return "History writes and project wide-table persistence can stop or lag."
	case "alarm-worker":
		return "Limit alarm enter, level_change, recover records, and alarm notifications can stop or lag."
	case "notification-dispatcher":
		return "Persisted notifications, unread counts, and notification fan-out can stop."
	case "notification-hub":
		return "Online WebSocket notification fan-out can stop; HTTP notification history may still be available if dispatch persists."
	case "task-flow-worker":
		return "Variable-triggered business flows, built-in modules, scripts, and lifecycle actions can stop or lag."
	case "task-flow-schedule-scanner":
		return "Schedule task flows can stop triggering."
	case "task-flow-fixed-duration-guard":
		return "Fixed-duration detection runs can fail to auto-stop."
	case "task-flow-qualified-hold-guard":
		return "Qualified-hold detection runs can fail to auto-stop after the hold condition."
	case "channel-pressure-logger":
		return "Queue pressure logs can stop, but runtime diagnostic APIs remain available."
	default:
		return "The named background worker can stop or degrade."
	}
}

func workerNextAction(stat WorkerRecoveryStat) string {
	switch stat.Health {
	case "ok":
		return "No action needed."
	case "stopped_after_panic":
		return "Check the latest worker panic log and restart the backend process if this worker is required for the current operation."
	default:
		return "Check the latest worker panic log, queue pressure diagnostics, and downstream dependency latency."
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func truncateWorkerError(recovered any) string {
	text := ""
	if recovered != nil {
		text = logPrefixValue(recovered)
	}
	const max = 500
	if len(text) > max {
		return text[:max]
	}
	return text
}

func logPrefixValue(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}
