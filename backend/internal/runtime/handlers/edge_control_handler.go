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
	"gorm.io/gorm"
)

type EdgeControlHandler struct {
	repo      *database.Repository
	detection *services.DetectionRunsService
	variables *services.VariableWriteService
	now       func() time.Time
}

type edgeControlEnvelope struct {
	CommandID        string          `json:"command_id"`
	OperatorID       string          `json:"operator_id"`
	OperatorName     string          `json:"operator_name"`
	OperatorUsername string          `json:"operator_username"`
	Reason           string          `json:"reason"`
	Payload          json.RawMessage `json:"payload"`
}

type edgeControlErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func NewEdgeControlHandler(repo *database.Repository, detection *services.DetectionRunsService, variables *services.VariableWriteService) *EdgeControlHandler {
	return &EdgeControlHandler{repo: repo, detection: detection, variables: variables, now: time.Now}
}

func (h *EdgeControlHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	control := group.Group("/edge-control")
	control.POST("/detection/start", authService.RequireServiceScope(auth.ScopeEdgeDetectionStart), h.handle("detection.start", "project", h.startDetection))
	control.POST("/detection/stop", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.stop", "task", h.stopDetection(false)))
	control.POST("/detection/abnormal-stop", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.abnormal_stop", "task", h.stopDetection(true)))
	control.POST("/detection/pause", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.pause", "task", h.pauseDetection))
	control.POST("/detection/resume", authService.RequireServiceScope(auth.ScopeEdgeDetectionStart), h.handle("detection.resume", "task", h.resumeDetection))
	control.POST("/detection/mute-alarms", authService.RequireServiceScope(auth.ScopeEdgeAlarmMute), h.handle("detection.mute_alarms", "task", h.muteDetectionAlarms))
	control.POST("/detection/update-limits", authService.RequireServiceScope(auth.ScopeEdgeLimitUpdate), h.handle("detection.update_limits", "task", h.updateDetectionLimits))
	control.POST("/detection/refresh-features", authService.RequireServiceScope(auth.ScopeEdgeFeatureRefresh), h.handle("detection.refresh_features", "task", h.refreshDetectionFeatures))
	control.POST("/detection/report-requests", authService.RequireServiceScope(auth.ScopeEdgeReportRequest), h.handle("detection.report_request", "task", h.createReportRequests))
	control.POST("/variables/write", authService.RequireServiceScope(auth.ScopeEdgeVariableWrite), h.handle("variable.write", "variable", h.writeVariable))
}

func (h *EdgeControlHandler) handle(action string, targetType string, execute func(*gin.Context, edgeControlEnvelope) (any, string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		envelope, ok := h.readEnvelope(c)
		if !ok {
			return
		}
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "service" {
			h.writeError(c, http.StatusUnauthorized, envelope.CommandID, "failed", "unauthorized", "service token required", false)
			return
		}
		edgeUser, ok := h.resolveOperator(c, envelope)
		if !ok {
			return
		}

		requestJSON := marshalJSON(map[string]any{
			"command_id":        envelope.CommandID,
			"operator_id":       envelope.OperatorID,
			"operator_name":     envelope.OperatorName,
			"operator_username": envelope.OperatorUsername,
			"reason":            envelope.Reason,
			"payload":           rawPayloadObject(envelope.Payload),
		})
		command := models.EdgeControlCommand{
			CommandID:        envelope.CommandID,
			ClientID:         principal.ClientID,
			OperatorID:       strings.TrimSpace(envelope.OperatorID),
			OperatorName:     strings.TrimSpace(envelope.OperatorName),
			OperatorUsername: strings.TrimSpace(envelope.OperatorUsername),
			EdgeUserID:       edgeUser.ID,
			Action:           action,
			TargetType:       targetType,
			RequestJSON:      requestJSON,
			Status:           "received",
			ReceivedAt:       h.now(),
		}
		if err := h.repo.CreateEdgeControlCommand(&command); err != nil {
			existing, findErr := h.repo.FindEdgeControlCommand(principal.ClientID, envelope.CommandID)
			if findErr != nil {
				h.writeError(c, http.StatusInternalServerError, envelope.CommandID, "failed", "internal_error", "failed to create command", true)
				return
			}
			h.writeExisting(c, existing)
			return
		}
		if err := h.repo.MarkEdgeControlCommandRunning(command.ID); err != nil {
			h.writeError(c, http.StatusInternalServerError, envelope.CommandID, "failed", "internal_error", "failed to mark command running", true)
			return
		}

		started := h.now()
		result, targetID, err := execute(c, envelope)
		if targetID != "" {
			command.TargetID = targetID
		}
		if err != nil {
			code, retryable, status := edgeControlErrorMeta(err)
			message := err.Error()
			resultJSON := marshalJSON(map[string]any{"error": message})
			_ = h.repo.CompleteEdgeControlCommand(command.ID, "failed", targetID, resultJSON, code, message)
			h.audit(principal, edgeUser, command, "failed", message, started)
			h.writeError(c, status, envelope.CommandID, "failed", code, message, retryable)
			return
		}

		resultJSON := marshalJSON(result)
		_ = h.repo.CompleteEdgeControlCommand(command.ID, "success", targetID, resultJSON, "", "")
		h.audit(principal, edgeUser, command, "success", "", started)
		c.JSON(http.StatusOK, gin.H{
			"ok":         true,
			"command_id": envelope.CommandID,
			"status":     "success",
			"result":     result,
		})
	}
}

func (h *EdgeControlHandler) readEnvelope(c *gin.Context) (edgeControlEnvelope, bool) {
	var envelope edgeControlEnvelope
	if err := c.ShouldBindJSON(&envelope); err != nil {
		h.writeError(c, http.StatusBadRequest, "", "failed", "invalid_payload", "request body is invalid", false)
		return envelope, false
	}
	envelope.CommandID = strings.TrimSpace(envelope.CommandID)
	headerCommandID := strings.TrimSpace(c.GetHeader("X-Command-ID"))
	if envelope.CommandID == "" {
		envelope.CommandID = headerCommandID
	}
	if envelope.CommandID == "" {
		h.writeError(c, http.StatusBadRequest, "", "failed", "invalid_payload", "command_id is required", false)
		return envelope, false
	}
	if headerCommandID != "" && headerCommandID != envelope.CommandID {
		h.writeError(c, http.StatusBadRequest, envelope.CommandID, "failed", "invalid_payload", "X-Command-ID and body command_id must match", false)
		return envelope, false
	}
	if len(envelope.Payload) == 0 {
		envelope.Payload = json.RawMessage(`{}`)
	}
	return envelope, true
}

func (h *EdgeControlHandler) resolveOperator(c *gin.Context, envelope edgeControlEnvelope) (models.SysUser, bool) {
	username := strings.TrimSpace(envelope.OperatorUsername)
	if username == "" {
		h.writeError(c, http.StatusBadRequest, envelope.CommandID, "failed", "invalid_payload", "operator_username is required", false)
		return models.SysUser{}, false
	}
	user, err := h.repo.FindUserByUsername(username)
	if err != nil || !user.Enabled {
		h.writeError(c, http.StatusForbidden, envelope.CommandID, "failed", "operator_not_found", "operator is not mapped to an enabled edge user", false)
		return models.SysUser{}, false
	}
	return user, true
}

func (h *EdgeControlHandler) startDetection(c *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req startDetectionRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.ProjectID == 0 || strings.TrimSpace(req.TestNo) == "" || strings.TrimSpace(req.Mode) == "" {
		return nil, "", edgeControlRequestError("invalid_payload", "project_id, test_no, and mode are required", false, http.StatusBadRequest)
	}
	edgeUser, _ := h.resolveOperator(c, envelope)
	operatorNote := req.OperatorNote
	if operatorNote == "" {
		operatorNote = envelope.Reason
	}
	task, err := h.detection.Start(database.StartDetectionOptions{
		ProjectID:        req.ProjectID,
		TestNo:           req.TestNo,
		Mode:             req.Mode,
		StandardID:       req.StandardID,
		DurationSec:      req.DurationSec,
		OperatorNote:     operatorNote,
		ReportTemplateID: req.ReportTemplateID,
		ReportRequest:    req.ReportRequest,
		StartedByUserID:  edgeUser.ID,
	})
	if err != nil {
		return nil, "", err
	}
	return task, strconv.FormatUint(uint64(task.ID), 10), nil
}

func (h *EdgeControlHandler) stopDetection(abnormal bool) func(*gin.Context, edgeControlEnvelope) (any, string, error) {
	return func(c *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
		var req struct {
			TaskID uint   `json:"task_id"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(envelope.Payload, &req); err != nil {
			return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
		}
		if req.TaskID == 0 {
			return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
		}
		reason := req.Reason
		if reason == "" {
			reason = envelope.Reason
		}
		var task *models.DetectionTask
		var err error
		if abnormal {
			task, err = h.detection.AbnormalStop(req.TaskID, reason)
		} else {
			task, err = h.detection.Stop(req.TaskID, reason)
		}
		if err != nil {
			return nil, strconv.FormatUint(uint64(req.TaskID), 10), err
		}
		return task, strconv.FormatUint(uint64(req.TaskID), 10), nil
	}
}

func (h *EdgeControlHandler) pauseDetection(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID uint   `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	reason := req.Reason
	if reason == "" {
		reason = envelope.Reason
	}
	task, err := h.detection.Pause(req.TaskID, reason)
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return task, targetID, nil
}

func (h *EdgeControlHandler) resumeDetection(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID uint `json:"task_id"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	task, err := h.detection.Resume(req.TaskID)
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return task, targetID, nil
}

func (h *EdgeControlHandler) writeVariable(c *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	if h.variables == nil {
		return nil, "", edgeControlRequestError("unsupported_command", "variable write service is not available", true, http.StatusBadRequest)
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
		AllowReentrant bool          `json:"allow_reentrant"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	trigger := true
	if req.Trigger != nil {
		trigger = *req.Trigger
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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
		AllowReentrant: req.AllowReentrant,
		RequestID:      envelope.CommandID,
	})
	targetID := result.VarIDText
	if targetID == "" && req.VarID.Int64() != 0 {
		targetID = strconv.FormatInt(req.VarID.Int64(), 10)
	}
	if err != nil {
		return result, targetID, err
	}
	return result, targetID, nil
}

func (h *EdgeControlHandler) muteDetectionAlarms(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID uint `json:"task_id"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	muted, err := h.detection.MuteDetectionAlarms(req.TaskID)
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return gin.H{"task_id": req.TaskID, "muted": muted}, targetID, nil
}

func (h *EdgeControlHandler) updateDetectionLimits(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID uint                        `json:"task_id"`
		Items  []edgeControlLimitItemInput `json:"items"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	if len(req.Items) == 0 {
		return nil, strconv.FormatUint(uint64(req.TaskID), 10), edgeControlRequestError("invalid_payload", "items are required", false, http.StatusBadRequest)
	}
	items := make([]services.UpdateDetectionLimitItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, services.UpdateDetectionLimitItemInput{
			VarID:           item.VarID.Int64(),
			AlarmEnabled:    item.AlarmEnabled,
			CheckEnabled:    item.CheckEnabled,
			StoreEnabled:    item.StoreEnabled,
			CheckOnStart:    item.CheckOnStart,
			CheckCycleMS:    item.CheckCycleMS,
			ViolationHoldMS: item.ViolationHoldMS,
			RecoverHoldMS:   item.RecoverHoldMS,
			LimitLL:         item.LimitLL,
			LimitL:          item.LimitL,
			LimitH:          item.LimitH,
			LimitHH:         item.LimitHH,
			LimitDeadband:   item.LimitDeadband,
		})
	}
	result, err := h.detection.UpdateDetectionLimits(services.UpdateDetectionLimitsInput{TaskID: req.TaskID, Items: items})
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return result, targetID, nil
}

func (h *EdgeControlHandler) refreshDetectionFeatures(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID uint `json:"task_id"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	features, err := h.detection.RefreshFeaturesWithEvent(req.TaskID)
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return gin.H{"task_id": req.TaskID, "feature_count": len(features), "features": features}, targetID, nil
}

func (h *EdgeControlHandler) createReportRequests(_ *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID        uint           `json:"task_id"`
		ReportRequest map[string]any `json:"report_request"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id is required", false, http.StatusBadRequest)
	}
	if req.ReportRequest == nil {
		return nil, strconv.FormatUint(uint64(req.TaskID), 10), edgeControlRequestError("invalid_payload", "report_request is required", false, http.StatusBadRequest)
	}
	requests, err := h.detection.CreateReportRequests(req.TaskID, req.ReportRequest)
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return gin.H{"task_id": req.TaskID, "request_count": len(requests), "requests": requests}, targetID, nil
}

type edgeControlLimitItemInput struct {
	VarID           flexibleInt64 `json:"var_id"`
	AlarmEnabled    *bool         `json:"alarm_enabled"`
	CheckEnabled    *bool         `json:"check_enabled"`
	StoreEnabled    *bool         `json:"store_enabled"`
	CheckOnStart    *bool         `json:"check_on_start"`
	CheckCycleMS    *int          `json:"check_cycle_ms"`
	ViolationHoldMS *int          `json:"violation_hold_ms"`
	RecoverHoldMS   *int          `json:"recover_hold_ms"`
	LimitLL         *float64      `json:"limit_ll"`
	LimitL          *float64      `json:"limit_l"`
	LimitH          *float64      `json:"limit_h"`
	LimitHH         *float64      `json:"limit_hh"`
	LimitDeadband   *float64      `json:"limit_deadband"`
}

func (h *EdgeControlHandler) writeExisting(c *gin.Context, command models.EdgeControlCommand) {
	if command.Status == "success" {
		c.JSON(http.StatusOK, gin.H{
			"ok":         true,
			"command_id": command.CommandID,
			"status":     command.Status,
			"result":     rawPayloadObject(json.RawMessage(command.ResultJSON)),
		})
		return
	}
	if command.Status == "failed" {
		h.writeError(c, http.StatusOK, command.CommandID, "failed", firstNonEmpty(command.ErrorCode, "command_failed"), firstNonEmpty(command.ErrorMessage, "command failed"), false)
		return
	}
	h.writeError(c, http.StatusConflict, command.CommandID, command.Status, "command_running", "command is still running", true)
}

func (h *EdgeControlHandler) writeError(c *gin.Context, status int, commandID string, commandStatus string, code string, message string, retryable bool) {
	c.JSON(status, gin.H{
		"ok":         false,
		"command_id": commandID,
		"status":     commandStatus,
		"error": edgeControlErrorBody{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
}

func (h *EdgeControlHandler) audit(principal auth.Principal, edgeUser models.SysUser, command models.EdgeControlCommand, result string, errText string, started time.Time) {
	detail := map[string]any{
		"command_id":        command.CommandID,
		"client_id":         principal.ClientID,
		"operator_id":       command.OperatorID,
		"operator_username": command.OperatorUsername,
		"operator_name":     command.OperatorName,
		"edge_user_id":      edgeUser.ID,
		"latency_ms":        time.Since(started).Milliseconds(),
	}
	if errText != "" {
		detail["error"] = errText
	}
	_ = h.repo.CreateAuditLog(&models.SysAuditLog{
		ActorType:  "service",
		ActorID:    principal.ClientID,
		Action:     "edge_control." + command.Action,
		TargetType: command.TargetType,
		TargetID:   command.TargetID,
		Result:     result,
		Detail:     marshalJSON(detail),
		CreatedAt:  h.now(),
	})
}

type edgeControlRequestErrorBody struct {
	code      string
	message   string
	retryable bool
	status    int
}

func (e edgeControlRequestErrorBody) Error() string {
	return e.message
}

func edgeControlRequestError(code string, message string, retryable bool, status int) error {
	return edgeControlRequestErrorBody{code: code, message: message, retryable: retryable, status: status}
}

func edgeControlErrorMeta(err error) (string, bool, int) {
	var reqErr edgeControlRequestErrorBody
	if errors.As(err, &reqErr) {
		return reqErr.code, reqErr.retryable, reqErr.status
	}
	if code, ok := services.VariableWriteErrorCode(err); ok {
		retryable := code == "write_service_unavailable"
		return code, retryable, services.HTTPStatusForError(err)
	}
	if errors.Is(err, database.ErrProjectAlreadyRunning) {
		return "detection_conflict", false, http.StatusConflict
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "not_found", false, http.StatusNotFound
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "must be running") {
		return "invalid_state", false, http.StatusConflict
	}
	if strings.Contains(message, "reason is required") || strings.Contains(message, "invalid") || strings.Contains(message, "required") {
		return "invalid_payload", false, http.StatusBadRequest
	}
	if strings.Contains(message, "at least one") {
		return "invalid_payload", false, http.StatusBadRequest
	}
	return "command_failed", true, services.HTTPStatusForError(err)
}

func marshalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func rawPayloadObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}
	}
	return value
}
