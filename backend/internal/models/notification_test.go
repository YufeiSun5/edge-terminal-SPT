package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRuntimeNotificationBuilders(t *testing.T) {
	at := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	notification := NewRuntimeNotification(NotificationDetectionResultOK, NotificationLevelSuccess, "ok", at)
	if notification.ID == "" || notification.Type != NotificationDetectionResultOK || notification.Level != NotificationLevelSuccess || notification.OccurredAt != at {
		t.Fatalf("unexpected notification: %+v", notification)
	}

	startValue := 12.0
	recoverValue := 8.0
	firstSeen := at.Add(-time.Second)
	alarm := DetectionLimitAlarm{
		TaskID:       7,
		TestNo:       "T-7",
		ProjectID:    2,
		ProjectCode:  "AC-02",
		VarID:        9212397624135540842,
		VarName:      "temp",
		DisplayName:  "Temperature",
		AlarmType:    "above_h",
		AlarmLevel:   "H",
		Status:       DetectionAlarmStatusClosed,
		StartValue:   &startValue,
		RecoverValue: &recoverValue,
		FirstSeenAt:  firstSeen,
		LastSeenAt:   at,
	}
	alarmNotification := RuntimeNotificationFromAlarm(NotificationAlarmLimitRecover, NotificationLevelInfo, alarm, map[string]any{"status": alarm.Status})
	if alarmNotification.TargetType != NotificationTargetProject || alarmNotification.TargetID != "2" || alarmNotification.ProjectID != 2 || alarmNotification.TaskID != 7 || alarmNotification.VarID != 9212397624135540842 || alarmNotification.DisplayName != "Temperature" || alarmNotification.OccurredAt != at {
		t.Fatalf("unexpected alarm notification: %+v", alarmNotification)
	}
	encoded, err := json.Marshal(alarmNotification)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"var_id_text":"9212397624135540842"`) {
		t.Fatalf("expected exact runtime notification var_id_text, body=%s", encoded)
	}
	enterNotification := RuntimeNotificationFromAlarm(NotificationAlarmLimitEnter, NotificationLevelWarning, alarm, nil)
	if enterNotification.OccurredAt != firstSeen {
		t.Fatalf("enter notification should use first_seen_at: %+v", enterNotification)
	}

	startedAt := at.Add(-2 * time.Minute)
	endedAt := at
	task := DetectionTask{ID: 9, TestNo: "T-9", ProjectID: 3, ProjectCode: "AC-03", StartedAt: &startedAt, EndedAt: &endedAt}
	runStart := RuntimeNotificationFromDetectionTask(NotificationDetectionRunStarted, NotificationLevelInfo, task, "started", nil)
	if runStart.TargetType != NotificationTargetProject || runStart.TargetID != "3" || runStart.OccurredAt != startedAt || runStart.TaskID != 9 || runStart.ProjectCode != "AC-03" {
		t.Fatalf("unexpected run start notification: %+v", runStart)
	}
	result := RuntimeNotificationFromDetectionTask(NotificationDetectionResultNG, NotificationLevelWarning, task, "ng", map[string]any{"result_status": DetectionSummaryStatusNG})
	if result.OccurredAt != endedAt || result.Payload["result_status"] != DetectionSummaryStatusNG {
		t.Fatalf("unexpected result notification: %+v", result)
	}
}
