package models

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

type MQTTMessage struct {
	GatewayID int
	Topic     string
	Payload   []byte
	Timestamp time.Time
}

type StoreTask struct {
	GatewayID      int
	Topic          string
	ProjectID      uint
	TaskID         uint
	TestNo         string
	VarID          int64
	VarName        string
	ProjectCode    string
	Value          float64
	StrValue       string
	IsString       bool
	Quality        int
	Timestamp      time.Time
	StorageRoutes  []DetectionRunStorageRoute
	SkipHistoryRow bool
}

type TagSnapshot struct {
	VarID         int64     `json:"var_id"`
	GatewayID     int       `json:"gateway_id"`
	SourceTopic   string    `json:"source_topic"`
	SourcePath    string    `json:"source_path"`
	SourceType    string    `json:"source_type"`
	ProjectID     *uint     `json:"project_id,omitempty"`
	ProjectCode   string    `json:"project_code"`
	VarGroup      string    `json:"var_group"`
	VarName       string    `json:"var_name"`
	DisplayName   string    `json:"display_name"`
	DisplayNameEN string    `json:"display_name_en"`
	DisplayNameJA string    `json:"display_name_ja"`
	Value         float64   `json:"value"`
	StrValue      string    `json:"str_value"`
	IsString      bool      `json:"is_string"`
	Quality       int       `json:"quality"`
	LastUpdate    time.Time `json:"last_update"`
	RWMode        string    `json:"rw_mode"`
	Writable      bool      `json:"writable"`
}

type TagRuntimeState struct {
	Value       float64
	StrValue    string
	IsString    bool
	Quality     int
	LastUpdate  time.Time
	Initialized bool
}

type Tag struct {
	Config TagConfig

	mu                   sync.RWMutex
	currentValue         float64
	lastValue            float64
	currentStrValue      string
	lastStrValue         string
	quality              int
	lastQuality          int
	lastUpdate           time.Time
	lastChange           time.Time
	lastStore            time.Time
	pendingNumericValue  float64
	pendingNumericSince  time.Time
	pendingNumericActive bool
	routeStores          map[string]routeStoreState
	initialized          bool
}

type routeStoreState struct {
	LastStore    time.Time
	LastValue    float64
	LastStrValue string
	Initialized  bool
}

func NewTag(cfg TagConfig) *Tag {
	if cfg.ScaleFactor == 0 {
		cfg.ScaleFactor = 1
	}
	return &Tag{
		Config:      cfg,
		quality:     1,
		routeStores: make(map[string]routeStoreState),
	}
}

func (t *Tag) UpdateNumeric(value float64, at time.Time, quality int) (oldValue float64, changed bool, first bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	scaled := value*t.Config.ScaleFactor + t.Config.OffsetVal
	if t.isSuspiciousNumeric(scaled) {
		return t.currentValue, false, false
	}

	first = !t.initialized
	oldValue = t.currentValue
	if !first {
		if t.shouldDebounceNumeric(scaled, at) {
			return oldValue, false, false
		}
		if t.inRuntimeDeadband(scaled) {
			t.lastQuality = t.quality
			t.quality = quality
			t.lastUpdate = at
			return oldValue, false, false
		}
	}
	t.lastValue = t.currentValue
	t.currentValue = scaled
	t.lastQuality = t.quality
	t.quality = quality
	t.lastUpdate = at
	if first || scaled != oldValue {
		t.lastChange = at
	}
	t.initialized = true
	t.clearPendingNumeric()

	if first {
		return oldValue, false, true
	}
	return oldValue, scaled != oldValue, false
}

func (t *Tag) UpdateString(value string, at time.Time, quality int) (oldValue string, changed bool, first bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	first = !t.initialized
	oldValue = t.currentStrValue
	t.lastStrValue = t.currentStrValue
	t.currentStrValue = value
	t.lastQuality = t.quality
	t.quality = quality
	t.lastUpdate = at
	if first || oldValue != value {
		t.lastChange = at
	}
	t.initialized = true

	if first {
		return oldValue, false, true
	}
	return oldValue, oldValue != value, false
}

func (t *Tag) ShouldStoreByChange(changed bool) bool {
	return changed
}

func (t *Tag) ShouldStoreByCycle(now time.Time) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.initialized
}

func (t *Tag) MarkStored(at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastStore = at
}

func (t *Tag) MarkStorageRoutesStored(routes []DetectionRunStorageRoute, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.routeStores == nil {
		t.routeStores = make(map[string]routeStoreState)
	}
	for _, route := range routes {
		key := routeStoreKey(route)
		state := t.routeStores[key]
		state.LastStore = at
		state.LastValue = t.currentValue
		state.LastStrValue = t.currentStrValue
		state.Initialized = true
		t.routeStores[key] = state
	}
	t.lastStore = at
}

func (t *Tag) isSuspiciousNumeric(value float64) bool {
	if t.Config.SuspiciousValue == nil {
		return false
	}
	return value == *t.Config.SuspiciousValue
}

func (t *Tag) shouldDebounceNumeric(value float64, at time.Time) bool {
	if t.Config.DebounceThreshold == nil || t.Config.DebounceMS <= 0 {
		t.clearPendingNumeric()
		return false
	}
	if math.Abs(value-t.currentValue) <= *t.Config.DebounceThreshold {
		t.clearPendingNumeric()
		return false
	}
	if !t.pendingNumericActive || t.pendingNumericValue != value {
		t.pendingNumericActive = true
		t.pendingNumericValue = value
		t.pendingNumericSince = at
		return true
	}
	return at.Sub(t.pendingNumericSince) < time.Duration(t.Config.DebounceMS)*time.Millisecond
}

func (t *Tag) inRuntimeDeadband(value float64) bool {
	return t.Config.Deadband > 0 && math.Abs(value-t.currentValue) <= t.Config.Deadband
}

func (t *Tag) clearPendingNumeric() {
	t.pendingNumericActive = false
	t.pendingNumericValue = 0
	t.pendingNumericSince = time.Time{}
}

func (t *Tag) Snapshot() TagSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	isString := isStringType(t.Config.DataType)
	return TagSnapshot{
		VarID:         t.Config.VarID,
		GatewayID:     t.Config.GatewayID,
		SourceTopic:   t.Config.SourceTopic,
		SourcePath:    t.Config.SourcePath,
		SourceType:    t.Config.SourceType,
		ProjectID:     t.Config.ProjectID,
		ProjectCode:   t.Config.ProjectCode,
		VarGroup:      t.Config.VarGroup,
		VarName:       t.Config.VarName,
		DisplayName:   t.Config.DisplayName,
		DisplayNameEN: t.Config.DisplayNameEN,
		DisplayNameJA: t.Config.DisplayNameJA,
		Value:         t.currentValue,
		StrValue:      t.currentStrValue,
		IsString:      isString,
		Quality:       t.quality,
		LastUpdate:    t.lastUpdate,
		RWMode:        t.Config.RWMode,
		Writable:      t.Config.Writable,
	}
}

func (t *Tag) RuntimeState() TagRuntimeState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TagRuntimeState{
		Value:       t.currentValue,
		StrValue:    t.currentStrValue,
		IsString:    isStringType(t.Config.DataType),
		Quality:     t.quality,
		LastUpdate:  t.lastUpdate,
		Initialized: t.initialized,
	}
}

func (t *Tag) StoreTask(gatewayID int, topic string, active ActiveTask, at time.Time) *StoreTask {
	t.mu.RLock()
	defer t.mu.RUnlock()

	task := &StoreTask{
		GatewayID:   gatewayID,
		Topic:       topic,
		ProjectID:   active.ProjectID,
		TaskID:      active.ID,
		TestNo:      active.TestNo,
		VarID:       t.Config.VarID,
		VarName:     t.Config.VarName,
		ProjectCode: t.Config.ProjectCode,
		Value:       t.currentValue,
		StrValue:    t.currentStrValue,
		IsString:    isStringType(t.Config.DataType),
		Quality:     t.quality,
		Timestamp:   at,
	}
	if routes := active.RoutesForStore(t.Config.VarID); len(routes) > 0 {
		task.StorageRoutes = append(task.StorageRoutes, routes...)
	}
	return task
}

func (t *Tag) StoreTaskForTrigger(gatewayID int, topic string, active ActiveTask, at time.Time, trigger string, changed bool, first bool) *StoreTask {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.initialized {
		return nil
	}
	routes := active.RoutesForStore(t.Config.VarID)
	if len(routes) == 0 {
		return nil
	}
	selected := make([]DetectionRunStorageRoute, 0, len(routes))
	for _, route := range routes {
		if storageRouteDueLocked(t, route, at, trigger, changed, first) {
			selected = append(selected, route)
		}
	}
	if len(selected) == 0 {
		return nil
	}
	task := t.storeTaskLocked(gatewayID, topic, active, at)
	task.StorageRoutes = selected
	return task
}

func (t *Tag) storeTaskLocked(gatewayID int, topic string, active ActiveTask, at time.Time) *StoreTask {
	return &StoreTask{
		GatewayID:   gatewayID,
		Topic:       topic,
		ProjectID:   active.ProjectID,
		TaskID:      active.ID,
		TestNo:      active.TestNo,
		VarID:       t.Config.VarID,
		VarName:     t.Config.VarName,
		ProjectCode: t.Config.ProjectCode,
		Value:       t.currentValue,
		StrValue:    t.currentStrValue,
		IsString:    isStringType(t.Config.DataType),
		Quality:     t.quality,
		Timestamp:   at,
	}
}

func storageRouteDueLocked(tag *Tag, route DetectionRunStorageRoute, at time.Time, trigger string, changed bool, first bool) bool {
	if route.StorageTarget == StorageTargetNone {
		return false
	}
	mode := strings.TrimSpace(route.TriggerMode)
	if mode == "" {
		mode = StoreTriggerOnCycle
	}
	switch trigger {
	case StoreTriggerOnStart:
		return route.StoreOnStart || mode == StoreTriggerOnStart
	case StoreTriggerOnCycle:
		return storageRouteCycleDueLocked(tag, route, at, mode)
	case StoreTriggerOnChange:
		return storageRouteChangeDueLocked(tag, route, mode, changed, first)
	case StoreTriggerOnDetection:
		return mode == StoreTriggerOnDetection || mode == StoreTriggerAlways
	default:
		return false
	}
}

func storageRouteCycleDueLocked(tag *Tag, route DetectionRunStorageRoute, at time.Time, mode string) bool {
	if mode != StoreTriggerOnCycle && mode != StoreTriggerOnDetection && mode != StoreTriggerAlways {
		return false
	}
	if route.CycleMS <= 0 {
		return false
	}
	state := tag.routeStores[routeStoreKey(route)]
	return !state.Initialized || at.Sub(state.LastStore) >= time.Duration(route.CycleMS)*time.Millisecond
}

func storageRouteChangeDueLocked(tag *Tag, route DetectionRunStorageRoute, mode string, changed bool, first bool) bool {
	if mode != StoreTriggerOnChange && mode != StoreTriggerOnDetection && mode != StoreTriggerAlways {
		return false
	}
	if first || !changed {
		return false
	}
	state := tag.routeStores[routeStoreKey(route)]
	if !state.Initialized {
		return true
	}
	if isStringType(tag.Config.DataType) {
		return tag.currentStrValue != state.LastStrValue
	}
	if route.Deadband <= 0 {
		return tag.currentValue != state.LastValue
	}
	return math.Abs(tag.currentValue-state.LastValue) > route.Deadband
}

func routeStoreKey(route DetectionRunStorageRoute) string {
	if route.ID > 0 {
		return fmt.Sprintf("run:%d", route.ID)
	}
	if route.RouteID > 0 {
		return fmt.Sprintf("route:%d", route.RouteID)
	}
	return fmt.Sprintf("var:%d:%s:%s", route.VarID, route.StorageTable, route.ColumnName)
}

func IsStringDataType(dataType string) bool {
	return isStringType(dataType)
}

func isStringType(dataType string) bool {
	switch strings.ToUpper(dataType) {
	case "STRING", "TEXT", "VARCHAR", "CHAR":
		return true
	default:
		return false
	}
}
