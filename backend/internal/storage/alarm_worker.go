package storage

import (
	"log"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

func StartAlarmWorkers(count int, channels *pipeline.Channels, repo *database.Repository) {
	if count <= 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		go alarmWorker(i, channels, repo)
	}
	log.Printf("alarm workers started: %d", count)
}

func alarmWorker(id int, channels *pipeline.Channels, repo *database.Repository) {
	batch := make([]*models.DetectionLimitAlarmEvent, 0, 1000)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	flushAlarmBatch := func() {
		if len(batch) == 0 {
			return
		}
		flushAlarms(id, batch, repo, channels)
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-channels.Alarm:
			if !ok {
				flushAlarmBatch()
				return
			}
			batch = append(batch, event)
			if len(batch) >= 1000 {
				flushAlarmBatch()
			}
		case <-ticker.C:
			flushAlarmBatch()
		}
	}
}

func flushAlarms(id int, batch []*models.DetectionLimitAlarmEvent, repo *database.Repository, channels *pipeline.Channels) {
	enters := make([]models.DetectionLimitAlarm, 0, len(batch))
	flushEnters := func() {
		if len(enters) == 0 {
			return
		}
		if err := repo.CreateDetectionLimitAlarms(enters); err != nil {
			log.Printf("[alarm-%d] create detection limit alarm batch failed count=%d err=%v", id, len(enters), err)
		} else {
			for _, alarm := range enters {
				publishAlarmNotification(channels, models.NotificationAlarmLimitEnter, models.NotificationLevelWarning, alarm, nil)
			}
		}
		enters = enters[:0]
	}
	for _, event := range batch {
		switch event.Action {
		case models.DetectionAlarmActionEnter:
			enters = append(enters, event.Alarm)
		case models.DetectionAlarmActionRecover:
			flushEnters()
			if err := repo.RecoverDetectionLimitAlarm(event); err != nil {
				log.Printf("[alarm-%d] recover detection limit alarm failed task_id=%d var_id=%d err=%v", id, event.Alarm.TaskID, event.Alarm.VarID, err)
			} else {
				publishAlarmNotification(channels, models.NotificationAlarmLimitRecover, models.NotificationLevelInfo, event.Alarm, nil)
			}
		case models.DetectionAlarmActionLevelChange:
			flushEnters()
			if err := repo.ChangeDetectionLimitAlarmLevel(event); err != nil {
				log.Printf("[alarm-%d] change detection limit alarm level failed task_id=%d var_id=%d err=%v", id, event.Alarm.TaskID, event.Alarm.VarID, err)
			} else {
				publishAlarmNotification(channels, models.NotificationAlarmLimitLevelChange, models.NotificationLevelWarning, event.Alarm, map[string]any{
					"previous_alarm_type": event.PreviousAlarmType,
				})
			}
		default:
			log.Printf("[alarm-%d] unsupported detection alarm action=%s task_id=%d var_id=%d", id, event.Action, event.Alarm.TaskID, event.Alarm.VarID)
		}
	}
	flushEnters()
}

func publishAlarmNotification(channels *pipeline.Channels, notificationType string, level string, alarm models.DetectionLimitAlarm, payload map[string]any) {
	if channels == nil || channels.Notify == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["alarm_type"] = alarm.AlarmType
	payload["alarm_level"] = alarm.AlarmLevel
	payload["status"] = alarm.Status
	payload["limit_value"] = alarm.LimitValue
	payload["start_value"] = alarm.StartValue
	payload["peak_value"] = alarm.PeakValue
	payload["recover_value"] = alarm.RecoverValue
	notification := models.RuntimeNotificationFromAlarm(notificationType, level, alarm, payload)
	select {
	case channels.Notify <- notification:
	default:
		log.Printf("alarm notification dropped type=%s task_id=%d var_id=%d: notify queue full", notificationType, alarm.TaskID, alarm.VarID)
	}
}
