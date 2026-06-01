package pipeline

import (
	"log"
	"sort"
	"sync/atomic"
	"time"

	"spindle-edge/backend/internal/models"
)

type Channels struct {
	Logic     chan *models.MQTTMessage
	Discovery chan *models.MQTTMessage
	Store     chan *models.StoreTask
	Alarm     chan *models.DetectionLimitAlarmEvent
	Notify    chan *models.RuntimeNotification
	drops     channelDropCounters
}

type ChannelPressure struct {
	Name       string  `json:"name"`
	Len        int     `json:"len"`
	Cap        int     `json:"cap"`
	Usage      float64 `json:"usage"`
	Dropped    uint64  `json:"dropped"`
	Pressure   bool    `json:"pressure"`
	Impact     string  `json:"impact,omitempty"`
	NextAction string  `json:"next_action,omitempty"`
}

type channelDropCounters struct {
	logic     atomic.Uint64
	discovery atomic.Uint64
	store     atomic.Uint64
	alarm     atomic.Uint64
	notify    atomic.Uint64
}

func NewChannels() *Channels {
	return &Channels{
		Logic:     make(chan *models.MQTTMessage, 2000),
		Discovery: make(chan *models.MQTTMessage, 200),
		Store:     make(chan *models.StoreTask, 1000),
		Alarm:     make(chan *models.DetectionLimitAlarmEvent, 5000),
		Notify:    make(chan *models.RuntimeNotification, 2000),
	}
}

func (c *Channels) Stats() map[string]int {
	return map[string]int{
		"logic":     len(c.Logic),
		"discovery": len(c.Discovery),
		"store":     len(c.Store),
		"alarm":     len(c.Alarm),
		"notify":    len(c.Notify),
	}
}

func (c *Channels) Pressure(threshold float64) []ChannelPressure {
	if c == nil {
		return nil
	}
	if threshold <= 0 {
		threshold = 0.8
	}
	pressures := make([]ChannelPressure, 0, 5)
	for _, stat := range c.DetailedStatsWithDiagnosis(threshold) {
		if stat.Pressure {
			pressures = append(pressures, stat)
		}
	}
	return pressures
}

func (c *Channels) DetailedStatsWithDiagnosis(threshold float64) []ChannelPressure {
	stats := c.DetailedStats()
	for i := range stats {
		stats[i] = DiagnoseChannelPressure(stats[i], threshold)
	}
	return stats
}

func (c *Channels) DetailedStats() []ChannelPressure {
	if c == nil {
		return nil
	}
	stats := []ChannelPressure{
		channelPressure("alarm", len(c.Alarm), cap(c.Alarm), c.DropCount("alarm")),
		channelPressure("discovery", len(c.Discovery), cap(c.Discovery), c.DropCount("discovery")),
		channelPressure("logic", len(c.Logic), cap(c.Logic), c.DropCount("logic")),
		channelPressure("notify", len(c.Notify), cap(c.Notify), c.DropCount("notify")),
		channelPressure("store", len(c.Store), cap(c.Store), c.DropCount("store")),
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})
	return stats
}

func (c *Channels) RecordDrop(name string) {
	if c == nil {
		return
	}
	switch name {
	case "logic":
		c.drops.logic.Add(1)
	case "discovery":
		c.drops.discovery.Add(1)
	case "store":
		c.drops.store.Add(1)
	case "alarm":
		c.drops.alarm.Add(1)
	case "notify":
		c.drops.notify.Add(1)
	}
}

func (c *Channels) DropCount(name string) uint64 {
	if c == nil {
		return 0
	}
	switch name {
	case "logic":
		return c.drops.logic.Load()
	case "discovery":
		return c.drops.discovery.Load()
	case "store":
		return c.drops.store.Load()
	case "alarm":
		return c.drops.alarm.Load()
	case "notify":
		return c.drops.notify.Load()
	default:
		return 0
	}
}

func channelPressure(name string, length int, capacity int, dropped ...uint64) ChannelPressure {
	stat := ChannelPressure{Name: name, Len: length, Cap: capacity}
	if len(dropped) > 0 {
		stat.Dropped = dropped[0]
	}
	if capacity > 0 {
		stat.Usage = float64(length) / float64(capacity)
	}
	return stat
}

func DiagnoseChannelPressure(stat ChannelPressure, threshold float64) ChannelPressure {
	if threshold <= 0 {
		threshold = 0.8
	}
	if stat.Cap > 0 {
		stat.Usage = float64(stat.Len) / float64(stat.Cap)
		stat.Pressure = stat.Usage >= threshold
	}
	stat.Impact = channelImpact(stat.Name)
	stat.NextAction = channelNextAction(stat.Name, stat.Pressure)
	return stat
}

func channelImpact(name string) string {
	switch name {
	case "logic":
		return "Realtime cleaning, project maps, task triggers, and WebSocket snapshots can lag."
	case "discovery":
		return "New variable discovery can lag; known variable cleaning is not blocked by this queue alone."
	case "store":
		return "History rows can lag behind memory realtime values and detection reports."
	case "alarm":
		return "Limit alarm enter, level_change, recover records, and notifications can lag."
	case "notify":
		return "Unread counts and WebSocket notification delivery can lag."
	case "task_flow":
		return "Variable-triggered business flows, built-in modules, and script execution can lag."
	default:
		return "Runtime work on this queue can lag."
	}
}

func channelNextAction(name string, pressure bool) string {
	if !pressure {
		return "No action needed."
	}
	switch name {
	case "logic":
		return "Check MQTT/KIO input burst, gateway topic/path index hit rate, task-flow fanout, and snapshot subscriber load."
	case "discovery":
		return "Check unknown variable volume, gateway query-all payload size, and whether discovery can be scheduled off peak."
	case "store":
		return "Check MySQL write latency, project wide-table expansion, storage batch size, and active storage routes."
	case "alarm":
		return "Check alarm rule volume, alarm DB write latency, and notification downstream pressure."
	case "notify":
		return "Check slow WebSocket subscribers, unread query pressure, and notification dispatcher logs."
	case "task_flow":
		return "Check high-priority flow volume, cooldown settings, script/SQL duration, and recursive write depth."
	default:
		return "Inspect worker logs and downstream dependencies for this queue."
	}
}

func StartChannelPressureLogger(channels *Channels, interval time.Duration, threshold float64) {
	if channels == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if threshold <= 0 {
		threshold = 0.8
	}
	GoRecovering("channel-pressure-logger", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			for _, pressure := range channels.Pressure(threshold) {
				log.Printf("[runtime] channel pressure channel=%s len=%d cap=%d usage=%.1f%%", pressure.Name, pressure.Len, pressure.Cap, pressure.Usage*100)
			}
		}
	})
}
