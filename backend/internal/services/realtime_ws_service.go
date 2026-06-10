package services

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

const (
	WSTypeConnectionReady           = "connection.ready"
	WSTypeSubscriptionUpdated       = "subscription.updated"
	WSTypeRealtimeVariablesSnapshot = "realtime.variables.snapshot"
	WSTypeDetectionRunsSnapshot     = "detection.runs.snapshot"
	WSTypeNotificationEvent         = "notification.event"
	WSTypeHeartbeat                 = "heartbeat"
	WSTypeCommandAck                = "command.ack"
	WSTypeError                     = "error"
)

type RealtimeWSService struct {
	tags  *pipeline.TagManager
	tasks *pipeline.TaskManager
	now   func() time.Time
}

type RealtimeSubscription struct {
	Topics     map[string]bool
	SourceType string
	GatewayID  *int
	ProjectID  *uint
	VarIDs     map[int64]bool
}

type RealtimeVariableFilter struct {
	SourceType string
	GatewayID  *int
	ProjectID  *uint
	VarIDs     map[int64]bool
}

type WSMessage struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id,omitempty"`
	CommandID string      `json:"command_id,omitempty"`
	At        time.Time   `json:"at"`
	Payload   interface{} `json:"payload,omitempty"`
	Error     *WSError    `json:"error,omitempty"`
}

type WSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewRealtimeWSService(tags *pipeline.TagManager, tasks *pipeline.TaskManager) *RealtimeWSService {
	return &RealtimeWSService{
		tags:  tags,
		tasks: tasks,
		now:   time.Now,
	}
}

func DefaultRealtimeSubscription() RealtimeSubscription {
	return RealtimeSubscription{
		Topics: map[string]bool{
			"realtime.variables": true,
			"detection.runs":     true,
			"notifications":      true,
		},
	}
}

func (s *RealtimeWSService) ReadyMessage(requestID string, sub RealtimeSubscription) WSMessage {
	return s.message(WSTypeConnectionReady, requestID, "", map[string]interface{}{
		"read_only":    false,
		"subscription": sub.ResponsePayload(),
	})
}

func (s *RealtimeWSService) SubscriptionMessage(requestID string, sub RealtimeSubscription) WSMessage {
	return s.message(WSTypeSubscriptionUpdated, requestID, "", sub.ResponsePayload())
}

func (s *RealtimeWSService) VariableSnapshotMessage(sub RealtimeSubscription) WSMessage {
	return s.message(WSTypeRealtimeVariablesSnapshot, "", "", map[string]interface{}{
		"items": s.FilteredSnapshots(sub),
	})
}

func (s *RealtimeWSService) DetectionRunsMessage(sub RealtimeSubscription) WSMessage {
	return s.message(WSTypeDetectionRunsSnapshot, "", "", map[string]interface{}{
		"items": s.FilteredActiveTasks(sub),
	})
}

func (s *RealtimeWSService) NotificationMessage(notification *models.RuntimeNotification) WSMessage {
	return s.message(WSTypeNotificationEvent, "", "", notification)
}

func (s *RealtimeWSService) HeartbeatMessage() WSMessage {
	return s.message(WSTypeHeartbeat, "", "", map[string]interface{}{
		"server_time": s.now(),
	})
}

func (s *RealtimeWSService) CommandAckMessage(requestID string, commandID string, payload interface{}) WSMessage {
	return s.message(WSTypeCommandAck, requestID, commandID, payload)
}

func (s *RealtimeWSService) ErrorMessage(requestID string, commandID string, code string, message string) WSMessage {
	return s.ErrorMessageWithPayload(requestID, commandID, code, message, nil)
}

func (s *RealtimeWSService) ErrorMessageWithPayload(requestID string, commandID string, code string, message string, payload interface{}) WSMessage {
	return WSMessage{
		Type:      WSTypeError,
		RequestID: requestID,
		CommandID: commandID,
		At:        s.now(),
		Payload:   payload,
		Error: &WSError{
			Code:    code,
			Message: message,
		},
	}
}

func (s *RealtimeWSService) FilteredSnapshots(sub RealtimeSubscription) []models.TagSnapshot {
	return filteredTagSnapshots(s.tags, RealtimeVariableFilter{
		SourceType: sub.SourceType,
		GatewayID:  sub.GatewayID,
		ProjectID:  sub.ProjectID,
		VarIDs:     sub.VarIDs,
	})
}

func filteredTagSnapshots(tags *pipeline.TagManager, filter RealtimeVariableFilter) []models.TagSnapshot {
	if len(filter.VarIDs) > 0 {
		ids := varIDList(filter.VarIDs)
		items := make([]models.TagSnapshot, 0, len(ids))
		for _, id := range ids {
			tag, ok := tags.Get(id)
			if !ok {
				continue
			}
			item := tag.Snapshot()
			if realtimeSnapshotMatches(item, filter) {
				items = append(items, item)
			}
		}
		return items
	}
	all := tags.Snapshots()
	if filter.ProjectID != nil {
		all = tags.SnapshotsForProject(*filter.ProjectID)
	}
	items := make([]models.TagSnapshot, 0, len(all))
	for _, item := range all {
		if realtimeSnapshotMatches(item, filter) {
			items = append(items, item)
		}
	}
	return items
}

func realtimeSnapshotMatches(item models.TagSnapshot, filter RealtimeVariableFilter) bool {
	if filter.SourceType != "" && item.SourceType != filter.SourceType {
		return false
	}
	if filter.GatewayID != nil && item.GatewayID != *filter.GatewayID {
		return false
	}
	if filter.ProjectID != nil && (item.ProjectID == nil || *item.ProjectID != *filter.ProjectID) {
		return false
	}
	return true
}

func (s *RealtimeWSService) FilteredActiveTasks(sub RealtimeSubscription) []models.ActiveTask {
	all := s.tasks.AllActive()
	items := make([]models.ActiveTask, 0, len(all))
	for _, item := range all {
		if sub.ProjectID != nil && item.ProjectID != *sub.ProjectID {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (s *RealtimeWSService) NotificationMatches(sub RealtimeSubscription, notification *models.RuntimeNotification) bool {
	if notification == nil {
		return false
	}
	if sub.ProjectID != nil && notification.ProjectID != 0 && notification.ProjectID != *sub.ProjectID {
		return false
	}
	if len(sub.VarIDs) > 0 && notification.VarID != 0 && !sub.VarIDs[notification.VarID] {
		return false
	}
	return true
}

func (s *RealtimeWSService) message(messageType string, requestID string, commandID string, payload interface{}) WSMessage {
	return WSMessage{
		Type:      messageType,
		RequestID: requestID,
		CommandID: commandID,
		At:        s.now(),
		Payload:   payload,
	}
}

func (sub RealtimeSubscription) Wants(topic string) bool {
	if len(sub.Topics) == 0 {
		return false
	}
	return sub.Topics[topic]
}

func (sub RealtimeSubscription) ResponsePayload() map[string]interface{} {
	topics := make([]string, 0, len(sub.Topics))
	for topic, enabled := range sub.Topics {
		if enabled {
			topics = append(topics, topic)
		}
	}
	return map[string]interface{}{
		"topics":       topics,
		"source_type":  sub.SourceType,
		"gateway_id":   sub.GatewayID,
		"project_id":   sub.ProjectID,
		"var_ids":      varIDList(sub.VarIDs),
		"var_id_texts": varIDTextList(sub.VarIDs),
	}
}

func NormalizeWSTopic(topic string) string {
	switch strings.TrimSpace(topic) {
	case "variables", "realtime.variables", WSTypeRealtimeVariablesSnapshot:
		return "realtime.variables"
	case "runs", "detection.runs", WSTypeDetectionRunsSnapshot:
		return "detection.runs"
	case "notifications", "notification", WSTypeNotificationEvent:
		return "notifications"
	default:
		return ""
	}
}

func varIDList(values map[int64]bool) []int64 {
	ids := make([]int64, 0, len(values))
	for id, enabled := range values {
		if enabled {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func varIDTextList(values map[int64]bool) []string {
	ids := varIDList(values)
	items := make([]string, 0, len(ids))
	for _, id := range ids {
		items = append(items, strconv.FormatInt(id, 10))
	}
	return items
}
