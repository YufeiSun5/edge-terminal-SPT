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

func TestNotificationHubRuntimeStats(t *testing.T) {
	hub := NewNotificationHub(nil)
	sub1, cancel1 := hub.Subscribe(1)
	defer cancel1()
	_, cancel2 := hub.Subscribe(1)
	defer cancel2()

	notification := models.NewRuntimeNotification(models.NotificationDetectionResultOK, models.NotificationLevelSuccess, "ok", time.Now())
	hub.Publish(notification)
	stats := hub.RuntimeStats()
	if stats.Subscribers != 2 || stats.Buffered != 2 || stats.Capacity != 2 || stats.Published != 1 || stats.Delivered != 2 || stats.Dropped != 0 || stats.Usage != 1 {
		t.Fatalf("unexpected full-buffer stats: %+v", stats)
	}
	if !stats.Pressure || stats.PressureThreshold != 0.8 || stats.Impact == "" || stats.NextAction == "" {
		t.Fatalf("expected actionable pressure stats: %+v", stats)
	}

	hub.Publish(notification)
	stats = hub.RuntimeStats()
	if stats.Published != 2 || stats.Delivered != 2 || stats.Dropped != 2 {
		t.Fatalf("unexpected dropped stats: %+v", stats)
	}

	<-sub1
	stats = hub.RuntimeStats()
	if stats.Buffered != 1 || stats.Usage != 0.5 {
		t.Fatalf("unexpected stats after draining one subscriber: %+v", stats)
	}
	if !stats.Pressure {
		t.Fatalf("expected dropped delivery to keep pressure flag true: %+v", stats)
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
