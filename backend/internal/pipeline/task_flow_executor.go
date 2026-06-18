package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"

	"github.com/dop251/goja"
)

type TaskFlowExecutor struct {
	repo             *database.Repository
	tags             *TagManager
	tasks            *TaskManager
	channels         *Channels
	variableWriter   TaskFlowVariableWriter
	index            *TaskFlowIndex
	input            chan taskFlowJob
	submitted        atomic.Uint64
	enqueued         atomic.Uint64
	dropped          atomic.Uint64
	guardsMu         sync.Mutex
	guards           map[string]struct{}
	schedulerMu      sync.Mutex
	schedulerStarted bool
}

type taskFlowJob struct {
	flow  models.TaskFlow
	event TaskFlowEvent
}

type taskFlowResult struct {
	Status string
	Result map[string]any
	Logs   []string
	Err    error
}

type taskFlowStep struct {
	Code   string         `json:"code"`
	Module string         `json:"module"`
	Params map[string]any `json:"params"`
	Script string         `json:"script"`
}

type taskFlowRunContext struct {
	Flow   models.TaskFlow
	Event  TaskFlowEvent
	RunID  uint64
	Params map[string]any
	Values map[string]any
	Steps  []map[string]any
}

type TaskFlowVariableWriter func(context.Context, TaskFlowVariableWriteInput) (map[string]any, error)

type TaskFlowVariableWriteInput struct {
	VarID          int64
	Value          any
	Quality        int
	Trigger        bool
	WaitAck        bool
	AckTimeoutSec  int
	OriginFlowID   uint64
	OriginRunID    uint64
	Depth          int
	MaxDepth       int
	AllowReentrant bool
	RequestID      string
}

const taskFlowStringParamMaxBytes = 256 * 1024

type TaskFlowRuntimeStats struct {
	Queue             ChannelPressure `json:"queue"`
	Guards            int             `json:"guards"`
	Submitted         uint64          `json:"submitted"`
	Enqueued          uint64          `json:"enqueued"`
	Dropped           uint64          `json:"dropped"`
	PressureThreshold float64         `json:"pressure_threshold"`
	Pressure          bool            `json:"pressure"`
}

func NewTaskFlowExecutor(repo *database.Repository, tags *TagManager, tasks *TaskManager, channels *Channels) *TaskFlowExecutor {
	return &TaskFlowExecutor{
		repo:     repo,
		tags:     tags,
		tasks:    tasks,
		channels: channels,
		index:    NewTaskFlowIndex(nil),
		input:    make(chan taskFlowJob, 1000),
		guards:   make(map[string]struct{}),
	}
}

func (e *TaskFlowExecutor) SetVariableWriter(writer TaskFlowVariableWriter) {
	e.variableWriter = writer
}

func (e *TaskFlowExecutor) RuntimeStats(threshold float64) TaskFlowRuntimeStats {
	if threshold <= 0 {
		threshold = 0.8
	}
	stats := TaskFlowRuntimeStats{
		Queue:             channelPressure("task_flow", 0, 0),
		PressureThreshold: threshold,
	}
	if e == nil {
		return stats
	}
	stats.Queue = DiagnoseChannelPressure(channelPressure("task_flow", len(e.input), cap(e.input)), threshold)
	stats.Pressure = stats.Queue.Pressure
	stats.Submitted = e.submitted.Load()
	stats.Enqueued = e.enqueued.Load()
	stats.Dropped = e.dropped.Load()
	e.guardsMu.Lock()
	stats.Guards = len(e.guards)
	e.guardsMu.Unlock()
	return stats
}

func (e *TaskFlowExecutor) Load(flows []models.TaskFlow) {
	e.index.Load(flows)
}

func (e *TaskFlowExecutor) Start(workerCount int) {
	if workerCount <= 0 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		workerID := i
		GoRecovering("task-flow-worker", func() {
			e.worker(workerID)
		})
	}
	log.Printf("task flow executor started: workers=%d", workerCount)
}

func (e *TaskFlowExecutor) StartScheduleScanner(interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	e.schedulerMu.Lock()
	if e.schedulerStarted {
		e.schedulerMu.Unlock()
		return
	}
	e.schedulerStarted = true
	e.schedulerMu.Unlock()

	GoRecovering("task-flow-schedule-scanner", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Printf("task flow schedule scanner started: interval=%s", interval)
		for now := range ticker.C {
			e.Trigger(TaskFlowEvent{
				TriggerType: models.TaskFlowTriggerSchedule,
				At:          now,
			})
		}
	})
}

func (e *TaskFlowExecutor) RecoverActiveDetectionGuards(tasks []models.DetectionTask) {
	recovered := 0
	for _, task := range tasks {
		if task.Status != models.DetectionStatusRunning {
			continue
		}
		switch task.EndPolicy {
		case models.DetectionEndPolicyFixedDuration:
			if task.ExpectedEndAt != nil {
				e.startFixedDurationGuard(task.ID)
				recovered++
			}
		case models.DetectionEndPolicyQualifiedHold:
			if task.QualifiedHoldMS > 0 {
				if e.startQualifiedHoldGuard(task.ID, time.Duration(task.QualifiedHoldMS)*time.Millisecond, 500*time.Millisecond) {
					recovered++
				}
			}
		}
	}
	if recovered > 0 {
		log.Printf("recovered active detection guards: %d", recovered)
	}
}

func (e *TaskFlowExecutor) Trigger(event TaskFlowEvent) int {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	matches := e.index.Match(event)
	for _, flow := range matches {
		if !e.submitJob(taskFlowJob{flow: flow, event: event}) {
			log.Printf("[task-flow] queue full, drop flow=%s trigger_var_id=%d", flow.FlowCode, event.TriggerVarID)
		}
	}
	return len(matches)
}

func (e *TaskFlowExecutor) Submit(flow models.TaskFlow, event TaskFlowEvent) bool {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if e.submitJob(taskFlowJob{flow: flow, event: event}) {
		return true
	}
	if e != nil {
		log.Printf("[task-flow] queue full, drop flow=%s", flow.FlowCode)
	}
	return false
}

func (e *TaskFlowExecutor) submitJob(job taskFlowJob) bool {
	if e == nil {
		return false
	}
	e.submitted.Add(1)
	select {
	case e.input <- job:
		e.enqueued.Add(1)
		return true
	default:
		e.dropped.Add(1)
		return false
	}
}

func (e *TaskFlowExecutor) worker(workerID int) {
	for first := range e.input {
		batch := []taskFlowJob{first}
		for len(batch) < 64 {
			select {
			case job, ok := <-e.input:
				if !ok {
					goto drained
				}
				batch = append(batch, job)
			default:
				goto drained
			}
		}
	drained:
		sort.SliceStable(batch, func(i, j int) bool {
			if batch[i].flow.Priority == batch[j].flow.Priority {
				return batch[i].flow.ID < batch[j].flow.ID
			}
			return batch[i].flow.Priority > batch[j].flow.Priority
		})
		for _, job := range batch {
			e.execute(workerID, job.flow, job.event)
		}
	}
}

func (e *TaskFlowExecutor) execute(workerID int, flow models.TaskFlow, event TaskFlowEvent) {
	startedAt := time.Now()
	snapshot := e.inputSnapshot(flow, event)
	run := models.TaskFlowRun{
		FlowID:         flow.ID,
		FlowCode:       flow.FlowCode,
		ProjectID:      flow.ProjectID,
		EdgeInstanceID: flow.EdgeInstanceID,
		TriggerType:    event.TriggerType,
		TriggerVarID:   event.TriggerVarID,
		OriginFlowID:   event.OriginFlowID,
		OriginRunID:    event.OriginRunID,
		Depth:          event.Depth,
		Status:         models.TaskFlowStatusRunning,
		StartedAt:      startedAt,
		InputSnapshot:  snapshot,
	}
	if err := e.repo.CreateTaskFlowRun(&run); err != nil {
		log.Printf("[task-flow-%d] create run failed flow=%s err=%v", workerID, flow.FlowCode, err)
		return
	}

	result := e.runFlow(flow, event, run.ID)
	status := result.Status
	if status == "" {
		status = models.TaskFlowStatusSuccess
	}
	errMessage := ""
	if result.Err != nil {
		errMessage = result.Err.Error()
		if status == models.TaskFlowStatusSuccess {
			status = models.TaskFlowStatusFailed
		}
	}
	resultJSON, _ := json.Marshal(result.Result)
	logsJSON, _ := json.Marshal(result.Logs)
	if err := e.repo.FinishTaskFlowRun(run.ID, status, string(resultJSON), errMessage, string(logsJSON), time.Now()); err != nil {
		log.Printf("[task-flow-%d] finish run failed flow=%s run_id=%d err=%v", workerID, flow.FlowCode, run.ID, err)
	}
}

func (e *TaskFlowExecutor) runFlow(flow models.TaskFlow, event TaskFlowEvent, runID uint64) taskFlowResult {
	logs := make([]string, 0)
	ctx := newTaskFlowRunContext(flow, event, runID)
	if ok, err := e.evaluateCondition(ctx, &logs); err != nil {
		return taskFlowResult{Status: models.TaskFlowStatusFailed, Logs: logs, Err: err}
	} else if !ok {
		return taskFlowResult{Status: models.TaskFlowStatusSkipped, Logs: logs, Result: map[string]any{"skipped": true}}
	}

	steps, err := taskFlowSteps(flow)
	if err != nil {
		return taskFlowResult{Status: models.TaskFlowStatusFailed, Logs: logs, Err: err}
	}
	for index, step := range steps {
		result, err := e.runStep(ctx, step, &logs)
		stepResult := map[string]any{
			"index":  index,
			"code":   step.Code,
			"module": step.Module,
			"result": result,
		}
		if err != nil {
			stepResult["error"] = err.Error()
			ctx.Steps = append(ctx.Steps, stepResult)
			return taskFlowResult{
				Status: statusForErr(err),
				Result: map[string]any{"steps": ctx.Steps, "context": ctx.Values},
				Logs:   logs,
				Err:    err,
			}
		}
		ctx.Steps = append(ctx.Steps, stepResult)
	}
	return taskFlowResult{Status: models.TaskFlowStatusSuccess, Result: map[string]any{"steps": ctx.Steps, "context": ctx.Values}, Logs: logs}
}

func statusForErr(err error) string {
	if err == nil {
		return models.TaskFlowStatusSuccess
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return models.TaskFlowStatusTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return models.TaskFlowStatusTimeout
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case message == "timeout":
		return models.TaskFlowStatusTimeout
	case strings.HasPrefix(message, "timeout at "):
		return models.TaskFlowStatusTimeout
	case strings.Contains(message, "context deadline exceeded"):
		return models.TaskFlowStatusTimeout
	case strings.Contains(message, "execution terminated: timeout"):
		return models.TaskFlowStatusTimeout
	}
	return models.TaskFlowStatusFailed
}

func (e *TaskFlowExecutor) evaluateCondition(ctx *taskFlowRunContext, logs *[]string) (bool, error) {
	if strings.TrimSpace(ctx.Flow.ConditionScript) == "" {
		return true, nil
	}
	result, err := e.runJavaScript(ctx, ctx.Flow.ConditionScript, ctx.Params, logs)
	if err != nil {
		return false, err
	}
	if value, ok := result["value"].(bool); ok {
		return value, nil
	}
	if value, ok := result["result"].(bool); ok {
		return value, nil
	}
	return false, nil
}

func (e *TaskFlowExecutor) runJavaScript(runCtx *taskFlowRunContext, source string, params map[string]any, logs *[]string) (map[string]any, error) {
	timeout := time.Duration(runCtx.Flow.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vm := goja.New()
	timer := time.AfterFunc(timeout, func() {
		vm.Interrupt("timeout")
	})
	defer timer.Stop()

	e.bindJavaScriptAPI(ctx, vm, runCtx, params, logs)
	value, err := vm.RunString(source)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	exported := value.Export()
	if result, ok := exported.(map[string]any); ok {
		return result, nil
	}
	return map[string]any{"value": exported}, nil
}

func (e *TaskFlowExecutor) bindJavaScriptAPI(ctx context.Context, vm *goja.Runtime, runCtx *taskFlowRunContext, params map[string]any, logs *[]string) {
	flow := runCtx.Flow
	event := runCtx.Event
	defaultProjectID := firstNonZeroUint(event.ProjectID, flow.ProjectID)
	_ = vm.Set("project", map[string]any{"id": flow.ProjectID})
	_ = vm.Set("trigger", map[string]any{"type": event.TriggerType, "var_id": event.TriggerVarID, "var_id_text": fmt.Sprintf("%d", event.TriggerVarID), "value": event.TriggerValue, "topic": event.Topic})
	_ = vm.Set("task_params", params)
	_ = vm.Set("params", params)
	_ = vm.Set("context", runCtx.Values)
	_ = vm.Set("log", map[string]any{
		"info": func(message any) {
			*logs = append(*logs, fmt.Sprint(message))
		},
		"warn": func(message any) {
			*logs = append(*logs, "WARN "+fmt.Sprint(message))
		},
		"error": func(message any) {
			*logs = append(*logs, "ERROR "+fmt.Sprint(message))
		},
	})
	_ = vm.Set("realtime", map[string]any{
		"get": func(varID any) map[string]any {
			tag, ok := e.tags.Get(toInt64(varID))
			if !ok {
				return nil
			}
			return snapshotMap(tag.Snapshot())
		},
		"getMany": func(varIDs []any) []map[string]any {
			out := make([]map[string]any, 0, len(varIDs))
			for _, raw := range varIDs {
				tag, ok := e.tags.Get(toInt64(raw))
				if !ok {
					continue
				}
				out = append(out, snapshotMap(tag.Snapshot()))
			}
			return out
		},
		"getByName": func(name string, projectIDs ...uint) map[string]any {
			projectID := defaultProjectID
			if len(projectIDs) > 0 {
				projectID = projectIDs[0]
			}
			for _, tag := range e.realtimeTagsForScript(projectID) {
				if tag.Config.VarName == name {
					return snapshotMap(tag.Snapshot())
				}
			}
			return nil
		},
		"project": func(projectIDs ...uint) []map[string]any {
			projectID := defaultProjectID
			if len(projectIDs) > 0 {
				projectID = projectIDs[0]
			}
			tags := e.realtimeTagsForScript(projectID)
			out := make([]map[string]any, 0, len(tags))
			for _, tag := range tags {
				out = append(out, snapshotMap(tag.Snapshot()))
			}
			return out
		},
		"write": func(varID any, value any, options ...map[string]any) map[string]any {
			writeParams := map[string]any{"var_id": varID, "value": value, "trigger": false}
			if len(options) > 0 {
				for key, item := range options[0] {
					writeParams[key] = item
				}
			}
			result, err := e.writeVariableFromTaskFlow(runCtx, writeParams)
			if err != nil {
				return map[string]any{"ok": false, "error": err.Error()}
			}
			result["ok"] = true
			return result
		},
	})
	_ = vm.Set("storage", map[string]any{
		"snapshot": func(input map[string]any) map[string]any {
			projectID := flow.ProjectID
			if raw, ok := input["project_id"]; ok {
				projectID = uint(toFloat64(raw))
			}
			count, err := e.storageSnapshot(projectID, event, models.StoreTriggerOnDetection)
			out := map[string]any{"stored": count}
			if err != nil {
				out["error"] = err.Error()
			}
			return out
		},
	})
	_ = vm.Set("db", map[string]any{
		"exec": func(sqlText string, args []any) map[string]any {
			affected, err := e.repo.ExecTaskFlowSQL(ctx, runCtx.RunID, flow.ID, sqlText, args)
			out := map[string]any{"affected_rows": affected}
			if err != nil {
				out["error"] = err.Error()
			}
			return out
		},
		"query": func(sqlText string, args []any) []map[string]any {
			rows, err := e.repo.QueryTaskFlowSQL(ctx, runCtx.RunID, flow.ID, sqlText, args, 1000)
			if err != nil {
				*logs = append(*logs, "SQL ERROR "+err.Error())
				return nil
			}
			return rows
		},
	})
}

func (e *TaskFlowExecutor) realtimeTagsForScript(projectID uint) []*models.Tag {
	if projectID > 0 {
		return e.tags.ForProject(projectID)
	}
	return e.tags.All()
}

func (e *TaskFlowExecutor) storageSnapshot(projectID uint, event TaskFlowEvent, trigger string) (int, error) {
	active, ok := e.tasks.ActiveForProject(projectID)
	if !ok {
		return 0, nil
	}
	at := event.At
	if at.IsZero() {
		at = time.Now()
	}
	count := 0
	for _, tag := range e.tags.ForProject(projectID) {
		task := tag.StoreTaskForTrigger(event.GatewayID, event.Topic, active, at, trigger, true, false)
		if task == nil {
			continue
		}
		select {
		case e.channels.Store <- task:
			tag.MarkStorageRoutesStored(task.StorageRoutes, task.Timestamp)
			count++
		default:
			e.channels.RecordDrop("store")
			return count, fmt.Errorf("store queue full")
		}
	}
	return count, nil
}

func (e *TaskFlowExecutor) prepareStorage(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	task, err := e.repo.GetDetectionTask(taskID)
	if err != nil {
		return nil, err
	}
	if err := e.repo.EnsureProjectWideTable(task.ProjectID, task.StorageRoutes); err != nil {
		return nil, err
	}
	return map[string]any{"task_id": task.ID, "project_id": task.ProjectID, "routes": len(task.StorageRoutes)}, nil
}

func (e *TaskFlowExecutor) updateDetectionLimits(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	if rawItems, ok := params["items"]; ok {
		items, err := updateDetectionLimitItemsFromAny(rawItems)
		if err != nil {
			return nil, err
		}
		updated := make([]map[string]any, 0, len(items))
		for _, itemParams := range items {
			itemParams["task_id"] = taskID
			item, err := e.updateOneDetectionLimit(itemParams)
			if err != nil {
				return nil, err
			}
			updated = append(updated, item)
		}
		task, err := e.repo.GetDetectionTask(taskID)
		if err != nil {
			return nil, err
		}
		e.tasks.UpdateActive(task)
		e.recordDetectionRunEvent(task, models.DetectionEventLimitsUpdated, "info", "running detection limits updated by task flow")
		return map[string]any{"task_id": task.ID, "project_id": task.ProjectID, "updated": updated, "count": len(updated)}, nil
	}
	result, err := e.updateOneDetectionLimit(params)
	if err != nil {
		return nil, err
	}
	task, err := e.repo.GetDetectionTask(taskID)
	if err != nil {
		return nil, err
	}
	e.tasks.UpdateActive(task)
	e.recordDetectionRunEvent(task, models.DetectionEventLimitsUpdated, "info", "running detection limit updated by task flow")
	return result, nil
}

func (e *TaskFlowExecutor) updateOneDetectionLimit(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	varID := toInt64(params["var_id"])
	if varID == 0 {
		return nil, fmt.Errorf("var_id is required")
	}
	updates := make(map[string]interface{})
	setOptionalBoolUpdate(updates, params, "alarm_enabled")
	setOptionalBoolUpdate(updates, params, "check_enabled")
	setOptionalBoolUpdate(updates, params, "store_enabled")
	setOptionalBoolUpdate(updates, params, "check_on_start")
	setOptionalIntUpdate(updates, params, "check_cycle_ms")
	setOptionalIntUpdate(updates, params, "violation_hold_ms")
	setOptionalIntUpdate(updates, params, "recover_hold_ms")
	setOptionalFloatUpdate(updates, params, "limit_ll")
	setOptionalFloatUpdate(updates, params, "limit_l")
	setOptionalFloatUpdate(updates, params, "limit_h")
	setOptionalFloatUpdate(updates, params, "limit_hh")
	setOptionalFloatUpdate(updates, params, "limit_deadband")
	item, err := e.repo.UpdateDetectionRunStandardItem(taskID, varID, updates)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"task_id":       taskID,
		"var_id":        item.VarID,
		"alarm_enabled": item.AlarmEnabled,
		"check_enabled": item.CheckEnabled,
		"store_enabled": item.StoreEnabled,
		"limit_ll":      item.LimitLL,
		"limit_l":       item.LimitL,
		"limit_h":       item.LimitH,
		"limit_hh":      item.LimitHH,
	}, nil
}

func (e *TaskFlowExecutor) registerReport(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	fileRef := strings.TrimSpace(stringFromAny(params["file_ref"]))
	if fileRef == "" {
		return nil, fmt.Errorf("file_ref is required")
	}
	task, err := e.repo.GetDetectionTask(taskID)
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(stringFromAny(params["status"]))
	if status == "" {
		status = "generated"
	}
	report := &models.DetectionRunReport{
		TaskID:          task.ID,
		TemplateID:      task.ReportTemplateID,
		TemplateCode:    task.ReportTemplateCode,
		TemplateVersion: task.ReportTemplateVersion,
		FileRef:         fileRef,
		FileName:        stringFromAny(params["file_name"]),
		Status:          status,
		ErrorMessage:    stringFromAny(params["error_message"]),
	}
	if raw := optionalUintFromAny(params["template_id"]); raw != nil {
		report.TemplateID = raw
	}
	if code := strings.TrimSpace(stringFromAny(params["template_code"])); code != "" {
		report.TemplateCode = code
	}
	if version := int(toFloat64(params["template_version"])); version > 0 {
		report.TemplateVersion = version
	}
	if status == "generated" {
		now := time.Now()
		report.GeneratedAt = &now
	}
	if err := e.repo.CreateDetectionRunReport(report); err != nil {
		return nil, err
	}
	return map[string]any{"task_id": task.ID, "report_id": report.ID, "file_ref": report.FileRef, "status": report.Status}, nil
}

func (e *TaskFlowExecutor) httpRequest(flow models.TaskFlow, params map[string]any) (map[string]any, error) {
	method := strings.ToUpper(strings.TrimSpace(stringFromAny(params["method"])))
	if method == "" {
		method = http.MethodPost
	}
	url := strings.TrimSpace(stringFromAny(params["url"]))
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("url must start with http:// or https://")
	}
	bodyText := stringFromAny(params["body"])
	timeout := time.Duration(int(toFloat64(params["timeout_ms"]))) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(flow.TimeoutMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	req, err := http.NewRequest(method, url, bytes.NewBufferString(bodyText))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if headers, ok := params["headers"].(map[string]any); ok {
		for key, value := range headers {
			req.Header.Set(key, fmt.Sprint(value))
		}
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	return map[string]any{"status_code": resp.StatusCode, "body": string(body)}, nil
}

func setOptionalBoolUpdate(updates map[string]interface{}, params map[string]any, key string) {
	if value, ok := params[key]; ok {
		updates[key] = boolFromAnyDefault(value, false)
	}
}

func setOptionalIntUpdate(updates map[string]interface{}, params map[string]any, key string) {
	if value, ok := params[key]; ok {
		updates[key] = int(toFloat64(value))
	}
}

func setOptionalFloatUpdate(updates map[string]interface{}, params map[string]any, key string) {
	if value, ok := params[key]; ok {
		if value == nil {
			updates[key] = nil
			return
		}
		updates[key] = toFloat64(value)
	}
}

func updateDetectionLimitItemsFromAny(value any) ([]map[string]any, error) {
	rawItems, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("items must be an array")
	}
	if len(rawItems) == 0 {
		return nil, fmt.Errorf("items cannot be empty")
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("items must contain objects")
		}
		items = append(items, item)
	}
	return items, nil
}

func (e *TaskFlowExecutor) writeVariableFromTaskFlow(ctx *taskFlowRunContext, params map[string]any) (map[string]any, error) {
	varID := toInt64(params["var_id"])
	if varID == 0 {
		err := fmt.Errorf("var_id is required")
		e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
		return nil, err
	}
	if _, ok := params["value"]; !ok {
		err := fmt.Errorf("value is required")
		e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
		return nil, err
	}
	tag, ok := e.tags.Get(varID)
	if !ok {
		err := fmt.Errorf("variable %d not found", varID)
		e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
		return nil, err
	}
	if !canTaskFlowWriteTag(tag.Config) {
		err := fmt.Errorf("variable %d is not writable", varID)
		e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
		return nil, err
	}

	value := params["value"]
	quality := int(toFloat64(params["quality"]))
	if quality == 0 {
		quality = 1
	}
	now := time.Now()
	if models.IsStringDataType(tag.Config.DataType) {
		tag.UpdateString(fmt.Sprint(value), now, quality)
	} else {
		numeric := toFloat64(value)
		if tag.Config.WriteMin != nil && numeric < *tag.Config.WriteMin {
			err := fmt.Errorf("value %.6g is below write_min %.6g", numeric, *tag.Config.WriteMin)
			e.recordTaskFlowWriteAudit(ctx, varID, fmt.Sprint(value), "failed", err.Error())
			return nil, err
		}
		if tag.Config.WriteMax != nil && numeric > *tag.Config.WriteMax {
			err := fmt.Errorf("value %.6g is above write_max %.6g", numeric, *tag.Config.WriteMax)
			e.recordTaskFlowWriteAudit(ctx, varID, fmt.Sprint(value), "failed", err.Error())
			return nil, err
		}
		tag.UpdateNumeric(numeric, now, quality)
	}

	triggered := 0
	triggerEnabled := boolFromAnyDefault(params["trigger"], true)
	maxDepth := int(toFloat64(params["max_depth"]))
	if maxDepth <= 0 {
		maxDepth = 1
	}
	nextDepth := ctx.Event.Depth + 1
	originFlowID := ctx.Event.OriginFlowID
	if originFlowID == 0 {
		originFlowID = ctx.Flow.ID
	}
	originRunID := ctx.Event.OriginRunID
	if originRunID == 0 {
		originRunID = ctx.RunID
	}
	requestID := strings.TrimSpace(stringFromAny(params["request_id"]))
	if requestID == "" {
		requestID = ctx.Event.RequestID
	}
	allowReentrant := boolFromAnyDefault(params["allow_reentrant"], false)
	if triggerEnabled && nextDepth <= maxDepth {
		projectID := firstNonZeroUint(ctx.Event.ProjectID, ctx.Flow.ProjectID)
		if tag.Config.ProjectID != nil {
			projectID = *tag.Config.ProjectID
		}
		valueForEvent := value
		if !models.IsStringDataType(tag.Config.DataType) {
			valueForEvent = toFloat64(value)
		}
		triggered = e.Trigger(TaskFlowEvent{
			TriggerType:    models.TaskFlowTriggerDataChange,
			ProjectID:      projectID,
			TriggerVarID:   varID,
			TriggerValue:   valueForEvent,
			GatewayID:      tag.Config.GatewayID,
			Topic:          tag.Config.SourceTopic,
			At:             now,
			OriginFlowID:   originFlowID,
			OriginRunID:    originRunID,
			Depth:          nextDepth,
			MaxDepth:       maxDepth,
			AllowReentrant: allowReentrant,
			RequestID:      requestID,
		})
	}
	e.recordTaskFlowWriteAudit(ctx, varID, fmt.Sprint(value), "success", "")
	return map[string]any{
		"var_id":          varID,
		"var_id_text":     fmt.Sprintf("%d", varID),
		"value":           value,
		"quality":         quality,
		"triggered":       triggered,
		"trigger_enabled": triggerEnabled,
		"origin_flow_id":  originFlowID,
		"origin_run_id":   originRunID,
		"depth":           ctx.Event.Depth,
		"next_depth":      nextDepth,
		"max_depth":       maxDepth,
		"allow_reentrant": allowReentrant,
		"request_id":      requestID,
		"updated_at":      now,
	}, nil
}

func (e *TaskFlowExecutor) writeControlVariables(ctx *taskFlowRunContext, params map[string]any) (map[string]any, error) {
	if e.variableWriter == nil {
		return nil, fmt.Errorf("variable writer is not available")
	}
	items, err := controlWriteItemsFromParams(params)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(items))
	written := 0
	for index, item := range items {
		varID := toInt64(item["var_id"])
		if varID == 0 {
			err := fmt.Errorf("items[%d].var_id is required", index)
			e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
			return nil, err
		}
		value, err := e.controlWriteValue(item, ctx)
		if err != nil {
			e.recordTaskFlowWriteAudit(ctx, varID, "", "failed", err.Error())
			return nil, fmt.Errorf("items[%d]: %w", index, err)
		}
		quality := int(toFloat64(item["quality"]))
		if quality == 0 {
			quality = 1
		}
		originFlowID := ctx.Event.OriginFlowID
		if originFlowID == 0 {
			originFlowID = ctx.Flow.ID
		}
		originRunID := ctx.Event.OriginRunID
		if originRunID == 0 {
			originRunID = ctx.RunID
		}
		requestID := strings.TrimSpace(stringFromAny(item["request_id"]))
		if requestID == "" {
			requestID = ctx.Event.RequestID
		}
		if requestID == "" {
			requestID = fmt.Sprintf("task-flow-%d-%d", ctx.RunID, index+1)
		}
		maxDepth := int(toFloat64(item["max_depth"]))
		if maxDepth <= 0 {
			maxDepth = 1
		}
		input := TaskFlowVariableWriteInput{
			VarID:          varID,
			Value:          value,
			Quality:        quality,
			Trigger:        boolFromAnyDefault(item["trigger"], false),
			WaitAck:        boolFromAnyDefault(item["wait_ack"], true),
			AckTimeoutSec:  int(toFloat64(item["ack_timeout_sec"])),
			OriginFlowID:   originFlowID,
			OriginRunID:    originRunID,
			Depth:          ctx.Event.Depth,
			MaxDepth:       maxDepth,
			AllowReentrant: boolFromAnyDefault(item["allow_reentrant"], false),
			RequestID:      requestID,
		}
		if input.AckTimeoutSec <= 0 {
			input.AckTimeoutSec = 5
		}
		result, err := e.variableWriter(context.Background(), input)
		if err != nil {
			e.recordTaskFlowWriteAudit(ctx, varID, fmt.Sprint(value), "failed", err.Error())
			return nil, fmt.Errorf("items[%d] write variable %d: %w", index, varID, err)
		}
		e.recordTaskFlowWriteAudit(ctx, varID, fmt.Sprint(value), "success", "")
		results = append(results, result)
		written++
		if settle := time.Duration(toFloat64(item["settle_ms"])) * time.Millisecond; settle > 0 {
			time.Sleep(settle)
		}
	}
	return map[string]any{
		"written": written,
		"items":   results,
	}, nil
}

func controlWriteItemsFromParams(params map[string]any) ([]map[string]any, error) {
	if rawItems, ok := params["items"]; ok {
		if rawItems == nil {
			return nil, nil
		}
		items, ok := rawItems.([]any)
		if !ok {
			return nil, fmt.Errorf("items must be an array")
		}
		if len(items) == 0 {
			return nil, nil
		}
		out := make([]map[string]any, 0, len(items))
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("items must contain objects")
			}
			out = append(out, item)
		}
		return out, nil
	}
	if _, ok := params["var_id"]; !ok {
		return nil, fmt.Errorf("items or var_id is required")
	}
	return []map[string]any{params}, nil
}

func (e *TaskFlowExecutor) controlWriteValue(item map[string]any, ctx *taskFlowRunContext) (any, error) {
	if value, ok := item["value"]; ok {
		return value, nil
	}
	valueFrom := strings.TrimSpace(stringFromAny(item["value_from"]))
	if valueFrom == "" {
		return nil, fmt.Errorf("value or value_from is required")
	}
	if value, ok := taskFlowValueByPath(ctx.Params, valueFrom); ok {
		return value, nil
	}
	if value, ok := taskFlowValueByPath(ctx.Values, valueFrom); ok {
		return value, nil
	}
	return nil, fmt.Errorf("value_from %q not found", valueFrom)
}

func canTaskFlowWriteTag(cfg models.TagConfig) bool {
	return cfg.SourceType == models.TagSourceVirtual
}

func (e *TaskFlowExecutor) recordTaskFlowWriteAudit(ctx *taskFlowRunContext, varID int64, value string, result string, errMessage string) {
	if e.repo == nil {
		return
	}
	detail, _ := json.Marshal(map[string]any{
		"flow_id":        ctx.Flow.ID,
		"flow_code":      ctx.Flow.FlowCode,
		"run_id":         ctx.RunID,
		"var_id":         varID,
		"value":          value,
		"origin_flow_id": ctx.Event.OriginFlowID,
		"origin_run_id":  ctx.Event.OriginRunID,
		"depth":          ctx.Event.Depth,
		"request_id":     ctx.Event.RequestID,
		"error":          errMessage,
	})
	if err := e.repo.CreateAuditLog(&models.SysAuditLog{
		ActorType:  "task_flow",
		ActorID:    fmt.Sprintf("%d", ctx.RunID),
		Action:     "task_flow.write_variable",
		TargetType: "variable",
		TargetID:   fmt.Sprintf("%d", varID),
		Result:     result,
		Detail:     string(detail),
	}); err != nil {
		log.Printf("task-flow write audit failed flow=%s run_id=%d var_id=%d err=%v", ctx.Flow.FlowCode, ctx.RunID, varID, err)
	}
}

func (e *TaskFlowExecutor) startDetectionRun(ctx *taskFlowRunContext, stepCode string, params map[string]any, logs *[]string) (map[string]any, error) {
	if errText := strings.TrimSpace(stringFromAny(ctx.Params["_error"])); errText != "" {
		return nil, fmt.Errorf("invalid task_params: %s", errText)
	}
	projectID := uintFromAny(params["project_id"])
	if projectID == 0 {
		projectID = firstNonZeroUint(ctx.Event.ProjectID, ctx.Flow.ProjectID)
	}
	if projectID == 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	testNo := strings.TrimSpace(stringFromAny(params["test_no"]))
	if testNo == "" {
		testNo = fmt.Sprintf("%s-%d", ctx.Flow.FlowCode, ctx.RunID)
	}
	mode := strings.TrimSpace(stringFromAny(params["mode"]))
	if mode == "" {
		mode = "task_flow"
	}
	endPolicy := strings.TrimSpace(stringFromAny(params["end_policy"]))
	if endPolicy == "" && boolFromAnyDefault(params["auto_stop_on_duration"], false) && toFloat64(params["duration_sec"]) > 0 {
		endPolicy = models.DetectionEndPolicyFixedDuration
	}
	if endPolicy == "" {
		endPolicy = models.DetectionEndPolicyManual
	}
	limitCheckEnabled := true
	if value, ok := params["limit_check_enabled"]; ok {
		limitCheckEnabled = boolFromAnyDefault(value, true)
	}
	customItems, err := detectionStandardItemsFromTaskParams(params["custom_items"])
	if err != nil {
		return nil, err
	}
	opts := database.StartDetectionOptions{
		ProjectID:         projectID,
		TestNo:            testNo,
		FactoryNo:         stringFromAny(params["factory_no"]),
		CustomerName:      stringFromAny(params["customer_name"]),
		DeviceModel:       stringFromAny(params["device_model"]),
		Mode:              mode,
		StandardID:        optionalUintFromAny(params["standard_id"]),
		ConfigEnabled:     optionalBoolFromAny(params["config_enabled"]),
		ConfigCode:        stringFromAny(params["config_code"]),
		ConfigName:        stringFromAny(params["config_name"]),
		ConfigVersion:     int(toFloat64(params["config_version"])),
		ConfigHash:        stringFromAny(params["config_hash"]),
		CustomItems:       customItems,
		ProcessParams:     params["process_params"],
		PLCWrites:         params["plc_writes"],
		ReportRequest:     params["report_request"],
		LimitCheckEnabled: &limitCheckEnabled,
		EndPolicy:         endPolicy,
		DurationSec:       int(toFloat64(params["duration_sec"])),
		QualifiedHoldMS:   qualifiedHoldMSFromParams(params),
		OperatorNote:      stringFromAny(params["operator_note"]),
		ReportTemplateID:  optionalUintFromAny(params["report_template_id"]),
	}
	task, err := e.repo.StartDetectionTaskWithOptions(opts)
	if err != nil {
		return nil, err
	}
	runtimeTask := *task
	enableStorage := boolFromAnyDefault(params["enable_storage"], true)
	enableAlarm := boolFromAnyDefault(params["enable_alarm"], true) && limitCheckEnabled
	if !enableStorage {
		runtimeTask.StorageRoutes = nil
	}
	if !enableAlarm {
		for i := range runtimeTask.StandardItems {
			runtimeTask.StandardItems[i].AlarmEnabled = false
		}
	}
	e.tasks.SetActive(runtimeTask)
	e.recordDetectionRunEvent(runtimeTask, models.DetectionEventRunStarted, "info", "detection run started by task flow")
	e.refreshDetectionSummary(task.ID)
	if enableStorage {
		e.enqueueDetectionStartSnapshots(runtimeTask)
	}
	if enableAlarm {
		e.evaluateDetectionOnStart(runtimeTask)
	}
	ctx.Values["detection_task_id"] = task.ID
	ctx.Values["task_id"] = task.ID
	ctx.Values[stepCode+".task_id"] = task.ID
	ctx.Values[stepCode+".project_id"] = task.ProjectID
	triggeredLifecycle := e.triggerDetectionLifecycle(ctx, models.TaskFlowTriggerProjectStart, *task)
	ctx.Values[stepCode+".project_start_triggered"] = triggeredLifecycle
	if endPolicy == models.DetectionEndPolicyFixedDuration && task.ExpectedEndAt != nil {
		e.startFixedDurationGuard(task.ID)
		*logs = append(*logs, fmt.Sprintf("fixed duration guard started task_id=%d", task.ID))
	}
	if endPolicy == models.DetectionEndPolicyQualifiedHold && opts.QualifiedHoldMS > 0 {
		started := e.startQualifiedHoldGuard(task.ID, time.Duration(opts.QualifiedHoldMS)*time.Millisecond, 500*time.Millisecond)
		*logs = append(*logs, fmt.Sprintf("qualified hold guard started=%t task_id=%d", started, task.ID))
	}
	return map[string]any{
		"task_id":             task.ID,
		"project_id":          task.ProjectID,
		"test_no":             task.TestNo,
		"status":              task.Status,
		"limit_check_enabled": limitCheckEnabled,
		"end_policy":          endPolicy,
		"qualified_hold_ms":   opts.QualifiedHoldMS,
		"enable_storage":      enableStorage,
		"enable_alarm":        enableAlarm,
		"project_start_flows": triggeredLifecycle,
	}, nil
}

func (e *TaskFlowExecutor) stopDetectionRunFromParams(params map[string]any, defaultEndType string, defaultReason string) (*models.DetectionTask, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(stringFromAny(params["reason"]))
	if reason == "" {
		reason = defaultReason
	}
	endType := strings.TrimSpace(stringFromAny(params["end_type"]))
	if endType == "" {
		endType = defaultEndType
	}
	return e.stopDetectionRun(taskID, reason, endType)
}

func qualifiedHoldMSFromParams(params map[string]any) int {
	holdMS := int(toFloat64(params["qualified_hold_ms"]))
	if holdMS <= 0 {
		holdMS = int(toFloat64(params["qualified_hold_sec"]) * 1000)
	}
	return holdMS
}

func detectionStandardItemsFromTaskParams(value any) ([]models.DetectionStandardItem, error) {
	if value == nil {
		return nil, nil
	}
	rawItems, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("custom_items must be an array")
	}
	items := make([]models.DetectionStandardItem, 0, len(rawItems))
	seen := make(map[int64]struct{}, len(rawItems))
	for _, raw := range rawItems {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("custom_items must contain objects")
		}
		varID := toInt64(itemMap["var_id"])
		if varID == 0 {
			return nil, fmt.Errorf("custom_items.var_id is required")
		}
		if _, ok := seen[varID]; ok {
			return nil, fmt.Errorf("custom_items contains duplicate var_id %d", varID)
		}
		seen[varID] = struct{}{}
		item := models.DetectionStandardItem{
			VarID:           varID,
			VarName:         strings.TrimSpace(stringFromAny(itemMap["var_name"])),
			DisplayName:     stringFromAny(itemMap["display_name"]),
			DisplayNameEN:   stringFromAny(itemMap["display_name_en"]),
			DisplayNameJA:   stringFromAny(itemMap["display_name_ja"]),
			CheckEnabled:    boolFromAnyDefault(itemMap["check_enabled"], true),
			AlarmEnabled:    boolFromAnyDefault(itemMap["alarm_enabled"], true),
			StoreEnabled:    boolFromAnyDefault(itemMap["store_enabled"], true),
			CheckCycleMS:    int(toFloat64(itemMap["check_cycle_ms"])),
			CheckOnStart:    boolFromAnyDefault(itemMap["check_on_start"], true),
			Required:        boolFromAnyDefault(itemMap["required"], false),
			CheckMethod:     firstNonEmpty(strings.TrimSpace(stringFromAny(itemMap["check_method"])), models.CheckMethodNumericRange),
			TargetValue:     strings.TrimSpace(stringFromAny(itemMap["target_value"])),
			LimitLL:         floatPointerFromTaskParam(itemMap, "limit_ll"),
			LimitL:          floatPointerFromTaskParam(itemMap, "limit_l"),
			LimitH:          floatPointerFromTaskParam(itemMap, "limit_h"),
			LimitHH:         floatPointerFromTaskParam(itemMap, "limit_hh"),
			LimitDeadband:   toFloat64(itemMap["limit_deadband"]),
			ViolationHoldMS: int(toFloat64(itemMap["violation_hold_ms"])),
			RecoverHoldMS:   int(toFloat64(itemMap["recover_hold_ms"])),
			QualityPolicy:   firstNonEmpty(strings.TrimSpace(stringFromAny(itemMap["quality_policy"])), models.QualityPolicyIgnoreBad),
			Unit:            stringFromAny(itemMap["unit"]),
			DecimalPlaces:   int(toFloat64(itemMap["decimal_places"])),
			SortOrder:       int(toFloat64(itemMap["sort_order"])),
		}
		if item.VarName == "" {
			return nil, fmt.Errorf("custom_items.var_name is required")
		}
		items = append(items, item)
	}
	return items, nil
}

func floatPointerFromTaskParam(values map[string]any, key string) *float64 {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	floatValue := toFloat64(value)
	return &floatValue
}

func (e *TaskFlowExecutor) stopDetectionRun(taskID uint, reason string, endType string) (*models.DetectionTask, error) {
	task, err := e.repo.StopDetectionTaskWithEndType(taskID, reason, endType)
	if err != nil {
		return nil, err
	}
	e.tasks.Clear(task.ProjectID)
	e.recordDetectionRunEvent(*task, models.DetectionEventRunStopped, "info", "detection run stopped by task flow")
	summary, ok := e.refreshDetectionSummary(task.ID)
	_, _ = e.refreshDetectionFeatures(task.ID)
	if ok {
		e.publishDetectionResult(*task, summary)
	}
	e.triggerDetectionLifecycle(nil, models.TaskFlowTriggerProjectEnd, *task)
	return task, nil
}

func (e *TaskFlowExecutor) triggerDetectionLifecycle(ctx *taskFlowRunContext, triggerType string, task models.DetectionTask) int {
	event := TaskFlowEvent{
		TriggerType:  triggerType,
		ProjectID:    task.ProjectID,
		TriggerVarID: 0,
		TriggerValue: map[string]any{
			"task_id":      task.ID,
			"project_id":   task.ProjectID,
			"project_code": task.ProjectCode,
			"test_no":      task.TestNo,
			"status":       task.Status,
			"end_type":     task.EndType,
			"trigger_type": triggerType,
		},
		At: time.Now(),
	}
	if ctx != nil {
		event.OriginFlowID = ctx.Event.OriginFlowID
		if event.OriginFlowID == 0 {
			event.OriginFlowID = ctx.Flow.ID
		}
		event.OriginRunID = ctx.Event.OriginRunID
		if event.OriginRunID == 0 {
			event.OriginRunID = ctx.RunID
		}
		event.Depth = ctx.Event.Depth + 1
		event.MaxDepth = ctx.Event.MaxDepth
		if event.MaxDepth <= 0 {
			event.MaxDepth = 3
		}
		event.AllowReentrant = ctx.Event.AllowReentrant
		event.RequestID = ctx.Event.RequestID
	}
	return e.Trigger(event)
}

func (e *TaskFlowExecutor) finishIfFixedDuration(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	task, err := e.repo.GetDetectionTask(taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != models.DetectionStatusRunning {
		return map[string]any{"task_id": task.ID, "status": task.Status, "stopped": false}, nil
	}
	dueAt := task.ExpectedEndAt
	if dueAt == nil && task.StartedAt != nil && task.DurationSec > 0 {
		value := task.StartedAt.Add(time.Duration(task.DurationSec) * time.Second)
		dueAt = &value
	}
	if dueAt == nil {
		return map[string]any{"task_id": task.ID, "stopped": false, "reason": "duration not configured"}, nil
	}
	if time.Now().Before(*dueAt) {
		return map[string]any{"task_id": task.ID, "stopped": false, "due_at": dueAt}, nil
	}
	stopped, err := e.stopDetectionRun(task.ID, "fixed duration reached", models.DetectionEndFixedDuration)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task_id": stopped.ID, "project_id": stopped.ProjectID, "stopped": true, "end_type": stopped.EndType}, nil
}

func (e *TaskFlowExecutor) qualifiedHoldGuard(params map[string]any) (map[string]any, error) {
	taskID, err := e.taskIDFromParams(params)
	if err != nil {
		return nil, err
	}
	holdMS := int(toFloat64(params["qualified_hold_ms"]))
	if holdMS <= 0 {
		holdMS = int(toFloat64(params["qualified_hold_sec"]) * 1000)
	}
	if holdMS <= 0 {
		ok := e.activeTaskQualified(taskID)
		if !ok {
			return map[string]any{"task_id": taskID, "qualified": false, "stopped": false}, nil
		}
		stopped, err := e.stopDetectionRun(taskID, "qualified hold reached", models.DetectionEndQualifiedHold)
		if err != nil {
			return nil, err
		}
		return map[string]any{"task_id": stopped.ID, "project_id": stopped.ProjectID, "qualified": true, "stopped": true, "end_type": stopped.EndType}, nil
	}
	interval := time.Duration(int(toFloat64(params["check_interval_ms"]))) * time.Millisecond
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	started := e.startQualifiedHoldGuard(taskID, time.Duration(holdMS)*time.Millisecond, interval)
	return map[string]any{"task_id": taskID, "guard_started": started, "qualified_hold_ms": holdMS}, nil
}

func (e *TaskFlowExecutor) startFixedDurationGuard(taskID uint) {
	key := fmt.Sprintf("fixed:%d", taskID)
	if !e.markGuardStarted(key) {
		return
	}
	GoRecovering("task-flow-fixed-duration-guard", func() {
		defer e.clearGuard(key)
		for {
			task, err := e.repo.GetDetectionTask(taskID)
			if err != nil || task.Status == models.DetectionStatusStopped {
				return
			}
			if task.Status == models.DetectionStatusPaused {
				time.Sleep(time.Second)
				continue
			}
			if task.ExpectedEndAt == nil {
				return
			}
			wait := time.Until(*task.ExpectedEndAt)
			if wait > 0 {
				time.Sleep(minDuration(wait, time.Second))
				continue
			}
			_, _ = e.stopDetectionRun(taskID, "fixed duration reached", models.DetectionEndFixedDuration)
			return
		}
	})
}

func (e *TaskFlowExecutor) startQualifiedHoldGuard(taskID uint, hold time.Duration, interval time.Duration) bool {
	key := fmt.Sprintf("qualified:%d", taskID)
	if !e.markGuardStarted(key) {
		return false
	}
	GoRecovering("task-flow-qualified-hold-guard", func() {
		defer e.clearGuard(key)
		var since time.Time
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			task, err := e.repo.GetDetectionTask(taskID)
			if err != nil || task.Status == models.DetectionStatusStopped {
				return
			}
			if task.Status == models.DetectionStatusPaused {
				since = time.Time{}
				continue
			}
			if !e.activeTaskQualified(taskID) {
				since = time.Time{}
				continue
			}
			now := time.Now()
			if since.IsZero() {
				since = now
				continue
			}
			if now.Sub(since) >= hold {
				_, _ = e.stopDetectionRun(taskID, "qualified hold reached", models.DetectionEndQualifiedHold)
				return
			}
		}
	})
	return true
}

func (e *TaskFlowExecutor) activeTaskQualified(taskID uint) bool {
	return e.tasks.ActiveTaskQualified(e.tags, taskID)
}

func (e *TaskFlowExecutor) enqueueDetectionStartSnapshots(task models.DetectionTask) {
	if e.channels == nil {
		return
	}
	active, ok := e.tasks.ActiveForProject(task.ProjectID)
	if !ok {
		return
	}
	now := time.Now()
	for _, tag := range e.tags.ForProject(task.ProjectID) {
		if !active.AllowsStore(tag.Config.VarID) {
			continue
		}
		if !tag.RuntimeState().Initialized {
			continue
		}
		storeTask := tag.StoreTaskForTrigger(tag.Config.GatewayID, tag.Config.SourceTopic, active, now, models.StoreTriggerOnStart, true, true)
		if storeTask == nil {
			continue
		}
		select {
		case e.channels.Store <- storeTask:
			tag.MarkStorageRoutesStored(storeTask.StorageRoutes, now)
		default:
			e.channels.RecordDrop("store")
			log.Printf("task-flow start snapshot dropped task_id=%d var_id=%d: store queue full", task.ID, tag.Config.VarID)
		}
	}
}

func (e *TaskFlowExecutor) evaluateDetectionOnStart(task models.DetectionTask) {
	if e.channels == nil {
		return
	}
	now := time.Now()
	for _, item := range task.StandardItems {
		if !item.CheckOnStart {
			continue
		}
		tag, ok := e.tags.Get(item.VarID)
		if !ok {
			continue
		}
		for _, event := range e.tasks.EvaluateLimitAlarm(tag, now, true) {
			select {
			case e.channels.Alarm <- event:
			default:
				e.channels.RecordDrop("alarm")
				log.Printf("task-flow start alarm dropped task_id=%d var_id=%d: alarm queue full", task.ID, item.VarID)
			}
		}
	}
}

func (e *TaskFlowExecutor) recordDetectionRunEvent(task models.DetectionTask, eventType string, level string, message string) {
	event := &models.DetectionRunEvent{
		TaskID:      task.ID,
		TestNo:      task.TestNo,
		ProjectID:   task.ProjectID,
		ProjectCode: task.ProjectCode,
		EventType:   eventType,
		EventLevel:  level,
		Message:     message,
	}
	if err := e.repo.CreateDetectionRunEvent(event); err != nil {
		log.Printf("task-flow create detection run event failed task_id=%d event_type=%s err=%v", task.ID, eventType, err)
		return
	}
	e.publishDetectionEventNotification(task, eventType, level, message)
}

func (e *TaskFlowExecutor) refreshDetectionSummary(taskID uint) (models.DetectionRunSummary, bool) {
	summary, err := e.repo.RefreshDetectionRunSummary(taskID)
	if err != nil {
		log.Printf("task-flow refresh detection summary failed task_id=%d err=%v", taskID, err)
		return models.DetectionRunSummary{}, false
	}
	return summary, true
}

func (e *TaskFlowExecutor) refreshDetectionFeatures(taskID uint) ([]models.DetectionRunFeature, error) {
	features, err := e.repo.RefreshDetectionRunFeatures(taskID)
	if err != nil {
		log.Printf("task-flow refresh detection features failed task_id=%d err=%v", taskID, err)
		return nil, err
	}
	task, err := e.repo.GetDetectionTask(taskID)
	if err == nil {
		e.recordDetectionRunEvent(task, models.DetectionEventFeaturesUpdated, "info", "detection run features refreshed")
	}
	return features, nil
}

func (e *TaskFlowExecutor) publishDetectionEventNotification(task models.DetectionTask, eventType string, level string, message string) {
	if e.channels == nil || e.channels.Notify == nil {
		return
	}
	notificationType := detectionEventNotificationType(eventType)
	if notificationType == "" {
		return
	}
	notification := models.RuntimeNotificationFromDetectionTask(notificationType, level, task, message, map[string]any{
		"event_type": eventType,
		"status":     task.Status,
		"end_type":   task.EndType,
	})
	select {
	case e.channels.Notify <- notification:
	default:
		e.channels.RecordDrop("notify")
		log.Printf("task-flow detection notification dropped type=%s task_id=%d: notify queue full", notificationType, task.ID)
	}
}

func (e *TaskFlowExecutor) publishDetectionResult(task models.DetectionTask, summary models.DetectionRunSummary) {
	if e.channels == nil || e.channels.Notify == nil {
		return
	}
	var notificationType string
	var level string
	var message string
	switch summary.ResultStatus {
	case models.DetectionSummaryStatusOK:
		notificationType = models.NotificationDetectionResultOK
		level = models.NotificationLevelSuccess
		message = "detection run result ok"
	case models.DetectionSummaryStatusNG:
		notificationType = models.NotificationDetectionResultNG
		level = models.NotificationLevelWarning
		message = "detection run result ng"
	default:
		return
	}
	notification := models.RuntimeNotificationFromDetectionTask(notificationType, level, task, message, map[string]any{
		"result_status":   summary.ResultStatus,
		"history_rows":    summary.HistoryRows,
		"alarm_total":     summary.AlarmTotal,
		"alarm_active":    summary.AlarmActive,
		"alarm_recovered": summary.AlarmRecovered,
		"duration_ms":     summary.DurationMS,
	})
	select {
	case e.channels.Notify <- notification:
	default:
		e.channels.RecordDrop("notify")
		log.Printf("task-flow detection result notification dropped task_id=%d status=%s: notify queue full", task.ID, summary.ResultStatus)
	}
}

func detectionEventNotificationType(eventType string) string {
	switch eventType {
	case models.DetectionEventRunStarted:
		return models.NotificationDetectionRunStarted
	case models.DetectionEventRunStopped:
		return models.NotificationDetectionRunStopped
	case models.DetectionEventRunAbnormalStop:
		return models.NotificationDetectionAbnormalStop
	case models.DetectionEventRunPaused:
		return models.NotificationDetectionRunPaused
	case models.DetectionEventRunResumed:
		return models.NotificationDetectionRunResumed
	case models.DetectionEventFeaturesUpdated:
		return models.NotificationDetectionFeatures
	default:
		return ""
	}
}

func (e *TaskFlowExecutor) taskIDFromParams(params map[string]any) (uint, error) {
	taskID := uintFromAny(params["task_id"])
	if taskID > 0 {
		return taskID, nil
	}
	projectID := uintFromAny(params["project_id"])
	if projectID > 0 {
		if active, ok := e.tasks.ActiveForProject(projectID); ok {
			return active.ID, nil
		}
		return 0, fmt.Errorf("no active detection run for project_id=%d", projectID)
	}
	return 0, fmt.Errorf("task_id or project_id is required")
}

func (e *TaskFlowExecutor) markGuardStarted(key string) bool {
	e.guardsMu.Lock()
	defer e.guardsMu.Unlock()
	if _, ok := e.guards[key]; ok {
		return false
	}
	e.guards[key] = struct{}{}
	return true
}

func (e *TaskFlowExecutor) clearGuard(key string) {
	e.guardsMu.Lock()
	defer e.guardsMu.Unlock()
	delete(e.guards, key)
}

func (e *TaskFlowExecutor) inputSnapshot(flow models.TaskFlow, event TaskFlowEvent) string {
	items := map[string]any{
		"flow_id":             flow.ID,
		"flow_code":           flow.FlowCode,
		"project_id":          flow.ProjectID,
		"trigger_type":        event.TriggerType,
		"trigger_var_id":      event.TriggerVarID,
		"trigger_var_id_text": fmt.Sprintf("%d", event.TriggerVarID),
		"trigger_value":       event.TriggerValue,
		"trigger_params":      taskFlowParamsFromTrigger(event.TriggerValue),
		"origin_flow_id":      event.OriginFlowID,
		"origin_run_id":       event.OriginRunID,
		"depth":               event.Depth,
		"request_id":          event.RequestID,
		"at":                  event.At,
	}
	if tag, ok := e.tags.Get(event.TriggerVarID); ok {
		items["trigger_var"] = snapshotMap(tag.Snapshot())
	}
	raw, _ := json.Marshal(items)
	return string(raw)
}

func newTaskFlowRunContext(flow models.TaskFlow, event TaskFlowEvent, runID uint64) *taskFlowRunContext {
	params := taskFlowParamsFromTrigger(event.TriggerValue)
	values := map[string]any{
		"flow_id":             flow.ID,
		"flow_code":           flow.FlowCode,
		"project_id":          firstNonZeroUint(event.ProjectID, flow.ProjectID),
		"trigger_type":        event.TriggerType,
		"trigger_var_id":      event.TriggerVarID,
		"trigger_var_id_text": fmt.Sprintf("%d", event.TriggerVarID),
		"trigger_value":       event.TriggerValue,
		"origin_flow_id":      event.OriginFlowID,
		"origin_run_id":       event.OriginRunID,
		"depth":               event.Depth,
		"request_id":          event.RequestID,
		"run_id":              runID,
	}
	for key, value := range params {
		values["param."+key] = value
	}
	return &taskFlowRunContext{
		Flow:   flow,
		Event:  event,
		RunID:  runID,
		Params: params,
		Values: values,
		Steps:  make([]map[string]any, 0),
	}
}

func taskFlowParamsFromTrigger(value any) map[string]any {
	out := map[string]any{}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			out[key] = item
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || !strings.HasPrefix(text, "{") {
			return out
		}
		if len([]byte(text)) > taskFlowStringParamMaxBytes {
			out["_error"] = fmt.Sprintf("payload exceeds %d bytes", taskFlowStringParamMaxBytes)
			return out
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			out["_error"] = "invalid JSON"
		}
	}
	return out
}

func taskFlowSteps(flow models.TaskFlow) ([]taskFlowStep, error) {
	if strings.TrimSpace(flow.StepsJSON) != "" {
		var steps []taskFlowStep
		if err := json.Unmarshal([]byte(flow.StepsJSON), &steps); err != nil {
			var single taskFlowStep
			if singleErr := json.Unmarshal([]byte(flow.StepsJSON), &single); singleErr != nil {
				return nil, fmt.Errorf("invalid steps_json: %w", err)
			}
			steps = []taskFlowStep{single}
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("steps_json cannot be empty")
		}
		seenCodes := map[string]struct{}{}
		for i := range steps {
			steps[i].Module = strings.ToLower(strings.TrimSpace(steps[i].Module))
			steps[i].Code = strings.TrimSpace(steps[i].Code)
			if steps[i].Code == "" {
				steps[i].Code = fmt.Sprintf("step_%d", i+1)
			}
			if _, exists := seenCodes[steps[i].Code]; exists {
				return nil, fmt.Errorf("duplicate task flow step code: %s", steps[i].Code)
			}
			seenCodes[steps[i].Code] = struct{}{}
		}
		return steps, nil
	}
	params := map[string]any{}
	if strings.TrimSpace(flow.ActionPayload) != "" {
		if err := json.Unmarshal([]byte(flow.ActionPayload), &params); err != nil {
			params["_raw"] = flow.ActionPayload
		}
	}
	return []taskFlowStep{{
		Code:   "default",
		Module: strings.ToLower(strings.TrimSpace(flow.ActionType)),
		Params: params,
		Script: flow.ActionScript,
	}}, nil
}

func (e *TaskFlowExecutor) runStep(ctx *taskFlowRunContext, step taskFlowStep, logs *[]string) (map[string]any, error) {
	params, err := resolveTaskFlowParams(step.Params, ctx)
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(step.Module) {
	case models.TaskFlowActionBuiltinStartDetectionRun:
		result, err := e.startDetectionRun(ctx, step.Code, params, logs)
		return result, err
	case models.TaskFlowActionBuiltinStopDetectionRun:
		task, err := e.stopDetectionRunFromParams(params, models.DetectionEndTaskFlowStop, "task flow stop")
		if err != nil {
			return nil, err
		}
		ctx.Values[step.Code+".task_id"] = task.ID
		ctx.Values["detection_task_id"] = task.ID
		return map[string]any{"task_id": task.ID, "project_id": task.ProjectID, "status": task.Status, "end_type": task.EndType}, nil
	case models.TaskFlowActionBuiltinPauseDetectionRun:
		taskID, err := e.taskIDFromParams(params)
		if err != nil {
			return nil, err
		}
		reason := stringFromAny(params["reason"])
		task, err := e.repo.PauseDetectionTask(taskID, reason)
		if err != nil {
			return nil, err
		}
		e.tasks.Clear(task.ProjectID)
		e.recordDetectionRunEvent(*task, models.DetectionEventRunPaused, "info", "detection run paused by task flow")
		e.refreshDetectionSummary(task.ID)
		ctx.Values[step.Code+".task_id"] = task.ID
		return map[string]any{"task_id": task.ID, "project_id": task.ProjectID, "status": task.Status}, nil
	case models.TaskFlowActionBuiltinResumeDetectionRun:
		taskID, err := e.taskIDFromParams(params)
		if err != nil {
			return nil, err
		}
		task, err := e.repo.ResumeDetectionTask(taskID)
		if err != nil {
			return nil, err
		}
		e.tasks.SetActive(*task)
		e.recordDetectionRunEvent(*task, models.DetectionEventRunResumed, "info", "detection run resumed by task flow")
		e.refreshDetectionSummary(task.ID)
		e.recoverDetectionGuardForTask(*task)
		ctx.Values[step.Code+".task_id"] = task.ID
		return map[string]any{"task_id": task.ID, "project_id": task.ProjectID, "status": task.Status}, nil
	case models.TaskFlowActionBuiltinFixedDurationGuard:
		result, err := e.finishIfFixedDuration(params)
		if taskID := uintFromAny(result["task_id"]); taskID > 0 {
			ctx.Values[step.Code+".task_id"] = taskID
		}
		return result, err
	case models.TaskFlowActionBuiltinQualifiedHoldGuard:
		result, err := e.qualifiedHoldGuard(params)
		if taskID := uintFromAny(result["task_id"]); taskID > 0 {
			ctx.Values[step.Code+".task_id"] = taskID
		}
		return result, err
	case models.TaskFlowActionBuiltinMuteDetectionAlarms:
		taskID, err := e.taskIDFromParams(params)
		if err != nil {
			return nil, err
		}
		muted := e.tasks.MuteActiveLimitAlarms(taskID)
		ctx.Values[step.Code+".task_id"] = taskID
		ctx.Values[step.Code+".muted"] = muted
		return map[string]any{"task_id": taskID, "muted": muted}, nil
	case models.TaskFlowActionBuiltinRefreshFeatures:
		taskID, err := e.taskIDFromParams(params)
		if err != nil {
			return nil, err
		}
		features, err := e.refreshDetectionFeatures(taskID)
		if err != nil {
			return nil, err
		}
		ctx.Values[step.Code+".feature_count"] = len(features)
		return map[string]any{"task_id": taskID, "feature_count": len(features)}, nil
	case models.TaskFlowActionBuiltinUpdateDetectionLimits:
		result, err := e.updateDetectionLimits(params)
		if err != nil {
			return nil, err
		}
		if taskID := uintFromAny(result["task_id"]); taskID > 0 {
			ctx.Values[step.Code+".task_id"] = taskID
		}
		return result, err
	case models.TaskFlowActionBuiltinStorageSnapshot:
		projectID := uintFromAny(params["project_id"])
		if projectID == 0 {
			projectID = firstNonZeroUint(ctx.Event.ProjectID, ctx.Flow.ProjectID)
		}
		count, err := e.storageSnapshot(projectID, ctx.Event, models.StoreTriggerOnDetection)
		result := map[string]any{"stored": count, "project_id": projectID}
		ctx.Values[step.Code+".stored"] = count
		ctx.Values[step.Code+".project_id"] = projectID
		return result, err
	case models.TaskFlowActionBuiltinStoragePrepare:
		result, err := e.prepareStorage(params)
		if err != nil {
			return nil, err
		}
		if taskID := uintFromAny(result["task_id"]); taskID > 0 {
			ctx.Values[step.Code+".task_id"] = taskID
		}
		return result, err
	case models.TaskFlowActionBuiltinRegisterReport:
		result, err := e.registerReport(params)
		if err != nil {
			return nil, err
		}
		if taskID := uintFromAny(result["task_id"]); taskID > 0 {
			ctx.Values[step.Code+".task_id"] = taskID
		}
		return result, nil
	case models.TaskFlowActionBuiltinWriteVariable:
		result, err := e.writeVariableFromTaskFlow(ctx, params)
		for key, value := range result {
			ctx.Values[step.Code+"."+key] = value
		}
		return result, err
	case models.TaskFlowActionBuiltinWriteControlVariables:
		result, err := e.writeControlVariables(ctx, params)
		for key, value := range result {
			ctx.Values[step.Code+"."+key] = value
		}
		return result, err
	case models.TaskFlowActionBuiltinHTTPRequest:
		result, err := e.httpRequest(ctx.Flow, params)
		return result, err
	case models.TaskFlowActionBuiltinContextSet:
		for key, value := range params {
			ctx.Values[key] = value
			ctx.Values[step.Code+"."+key] = value
		}
		return map[string]any{"set": params}, nil
	case models.TaskFlowActionJavaScript:
		result, err := e.runJavaScript(ctx, step.Script, params, logs)
		for key, value := range result {
			ctx.Values[step.Code+"."+key] = value
		}
		return result, err
	default:
		return nil, fmt.Errorf("unsupported task flow module %q", step.Module)
	}
}

func (e *TaskFlowExecutor) recoverDetectionGuardForTask(task models.DetectionTask) {
	switch task.EndPolicy {
	case models.DetectionEndPolicyFixedDuration:
		if task.ExpectedEndAt != nil {
			e.startFixedDurationGuard(task.ID)
		}
	case models.DetectionEndPolicyQualifiedHold:
		if task.QualifiedHoldMS > 0 {
			e.startQualifiedHoldGuard(task.ID, time.Duration(task.QualifiedHoldMS)*time.Millisecond, 500*time.Millisecond)
		}
	}
}

func resolveTaskFlowParams(params map[string]any, ctx *taskFlowRunContext) (map[string]any, error) {
	out := make(map[string]any, len(params))
	for key, value := range params {
		resolved, err := resolveTaskFlowValue(value, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve param %s: %w", key, err)
		}
		out[key] = resolved
	}
	return out, nil
}

func resolveTaskFlowValue(value any, ctx *taskFlowRunContext) (any, error) {
	switch typed := value.(type) {
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			resolved, err := resolveTaskFlowValue(item, ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, resolved)
		}
		return out, nil
	case map[string]any:
		sourceRaw, hasSource := typed["source"]
		if !hasSource {
			out := make(map[string]any, len(typed))
			for key, item := range typed {
				resolved, err := resolveTaskFlowValue(item, ctx)
				if err != nil {
					return nil, err
				}
				out[key] = resolved
			}
			return out, nil
		}
		source := strings.ToLower(strings.TrimSpace(fmt.Sprint(sourceRaw)))
		key := strings.TrimSpace(fmt.Sprint(typed["key"]))
		defaultValue, hasDefault := typed["default"]
		optional := boolFromAnyDefault(typed["optional"], false)
		switch source {
		case "literal":
			return typed["value"], nil
		case "trigger_param":
			value, ok := taskFlowValueByPath(ctx.Params, key)
			if !ok {
				if hasDefault {
					return defaultValue, nil
				}
				if optional {
					return nil, nil
				}
				return nil, fmt.Errorf("missing trigger_param %q", key)
			}
			return value, nil
		case "event":
			return taskFlowEventValue(ctx.Event, key)
		case "context":
			value, ok := taskFlowValueByPath(ctx.Values, key)
			if !ok {
				if hasDefault {
					return defaultValue, nil
				}
				if optional {
					return nil, nil
				}
				return nil, fmt.Errorf("missing context %q", key)
			}
			return value, nil
		default:
			return nil, fmt.Errorf("unknown param source %q", source)
		}
	default:
		return value, nil
	}
}

func taskFlowValueByPath(values map[string]any, path string) (any, bool) {
	if values == nil {
		return nil, false
	}
	if value, ok := values[path]; ok {
		return value, true
	}
	parts := strings.Split(path, ".")
	if len(parts) <= 1 {
		return nil, false
	}
	var current any = values
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
	}
	return current, true
}

func taskFlowEventValue(event TaskFlowEvent, key string) (any, error) {
	switch key {
	case "trigger_type":
		return event.TriggerType, nil
	case "project_id":
		return event.ProjectID, nil
	case "trigger_var_id":
		return event.TriggerVarID, nil
	case "trigger_var_id_text":
		return fmt.Sprintf("%d", event.TriggerVarID), nil
	case "trigger_value":
		return event.TriggerValue, nil
	case "gateway_id":
		return event.GatewayID, nil
	case "topic":
		return event.Topic, nil
	case "origin_flow_id":
		return event.OriginFlowID, nil
	case "origin_run_id":
		return event.OriginRunID, nil
	case "depth":
		return event.Depth, nil
	case "request_id":
		return event.RequestID, nil
	case "at":
		return event.At, nil
	default:
		return nil, fmt.Errorf("unknown event key %q", key)
	}
}

func firstNonZeroUint(values ...uint) uint {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func uintFromAny(value any) uint {
	if value == nil {
		return 0
	}
	return uint(toFloat64(value))
}

func optionalUintFromAny(value any) *uint {
	out := uintFromAny(value)
	if out == 0 {
		return nil
	}
	return &out
}

func optionalBoolFromAny(value any) *bool {
	if value == nil {
		return nil
	}
	out := boolFromAnyDefault(value, false)
	return &out
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boolFromAnyDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	default:
		return toFloat64(value) != 0
	}
}

func minDuration(a time.Duration, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func snapshotMap(snapshot models.TagSnapshot) map[string]any {
	return map[string]any{
		"var_id":       snapshot.VarID,
		"var_id_text":  fmt.Sprintf("%d", snapshot.VarID),
		"var_name":     snapshot.VarName,
		"project_id":   snapshot.ProjectID,
		"value":        snapshot.Value,
		"str_value":    snapshot.StrValue,
		"is_string":    snapshot.IsString,
		"quality":      snapshot.Quality,
		"last_update":  snapshot.LastUpdate,
		"display_name": snapshot.DisplayName,
	}
}

func toFloat64(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case uint32:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "on":
			return 1
		case "false", "no", "off":
			return 0
		}
		return 0
	default:
		return 0
	}
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case uint:
		return int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return 0
		}
		return int64(v)
	case uint32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return parsed
		}
		return 0
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}
