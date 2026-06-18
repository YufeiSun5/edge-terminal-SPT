package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

type DetectionPlansService struct {
	repo           *database.Repository
	detection      *DetectionRunsService
	variables      *VariableWriteService
	edgeInstanceID string
}

type StartDetectionPlanInput struct {
	PlanID          uint
	ProjectID       uint
	OperatorNote    string
	RequestVarID    int64
	RequestVarName  string
	WaitTaskTimeout time.Duration
}

type StartDetectionPlanResult struct {
	Plan models.DetectionPlan  `json:"plan"`
	Task *models.DetectionTask `json:"task"`
}

func NewDetectionPlansService(repo *database.Repository, detection *DetectionRunsService, edgeInstanceID string, variableWriters ...*VariableWriteService) *DetectionPlansService {
	var writer *VariableWriteService
	if len(variableWriters) > 0 {
		writer = variableWriters[0]
	}
	return &DetectionPlansService{repo: repo, detection: detection, variables: writer, edgeInstanceID: edgeInstanceID}
}

func (s *DetectionPlansService) List(filter database.DetectionPlanFilter) ([]models.DetectionPlan, int64, error) {
	return s.repo.ListDetectionPlans(filter)
}

func (s *DetectionPlansService) Get(id uint) (models.DetectionPlan, error) {
	return s.repo.GetDetectionPlan(id)
}

func (s *DetectionPlansService) Cancel(id uint, reason string) (models.DetectionPlan, error) {
	return s.repo.CancelDetectionPlan(id, reason)
}

func (s *DetectionPlansService) Start(input StartDetectionPlanInput) (StartDetectionPlanResult, error) {
	if input.PlanID == 0 {
		return StartDetectionPlanResult{}, fmt.Errorf("plan_id is required")
	}
	if input.ProjectID == 0 {
		return StartDetectionPlanResult{}, fmt.Errorf("project_id is required")
	}
	plan, err := s.repo.MarkDetectionPlanStarting(input.PlanID)
	if err != nil {
		return StartDetectionPlanResult{}, err
	}
	resetOnError := func(startErr error) (StartDetectionPlanResult, error) {
		_ = s.repo.ResetDetectionPlanPending(plan.ID, startErr.Error())
		return StartDetectionPlanResult{}, startErr
	}
	if strings.TrimSpace(plan.FactoryNo) == "" {
		return resetOnError(fmt.Errorf("factory_no is required"))
	}
	if strings.TrimSpace(plan.StandardCode) == "" {
		return resetOnError(fmt.Errorf("standard_code is required"))
	}
	project, err := s.repo.GetProject(input.ProjectID)
	if err != nil {
		return resetOnError(err)
	}
	standard, err := s.repo.GetDetectionStandardByCode(plan.StandardCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resetOnError(fmt.Errorf("config_not_ready"))
		}
		return resetOnError(err)
	}
	if !standard.Enabled {
		return resetOnError(fmt.Errorf("detection standard is disabled"))
	}
	if strings.TrimSpace(standard.ConfigHash) == "" {
		hash, hashErr := s.repo.ComputeDetectionStandardHash(standard.ID)
		if hashErr != nil {
			return resetOnError(hashErr)
		}
		standard.ConfigHash = hash
	}
	configEnabled := true
	mode := strings.TrimSpace(plan.Mode)
	if mode == "" {
		mode = "standard"
	}
	operatorNote := strings.TrimSpace(input.OperatorNote)
	if operatorNote == "" {
		operatorNote = fmt.Sprintf("started from detection plan %s", plan.PlanNo)
	}
	endPolicy, durationSec, qualifiedHoldMS, err := detectionPlanRunEndOptions(plan.ReportRequestJSON)
	if err != nil {
		return resetOnError(err)
	}
	if s.variables == nil {
		return resetOnError(fmt.Errorf("task_request_variable_writer_not_configured"))
	}
	taskRequest, err := s.buildPlanTaskRequest(plan, standard, input.ProjectID, mode, configEnabled, endPolicy, durationSec, qualifiedHoldMS, operatorNote)
	if err != nil {
		return resetOnError(err)
	}
	taskRequestJSON, err := json.Marshal(taskRequest)
	if err != nil {
		return resetOnError(err)
	}
	requestVarName := strings.TrimSpace(input.RequestVarName)
	if input.RequestVarID == 0 && requestVarName == "" {
		requestVarName = "task_request"
	}
	writeResult, err := s.variables.Write(context.Background(), VariableWriteInput{
		VarID:     input.RequestVarID,
		ProjectID: input.ProjectID,
		VarName:   requestVarName,
		Value:     string(taskRequestJSON),
		Trigger:   true,
		MaxDepth:  3,
		RequestID: fmt.Sprintf("plan-start-%d-%d", plan.ID, time.Now().UnixNano()),
	})
	if err != nil {
		return resetOnError(err)
	}
	if writeResult.Triggered <= 0 {
		return resetOnError(fmt.Errorf("task_request_flow_not_triggered"))
	}
	task, err := s.waitPlanTaskCreated(input.ProjectID, strings.TrimSpace(plan.PlanNo), input.WaitTaskTimeout)
	if err != nil {
		return resetOnError(err)
	}
	updated, err := s.repo.MarkDetectionPlanStarted(database.DetectionPlanStartedUpdate{
		PlanID:              plan.ID,
		TaskID:              task.ID,
		OwnerEdgeInstanceID: firstNonEmpty(s.edgeInstanceID, task.EdgeInstanceID, project.EdgeInstanceID),
		OwnerProjectID:      project.ID,
		OwnerProjectCode:    project.ProjectCode,
	})
	if err != nil {
		return StartDetectionPlanResult{}, err
	}
	return StartDetectionPlanResult{Plan: updated, Task: task}, nil
}

func (s *DetectionPlansService) buildPlanTaskRequest(plan models.DetectionPlan, standard models.DetectionStandard, projectID uint, mode string, configEnabled bool, endPolicy string, durationSec int, qualifiedHoldMS int, operatorNote string) (map[string]any, error) {
	params, err := detectionPlanReportParams(plan.ReportRequestJSON)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"command":        "start_detection",
		"project_id":     projectID,
		"test_no":        strings.TrimSpace(plan.PlanNo),
		"factory_no":     strings.TrimSpace(plan.FactoryNo),
		"customer_name":  strings.TrimSpace(plan.CustomerName),
		"device_model":   strings.TrimSpace(plan.DeviceModel),
		"mode":           mode,
		"standard_id":    standard.ID,
		"config_enabled": configEnabled,
		"config_code":    standard.StandardCode,
		"config_name":    firstNonEmpty(standard.DisplayName, standard.Name, standard.StandardCode),
		"config_version": standard.Version,
		"config_hash":    standard.ConfigHash,
		"operator_note":  operatorNote,
		"report_request": strings.TrimSpace(plan.ReportRequestJSON),
		"enable_storage": true,
		"enable_alarm":   true,
	}
	if len(params) > 0 {
		request["process_params"] = params
	}
	if endPolicy != "" {
		request["end_policy"] = endPolicy
	}
	if durationSec > 0 {
		request["duration_sec"] = durationSec
	}
	if qualifiedHoldMS > 0 {
		request["qualified_hold_ms"] = qualifiedHoldMS
	}
	return request, nil
}

func (s *DetectionPlansService) waitPlanTaskCreated(projectID uint, testNo string, timeout time.Duration) (*models.DetectionTask, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		task, err := s.repo.FindDetectionTaskByProjectAndTestNo(projectID, testNo)
		if err == nil {
			return &task, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if errors.Is(lastErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("detection_task_not_created_by_task_flow")
	}
	return nil, lastErr
}

func detectionPlanRunEndOptions(raw string) (string, int, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, 0, nil
	}
	params, err := detectionPlanReportParams(raw)
	if err != nil {
		return "", 0, 0, fmt.Errorf("report_request_json is invalid: %w", err)
	}
	endPolicy := strings.TrimSpace(paramString(params, "end_policy", "task.end_policy", "detection_end_policy"))
	durationSec := paramInt(params, "duration_sec", "task.duration_sec", "fixed_duration_sec")
	qualifiedHoldMS := paramInt(params, "qualified_hold_ms", "task.qualified_hold_ms", "qualified_hold_millis")
	if qualifiedHoldMS <= 0 {
		if seconds := paramInt(params, "qualified_hold_sec", "qualified_hold_seconds", "task.qualified_hold_sec"); seconds > 0 {
			qualifiedHoldMS = seconds * 1000
		}
	}
	if qualifiedHoldMS <= 0 {
		if minutes := paramInt(params, "qualified_hold_min", "qualified_hold_minutes", "task.qualified_hold_minutes"); minutes > 0 {
			qualifiedHoldMS = minutes * 60 * 1000
		}
	}
	if endPolicy == "" {
		switch {
		case qualifiedHoldMS > 0:
			endPolicy = models.DetectionEndPolicyQualifiedHold
		case durationSec > 0:
			endPolicy = models.DetectionEndPolicyFixedDuration
		}
	}
	if endPolicy == models.DetectionEndPolicyQualifiedHold && qualifiedHoldMS <= 0 {
		return "", 0, 0, fmt.Errorf("qualified_hold_ms is required for qualified_hold")
	}
	if endPolicy == models.DetectionEndPolicyFixedDuration && durationSec <= 0 {
		return "", 0, 0, fmt.Errorf("duration_sec is required for fixed_duration")
	}
	return endPolicy, durationSec, qualifiedHoldMS, nil
}

func detectionPlanReportParams(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var spec struct {
		Params  map[string]any `json:"params"`
		Reports []struct {
			Params map[string]any `json:"params"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, err
	}
	params := map[string]any{}
	for key, value := range spec.Params {
		params[strings.TrimSpace(key)] = value
	}
	for _, report := range spec.Reports {
		for key, value := range report.Params {
			params[strings.TrimSpace(key)] = value
		}
	}
	return params, nil
}

func paramString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		}
	}
	return ""
}

func paramInt(params map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed
			}
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil {
				return int(parsed)
			}
		case float64:
			return int(typed)
		case int:
			return typed
		}
	}
	return 0
}
