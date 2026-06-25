package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	wsReadLimit               = 256 * 1024
	wsWriteWait               = 5 * time.Second
	wsPongWait                = 45 * time.Second
	wsPingPeriod              = 15 * time.Second
	defaultWSSnapshotInterval = 500 * time.Millisecond
	minimumWSSnapshotInterval = 250 * time.Millisecond
	maximumWSSnapshotInterval = 5 * time.Second
)

type RealtimeWSHandler struct {
	service   *services.RealtimeWSService
	detection *services.DetectionRunsService
	variables *services.VariableWriteService
	notify    *services.NotificationHub
	audit     wsAuditStore
	upgrader  websocket.Upgrader
	interval  time.Duration
}

type wsClientMessage struct {
	Type       string            `json:"type"`
	RequestID  string            `json:"request_id"`
	CommandID  string            `json:"command_id"`
	Topics     []string          `json:"topics"`
	SourceType string            `json:"source_type"`
	GatewayID  *int              `json:"gateway_id"`
	ProjectID  *uint             `json:"project_id"`
	VarIDs     flexibleInt64List `json:"var_ids"`
	Payload    json.RawMessage   `json:"payload"`
}

type wsAuditStore interface {
	CreateAuditLog(entry *models.SysAuditLog) error
}

func NewRealtimeWSHandler(service *services.RealtimeWSService, detection *services.DetectionRunsService, audit wsAuditStore, variableWriters ...*services.VariableWriteService) *RealtimeWSHandler {
	var variableWriter *services.VariableWriteService
	if len(variableWriters) > 0 {
		variableWriter = variableWriters[0]
	}
	return &RealtimeWSHandler{
		service:   service,
		detection: detection,
		variables: variableWriter,
		audit:     audit,
		interval:  defaultWSSnapshotInterval,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *RealtimeWSHandler) WithSnapshotInterval(interval time.Duration) *RealtimeWSHandler {
	if interval < minimumWSSnapshotInterval {
		interval = minimumWSSnapshotInterval
	}
	if interval > maximumWSSnapshotInterval {
		interval = maximumWSSnapshotInterval
	}
	h.interval = interval
	return h
}

func (h *RealtimeWSHandler) WithNotificationHub(hub *services.NotificationHub) *RealtimeWSHandler {
	h.notify = hub
	return h
}

func (h *RealtimeWSHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/ws", authService.RequireUserFromBearerOrQuery(), authService.RequirePermission(auth.PermViewRealtime), h.connect)
	control := group.Group("/edge-control")
	control.GET("/ws", authService.RequireServiceScope(auth.ScopeServiceRealtimeRead), h.connect)
}

func (h *RealtimeWSHandler) connect(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	sub := subscriptionFromQuery(c)
	principal, _ := auth.PrincipalFromContext(c)
	if err := writeWSJSON(conn, h.service.ReadyMessage(c.Query("request_id"), sub)); err != nil {
		return
	}
	if err := h.writeSnapshots(conn, sub); err != nil {
		return
	}

	incoming := make(chan wsClientMessage, 8)
	done := make(chan struct{})
	go readWSMessages(conn, incoming, done)

	snapshotTicker := time.NewTicker(h.interval)
	heartbeatTicker := time.NewTicker(wsPingPeriod)
	defer snapshotTicker.Stop()
	defer heartbeatTicker.Stop()
	notifications, unsubscribe := h.notify.Subscribe(128)
	defer unsubscribe()

	for {
		select {
		case <-done:
			return
		case notification, ok := <-notifications:
			if !ok {
				notifications = nil
				continue
			}
			if sub.Wants("notifications") && h.service.NotificationMatches(sub, notification) {
				if err := writeWSJSON(conn, h.service.NotificationMessage(notification)); err != nil {
					return
				}
			}
		case msg := <-incoming:
			next, responses := h.handleClientMessage(sub, msg, principal)
			sub = next
			for _, response := range responses {
				if err := writeWSJSON(conn, response); err != nil {
					return
				}
			}
		case <-snapshotTicker.C:
			if err := h.writeSnapshots(conn, sub); err != nil {
				return
			}
		case <-heartbeatTicker.C:
			if err := writeWSControl(conn, websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
			if err := writeWSJSON(conn, h.service.HeartbeatMessage()); err != nil {
				return
			}
		}
	}
}

func readWSMessages(conn *websocket.Conn, incoming chan<- wsClientMessage, done chan<- struct{}) {
	defer close(done)
	conn.SetReadLimit(wsReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	for {
		var msg wsClientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		select {
		case incoming <- msg:
		default:
		}
	}
}

func (h *RealtimeWSHandler) writeSnapshots(conn *websocket.Conn, sub services.RealtimeSubscription) error {
	if sub.Wants("realtime.variables") {
		if err := writeWSJSON(conn, h.service.VariableSnapshotMessage(sub)); err != nil {
			return err
		}
	}
	if sub.Wants("detection.runs") {
		if err := writeWSJSON(conn, h.service.DetectionRunsMessage(sub)); err != nil {
			return err
		}
	}
	return nil
}

func writeWSJSON(conn *websocket.Conn, value interface{}) error {
	if err := conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
		return err
	}
	return conn.WriteJSON(value)
}

func writeWSControl(conn *websocket.Conn, messageType int, data []byte) error {
	return conn.WriteControl(messageType, data, time.Now().Add(wsWriteWait))
}

func (h *RealtimeWSHandler) handleClientMessage(current services.RealtimeSubscription, msg wsClientMessage, principal auth.Principal) (services.RealtimeSubscription, []services.WSMessage) {
	if strings.HasPrefix(msg.Type, "command.") {
		return current, []services.WSMessage{h.handleCommand(msg, principal)}
	}
	if msg.CommandID != "" {
		return current, []services.WSMessage{h.service.ErrorMessage(msg.RequestID, msg.CommandID, "unsupported_command", "command_id is only valid for supported command messages")}
	}
	switch msg.Type {
	case "ping":
		return current, []services.WSMessage{h.service.HeartbeatMessage()}
	case "subscribe":
		next := current
		if len(msg.Topics) > 0 {
			next.Topics = make(map[string]bool, len(msg.Topics))
			for _, raw := range msg.Topics {
				topic := services.NormalizeWSTopic(raw)
				if topic != "" {
					next.Topics[topic] = true
				}
			}
			if len(next.Topics) == 0 {
				return current, []services.WSMessage{
					h.service.ErrorMessage(msg.RequestID, "", "invalid_subscription", "at least one supported topic is required"),
				}
			}
		}
		next.SourceType = strings.TrimSpace(msg.SourceType)
		next.GatewayID = msg.GatewayID
		next.ProjectID = msg.ProjectID
		varIDs := msg.VarIDs.Int64s()
		next.VarIDs = make(map[int64]bool, len(varIDs))
		for _, id := range varIDs {
			next.VarIDs[id] = true
		}
		return next, []services.WSMessage{h.service.SubscriptionMessage(msg.RequestID, next)}
	case "":
		return current, []services.WSMessage{
			h.service.ErrorMessage(msg.RequestID, "", "invalid_message", "message type is required"),
		}
	default:
		return current, []services.WSMessage{
			h.service.ErrorMessage(msg.RequestID, "", "unsupported_message", "message type is not supported by the read-only websocket"),
		}
	}
}

func (h *RealtimeWSHandler) handleCommand(msg wsClientMessage, principal auth.Principal) services.WSMessage {
	started := time.Now()
	if msg.CommandID == "" {
		response := h.service.ErrorMessage(msg.RequestID, msg.CommandID, "command_id_required", "command_id is required")
		h.auditWSCommand(principal, msg, "failed", "command_id is required", started, nil)
		return response
	}
	var payload interface{}
	var err error
	status := http.StatusOK
	switch msg.Type {
	case "command.detection.start":
		if !auth.RoleHasPermission(principal.Role, auth.PermStartDetection) {
			err = authPermissionError(auth.PermStartDetection)
			status = http.StatusForbidden
			break
		}
		payload, err = h.startDetectionFromWS(msg)
	case "command.detection.stop":
		if !auth.RoleHasPermission(principal.Role, auth.PermStopDetection) {
			err = authPermissionError(auth.PermStopDetection)
			status = http.StatusForbidden
			break
		}
		payload, err = h.stopDetectionFromWS(msg, false)
	case "command.detection.abnormal_stop":
		if !auth.RoleHasPermission(principal.Role, auth.PermStopDetection) {
			err = authPermissionError(auth.PermStopDetection)
			status = http.StatusForbidden
			break
		}
		payload, err = h.stopDetectionFromWS(msg, true)
	case "command.write_variable":
		if !auth.RoleHasPermission(principal.Role, auth.PermKIOWrite) {
			err = authPermissionError(auth.PermKIOWrite)
			status = http.StatusForbidden
			break
		}
		payload, err = h.writeVariableFromWS(msg)
	default:
		err = wsCommandError{code: "unsupported_command", message: "websocket command is not supported"}
		status = http.StatusBadRequest
	}
	if err != nil {
		if status == http.StatusOK {
			status = services.HTTPStatusForError(err)
		}
		code := wsErrorCode(err)
		h.auditWSCommand(principal, msg, "failed", err.Error(), started, map[string]interface{}{"status": status, "code": code})
		return h.service.ErrorMessageWithPayload(msg.RequestID, msg.CommandID, code, err.Error(), wsCommandErrorPayload(payload))
	}
	h.auditWSCommand(principal, msg, "success", "", started, map[string]interface{}{"status": status})
	return h.service.CommandAckMessage(msg.RequestID, msg.CommandID, map[string]interface{}{
		"command": msg.Type,
		"result":  payload,
	})
}

func wsCommandErrorPayload(result interface{}) interface{} {
	if result == nil {
		return nil
	}
	switch typed := result.(type) {
	case services.VariableWriteResult:
		if typed.VarID == 0 && typed.VarIDText == "" && typed.VarName == "" && typed.KIO == nil {
			return nil
		}
		return map[string]interface{}{"result": typed}
	default:
		return map[string]interface{}{"result": result}
	}
}

func (h *RealtimeWSHandler) startDetectionFromWS(msg wsClientMessage) (*models.DetectionTask, error) {
	var req startDetectionRequest
	if err := decodeWSPayload(msg.Payload, &req); err != nil {
		return nil, err
	}
	if req.ProjectID == 0 {
		return nil, wsCommandError{code: "invalid_payload", message: "project_id is required"}
	}
	return h.detection.Start(database.StartDetectionOptions{
		ProjectID:        req.ProjectID,
		TestNo:           req.TestNo,
		FactoryNo:        req.FactoryNo,
		CustomerName:     req.CustomerName,
		DeviceModel:      req.DeviceModel,
		Mode:             req.Mode,
		StandardID:       req.StandardID,
		ConfigEnabled:    req.ConfigEnabled,
		ConfigCode:       req.ConfigCode,
		ConfigName:       req.ConfigName,
		ConfigVersion:    req.ConfigVersion,
		ConfigHash:       req.ConfigHash,
		DurationSec:      req.DurationSec,
		OperatorNote:     req.OperatorNote,
		ReportTemplateID: req.ReportTemplateID,
	})
}

func (h *RealtimeWSHandler) stopDetectionFromWS(msg wsClientMessage, abnormal bool) (*models.DetectionTask, error) {
	var req struct {
		TaskID uint   `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := decodeWSPayload(msg.Payload, &req); err != nil {
		return nil, err
	}
	if req.TaskID == 0 {
		return nil, wsCommandError{code: "invalid_payload", message: "task_id is required"}
	}
	if abnormal {
		return h.detection.AbnormalStop(req.TaskID, req.Reason)
	}
	return h.detection.Stop(req.TaskID, req.Reason)
}

func (h *RealtimeWSHandler) writeVariableFromWS(msg wsClientMessage) (services.VariableWriteResult, error) {
	if h.variables == nil {
		return services.VariableWriteResult{}, wsCommandError{code: "unsupported_command", message: "variable write service is not available"}
	}
	var req struct {
		VarID          flexibleInt64 `json:"var_id"`
		ProjectID      uint          `json:"project_id"`
		ProjectCode    string        `json:"project_code"`
		VarName        string        `json:"var_name"`
		Value          any           `json:"value"`
		Quality        int           `json:"quality"`
		Trigger        *bool         `json:"trigger"`
		WaitAck        bool          `json:"wait_ack"`
		AckTimeoutSec  int           `json:"ack_timeout_sec"`
		OriginFlowID   uint64        `json:"origin_flow_id"`
		OriginRunID    uint64        `json:"origin_run_id"`
		Depth          int           `json:"depth"`
		MaxDepth       int           `json:"max_depth"`
		AllowReentrant bool          `json:"allow_reentrant"`
	}
	if err := decodeWSPayload(msg.Payload, &req); err != nil {
		return services.VariableWriteResult{}, err
	}
	trigger := true
	if req.Trigger != nil {
		trigger = *req.Trigger
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := h.variables.Write(ctx, services.VariableWriteInput{
		VarID:          req.VarID.Int64(),
		ProjectID:      req.ProjectID,
		ProjectCode:    req.ProjectCode,
		VarName:        req.VarName,
		Value:          req.Value,
		Quality:        req.Quality,
		Trigger:        trigger,
		WaitAck:        req.WaitAck,
		AckTimeoutSec:  req.AckTimeoutSec,
		OriginFlowID:   req.OriginFlowID,
		OriginRunID:    req.OriginRunID,
		Depth:          req.Depth,
		MaxDepth:       req.MaxDepth,
		AllowReentrant: req.AllowReentrant,
		RequestID:      msg.RequestID,
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func decodeWSPayload(raw json.RawMessage, out interface{}) error {
	if len(raw) == 0 {
		return wsCommandError{code: "invalid_payload", message: "payload is required"}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return wsCommandError{code: "invalid_payload", message: "payload is invalid"}
	}
	return nil
}

func (h *RealtimeWSHandler) auditWSCommand(principal auth.Principal, msg wsClientMessage, result string, errText string, started time.Time, extra map[string]interface{}) {
	if h.audit == nil {
		return
	}
	actorType, actorID := wsAuditActor(principal)
	detail := map[string]interface{}{
		"request_id": msg.RequestID,
		"command_id": msg.CommandID,
		"command":    msg.Type,
		"latency_ms": time.Since(started).Milliseconds(),
	}
	if principal.Username != "" {
		detail["actor_name"] = principal.Username
	}
	if errText != "" {
		detail["error"] = errText
	}
	for key, value := range extra {
		detail[key] = value
	}
	raw, marshalErr := json.Marshal(detail)
	if marshalErr != nil {
		raw = []byte(`{"error":"failed to marshal audit detail"}`)
	}
	_ = h.audit.CreateAuditLog(&models.SysAuditLog{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "ws." + msg.Type,
		TargetType: "ws_command",
		TargetID:   msg.Type,
		Result:     result,
		Detail:     string(raw),
		CreatedAt:  time.Now(),
	})
}

func wsAuditActor(principal auth.Principal) (string, string) {
	if principal.AuthType == "user" {
		return "user", strconv.FormatUint(uint64(principal.UserID), 10)
	}
	if principal.AuthType == "service" {
		return "service", principal.ClientID
	}
	return "unknown", ""
}

func authPermissionError(permission string) error {
	return wsCommandError{code: "forbidden", message: "permission " + permission + " required"}
}

func wsErrorCode(err error) string {
	if typed, ok := err.(wsCommandError); ok {
		return typed.code
	}
	var writeErr *services.VariableWriteError
	if errors.As(err, &writeErr) {
		return writeErr.Code
	}
	if strings.Contains(err.Error(), "permission ") {
		return "forbidden"
	}
	if strings.Contains(err.Error(), "already has a running") || strings.Contains(err.Error(), "referenced") {
		return "conflict"
	}
	return "command_failed"
}

type wsCommandError struct {
	code    string
	message string
}

func (e wsCommandError) Error() string {
	return e.message
}

func subscriptionFromQuery(c *gin.Context) services.RealtimeSubscription {
	sub := services.DefaultRealtimeSubscription()
	if topics := c.QueryArray("topic"); len(topics) > 0 {
		sub.Topics = make(map[string]bool, len(topics))
		for _, raw := range topics {
			if topic := services.NormalizeWSTopic(raw); topic != "" {
				sub.Topics[topic] = true
			}
		}
		if len(sub.Topics) == 0 {
			sub.Topics = services.DefaultRealtimeSubscription().Topics
		}
	}
	sub.SourceType = strings.TrimSpace(c.Query("source_type"))
	if raw := c.Query("gateway_id"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			sub.GatewayID = &parsed
		}
	}
	if raw := c.Query("project_id"); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
			value := uint(parsed)
			sub.ProjectID = &value
		}
	}
	rawVarIDs := c.QueryArray("var_id")
	if len(rawVarIDs) > 0 {
		varIDs, err := parseVarIDQueryValues(rawVarIDs)
		if err == nil {
			sub.VarIDs = varIDs
		}
	}
	return sub
}
