package reports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/query"

	"github.com/xuri/excelize/v2"
)

type PlanImportDraft struct {
	Artifact       ArtifactMeta      `json:"artifact"`
	SourceFileName string            `json:"source_file_name"`
	SheetName      string            `json:"sheet_name"`
	Rows           []PlanImportRow   `json:"rows"`
	Summary        PlanImportSummary `json:"summary"`
	Issues         []PlanImportIssue `json:"issues,omitempty"`
	ParsedAt       time.Time         `json:"parsed_at"`
}

type PlanImportSummary struct {
	TotalRows           int `json:"total_rows"`
	ReadyRows           int `json:"ready_rows"`
	RowsWithIssues      int `json:"rows_with_issues"`
	ProjectMatchedRows  int `json:"project_matched_rows"`
	VariableMatchedRows int `json:"variable_matched_rows"`
	TemplateMatchedRows int `json:"template_matched_rows"`
	LimitParsedRows     int `json:"limit_parsed_rows"`
	NeedsConfirmation   int `json:"needs_confirmation"`
}

type PlanImportRow struct {
	RowNumber       int                `json:"row_number"`
	ProjectCode     string             `json:"project_code,omitempty"`
	ProjectName     string             `json:"project_name,omitempty"`
	ProjectGroup    string             `json:"project_group,omitempty"`
	ProjectMatch    *PlanProjectMatch  `json:"project_match,omitempty"`
	TestNo          string             `json:"test_no,omitempty"`
	FactoryNo       string             `json:"factory_no,omitempty"`
	CustomerName    string             `json:"customer_name,omitempty"`
	DeviceModel     string             `json:"device_model,omitempty"`
	VariableRaw     string             `json:"variable_raw,omitempty"`
	VarIDText       string             `json:"var_id_text,omitempty"`
	VariableMatch   *PlanVariableMatch `json:"variable_match,omitempty"`
	LimitRaw        string             `json:"limit_raw,omitempty"`
	Limit           PlanLimitParse     `json:"limit"`
	SettingRaw      string             `json:"setting_raw,omitempty"`
	Unit            string             `json:"unit,omitempty"`
	TemplateCode    string             `json:"template_code,omitempty"`
	TemplateMatch   *PlanTemplateMatch `json:"template_match,omitempty"`
	ReportName      string             `json:"report_name,omitempty"`
	Params          map[string]string  `json:"params,omitempty"`
	NeedsConfirm    bool               `json:"needs_confirm"`
	Issues          []PlanImportIssue  `json:"issues,omitempty"`
	NormalizedInput map[string]string  `json:"normalized_input,omitempty"`
}

type PlanProjectMatch struct {
	ProjectID      uint    `json:"project_id"`
	ProjectCode    string  `json:"project_code"`
	ProjectGroup   string  `json:"project_group"`
	Name           string  `json:"name"`
	EdgeInstanceID string  `json:"edge_instance_id"`
	Confidence     float64 `json:"confidence"`
}

type PlanVariableMatch struct {
	VarID       int64   `json:"var_id"`
	VarIDText   string  `json:"var_id_text"`
	VarName     string  `json:"var_name"`
	DisplayName string  `json:"display_name"`
	Unit        string  `json:"unit,omitempty"`
	Confidence  float64 `json:"confidence"`
}

type PlanTemplateMatch struct {
	TemplateID   uint    `json:"template_id"`
	TemplateCode string  `json:"template_code"`
	Version      int     `json:"version"`
	FileRef      string  `json:"file_ref"`
	Confidence   float64 `json:"confidence"`
}

type PlanLimitParse struct {
	Raw               string   `json:"raw"`
	LimitL            *float64 `json:"limit_l,omitempty"`
	LimitH            *float64 `json:"limit_h,omitempty"`
	Unit              string   `json:"unit,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	Normalized        string   `json:"normalized,omitempty"`
	Confidence        float64  `json:"confidence"`
	NeedsConfirmation bool     `json:"needs_confirmation"`
	Error             string   `json:"error,omitempty"`
}

type PlanImportIssue struct {
	RowNumber int    `json:"row_number,omitempty"`
	Field     string `json:"field"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

var ErrPlanImportNotReady = errors.New("plan import is not ready")

type PlanImportConfirmInput struct {
	Rows                   []PlanImportRow `json:"rows"`
	SourceArtifactKey      string          `json:"source_artifact_key,omitempty"`
	EdgeInstanceID         string          `json:"edge_instance_id,omitempty"`
	AllowNeedsConfirmation bool            `json:"allow_needs_confirmation"`
}

type PlanImportConfirmResult struct {
	Standards          []query.DetectionStandard `json:"standards"`
	Plans              []query.DetectionPlan     `json:"plans"`
	Issues             []PlanImportIssue         `json:"issues,omitempty"`
	CreatedStandards   int                       `json:"created_standards"`
	CreatedPlans       int                       `json:"created_plans"`
	PlanCreationStatus string                    `json:"plan_creation_status"`
	PlanCreationNote   string                    `json:"plan_creation_note"`
}

func (s *Service) ParsePlanImport(ctx context.Context, raw []byte, originalName string, edgeInstanceID string) (PlanImportDraft, error) {
	if len(raw) == 0 {
		return PlanImportDraft{}, fmt.Errorf("%w: file is required", ErrInvalidReportTemplate)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return PlanImportDraft{}, fmt.Errorf("%w: xlsx cannot be opened", ErrInvalidReportTemplate)
	}
	defer func() { _ = workbook.Close() }()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return PlanImportDraft{}, fmt.Errorf("%w: workbook has no sheets", ErrInvalidReportTemplate)
	}
	sheet := sheets[0]
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		return PlanImportDraft{}, err
	}
	if len(rows) < 2 {
		return PlanImportDraft{}, fmt.Errorf("%w: plan import requires a header row and at least one data row", ErrInvalidReportTemplate)
	}
	sha := sha256Hex(raw)
	key := fmt.Sprintf("plan-imports/%s/%s/%s", time.Now().Format("2006"), sha, firstNonEmpty(safeArtifactName(originalName), "source.xlsx"))
	meta, err := s.store.Put(ctx, key, raw, reportXLSXContentType)
	if err != nil {
		return PlanImportDraft{}, err
	}
	header := planHeaderIndex(rows[0])
	draft := PlanImportDraft{
		Artifact:       meta,
		SourceFileName: originalName,
		SheetName:      sheet,
		Rows:           make([]PlanImportRow, 0, len(rows)-1),
		ParsedAt:       time.Now(),
	}
	for index, rawRow := range rows[1:] {
		if planRowBlank(rawRow) {
			continue
		}
		row := s.parsePlanImportRow(rawRow, header, index+2, edgeInstanceID)
		draft.Rows = append(draft.Rows, row)
		draft.Issues = append(draft.Issues, row.Issues...)
	}
	draft.Summary = summarizePlanImportRows(draft.Rows)
	return draft, nil
}

func (s *Service) ConfirmPlanImport(_ context.Context, input PlanImportConfirmInput, meta query.SyncWriteMeta) (PlanImportConfirmResult, error) {
	result := PlanImportConfirmResult{
		PlanCreationStatus: "pending",
		PlanCreationNote:   "pending detection plans are created in sys_detection_plans",
	}
	if strings.TrimSpace(input.EdgeInstanceID) != "" {
		meta.EdgeInstanceID = strings.TrimSpace(input.EdgeInstanceID)
	}
	if meta.SyncScope == "" && strings.TrimSpace(meta.EdgeInstanceID) != "" {
		meta.SyncScope = "edge"
	}
	if len(input.Rows) == 0 {
		return result, fmt.Errorf("%w: rows are required", ErrPlanImportNotReady)
	}
	groups := map[string][]PlanImportRow{}
	for _, row := range input.Rows {
		issues := confirmPlanImportRowIssues(row, input.AllowNeedsConfirmation)
		result.Issues = append(result.Issues, issues...)
		if len(issues) > 0 {
			continue
		}
		key := confirmPlanImportGroupKey(row)
		groups[key] = append(groups[key], row)
	}
	if len(result.Issues) > 0 {
		return result, fmt.Errorf("%w: %d row issues", ErrPlanImportNotReady, len(result.Issues))
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	now := time.Now()
	for groupIndex, key := range keys {
		rows := groups[key]
		standard, items, err := buildImportedDetectionStandard(rows, input.SourceArtifactKey, now, groupIndex)
		if err != nil {
			result.Issues = append(result.Issues, PlanImportIssue{Field: "standard", Code: "standard_build_failed", Message: err.Error()})
			continue
		}
		created, err := s.query.CreateDetectionStandard(&standard, items, meta)
		if err != nil {
			return result, err
		}
		result.Standards = append(result.Standards, created)
		planInput, err := buildImportedDetectionPlan(rows, created, input.SourceArtifactKey, groupIndex)
		if err != nil {
			result.Issues = append(result.Issues, PlanImportIssue{Field: "plan", Code: "plan_build_failed", Message: err.Error()})
			continue
		}
		plan, err := s.query.CreateDetectionPlan(planInput, meta)
		if err != nil {
			return result, err
		}
		result.Plans = append(result.Plans, plan)
	}
	result.CreatedStandards = len(result.Standards)
	result.CreatedPlans = len(result.Plans)
	if len(result.Issues) > 0 {
		return result, fmt.Errorf("%w: %d standard issues", ErrPlanImportNotReady, len(result.Issues))
	}
	result.PlanCreationStatus = "created"
	return result, nil
}

func (s *Service) parsePlanImportRow(rawRow []string, header map[string]int, rowNumber int, edgeInstanceID string) PlanImportRow {
	value := func(key string) string {
		index, ok := header[key]
		if !ok || index >= len(rawRow) {
			return ""
		}
		return strings.TrimSpace(rawRow[index])
	}
	row := PlanImportRow{
		RowNumber:    rowNumber,
		ProjectCode:  value("project_code"),
		ProjectName:  value("project_name"),
		ProjectGroup: value("project_group"),
		TestNo:       value("test_no"),
		FactoryNo:    value("factory_no"),
		CustomerName: value("customer_name"),
		DeviceModel:  value("device_model"),
		VariableRaw:  firstNonEmpty(value("variable"), value("var_name"), value("display_name")),
		VarIDText:    value("var_id"),
		LimitRaw:     firstNonEmpty(value("limit"), limitRangeText(value("limit_l"), value("limit_h"))),
		SettingRaw:   value("setting"),
		Unit:         value("unit"),
		TemplateCode: value("template_code"),
		ReportName:   value("report_name"),
		Params:       map[string]string{},
		NormalizedInput: map[string]string{
			"project_code":  value("project_code"),
			"project_name":  value("project_name"),
			"project_group": value("project_group"),
			"variable":      firstNonEmpty(value("variable"), value("var_name"), value("display_name"), value("var_id")),
			"limit":         firstNonEmpty(value("limit"), limitRangeText(value("limit_l"), value("limit_h"))),
			"template_code": value("template_code"),
		},
	}
	for key, index := range header {
		if !strings.HasPrefix(key, "param.") || index >= len(rawRow) {
			continue
		}
		if cell := strings.TrimSpace(rawRow[index]); cell != "" {
			row.Params[strings.TrimPrefix(key, "param.")] = cell
		}
	}
	if len(row.Params) == 0 {
		row.Params = nil
	}
	row.ProjectMatch = s.matchPlanProject(row.ProjectCode, row.ProjectName, edgeInstanceID)
	if row.ProjectMatch == nil {
		row.Issues = append(row.Issues, PlanImportIssue{RowNumber: rowNumber, Field: "project", Code: "project_unmatched", Message: "project could not be matched"})
	} else {
		if row.ProjectGroup == "" {
			row.ProjectGroup = row.ProjectMatch.ProjectGroup
			row.NormalizedInput["project_group"] = row.ProjectGroup
		} else if row.ProjectMatch.ProjectGroup != "" && row.ProjectGroup != row.ProjectMatch.ProjectGroup {
			row.Issues = append(row.Issues, PlanImportIssue{RowNumber: rowNumber, Field: "project_group", Code: "project_group_mismatch", Message: "project_group does not match matched project"})
		}
	}
	row.VariableMatch = s.matchPlanVariable(row.VarIDText, row.VariableRaw, row.ProjectMatch, edgeInstanceID)
	if row.VariableMatch == nil {
		row.Issues = append(row.Issues, PlanImportIssue{RowNumber: rowNumber, Field: "variable", Code: "variable_unmatched", Message: "variable could not be matched"})
	}
	if row.Unit == "" && row.VariableMatch != nil {
		row.Unit = row.VariableMatch.Unit
	}
	row.Limit = ParseLimitExpression(row.LimitRaw, row.Unit)
	if row.Limit.Error != "" {
		row.Issues = append(row.Issues, PlanImportIssue{RowNumber: rowNumber, Field: "limit", Code: "limit_parse_failed", Message: row.Limit.Error})
	}
	row.TemplateMatch = s.matchPlanTemplate(row.TemplateCode)
	if strings.TrimSpace(row.TemplateCode) != "" && row.TemplateMatch == nil {
		row.Issues = append(row.Issues, PlanImportIssue{RowNumber: rowNumber, Field: "template_code", Code: "template_unmatched", Message: "report template could not be matched"})
	}
	row.NeedsConfirm = len(row.Issues) > 0 || row.Limit.NeedsConfirmation
	return row
}

func (s *Service) matchPlanProject(code string, name string, edgeInstanceID string) *PlanProjectMatch {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	var project query.Project
	stmt := s.db.Model(&query.Project{})
	if strings.TrimSpace(edgeInstanceID) != "" {
		stmt = stmt.Where("edge_instance_id = ? OR edge_instance_id = '' OR edge_instance_id IS NULL", strings.TrimSpace(edgeInstanceID))
	}
	if code != "" {
		if err := stmt.Where("project_code = ?", code).First(&project).Error; err == nil {
			return &PlanProjectMatch{ProjectID: project.ID, ProjectCode: project.ProjectCode, ProjectGroup: project.ProjectGroup, Name: project.Name, EdgeInstanceID: project.EdgeInstanceID, Confidence: 1}
		}
	}
	if name != "" {
		like := "%" + name + "%"
		if err := stmt.Where("name LIKE ? OR display_name LIKE ?", like, like).First(&project).Error; err == nil {
			return &PlanProjectMatch{ProjectID: project.ID, ProjectCode: project.ProjectCode, ProjectGroup: project.ProjectGroup, Name: project.Name, EdgeInstanceID: project.EdgeInstanceID, Confidence: 0.75}
		}
	}
	return nil
}

func (s *Service) matchPlanVariable(varIDText string, variableRaw string, project *PlanProjectMatch, edgeInstanceID string) *PlanVariableMatch {
	var filter query.VariableFilter
	if project != nil {
		filter.ProjectID = &project.ProjectID
	}
	if varID, err := strconv.ParseInt(strings.TrimSpace(varIDText), 10, 64); err == nil && varID != 0 {
		tags, err := s.query.ListVariables(filter, edgeInstanceID)
		if err == nil {
			for _, tag := range tags {
				if tag.VarID == varID {
					return planVariableMatch(tag, 1)
				}
			}
		}
	}
	filter.Keyword = strings.TrimSpace(variableRaw)
	if filter.Keyword == "" {
		return nil
	}
	tags, err := s.query.ListVariables(filter, edgeInstanceID)
	if err != nil || len(tags) != 1 {
		return nil
	}
	return planVariableMatch(tags[0], 0.8)
}

func planVariableMatch(tag query.TagConfig, confidence float64) *PlanVariableMatch {
	return &PlanVariableMatch{
		VarID:       tag.VarID,
		VarIDText:   strconv.FormatInt(tag.VarID, 10),
		VarName:     tag.VarName,
		DisplayName: firstNonEmpty(tag.DisplayName, tag.RawName, tag.VarName),
		Unit:        tag.Unit,
		Confidence:  confidence,
	}
}

func (s *Service) matchPlanTemplate(code string) *PlanTemplateMatch {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	var template query.ReportTemplate
	if err := s.db.First(&template, "template_code = ? AND enabled = ?", code, true).Error; err == nil {
		return &PlanTemplateMatch{TemplateID: template.ID, TemplateCode: template.TemplateCode, Version: template.Version, FileRef: template.FileRef, Confidence: 1}
	}
	return nil
}

func summarizePlanImportRows(rows []PlanImportRow) PlanImportSummary {
	var summary PlanImportSummary
	summary.TotalRows = len(rows)
	for _, row := range rows {
		if len(row.Issues) == 0 && !row.Limit.NeedsConfirmation {
			summary.ReadyRows++
		}
		if len(row.Issues) > 0 {
			summary.RowsWithIssues++
		}
		if row.ProjectMatch != nil {
			summary.ProjectMatchedRows++
		}
		if row.VariableMatch != nil {
			summary.VariableMatchedRows++
		}
		if row.TemplateMatch != nil {
			summary.TemplateMatchedRows++
		}
		if row.Limit.Error == "" && (row.Limit.LimitL != nil || row.Limit.LimitH != nil) {
			summary.LimitParsedRows++
		}
		if row.NeedsConfirm {
			summary.NeedsConfirmation++
		}
	}
	return summary
}

func confirmPlanImportRowIssues(row PlanImportRow, allowNeedsConfirmation bool) []PlanImportIssue {
	issues := make([]PlanImportIssue, 0, len(row.Issues)+4)
	issues = append(issues, row.Issues...)
	if row.ProjectMatch == nil {
		issues = append(issues, PlanImportIssue{RowNumber: row.RowNumber, Field: "project", Code: "project_required", Message: "project match is required"})
	}
	if row.VariableMatch == nil {
		issues = append(issues, PlanImportIssue{RowNumber: row.RowNumber, Field: "variable", Code: "variable_required", Message: "variable match is required"})
	}
	if row.Limit.Error != "" || (row.Limit.LimitL == nil && row.Limit.LimitH == nil) {
		issues = append(issues, PlanImportIssue{RowNumber: row.RowNumber, Field: "limit", Code: "limit_required", Message: "valid limit is required"})
	}
	if row.Limit.NeedsConfirmation && !allowNeedsConfirmation {
		issues = append(issues, PlanImportIssue{RowNumber: row.RowNumber, Field: "limit", Code: "explicit_confirmation_required", Message: "limit needs explicit confirmation"})
	}
	return issues
}

func confirmPlanImportGroupKey(row PlanImportRow) string {
	projectID := uint(0)
	if row.ProjectMatch != nil {
		projectID = row.ProjectMatch.ProjectID
	}
	runKey := firstNonEmpty(row.TestNo, row.FactoryNo, row.ReportName, strconv.Itoa(row.RowNumber))
	return fmt.Sprintf("%d/%s", projectID, strings.TrimSpace(runKey))
}

func buildImportedDetectionStandard(rows []PlanImportRow, sourceArtifactKey string, now time.Time, groupIndex int) (query.DetectionStandard, []query.DetectionStandardItem, error) {
	if len(rows) == 0 || rows[0].ProjectMatch == nil {
		return query.DetectionStandard{}, nil, errors.New("project is required")
	}
	project := rows[0].ProjectMatch
	standardCode := importedStandardCode(project.ProjectCode, firstNonEmpty(rows[0].TestNo, rows[0].FactoryNo, rows[0].ReportName), now, groupIndex)
	name := firstNonEmpty(rows[0].TestNo, rows[0].FactoryNo, rows[0].ReportName, standardCode)
	standard := query.DetectionStandard{
		StandardCode: standardCode,
		Name:         "Imported plan " + name,
		DisplayName:  "导入计划 " + name,
		ProjectID:    &project.ProjectID,
		ProjectCode:  project.ProjectCode,
		ProjectGroup: project.ProjectGroup,
		Mode:         "standard",
		Enabled:      true,
		Remark:       importedPlanRemark(rows, sourceArtifactKey),
	}
	if rows[0].TemplateMatch != nil {
		standard.ReportTemplateID = &rows[0].TemplateMatch.TemplateID
	}
	seenVars := map[string]int{}
	items := make([]query.DetectionStandardItem, 0, len(rows))
	for index, row := range rows {
		if row.VariableMatch == nil {
			return query.DetectionStandard{}, nil, fmt.Errorf("row %d variable is required", row.RowNumber)
		}
		varIDText := strings.TrimSpace(row.VariableMatch.VarIDText)
		if varIDText == "" {
			varIDText = strconv.FormatInt(row.VariableMatch.VarID, 10)
		}
		varID, err := strconv.ParseInt(varIDText, 10, 64)
		if err != nil {
			return query.DetectionStandard{}, nil, fmt.Errorf("row %d variable id %q is invalid", row.RowNumber, varIDText)
		}
		if first := seenVars[varIDText]; first > 0 {
			return query.DetectionStandard{}, nil, fmt.Errorf("duplicate variable %s in rows %d and %d", row.VariableMatch.VarIDText, first, row.RowNumber)
		}
		seenVars[varIDText] = row.RowNumber
		items = append(items, query.DetectionStandardItem{
			VarID:           varID,
			VarName:         row.VariableMatch.VarName,
			DisplayName:     row.VariableMatch.DisplayName,
			CheckEnabled:    true,
			AlarmEnabled:    true,
			StoreEnabled:    true,
			CheckCycleMS:    3000,
			CheckOnStart:    true,
			Required:        true,
			CheckMethod:     "numeric_range",
			TargetValue:     strings.TrimSpace(row.SettingRaw),
			LimitL:          row.Limit.LimitL,
			LimitH:          row.Limit.LimitH,
			ViolationHoldMS: 3000,
			RecoverHoldMS:   3000,
			QualityPolicy:   "ignore_bad",
			Unit:            firstNonEmpty(row.Unit, row.Limit.Unit, row.VariableMatch.Unit),
			DecimalPlaces:   2,
			SortOrder:       index + 1,
		})
	}
	return standard, items, nil
}

func buildImportedDetectionPlan(rows []PlanImportRow, standard query.DetectionStandard, sourceArtifactKey string, groupIndex int) (query.DetectionPlanCreate, error) {
	if len(rows) == 0 || rows[0].ProjectMatch == nil {
		return query.DetectionPlanCreate{}, errors.New("project is required")
	}
	reportRequestJSON, err := importedReportRequestJSON(rows, sourceArtifactKey)
	if err != nil {
		return query.DetectionPlanCreate{}, err
	}
	project := rows[0].ProjectMatch
	runKey := firstNonEmpty(rows[0].TestNo, rows[0].FactoryNo, rows[0].ReportName, standard.StandardCode)
	planNo := importedPlanNo(runKey, standard.StandardCode)
	return query.DetectionPlanCreate{
		PlanNo:            planNo,
		SourceSystem:      "report-plan-import",
		ExternalPlanID:    importedPlanExternalID(standard.StandardCode),
		ExternalOrderNo:   strings.TrimSpace(sourceArtifactKey),
		FactoryNo:         firstNonEmpty(rows[0].FactoryNo, planNo),
		CustomerName:      strings.TrimSpace(rows[0].CustomerName),
		DeviceModel:       strings.TrimSpace(rows[0].DeviceModel),
		TestItemCode:      firstNonEmpty(rows[0].TestNo, rows[0].TemplateCode, standard.StandardCode),
		TestItemName:      firstNonEmpty(rows[0].ReportName, standard.DisplayName, standard.Name),
		TestSequence:      groupIndex + 1,
		Mode:              firstNonEmpty(standard.Mode, "standard"),
		StandardCode:      standard.StandardCode,
		ReportRequestJSON: reportRequestJSON,
		SyncScope:         "edge",
		EdgeInstanceID:    project.EdgeInstanceID,
	}, nil
}

func importedReportRequestJSON(rows []PlanImportRow, sourceArtifactKey string) (string, error) {
	type reportVariable struct {
		VarID       string `json:"var_id"`
		VarName     string `json:"var_name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}
	type reportSpec struct {
		TemplateCode string            `json:"template_code,omitempty"`
		ReportName   string            `json:"report_name,omitempty"`
		Variables    []reportVariable  `json:"variables"`
		Params       map[string]string `json:"params,omitempty"`
	}
	type requestSpec struct {
		Enabled bool         `json:"enabled"`
		Reports []reportSpec `json:"reports"`
	}
	reportsByKey := map[string]*reportSpec{}
	keys := make([]string, 0)
	for _, row := range rows {
		if row.VariableMatch == nil {
			continue
		}
		templateCode := strings.TrimSpace(row.TemplateCode)
		if row.TemplateMatch != nil {
			templateCode = row.TemplateMatch.TemplateCode
		}
		reportName := strings.TrimSpace(row.ReportName)
		key := templateCode + "\x00" + reportName
		report := reportsByKey[key]
		if report == nil {
			report = &reportSpec{
				TemplateCode: templateCode,
				ReportName:   reportName,
				Variables:    []reportVariable{},
				Params:       map[string]string{},
			}
			if strings.TrimSpace(sourceArtifactKey) != "" {
				report.Params["source_artifact_key"] = strings.TrimSpace(sourceArtifactKey)
			}
			reportsByKey[key] = report
			keys = append(keys, key)
		}
		report.Variables = append(report.Variables, reportVariable{
			VarID:       row.VariableMatch.VarIDText,
			VarName:     row.VariableMatch.VarName,
			DisplayName: row.VariableMatch.DisplayName,
		})
		for paramKey, paramValue := range row.Params {
			if strings.TrimSpace(paramKey) != "" && strings.TrimSpace(paramValue) != "" {
				report.Params[strings.TrimSpace(paramKey)] = strings.TrimSpace(paramValue)
			}
		}
		if strings.TrimSpace(row.SettingRaw) != "" {
			report.Params["setting_"+row.VariableMatch.VarIDText] = strings.TrimSpace(row.SettingRaw)
		}
	}
	sort.Strings(keys)
	spec := requestSpec{Enabled: true, Reports: make([]reportSpec, 0, len(keys))}
	for _, key := range keys {
		report := reportsByKey[key]
		if report == nil || len(report.Variables) == 0 || (report.TemplateCode == "" && report.ReportName == "") {
			continue
		}
		if len(report.Params) == 0 {
			report.Params = nil
		}
		spec.Reports = append(spec.Reports, *report)
	}
	if len(spec.Reports) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func importedPlanNo(runKey string, standardCode string) string {
	base := strings.TrimSpace(runKey)
	if base == "" {
		base = standardCode
	}
	base = regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "PLAN"
	}
	if len(base) > 128 {
		base = strings.Trim(base[:128], "-")
	}
	return base
}

func importedPlanExternalID(standardCode string) string {
	value := "standard:" + strings.TrimSpace(standardCode)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func importedStandardCode(projectCode string, runKey string, now time.Time, groupIndex int) string {
	base := strings.ToUpper(projectCode + "-" + runKey)
	base = regexp.MustCompile(`[^A-Z0-9]+`).ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "PLAN"
	}
	suffix := now.Format("060102150405") + strconv.Itoa(groupIndex+1)
	maxBase := 64 - len("IMP--") - len(suffix)
	if maxBase < 8 {
		maxBase = 8
	}
	if len(base) > maxBase {
		base = strings.Trim(base[:maxBase], "-")
	}
	return "IMP-" + base + "-" + suffix
}

func importedPlanRemark(rows []PlanImportRow, sourceArtifactKey string) string {
	payload := map[string]any{
		"source":       "report_plan_import",
		"source_key":   strings.TrimSpace(sourceArtifactKey),
		"row_numbers":  planRowNumbers(rows),
		"confirmed_at": time.Now().Format(time.RFC3339Nano),
	}
	raw, _ := json.Marshal(payload)
	if len(raw) > 255 {
		return string(raw[:255])
	}
	return string(raw)
}

func planRowNumbers(rows []PlanImportRow) []int {
	numbers := make([]int, 0, len(rows))
	for _, row := range rows {
		numbers = append(numbers, row.RowNumber)
	}
	return numbers
}

func planHeaderIndex(row []string) map[string]int {
	header := map[string]int{}
	for index, cell := range row {
		key := normalizePlanHeader(cell)
		if key != "" {
			header[key] = index
		}
	}
	return header
}

func normalizePlanHeader(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	aliases := map[string]string{
		"project": "project_code", "项目": "project_code", "项目编码": "project_code", "project_code": "project_code",
		"project_group": "project_group", "项目组": "project_group", "项目类别": "project_group", "适用项目组": "project_group",
		"project_name": "project_name", "项目名称": "project_name",
		"test_no": "test_no", "检测编号": "test_no", "测试编号": "test_no",
		"factory_no": "factory_no", "出厂编号": "factory_no",
		"customer_name": "customer_name", "客户": "customer_name", "客户名称": "customer_name",
		"device_model": "device_model", "机型": "device_model", "设备型号": "device_model",
		"var_id": "var_id", "变量id": "var_id", "变量_id": "var_id",
		"var_name": "var_name", "变量名": "var_name", "变量编码": "var_name",
		"variable": "variable", "变量": "variable", "参数": "variable",
		"display_name": "display_name", "显示名": "display_name", "参数名称": "display_name",
		"limit": "limit", "上下限": "limit", "范围": "limit", "判定范围": "limit",
		"limit_l": "limit_l", "下限": "limit_l",
		"limit_h": "limit_h", "上限": "limit_h",
		"unit": "unit", "单位": "unit",
		"setting": "setting", "设定值": "setting",
		"template_code": "template_code", "模板": "template_code", "模板编码": "template_code",
		"report_name": "report_name", "报表名称": "report_name",
	}
	if strings.HasPrefix(value, "param.") {
		return value
	}
	if mapped, ok := aliases[value]; ok {
		return mapped
	}
	return value
}

func planRowBlank(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func limitRangeText(lower string, upper string) string {
	lower = strings.TrimSpace(lower)
	upper = strings.TrimSpace(upper)
	if lower != "" && upper != "" {
		return lower + "~" + upper
	}
	if lower != "" {
		return ">=" + lower
	}
	if upper != "" {
		return "<=" + upper
	}
	return ""
}

var (
	limitRangePattern       = regexp.MustCompile(`^\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*[~～\-]\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*$`)
	limitCenterDeltaPattern = regexp.MustCompile(`^\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*(?:±|\+/-)\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*$`)
	limitUpperPattern       = regexp.MustCompile(`^\s*(?:<=|≤|<)\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*$`)
	limitLowerPattern       = regexp.MustCompile(`^\s*(?:>=|≥|>)\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*$`)
	limitAsymmetricPattern  = regexp.MustCompile(`^\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*\+\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*/\s*-\s*([+-]?\d+(?:\.\d+)?)\s*(%?)\s*$`)
)

func ParseLimitExpression(raw string, defaultUnit string) PlanLimitParse {
	result := PlanLimitParse{Raw: strings.TrimSpace(raw), Unit: strings.TrimSpace(defaultUnit)}
	if result.Raw == "" {
		result.Error = "limit is empty"
		result.NeedsConfirmation = true
		return result
	}
	text := strings.ReplaceAll(result.Raw, " ", "")
	if match := limitRangePattern.FindStringSubmatch(text); match != nil {
		low := mustParseFloat(match[1])
		high := mustParseFloat(match[3])
		result.LimitL, result.LimitH = &low, &high
		result.Unit = firstNonEmpty(percentUnit(match[2], match[4]), result.Unit)
		result.Mode = "range"
		result.Confidence = 1
		result.Normalized = fmt.Sprintf("%s~%s%s", trimFloat(low), trimFloat(high), result.Unit)
		return result
	}
	if match := limitCenterDeltaPattern.FindStringSubmatch(text); match != nil {
		center := mustParseFloat(match[1])
		delta := mustParseFloat(match[3])
		low, high := center-delta, center+delta
		result.LimitL, result.LimitH = &low, &high
		result.Unit = firstNonEmpty(percentUnit(match[2], match[4]), result.Unit)
		result.Mode = "center_delta"
		result.Confidence = 0.95
		result.Normalized = fmt.Sprintf("%s~%s%s", trimFloat(low), trimFloat(high), result.Unit)
		return result
	}
	if match := limitAsymmetricPattern.FindStringSubmatch(text); match != nil {
		center := mustParseFloat(match[1])
		plus := mustParseFloat(match[3])
		minus := mustParseFloat(match[5])
		low, high := center-minus, center+plus
		result.LimitL, result.LimitH = &low, &high
		result.Unit = firstNonEmpty(percentUnit(match[2], match[4], match[6]), result.Unit)
		result.Mode = "asymmetric_delta"
		result.Confidence = 0.9
		result.Normalized = fmt.Sprintf("%s~%s%s", trimFloat(low), trimFloat(high), result.Unit)
		return result
	}
	if match := limitLowerPattern.FindStringSubmatch(text); match != nil {
		low := mustParseFloat(match[1])
		result.LimitL = &low
		result.Unit = firstNonEmpty(percentUnit(match[2]), result.Unit)
		result.Mode = "lower_bound"
		result.Confidence = 0.9
		result.NeedsConfirmation = true
		result.Normalized = fmt.Sprintf(">=%s%s", trimFloat(low), result.Unit)
		return result
	}
	if match := limitUpperPattern.FindStringSubmatch(text); match != nil {
		high := mustParseFloat(match[1])
		result.LimitH = &high
		result.Unit = firstNonEmpty(percentUnit(match[2]), result.Unit)
		result.Mode = "upper_bound"
		result.Confidence = 0.9
		result.NeedsConfirmation = true
		result.Normalized = fmt.Sprintf("<=%s%s", trimFloat(high), result.Unit)
		return result
	}
	result.Error = "unsupported limit expression"
	result.NeedsConfirmation = true
	return result
}

func percentUnit(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) == "%" {
			return "%"
		}
	}
	return ""
}

func mustParseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
