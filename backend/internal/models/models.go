package models

const (
	StoreTriggerAlways      = "always"
	StoreTriggerOnDetection = "on_detection"
	StoreTriggerOnStart     = "on_start"
	StoreTriggerOnCycle     = "on_cycle"
	StoreTriggerOnChange    = "on_change"

	StorageTargetNone          = "none"
	StorageTargetHistoryEAV    = "history_eav"
	StorageTargetDetectionForm = "detection_form"
	StorageTargetReportField   = "report_field"
	StorageTargetWideTable     = "wide_table"

	TagSourceMQTT    = "mqtt"
	TagSourceVirtual = "virtual"
	TagSourceManual  = "manual"

	RWModeRead      = "R"
	RWModeWrite     = "W"
	RWModeReadWrite = "RW"

	DetectionStatusRunning = "running"
	DetectionStatusPaused  = "paused"
	DetectionStatusStopped = "stopped"

	DetectionEndManualStop    = "manual_stop"
	DetectionEndAbnormalStop  = "abnormal_stop"
	DetectionEndFixedDuration = "fixed_duration"
	DetectionEndQualifiedHold = "qualified_hold"
	DetectionEndTaskFlowStop  = "task_flow_stop"

	DetectionEndPolicyManual        = "manual"
	DetectionEndPolicyFixedDuration = "fixed_duration"
	DetectionEndPolicyQualifiedHold = "qualified_hold"

	DetectionEventRunStarted      = "run_started"
	DetectionEventRunStopped      = "run_stopped"
	DetectionEventRunAbnormalStop = "run_abnormal_stop"
	DetectionEventRunPaused       = "run_paused"
	DetectionEventRunResumed      = "run_resumed"
	DetectionEventLimitsUpdated   = "limits_updated"
	DetectionEventFeaturesUpdated = "features_updated"
	DetectionSummaryStatusRunning = "running"
	DetectionSummaryStatusOK      = "ok"
	DetectionSummaryStatusNG      = "ng"
	DetectionSummaryStatusUnknown = "unknown"

	CheckMethodNumericRange = "numeric_range"
	CheckMethodBoolEquals   = "bool_equals"
	CheckMethodStringEquals = "string_equals"
	CheckMethodRegex        = "regex"

	QualityPolicyIgnoreBad     = "ignore_bad"
	QualityPolicyRecordInvalid = "record_invalid"
	QualityPolicyFailOnBad     = "fail_on_bad"

	DetectionAlarmActionEnter       = "enter"
	DetectionAlarmActionRecover     = "recover"
	DetectionAlarmActionLevelChange = "level_change"
	DetectionAlarmStatusActive      = "active"
	DetectionAlarmStatusClosed      = "recovered"
	AlarmScopeDetection             = "detection"
	AlarmScopeDefault               = "default"

	NotificationLevelInfo    = "info"
	NotificationLevelSuccess = "success"
	NotificationLevelWarning = "warning"
	NotificationLevelError   = "error"

	NotificationAlarmLimitEnter       = "alarm.limit.enter"
	NotificationAlarmLimitRecover     = "alarm.limit.recover"
	NotificationAlarmLimitLevelChange = "alarm.limit.level_change"
	NotificationDetectionRunStarted   = "detection.run_started"
	NotificationDetectionRunStopped   = "detection.run_stopped"
	NotificationDetectionAbnormalStop = "detection.run_abnormal_stop"
	NotificationDetectionRunPaused    = "detection.run_paused"
	NotificationDetectionRunResumed   = "detection.run_resumed"
	NotificationDetectionResultOK     = "detection.result_ok"
	NotificationDetectionResultNG     = "detection.result_ng"
	NotificationDetectionFeatures     = "detection.features_updated"

	NotificationTargetAll     = "all"
	NotificationTargetUser    = "user"
	NotificationTargetRole    = "role"
	NotificationTargetProject = "project"
)
