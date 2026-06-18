package reports

import (
	"context"
	"strings"
	"testing"

	"spindle-main-server/backend/internal/query"

	"github.com/xuri/excelize/v2"
)

func TestParseLimitExpressionFormats(t *testing.T) {
	cases := []struct {
		raw         string
		wantL       *float64
		wantH       *float64
		wantUnit    string
		wantMode    string
		wantConfirm bool
	}{
		{raw: "10~20%", wantL: floatPtr(10), wantH: floatPtr(20), wantUnit: "%", wantMode: "range"},
		{raw: "10±5", wantL: floatPtr(5), wantH: floatPtr(15), wantMode: "center_delta"},
		{raw: "10±5%", wantL: floatPtr(5), wantH: floatPtr(15), wantUnit: "%", wantMode: "center_delta"},
		{raw: ">=10", wantL: floatPtr(10), wantMode: "lower_bound", wantConfirm: true},
		{raw: "<=20", wantH: floatPtr(20), wantMode: "upper_bound", wantConfirm: true},
		{raw: "10 +5/-3", wantL: floatPtr(7), wantH: floatPtr(15), wantMode: "asymmetric_delta"},
	}
	for _, tc := range cases {
		got := ParseLimitExpression(tc.raw, "")
		if got.Error != "" || got.Mode != tc.wantMode || got.Unit != tc.wantUnit || got.NeedsConfirmation != tc.wantConfirm {
			t.Fatalf("%s parse mismatch: %+v", tc.raw, got)
		}
		assertFloatPtr(t, got.LimitL, tc.wantL, tc.raw+" lower")
		assertFloatPtr(t, got.LimitH, tc.wantH, tc.raw+" upper")
	}
	bad := ParseLimitExpression("abc", "")
	if bad.Error == "" || !bad.NeedsConfirmation {
		t.Fatalf("invalid limit should need confirmation: %+v", bad)
	}
}

func TestServiceParsesPlanImportDraft(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-PLAN", Name: "Plan Project", ProjectGroup: "AC", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := query.TagConfig{VarID: 7001, GatewayID: 1, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "supply_air_temp", DisplayName: "送风温度", Unit: "C", Enabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	templatePath := seedBasicReportTemplate(t, db, t.TempDir(), "PLAN-TEMPLATE")
	if templatePath == "" {
		t.Fatal("template seed failed")
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
	raw := buildPlanImportWorkbook(t)
	draft, err := service.ParsePlanImport(context.Background(), raw, "plan.xlsx", "edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Artifact.Key == "" || draft.Summary.TotalRows != 3 || draft.Summary.ProjectMatchedRows != 3 || draft.Summary.VariableMatchedRows != 2 || draft.Summary.TemplateMatchedRows != 2 {
		t.Fatalf("unexpected draft summary: %+v artifact=%+v", draft.Summary, draft.Artifact)
	}
	if draft.Rows[0].ProjectMatch == nil || draft.Rows[0].VariableMatch == nil || draft.Rows[0].TemplateMatch == nil {
		t.Fatalf("first row should match project variable template: %+v", draft.Rows[0])
	}
	if draft.Rows[0].ProjectGroup != "AC" || draft.Rows[0].ProjectMatch.ProjectGroup != "AC" || draft.Rows[0].NormalizedInput["project_group"] != "AC" {
		t.Fatalf("project group should be hydrated from matched project: %+v", draft.Rows[0])
	}
	assertFloatPtr(t, draft.Rows[0].Limit.LimitL, floatPtr(10), "row1 lower")
	assertFloatPtr(t, draft.Rows[0].Limit.LimitH, floatPtr(20), "row1 upper")
	if draft.Rows[1].Limit.Mode != "center_delta" || draft.Rows[1].Limit.NeedsConfirmation {
		t.Fatalf("row2 limit should parse as center delta without confirmation: %+v", draft.Rows[1].Limit)
	}
	if !draft.Rows[2].NeedsConfirm || len(draft.Rows[2].Issues) == 0 {
		t.Fatalf("row3 should expose unmatched issues: %+v", draft.Rows[2])
	}
}

func TestServiceConfirmsPlanImportCreatesDetectionStandard(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-CONFIRM", Name: "Confirm Project", ProjectGroup: "AC", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := query.TagConfig{VarID: 7101, GatewayID: 1, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", DisplayName: "温度", Unit: "C", Enabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
	row := PlanImportRow{
		RowNumber:    2,
		TestNo:       "PLAN-CONFIRM-001",
		FactoryNo:    "F-CONFIRM-001",
		SettingRaw:   "12",
		TemplateCode: "PLAN-CONFIRM-TPL",
		ReportName:   "性能报表",
		Params:       map[string]string{"operator": "tester"},
		ProjectMatch: &PlanProjectMatch{
			ProjectID:      project.ID,
			ProjectCode:    project.ProjectCode,
			ProjectGroup:   project.ProjectGroup,
			Name:           project.Name,
			EdgeInstanceID: project.EdgeInstanceID,
			Confidence:     1,
		},
		VariableMatch: planVariableMatch(tag, 1),
		Limit:         ParseLimitExpression("10~20", "C"),
		Unit:          "C",
		TemplateMatch: &PlanTemplateMatch{TemplateCode: "PLAN-CONFIRM-TPL", Version: 1, Confidence: 1},
	}
	result, err := service.ConfirmPlanImport(context.Background(), PlanImportConfirmInput{
		Rows:              []PlanImportRow{row},
		SourceArtifactKey: "plan-imports/2026/source.xlsx",
		EdgeInstanceID:    "edge-a",
	}, query.SyncWriteMeta{UpdatedByUser: "admin", UpdatedByNode: "main-server"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedStandards != 1 || result.CreatedPlans != 1 || result.PlanCreationStatus != "created" {
		t.Fatalf("unexpected confirm result: %+v", result)
	}
	var standard query.DetectionStandard
	if err := db.First(&standard, "id = ?", result.Standards[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if standard.ProjectID == nil || *standard.ProjectID != project.ID || standard.ProjectGroup != project.ProjectGroup || standard.EdgeInstanceID != "edge-a" || standard.ConfigHash == "" {
		t.Fatalf("unexpected standard: %+v", standard)
	}
	var items []query.DetectionStandardItem
	if err := db.Where("standard_id = ?", standard.ID).Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].VarID != tag.VarID || items[0].LimitL == nil || *items[0].LimitL != 10 || items[0].LimitH == nil || *items[0].LimitH != 20 {
		t.Fatalf("unexpected standard items: %+v", items)
	}
	var plan query.DetectionPlan
	if err := db.First(&plan, "id = ?", result.Plans[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if plan.PlanNo != "PLAN-CONFIRM-001" || plan.Status != "pending" || plan.StandardCode != standard.StandardCode || plan.EdgeInstanceID != "edge-a" {
		t.Fatalf("unexpected detection plan: %+v", plan)
	}
	if !strings.Contains(plan.ReportRequestJSON, `"template_code":"PLAN-CONFIRM-TPL"`) || !strings.Contains(plan.ReportRequestJSON, `"var_id":"7101"`) || !strings.Contains(plan.ReportRequestJSON, `"operator":"tester"`) {
		t.Fatalf("report request snapshot not preserved: %s", plan.ReportRequestJSON)
	}
}

func TestServiceConfirmPlanImportUsesVarIDTextWhenNumericVarIDLosesPrecision(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-PRECISION", Name: "Precision Project", ProjectGroup: "AC", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := query.TagConfig{
		VarID:       7943629295557820374,
		GatewayID:   1,
		ProjectID:   &project.ID,
		ProjectCode: project.ProjectCode,
		VarName:     "precision_temp",
		DisplayName: "精度温度",
		Unit:        "C",
		Enabled:     true,
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
	row := PlanImportRow{
		RowNumber:  2,
		TestNo:     "PLAN-PRECISION-001",
		FactoryNo:  "F-PRECISION-001",
		ProjectMatch: &PlanProjectMatch{
			ProjectID:      project.ID,
			ProjectCode:    project.ProjectCode,
			ProjectGroup:   project.ProjectGroup,
			Name:           project.Name,
			EdgeInstanceID: project.EdgeInstanceID,
			Confidence:     1,
		},
		VariableMatch: &PlanVariableMatch{
			VarID:       7943629295557820000,
			VarIDText:   "7943629295557820374",
			VarName:     tag.VarName,
			DisplayName: tag.DisplayName,
			Unit:        tag.Unit,
			Confidence:  1,
		},
		Limit: ParseLimitExpression("10~20", "C"),
		Unit:  "C",
	}
	result, err := service.ConfirmPlanImport(context.Background(), PlanImportConfirmInput{
		Rows:              []PlanImportRow{row},
		SourceArtifactKey: "plan-imports/2026/source.xlsx",
		EdgeInstanceID:    "edge-a",
	}, query.SyncWriteMeta{UpdatedByUser: "admin", UpdatedByNode: "main-server"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedStandards != 1 || len(result.Standards) != 1 {
		t.Fatalf("unexpected confirm result: %+v", result)
	}
	var items []query.DetectionStandardItem
	if err := db.Where("standard_id = ?", result.Standards[0].ID).Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].VarID != tag.VarID {
		t.Fatalf("expected exact var id from var_id_text, got %+v", items)
	}
}

func TestServiceConfirmPlanImportRequiresExplicitConfirmation(t *testing.T) {
	db := newReportTestDB(t)
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
	project := &PlanProjectMatch{ProjectID: 1, ProjectCode: "AC", Confidence: 1}
	variable := &PlanVariableMatch{VarID: 1, VarIDText: "1", VarName: "temp", Confidence: 1}
	_, err := service.ConfirmPlanImport(context.Background(), PlanImportConfirmInput{Rows: []PlanImportRow{{
		RowNumber:     2,
		TestNo:        "PLAN-NEEDS-CONFIRM",
		ProjectMatch:  project,
		VariableMatch: variable,
		Limit:         ParseLimitExpression(">=10", "C"),
	}}}, query.SyncWriteMeta{})
	if err == nil {
		t.Fatal("expected confirm to reject one-sided limit without explicit confirmation")
	}
}

func buildPlanImportWorkbook(t *testing.T) []byte {
	t.Helper()
	workbook := excelize.NewFile()
	defer func() { _ = workbook.Close() }()
	if err := workbook.SetSheetName("Sheet1", "计划导入"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"项目编码", "检测编号", "出厂编号", "变量编码", "变量", "上下限", "单位", "模板编码", "报表名称", "param.operator"},
		{"AC-PLAN", "PLAN-001", "F-001", "7001", "supply_air_temp", "10~20", "C", "PLAN-TEMPLATE", "性能报表", "tester"},
		{"AC-PLAN", "PLAN-002", "F-002", "", "送风温度", "10±5", "C", "PLAN-TEMPLATE", "性能报表2", "tester"},
		{"AC-PLAN", "PLAN-003", "F-003", "", "不存在变量", "abc", "C", "MISSING-TEMPLATE", "错误报表", "tester"},
	}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := workbook.SetCellValue("计划导入", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFloatPtr(t *testing.T, got *float64, want *float64, label string) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil || *got != *want {
		t.Fatalf("%s: got %v want %v", label, got, want)
	}
}
