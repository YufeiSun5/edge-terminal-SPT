package services

import (
	"testing"
	"time"

	"spindle-edge/backend/internal/models"
)

func TestNotificationHubBroadcastAndUnsubscribe(t *testing.T) {
	source := make(chan *models.RuntimeNotification, 1)
	hub := NewNotificationHub(source)
	hub.Start()

	sub1, cancel1 := hub.Subscribe(1)
	sub2, cancel2 := hub.Subscribe(1)
	defer cancel2()

	source <- models.NewRuntimeNotification(models.NotificationDetectionResultOK, models.NotificationLevelSuccess, "ok", time.Now())
	if got := waitNotification(t, sub1); got.Type != models.NotificationDetectionResultOK {
		t.Fatalf("unexpected sub1 notification: %+v", got)
	}
	if got := waitNotification(t, sub2); got.Type != models.NotificationDetectionResultOK {
		t.Fatalf("unexpected sub2 notification: %+v", got)
	}

	cancel1()
	if _, ok := <-sub1; ok {
		t.Fatal("expected unsubscribed channel to close")
	}
	hub.Publish(models.NewRuntimeNotification(models.NotificationDetectionResultNG, models.NotificationLevelWarning, "ng", time.Now()))
	if got := waitNotification(t, sub2); got.Type != models.NotificationDetectionResultNG {
		t.Fatalf("unexpected sub2 notification after unsubscribe: %+v", got)
	}
}

func waitNotification(t *testing.T, ch <-chan *models.RuntimeNotification) *models.RuntimeNotification {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification")
		return nil
	}
}
