package pipeline

import (
	"log"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/protocol/kio"

	"github.com/tidwall/gjson"
)

func StartLogicWorkers(count int, channels *Channels, tags *TagManager, tasks *TaskManager, flows ...*TaskFlowExecutor) {
	for i := 0; i < count; i++ {
		go logicWorker(i, channels, tags, tasks, firstTaskFlowExecutor(flows))
	}
	log.Printf("logic workers started: %d", count)
}

func firstTaskFlowExecutor(flows []*TaskFlowExecutor) *TaskFlowExecutor {
	if len(flows) == 0 {
		return nil
	}
	return flows[0]
}

func logicWorker(id int, channels *Channels, tags *TagManager, tasks *TaskManager, flows *TaskFlowExecutor) {
	for msg := range channels.Logic {
		processMessage(id, msg, channels, tags, tasks, flows)
	}
}

func processMessage(workerID int, msg *models.MQTTMessage, channels *Channels, tags *TagManager, tasks *TaskManager, flows ...*TaskFlowExecutor) {
	payload := string(msg.Payload)
	if !gjson.Valid(payload) {
		log.Printf("[logic-%d] invalid json topic=%s", workerID, msg.Topic)
		return
	}

	matchedTags := tags.ForMessage(msg.GatewayID, msg.Topic)
	changed := make([]*models.StoreTask, 0, 16)

	for _, tag := range matchedTags {
		result, quality := extractValue(payload, tag)
		if !result.Exists() || result.Type == gjson.Null {
			continue
		}

		now := msg.Timestamp
		if now.IsZero() {
			now = time.Now()
		}

		if models.IsStringDataType(strings.ToUpper(tag.Config.DataType)) {
			_, didChange, first := tag.UpdateString(result.String(), now, quality)
			if didChange || first {
				if executor := firstTaskFlowExecutor(flows); executor != nil {
					executor.Trigger(TaskFlowEvent{
						TriggerType:  models.TaskFlowTriggerDataChange,
						ProjectID:    valueUint(tag.Config.ProjectID),
						TriggerVarID: tag.Config.VarID,
						TriggerValue: result.String(),
						GatewayID:    msg.GatewayID,
						Topic:        msg.Topic,
						At:           now,
					})
				}
			}
			if storeTask := buildStoreTaskForTrigger(tag, tasks, msg.GatewayID, msg.Topic, now, models.StoreTriggerOnChange, didChange, first); storeTask != nil {
				changed = append(changed, storeTask)
			}
			continue
		}

		value := numericValue(result, tag.Config.DataType)
		oldValue, didChange, first := tag.UpdateNumeric(value, now, quality)
		if didChange || first {
			if executor := firstTaskFlowExecutor(flows); executor != nil {
				executor.Trigger(TaskFlowEvent{
					TriggerType:  models.TaskFlowTriggerDataChange,
					ProjectID:    valueUint(tag.Config.ProjectID),
					TriggerVarID: tag.Config.VarID,
					TriggerValue: value,
					GatewayID:    msg.GatewayID,
					Topic:        msg.Topic,
					At:           now,
				})
			}
		}
		for _, match := range tasks.EvaluateTaskRules(tag.Config.VarID, oldValue, tag.RuntimeState().Value, didChange, first) {
			log.Printf("[logic-%d] task rule matched rule=%s action=%s var_id=%d", workerID, match.Rule.RuleCode, match.Rule.ActionType, tag.Config.VarID)
		}
		enqueueAlarmEvents(channels, tasks.EvaluateLimitAlarm(tag, now, false), workerID)
		enqueueAlarmEvents(channels, tasks.EvaluateDefaultLimitAlarm(tag, now), workerID)
		if storeTask := buildStoreTaskForTrigger(tag, tasks, msg.GatewayID, msg.Topic, now, models.StoreTriggerOnChange, didChange, first); storeTask != nil {
			changed = append(changed, storeTask)
		}
	}

	for _, task := range changed {
		select {
		case channels.Store <- task:
			if tag, ok := tags.Get(task.VarID); ok {
				tag.MarkStorageRoutesStored(task.StorageRoutes, task.Timestamp)
			}
		default:
			log.Printf("[logic-%d] store queue full, drop var_id=%d", workerID, task.VarID)
		}
	}
}

func valueUint(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func enqueueAlarmEvents(channels *Channels, events []*models.DetectionLimitAlarmEvent, workerID int) {
	for _, event := range events {
		select {
		case channels.Alarm <- event:
		default:
			log.Printf("[logic-%d] alarm queue full, drop action=%s task_id=%d var_id=%d", workerID, event.Action, event.Alarm.TaskID, event.Alarm.VarID)
		}
	}
}

func buildStoreTaskIfAllowed(tag *models.Tag, tasks *TaskManager, gatewayID int, topic string, at time.Time) *models.StoreTask {
	return buildStoreTaskForTrigger(tag, tasks, gatewayID, topic, at, models.StoreTriggerOnChange, true, false)
}

func buildStoreTaskForTrigger(tag *models.Tag, tasks *TaskManager, gatewayID int, topic string, at time.Time, trigger string, changed bool, first bool) *models.StoreTask {
	if tag.Config.ProjectID == nil {
		return nil
	}
	active, ok := tasks.ActiveForProject(*tag.Config.ProjectID)
	if !ok {
		return nil
	}
	if !active.AllowsStore(tag.Config.VarID) {
		return nil
	}
	return tag.StoreTaskForTrigger(gatewayID, topic, active, at, trigger, changed, first)
}

func extractValue(payload string, tag *models.Tag) (gjson.Result, int) {
	return kio.ExtractValue(payload, tag.Config.VarName, tag.Config.JSONPath)
}

func numericValue(result gjson.Result, dataType string) float64 {
	switch strings.ToUpper(dataType) {
	case "BOOL", "BOOLEAN":
		if result.Bool() {
			return 1
		}
		return 0
	case "INT", "INT16", "INT32", "INT64", "INTEGER", "LONG":
		return float64(result.Int())
	default:
		return result.Float()
	}
}
