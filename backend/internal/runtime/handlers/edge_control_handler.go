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
	repo            *database.Repository
	detection       *services.DetectionRunsService
	detectionPlans  *services.DetectionPlansService
	variables       *services.VariableWriteService
	runtimeSettings *services.RuntimeSettingsService
	notify          *services.NotificationHub
	now             func() time.Time
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

func (h *EdgeControlHandler) WithRuntimeSettings(runtimeSettings *services.RuntimeSettingsService) *EdgeControlHandler {
	h.runtimeSettings = runtimeSettings
	return h
}

func (h *EdgeControlHandler) WithNotificationHub(hub *services.NotificationHub) *EdgeControlHandler {
	h.notify = hub
	return h
}

func (h *EdgeControlHandler) WithDetectionPlans(service *services.DetectionPlansService) *EdgeControlHandler {
	h.detectionPlans = service
	return h
}

type edgeControlAsyncAccepted struct {
	Status   string         `json:"status"`
	TargetID string         `json:"target_id,omitempty"`
	Result   map[string]any `json:"result"`
}

func (h *EdgeControlHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	control := group.Group("/edge-control")
	control.GET("/commands/:command_id", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.commandStatus)
	control.POST("/detection/start", authService.RequireServiceScope(auth.ScopeEdgeDetectionStart), h.handle("detection.start", "project", h.startDetection))
	control.POST("/detection/stop", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.stop", "task", h.stopDetection(false)))
	control.POST("/detection/abnormal-stop", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.abnormal_stop", "task", h.stopDetection(true)))
	control.POST("/detection/pause", authService.RequireServiceScope(auth.ScopeEdgeDetectionStop), h.handle("detection.pause", "task", h.pauseDetection))
	control.POST("/detection/resume", authService.RequireServiceScope(auth.ScopeEdgeDetectionStart), h.handle("detection.resume", "task", h.resumeDetection))
	control.POST("/detection/mute-alarms", authService.RequireServiceScope(auth.ScopeEdgeAlarmMute), h.handle("detection.mute_alarms", "task", h.muteDetectionAlarms))
	control.POST("/detection/update-limits", authService.RequireServiceScope(auth.ScopeEdgeLimitUpdate), h.handle("detection.update_limits", "task", h.updateDetectionLimits))
	control.POST("/detection/apply-config", authService.RequireServiceScope(auth.ScopeEdgeLimitUpdate), h.handle("detection.apply_config", "task", h.applyDetectionConfig))
	control.POST("/detection/refresh-features", authService.RequireServiceScope(auth.ScopeEdgeFeatureRefresh), h.handle("detection.refresh_features", "task", h.refreshDetectionFeatures))
	control.POST("/detection/report-requests", authService.RequireServiceScope(auth.ScopeEdgeReportRequest), h.handle("detection.report_request", "task", h.createReportRequests))
	control.POST("/detection-plans/:id/start", authService.RequireServiceScope(auth.ScopeEdgeDetectionStart), h.handle("detection_plan.start", "plan", h.startDetectionPlan))
	control.POST("/variables/write", authService.RequireServiceScope(auth.ScopeEdgeVariableWrite), h.handle("variable.write", "variable", h.writeVariable))
}

func (h *EdgeControlHandler) commandStatus(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || principal.AuthType != "service" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "service token required", "code": "unauthorized"})
		return
	}
	commandID := strings.TrimSpace(c.Param("command_id"))
	if commandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command_id is required", "code": "invalid_command_id"})
		return
	}
	command, err := h.repo.FindEdgeControlCommand(principal.ClientID, commandID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "command not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "command status query failed", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, edgeControlCommandStatusResponse(command))
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
			resultJSON := marshalJSON(map[string]any{"error": message, "result": result})
			_ = h.repo.CompleteEdgeControlCommand(command.ID, "failed", targetID, resultJSON, code, message)
			h.audit(principal, edgeUser, command, "failed", message, started)
			h.writeErrorWithResult(c, status, envelope.CommandID, "failed", code, message, retryable, result)
			return
		}
		if accepted, ok := result.(edgeControlAsyncAccepted); ok {
			if accepted.TargetID != "" {
				targetID = accepted.TargetID
			}
			_ = h.repo.UpdateEdgeControlCommandResult(command.ID, "running", targetID, marshalJSON(accepted.Result), "", "")
			command.TargetID = targetID
			h.audit(principal, edgeUser, command, "accepted", "", started)
			c.JSON(http.StatusAccepted, gin.H{
				"ok":         true,
				"command_id": envelope.CommandID,
				"status":     accepted.Status,
				"result":     accepted.Result,
			})
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
	if req.ProjectID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "project_id is required", false, http.StatusBadRequest)
	}
	if req.ConfigEnabled != nil && *req.ConfigEnabled && strings.TrimSpace(req.ConfigCode) != "" {
		if ready, detail := h.detectionConfigReady(req); !ready {
			principal, _ := auth.PrincipalFromContext(c)
			edgeUser, _ := h.resolveOperator(c, envelope)
			operatorNote := req.OperatorNote
			if operatorNote == "" {
				operatorNote = envelope.Reason
			}
			opts := database.StartDetectionOptions{
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
				OperatorNote:     operatorNote,
				ReportTemplateID: req.ReportTemplateID,
				ReportRequest:    req.ReportRequest,
				StartedByUserID:  edgeUser.ID,
			}
			h.publishConfigNotification(models.NotificationDetectionConfigWaiting, models.NotificationLevelInfo, req.ProjectID, "detection config is waiting for database sync", detail)
			go h.waitAndStartDetection(principal.ClientID, envelope.CommandID, opts, detail)
			return edgeControlAsyncAccepted{
				Status: "config_waiting",
				Result: map[string]any{
					"status":     "config_waiting",
					"project_id": req.ProjectID,
					"detail":     detail,
				},
			}, strconv.FormatUint(uint64(req.ProjectID), 10), nil
		}
	}
	edgeUser, _ := h.resolveOperator(c, envelope)
	operatorNote := req.OperatorNote
	if operatorNote == "" {
		operatorNote = envelope.Reason
	}
	task, err := h.detection.Start(database.StartDetectionOptions{
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

func (h *EdgeControlHandler) startDetectionPlan(c *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	if h.detectionPlans == nil {
		return nil, "", edgeControlRequestError("detection_plan_service_missing", "detection plan service is not available", true, http.StatusServiceUnavailable)
	}
	planID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || planID == 0 {
		return nil, "", edgeControlRequestError("invalid_plan_id", "plan_id is required", false, http.StatusBadRequest)
	}
	var req struct {
		ProjectID      uint   `json:"project_id"`
		OperatorNote   string `json:"operator_note"`
		RequestVarID   int64  `json:"request_var_id"`
		RequestVarName string `json:"request_var_name"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.ProjectID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "project_id is required", false, http.StatusBadRequest)
	}
	operatorNote := strings.TrimSpace(req.OperatorNote)
	if operatorNote == "" {
		operatorNote = envelope.Reason
	}
	result, err := h.detectionPlans.Start(services.StartDetectionPlanInput{
		PlanID:         uint(planID),
		ProjectID:      req.ProjectID,
		OperatorNote:   operatorNote,
		RequestVarID:   req.RequestVarID,
		RequestVarName: req.RequestVarName,
	})
	if err != nil {
		return nil, strconv.FormatUint(planID, 10), err
	}
	return result, strconv.FormatUint(uint64(result.Plan.ID), 10), nil
}

func (h *EdgeControlHandler) waitAndStartDetection(clientID string, commandID string, opts database.StartDetectionOptions, firstDetail map[string]any) {
	timeout := h.configReadyTimeout()
	interval := h.configReadyInterval()
	deadline := time.Now().Add(timeout)
	attempts := 1
	req := startDetectionRequest{
		ProjectID:        opts.ProjectID,
		TestNo:           opts.TestNo,
		FactoryNo:        opts.FactoryNo,
		CustomerName:     opts.CustomerName,
		DeviceModel:      opts.DeviceModel,
		Mode:             opts.Mode,
		StandardID:       opts.StandardID,
		ConfigEnabled:    opts.ConfigEnabled,
		ConfigCode:       opts.ConfigCode,
		ConfigName:       opts.ConfigName,
		ConfigVersion:    opts.ConfigVersion,
		ConfigHash:       opts.ConfigHash,
		DurationSec:      opts.DurationSec,
		OperatorNote:     opts.OperatorNote,
		ReportTemplateID: opts.ReportTemplateID,
	}
	var lastDetail map[string]any
	for {
		ready, detail := h.detectionConfigReady(req)
		lastDetail = detail
		if ready {
			task, err := h.detection.Start(opts)
			if err != nil {
				_ = h.completeCommandByIdentity(clientID, commandID, "failed", "", map[string]any{"status": "failed", "error": err.Error(), "detail": detail}, edgeControlErrorCode(err), err.Error())
				h.publishConfigNotification(models.NotificationDetectionConfigNotReady, models.NotificationLevelWarning, opts.ProjectID, "detection start failed after config ready", map[string]any{"command_id": commandID, "error": err.Error(), "detail": detail})
				return
			}
			result := map[string]any{"status": "success", "task": task, "attempts": attempts}
			_ = h.completeCommandByIdentity(clientID, commandID, "success", strconv.FormatUint(uint64(task.ID), 10), result, "", "")
			return
		}
		if !time.Now().Add(interval).Before(deadline) {
			break
		}
		attempts++
		time.Sleep(interval)
	}
	result := map[string]any{
		"status":       "config_not_ready",
		"attempts":     attempts,
		"timeout_ms":   timeout.Milliseconds(),
		"first_detail": firstDetail,
		"last_detail":  lastDetail,
	}
	_ = h.completeCommandByIdentity(clientID, commandID, "failed", "", result, "config_not_ready", "detection config is not synchronized to edge")
	h.publishConfigNotification(models.NotificationDetectionConfigNotReady, models.NotificationLevelWarning, opts.ProjectID, "detection config is not ready", map[string]any{"command_id": commandID, "attempts": attempts, "timeout_ms": timeout.Milliseconds(), "detail": lastDetail})
}

func (h *EdgeControlHandler) detectionConfigReady(req startDetectionRequest) (bool, map[string]any) {
	detail := map[string]any{
		"config_code":      strings.TrimSpace(req.ConfigCode),
		"config_name":      strings.TrimSpace(req.ConfigName),
		"expected_version": req.ConfigVersion,
		"expected_hash":    strings.TrimSpace(req.ConfigHash),
	}
	if strings.TrimSpace(req.ConfigCode) == "" || req.ConfigVersion <= 0 || strings.TrimSpace(req.ConfigHash) == "" {
		detail["reason"] = "config_reference_incomplete"
		return false, detail
	}
	standard, err := h.repo.GetDetectionStandardByCode(req.ConfigCode)
	if err != nil {
		detail["reason"] = "config_missing"
		detail["error"] = err.Error()
		return false, detail
	}
	localHash, err := h.repo.ComputeDetectionStandardHash(standard.ID)
	if err != nil {
		detail["reason"] = "config_hash_failed"
		detail["error"] = err.Error()
		return false, detail
	}
	detail["local_version"] = standard.Version
	detail["local_hash"] = localHash
	detail["local_config_hash"] = standard.ConfigHash
	if standard.Version != req.ConfigVersion {
		detail["reason"] = "version_mismatch"
		return false, detail
	}
	if localHash != strings.TrimSpace(req.ConfigHash) {
		detail["reason"] = "hash_mismatch"
		return false, detail
	}
	return true, detail
}

func (h *EdgeControlHandler) configReadyTimeout() time.Duration {
	if h.runtimeSettings == nil {
		return 60 * time.Second
	}
	timeout := h.runtimeSettings.DetectionConfigWaitTimeout()
	if timeout <= 0 {
		return 60 * time.Second
	}
	return timeout
}

func (h *EdgeControlHandler) configReadyInterval() time.Duration {
	if h.runtimeSettings == nil {
		return 5 * time.Second
	}
	interval := h.runtimeSettings.DetectionConfigWaitInterval()
	if interval <= 0 {
		return 5 * time.Second
	}
	return interval
}

func (h *EdgeControlHandler) completeCommandByIdentity(clientID string, commandID string, status string, targetID string, result map[string]any, code string, message string) error {
	command, err := h.repo.FindEdgeControlCommand(clientID, commandID)
	if err != nil {
		return err
	}
	return h.repo.CompleteEdgeControlCommand(command.ID, status, targetID, marshalJSON(result), code, message)
}

func (h *EdgeControlHandler) publishConfigNotification(notificationType string, level string, projectID uint, message string, payload map[string]any) {
	notification := models.NewRuntimeNotification(notificationType, level, message, time.Now())
	notification.TargetType = models.NotificationTargetProject
	notification.TargetID = strconv.FormatUint(uint64(projectID), 10)
	notification.ProjectID = projectID
	notification.Payload = payload
	if _, err := h.repo.CreateRuntimeNotification(notification); err != nil {
		return
	}
	if h.notify != nil {
		h.notify.Publish(notification)
	}
}

func edgeControlErrorCode(err error) string {
	code, _, _ := edgeControlErrorMeta(err)
	return code
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

func (h *EdgeControlHandler) applyDetectionConfig(c *gin.Context, envelope edgeControlEnvelope) (any, string, error) {
	var req struct {
		TaskID        uint   `json:"task_id"`
		ProjectID     uint   `json:"project_id"`
		ConfigCode    string `json:"config_code"`
		ConfigName    string `json:"config_name"`
		ConfigVersion int    `json:"config_version"`
		ConfigHash    string `json:"config_hash"`
	}
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, "", edgeControlRequestError("invalid_payload", "payload is invalid", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 && req.ProjectID == 0 {
		return nil, "", edgeControlRequestError("invalid_payload", "task_id or project_id is required", false, http.StatusBadRequest)
	}
	if req.TaskID == 0 {
		if active, ok := h.detectionActiveForProject(req.ProjectID); ok {
			req.TaskID = active.ID
		}
	}
	if req.TaskID == 0 {
		return nil, "", edgeControlRequestError("invalid_state", "no active detection run for project", false, http.StatusConflict)
	}
	startReq := startDetectionRequest{ProjectID: req.ProjectID, ConfigEnabled: boolPtr(true), ConfigCode: req.ConfigCode, ConfigName: req.ConfigName, ConfigVersion: req.ConfigVersion, ConfigHash: req.ConfigHash}
	if ready, detail := h.detectionConfigReady(startReq); !ready {
		principal, _ := auth.PrincipalFromContext(c)
		h.publishConfigNotification(models.NotificationDetectionConfigWaiting, models.NotificationLevelInfo, req.ProjectID, "detection config apply is waiting for database sync", detail)
		go h.waitAndApplyDetectionConfig(principal.ClientID, envelope.CommandID, services.ApplyDetectionConfigInput{
			TaskID:        req.TaskID,
			ConfigCode:    req.ConfigCode,
			ConfigVersion: req.ConfigVersion,
			ConfigHash:    req.ConfigHash,
		}, req.ProjectID, detail)
		return edgeControlAsyncAccepted{
			Status:   "config_apply_waiting",
			TargetID: strconv.FormatUint(uint64(req.TaskID), 10),
			Result: map[string]any{
				"status":     "config_apply_waiting",
				"task_id":    req.TaskID,
				"project_id": req.ProjectID,
				"detail":     detail,
			},
		}, strconv.FormatUint(uint64(req.TaskID), 10), nil
	}
	result, err := h.detection.ApplyDetectionConfig(services.ApplyDetectionConfigInput{
		TaskID:        req.TaskID,
		ConfigCode:    req.ConfigCode,
		ConfigVersion: req.ConfigVersion,
		ConfigHash:    req.ConfigHash,
	})
	targetID := strconv.FormatUint(uint64(req.TaskID), 10)
	if err != nil {
		return nil, targetID, err
	}
	return result, targetID, nil
}

func (h *EdgeControlHandler) waitAndApplyDetectionConfig(clientID string, commandID string, input services.ApplyDetectionConfigInput, projectID uint, firstDetail map[string]any) {
	timeout := h.configReadyTimeout()
	interval := h.configReadyInterval()
	deadline := time.Now().Add(timeout)
	attempts := 1
	req := startDetectionRequest{ProjectID: projectID, ConfigEnabled: boolPtr(true), ConfigCode: input.ConfigCode, ConfigVersion: input.ConfigVersion, ConfigHash: input.ConfigHash}
	var lastDetail map[string]any
	for {
		ready, detail := h.detectionConfigReady(req)
		lastDetail = detail
		if ready {
			result, err := h.detection.ApplyDetectionConfig(input)
			if err != nil {
				_ = h.completeCommandByIdentity(clientID, commandID, "failed", strconv.FormatUint(uint64(input.TaskID), 10), map[string]any{"status": "failed", "error": err.Error(), "detail": detail}, edgeControlErrorCode(err), err.Error())
				h.publishConfigNotification(models.NotificationDetectionConfigApplyFailed, models.NotificationLevelWarning, projectID, "detection config apply failed", map[string]any{"command_id": commandID, "error": err.Error(), "detail": detail})
				return
			}
			_ = h.completeCommandByIdentity(clientID, commandID, "success", strconv.FormatUint(uint64(input.TaskID), 10), map[string]any{"status": "success", "result": result, "attempts": attempts}, "", "")
			return
		}
		if !time.Now().Add(interval).Before(deadline) {
			break
		}
		attempts++
		time.Sleep(interval)
	}
	result := map[string]any{"status": "config_not_ready", "attempts": attempts, "timeout_ms": timeout.Milliseconds(), "first_detail": firstDetail, "last_detail": lastDetail}
	_ = h.completeCommandByIdentity(clientID, commandID, "failed", strconv.FormatUint(uint64(input.TaskID), 10), result, "config_not_ready", "detection config is not synchronized to edge")
	h.publishConfigNotification(models.NotificationDetectionConfigApplyFailed, models.NotificationLevelWarning, projectID, "detection config apply timed out", map[string]any{"command_id": commandID, "attempts": attempts, "timeout_ms": timeout.Milliseconds(), "detail": lastDetail})
}

func (h *EdgeControlHandler) detectionActiveForProject(projectID uint) (models.DetectionTask, bool) {
	if projectID == 0 {
		return models.DetectionTask{}, false
	}
	task, err := h.detection.Current(projectID)
	return task, err == nil && task.Status == models.DetectionStatusRunning
}

func boolPtr(value bool) *bool {
	return &value
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

func edgeControlCommandStatusResponse(command models.EdgeControlCommand) gin.H {
	return gin.H{
		"command_id":        command.CommandID,
		"client_id":         command.ClientID,
		"operator_id":       command.OperatorID,
		"operator_name":     command.OperatorName,
		"operator_username": command.OperatorUsername,
		"edge_user_id":      command.EdgeUserID,
		"action":            command.Action,
		"target_type":       command.TargetType,
		"target_id":         command.TargetID,
		"status":            command.Status,
		"result":            rawPayloadObject(json.RawMessage(command.ResultJSON)),
		"error_code":        command.ErrorCode,
		"error_message":     command.ErrorMessage,
		"received_at":       command.ReceivedAt,
		"completed_at":      command.CompletedAt,
		"created_at":        command.CreatedAt,
		"updated_at":        command.UpdatedAt,
	}
}

func (h *EdgeControlHandler) writeError(c *gin.Context, status int, commandID string, commandStatus string, code string, message string, retryable bool) {
	h.writeErrorWithResult(c, status, commandID, commandStatus, code, message, retryable, nil)
}

func (h *EdgeControlHandler) writeErrorWithResult(c *gin.Context, status int, commandID string, commandStatus string, code string, message string, retryable bool, result any) {
	body := gin.H{
		"ok":         false,
		"command_id": commandID,
		"status":     commandStatus,
		"error": edgeControlErrorBody{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
	if result != nil {
		body["result"] = result
	}
	c.JSON(status, body)
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
	if errors.Is(err, database.ErrDetectionPlanNotPending) {
		return "detection_plan_not_pending", false, http.StatusConflict
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "not_found", false, http.StatusNotFound
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "config_not_ready") {
		return "config_not_ready", true, http.StatusConflict
	}
	if strings.Contains(message, "config_disabled_for_run") {
		return "config_disabled_for_run", false, http.StatusConflict
	}
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
