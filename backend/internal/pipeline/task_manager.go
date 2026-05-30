package pipeline

import (
	"math"
	"sync"
	"time"

	"spindle-edge/backend/internal/models"
)

type TaskManager struct {
	mu          sync.RWMutex
	byProject   map[uint]models.ActiveTask
	alarmStates map[uint]map[int64]*limitAlarmState
	ruleIndex   TaskRuleIndex
}

type limitAlarmState struct {
	Active              bool
	AlarmType           string
	AlarmLevel          string
	LimitValue          float64
	StartedAt           time.Time
	LastCheckAt         time.Time
	PeakValue           float64
	Muted               bool
	PendingAlarmType    string
	PendingAlarmLevel   string
	PendingLimitValue   float64
	PendingSince        time.Time
	RecoverPendingSince time.Time
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		byProject:   make(map[uint]models.ActiveTask),
		alarmStates: make(map[uint]map[int64]*limitAlarmState),
		ruleIndex:   NewTaskRuleIndex(nil),
	}
}

func (tm *TaskManager) Load(tasks []models.DetectionTask) {
	next := make(map[uint]models.ActiveTask, len(tasks))
	for _, task := range tasks {
		next[task.ProjectID] = activeTaskFromDetection(task)
	}

	tm.mu.Lock()
	tm.byProject = next
	tm.alarmStates = make(map[uint]map[int64]*limitAlarmState)
	tm.mu.Unlock()
}

func (tm *TaskManager) LoadTaskRules(rules []models.TaskRule) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.ruleIndex = NewTaskRuleIndex(rules)
}

func (tm *TaskManager) SetActive(task models.DetectionTask) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.byProject[task.ProjectID] = activeTaskFromDetection(task)
	tm.alarmStates[task.ID] = make(map[int64]*limitAlarmState)
}

func (tm *TaskManager) UpdateActive(task models.DetectionTask) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.byProject[task.ProjectID] = activeTaskFromDetection(task)
	if tm.alarmStates[task.ID] == nil {
		tm.alarmStates[task.ID] = make(map[int64]*limitAlarmState)
	}
}

func (tm *TaskManager) Clear(ProjectID uint) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if task, ok := tm.byProject[ProjectID]; ok {
		delete(tm.alarmStates, task.ID)
	}
	delete(tm.byProject, ProjectID)
}

func activeTaskFromDetection(task models.DetectionTask) models.ActiveTask {
	active := models.ActiveTask{
		ID:              task.ID,
		TestNo:          task.TestNo,
		ProjectID:       task.ProjectID,
		ProjectCode:     task.ProjectCode,
		Mode:            task.Mode,
		StandardID:      task.StandardID,
		StandardCode:    task.StandardCode,
		StandardVersion: task.StandardVer,
	}
	if len(task.StandardItems) > 0 {
		active.StandardItems = make(map[int64]models.DetectionRunStandardItem, len(task.StandardItems))
		for _, item := range task.StandardItems {
			active.StandardItems[item.VarID] = item
		}
	}
	if len(task.StorageRoutes) > 0 {
		active.StorageRoutes = make(map[int64][]models.DetectionRunStorageRoute, len(task.StorageRoutes))
		for _, route := range task.StorageRoutes {
			active.StorageRoutes[route.VarID] = append(active.StorageRoutes[route.VarID], route)
		}
	}
	return active
}

func (tm *TaskManager) ActiveForProject(ProjectID uint) (models.ActiveTask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.byProject[ProjectID]
	return task, ok
}

func (tm *TaskManager) EvaluateTaskRules(varID int64, oldValue float64, newValue float64, changed bool, first bool) []TaskRuleMatch {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.ruleIndex.Evaluate(varID, oldValue, newValue, changed, first)
}

func (tm *TaskManager) AllActive() []models.ActiveTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]models.ActiveTask, 0, len(tm.byProject))
	for _, task := range tm.byProject {
		tasks = append(tasks, task)
	}
	return tasks
}

func (tm *TaskManager) MuteActiveLimitAlarms(taskID uint) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	byVar := tm.alarmStates[taskID]
	count := 0
	for _, state := range byVar {
		if state.Active && !state.Muted {
			state.Muted = true
			count++
		}
	}
	return count
}

func (tm *TaskManager) EvaluateLimitAlarm(tag *models.Tag, at time.Time, onStart bool) []*models.DetectionLimitAlarmEvent {
	if tag.Config.ProjectID == nil {
		return nil
	}
	state := tag.RuntimeState()
	if !state.Initialized || state.IsString {
		return nil
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	active, ok := tm.byProject[*tag.Config.ProjectID]
	if !ok || active.StandardItems == nil {
		return nil
	}
	item, ok := active.StandardItems[tag.Config.VarID]
	if !ok || !item.CheckEnabled || !item.AlarmEnabled || item.CheckMethod != models.CheckMethodNumericRange {
		return nil
	}
	if onStart && !item.CheckOnStart {
		return nil
	}
	if state.Quality != 1 && item.QualityPolicy == models.QualityPolicyIgnoreBad {
		return nil
	}

	byVar := tm.alarmStates[active.ID]
	if byVar == nil {
		byVar = make(map[int64]*limitAlarmState)
		tm.alarmStates[active.ID] = byVar
	}
	alarmState := byVar[tag.Config.VarID]
	if alarmState == nil {
		alarmState = &limitAlarmState{}
		byVar[tag.Config.VarID] = alarmState
	}
	if !onStart && item.CheckCycleMS > 0 && !alarmState.LastCheckAt.IsZero() && at.Sub(alarmState.LastCheckAt) < time.Duration(item.CheckCycleMS)*time.Millisecond {
		return nil
	}
	alarmState.LastCheckAt = at

	alarmType, alarmLevel, limitValue, violated := limitAlarmForValue(state.Value, item)
	if !alarmState.Active {
		if !violated {
			alarmState.PendingAlarmType = ""
			alarmState.PendingSince = time.Time{}
			return nil
		}
		if item.ViolationHoldMS > 0 {
			if alarmState.PendingAlarmType != alarmType || alarmState.PendingLimitValue != limitValue {
				alarmState.PendingAlarmType = alarmType
				alarmState.PendingAlarmLevel = alarmLevel
				alarmState.PendingLimitValue = limitValue
				alarmState.PendingSince = at
				return nil
			}
			if at.Sub(alarmState.PendingSince) < time.Duration(item.ViolationHoldMS)*time.Millisecond {
				return nil
			}
		}
		alarmState.Active = true
		alarmState.AlarmType = alarmType
		alarmState.AlarmLevel = alarmLevel
		alarmState.LimitValue = limitValue
		alarmState.StartedAt = at
		alarmState.PeakValue = state.Value
		alarmState.RecoverPendingSince = time.Time{}
		alarmState.PendingAlarmType = ""
		alarmState.PendingSince = time.Time{}
		value := state.Value
		alarm := models.DetectionLimitAlarm{
			TaskID:         active.ID,
			TestNo:         active.TestNo,
			ProjectID:      active.ProjectID,
			ProjectCode:    active.ProjectCode,
			StandardID:     active.StandardID,
			StandardItemID: item.StandardItemID,
			RunStandardID:  item.ID,
			VarID:          item.VarID,
			VarName:        item.VarName,
			DisplayName:    item.DisplayName,
			DisplayNameEN:  item.DisplayNameEN,
			DisplayNameJA:  item.DisplayNameJA,
			CheckMethod:    item.CheckMethod,
			AlarmType:      alarmType,
			AlarmLevel:     alarmLevel,
			Status:         models.DetectionAlarmStatusActive,
			StartValue:     &value,
			PeakValue:      &value,
			LimitValue:     &limitValue,
			LimitDeadband:  item.LimitDeadband,
			Quality:        state.Quality,
			FirstSeenAt:    at,
			LastSeenAt:     at,
		}
		return []*models.DetectionLimitAlarmEvent{{Action: models.DetectionAlarmActionEnter, Alarm: alarm}}
	}

	if alarmState.isMoreSeverePeak(state.Value) {
		alarmState.PeakValue = state.Value
	}
	if violated && alarmType != alarmState.AlarmType && alarmSeverity(alarmType) > alarmSeverity(alarmState.AlarmType) {
		oldType := alarmState.AlarmType
		alarmState.Active = true
		alarmState.AlarmType = alarmType
		alarmState.AlarmLevel = alarmLevel
		alarmState.LimitValue = limitValue
		alarmState.StartedAt = at
		alarmState.PeakValue = state.Value
		alarmState.Muted = false
		alarmState.RecoverPendingSince = time.Time{}
		value := state.Value
		alarm := models.DetectionLimitAlarm{
			TaskID:         active.ID,
			TestNo:         active.TestNo,
			ProjectID:      active.ProjectID,
			ProjectCode:    active.ProjectCode,
			StandardID:     active.StandardID,
			StandardItemID: item.StandardItemID,
			RunStandardID:  item.ID,
			VarID:          item.VarID,
			VarName:        item.VarName,
			DisplayName:    item.DisplayName,
			DisplayNameEN:  item.DisplayNameEN,
			DisplayNameJA:  item.DisplayNameJA,
			CheckMethod:    item.CheckMethod,
			AlarmType:      alarmType,
			AlarmLevel:     alarmLevel,
			Status:         models.DetectionAlarmStatusActive,
			StartValue:     &value,
			PeakValue:      &value,
			LimitValue:     &limitValue,
			LimitDeadband:  item.LimitDeadband,
			Quality:        state.Quality,
			FirstSeenAt:    at,
			LastSeenAt:     at,
			Message:        "level_change",
		}
		return []*models.DetectionLimitAlarmEvent{{Action: models.DetectionAlarmActionLevelChange, PreviousAlarmType: oldType, Alarm: alarm}}
	}
	if !recoveredFromLimitAlarm(state.Value, alarmState, item.LimitDeadband) {
		alarmState.RecoverPendingSince = time.Time{}
		return nil
	}
	if item.RecoverHoldMS > 0 {
		if alarmState.RecoverPendingSince.IsZero() {
			alarmState.RecoverPendingSince = at
			return nil
		}
		if at.Sub(alarmState.RecoverPendingSince) < time.Duration(item.RecoverHoldMS)*time.Millisecond {
			return nil
		}
	}

	recoverValue := state.Value
	peakValue := alarmState.PeakValue
	recoveredAt := at
	durationMS := at.Sub(alarmState.StartedAt).Milliseconds()
	alarm := models.DetectionLimitAlarm{
		TaskID:       active.ID,
		VarID:        tag.Config.VarID,
		AlarmType:    alarmState.AlarmType,
		Status:       models.DetectionAlarmStatusClosed,
		PeakValue:    &peakValue,
		RecoverValue: &recoverValue,
		Quality:      state.Quality,
		LastSeenAt:   at,
		RecoveredAt:  &recoveredAt,
		DurationMS:   durationMS,
	}
	alarmState.Active = false
	alarmState.AlarmType = ""
	alarmState.AlarmLevel = ""
	alarmState.LimitValue = 0
	alarmState.StartedAt = time.Time{}
	alarmState.PeakValue = 0
	alarmState.Muted = false
	alarmState.RecoverPendingSince = time.Time{}
	return []*models.DetectionLimitAlarmEvent{{Action: models.DetectionAlarmActionRecover, Alarm: alarm}}
}

func (s *limitAlarmState) isMoreSeverePeak(value float64) bool {
	switch s.AlarmType {
	case "above_h", "above_hh":
		return value > s.PeakValue
	case "below_l", "below_ll":
		return value < s.PeakValue
	default:
		return math.Abs(value-s.LimitValue) > math.Abs(s.PeakValue-s.LimitValue)
	}
}

func limitAlarmForValue(value float64, item models.DetectionRunStandardItem) (string, string, float64, bool) {
	if item.LimitHH != nil && value > *item.LimitHH {
		return "above_hh", "HH", *item.LimitHH, true
	}
	if item.LimitH != nil && value > *item.LimitH {
		return "above_h", "H", *item.LimitH, true
	}
	if item.LimitLL != nil && value < *item.LimitLL {
		return "below_ll", "LL", *item.LimitLL, true
	}
	if item.LimitL != nil && value < *item.LimitL {
		return "below_l", "L", *item.LimitL, true
	}
	return "", "", 0, false
}

func alarmSeverity(alarmType string) int {
	switch alarmType {
	case "above_hh", "below_ll":
		return 2
	case "above_h", "below_l":
		return 1
	default:
		return 0
	}
}

func recoveredFromLimitAlarm(value float64, state *limitAlarmState, deadband float64) bool {
	switch state.AlarmType {
	case "above_h", "above_hh":
		return value <= state.LimitValue-deadband
	case "below_l", "below_ll":
		return value >= state.LimitValue+deadband
	default:
		return true
	}
}
