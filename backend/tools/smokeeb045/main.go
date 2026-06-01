//go:build smoke_tools

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	base  string
	token string
	http  *http.Client
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

type project struct {
	ID          uint   `json:"id"`
	ProjectCode string `json:"project_code"`
}

type tag struct {
	VarID     int64  `json:"var_id"`
	VarIDText string `json:"var_id_text"`
}

type taskFlow struct {
	ID       uint64 `json:"id"`
	FlowCode string `json:"flow_code"`
}

type taskFlowRun struct {
	ID            uint64 `json:"id"`
	FlowCode      string `json:"flow_code"`
	Status        string `json:"status"`
	TriggerType   string `json:"trigger_type"`
	TriggerVarID  int64  `json:"trigger_var_id"`
	ResultJSON    string `json:"result_json"`
	ErrorMessage  string `json:"error_message"`
	InputSnapshot string `json:"input_snapshot"`
	ScriptLogs    string `json:"script_logs"`
}

type taskFlowSQLLog struct {
	ID           uint64 `json:"id"`
	RunID        uint64 `json:"run_id"`
	SQLText      string `json:"sql_text"`
	ErrorMessage string `json:"error_message"`
}

type detectionTask struct {
	ID               uint                       `json:"id"`
	ProjectID        uint                       `json:"project_id"`
	TestNo           string                     `json:"test_no"`
	Status           string                     `json:"status"`
	EndType          string                     `json:"end_type"`
	EndPolicy        string                     `json:"end_policy"`
	CustomConfigJSON string                     `json:"custom_config_json"`
	StandardItems    []detectionRunStandardItem `json:"standard_items"`
	StorageRoutes    []detectionRunStorageRoute `json:"storage_routes"`
	Reports          []detectionRunReport       `json:"reports"`
}

type detectionRunStandardItem struct {
	VarID           int64    `json:"var_id"`
	VarIDText       string   `json:"var_id_text"`
	CheckEnabled    bool     `json:"check_enabled"`
	AlarmEnabled    bool     `json:"alarm_enabled"`
	StoreEnabled    bool     `json:"store_enabled"`
	CheckCycleMS    int      `json:"check_cycle_ms"`
	LimitL          *float64 `json:"limit_l"`
	LimitH          *float64 `json:"limit_h"`
	LimitHH         *float64 `json:"limit_hh"`
	LimitDeadband   float64  `json:"limit_deadband"`
	RecoverHoldMS   int      `json:"recover_hold_ms"`
	ViolationHoldMS int      `json:"violation_hold_ms"`
}

type storageRoute struct {
	ID        uint64 `json:"id"`
	Enabled   bool   `json:"enabled"`
	CycleMS   int    `json:"cycle_ms"`
	Column    string `json:"column_name"`
	TableName string `json:"table_name"`
}

type detectionRunStorageRoute struct {
	ID        uint64 `json:"id"`
	VarID     int64  `json:"var_id"`
	VarIDText string `json:"var_id_text"`
	Enabled   bool   `json:"enabled"`
	Column    string `json:"column_name"`
	TableName string `json:"table_name"`
}

type detectionRunReport struct {
	ID      uint64 `json:"id"`
	TaskID  uint   `json:"task_id"`
	FileRef string `json:"file_ref"`
	Status  string `json:"status"`
}

type detectionRunEvent struct {
	ID        uint64 `json:"id"`
	TaskID    uint   `json:"task_id"`
	EventType string `json:"event_type"`
}

type detectionLimitAlarm struct {
	ID         uint64   `json:"id"`
	Scope      string   `json:"scope"`
	TaskID     uint     `json:"task_id"`
	ProjectID  uint     `json:"project_id"`
	VarID      int64    `json:"var_id"`
	VarIDText  string   `json:"var_id_text"`
	AlarmType  string   `json:"alarm_type"`
	AlarmLevel string   `json:"alarm_level"`
	Status     string   `json:"status"`
	LimitValue *float64 `json:"limit_value"`
}

type runtimeNotification struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	ProjectID uint   `json:"project_id"`
	TaskID    uint   `json:"task_id"`
	Message   string `json:"message"`
}

type userNotification struct {
	ID        uint64  `json:"id"`
	EventUID  string  `json:"event_uid"`
	Type      string  `json:"type"`
	ProjectID uint    `json:"project_id"`
	TaskID    uint    `json:"task_id"`
	Message   string  `json:"message"`
	ReadAt    *string `json:"read_at"`
}

type auditLog struct {
	ID         uint64 `json:"id"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Result     string `json:"result"`
	Detail     string `json:"detail"`
}

type listResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Count int `json:"count"`
}

type wsMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	CommandID string          `json:"command_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *wsError        `json:"error,omitempty"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	base := flag.String("base", "http://127.0.0.1:18080", "backend base URL")
	user := flag.String("user", "admin", "login username")
	pass := flag.String("pass", "Admin@12345", "login password")
	wait := flag.Duration("wait", 10*time.Second, "max wait for async task-flow runs")
	flag.Parse()

	if err := run(*base, *user, *pass, *wait); err != nil {
		fmt.Fprintln(os.Stderr, "EB-045 smoke failed:", err)
		os.Exit(1)
	}
}

func run(base, username, password string, wait time.Duration) error {
	c := &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 15 * time.Second}}
	if err := c.login(username, password); err != nil {
		return err
	}
	fmt.Println("login ok")

	suffix := time.Now().Format("20060102150405")
	p, err := c.createProject(suffix)
	if err != nil {
		return err
	}
	fmt.Printf("project ok: %s=%d\n", p.ProjectCode, p.ID)

	requestVar, err := c.createVirtualVariable(p, "STRING", "smoke_eb045_request_"+suffix)
	if err != nil {
		return err
	}
	controlVar, err := c.createVirtualVariable(p, "FLOAT", "smoke_eb045_inlet_area_"+suffix)
	if err != nil {
		return err
	}
	measurementVar, err := c.createVirtualVariable(p, "FLOAT", "smoke_eb045_measurement_"+suffix)
	if err != nil {
		return err
	}
	defaultAlarmVar, err := c.createVirtualVariable(p, "FLOAT", "smoke_eb045_default_alarm_"+suffix)
	if err != nil {
		return err
	}
	if err := c.patchVariableDefaultAlarm(defaultAlarmVar.VarID); err != nil {
		return err
	}
	if err := c.enableCycleRoute(p.ID, measurementVar.VarID); err != nil {
		return err
	}
	fmt.Printf("variables ok: request=%d control=%d measurement=%d default_alarm=%d\n", requestVar.VarID, controlVar.VarID, measurementVar.VarID, defaultAlarmVar.VarID)

	flows, err := c.createTaskFlows(p, requestVar, suffix)
	if err != nil {
		return err
	}
	scheduleFlow, err := c.createScheduledJSFlow(p, suffix)
	if err != nil {
		return err
	}
	flows["schedule_js"] = scheduleFlow
	fmt.Printf("task flows ok: start=%d fixed=%d qualified=%d prepare=%d update=%d report=%d http=%d features=%d pause=%d resume=%d stop=%d mute=%d js=%d schedule_js=%d\n", flows["start"].ID, flows["fixed"].ID, flows["qualified"].ID, flows["prepare"].ID, flows["update"].ID, flows["report"].ID, flows["http"].ID, flows["features"].ID, flows["pause"].ID, flows["resume"].ID, flows["stop"].ID, flows["mute"].ID, flows["js"].ID, flows["schedule_js"].ID)

	if scheduleRun, err := c.waitTaskFlowRunByFlowCode(flows["schedule_js"].FlowCode, "schedule", wait); err != nil {
		return err
	} else if scheduleRun.Status != "success" || !strings.Contains(scheduleRun.ScriptLogs, "scheduled smoke") || !strings.Contains(scheduleRun.ResultJSON, `"trigger_type":"schedule"`) {
		return fmt.Errorf("schedule JS flow unexpected status=%s result=%s logs=%s error=%s", scheduleRun.Status, scheduleRun.ResultJSON, scheduleRun.ScriptLogs, scheduleRun.ErrorMessage)
	} else if err := c.disableTaskFlow(flows["schedule_js"].ID); err != nil {
		return err
	} else {
		fmt.Printf("schedule JS flow ok: flow_run=%d\n", scheduleRun.ID)
	}

	notifyConn, err := c.openNotificationWS(p.ID)
	if err != nil {
		return err
	}
	defer func() {
		_ = notifyConn.Close()
	}()

	testNo := "SMOKE-EB045-" + suffix

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "run_javascript", "expected": 7}, "js-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["js"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" || !strings.Contains(run.ResultJSON, `"ok":1`) || !strings.Contains(run.ResultJSON, `"expected":7`) || !strings.Contains(run.ScriptLogs, "js_smoke rows=1") {
		return fmt.Errorf("javascript flow unexpected status=%s result=%s logs=%s error=%s", run.Status, run.ResultJSON, run.ScriptLogs, run.ErrorMessage)
	} else if logs, err := c.getTaskFlowSQLLogs(run.ID); err != nil {
		return err
	} else if len(logs) == 0 || !strings.Contains(logs[0].SQLText, "SELECT 1 AS ok") || logs[0].ErrorMessage != "" {
		return fmt.Errorf("javascript SQL logs unexpected: %+v", logs)
	} else {
		fmt.Printf("javascript task flow ok: flow_run=%d sql_logs=%d\n", run.ID, len(logs))
	}
	startPayload := map[string]any{
		"command":             "start_detection",
		"project_id":          p.ID,
		"test_no":             testNo,
		"mode":                "smoke_task_flow",
		"limit_check_enabled": true,
		"end_policy":          "manual",
		"operator_note":       "EB-045 smoke formal STRING request",
		"process_params": map[string]any{
			"inlet_area_m2": 1.25,
			"fixture_code":  "SMOKE-FIXTURE",
		},
		"plc_writes": []map[string]any{{
			"var_id":      fmt.Sprintf("%d", controlVar.VarID),
			"value_from":  "process_params.inlet_area_m2",
			"wait_ack":    false,
			"request_id":  "smoke-eb045-control-" + suffix,
			"ack_timeout": 1,
		}},
		"custom_items": []map[string]any{{
			"var_id":            fmt.Sprintf("%d", measurementVar.VarID),
			"var_name":          "smoke_measurement",
			"display_name":      "Smoke measurement",
			"check_enabled":     true,
			"alarm_enabled":     true,
			"store_enabled":     true,
			"check_on_start":    true,
			"check_cycle_ms":    500,
			"limit_l":           0,
			"limit_h":           30,
			"limit_hh":          35,
			"limit_deadband":    0.2,
			"violation_hold_ms": 0,
			"recover_hold_ms":   0,
			"quality_policy":    "ignore_bad",
			"unit":              "Pa",
			"decimal_places":    2,
		}},
	}
	if err := c.writeTaskRequest(requestVar.VarID, startPayload, "start-"+suffix); err != nil {
		return err
	}
	startRun, err := c.waitTaskFlowRun(flows["start"].FlowCode, requestVar.VarID, wait)
	if err != nil {
		return err
	}
	if startRun.Status != "success" {
		return fmt.Errorf("start flow status=%s error=%s", startRun.Status, startRun.ErrorMessage)
	}
	task, err := c.currentRun(p.ID)
	if err != nil {
		return err
	}
	if task.Status != "running" || task.TestNo != testNo {
		return fmt.Errorf("unexpected started task: %+v", task)
	}
	detail, err := c.getRun(task.ID)
	if err != nil {
		return err
	}
	if !strings.Contains(detail.CustomConfigJSON, "process_params") || !strings.Contains(detail.CustomConfigJSON, "inlet_area_m2") || !strings.Contains(detail.CustomConfigJSON, "plc_writes") {
		return fmt.Errorf("detection custom_config_json did not freeze process/plc params: %s", detail.CustomConfigJSON)
	}
	if item := findStandardItem(detail.StandardItems, measurementVar.VarID); item == nil || item.LimitH == nil || *item.LimitH != 30 {
		return fmt.Errorf("custom standard item not frozen for measurement var %d: %+v", measurementVar.VarID, detail.StandardItems)
	}
	if len(detail.StorageRoutes) == 0 {
		return fmt.Errorf("expected frozen storage routes for measurement var %d", measurementVar.VarID)
	}
	if audit, err := c.waitWSCommandAudit("cmd-eb045-start-"+suffix, wait); err != nil {
		return err
	} else if audit.Result != "success" {
		return fmt.Errorf("unexpected start WS audit result: %+v", audit)
	}
	if audit, err := c.waitTaskFlowWriteAudit(controlVar.VarID, wait); err != nil {
		return err
	} else if audit.Result != "success" {
		return fmt.Errorf("unexpected control write audit result: %+v", audit)
	}
	startNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.run_started", p.ID, task.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("start request ok: flow_run=%d task=%d lifecycle_notification=%s\n", startRun.ID, task.ID, startNotification.EventUID)
	lastStartRunID := startRun.ID

	defer func() {
		_, _ = c.stopRun(task.ID, "smoke cleanup")
	}()

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "prepare_storage", "project_id": p.ID}, "prepare-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["prepare"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" || !strings.Contains(run.ResultJSON, `"routes":1`) {
		return fmt.Errorf("prepare storage flow unexpected status=%s result=%s error=%s", run.Status, run.ResultJSON, run.ErrorMessage)
	}
	fmt.Println("storage prepare request ok")

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":    "update_limits",
		"project_id": p.ID,
		"items": []map[string]any{{
			"var_id":            fmt.Sprintf("%d", measurementVar.VarID),
			"limit_h":           42,
			"limit_hh":          45,
			"limit_deadband":    0.5,
			"check_cycle_ms":    750,
			"violation_hold_ms": 100,
			"recover_hold_ms":   200,
			"store_enabled":     true,
			"alarm_enabled":     true,
			"check_enabled":     true,
		}},
	}, "update-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["update"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("update limits flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	}
	updatedDetail, err := c.getRun(task.ID)
	if err != nil {
		return err
	}
	updatedItem := findStandardItem(updatedDetail.StandardItems, measurementVar.VarID)
	if updatedItem == nil || updatedItem.LimitH == nil || *updatedItem.LimitH != 42 || updatedItem.LimitHH == nil || *updatedItem.LimitHH != 45 || updatedItem.CheckCycleMS != 750 || updatedItem.LimitDeadband != 0.5 || updatedItem.ViolationHoldMS != 100 || updatedItem.RecoverHoldMS != 200 {
		return fmt.Errorf("running limit update did not apply: %+v", updatedItem)
	}
	fmt.Println("update limits request ok")

	reportRef := "reports/smoke-eb045-" + suffix + ".xlsx"
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":   "register_report",
		"task_id":   task.ID,
		"file_ref":  reportRef,
		"file_name": "smoke-eb045-" + suffix + ".xlsx",
		"status":    "generated",
	}, "report-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["report"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("register report flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	}
	reportDetail, err := c.getRun(task.ID)
	if err != nil {
		return err
	}
	if !hasReport(reportDetail.Reports, reportRef) {
		return fmt.Errorf("registered report not visible in run detail: file_ref=%s reports=%+v", reportRef, reportDetail.Reports)
	}
	fmt.Println("register report request ok")

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":    "http_request",
		"method":     "GET",
		"url":        c.base + "/health",
		"timeout_ms": 3000,
	}, "http-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["http"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" || !strings.Contains(run.ResultJSON, `"status_code":200`) {
		return fmt.Errorf("http request flow unexpected status=%s result=%s error=%s", run.Status, run.ResultJSON, run.ErrorMessage)
	}
	fmt.Println("http request request ok")

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "pause_detection", "project_id": p.ID, "reason": "smoke pause"}, "pause-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["pause"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("pause flow status=%s error=%s", run.Status, run.ErrorMessage)
	}
	if task, err = c.currentRun(p.ID); err != nil {
		return err
	}
	if task.Status != "paused" {
		return fmt.Errorf("expected paused task, got %+v", task)
	}
	pauseNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.run_paused", p.ID, task.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("pause request ok: lifecycle_notification=%s\n", pauseNotification.EventUID)

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "resume_detection", "task_id": task.ID}, "resume-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["resume"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("resume flow status=%s error=%s", run.Status, run.ErrorMessage)
	}
	if task, err = c.currentRun(p.ID); err != nil {
		return err
	}
	if task.Status != "running" {
		return fmt.Errorf("expected resumed running task, got %+v", task)
	}
	resumeNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.run_resumed", p.ID, task.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("resume request ok: lifecycle_notification=%s\n", resumeNotification.EventUID)

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "stop_detection", "project_id": p.ID, "reason": "smoke stop"}, "stop-"+suffix); err != nil {
		return err
	}
	var stopRun taskFlowRun
	if run, err := c.waitTaskFlowRun(flows["stop"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("stop flow status=%s error=%s", run.Status, run.ErrorMessage)
	} else {
		stopRun = run
	}
	stopped, err := c.getRun(task.ID)
	if err != nil {
		return err
	}
	if stopped.Status != "stopped" || stopped.EndType != "task_flow_stop" {
		return fmt.Errorf("expected task_flow_stop, got %+v", stopped)
	}
	stopNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.run_stopped", p.ID, task.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("task flow stop request ok: lifecycle_notification=%s\n", stopNotification.EventUID)

	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "refresh_features", "task_id": task.ID}, "features-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["features"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("refresh features flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	}
	events, err := c.getRunEvents(task.ID)
	if err != nil {
		return err
	}
	if !hasEvent(events.Items, "features_updated") {
		return fmt.Errorf("features_updated event not found for task=%d events=%+v", task.ID, events.Items)
	}
	persisted, err := c.verifyNotificationReadCycle(notifyConn, "detection.features_updated", p.ID, task.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("refresh features request ok: notification=%s http_id=%d\n", persisted.EventUID, persisted.ID)

	fixedTestNo := "SMOKE-EB045-FIXED-" + suffix
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":        "start_fixed_duration_detection",
		"project_id":     p.ID,
		"test_no":        fixedTestNo,
		"duration_sec":   1,
		"enable_storage": false,
		"enable_alarm":   false,
	}, "fixed-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["fixed"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("fixed duration start flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	}
	fixedTask, err := c.currentRun(p.ID)
	if err != nil {
		return err
	}
	if fixedTask.TestNo != fixedTestNo || fixedTask.EndPolicy != "fixed_duration" {
		return fmt.Errorf("unexpected fixed duration task: %+v", fixedTask)
	}
	if fixedStopped, err := c.waitRunEnd(fixedTask.ID, "fixed_duration", 6*time.Second); err != nil {
		return err
	} else if fixedStopped.Status != "stopped" {
		return fmt.Errorf("expected fixed duration stopped task, got %+v", fixedStopped)
	}
	fixedNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.result_ok", p.ID, fixedTask.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("fixed duration request ok: result_notification=%s\n", fixedNotification.EventUID)

	if err := c.writeVariableWS(measurementVar.VarID, 25, false, "cmd-eb045-qualified-value-"+suffix); err != nil {
		return err
	}
	qualifiedTestNo := "SMOKE-EB045-QUALIFIED-" + suffix
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":           "start_qualified_hold_detection",
		"project_id":        p.ID,
		"test_no":           qualifiedTestNo,
		"qualified_hold_ms": 200,
		"enable_storage":    false,
		"enable_alarm":      true,
		"custom_items": []map[string]any{{
			"var_id":         fmt.Sprintf("%d", measurementVar.VarID),
			"var_name":       "smoke_measurement",
			"check_enabled":  true,
			"alarm_enabled":  true,
			"store_enabled":  false,
			"check_on_start": true,
			"check_cycle_ms": 100,
			"limit_l":        0,
			"limit_h":        42,
			"quality_policy": "ignore_bad",
			"decimal_places": 2,
			"limit_deadband": 0.1,
		}},
	}, "qualified-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["qualified"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("qualified hold start flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	}
	qualifiedTask, err := c.currentRun(p.ID)
	if err != nil {
		return err
	}
	if qualifiedTask.TestNo != qualifiedTestNo || qualifiedTask.EndPolicy != "qualified_hold" {
		return fmt.Errorf("unexpected qualified hold task: %+v", qualifiedTask)
	}
	if qualifiedStopped, err := c.waitRunEnd(qualifiedTask.ID, "qualified_hold", 6*time.Second); err != nil {
		return err
	} else if qualifiedStopped.Status != "stopped" {
		return fmt.Errorf("expected qualified hold stopped task, got %+v", qualifiedStopped)
	}
	qualifiedNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.result_ok", p.ID, qualifiedTask.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("qualified hold request ok: result_notification=%s\n", qualifiedNotification.EventUID)

	abnormalTestNo := "SMOKE-EB045-ABNORMAL-" + suffix
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":             "start_detection",
		"project_id":          p.ID,
		"test_no":             abnormalTestNo,
		"mode":                "smoke_task_flow",
		"limit_check_enabled": false,
		"end_policy":          "manual",
		"operator_note":       "EB-045 smoke abnormal stop notification",
	}, "abnormal-start-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRunAfter(flows["start"].FlowCode, requestVar.VarID, lastStartRunID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("abnormal start flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	} else {
		lastStartRunID = run.ID
	}
	abnormalTask, err := c.currentRun(p.ID)
	if err != nil {
		return err
	}
	if abnormalTask.TestNo != abnormalTestNo || abnormalTask.Status != "running" {
		return fmt.Errorf("unexpected abnormal-stop task: %+v", abnormalTask)
	}
	abnormalCommandID := "cmd-eb045-abnormal-stop-" + suffix
	if err := c.abnormalStopRunWS(abnormalTask.ID, "smoke abnormal stop", abnormalCommandID); err != nil {
		return err
	}
	if abnormalStopped, err := c.waitRunEnd(abnormalTask.ID, "abnormal_stop", wait); err != nil {
		return err
	} else if abnormalStopped.Status != "stopped" {
		return fmt.Errorf("expected abnormal stopped task, got %+v", abnormalStopped)
	}
	abnormalEvents, err := c.getRunEvents(abnormalTask.ID)
	if err != nil {
		return err
	}
	if !hasEvent(abnormalEvents.Items, "run_abnormal_stop") {
		return fmt.Errorf("run_abnormal_stop event not found for task=%d events=%+v", abnormalTask.ID, abnormalEvents.Items)
	}
	abnormalNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.run_abnormal_stop", p.ID, abnormalTask.ID, wait)
	if err != nil {
		return err
	}
	if audit, err := c.waitWSCommandAuditByType("command.detection.abnormal_stop", abnormalCommandID, wait); err != nil {
		return err
	} else if audit.Result != "success" {
		return fmt.Errorf("unexpected abnormal-stop WS audit result: %+v", audit)
	}
	fmt.Printf("abnormal stop request ok: lifecycle_notification=%s\n", abnormalNotification.EventUID)

	if err := c.writeVariableWS(measurementVar.VarID, 43, false, "cmd-eb045-ng-value-"+suffix); err != nil {
		return err
	}
	ngTestNo := "SMOKE-EB045-NG-" + suffix
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{
		"command":             "start_detection",
		"project_id":          p.ID,
		"test_no":             ngTestNo,
		"mode":                "smoke_task_flow",
		"limit_check_enabled": true,
		"end_policy":          "manual",
		"enable_storage":      false,
		"enable_alarm":        true,
		"operator_note":       "EB-045 smoke NG notification",
		"custom_items": []map[string]any{{
			"var_id":            fmt.Sprintf("%d", measurementVar.VarID),
			"var_name":          "smoke_measurement",
			"display_name":      "Smoke measurement",
			"check_enabled":     true,
			"alarm_enabled":     true,
			"store_enabled":     false,
			"check_on_start":    true,
			"check_cycle_ms":    100,
			"limit_l":           0,
			"limit_h":           42,
			"limit_hh":          45,
			"limit_deadband":    0.1,
			"violation_hold_ms": 0,
			"recover_hold_ms":   0,
			"quality_policy":    "ignore_bad",
			"decimal_places":    2,
		}},
	}, "ng-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRunAfter(flows["start"].FlowCode, requestVar.VarID, lastStartRunID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("NG start flow status=%s error=%s result=%s", run.Status, run.ErrorMessage, run.ResultJSON)
	} else {
		lastStartRunID = run.ID
	}
	ngTask, err := c.currentRun(p.ID)
	if err != nil {
		return err
	}
	if ngTask.TestNo != ngTestNo || ngTask.Status != "running" {
		return fmt.Errorf("unexpected NG task: %+v", ngTask)
	}
	alarmNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.enter", p.ID, ngTask.ID, wait)
	if err != nil {
		return err
	}
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "mute_detection_alarms", "task_id": ngTask.ID}, "mute-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRun(flows["mute"].FlowCode, requestVar.VarID, wait); err != nil {
		return err
	} else if run.Status != "success" || !strings.Contains(run.ResultJSON, `"muted":1`) {
		return fmt.Errorf("mute detection alarms flow unexpected status=%s result=%s error=%s", run.Status, run.ResultJSON, run.ErrorMessage)
	}
	if err := c.writeVariableWS(measurementVar.VarID, 50, false, "cmd-eb045-ng-level-change-value-"+suffix); err != nil {
		return err
	}
	levelChangeNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.level_change", p.ID, ngTask.ID, wait)
	if err != nil {
		return err
	}
	if err := c.writeVariableWS(measurementVar.VarID, 25, false, "cmd-eb045-ng-recover-value-"+suffix); err != nil {
		return err
	}
	recoverNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.recover", p.ID, ngTask.ID, wait)
	if err != nil {
		return err
	}
	if err := c.writeTaskRequest(requestVar.VarID, map[string]any{"command": "stop_detection", "project_id": p.ID, "reason": "smoke NG stop"}, "ng-stop-"+suffix); err != nil {
		return err
	}
	if run, err := c.waitTaskFlowRunAfter(flows["stop"].FlowCode, requestVar.VarID, stopRun.ID, wait); err != nil {
		return err
	} else if run.Status != "success" {
		return fmt.Errorf("NG stop flow status=%s error=%s", run.Status, run.ErrorMessage)
	}
	ngStopped, err := c.getRun(ngTask.ID)
	if err != nil {
		return err
	}
	if ngStopped.Status != "stopped" || ngStopped.EndType != "task_flow_stop" {
		return fmt.Errorf("expected NG task_flow_stop, got %+v", ngStopped)
	}
	ngNotification, err := c.verifyNotificationReadCycle(notifyConn, "detection.result_ng", p.ID, ngTask.ID, wait)
	if err != nil {
		return err
	}
	fmt.Printf("NG notification request ok: alarm_notification=%s muted=1 level_change_notification=%s recover_notification=%s result_notification=%s\n", alarmNotification.EventUID, levelChangeNotification.EventUID, recoverNotification.EventUID, ngNotification.EventUID)

	if err := c.writeVariableWS(defaultAlarmVar.VarID, 43, false, "cmd-eb045-default-alarm-enter-"+suffix); err != nil {
		return err
	}
	defaultEnterNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.enter", p.ID, 0, wait)
	if err != nil {
		return err
	}
	if err := c.writeVariableWS(defaultAlarmVar.VarID, 50, false, "cmd-eb045-default-alarm-level-change-"+suffix); err != nil {
		return err
	}
	defaultLevelChangeNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.level_change", p.ID, 0, wait)
	if err != nil {
		return err
	}
	if err := c.writeVariableWS(defaultAlarmVar.VarID, 25, false, "cmd-eb045-default-alarm-recover-"+suffix); err != nil {
		return err
	}
	defaultRecoverNotification, err := c.verifyNotificationReadCycle(notifyConn, "alarm.limit.recover", p.ID, 0, wait)
	if err != nil {
		return err
	}
	defaultAlarm, err := c.waitLimitAlarm("default", p.ID, defaultAlarmVar.VarID, "recovered", wait)
	if err != nil {
		return err
	}
	if defaultAlarm.TaskID != 0 || defaultAlarm.AlarmType != "above_hh" || defaultAlarm.AlarmLevel != "HH" {
		return fmt.Errorf("unexpected default alarm record: %+v", defaultAlarm)
	}
	if audit, err := c.waitWSCommandAudit("cmd-eb045-default-alarm-recover-"+suffix, wait); err != nil {
		return err
	} else if audit.Result != "success" {
		return fmt.Errorf("unexpected default alarm WS audit result: %+v", audit)
	}
	fmt.Printf("default alarm notification ok: enter=%s level_change=%s recover=%s alarm_id=%d\n", defaultEnterNotification.EventUID, defaultLevelChangeNotification.EventUID, defaultRecoverNotification.EventUID, defaultAlarm.ID)

	fmt.Printf("EB-045 smoke passed: project=%d request_var=%d control_var=%d measurement_var=%d default_alarm_var=%d task=%d fixed_task=%d qualified_task=%d abnormal_task=%d ng_task=%d\n", p.ID, requestVar.VarID, controlVar.VarID, measurementVar.VarID, defaultAlarmVar.VarID, task.ID, fixedTask.ID, qualifiedTask.ID, abnormalTask.ID, ngTask.ID)
	return nil
}

func (c *client) login(username, password string) error {
	var out loginResponse
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, &out); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("login response has empty access_token")
	}
	c.token = out.AccessToken
	return nil
}

func (c *client) createProject(suffix string) (project, error) {
	var p project
	code := "SMOKE-EB045-" + suffix
	err := c.do(http.MethodPost, "/api/v1/projects", map[string]any{
		"project_code":    code,
		"name":            "EB045 Smoke",
		"display_name":    "EB045 Smoke",
		"display_name_en": "EB045 Smoke",
		"display_name_ja": "EB045 Smoke",
	}, &p)
	return p, err
}

func (c *client) createVirtualVariable(p project, dataType string, name string) (tag, error) {
	var out tag
	payload := map[string]any{
		"var_id":          safeVarID(),
		"source_type":     "virtual",
		"project_id":      p.ID,
		"var_group":       "EB045 smoke",
		"var_name":        name,
		"display_name":    name,
		"display_name_en": name,
		"display_name_ja": name,
		"data_type":       dataType,
		"enabled":         true,
	}
	if err := c.do(http.MethodPost, "/api/v1/variables", payload, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *client) patchVariableDefaultAlarm(varID int64) error {
	var out tag
	payload := map[string]any{
		"default_alarm_enabled":     true,
		"default_limit_l":           0,
		"default_limit_h":           42,
		"default_limit_hh":          45,
		"default_limit_deadband":    0.1,
		"default_violation_hold_ms": 0,
		"default_recover_hold_ms":   0,
	}
	if err := c.do(http.MethodPatch, fmt.Sprintf("/api/v1/variables/%d", varID), payload, &out); err != nil {
		return err
	}
	if out.VarID != varID {
		return fmt.Errorf("variable default alarm patch returned unexpected var_id=%d want=%d", out.VarID, varID)
	}
	return nil
}

func (c *client) enableCycleRoute(projectID uint, varID int64) error {
	var routes []storageRoute
	path := fmt.Sprintf("/api/v1/storage-routes?project_id=%d&var_id=%d", projectID, varID)
	if err := c.do(http.MethodGet, path, nil, &routes); err != nil {
		return err
	}
	if len(routes) == 0 {
		return fmt.Errorf("no default storage route for var_id=%d", varID)
	}
	patch := map[string]any{
		"enabled":        true,
		"trigger_mode":   "on_cycle",
		"cycle_ms":       500,
		"store_on_start": true,
	}
	var updated storageRoute
	if err := c.do(http.MethodPatch, fmt.Sprintf("/api/v1/storage-routes/%d", routes[0].ID), patch, &updated); err != nil {
		return err
	}
	if !updated.Enabled || updated.CycleMS != 500 {
		return fmt.Errorf("storage route update did not apply: %+v", updated)
	}
	return nil
}

func (c *client) createTaskFlows(p project, requestVar tag, suffix string) (map[string]taskFlow, error) {
	defs := []struct {
		key       string
		command   string
		action    string
		steps     []map[string]any
		condition string
	}{
		{
			key:     "start",
			command: "start_detection",
			action:  "builtin.start_detection_run",
			steps: []map[string]any{
				{
					"code":   "control",
					"module": "builtin.write_control_variables",
					"params": map[string]any{"items": map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true}},
				},
				{
					"code":   "start",
					"module": "builtin.start_detection_run",
					"params": map[string]any{
						"project_id":          map[string]any{"source": "trigger_param", "key": "project_id"},
						"test_no":             map[string]any{"source": "trigger_param", "key": "test_no", "optional": true},
						"mode":                map[string]any{"source": "trigger_param", "key": "mode", "default": "smoke_task_flow"},
						"limit_check_enabled": map[string]any{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
						"end_policy":          map[string]any{"source": "trigger_param", "key": "end_policy", "default": "manual"},
						"duration_sec":        map[string]any{"source": "trigger_param", "key": "duration_sec", "optional": true},
						"qualified_hold_ms":   map[string]any{"source": "trigger_param", "key": "qualified_hold_ms", "optional": true},
						"operator_note":       map[string]any{"source": "trigger_param", "key": "operator_note", "optional": true},
						"custom_items":        map[string]any{"source": "trigger_param", "key": "custom_items", "optional": true},
						"process_params":      map[string]any{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":          map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true},
					},
				},
			},
		},
		{
			key:     "fixed",
			command: "start_fixed_duration_detection",
			action:  "builtin.start_detection_run",
			steps: []map[string]any{
				{
					"code":   "control",
					"module": "builtin.write_control_variables",
					"params": map[string]any{"items": map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true}},
				},
				{
					"code":   "start",
					"module": "builtin.start_detection_run",
					"params": map[string]any{
						"project_id":          map[string]any{"source": "trigger_param", "key": "project_id"},
						"test_no":             map[string]any{"source": "trigger_param", "key": "test_no", "optional": true},
						"mode":                map[string]any{"source": "trigger_param", "key": "mode", "default": "smoke_task_flow"},
						"limit_check_enabled": map[string]any{"source": "trigger_param", "key": "limit_check_enabled", "default": false},
						"end_policy":          map[string]any{"source": "literal", "value": "fixed_duration"},
						"duration_sec":        map[string]any{"source": "trigger_param", "key": "duration_sec"},
						"operator_note":       map[string]any{"source": "trigger_param", "key": "operator_note", "optional": true},
						"custom_items":        map[string]any{"source": "trigger_param", "key": "custom_items", "optional": true},
						"process_params":      map[string]any{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":          map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true},
						"enable_storage":      map[string]any{"source": "trigger_param", "key": "enable_storage", "default": false},
						"enable_alarm":        map[string]any{"source": "trigger_param", "key": "enable_alarm", "default": false},
					},
				},
			},
		},
		{
			key:     "qualified",
			command: "start_qualified_hold_detection",
			action:  "builtin.start_detection_run",
			steps: []map[string]any{
				{
					"code":   "control",
					"module": "builtin.write_control_variables",
					"params": map[string]any{"items": map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true}},
				},
				{
					"code":   "start",
					"module": "builtin.start_detection_run",
					"params": map[string]any{
						"project_id":          map[string]any{"source": "trigger_param", "key": "project_id"},
						"test_no":             map[string]any{"source": "trigger_param", "key": "test_no", "optional": true},
						"mode":                map[string]any{"source": "trigger_param", "key": "mode", "default": "smoke_task_flow"},
						"limit_check_enabled": map[string]any{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
						"end_policy":          map[string]any{"source": "literal", "value": "qualified_hold"},
						"qualified_hold_ms":   map[string]any{"source": "trigger_param", "key": "qualified_hold_ms"},
						"operator_note":       map[string]any{"source": "trigger_param", "key": "operator_note", "optional": true},
						"custom_items":        map[string]any{"source": "trigger_param", "key": "custom_items"},
						"process_params":      map[string]any{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":          map[string]any{"source": "trigger_param", "key": "plc_writes", "optional": true},
						"enable_storage":      map[string]any{"source": "trigger_param", "key": "enable_storage", "default": false},
						"enable_alarm":        map[string]any{"source": "trigger_param", "key": "enable_alarm", "default": true},
					},
				},
			},
		},
		{
			key:     "prepare",
			command: "prepare_storage",
			action:  "builtin.storage_prepare",
			steps: []map[string]any{{
				"code":   "prepare",
				"module": "builtin.storage_prepare",
				"params": map[string]any{
					"project_id": map[string]any{"source": "trigger_param", "key": "project_id"},
				},
			}},
		},
		{
			key:     "update",
			command: "update_limits",
			action:  "builtin.update_detection_limits",
			steps: []map[string]any{{
				"code":   "update",
				"module": "builtin.update_detection_limits",
				"params": map[string]any{
					"project_id": map[string]any{"source": "trigger_param", "key": "project_id"},
					"items":      map[string]any{"source": "trigger_param", "key": "items"},
				},
			}},
		},
		{
			key:     "report",
			command: "register_report",
			action:  "builtin.register_report",
			steps: []map[string]any{{
				"code":   "report",
				"module": "builtin.register_report",
				"params": map[string]any{
					"task_id":       map[string]any{"source": "trigger_param", "key": "task_id"},
					"file_ref":      map[string]any{"source": "trigger_param", "key": "file_ref"},
					"file_name":     map[string]any{"source": "trigger_param", "key": "file_name", "optional": true},
					"status":        map[string]any{"source": "trigger_param", "key": "status", "default": "generated"},
					"template_id":   map[string]any{"source": "trigger_param", "key": "template_id", "optional": true},
					"template_code": map[string]any{"source": "trigger_param", "key": "template_code", "optional": true},
				},
			}},
		},
		{
			key:     "http",
			command: "http_request",
			action:  "builtin.http_request",
			steps: []map[string]any{{
				"code":   "http",
				"module": "builtin.http_request",
				"params": map[string]any{
					"method":     map[string]any{"source": "trigger_param", "key": "method", "default": "POST"},
					"url":        map[string]any{"source": "trigger_param", "key": "url"},
					"body":       map[string]any{"source": "trigger_param", "key": "body", "optional": true},
					"timeout_ms": map[string]any{"source": "trigger_param", "key": "timeout_ms", "optional": true},
				},
			}},
		},
		{
			key:     "js",
			command: "run_javascript",
			action:  "javascript",
			steps: []map[string]any{{
				"code":   "js",
				"module": "javascript",
				"params": map[string]any{
					"expected": map[string]any{"source": "trigger_param", "key": "expected", "default": 7},
				},
				"script": `const rows = db.query("SELECT 1 AS ok", []); const vars = realtime.project(); log.info("js_smoke rows=" + rows.length + " vars=" + vars.length); ({ok: rows[0].ok, expected: params.expected, project_vars: vars.length, trigger_var_id_text: trigger.var_id_text});`,
			}},
		},
		{
			key:     "features",
			command: "refresh_features",
			action:  "builtin.refresh_features",
			steps: []map[string]any{{
				"code":   "features",
				"module": "builtin.refresh_features",
				"params": map[string]any{
					"task_id": map[string]any{"source": "trigger_param", "key": "task_id"},
				},
			}},
		},
		{
			key:     "pause",
			command: "pause_detection",
			action:  "builtin.pause_detection_run",
			steps: []map[string]any{{
				"code":   "pause",
				"module": "builtin.pause_detection_run",
				"params": map[string]any{
					"project_id": map[string]any{"source": "trigger_param", "key": "project_id"},
					"reason":     map[string]any{"source": "trigger_param", "key": "reason", "optional": true},
				},
			}},
		},
		{
			key:     "resume",
			command: "resume_detection",
			action:  "builtin.resume_detection_run",
			steps: []map[string]any{{
				"code":   "resume",
				"module": "builtin.resume_detection_run",
				"params": map[string]any{"task_id": map[string]any{"source": "trigger_param", "key": "task_id"}},
			}},
		},
		{
			key:     "stop",
			command: "stop_detection",
			action:  "builtin.stop_detection_run",
			steps: []map[string]any{{
				"code":   "stop",
				"module": "builtin.stop_detection_run",
				"params": map[string]any{
					"project_id": map[string]any{"source": "trigger_param", "key": "project_id"},
					"reason":     map[string]any{"source": "trigger_param", "key": "reason", "optional": true},
				},
			}},
		},
		{
			key:     "mute",
			command: "mute_detection_alarms",
			action:  "builtin.mute_detection_alarms",
			steps: []map[string]any{{
				"code":   "mute",
				"module": "builtin.mute_detection_alarms",
				"params": map[string]any{
					"task_id":    map[string]any{"source": "trigger_param", "key": "task_id", "optional": true},
					"project_id": map[string]any{"source": "trigger_param", "key": "project_id", "optional": true},
				},
			}},
		},
	}
	out := make(map[string]taskFlow, len(defs))
	for _, def := range defs {
		stepsJSON, err := json.Marshal(def.steps)
		if err != nil {
			return nil, err
		}
		var flow taskFlow
		payload := map[string]any{
			"project_id":       p.ID,
			"flow_code":        "smoke-eb045-" + def.key + "-" + suffix,
			"name":             "EB045 smoke " + def.key,
			"enabled":          true,
			"trigger_type":     "data_change",
			"condition_script": fmt.Sprintf(`task_params.command === %q`, def.command),
			"action_type":      def.action,
			"steps_json":       string(stepsJSON),
			"timeout_ms":       5000,
			"cooldown_ms":      0,
			"priority":         100,
			"vars": []map[string]any{{
				"var_id":   fmt.Sprintf("%d", requestVar.VarID),
				"var_name": "request",
				"role":     "watch",
			}},
		}
		if err := c.do(http.MethodPost, "/api/v1/task-flows", payload, &flow); err != nil {
			return nil, err
		}
		out[def.key] = flow
	}
	return out, nil
}

func (c *client) createScheduledJSFlow(p project, suffix string) (taskFlow, error) {
	var flow taskFlow
	payload := map[string]any{
		"project_id":           p.ID,
		"flow_code":            "smoke-eb045-schedule-js-" + suffix,
		"name":                 "EB045 smoke schedule JS",
		"enabled":              true,
		"trigger_type":         "schedule",
		"action_type":          "javascript",
		"action_script":        `log.info("scheduled smoke " + trigger.type); ({trigger_type: trigger.type, project_id: project.id});`,
		"timeout_ms":           3000,
		"schedule_interval_ms": 1000,
		"priority":             10,
	}
	err := c.do(http.MethodPost, "/api/v1/task-flows", payload, &flow)
	return flow, err
}

func (c *client) disableTaskFlow(flowID uint64) error {
	var flow taskFlow
	return c.do(http.MethodPatch, fmt.Sprintf("/api/v1/task-flows/%d", flowID), map[string]any{"enabled": false}, &flow)
}

func findStandardItem(items []detectionRunStandardItem, varID int64) *detectionRunStandardItem {
	for i := range items {
		if items[i].VarID == varID {
			return &items[i]
		}
	}
	return nil
}

func hasReport(reports []detectionRunReport, fileRef string) bool {
	for _, report := range reports {
		if report.FileRef == fileRef && report.Status == "generated" {
			return true
		}
	}
	return false
}

func hasEvent(events []detectionRunEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func (c *client) writeTaskRequest(varID int64, payload map[string]any, commandID string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.writeVariableWS(varID, string(raw), true, "cmd-eb045-"+commandID)
}

func (c *client) writeVariableWS(varID int64, value any, trigger bool, commandID string) error {
	return c.writeWSCommand("realtime.variables", 0, "command.write_variable", commandID, map[string]any{
		"var_id":  fmt.Sprintf("%d", varID),
		"value":   value,
		"trigger": trigger,
	})
}

func (c *client) abnormalStopRunWS(taskID uint, reason string, commandID string) error {
	return c.writeWSCommand("detection.tasks", 0, "command.detection.abnormal_stop", commandID, map[string]any{
		"task_id": taskID,
		"reason":  reason,
	})
}

func (c *client) writeWSCommand(topic string, projectID uint, commandType string, commandID string, payload map[string]any) error {
	wsURL, err := c.wsURL(topic, projectID)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.Type == "connection.ready" {
			break
		}
	}
	if err := conn.WriteJSON(map[string]any{
		"type":       commandType,
		"request_id": "req-" + commandID,
		"command_id": commandID,
		"payload":    payload,
	}); err != nil {
		return err
	}
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.CommandID != commandID {
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("ws command %s failed: %s %s", commandType, msg.Error.Code, msg.Error.Message)
		}
		if msg.Type != "command.ack" {
			return fmt.Errorf("ws command %s returned unexpected type %s", commandType, msg.Type)
		}
		return nil
	}
}

func (c *client) openNotificationWS(projectID uint) (*websocket.Conn, error) {
	wsURL, err := c.wsURL("notifications", projectID)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if msg.Type == "connection.ready" {
			return conn, nil
		}
	}
}

func (c *client) waitNotificationWS(conn *websocket.Conn, notificationType string, taskID uint, timeout time.Duration) (runtimeNotification, error) {
	deadline := time.Now().Add(timeout)
	for {
		if remaining := time.Until(deadline); remaining <= 0 {
			return runtimeNotification{}, fmt.Errorf("notification ws timeout type=%s task=%d", notificationType, taskID)
		} else {
			_ = conn.SetReadDeadline(time.Now().Add(remaining))
		}
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return runtimeNotification{}, err
		}
		if msg.Type != "notification.event" {
			continue
		}
		var notification runtimeNotification
		if err := json.Unmarshal(msg.Payload, &notification); err != nil {
			return runtimeNotification{}, fmt.Errorf("decode notification payload: %w body=%s", err, string(msg.Payload))
		}
		if notification.Type == notificationType && notification.TaskID == taskID {
			return notification, nil
		}
	}
}

func (c *client) waitTaskFlowRun(flowCode string, triggerVarID int64, timeout time.Duration) (taskFlowRun, error) {
	return c.waitTaskFlowRunAfter(flowCode, triggerVarID, 0, timeout)
}

func (c *client) waitTaskFlowRunAfter(flowCode string, triggerVarID int64, minRunID uint64, timeout time.Duration) (taskFlowRun, error) {
	deadline := time.Now().Add(timeout)
	var last taskFlowRun
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/api/v1/task-flow-runs?flow_code=%s&trigger_var_id=%d&limit=5", url.QueryEscape(flowCode), triggerVarID)
		var out listResponse[taskFlowRun]
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return last, err
		}
		for _, run := range out.Items {
			if run.FlowCode == flowCode && run.TriggerVarID == triggerVarID && run.ID > minRunID {
				last = run
				switch run.Status {
				case "success", "failed", "timeout":
					return run, nil
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last, fmt.Errorf("task flow run not finished for flow_code=%s min_run_id=%d last=%+v", flowCode, minRunID, last)
}

func (c *client) waitTaskFlowRunByFlowCode(flowCode string, triggerType string, timeout time.Duration) (taskFlowRun, error) {
	deadline := time.Now().Add(timeout)
	var last taskFlowRun
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/api/v1/task-flow-runs?flow_code=%s&trigger_type=%s&limit=5", url.QueryEscape(flowCode), url.QueryEscape(triggerType))
		var out listResponse[taskFlowRun]
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return last, err
		}
		for _, run := range out.Items {
			if run.FlowCode == flowCode && run.TriggerType == triggerType {
				last = run
				switch run.Status {
				case "success", "failed", "timeout":
					return run, nil
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last, fmt.Errorf("task flow run not finished for flow_code=%s trigger_type=%s last=%+v", flowCode, triggerType, last)
}

func (c *client) getTaskFlowSQLLogs(runID uint64) ([]taskFlowSQLLog, error) {
	var out []taskFlowSQLLog
	err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/task-flow-runs/%d/sql-logs", runID), nil, &out)
	return out, err
}

func (c *client) currentRun(projectID uint) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/detection-runs/current?project_id=%d", projectID), nil, &out)
	return out, err
}

func (c *client) getRun(taskID uint) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/detection-runs/%d", taskID), nil, &out)
	return out, err
}

func (c *client) getRunEvents(taskID uint) (listResponse[detectionRunEvent], error) {
	var out listResponse[detectionRunEvent]
	err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/detection-runs/%d/events", taskID), nil, &out)
	return out, err
}

func (c *client) waitRunEnd(taskID uint, endType string, timeout time.Duration) (detectionTask, error) {
	deadline := time.Now().Add(timeout)
	var last detectionTask
	for time.Now().Before(deadline) {
		task, err := c.getRun(taskID)
		if err != nil {
			return last, err
		}
		last = task
		if task.Status == "stopped" {
			if endType == "" || task.EndType == endType {
				return task, nil
			}
			return task, fmt.Errorf("task %d stopped with end_type=%s, want %s", taskID, task.EndType, endType)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last, fmt.Errorf("task %d did not stop with end_type=%s last=%+v", taskID, endType, last)
}

func (c *client) waitUserNotification(notificationType string, projectID uint, taskID uint, eventUID string, timeout time.Duration) (userNotification, error) {
	deadline := time.Now().Add(timeout)
	var last []userNotification
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/api/v1/notifications?type=%s&project_id=%d&limit=20", url.QueryEscape(notificationType), projectID)
		var out listResponse[userNotification]
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return userNotification{}, err
		}
		last = out.Items
		for _, item := range out.Items {
			if item.Type != notificationType || item.TaskID != taskID {
				continue
			}
			if eventUID == "" || item.EventUID == eventUID {
				return item, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return userNotification{}, fmt.Errorf("notification not found type=%s task=%d event_uid=%s last=%+v", notificationType, taskID, eventUID, last)
}

func (c *client) waitLimitAlarm(scope string, projectID uint, varID int64, status string, timeout time.Duration) (detectionLimitAlarm, error) {
	deadline := time.Now().Add(timeout)
	var last []detectionLimitAlarm
	for time.Now().Before(deadline) {
		path := fmt.Sprintf("/api/v1/limit-alarms?scope=%s&project_id=%d&var_id=%d&status=%s&limit=10", url.QueryEscape(scope), projectID, varID, url.QueryEscape(status))
		var out listResponse[detectionLimitAlarm]
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return detectionLimitAlarm{}, err
		}
		last = out.Items
		for _, item := range out.Items {
			if item.Scope == scope && item.ProjectID == projectID && item.VarID == varID && item.Status == status {
				return item, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return detectionLimitAlarm{}, fmt.Errorf("limit alarm not found scope=%s project=%d var_id=%d status=%s last=%+v", scope, projectID, varID, status, last)
}

func (c *client) waitWSCommandAudit(commandID string, timeout time.Duration) (auditLog, error) {
	return c.waitWSCommandAuditByType("command.write_variable", commandID, timeout)
}

func (c *client) waitWSCommandAuditByType(commandType string, commandID string, timeout time.Duration) (auditLog, error) {
	escapedCommand := url.QueryEscape(commandType)
	return c.waitAuditLog(
		fmt.Sprintf("/api/v1/audit-logs?action=ws.%s&target_type=ws_command&target_id=%s&result=success&limit=200", escapedCommand, escapedCommand),
		func(item auditLog) bool {
			return item.TargetID == commandType && strings.Contains(item.Detail, commandID)
		},
		fmt.Sprintf("ws %s command_id=%s", commandType, commandID),
		timeout,
	)
}

func (c *client) waitTaskFlowWriteAudit(varID int64, timeout time.Duration) (auditLog, error) {
	path := fmt.Sprintf("/api/v1/audit-logs?action=task_flow.write_variable&target_type=variable&target_id=%d&result=success&limit=50", varID)
	return c.waitAuditLog(
		path,
		func(item auditLog) bool {
			return item.TargetID == fmt.Sprintf("%d", varID)
		},
		fmt.Sprintf("task_flow.write_variable var_id=%d", varID),
		timeout,
	)
}

func (c *client) waitAuditLog(path string, match func(auditLog) bool, label string, timeout time.Duration) (auditLog, error) {
	deadline := time.Now().Add(timeout)
	var last []auditLog
	for time.Now().Before(deadline) {
		var out listResponse[auditLog]
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return auditLog{}, err
		}
		last = out.Items
		for _, item := range out.Items {
			if match(item) {
				return item, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return auditLog{}, fmt.Errorf("audit log not found for %s last=%+v", label, last)
}

func (c *client) verifyNotificationReadCycle(conn *websocket.Conn, notificationType string, projectID uint, taskID uint, timeout time.Duration) (userNotification, error) {
	notification, err := c.waitNotificationWS(conn, notificationType, taskID, timeout)
	if err != nil {
		return userNotification{}, err
	}
	persisted, err := c.waitUserNotification(notificationType, projectID, taskID, notification.ID, timeout)
	if err != nil {
		return userNotification{}, err
	}
	if persisted.ReadAt != nil {
		return userNotification{}, fmt.Errorf("new notification should be unread: %+v", persisted)
	}
	if err := c.markNotificationRead(persisted.ID); err != nil {
		return userNotification{}, err
	}
	readBack, err := c.waitUserNotification(notificationType, projectID, taskID, notification.ID, timeout)
	if err != nil {
		return userNotification{}, err
	}
	if readBack.ReadAt == nil {
		return userNotification{}, fmt.Errorf("notification was not marked read: %+v", readBack)
	}
	return readBack, nil
}

func (c *client) markNotificationRead(id uint64) error {
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/notifications/%d/read", id), map[string]any{}, nil)
}

func (c *client) stopRun(taskID uint, reason string) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodPost, fmt.Sprintf("/api/v1/detection-runs/%d/stop", taskID), map[string]any{"reason": reason}, &out)
	return out, err
}

func (c *client) wsURL(topic string, projectID uint) (string, error) {
	parsed, err := url.Parse(c.base)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/ws"
	query := parsed.Query()
	query.Set("access_token", c.token)
	query.Set("topic", topic)
	if projectID > 0 {
		query.Set("project_id", fmt.Sprintf("%d", projectID))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *client) do(method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w body=%s", method, path, err, string(raw))
	}
	return nil
}

func safeVarID() int64 {
	return 720_000_000 + rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(170_000_000)
}
