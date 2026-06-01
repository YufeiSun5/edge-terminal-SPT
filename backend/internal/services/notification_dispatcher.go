package services

import (
	"log"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

type NotificationDispatcher struct {
	repo *database.Repository
	hub  *NotificationHub
}

func NewNotificationDispatcher(repo *database.Repository, hub *NotificationHub) *NotificationDispatcher {
	return &NotificationDispatcher{repo: repo, hub: hub}
}

func (d *NotificationDispatcher) Start(source <-chan *models.RuntimeNotification) {
	if d == nil || source == nil {
		return
	}
	pipeline.GoRecovering("notification-dispatcher", func() {
		for notification := range source {
			d.Dispatch(notification)
		}
	})
	log.Printf("notification dispatcher started")
}

func (d *NotificationDispatcher) Dispatch(notification *models.RuntimeNotification) {
	if notification == nil {
		return
	}
	if d.repo != nil {
		if _, err := d.repo.CreateRuntimeNotification(notification); err != nil {
			log.Printf("persist notification failed type=%s event_uid=%s err=%v", notification.Type, notification.ID, err)
		}
	}
	if d.hub != nil {
		d.hub.Publish(notification)
	}
}
