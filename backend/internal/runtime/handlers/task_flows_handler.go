package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type TaskFlowsHandler struct {
	repo  *database.Repository
	flows *pipeline.TaskFlowExecutor
}

type taskFlowRequest struct {
	ProjectID          uint                 `json:"project_id"`
	FlowCode           string               `json:"flow_code"`
	Name               string               `json:"name"`
	Enabled            *bool                `json:"enabled"`
	TriggerType        string               `json:"trigger_type"`
	ConditionScript    string               `json:"condition_script"`
	ActionType         string               `json:"action_type"`
	ActionScript       string               `json:"action_script"`
	ActionPayload      string               `json:"action_payload"`
	StepsJSON          string               `json:"steps_json"`
	TimeoutMS          int                  `json:"timeout_ms"`
	CooldownMS         int                  `json:"cooldown_ms"`
	HoldMS             int                  `json:"hold_ms"`
	ScheduleIntervalMS int                  `json:"schedule_interval_ms"`
	Priority           int                  `json:"priority"`
	Remark             string               `json:"remark"`
	Vars               []taskFlowVarRequest `json:"vars"`
}

type taskFlowPatchRequest struct {
	ProjectID          *uint                 `json:"project_id"`
	FlowCode           *string               `json:"flow_code"`
	Name               *string               `json:"name"`
	Enabled            *bool                 `json:"enabled"`
	TriggerType        *string               `json:"trigger_type"`
	ConditionScript    *string               `json:"condition_script"`
	ActionType         *string               `json:"action_type"`
	ActionScript       *string               `json:"action_script"`
	ActionPayload      *string               `json:"action_payload"`
	StepsJSON          *string               `json:"steps_json"`
	TimeoutMS          *int                  `json:"timeout_ms"`
	CooldownMS         *int                  `json:"cooldown_ms"`
	HoldMS             *int                  `json:"hold_ms"`
	ScheduleIntervalMS *int                  `json:"schedule_interval_ms"`
	Priority           *int                  `json:"priority"`
	Remark             *string               `json:"remark"`
	Vars               *[]taskFlowVarRequest `json:"vars"`
}

type taskFlowVarRequest struct {
	VarID   flexibleInt64 `json:"var_id"`
	VarName string        `json:"var_name"`
	Role    string        `json:"role"`
}

type taskFlowStepRequest struct {
	Code   string         `json:"code"`
	Module string         `json:"module"`
	Params map[string]any `json:"params"`
}

func NewTaskFlowsHandler(repo *database.Repository, flows *pipeline.TaskFlowExecutor) *TaskFlowsHandler {
	return &TaskFlowsHandler{repo: repo, flows: flows}
}

func (h *TaskFlowsHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/task-modules", authService.RequirePermission(auth.PermSystemSettings), h.modules)
	group.GET("/task-flow-templates", authService.RequirePermission(auth.PermSystemSettings), h.templates)
	group.GET("/task-flows", authService.RequirePermission(auth.PermSystemSettings), h.list)
	group.GET("/task-flows/runtime", authService.RequirePermission(auth.PermSystemSettings), h.runtime)
	group.GET("/task-flows/:id", authService.RequirePermission(auth.PermSystemSettings), h.get)
	group.POST("/task-flows", authService.RequirePermission(auth.PermSystemSettings), h.create)
	group.PATCH("/task-flows/:id", authService.RequirePermission(auth.PermSystemSettings), h.patch)
	group.DELETE("/task-flows/:id", authService.RequirePermission(auth.PermSystemSettings), h.delete)
	group.POST("/task-flows/:id/run", authService.RequirePermission(auth.PermSystemSettings), h.run)
	group.GET("/task-flow-runs", authService.RequirePermission(auth.PermSystemSettings), h.listRuns)
	group.GET("/task-flow-runs/:id", authService.RequirePermission(auth.PermSystemSettings), h.getRun)
	group.GET("/task-flow-runs/:id/sql-logs", authService.RequirePermission(auth.PermSystemSettings), h.listRunSQLLogs)
}

func (h *TaskFlowsHandler) modules(c *gin.Context) {
	c.JSON(http.StatusOK, []gin.H{
		{
			"code":          models.TaskFlowActionBuiltinStartDetectionRun,
			"name":          "启动检测任务",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart},
			"params_schema": gin.H{
				"project_id":            gin.H{"type": "project", "required": true, "source": []string{"literal", "trigger_param", "event", "context"}},
				"test_no":               gin.H{"type": "string", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"mode":                  gin.H{"type": "string", "required": false, "default": "task_flow"},
				"standard_id":           gin.H{"type": "detection_standard", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"custom_items":          gin.H{"type": "array", "required": false, "source": []string{"trigger_param", "context"}},
				"limit_check_enabled":   gin.H{"type": "boolean", "required": false, "default": true, "source": []string{"literal", "trigger_param", "context"}},
				"end_policy":            gin.H{"type": "select", "required": false, "default": models.DetectionEndPolicyManual, "options": []string{models.DetectionEndPolicyManual, models.DetectionEndPolicyFixedDuration, models.DetectionEndPolicyQualifiedHold}, "source": []string{"literal", "trigger_param", "context"}},
				"duration_sec":          gin.H{"type": "number", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"qualified_hold_ms":     gin.H{"type": "number", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"operator_note":         gin.H{"type": "text", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"report_template_id":    gin.H{"type": "report_template", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"report_request":        gin.H{"type": "object", "required": false, "source": []string{"trigger_param", "context"}, "description": "本次检测需要主服务器生成报表的请求快照；推荐 reports[]，每项选择模板、变量组和 params 参数。旧 variables/var_ids/variable_names/ext_* 仍兼容。"},
				"process_params":        gin.H{"type": "object", "required": false, "source": []string{"trigger_param", "context"}, "description": "本次检测的工艺/报表参数，例如进风口面积、工装编号或环境修正参数；随检测冻结用于追溯。"},
				"plc_writes":            gin.H{"type": "array", "required": false, "source": []string{"trigger_param", "context"}, "description": "本次检测关联的 PLC 下设计划；实际写入应由 builtin.write_control_variables 执行。"},
				"enable_storage":        gin.H{"type": "boolean", "required": false, "default": true},
				"enable_alarm":          gin.H{"type": "boolean", "required": false, "default": true},
				"auto_stop_on_duration": gin.H{"type": "boolean", "required": false, "default": false},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinStopDetectionRun,
			"name":          "结束检测任务",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{
				"task_id":    gin.H{"type": "detection_run", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"project_id": gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}},
				"end_type":   gin.H{"type": "select", "required": false, "options": []string{models.DetectionEndManualStop, models.DetectionEndTaskFlowStop}},
				"reason":     gin.H{"type": "text", "required": false},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinPauseDetectionRun,
			"name":          "暂停检测任务",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": false}, "project_id": gin.H{"type": "project", "required": false}, "reason": gin.H{"type": "text", "required": false}},
		},
		{
			"code":          models.TaskFlowActionBuiltinResumeDetectionRun,
			"name":          "恢复检测任务",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": true}},
		},
		{
			"code":          models.TaskFlowActionBuiltinFixedDurationGuard,
			"name":          "固定时长结束检查",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": false}, "project_id": gin.H{"type": "project", "required": false}},
		},
		{
			"code":          models.TaskFlowActionBuiltinQualifiedHoldGuard,
			"name":          "合格持续时长结束守护",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": false}, "project_id": gin.H{"type": "project", "required": false}, "qualified_hold_ms": gin.H{"type": "number", "required": false}, "check_interval_ms": gin.H{"type": "number", "required": false}},
		},
		{
			"code":          models.TaskFlowActionBuiltinMuteDetectionAlarms,
			"name":          "静音当前检测超限",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": false, "source": []string{"literal", "trigger_param", "context"}}, "project_id": gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}}},
		},
		{
			"code":          models.TaskFlowActionBuiltinRefreshFeatures,
			"name":          "计算检测特征值",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{"task_id": gin.H{"type": "detection_run", "required": true, "source": []string{"literal", "trigger_param", "context"}}},
		},
		{
			"code":          models.TaskFlowActionBuiltinUpdateDetectionLimits,
			"name":          "调整运行中检测限值",
			"category":      "detection",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange},
			"params_schema": gin.H{
				"task_id":           gin.H{"type": "detection_run", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"project_id":        gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}},
				"var_id":            gin.H{"type": "variable", "required": true, "source": []string{"literal", "trigger_param", "context"}},
				"items":             gin.H{"type": "array", "required": false, "source": []string{"trigger_param", "context"}},
				"check_enabled":     gin.H{"type": "boolean", "required": false},
				"alarm_enabled":     gin.H{"type": "boolean", "required": false},
				"store_enabled":     gin.H{"type": "boolean", "required": false},
				"limit_ll":          gin.H{"type": "number", "required": false},
				"limit_l":           gin.H{"type": "number", "required": false},
				"limit_h":           gin.H{"type": "number", "required": false},
				"limit_hh":          gin.H{"type": "number", "required": false},
				"limit_deadband":    gin.H{"type": "number", "required": false},
				"check_cycle_ms":    gin.H{"type": "number", "required": false},
				"violation_hold_ms": gin.H{"type": "number", "required": false},
				"recover_hold_ms":   gin.H{"type": "number", "required": false},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinStorageSnapshot,
			"name":          "存储当前快照",
			"category":      "storage",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{
				"project_id": gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinStoragePrepare,
			"name":          "准备项目宽表",
			"category":      "storage",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule},
			"params_schema": gin.H{
				"task_id":    gin.H{"type": "detection_run", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"project_id": gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinWriteVariable,
			"name":          "写入虚拟变量",
			"category":      "realtime",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{
				"var_id":          gin.H{"type": "variable", "required": true, "source": []string{"literal", "trigger_param", "event", "context"}},
				"value":           gin.H{"type": "any", "required": true, "source": []string{"literal", "trigger_param", "event", "context"}},
				"quality":         gin.H{"type": "number", "required": false, "default": 1},
				"trigger":         gin.H{"type": "boolean", "required": false, "default": true},
				"allow_reentrant": gin.H{"type": "boolean", "required": false, "default": false},
				"max_depth":       gin.H{"type": "number", "required": false, "default": 1},
				"request_id":      gin.H{"type": "string", "required": false},
			},
			"note": "任务流内置写变量当前只允许写 STRING/数值虚拟变量；物理变量下设必须走 WS/HTTP 的 VariableWriteService + KIOWriteService，避免任务执行器绕过现场确认。",
		},
		{
			"code":          models.TaskFlowActionBuiltinWriteControlVariables,
			"name":          "下设控制变量",
			"category":      "realtime",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerProjectStart},
			"params_schema": gin.H{
				"items": gin.H{"type": "array", "required": true, "source": []string{"literal", "trigger_param", "context"}, "item_schema": gin.H{
					"var_id":          gin.H{"type": "variable", "required": true},
					"value":           gin.H{"type": "any", "required": false},
					"value_from":      gin.H{"type": "string", "required": false},
					"quality":         gin.H{"type": "number", "required": false, "default": 1},
					"wait_ack":        gin.H{"type": "boolean", "required": false, "default": true},
					"ack_timeout_sec": gin.H{"type": "number", "required": false, "default": 5},
					"settle_ms":       gin.H{"type": "number", "required": false},
				}},
			},
			"note": "用于检测开始前把进风口面积等工艺参数下设给 PLC；每项写入仍走后端 VariableWriteService/KIOWriteService、变量写入约束和审计日志。",
		},
		{
			"code":          models.TaskFlowActionBuiltinRegisterReport,
			"name":          "登记检测报表结果",
			"category":      "report",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{
				"task_id":          gin.H{"type": "detection_run", "required": false, "source": []string{"literal", "trigger_param", "context"}},
				"project_id":       gin.H{"type": "project", "required": false, "source": []string{"literal", "trigger_param", "event", "context"}},
				"file_ref":         gin.H{"type": "string", "required": true, "source": []string{"literal", "trigger_param", "context"}},
				"file_name":        gin.H{"type": "string", "required": false},
				"status":           gin.H{"type": "select", "required": false, "options": []string{"pending", "generated", "failed"}},
				"template_id":      gin.H{"type": "report_template", "required": false},
				"template_code":    gin.H{"type": "string", "required": false},
				"template_version": gin.H{"type": "number", "required": false},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinHTTPRequest,
			"name":          "调用 HTTP 接口",
			"category":      "developer",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{
				"method":     gin.H{"type": "select", "required": false, "options": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
				"url":        gin.H{"type": "string", "required": true},
				"headers":    gin.H{"type": "object", "required": false},
				"body":       gin.H{"type": "text", "required": false},
				"timeout_ms": gin.H{"type": "number", "required": false},
			},
		},
		{
			"code":          models.TaskFlowActionBuiltinContextSet,
			"name":          "设置上下文变量",
			"category":      "context",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{"*": gin.H{"type": "any", "required": false}},
		},
		{
			"code":          models.TaskFlowActionJavaScript,
			"name":          "执行 JavaScript",
			"category":      "developer",
			"trigger_types": []string{models.TaskFlowTriggerManual, models.TaskFlowTriggerDataChange, models.TaskFlowTriggerSchedule, models.TaskFlowTriggerProjectStart, models.TaskFlowTriggerProjectEnd},
			"params_schema": gin.H{"script": gin.H{"type": "code", "language": "javascript", "required": true}},
			"runtime_api": gin.H{
				"realtime": []string{
					"get(var_id)",
					"getMany([var_id])",
					"getByName(var_name, project_id?)",
					"project(project_id?)",
					"write(var_id, value, options?)",
				},
				"storage": []string{"snapshot({project_id?})"},
				"db":      []string{"query(sql, args)", "exec(sql, args)"},
				"log":     []string{"info(message)", "warn(message)", "error(message)"},
				"note":    "realtime.get/getMany/write 接受数字或字符串形式 var_id，trigger.var_id_text 与 context.trigger_var_id_text 提供精确 ID；realtime.write 复用后端任务写入审计和变量可写约束，默认 trigger=false；需要触发后续任务时显式传 {trigger:true, max_depth:n}。",
			},
		},
	})
}

func (h *TaskFlowsHandler) templates(c *gin.Context) {
	c.JSON(http.StatusOK, []gin.H{
		{
			"template_code": "variable-request-start-detection",
			"name":          "变量请求启动检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后触发检测启动，可选启用存储、报警和固定时长守护。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{
				{
					"code":   "control",
					"module": models.TaskFlowActionBuiltinWriteControlVariables,
					"params": gin.H{
						"items": gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
					},
				},
				{
					"code":   "start",
					"module": models.TaskFlowActionBuiltinStartDetectionRun,
					"params": gin.H{
						"project_id":            gin.H{"source": "trigger_param", "key": "project_id"},
						"test_no":               gin.H{"source": "trigger_param", "key": "test_no", "optional": true},
						"standard_id":           gin.H{"source": "trigger_param", "key": "standard_id", "optional": true},
						"custom_items":          gin.H{"source": "trigger_param", "key": "custom_items", "optional": true},
						"process_params":        gin.H{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":            gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
						"limit_check_enabled":   gin.H{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
						"end_policy":            gin.H{"source": "trigger_param", "key": "end_policy", "default": models.DetectionEndPolicyManual},
						"duration_sec":          gin.H{"source": "trigger_param", "key": "duration_sec", "optional": true},
						"qualified_hold_ms":     gin.H{"source": "trigger_param", "key": "qualified_hold_ms", "optional": true},
						"operator_note":         gin.H{"source": "trigger_param", "key": "operator_note", "optional": true},
						"report_template_id":    gin.H{"source": "trigger_param", "key": "report_template_id", "optional": true},
						"report_request":        gin.H{"source": "trigger_param", "key": "report_request", "optional": true},
						"enable_storage":        gin.H{"source": "trigger_param", "key": "enable_storage", "default": true},
						"enable_alarm":          gin.H{"source": "trigger_param", "key": "enable_alarm", "default": true},
						"auto_stop_on_duration": gin.H{"source": "trigger_param", "key": "auto_stop_on_duration", "default": false},
					},
				},
			},
			"condition_script": `task_params.command === "start_detection"`,
		},
		{
			"template_code": "variable-request-stop-detection",
			"name":          "变量请求结束检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后按 task_id 或 project_id 结束检测。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "stop",
				"module": models.TaskFlowActionBuiltinStopDetectionRun,
				"params": gin.H{
					"task_id":    gin.H{"source": "trigger_param", "key": "task_id", "optional": true},
					"project_id": gin.H{"source": "trigger_param", "key": "project_id", "optional": true},
					"end_type":   gin.H{"source": "trigger_param", "key": "end_type", "optional": true},
					"reason":     gin.H{"source": "trigger_param", "key": "reason", "optional": true},
				},
			}},
			"condition_script": `task_params.command === "stop_detection"`,
		},
		{
			"template_code": "variable-request-storage-snapshot",
			"name":          "变量请求存储快照",
			"category":      "storage",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后将当前项目活动检测任务的可存储变量入队。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "store",
				"module": models.TaskFlowActionBuiltinStorageSnapshot,
				"params": gin.H{"project_id": gin.H{"source": "trigger_param", "key": "project_id", "optional": true}},
			}},
			"condition_script": `task_params.command === "storage_snapshot"`,
		},
		{
			"template_code": "variable-request-mute-detection-alarms",
			"name":          "变量请求静音当前报警",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后将当前活动检测中已经处于报警的变量标记为静音。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "mute",
				"module": models.TaskFlowActionBuiltinMuteDetectionAlarms,
				"params": gin.H{"task_id": gin.H{"source": "trigger_param", "key": "task_id", "optional": true}, "project_id": gin.H{"source": "trigger_param", "key": "project_id", "optional": true}},
			}},
			"condition_script": `task_params.command === "mute_detection_alarms"`,
		},
		{
			"template_code": "variable-request-start-fixed-duration-detection",
			"name":          "变量请求固定时长检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后启动检测，并在 duration_sec 到期后由后端守护自动结束；暂停时长不计入固定时长。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{
				{
					"code":   "control",
					"module": models.TaskFlowActionBuiltinWriteControlVariables,
					"params": gin.H{
						"items": gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
					},
				},
				{
					"code":   "start",
					"module": models.TaskFlowActionBuiltinStartDetectionRun,
					"params": gin.H{
						"project_id":            gin.H{"source": "trigger_param", "key": "project_id"},
						"test_no":               gin.H{"source": "trigger_param", "key": "test_no", "optional": true},
						"standard_id":           gin.H{"source": "trigger_param", "key": "standard_id", "optional": true},
						"custom_items":          gin.H{"source": "trigger_param", "key": "custom_items", "optional": true},
						"process_params":        gin.H{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":            gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
						"limit_check_enabled":   gin.H{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
						"end_policy":            models.DetectionEndPolicyFixedDuration,
						"duration_sec":          gin.H{"source": "trigger_param", "key": "duration_sec"},
						"operator_note":         gin.H{"source": "trigger_param", "key": "operator_note", "optional": true},
						"report_template_id":    gin.H{"source": "trigger_param", "key": "report_template_id", "optional": true},
						"report_request":        gin.H{"source": "trigger_param", "key": "report_request", "optional": true},
						"enable_storage":        gin.H{"source": "trigger_param", "key": "enable_storage", "default": true},
						"enable_alarm":          gin.H{"source": "trigger_param", "key": "enable_alarm", "default": true},
						"auto_stop_on_duration": true,
					},
				},
			},
			"condition_script": `task_params.command === "start_fixed_duration_detection"`,
		},
		{
			"template_code": "variable-request-start-qualified-hold-detection",
			"name":          "变量请求合格持续检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后启动检测，并启动合格持续守护；全部检测项持续合格达到 qualified_hold_ms 后自动结束。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{
				{
					"code":   "control",
					"module": models.TaskFlowActionBuiltinWriteControlVariables,
					"params": gin.H{
						"items": gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
					},
				},
				{
					"code":   "start",
					"module": models.TaskFlowActionBuiltinStartDetectionRun,
					"params": gin.H{
						"project_id":          gin.H{"source": "trigger_param", "key": "project_id"},
						"test_no":             gin.H{"source": "trigger_param", "key": "test_no", "optional": true},
						"standard_id":         gin.H{"source": "trigger_param", "key": "standard_id", "optional": true},
						"custom_items":        gin.H{"source": "trigger_param", "key": "custom_items", "optional": true},
						"process_params":      gin.H{"source": "trigger_param", "key": "process_params", "optional": true},
						"plc_writes":          gin.H{"source": "trigger_param", "key": "plc_writes", "optional": true},
						"limit_check_enabled": gin.H{"source": "trigger_param", "key": "limit_check_enabled", "default": true},
						"end_policy":          models.DetectionEndPolicyQualifiedHold,
						"qualified_hold_ms":   gin.H{"source": "trigger_param", "key": "qualified_hold_ms"},
						"operator_note":       gin.H{"source": "trigger_param", "key": "operator_note", "optional": true},
						"report_template_id":  gin.H{"source": "trigger_param", "key": "report_template_id", "optional": true},
						"report_request":      gin.H{"source": "trigger_param", "key": "report_request", "optional": true},
						"enable_storage":      gin.H{"source": "trigger_param", "key": "enable_storage", "default": true},
						"enable_alarm":        gin.H{"source": "trigger_param", "key": "enable_alarm", "default": true},
					},
				},
				{
					"code":   "qualified",
					"module": models.TaskFlowActionBuiltinQualifiedHoldGuard,
					"params": gin.H{
						"task_id":           gin.H{"source": "context", "key": "task_id"},
						"qualified_hold_ms": gin.H{"source": "trigger_param", "key": "qualified_hold_ms"},
						"check_interval_ms": gin.H{"source": "trigger_param", "key": "check_interval_ms", "optional": true},
					},
				},
			},
			"condition_script": `task_params.command === "start_qualified_hold_detection"`,
		},
		{
			"template_code": "variable-request-pause-detection",
			"name":          "变量请求暂停检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后按 task_id 或 project_id 暂停当前检测，暂停期间不计入累计检测时长。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "pause",
				"module": models.TaskFlowActionBuiltinPauseDetectionRun,
				"params": gin.H{
					"task_id":    gin.H{"source": "trigger_param", "key": "task_id", "optional": true},
					"project_id": gin.H{"source": "trigger_param", "key": "project_id", "optional": true},
					"reason":     gin.H{"source": "trigger_param", "key": "reason", "optional": true},
				},
			}},
			"condition_script": `task_params.command === "pause_detection"`,
		},
		{
			"template_code": "variable-request-resume-detection",
			"name":          "变量请求恢复检测",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后恢复已暂停检测，并顺延固定时长任务的预计结束时间。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "resume",
				"module": models.TaskFlowActionBuiltinResumeDetectionRun,
				"params": gin.H{"task_id": gin.H{"source": "trigger_param", "key": "task_id"}},
			}},
			"condition_script": `task_params.command === "resume_detection"`,
		},
		{
			"template_code": "variable-request-update-detection-limits",
			"name":          "变量请求调整运行限值",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后调整 running 任务的某个变量检测快照，不修改全局变量默认属性或原始检测标准。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "limits",
				"module": models.TaskFlowActionBuiltinUpdateDetectionLimits,
				"params": gin.H{
					"task_id":           gin.H{"source": "trigger_param", "key": "task_id", "optional": true},
					"project_id":        gin.H{"source": "trigger_param", "key": "project_id", "optional": true},
					"items":             gin.H{"source": "trigger_param", "key": "items", "optional": true},
					"var_id":            gin.H{"source": "trigger_param", "key": "var_id", "optional": true},
					"check_enabled":     gin.H{"source": "trigger_param", "key": "check_enabled", "optional": true},
					"alarm_enabled":     gin.H{"source": "trigger_param", "key": "alarm_enabled", "optional": true},
					"store_enabled":     gin.H{"source": "trigger_param", "key": "store_enabled", "optional": true},
					"limit_ll":          gin.H{"source": "trigger_param", "key": "limit_ll", "optional": true},
					"limit_l":           gin.H{"source": "trigger_param", "key": "limit_l", "optional": true},
					"limit_h":           gin.H{"source": "trigger_param", "key": "limit_h", "optional": true},
					"limit_hh":          gin.H{"source": "trigger_param", "key": "limit_hh", "optional": true},
					"limit_deadband":    gin.H{"source": "trigger_param", "key": "limit_deadband", "optional": true},
					"check_cycle_ms":    gin.H{"source": "trigger_param", "key": "check_cycle_ms", "optional": true},
					"violation_hold_ms": gin.H{"source": "trigger_param", "key": "violation_hold_ms", "optional": true},
					"recover_hold_ms":   gin.H{"source": "trigger_param", "key": "recover_hold_ms", "optional": true},
				},
			}},
			"condition_script": `task_params.command === "update_detection_limits"`,
		},
		{
			"template_code": "variable-request-refresh-detection-features",
			"name":          "变量请求计算检测特征值",
			"category":      "detection",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后刷新指定检测任务的平均值、最小值、最大值和样本数。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "features",
				"module": models.TaskFlowActionBuiltinRefreshFeatures,
				"params": gin.H{"task_id": gin.H{"source": "trigger_param", "key": "task_id"}},
			}},
			"condition_script": `task_params.command === "refresh_detection_features"`,
		},
		{
			"template_code": "variable-request-register-report",
			"name":          "变量请求登记检测报表",
			"category":      "report",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后登记某次检测生成的 Excel 报表文件引用；只存 file_ref，不写二进制。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "report",
				"module": models.TaskFlowActionBuiltinRegisterReport,
				"params": gin.H{
					"task_id":          gin.H{"source": "trigger_param", "key": "task_id", "optional": true},
					"project_id":       gin.H{"source": "trigger_param", "key": "project_id", "optional": true},
					"file_ref":         gin.H{"source": "trigger_param", "key": "file_ref"},
					"file_name":        gin.H{"source": "trigger_param", "key": "file_name", "optional": true},
					"status":           gin.H{"source": "trigger_param", "key": "status", "optional": true},
					"template_id":      gin.H{"source": "trigger_param", "key": "template_id", "optional": true},
					"template_code":    gin.H{"source": "trigger_param", "key": "template_code", "optional": true},
					"template_version": gin.H{"source": "trigger_param", "key": "template_version", "optional": true},
				},
			}},
			"condition_script": `task_params.command === "register_report"`,
		},
		{
			"template_code": "write-variable-command",
			"name":          "写变量并触发任务",
			"category":      "realtime",
			"trigger_type":  models.TaskFlowTriggerManual,
			"description":   "后端受控写入虚拟变量，并携带 origin/depth 防止默认自递归；物理变量下设请走 WS/HTTP 写命令。",
			"steps": []gin.H{{
				"code":   "write",
				"module": models.TaskFlowActionBuiltinWriteVariable,
				"params": gin.H{
					"var_id":          gin.H{"source": "trigger_param", "key": "var_id"},
					"value":           gin.H{"source": "trigger_param", "key": "value"},
					"trigger":         true,
					"max_depth":       1,
					"allow_reentrant": false,
				},
			}},
		},
		{
			"template_code": "write-control-variables",
			"name":          "下设 PLC 控制参数",
			"category":      "realtime",
			"trigger_type":  models.TaskFlowTriggerDataChange,
			"description":   "STRING 虚拟变量写入 JSON 后，把 plc_writes 中的工艺参数通过后端写变量 service 下设到 PLC。",
			"watch_vars":    []gin.H{{"role": models.TaskFlowVarRoleWatch, "description": "任务请求 STRING 虚拟变量"}},
			"steps": []gin.H{{
				"code":   "control",
				"module": models.TaskFlowActionBuiltinWriteControlVariables,
				"params": gin.H{"items": gin.H{"source": "trigger_param", "key": "plc_writes"}},
			}},
			"condition_script": `task_params.command === "write_control_variables"`,
		},
	})
}

func (h *TaskFlowsHandler) runtime(c *gin.Context) {
	if h.flows == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task flow executor is not available"})
		return
	}
	c.JSON(http.StatusOK, h.flows.RuntimeStats(0.8))
}

func (h *TaskFlowsHandler) list(c *gin.Context) {
	filter := database.TaskFlowFilter{}
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	if raw := c.Query("trigger_type"); raw != "" {
		if err := validateTaskFlowTriggerType(raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		filter.TriggerType = normalizeTaskFlowEnum(raw)
	}
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid enabled"})
			return
		}
		filter.Enabled = &value
	}
	flows, err := h.repo.ListTaskFlows(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, flows)
}

func (h *TaskFlowsHandler) create(c *gin.Context) {
	var req taskFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTaskFlowCreateRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	flow := taskFlowFromRequest(req)
	if err := h.repo.CreateTaskFlow(&flow); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	h.reload()
	c.JSON(http.StatusOK, flow)
}

func (h *TaskFlowsHandler) get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	flow, err := h.repo.GetTaskFlow(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, flow)
}

func (h *TaskFlowsHandler) patch(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req taskFlowPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTaskFlowPatchRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, err := h.repo.GetTaskFlow(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	if err := validateTaskFlowPatchEffectiveState(existing, req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	flow, err := h.repo.UpdateTaskFlow(id, taskFlowUpdates(req), varsFromPatch(req), req.Vars != nil)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	h.reload()
	c.JSON(http.StatusOK, flow)
}

func (h *TaskFlowsHandler) delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteTaskFlow(id); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	h.reload()
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *TaskFlowsHandler) run(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	flow, err := h.repo.GetTaskFlow(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	ok := h.flows.Submit(flow, pipeline.TaskFlowEvent{TriggerType: models.TaskFlowTriggerManual, ProjectID: flow.ProjectID})
	if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "task flow queue full"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "queued"})
}

func (h *TaskFlowsHandler) listRuns(c *gin.Context) {
	filter, err := taskFlowRunFilterFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.repo.ListTaskFlowRuns(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": normalizedLimit(filter.Limit, 50, 500), "offset": maxInt(filter.Offset, 0)})
}

func (h *TaskFlowsHandler) getRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	run, err := h.repo.GetTaskFlowRun(id)
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *TaskFlowsHandler) listRunSQLLogs(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	limit, err := intQuery(c, "limit", 100)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if limit <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be positive"})
		return
	}
	if _, err := h.repo.GetTaskFlowRun(id); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	logs, err := h.repo.ListTaskFlowSQLLogs(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

func (h *TaskFlowsHandler) reload() {
	flows, err := h.repo.LoadEnabledTaskFlows()
	if err != nil {
		return
	}
	h.flows.Load(flows)
}

func taskFlowFromRequest(req taskFlowRequest) models.TaskFlow {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return models.TaskFlow{
		ProjectID:          req.ProjectID,
		FlowCode:           req.FlowCode,
		Name:               req.Name,
		Enabled:            enabled,
		TriggerType:        normalizeTaskFlowEnum(req.TriggerType),
		ConditionScript:    req.ConditionScript,
		ActionType:         normalizeTaskFlowEnum(req.ActionType),
		ActionScript:       req.ActionScript,
		ActionPayload:      req.ActionPayload,
		StepsJSON:          req.StepsJSON,
		TimeoutMS:          req.TimeoutMS,
		CooldownMS:         req.CooldownMS,
		HoldMS:             req.HoldMS,
		ScheduleIntervalMS: req.ScheduleIntervalMS,
		Priority:           req.Priority,
		Remark:             req.Remark,
		Vars:               varsFromRequests(req.ProjectID, req.Vars),
	}
}

func varsFromRequests(projectID uint, reqs []taskFlowVarRequest) []models.TaskFlowVar {
	vars := make([]models.TaskFlowVar, 0, len(reqs))
	for _, req := range reqs {
		vars = append(vars, models.TaskFlowVar{
			ProjectID: projectID,
			VarID:     req.VarID.Int64(),
			VarName:   req.VarName,
			Role:      normalizeTaskFlowEnum(req.Role),
		})
	}
	return vars
}

func varsFromPatch(req taskFlowPatchRequest) []models.TaskFlowVar {
	if req.Vars == nil {
		return nil
	}
	return varsFromRequests(0, *req.Vars)
}

func taskFlowUpdates(req taskFlowPatchRequest) map[string]any {
	updates := make(map[string]any)
	if req.ProjectID != nil {
		updates["project_id"] = *req.ProjectID
	}
	setStringUpdate(updates, "flow_code", req.FlowCode)
	setStringUpdate(updates, "name", req.Name)
	setStringUpdate(updates, "trigger_type", req.TriggerType)
	setStringUpdate(updates, "condition_script", req.ConditionScript)
	setStringUpdate(updates, "action_type", req.ActionType)
	setStringUpdate(updates, "action_script", req.ActionScript)
	setStringUpdate(updates, "action_payload", req.ActionPayload)
	setStringUpdate(updates, "steps_json", req.StepsJSON)
	setStringUpdate(updates, "remark", req.Remark)
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.TriggerType != nil {
		updates["trigger_type"] = normalizeTaskFlowEnum(*req.TriggerType)
	}
	if req.ActionType != nil {
		updates["action_type"] = normalizeTaskFlowEnum(*req.ActionType)
	}
	if req.TimeoutMS != nil {
		updates["timeout_ms"] = *req.TimeoutMS
	}
	if req.CooldownMS != nil {
		updates["cooldown_ms"] = *req.CooldownMS
	}
	if req.HoldMS != nil {
		updates["hold_ms"] = *req.HoldMS
	}
	if req.ScheduleIntervalMS != nil {
		updates["schedule_interval_ms"] = *req.ScheduleIntervalMS
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	return updates
}

func validateTaskFlowCreateRequest(req taskFlowRequest) error {
	if req.ProjectID == 0 {
		return fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(req.FlowCode) == "" {
		return fmt.Errorf("flow_code is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := validateOptionalTaskFlowTriggerType(req.TriggerType); err != nil {
		return err
	}
	if err := validateOptionalTaskFlowActionType(req.ActionType); err != nil {
		return err
	}
	if err := validateTaskFlowVars(req.Vars); err != nil {
		return err
	}
	if err := validateTaskFlowCreateTiming(req); err != nil {
		return err
	}
	if err := validateTaskFlowWatchVars(req.TriggerType, req.Vars); err != nil {
		return err
	}
	if err := validateTaskFlowScheduleInterval(req.TriggerType, req.ScheduleIntervalMS, req.CooldownMS); err != nil {
		return err
	}
	return validateTaskFlowStepsJSON(req.StepsJSON)
}

func validateTaskFlowPatchRequest(req taskFlowPatchRequest) error {
	if req.ProjectID != nil && *req.ProjectID == 0 {
		return fmt.Errorf("project_id must be greater than 0")
	}
	if req.FlowCode != nil && strings.TrimSpace(*req.FlowCode) == "" {
		return fmt.Errorf("flow_code cannot be empty")
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if req.TriggerType != nil {
		if strings.TrimSpace(*req.TriggerType) == "" {
			return fmt.Errorf("invalid task flow trigger_type: empty")
		}
		if err := validateTaskFlowTriggerType(*req.TriggerType); err != nil {
			return err
		}
	}
	if req.ActionType != nil {
		if strings.TrimSpace(*req.ActionType) == "" {
			return fmt.Errorf("invalid task flow action_type: empty")
		}
		if err := validateTaskFlowActionType(*req.ActionType); err != nil {
			return err
		}
	}
	if req.Vars != nil {
		if err := validateTaskFlowVars(*req.Vars); err != nil {
			return err
		}
	}
	if err := validateTaskFlowPatchTiming(req); err != nil {
		return err
	}
	if req.StepsJSON != nil {
		return validateTaskFlowStepsJSON(*req.StepsJSON)
	}
	return nil
}

func validateTaskFlowCreateTiming(req taskFlowRequest) error {
	if req.TimeoutMS < 0 {
		return fmt.Errorf("timeout_ms cannot be negative")
	}
	if req.CooldownMS < 0 {
		return fmt.Errorf("cooldown_ms cannot be negative")
	}
	if req.HoldMS < 0 {
		return fmt.Errorf("hold_ms cannot be negative")
	}
	if req.ScheduleIntervalMS < 0 {
		return fmt.Errorf("schedule_interval_ms cannot be negative")
	}
	return nil
}

func validateTaskFlowPatchTiming(req taskFlowPatchRequest) error {
	if req.TimeoutMS != nil && *req.TimeoutMS <= 0 {
		return fmt.Errorf("timeout_ms must be greater than 0")
	}
	if req.CooldownMS != nil && *req.CooldownMS < 0 {
		return fmt.Errorf("cooldown_ms cannot be negative")
	}
	if req.HoldMS != nil && *req.HoldMS < 0 {
		return fmt.Errorf("hold_ms cannot be negative")
	}
	if req.ScheduleIntervalMS != nil && *req.ScheduleIntervalMS < 0 {
		return fmt.Errorf("schedule_interval_ms cannot be negative")
	}
	return nil
}

func validateTaskFlowPatchEffectiveState(existing models.TaskFlow, req taskFlowPatchRequest) error {
	triggerType := existing.TriggerType
	if req.TriggerType != nil {
		triggerType = *req.TriggerType
	}
	scheduleIntervalMS := existing.ScheduleIntervalMS
	if req.ScheduleIntervalMS != nil {
		scheduleIntervalMS = *req.ScheduleIntervalMS
	}
	cooldownMS := existing.CooldownMS
	if req.CooldownMS != nil {
		cooldownMS = *req.CooldownMS
	}
	if err := validateTaskFlowScheduleInterval(triggerType, scheduleIntervalMS, cooldownMS); err != nil {
		return err
	}
	if req.Vars != nil {
		return validateTaskFlowWatchVars(triggerType, *req.Vars)
	}
	if effectiveTaskFlowTriggerType(triggerType) != models.TaskFlowTriggerDataChange {
		return nil
	}
	for _, variable := range existing.Vars {
		role := normalizeTaskFlowEnum(variable.Role)
		if role == "" || role == models.TaskFlowVarRoleWatch {
			return nil
		}
	}
	return fmt.Errorf("data_change task flow requires at least one watch var")
}

func validateTaskFlowWatchVars(triggerType string, vars []taskFlowVarRequest) error {
	if effectiveTaskFlowTriggerType(triggerType) != models.TaskFlowTriggerDataChange {
		return nil
	}
	for _, variable := range vars {
		role := normalizeTaskFlowEnum(variable.Role)
		if role == "" || role == models.TaskFlowVarRoleWatch {
			return nil
		}
	}
	return fmt.Errorf("data_change task flow requires at least one watch var")
}

func validateTaskFlowScheduleInterval(triggerType string, scheduleIntervalMS int, cooldownMS int) error {
	if effectiveTaskFlowTriggerType(triggerType) != models.TaskFlowTriggerSchedule {
		return nil
	}
	if scheduleIntervalMS > 0 || cooldownMS > 0 {
		return nil
	}
	return fmt.Errorf("schedule task flow requires schedule_interval_ms or cooldown_ms greater than 0")
}

func effectiveTaskFlowTriggerType(triggerType string) string {
	normalized := normalizeTaskFlowEnum(triggerType)
	if normalized == "" {
		return models.TaskFlowTriggerDataChange
	}
	return normalized
}

func validateOptionalTaskFlowTriggerType(triggerType string) error {
	if strings.TrimSpace(triggerType) == "" {
		return nil
	}
	return validateTaskFlowTriggerType(triggerType)
}

func validateOptionalTaskFlowActionType(actionType string) error {
	if strings.TrimSpace(actionType) == "" {
		return nil
	}
	return validateTaskFlowActionType(actionType)
}

func validateTaskFlowActionType(actionType string) error {
	switch normalizeTaskFlowEnum(actionType) {
	case models.TaskFlowActionBuiltinStorageSnapshot,
		models.TaskFlowActionBuiltinStoragePrepare,
		models.TaskFlowActionBuiltinStartDetectionRun,
		models.TaskFlowActionBuiltinStopDetectionRun,
		models.TaskFlowActionBuiltinPauseDetectionRun,
		models.TaskFlowActionBuiltinResumeDetectionRun,
		models.TaskFlowActionBuiltinFixedDurationGuard,
		models.TaskFlowActionBuiltinQualifiedHoldGuard,
		models.TaskFlowActionBuiltinRefreshFeatures,
		models.TaskFlowActionBuiltinMuteDetectionAlarms,
		models.TaskFlowActionBuiltinUpdateDetectionLimits,
		models.TaskFlowActionBuiltinRegisterReport,
		models.TaskFlowActionBuiltinHTTPRequest,
		models.TaskFlowActionBuiltinContextSet,
		models.TaskFlowActionBuiltinWriteVariable,
		models.TaskFlowActionBuiltinWriteControlVariables,
		models.TaskFlowActionJavaScript:
		return nil
	default:
		return fmt.Errorf("invalid task flow action_type: %s", actionType)
	}
}

func validateTaskFlowVars(vars []taskFlowVarRequest) error {
	for _, variable := range vars {
		if variable.VarID.Int64() <= 0 {
			return fmt.Errorf("invalid task flow var_id: %d", variable.VarID.Int64())
		}
		if err := validateTaskFlowVarRole(variable.Role); err != nil {
			return err
		}
	}
	return nil
}

func validateTaskFlowVarRole(role string) error {
	if strings.TrimSpace(role) == "" {
		return nil
	}
	switch normalizeTaskFlowEnum(role) {
	case models.TaskFlowVarRoleWatch, models.TaskFlowVarRoleRead, models.TaskFlowVarRoleWrite:
		return nil
	default:
		return fmt.Errorf("invalid task flow var role: %s", role)
	}
}

func validateTaskFlowStepsJSON(stepsJSON string) error {
	text := strings.TrimSpace(stepsJSON)
	if text == "" {
		return nil
	}
	var steps []taskFlowStepRequest
	if err := json.Unmarshal([]byte(text), &steps); err != nil {
		var single taskFlowStepRequest
		if singleErr := json.Unmarshal([]byte(text), &single); singleErr != nil {
			return fmt.Errorf("invalid steps_json: %w", err)
		}
		steps = []taskFlowStepRequest{single}
	}
	if len(steps) == 0 {
		return fmt.Errorf("steps_json cannot be empty")
	}
	seenCodes := map[string]struct{}{}
	for _, step := range steps {
		code := strings.TrimSpace(step.Code)
		if code != "" {
			if _, exists := seenCodes[code]; exists {
				return fmt.Errorf("duplicate task flow step code: %s", code)
			}
			seenCodes[code] = struct{}{}
		}
		if strings.TrimSpace(step.Module) == "" {
			return fmt.Errorf("invalid task flow step module: empty")
		}
		if err := validateTaskFlowActionType(step.Module); err != nil {
			return fmt.Errorf("invalid task flow step module: %s", step.Module)
		}
		if err := validateTaskFlowStepParams(step.Params); err != nil {
			return err
		}
	}
	return nil
}

func validateTaskFlowStepParams(params map[string]any) error {
	for key, value := range params {
		if err := validateTaskFlowParamBindingValue(value, "params."+key); err != nil {
			return err
		}
	}
	return nil
}

func validateTaskFlowParamBindingValue(value any, path string) error {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			if err := validateTaskFlowParamBindingValue(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		if sourceRaw, hasSource := typed["source"]; hasSource {
			source := normalizeTaskFlowEnum(fmt.Sprint(sourceRaw))
			switch source {
			case "literal", "trigger_param", "context":
				return nil
			case "event":
				key := strings.TrimSpace(fmt.Sprint(typed["key"]))
				if err := validateTaskFlowEventParamKey(key); err != nil {
					return fmt.Errorf("invalid task flow event key at %s: %s", path, key)
				}
				return nil
			default:
				return fmt.Errorf("invalid task flow param source at %s: %s", path, sourceRaw)
			}
		}
		for key, item := range typed {
			if err := validateTaskFlowParamBindingValue(item, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTaskFlowEventParamKey(key string) error {
	switch key {
	case "trigger_type",
		"project_id",
		"trigger_var_id",
		"trigger_var_id_text",
		"trigger_value",
		"gateway_id",
		"topic",
		"origin_flow_id",
		"origin_run_id",
		"depth",
		"request_id",
		"at":
		return nil
	default:
		return fmt.Errorf("unsupported event key")
	}
}

func normalizeTaskFlowEnum(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func taskFlowRunFilterFromQuery(c *gin.Context) (database.TaskFlowRunFilter, error) {
	limit, err := intQuery(c, "limit", 50)
	if err != nil {
		return database.TaskFlowRunFilter{}, err
	}
	if limit <= 0 {
		return database.TaskFlowRunFilter{}, fmt.Errorf("limit must be positive")
	}
	offset, err := intQuery(c, "offset", 0)
	if err != nil {
		return database.TaskFlowRunFilter{}, err
	}
	if offset < 0 {
		return database.TaskFlowRunFilter{}, fmt.Errorf("offset must be non-negative")
	}
	filter := database.TaskFlowRunFilter{
		FlowCode:    c.Query("flow_code"),
		Status:      normalizeTaskFlowEnum(c.Query("status")),
		TriggerType: normalizeTaskFlowEnum(c.Query("trigger_type")),
		Limit:       limit,
		Offset:      offset,
	}
	if err := validateTaskFlowRunStatus(filter.Status); err != nil {
		return database.TaskFlowRunFilter{}, err
	}
	if err := validateTaskFlowTriggerType(filter.TriggerType); err != nil {
		return database.TaskFlowRunFilter{}, err
	}
	if raw := c.Query("project_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		if value == 0 {
			return database.TaskFlowRunFilter{}, fmt.Errorf("project_id must be positive")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	if raw := c.Query("flow_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		if value == 0 {
			return database.TaskFlowRunFilter{}, fmt.Errorf("flow_id must be positive")
		}
		filter.FlowID = &value
	}
	if raw := c.Query("trigger_var_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		if value <= 0 {
			return database.TaskFlowRunFilter{}, fmt.Errorf("trigger_var_id must be positive")
		}
		filter.TriggerVarID = &value
	}
	if raw := c.Query("origin_flow_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		if value == 0 {
			return database.TaskFlowRunFilter{}, fmt.Errorf("origin_flow_id must be positive")
		}
		filter.OriginFlowID = &value
	}
	if raw := c.Query("from"); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		filter.From = &value
	}
	if raw := c.Query("to"); raw != "" {
		value, err := parseQueryTime(raw)
		if err != nil {
			return database.TaskFlowRunFilter{}, err
		}
		filter.To = &value
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return database.TaskFlowRunFilter{}, fmt.Errorf("from must be before or equal to to")
	}
	return filter, nil
}

func validateTaskFlowRunStatus(status string) error {
	if status == "" {
		return nil
	}
	switch status {
	case models.TaskFlowStatusPending,
		models.TaskFlowStatusRunning,
		models.TaskFlowStatusSuccess,
		models.TaskFlowStatusFailed,
		models.TaskFlowStatusTimeout,
		models.TaskFlowStatusSkipped:
		return nil
	default:
		return fmt.Errorf("invalid task flow run status: %s", status)
	}
}

func validateTaskFlowTriggerType(triggerType string) error {
	if strings.TrimSpace(triggerType) == "" {
		return nil
	}
	switch normalizeTaskFlowEnum(triggerType) {
	case models.TaskFlowTriggerManual,
		models.TaskFlowTriggerDataChange,
		models.TaskFlowTriggerSchedule,
		models.TaskFlowTriggerProjectStart,
		models.TaskFlowTriggerProjectEnd:
		return nil
	default:
		return fmt.Errorf("invalid task flow trigger_type: %s", triggerType)
	}
}

func intQuery(c *gin.Context, key string, fallback int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func normalizedLimit(value int, fallback int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
