package reports

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/query"

	"github.com/xuri/excelize/v2"
)

const reportPackageVersion = 1

type ReportPackage struct {
	Kind        string             `json:"kind"`
	Version     int                `json:"version"`
	GeneratedAt string             `json:"generated_at"`
	Task        ReportTaskIdentity `json:"task"`
	Reports     []ReportItem       `json:"reports"`
	Counts      any                `json:"counts"`
	Checks      any                `json:"checks"`
	Warnings    []string           `json:"warnings,omitempty"`
	Summary     any                `json:"summary,omitempty"`
}

type ReportTaskIdentity struct {
	EdgeInstanceID string     `json:"edge_instance_id"`
	TaskID         uint       `json:"task_id"`
	TestNo         string     `json:"test_no"`
	FactoryNo      string     `json:"factory_no,omitempty"`
	CustomerName   string     `json:"customer_name,omitempty"`
	DeviceModel    string     `json:"device_model,omitempty"`
	ProjectID      uint       `json:"project_id"`
	ProjectCode    string     `json:"project_code"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
}

type ReportItem struct {
	RequestID          uint64                       `json:"request_id"`
	ReportName         string                       `json:"report_name"`
	TemplateID         *uint                        `json:"template_id,omitempty"`
	TemplateCode       string                       `json:"template_code"`
	TemplateVersion    int                          `json:"template_version"`
	Status             string                       `json:"status"`
	Readiness          query.ReportRequestReadiness `json:"readiness"`
	Variables          []ReportVariable             `json:"variables"`
	Params             map[string]any               `json:"params,omitempty"`
	Ext1               string                       `json:"ext_1,omitempty"`
	Ext2               string                       `json:"ext_2,omitempty"`
	Ext3               string                       `json:"ext_3,omitempty"`
	CellMappingVersion int                          `json:"cell_mapping_version,omitempty"`
}

type ReportVariable struct {
	VarID         int64               `json:"var_id"`
	VarIDText     string              `json:"var_id_text"`
	VarName       string              `json:"var_name"`
	DisplayName   string              `json:"display_name,omitempty"`
	DisplayNameEN string              `json:"display_name_en,omitempty"`
	DisplayNameJA string              `json:"display_name_ja,omitempty"`
	Unit          string              `json:"unit,omitempty"`
	DecimalPlaces int                 `json:"decimal_places"`
	Metrics       ReportMetrics       `json:"metrics"`
	Limits        ReportLimitSnapshot `json:"limits"`
}

type ReportMetrics struct {
	FullDetection     ReportMetricWindow `json:"full_detection"`
	QualifiedTwoHours ReportMetricWindow `json:"qualified_two_hours"`
}

type ReportMetricWindow struct {
	Status          string     `json:"status"`
	SampleCount     int64      `json:"sample_count"`
	AvgValue        *float64   `json:"avg_value,omitempty"`
	MinValue        *float64   `json:"min_value,omitempty"`
	MaxValue        *float64   `json:"max_value,omitempty"`
	FirstSampleTime *time.Time `json:"first_sample_time,omitempty"`
	LastSampleTime  *time.Time `json:"last_sample_time,omitempty"`
	Message         string     `json:"message,omitempty"`
}

type ReportLimitSnapshot struct {
	Source          string   `json:"source"`
	CheckEnabled    bool     `json:"check_enabled"`
	AlarmEnabled    bool     `json:"alarm_enabled"`
	LimitLL         *float64 `json:"limit_ll,omitempty"`
	LimitL          *float64 `json:"limit_l,omitempty"`
	LimitH          *float64 `json:"limit_h,omitempty"`
	LimitHH         *float64 `json:"limit_hh,omitempty"`
	LimitDeadband   float64  `json:"limit_deadband"`
	RecoverHoldMS   int      `json:"recover_hold_ms"`
	ViolationHoldMS int      `json:"violation_hold_ms"`
	QualityPolicy   string   `json:"quality_policy,omitempty"`
}

type CellMappingSpec struct {
	Version    int               `json:"version"`
	Sheet      string            `json:"sheet"`
	ChartSheet string            `json:"chart_sheet"`
	ChartCell  string            `json:"chart_cell"`
	Items      []CellMappingItem `json:"items"`
}

type CellMappingItem struct {
	Sheet     string `json:"sheet"`
	Cell      string `json:"cell"`
	Source    string `json:"source"`
	VarID     int64  `json:"var_id"`
	VarIDText string `json:"var_id_text"`
	VarName   string `json:"var_name"`
	ParamKey  string `json:"param_key"`
	Required  bool   `json:"required"`
}

func buildReportPackage(job MainReportJob, readiness query.ReportReadiness, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness, qualifiedMetrics map[int64]ReportMetricWindow) (ReportPackage, error) {
	params, err := parseReportParams(request.ParamsJSON)
	if err != nil {
		return ReportPackage{}, err
	}
	requestedIDs := parseRequiredVarIDs(requestReadiness.RequiredVarIDs)
	if request.VarID > 0 {
		requestedIDs = appendMissingVarID(requestedIDs, request.VarID)
	}
	features := map[int64]query.DetectionRunFeature{}
	for _, feature := range readiness.Features {
		features[feature.VarID] = feature
	}
	standards := map[int64]query.DetectionRunStandardItem{}
	for _, item := range readiness.Task.StandardItems {
		standards[item.VarID] = item
	}
	report := ReportItem{
		RequestID:       request.ID,
		ReportName:      firstNonEmpty(request.ReportName, job.ReportName, request.TemplateCode),
		TemplateID:      request.TemplateID,
		TemplateCode:    request.TemplateCode,
		TemplateVersion: request.TemplateVersion,
		Status:          request.Status,
		Readiness:       requestReadiness,
		Variables:       make([]ReportVariable, 0, len(requestedIDs)),
		Params:          params,
		Ext1:            request.Ext1,
		Ext2:            request.Ext2,
		Ext3:            request.Ext3,
	}
	if spec, ok, err := parseCellMapping(params); err != nil {
		return ReportPackage{}, err
	} else if ok {
		report.CellMappingVersion = spec.Version
	}
	for _, varID := range requestedIDs {
		report.Variables = append(report.Variables, buildReportVariable(varID, request, features[varID], standards[varID], qualifiedMetrics[varID]))
	}
	return ReportPackage{
		Kind:        "main_server_report_package",
		Version:     reportPackageVersion,
		GeneratedAt: time.Now().Format(time.RFC3339Nano),
		Task: ReportTaskIdentity{
			EdgeInstanceID: job.EdgeInstanceID,
			TaskID:         readiness.Task.ID,
			TestNo:         readiness.Task.TestNo,
			FactoryNo:      readiness.Task.FactoryNo,
			CustomerName:   readiness.Task.CustomerName,
			DeviceModel:    readiness.Task.DeviceModel,
			ProjectID:      readiness.Task.ProjectID,
			ProjectCode:    readiness.Task.ProjectCode,
			Status:         readiness.Task.Status,
			StartedAt:      readiness.Task.StartedAt,
			EndedAt:        readiness.Task.EndedAt,
		},
		Reports:  []ReportItem{report},
		Counts:   readiness.Counts,
		Checks:   readiness.Checks,
		Warnings: readiness.Warnings,
		Summary:  readiness.Summary,
	}, nil
}

func buildReportVariable(varID int64, request query.DetectionRunReportRequest, feature query.DetectionRunFeature, standard query.DetectionRunStandardItem, qualifiedMetric ReportMetricWindow) ReportVariable {
	variable := ReportVariable{
		VarID:     varID,
		VarIDText: strconv.FormatInt(varID, 10),
		VarName:   firstNonEmpty(feature.VarName, standard.VarName, request.VarName),
		Limits:    buildLimitSnapshot(standard),
	}
	if request.VarID == varID {
		variable.DisplayName = request.DisplayName
		variable.DisplayNameEN = request.DisplayNameEN
		variable.DisplayNameJA = request.DisplayNameJA
	}
	if standard.VarID == varID {
		variable.DisplayName = firstNonEmpty(variable.DisplayName, standard.DisplayName)
		variable.DisplayNameEN = firstNonEmpty(variable.DisplayNameEN, standard.DisplayNameEN)
		variable.DisplayNameJA = firstNonEmpty(variable.DisplayNameJA, standard.DisplayNameJA)
		variable.Unit = standard.Unit
		variable.DecimalPlaces = standard.DecimalPlaces
	}
	if feature.VarID == varID {
		first := feature.FirstSampleTime
		last := feature.LastSampleTime
		variable.Metrics.FullDetection = ReportMetricWindow{
			Status:          "available",
			SampleCount:     feature.SampleCount,
			AvgValue:        feature.AvgValue,
			MinValue:        feature.MinValue,
			MaxValue:        feature.MaxValue,
			FirstSampleTime: &first,
			LastSampleTime:  &last,
		}
		variable.Metrics.QualifiedTwoHours = qualifiedMetric
		if variable.Metrics.QualifiedTwoHours.Status == "" {
			variable.Metrics.QualifiedTwoHours = ReportMetricWindow{Status: "missing_history", Message: "qualified two-hour window was not scanned"}
		}
	} else {
		variable.Metrics.FullDetection = ReportMetricWindow{Status: "missing_feature", Message: "requested variable feature row is not synchronized"}
		variable.Metrics.QualifiedTwoHours = ReportMetricWindow{Status: "missing_feature", Message: "qualified two-hour average requires synchronized feature/history data"}
	}
	return variable
}

func buildLimitSnapshot(item query.DetectionRunStandardItem) ReportLimitSnapshot {
	if item.VarID == 0 {
		return ReportLimitSnapshot{Source: "none"}
	}
	source := "detection_run_standard_items"
	limitLL, limitL, limitH, limitHH := item.LimitLL, item.LimitL, item.LimitH, item.LimitHH
	deadband := item.LimitDeadband
	if limitLL == nil && limitL == nil && limitH == nil && limitHH == nil &&
		(item.VariableDefaultLimitLL != nil || item.VariableDefaultLimitL != nil || item.VariableDefaultLimitH != nil || item.VariableDefaultLimitHH != nil) {
		source = "variable_default_snapshot"
		limitLL, limitL, limitH, limitHH = item.VariableDefaultLimitLL, item.VariableDefaultLimitL, item.VariableDefaultLimitH, item.VariableDefaultLimitHH
		deadband = item.VariableDefaultLimitDeadband
	}
	return ReportLimitSnapshot{
		Source:          source,
		CheckEnabled:    item.CheckEnabled,
		AlarmEnabled:    item.AlarmEnabled,
		LimitLL:         limitLL,
		LimitL:          limitL,
		LimitH:          limitH,
		LimitHH:         limitHH,
		LimitDeadband:   deadband,
		RecoverHoldMS:   item.RecoverHoldMS,
		ViolationHoldMS: item.ViolationHoldMS,
		QualityPolicy:   item.QualityPolicy,
	}
}

func parseReportParams(raw string) (map[string]any, error) {
	params := map[string]any{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return params, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&params); err != nil {
		return nil, fmt.Errorf("invalid report params_json: %w", err)
	}
	return params, nil
}

func parseRequiredVarIDs(values []string) []int64 {
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil && parsed > 0 {
			ids = appendMissingVarID(ids, parsed)
		}
	}
	return ids
}

func appendMissingVarID(ids []int64, id int64) []int64 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func parseCellMapping(params map[string]any) (CellMappingSpec, bool, error) {
	raw, ok := params["cell_mapping"]
	if !ok {
		raw, ok = params["cell_mappings"]
	}
	if !ok {
		raw, ok = params["template_cell_mapping"]
	}
	if !ok {
		return CellMappingSpec{}, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return CellMappingSpec{}, true, err
	}
	var spec CellMappingSpec
	if err := json.Unmarshal(encoded, &spec); err != nil {
		return CellMappingSpec{}, true, err
	}
	if spec.Version == 0 {
		spec.Version = 1
	}
	if len(spec.Items) == 0 {
		return spec, true, fmt.Errorf("cell_mapping.items is required when cell_mapping is provided")
	}
	return spec, true, nil
}

func reportPackageWithTemplateMapping(pkg ReportPackage, templateParamsSchemaJSON string) (ReportPackage, error) {
	if len(pkg.Reports) == 0 {
		return pkg, nil
	}
	if _, ok, err := parseCellMapping(pkg.Reports[0].Params); ok || err != nil {
		return pkg, err
	}
	templateParams := map[string]any{}
	if strings.TrimSpace(templateParamsSchemaJSON) == "" {
		return pkg, nil
	}
	if err := json.Unmarshal([]byte(templateParamsSchemaJSON), &templateParams); err != nil {
		return pkg, fmt.Errorf("parse template params_schema_json: %w", err)
	}
	if _, ok, err := parseCellMapping(templateParams); err != nil || !ok {
		return pkg, err
	}
	next := pkg
	next.Reports = append([]ReportItem(nil), pkg.Reports...)
	params := map[string]any{}
	for key, value := range templateParams {
		params[key] = value
	}
	for key, value := range pkg.Reports[0].Params {
		params[key] = value
	}
	next.Reports[0].Params = params
	return next, nil
}

func applyCellMapping(file *excelize.File, pkg ReportPackage) error {
	if len(pkg.Reports) == 0 {
		return nil
	}
	spec, ok, err := parseCellMapping(pkg.Reports[0].Params)
	if err != nil || !ok {
		return err
	}
	defaultSheet := strings.TrimSpace(spec.Sheet)
	if defaultSheet == "" {
		defaultSheet = "Sheet1"
	}
	if err := ensureSheet(file, defaultSheet); err != nil {
		return err
	}
	for _, item := range spec.Items {
		sheet := firstNonEmpty(item.Sheet, defaultSheet)
		if err := ensureSheet(file, sheet); err != nil {
			return err
		}
		if _, _, err := excelize.CellNameToCoordinates(item.Cell); err != nil {
			return fmt.Errorf("invalid cell_mapping cell %q: %w", item.Cell, err)
		}
		value, found := resolveCellMappingValue(pkg, item)
		if !found && item.Required {
			return fmt.Errorf("required cell_mapping source %q not found for cell %s", item.Source, item.Cell)
		}
		if err := file.SetCellValue(sheet, item.Cell, excelValue(value)); err != nil {
			return err
		}
	}
	return nil
}

func resolveCellMappingValue(pkg ReportPackage, item CellMappingItem) (any, bool) {
	source := strings.ToLower(strings.TrimSpace(item.Source))
	if source == "" {
		return "", false
	}
	report := pkg.Reports[0]
	switch source {
	case "task.edge_instance_id":
		return pkg.Task.EdgeInstanceID, true
	case "task.task_id":
		return pkg.Task.TaskID, true
	case "task.test_no":
		return pkg.Task.TestNo, true
	case "task.factory_no":
		return pkg.Task.FactoryNo, pkg.Task.FactoryNo != ""
	case "task.customer_name":
		return pkg.Task.CustomerName, pkg.Task.CustomerName != ""
	case "task.device_model":
		return pkg.Task.DeviceModel, pkg.Task.DeviceModel != ""
	case "task.project_id":
		return pkg.Task.ProjectID, true
	case "task.project_code":
		return pkg.Task.ProjectCode, true
	case "task.status":
		return pkg.Task.Status, true
	case "task.started_at", "task.start_time":
		return formatReportTime(pkg.Task.StartedAt), pkg.Task.StartedAt != nil
	case "task.ended_at", "task.end_time":
		return formatReportTime(pkg.Task.EndedAt), pkg.Task.EndedAt != nil
	case "request.request_id":
		return report.RequestID, true
	case "request.report_name":
		return report.ReportName, true
	case "request.template_code":
		return report.TemplateCode, true
	case "request.template_version":
		return report.TemplateVersion, true
	case "summary.result_status":
		if summary, ok := pkg.Summary.(*query.DetectionRunSummary); ok && summary != nil {
			return summary.ResultStatus, true
		}
	case "summary.duration_ms":
		if summary, ok := pkg.Summary.(*query.DetectionRunSummary); ok && summary != nil {
			return summary.DurationMS, true
		}
	}
	if strings.HasPrefix(source, "param.") || strings.HasPrefix(source, "params.") {
		key := strings.TrimPrefix(strings.TrimPrefix(source, "param."), "params.")
		if item.ParamKey != "" {
			key = item.ParamKey
		}
		value, ok := lookupPath(report.Params, key)
		return value, ok
	}
	variable, ok := findReportVariable(report.Variables, item)
	if !ok {
		return "", false
	}
	switch source {
	case "variable.var_id":
		return variable.VarIDText, true
	case "variable.var_name":
		return variable.VarName, true
	case "variable.display_name":
		return variable.DisplayName, true
	case "variable.display_name_en":
		return variable.DisplayNameEN, true
	case "variable.display_name_ja":
		return variable.DisplayNameJA, true
	case "variable.unit":
		return variable.Unit, true
	case "metric.avg", "metric.avg_value", "metric.full_detection.avg_value":
		return floatValue(variable.Metrics.FullDetection.AvgValue), variable.Metrics.FullDetection.AvgValue != nil
	case "metric.min", "metric.min_value", "metric.full_detection.min_value":
		return floatValue(variable.Metrics.FullDetection.MinValue), variable.Metrics.FullDetection.MinValue != nil
	case "metric.max", "metric.max_value", "metric.full_detection.max_value":
		return floatValue(variable.Metrics.FullDetection.MaxValue), variable.Metrics.FullDetection.MaxValue != nil
	case "metric.sample_count", "metric.full_detection.sample_count":
		return variable.Metrics.FullDetection.SampleCount, true
	case "metric.qualified_two_hours.avg_value":
		return floatValue(variable.Metrics.QualifiedTwoHours.AvgValue), variable.Metrics.QualifiedTwoHours.AvgValue != nil
	case "metric.qualified_two_hours.status":
		return variable.Metrics.QualifiedTwoHours.Status, true
	case "limit.source":
		return variable.Limits.Source, true
	case "limit.limit_ll":
		return floatValue(variable.Limits.LimitLL), variable.Limits.LimitLL != nil
	case "limit.limit_l":
		return floatValue(variable.Limits.LimitL), variable.Limits.LimitL != nil
	case "limit.limit_h":
		return floatValue(variable.Limits.LimitH), variable.Limits.LimitH != nil
	case "limit.limit_hh":
		return floatValue(variable.Limits.LimitHH), variable.Limits.LimitHH != nil
	case "limit.deadband", "limit.limit_deadband":
		return variable.Limits.LimitDeadband, true
	}
	return "", false
}

func formatReportTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func findReportVariable(variables []ReportVariable, item CellMappingItem) (ReportVariable, bool) {
	if item.VarID <= 0 && item.VarIDText != "" {
		item.VarID, _ = strconv.ParseInt(strings.TrimSpace(item.VarIDText), 10, 64)
	}
	for _, variable := range variables {
		if item.VarID > 0 && variable.VarID == item.VarID {
			return variable, true
		}
		if item.VarName != "" && variable.VarName == item.VarName {
			return variable, true
		}
	}
	if len(variables) == 1 {
		return variables[0], true
	}
	return ReportVariable{}, false
}

func lookupPath(values map[string]any, path string) (any, bool) {
	current := any(values)
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapped[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
