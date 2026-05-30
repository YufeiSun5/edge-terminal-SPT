package services

import (
	"sync"

	"spindle-edge/backend/internal/models"
)

type NotificationHub struct {
	source      <-chan *models.RuntimeNotification
	mu          sync.RWMutex
	subscribers map[chan *models.RuntimeNotification]struct{}
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
	go func() {
		for notification := range h.source {
			h.Publish(notification)
		}
	}()
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
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- notification:
		default:
		}
	}
}
