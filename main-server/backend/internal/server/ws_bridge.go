package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"spindle-main-server/backend/internal/auth"
	"spindle-main-server/backend/internal/edgecontrol"
	"spindle-main-server/backend/internal/query"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	mainWSReadLimit  = 4 * 1024 * 1024
	mainWSWriteWait  = 5 * time.Second
	mainWSEdgeWSPath = "api/v1/edge-control/ws"
)

var mainWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type mainWSClientMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	CommandID string          `json:"command_id"`
	Payload   json.RawMessage `json:"payload"`
}

func mainServerRealtimeWSBridge(registry *edgeRegistry, stationViewQuery *query.StationViewQuery) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, edgeInstanceID, ok := resolveRealtimeEdgeClient(c, registry, stationViewQuery)
		if !ok {
			return
		}
		token, err := client.ServiceToken()
		if err != nil {
			writeEdgeRealtimeForwardError(c, err, client.ServiceTokenRef())
			return
		}
		edgeURL, err := client.WebSocketURL(mainWSEdgeWSPath, edgeForwardRawQuery(c.Request.URL.Query()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "edge websocket url build failed", "code": "internal_error"})
			return
		}
		edgeConn, _, err := websocket.DefaultDialer.DialContext(c.Request.Context(), edgeURL, http.Header{
			"Authorization": []string{"Bearer " + token},
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":       "edge realtime websocket backend unavailable",
				"code":        "edge_realtime_ws_unavailable",
				"next_action": "check edge base_url, service token scopes, and edge backend websocket health",
			})
			return
		}
		defer func() { _ = edgeConn.Close() }()

		clientConn, err := mainWSUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer func() { _ = clientConn.Close() }()

		principal, _ := auth.PrincipalFromContext(c)
		commandEdgeHint := commandEdgeHintFromQuery(c, stationViewQuery)
		bridgeMainWebSocket(c.Request.Context(), clientConn, edgeConn, registry, stationViewQuery, client, principal, edgeInstanceID, commandEdgeHint)
	}
}

func bridgeMainWebSocket(ctx context.Context, clientConn *websocket.Conn, edgeConn *websocket.Conn, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, edgeClient *edgecontrol.Client, principal auth.Principal, edgeInstanceID string, commandEdgeHint string) {
	edgeDone := make(chan struct{}, 1)
	clientDone := make(chan struct{}, 1)
	var clientWriteMu sync.Mutex
	go forwardEdgeWSToClient(edgeConn, clientConn, &clientWriteMu, edgeInstanceID, edgeDone)
	go forwardClientWSToEdge(ctx, clientConn, edgeConn, &clientWriteMu, registry, stationViewQuery, edgeClient, principal, edgeInstanceID, commandEdgeHint, clientDone)
	<-clientDone
}

func forwardEdgeWSToClient(edgeConn *websocket.Conn, clientConn *websocket.Conn, clientWriteMu *sync.Mutex, edgeInstanceID string, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	edgeConn.SetReadLimit(mainWSReadLimit)
	for {
		messageType, payload, err := edgeConn.ReadMessage()
		if err != nil {
			return
		}
		if messageType == websocket.TextMessage {
			payload = injectEdgeInstanceID(payload, edgeInstanceID)
		}
		clientWriteMu.Lock()
		err = writeMainWSMessage(clientConn, messageType, payload)
		clientWriteMu.Unlock()
		if err != nil {
			return
		}
	}
}

func forwardClientWSToEdge(ctx context.Context, clientConn *websocket.Conn, edgeConn *websocket.Conn, clientWriteMu *sync.Mutex, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, edgeClient *edgecontrol.Client, principal auth.Principal, edgeInstanceID string, commandEdgeHint string, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	clientConn.SetReadLimit(mainWSReadLimit)
	for {
		messageType, payload, err := clientConn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			if err := writeMainWSMessage(edgeConn, messageType, payload); err != nil {
				return
			}
			continue
		}
		var msg mainWSClientMessage
		if err := json.Unmarshal(payload, &msg); err == nil && strings.HasPrefix(strings.TrimSpace(msg.Type), "command.") {
			response := handleMainWSCommand(ctx, registry, stationViewQuery, edgeClient, principal, edgeInstanceID, commandEdgeHint, msg)
			clientWriteMu.Lock()
			err = writeMainWSJSON(clientConn, response)
			clientWriteMu.Unlock()
			if err != nil {
				return
			}
			continue
		}
		if err := writeMainWSMessage(edgeConn, messageType, payload); err != nil {
			return
		}
	}
}

func writeMainWSMessage(conn *websocket.Conn, messageType int, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(mainWSWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, payload)
}

func writeMainWSJSON(conn *websocket.Conn, payload any) error {
	if err := conn.SetWriteDeadline(time.Now().Add(mainWSWriteWait)); err != nil {
		return err
	}
	return conn.WriteJSON(payload)
}

func injectEdgeInstanceID(payload []byte, edgeInstanceID string) []byte {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return payload
	}
	object["edge_instance_id"] = edgeInstanceID
	raw, err := json.Marshal(object)
	if err != nil {
		return payload
	}
	return raw
}

func handleMainWSCommand(ctx context.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, fallbackClient *edgecontrol.Client, principal auth.Principal, fallbackEdgeInstanceID string, commandEdgeHint string, msg mainWSClientMessage) map[string]any {
	if strings.TrimSpace(msg.CommandID) == "" {
		return mainWSError(msg.RequestID, msg.CommandID, fallbackEdgeInstanceID, "command_id_required", "command_id is required")
	}
	edgePath, permission, ok := mainWSCommandRoute(msg.Type)
	if !ok {
		return mainWSError(msg.RequestID, msg.CommandID, fallbackEdgeInstanceID, "unsupported_command", "websocket command is not supported by the main-server bridge")
	}
	if !auth.RoleHasPermission(principal.Role, permission) {
		return mainWSError(msg.RequestID, msg.CommandID, fallbackEdgeInstanceID, "forbidden", "permission "+permission+" required")
	}
	payload := msg.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	edgeInstanceID, client, err := resolveMainWSCommandEdge(registry, stationViewQuery, commandEdgeHint, payload, fallbackEdgeInstanceID, fallbackClient)
	if err != nil {
		return mainWSErrorFromResolveError(msg.RequestID, msg.CommandID, fallbackEdgeInstanceID, err)
	}
	envelope, err := json.Marshal(gin.H{
		"command_id":        msg.CommandID,
		"operator_id":       strconv.FormatUint(uint64(principal.UserID), 10),
		"operator_name":     principal.Username,
		"operator_username": principal.Username,
		"payload":           payload,
	})
	if err != nil {
		return mainWSError(msg.RequestID, msg.CommandID, edgeInstanceID, "internal_error", "edge control envelope build failed")
	}
	resp, err := client.Forward(ctx, edgePath, "", envelope, msg.CommandID)
	if err != nil {
		return mainWSError(msg.RequestID, msg.CommandID, edgeInstanceID, mainWSEdgeForwardCode(err), mainWSEdgeForwardMessage(err))
	}
	var body any
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message, result := mainWSErrorFromBody(body)
		response := mainWSError(msg.RequestID, msg.CommandID, edgeInstanceID, code, message)
		if result != nil {
			response["payload"] = gin.H{"result": result}
		}
		return response
	}
	return map[string]any{
		"type":             "command.ack",
		"request_id":       msg.RequestID,
		"command_id":       msg.CommandID,
		"edge_instance_id": edgeInstanceID,
		"at":               time.Now(),
		"payload": gin.H{
			"command": msg.Type,
			"result":  body,
		},
	}
}

func commandEdgeHintFromQuery(c *gin.Context, stationViewQuery *query.StationViewQuery) string {
	requested := controlRequestedEdgeID(c)
	if requested != "" {
		return requested
	}
	if projectID, ok, err := optionalProjectID(c); err == nil && ok {
		edgeID, lookupErr := stationViewQuery.ProjectEdgeInstanceID(uint(projectID), "")
		if lookupErr == nil {
			return edgeID
		}
	}
	if taskIDRaw := strings.TrimSpace(c.Query("task_id")); taskIDRaw != "" {
		taskID, err := strconv.ParseUint(taskIDRaw, 10, 64)
		if err == nil && taskID > 0 {
			edgeID, lookupErr := stationViewQuery.DetectionRunEdgeInstanceID(uint(taskID), "")
			if lookupErr == nil {
				return edgeID
			}
		}
	}
	return ""
}

func resolveMainWSCommandEdge(registry *edgeRegistry, stationViewQuery *query.StationViewQuery, requestedEdgeID string, payload json.RawMessage, fallbackEdgeInstanceID string, fallbackClient *edgecontrol.Client) (string, *edgecontrol.Client, error) {
	target := controlTargetFromJSON(payload)
	edgeInstanceID, err := resolveControlEdgeInstanceIDFromTarget(registry, stationViewQuery, requestedEdgeID, target)
	if err != nil {
		return "", nil, err
	}
	if edgeInstanceID == fallbackEdgeInstanceID && fallbackClient != nil {
		return edgeInstanceID, fallbackClient, nil
	}
	client, ok := registry.Client(edgeInstanceID)
	if !ok {
		return "", nil, controlEdgeResolveError{
			Status:         http.StatusNotFound,
			Code:           "edge_instance_not_found",
			Message:        "edge instance is not available on this main-server bridge",
			EdgeInstanceID: edgeInstanceID,
		}
	}
	return edgeInstanceID, client, nil
}

func mainWSErrorFromResolveError(requestID string, commandID string, fallbackEdgeInstanceID string, err error) map[string]any {
	var typed controlEdgeResolveError
	if errors.As(err, &typed) {
		edgeID := firstNonEmpty(typed.EdgeInstanceID, fallbackEdgeInstanceID)
		response := mainWSError(requestID, commandID, edgeID, typed.Code, typed.Message)
		if typed.TargetID != "" {
			response["target_id"] = typed.TargetID
		}
		return response
	}
	return mainWSError(requestID, commandID, fallbackEdgeInstanceID, "internal_error", "control edge lookup failed")
}

func mainWSCommandRoute(messageType string) (string, string, bool) {
	switch strings.TrimSpace(messageType) {
	case "command.detection.start":
		return "api/v1/edge-control/detection/start", auth.PermStartDetection, true
	case "command.detection.stop":
		return "api/v1/edge-control/detection/stop", auth.PermStopDetection, true
	case "command.detection.abnormal_stop":
		return "api/v1/edge-control/detection/abnormal-stop", auth.PermStopDetection, true
	case "command.detection.pause":
		return "api/v1/edge-control/detection/pause", auth.PermStopDetection, true
	case "command.detection.resume":
		return "api/v1/edge-control/detection/resume", auth.PermStartDetection, true
	case "command.detection.apply_config":
		return "api/v1/edge-control/detection/apply-config", auth.PermStartDetection, true
	case "command.write_variable":
		return "api/v1/edge-control/variables/write", auth.PermKIOWrite, true
	default:
		return "", "", false
	}
}

func mainWSError(requestID string, commandID string, edgeInstanceID string, code string, message string) map[string]any {
	return map[string]any{
		"type":             "error",
		"request_id":       requestID,
		"command_id":       commandID,
		"edge_instance_id": edgeInstanceID,
		"at":               time.Now(),
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
}

func mainWSErrorFromBody(body any) (string, string, any) {
	object, ok := body.(map[string]any)
	if !ok {
		return "edge_command_failed", "edge command failed", nil
	}
	code := "edge_command_failed"
	if value, ok := object["code"].(string); ok && strings.TrimSpace(value) != "" {
		code = strings.TrimSpace(value)
	}
	message := "edge command failed"
	if value, ok := object["message"].(string); ok && strings.TrimSpace(value) != "" {
		message = strings.TrimSpace(value)
	} else if value, ok := object["error"].(string); ok && strings.TrimSpace(value) != "" {
		message = strings.TrimSpace(value)
	} else if errorObject, ok := object["error"].(map[string]any); ok {
		if value, ok := errorObject["message"].(string); ok && strings.TrimSpace(value) != "" {
			message = strings.TrimSpace(value)
		}
		if value, ok := errorObject["code"].(string); ok && strings.TrimSpace(value) != "" {
			code = strings.TrimSpace(value)
		}
	}
	return code, message, object["result"]
}

func mainWSEdgeForwardCode(err error) string {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		return "edge_control_disabled"
	case errors.Is(err, edgecontrol.ErrMissingToken):
		return "edge_control_token_missing"
	default:
		return "edge_backend_unavailable"
	}
}

func mainWSEdgeForwardMessage(err error) string {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		return "edge control is disabled"
	case errors.Is(err, edgecontrol.ErrMissingToken):
		return "edge control service token is missing"
	default:
		return "edge backend unavailable"
	}
}
