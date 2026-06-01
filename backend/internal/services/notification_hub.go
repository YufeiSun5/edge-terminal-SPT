package services

import (
	"sync"
	"sync/atomic"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

type NotificationHub struct {
	source      <-chan *models.RuntimeNotification
	mu          sync.RWMutex
	subscribers map[chan *models.RuntimeNotification]struct{}
	published   atomic.Uint64
	delivered   atomic.Uint64
	dropped     atomic.Uint64
}

type NotificationHubRuntimeStats struct {
	Subscribers       int     `json:"subscribers"`
	Buffered          int     `json:"buffered"`
	Capacity          int     `json:"capacity"`
	Usage             float64 `json:"usage"`
	PressureThreshold float64 `json:"pressure_threshold"`
	Pressure          bool    `json:"pressure"`
	Impact            string  `json:"impact"`
	NextAction        string  `json:"next_action"`
	Published         uint64  `json:"published"`
	Delivered         uint64  `json:"delivered"`
	Dropped           uint64  `json:"dropped"`
}

func NewNotificationHub(source <-chan *models.RuntimeNotification) *NotificationHub {
	return &NotificationHub{
		source:      source,
		subscribers: make(map[chan *models.RuntimeNotification]struct{}),
	}
}

func (h *NotificationHub) Start() {
	if h == nil || h.source == nil {
		return
	}
	pipeline.GoRecovering("notification-hub", func() {
		for notification := range h.source {
			h.Publish(notification)
		}
	})
}

func (h *NotificationHub) Subscribe(buffer int) (<-chan *models.RuntimeNotification, func()) {
	if h == nil {
		closed := make(chan *models.RuntimeNotification)
		close(closed)
		return closed, func() {}
	}
	if buffer <= 0 {
		buffer = 32
	}
	ch := make(chan *models.RuntimeNotification, buffer)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

func (h *NotificationHub) Publish(notification *models.RuntimeNotification) {
	if h == nil || notification == nil {
		return
	}
	h.published.Add(1)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- notification:
			h.delivered.Add(1)
		default:
			h.dropped.Add(1)
		}
	}
}

func (h *NotificationHub) RuntimeStats() NotificationHubRuntimeStats {
	const pressureThreshold = 0.8
	if h == nil {
		return NotificationHubRuntimeStats{
			PressureThreshold: pressureThreshold,
			Impact:            "Unread counts and WebSocket notification delivery can lag.",
			NextAction:        "Notification hub is not initialized; check runtime wiring.",
		}
	}
	stats := NotificationHubRuntimeStats{
		PressureThreshold: pressureThreshold,
		Impact:            "Unread counts and WebSocket notification delivery can lag.",
		Published:         h.published.Load(),
		Delivered:         h.delivered.Load(),
		Dropped:           h.dropped.Load(),
	}
	h.mu.RLock()
	for ch := range h.subscribers {
		stats.Subscribers++
		stats.Buffered += len(ch)
		stats.Capacity += cap(ch)
	}
	h.mu.RUnlock()
	if stats.Capacity > 0 {
		stats.Usage = float64(stats.Buffered) / float64(stats.Capacity)
	}
	stats.Pressure = stats.Usage >= pressureThreshold || stats.Dropped > 0
	if stats.Pressure {
		stats.NextAction = "Check slow WebSocket subscribers, unread query pressure, notification dispatcher logs, and dropped delivery count."
	} else {
		stats.NextAction = "No action needed."
	}
	return stats
}
