package pipeline

import (
	"sort"
	"sync"
	"time"

	"spindle-edge/backend/internal/models"
)

type TaskFlowEvent struct {
	TriggerType    string
	ProjectID      uint
	TriggerVarID   int64
	TriggerValue   any
	GatewayID      int
	Topic          string
	At             time.Time
	OriginFlowID   uint64
	OriginRunID    uint64
	Depth          int
	MaxDepth       int
	AllowReentrant bool
	RequestID      string
}

type TaskFlowIndex struct {
	mu            sync.Mutex
	byVar         map[int64][]models.TaskFlow
	byTrigger     map[string][]models.TaskFlow
	lastTriggered map[uint64]time.Time
	holdSince     map[uint64]time.Time
}

func NewTaskFlowIndex(flows []models.TaskFlow) *TaskFlowIndex {
	idx := &TaskFlowIndex{
		byVar:         make(map[int64][]models.TaskFlow),
		byTrigger:     make(map[string][]models.TaskFlow),
		lastTriggered: make(map[uint64]time.Time),
		holdSince:     make(map[uint64]time.Time),
	}
	idx.Load(flows)
	return idx
}

func (idx *TaskFlowIndex) Load(flows []models.TaskFlow) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.byVar = make(map[int64][]models.TaskFlow)
	idx.byTrigger = make(map[string][]models.TaskFlow)
	for _, flow := range flows {
		if !flow.Enabled {
			continue
		}
		idx.byTrigger[flow.TriggerType] = append(idx.byTrigger[flow.TriggerType], flow)
		if flow.TriggerType != models.TaskFlowTriggerDataChange {
			continue
		}
		for _, item := range flow.Vars {
			if item.Role == "" || item.Role == models.TaskFlowVarRoleWatch {
				idx.byVar[item.VarID] = append(idx.byVar[item.VarID], flow)
			}
		}
	}
	sortTaskFlows(idx.byTrigger)
	sortTaskFlows(idx.byVar)
}

func sortTaskFlows[T comparable](items map[T][]models.TaskFlow) {
	for key := range items {
		sort.SliceStable(items[key], func(i, j int) bool {
			if items[key][i].Priority == items[key][j].Priority {
				return items[key][i].ID < items[key][j].ID
			}
			return items[key][i].Priority > items[key][j].Priority
		})
	}
}

func (idx *TaskFlowIndex) Match(event TaskFlowEvent) []models.TaskFlow {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	var candidates []models.TaskFlow
	if event.TriggerType == models.TaskFlowTriggerDataChange {
		candidates = idx.byVar[event.TriggerVarID]
	} else {
		candidates = idx.byTrigger[event.TriggerType]
	}
	if len(candidates) == 0 {
		return nil
	}
	now := event.At
	if now.IsZero() {
		now = time.Now()
	}
	matches := make([]models.TaskFlow, 0, len(candidates))
	for _, flow := range candidates {
		if flow.ProjectID != 0 && event.ProjectID != 0 && flow.ProjectID != event.ProjectID {
			continue
		}
		if event.MaxDepth > 0 && event.Depth > event.MaxDepth {
			continue
		}
		if !event.AllowReentrant && event.OriginFlowID != 0 && flow.ID == event.OriginFlowID {
			continue
		}
		if event.TriggerType == models.TaskFlowTriggerSchedule {
			intervalMS := flow.ScheduleIntervalMS
			if intervalMS <= 0 {
				intervalMS = flow.CooldownMS
			}
			if intervalMS <= 0 {
				continue
			}
			if last := idx.lastTriggered[flow.ID]; !last.IsZero() && now.Sub(last) < time.Duration(intervalMS)*time.Millisecond {
				continue
			}
		}
		if flow.CooldownMS > 0 {
			if last := idx.lastTriggered[flow.ID]; !last.IsZero() && now.Sub(last) < time.Duration(flow.CooldownMS)*time.Millisecond {
				continue
			}
		}
		if flow.HoldMS > 0 {
			since := idx.holdSince[flow.ID]
			if since.IsZero() {
				idx.holdSince[flow.ID] = now
				continue
			}
			if now.Sub(since) < time.Duration(flow.HoldMS)*time.Millisecond {
				continue
			}
		}
		idx.lastTriggered[flow.ID] = now
		matches = append(matches, flow)
	}
	return matches
}
