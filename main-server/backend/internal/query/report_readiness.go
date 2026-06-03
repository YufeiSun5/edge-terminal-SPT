package query

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	ReportReadinessReady        = "ready"
	ReportReadinessWaiting      = "waiting"
	ReportReadinessNotRequested = "not_requested"

	ReadinessCheckOK           = "ok"
	ReadinessCheckWaiting      = "waiting"
	ReadinessCheckNotRequested = "not_requested"
)

type ReportReadiness struct {
	OverallStatus string                      `json:"overall_status"`
	CheckedAt     time.Time                   `json:"checked_at"`
	Task          DetectionTask               `json:"task"`
	Checks        []ReportReadinessCheck      `json:"checks"`
	Counts        ReportReadinessCounts       `json:"counts"`
	Requests      []ReportRequestReadiness    `json:"requests"`
	Warnings      []string                    `json:"warnings,omitempty"`
	Summary       *DetectionRunSummary        `json:"summary,omitempty"`
	ReportItems   []DetectionRunReportRequest `json:"report_requests"`
	Features      []DetectionRunFeature       `json:"features,omitempty"`
}

type ReportReadinessCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Count   int64  `json:"count,omitempty"`
	Message string `json:"message,omitempty"`
}

type ReportReadinessCounts struct {
	ReportRequests int   `json:"report_requests"`
	StorageRoutes  int   `json:"storage_routes"`
	SummaryRows    int   `json:"summary_rows"`
	FeatureRows    int   `json:"feature_rows"`
	HistoryRows    int64 `json:"history_rows"`
	AlarmRows      int64 `json:"alarm_rows"`
}

type ReportRequestReadiness struct {
	RequestID            uint64   `json:"request_id"`
	TemplateID           *uint    `json:"template_id,omitempty"`
	TemplateCode         string   `json:"template_code"`
	TemplateVersion      int      `json:"template_version"`
	ReportName           string   `json:"report_name"`
	Status               string   `json:"status"`
	RequiredVarIDs       []string `json:"required_var_ids"`
	MissingHistoryVarIDs []string `json:"missing_history_var_ids,omitempty"`
	MissingFeatureVarIDs []string `json:"missing_feature_var_ids,omitempty"`
	HistoryRows          int64    `json:"history_rows"`
	AlarmRows            int64    `json:"alarm_rows"`
	Ready                bool     `json:"ready"`
}

func (q *StationViewQuery) ReportReadiness(taskID uint, edgeInstanceID string) (ReportReadiness, error) {
	if taskID == 0 {
		return ReportReadiness{}, gorm.ErrRecordNotFound
	}
	var task DetectionTask
	if err := q.db.First(&task, "id = ?", taskID).Error; err != nil {
		return ReportReadiness{}, err
	}
	if _, err := q.projectForEdge(task.ProjectID, edgeInstanceID); err != nil {
		return ReportReadiness{}, err
	}
	if err := q.attachRunSnapshots(&task, true); err != nil {
		return ReportReadiness{}, err
	}

	readiness := ReportReadiness{
		OverallStatus: ReportReadinessReady,
		CheckedAt:     time.Now(),
		Task:          task,
		ReportItems:   task.ReportRequests,
	}
	readiness.Counts.ReportRequests = len(task.ReportRequests)
	readiness.Counts.StorageRoutes = len(task.StorageRoutes)

	if strings.EqualFold(task.Status, DetectionStatusStopped) {
		readiness.addCheck("task_finished", ReadinessCheckOK, 1, "detection run is stopped")
	} else {
		readiness.addCheck("task_finished", ReadinessCheckWaiting, 0, "detection run is not stopped yet")
	}

	if len(task.ReportRequests) == 0 {
		readiness.addCheck("report_requests", ReadinessCheckNotRequested, 0, "no report requests have been synchronized for this run")
		readiness.OverallStatus = ReportReadinessNotRequested
		return readiness, nil
	}
	readiness.addCheck("report_requests", ReadinessCheckOK, int64(len(task.ReportRequests)), "report request snapshots are synchronized")

	summary, err := q.findDetectionRunSummary(taskID)
	if err != nil {
		return ReportReadiness{}, err
	}
	if summary != nil {
		readiness.Summary = summary
		readiness.Counts.SummaryRows = 1
		readiness.addCheck("summary", ReadinessCheckOK, 1, "detection summary is synchronized")
	} else {
		readiness.addCheck("summary", ReadinessCheckWaiting, 0, "detection summary is not synchronized yet")
	}

	features, err := q.DetectionRunFeatures(taskID)
	if err != nil {
		return ReportReadiness{}, err
	}
	readiness.Features = features
	readiness.Counts.FeatureRows = len(features)
	featureVarIDs := make(map[int64]bool, len(features))
	for _, feature := range features {
		featureVarIDs[feature.VarID] = true
	}

	historyByVar, totalHistoryRows, historyWarnings, err := q.historyCountsByVar(taskID, task.StorageRoutes)
	if err != nil {
		return ReportReadiness{}, err
	}
	readiness.Counts.HistoryRows = totalHistoryRows
	readiness.Warnings = append(readiness.Warnings, historyWarnings...)

	alarmByVar, totalAlarmRows, err := q.alarmCountsByVar(taskID)
	if err != nil {
		return ReportReadiness{}, err
	}
	readiness.Counts.AlarmRows = totalAlarmRows
	readiness.addCheck("alarms", ReadinessCheckOK, totalAlarmRows, "detection alarm rows are queryable; zero rows is a valid synchronized state")

	overallReady := strings.EqualFold(task.Status, DetectionStatusStopped) && summary != nil
	allHistoryReady := true
	allFeaturesReady := true
	for _, request := range task.ReportRequests {
		item := q.reportRequestReadiness(request, historyByVar, featureVarIDs, alarmByVar)
		readiness.Requests = append(readiness.Requests, item)
		if len(item.MissingHistoryVarIDs) > 0 {
			allHistoryReady = false
		}
		if len(item.MissingFeatureVarIDs) > 0 {
			allFeaturesReady = false
		}
	}
	if allHistoryReady {
		readiness.addCheck("history", ReadinessCheckOK, totalHistoryRows, "requested variable history rows are synchronized")
	} else {
		readiness.addCheck("history", ReadinessCheckWaiting, totalHistoryRows, "one or more requested variables have no synchronized history rows")
	}
	if allFeaturesReady {
		readiness.addCheck("features", ReadinessCheckOK, int64(len(features)), "requested variable features are synchronized")
	} else {
		readiness.addCheck("features", ReadinessCheckWaiting, int64(len(features)), "one or more requested variables have no synchronized feature rows")
	}
	if !overallReady || !allHistoryReady || !allFeaturesReady {
		readiness.OverallStatus = ReportReadinessWaiting
	}
	return readiness, nil
}

func (r *ReportReadiness) addCheck(name string, status string, count int64, message string) {
	r.Checks = append(r.Checks, ReportReadinessCheck{Name: name, Status: status, Count: count, Message: message})
	if status == ReadinessCheckWaiting && r.OverallStatus == ReportReadinessReady {
		r.OverallStatus = ReportReadinessWaiting
	}
}

func (q *StationViewQuery) findDetectionRunSummary(taskID uint) (*DetectionRunSummary, error) {
	var summary DetectionRunSummary
	if err := q.db.First(&summary, "task_id = ?", taskID).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, err
		}
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &summary, nil
}

func (q *StationViewQuery) reportRequestReadiness(request DetectionRunReportRequest, historyByVar map[int64]int64, featureVarIDs map[int64]bool, alarmByVar map[int64]int64) ReportRequestReadiness {
	varIDs := extractReportRequestVarIDs(request)
	item := ReportRequestReadiness{
		RequestID:       request.ID,
		TemplateID:      request.TemplateID,
		TemplateCode:    request.TemplateCode,
		TemplateVersion: request.TemplateVersion,
		ReportName:      request.ReportName,
		Status:          request.Status,
		RequiredVarIDs:  make([]string, 0, len(varIDs)),
		Ready:           true,
	}
	for _, varID := range varIDs {
		item.RequiredVarIDs = append(item.RequiredVarIDs, strconv.FormatInt(varID, 10))
		item.HistoryRows += historyByVar[varID]
		item.AlarmRows += alarmByVar[varID]
		if historyByVar[varID] == 0 {
			item.MissingHistoryVarIDs = append(item.MissingHistoryVarIDs, strconv.FormatInt(varID, 10))
			item.Ready = false
		}
		if !featureVarIDs[varID] {
			item.MissingFeatureVarIDs = append(item.MissingFeatureVarIDs, strconv.FormatInt(varID, 10))
			item.Ready = false
		}
	}
	return item
}

func extractReportRequestVarIDs(request DetectionRunReportRequest) []int64 {
	found := map[int64]bool{}
	varIDs := make([]int64, 0)
	add := func(value int64) {
		if value <= 0 || found[value] {
			return
		}
		found[value] = true
		varIDs = append(varIDs, value)
	}
	if strings.TrimSpace(request.VariablesJSON) != "" {
		var decoded any
		if err := json.Unmarshal([]byte(request.VariablesJSON), &decoded); err == nil {
			collectReportVarIDs(decoded, add)
		}
	}
	add(request.VarID)
	return varIDs
}

func collectReportVarIDs(value any, add func(int64)) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectReportVarIDs(item, add)
		}
	case map[string]any:
		for key, item := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if lower == "var_id" || lower == "varid" {
				if parsed := reportVarIDFromAny(item); parsed > 0 {
					add(parsed)
				}
				continue
			}
			if lower == "var_ids" || lower == "varids" {
				collectReportVarIDs(item, add)
				continue
			}
			collectReportVarIDs(item, add)
		}
	}
}

func reportVarIDFromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func (q *StationViewQuery) historyCountsByVar(taskID uint, routes []DetectionRunStorageRoute) (map[int64]int64, int64, []string, error) {
	counts := make(map[int64]int64)
	var total int64
	var eav []struct {
		VarID int64
		Count int64
	}
	if err := q.db.Model(&HistoryData{}).Select("var_id, COUNT(*) as count").Where("task_id = ?", taskID).Group("var_id").Scan(&eav).Error; err != nil {
		return nil, 0, nil, err
	}
	for _, row := range eav {
		counts[row.VarID] += row.Count
		total += row.Count
	}

	warnings := make([]string, 0)
	dialect := q.db.Name()
	for _, route := range routes {
		if route.StorageTarget != StorageTargetWideTable {
			continue
		}
		if err := validateStorageIdentifier(route.StorageTable); err != nil {
			return nil, 0, nil, err
		}
		if err := validateStorageIdentifier(route.ColumnName); err != nil {
			return nil, 0, nil, err
		}
		if !q.db.Migrator().HasTable(route.StorageTable) {
			warnings = append(warnings, fmt.Sprintf("wide history table %s is not synchronized", route.StorageTable))
			continue
		}
		var count int64
		whereColumn := fmt.Sprintf("%s IS NOT NULL", quoteIdentifier(dialect, route.ColumnName))
		if err := q.db.Table(route.StorageTable).Where("task_id = ?", taskID).Where(whereColumn).Count(&count).Error; err != nil {
			return nil, 0, nil, err
		}
		counts[route.VarID] += count
		total += count
	}
	return counts, total, warnings, nil
}

func (q *StationViewQuery) alarmCountsByVar(taskID uint) (map[int64]int64, int64, error) {
	counts := make(map[int64]int64)
	var rows []struct {
		VarID int64
		Count int64
	}
	if err := q.db.Model(&DetectionLimitAlarm{}).
		Select("var_id, COUNT(*) as count").
		Where("task_id = ? AND scope = ?", taskID, AlarmScopeDetection).
		Group("var_id").
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int64
	for _, row := range rows {
		counts[row.VarID] = row.Count
		total += row.Count
	}
	return counts, total, nil
}
