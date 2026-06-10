package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"spindle-main-server/backend/internal/query"

	"github.com/glebarez/sqlite"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func TestServiceEnqueueProcessAndRetryBoundaries(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-RPT-SVC", Name: "Report Service Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	task := query.DetectionTask{TestNo: "RUN-RPT-SVC", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &ended, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	paramsJSON := `{
		"inlet_area_m2": 1.25,
		"cell_mapping": {
			"version": 1,
			"sheet": "Customer",
			"items": [
				{"cell":"B2","source":"task.test_no","required":true},
				{"cell":"B3","source":"param.inlet_area_m2","required":true},
				{"cell":"B4","source":"metric.avg","var_id":8101,"required":true},
				{"cell":"B5","source":"limit.limit_h","var_id":8101,"required":true},
				{"cell":"B6","source":"metric.avg","var_id":8102,"required":true},
				{"cell":"B7","source":"metric.qualified_two_hours.avg_value","var_id":8102,"required":true},
				{"cell":"B8","source":"metric.qualified_two_hours.status","var_id":8102,"required":true}
			]
		}
	}`
	request := query.DetectionRunReportRequest{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF", TemplateVersion: 1, VarID: 8101, VarName: "temp", VariablesJSON: `[{"var_id":"8101"},{"var_id":"8102"}]`, ParamsJSON: paramsJSON, Status: "pending"}
	if err := db.Create(&request).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, ResultStatus: "ok", HistoryRows: 1, StartedAt: &ended, EndedAt: &ended, LastRefreshedAt: ended}).Error; err != nil {
		t.Fatal(err)
	}
	avg := 12.3
	if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 8101, VarName: "temp", SampleCount: 1, AvgValue: &avg, FirstSampleTime: ended, LastSampleTime: ended}).Error; err != nil {
		t.Fatal(err)
	}
	avg2 := 45.6
	if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 8102, VarName: "pressure", SampleCount: 2, AvgValue: &avg2, FirstSampleTime: ended.Add(-2 * time.Hour), LastSampleTime: ended}).Error; err != nil {
		t.Fatal(err)
	}
	limitH := 20.0
	if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 8101, VarName: "temp", DisplayName: "温度", Unit: "C", CheckEnabled: true, AlarmEnabled: true, LimitH: &limitH}).Error; err != nil {
		t.Fatal(err)
	}
	limitL2 := 30.0
	limitH2 := 70.0
	if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 8102, VarName: "pressure", DisplayName: "压力", Unit: "Pa", CheckEnabled: true, AlarmEnabled: true, LimitL: &limitL2, LimitH: &limitH2}).Error; err != nil {
		t.Fatal(err)
	}
	value := 12.3
	if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 8101, VarName: "temp", ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: ended}).Error; err != nil {
		t.Fatal(err)
	}
	qualifiedRows := []struct {
		at    time.Time
		value float64
	}{
		{ended.Add(-2 * time.Hour), 40},
		{ended.Add(-1 * time.Hour), 50},
		{ended, 60},
	}
	for _, row := range qualifiedRows {
		value := row.value
		if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 8102, VarName: "pressure", ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: row.at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir(), WaitingDelay: time.Millisecond, RetryDelay: time.Millisecond})
	result, err := service.EnqueueTask(task.ID, "edge-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Status != StatusPending {
		t.Fatalf("unexpected enqueue result: %+v", result)
	}
	again, err := service.EnqueueTask(task.ID, "edge-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Jobs) != 1 || again.Jobs[0].ID != result.Jobs[0].ID {
		t.Fatalf("enqueue should be idempotent: first=%+v second=%+v", result.Jobs, again.Jobs)
	}
	processed, err := service.RunDueOnce(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed) != 1 || processed[0].Status != StatusSuccess || processed[0].ArtifactRef == "" {
		t.Fatalf("unexpected processed result: %+v", processed)
	}
	if _, err := os.Stat(processed[0].ArtifactRef); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	path, name, contentType, err := service.Artifact(processed[0].ID)
	if err != nil {
		t.Fatalf("artifact should be downloadable: %v", err)
	}
	if path != processed[0].ArtifactRef || name != processed[0].ArtifactName || contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("unexpected artifact metadata path=%s name=%s content_type=%s job=%+v", path, name, contentType, processed[0])
	}
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("xlsx artifact should be readable: %v", err)
	}
	defer func() { _ = workbook.Close() }()
	if workbook.GetSheetName(0) == "" {
		t.Fatalf("xlsx artifact has no sheets")
	}
	assertCellValue(t, workbook, "Customer", "B2", task.TestNo)
	assertCellValue(t, workbook, "Customer", "B3", "1.25")
	assertCellValue(t, workbook, "Customer", "B4", "12.3")
	assertCellValue(t, workbook, "Customer", "B5", "20")
	assertCellValue(t, workbook, "Customer", "B6", "45.6")
	assertCellValue(t, workbook, "Customer", "B7", "50")
	assertCellValue(t, workbook, "Customer", "B8", "available")
	packageJSON, err := workbook.GetCellValue("Report_Package", "A2")
	if err != nil {
		t.Fatal(err)
	}
	var packagePayload ReportPackage
	if err := json.Unmarshal([]byte(packageJSON), &packagePayload); err != nil {
		t.Fatalf("report package sheet should contain json package: %v", err)
	}
	if packagePayload.Kind != "main_server_report_package" || len(packagePayload.Reports) != 1 || len(packagePayload.Reports[0].Variables) != 2 {
		t.Fatalf("unexpected report package payload: %+v", packagePayload)
	}
	qualifiedMetric := packagePayload.Reports[0].Variables[1].Metrics.QualifiedTwoHours
	if qualifiedMetric.Status != "available" || qualifiedMetric.AvgValue == nil || *qualifiedMetric.AvgValue != 50 {
		t.Fatalf("expected available qualified two-hour average, got %+v", qualifiedMetric)
	}
	manifestPath := filepath.Join(filepath.Dir(processed[0].ArtifactRef), fmt.Sprintf("task-%d-request-%d-manifest.json", processed[0].TaskID, processed[0].RequestID))
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest should exist: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("manifest should be json: %v", err)
	}
	if _, ok := manifest["report_package"]; !ok {
		t.Fatalf("manifest should include report_package: %s", string(rawManifest))
	}
	events, _, err := service.ListEvents(processed[0].ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !reportEventsContain(events, EventEnqueued) || !reportEventsContain(events, EventStarted) || !reportEventsContain(events, EventSucceeded) {
		t.Fatalf("expected enqueue/start/success events, got %+v", events)
	}
	notifications, total, _, _, err := service.ListNotifications(1, NotificationFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(notifications) != 2 || !reportNotificationsContain(notifications, "报表开始生成") || !reportNotificationsContain(notifications, "报表生成完成") {
		t.Fatalf("expected start/success report notifications, total=%d items=%+v", total, notifications)
	}
	unread, err := service.UnreadNotificationCount(1, NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if unread != 2 {
		t.Fatalf("expected 2 unread report notifications, got %d", unread)
	}
	if err := service.MarkNotificationRead(1, notifications[0].ID); err != nil {
		t.Fatal(err)
	}
	unread, err = service.UnreadNotificationCount(1, NotificationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if unread != 1 {
		t.Fatalf("expected 1 unread report notification after read, got %d", unread)
	}
	if _, err := service.RetryJob(processed[0].ID); err != ErrJobNotRetryable {
		t.Fatalf("success job should not be retryable, err=%v", err)
	}
}

func TestServiceProcessesSameTaskMultiReportRequestsIndependently(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-RPT-EB069", Name: "EB069 Multi Report Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	started := ended.Add(-3 * time.Hour)
	task := query.DetectionTask{TestNo: "RUN-EB069-MULTI", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &started, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	requestAParams := `{
		"customer": {"area": 1.25},
		"cell_mapping": {
			"version": 1,
			"sheet": "CustomerA",
			"items": [
				{"cell":"B2","source":"task.test_no","required":true},
				{"cell":"B3","source":"request.report_name","required":true},
				{"cell":"B4","source":"metric.avg","var_id":9101,"required":true},
				{"cell":"B5","source":"limit.limit_h","var_id":9101,"required":true},
				{"cell":"B6","source":"metric.qualified_two_hours.avg_value","var_id":9102,"required":true},
				{"cell":"B7","source":"variable.unit","var_id":9102,"required":true},
				{"cell":"B8","source":"param.customer.area","required":true},
				{"cell":"B9","source":"limit.limit_l","var_id":9102,"required":true}
			]
		}
	}`
	requestBParams := `{
		"customer": {"area": 2.5},
		"cell_mapping": {
			"version": 1,
			"sheet": "CustomerB",
			"items": [
				{"cell":"C2","source":"task.test_no","required":true},
				{"cell":"C3","source":"request.report_name","required":true},
				{"cell":"C4","source":"metric.avg","var_id":9103,"required":true},
				{"cell":"C5","source":"metric.qualified_two_hours.status","var_id":9103,"required":true},
				{"cell":"C6","source":"limit.limit_l","var_id":9103,"required":true},
				{"cell":"C7","source":"param.customer.area","required":true},
				{"cell":"C8","source":"variable.var_name","var_id":9103,"required":true}
			]
		}
	}`
	requests := []query.DetectionRunReportRequest{
		{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF-A", TemplateVersion: 1, VarID: 9101, VarName: "temp", ReportName: "性能报表A", VariablesJSON: `[{"var_id":"9101"},{"var_id":"9102"}]`, ParamsJSON: requestAParams, Status: "pending"},
		{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF-B", TemplateVersion: 1, VarID: 9103, VarName: "humidity", ReportName: "性能报表B", VariablesJSON: `[{"var_id":"9103"}]`, ParamsJSON: requestBParams, Status: "pending"},
	}
	for i := range requests {
		if err := db.Create(&requests[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
		{VarID: 9101, VarName: "temp", DisplayName: "温度", Unit: "C", LimitL: floatPtr(0), LimitH: floatPtr(30), Values: []float64{10, 12, 14}},
		{VarID: 9102, VarName: "pressure", DisplayName: "压力", Unit: "Pa", LimitL: floatPtr(30), LimitH: floatPtr(70), Values: []float64{40, 50, 60}},
		{VarID: 9103, VarName: "humidity", DisplayName: "湿度", Unit: "%RH", LimitL: floatPtr(40), LimitH: floatPtr(80), Values: []float64{55, 65, 75}},
	})
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir(), WaitingDelay: time.Millisecond, RetryDelay: time.Millisecond})
	result, err := service.EnqueueTask(task.ID, "edge-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected two jobs for two report requests, got %+v", result.Jobs)
	}
	for _, job := range result.Jobs {
		if job.Status != StatusPending {
			t.Fatalf("multi-report jobs should be pending when ready, got %+v", result.Jobs)
		}
	}
	processed, err := service.RunDueOnce(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed) != 2 {
		t.Fatalf("expected two processed jobs, got %+v", processed)
	}
	jobsByRequest := map[uint64]MainReportJob{}
	for _, job := range processed {
		if job.Status != StatusSuccess || job.ArtifactRef == "" {
			t.Fatalf("expected successful artifact job, got %+v", job)
		}
		if _, exists := jobsByRequest[job.RequestID]; exists {
			t.Fatalf("duplicate job for request_id=%d", job.RequestID)
		}
		jobsByRequest[job.RequestID] = job
	}
	jobA := jobsByRequest[requests[0].ID]
	jobB := jobsByRequest[requests[1].ID]
	if jobA.ID == 0 || jobB.ID == 0 || jobA.ArtifactRef == jobB.ArtifactRef {
		t.Fatalf("report requests should have independent jobs/artifacts: A=%+v B=%+v", jobA, jobB)
	}
	pkgA := assertReportArtifactPackage(t, jobA, []int64{9101, 9102})
	pkgB := assertReportArtifactPackage(t, jobB, []int64{9103})
	if pkgA.Reports[0].ReportName != "性能报表A" || pkgB.Reports[0].ReportName != "性能报表B" {
		t.Fatalf("report names crossed: A=%s B=%s", pkgA.Reports[0].ReportName, pkgB.Reports[0].ReportName)
	}
	workbookA := openReportWorkbook(t, jobA.ArtifactRef)
	defer func() { _ = workbookA.Close() }()
	assertCellValue(t, workbookA, "CustomerA", "B2", task.TestNo)
	assertCellValue(t, workbookA, "CustomerA", "B3", "性能报表A")
	assertCellValue(t, workbookA, "CustomerA", "B4", "12")
	assertCellValue(t, workbookA, "CustomerA", "B5", "30")
	assertCellValue(t, workbookA, "CustomerA", "B6", "50")
	assertCellValue(t, workbookA, "CustomerA", "B7", "Pa")
	assertCellValue(t, workbookA, "CustomerA", "B8", "1.25")
	assertCellValue(t, workbookA, "CustomerA", "B9", "30")
	workbookB := openReportWorkbook(t, jobB.ArtifactRef)
	defer func() { _ = workbookB.Close() }()
	assertCellValue(t, workbookB, "CustomerB", "C2", task.TestNo)
	assertCellValue(t, workbookB, "CustomerB", "C3", "性能报表B")
	assertCellValue(t, workbookB, "CustomerB", "C4", "65")
	assertCellValue(t, workbookB, "CustomerB", "C5", "available")
	assertCellValue(t, workbookB, "CustomerB", "C6", "40")
	assertCellValue(t, workbookB, "CustomerB", "C7", "2.5")
	assertCellValue(t, workbookB, "CustomerB", "C8", "humidity")
}

func TestServiceFillsCustomerWorkbookCellsAndTracePackage(t *testing.T) {
	db := newReportTestDB(t)
	artifactDir := t.TempDir()
	templatePath := filepath.Join(artifactDir, "customer-template.xlsx")
	writeCustomerTemplateWorkbook(t, templatePath)
	if err := db.Create(&query.ReportTemplate{TemplateCode: "CUSTOMER-CELL-MAP", Name: "Customer Cell Map", DisplayName: "客户模板", FileRef: templatePath, FileKind: "xlsx", Version: 1, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	project := query.Project{ProjectCode: "AC-RPT-CUSTOMER", Name: "Customer Workbook Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	started := ended.Add(-3 * time.Hour)
	task := query.DetectionTask{TestNo: "RUN-EB069-CUSTOMER", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &started, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	paramsJSON := `{
		"judgement": "OK",
		"cell_mapping": {
			"version": 1,
			"sheet": "Customer_Report",
			"items": [
				{"cell":"B2","source":"task.test_no","required":true},
				{"cell":"B3","source":"task.project_code","required":true},
				{"cell":"B4","source":"task.edge_instance_id","required":true},
				{"cell":"B5","source":"task.started_at","required":true},
				{"cell":"B6","source":"task.ended_at","required":true},
				{"cell":"B7","source":"metric.avg","var_id":9401,"required":true},
				{"cell":"B8","source":"metric.qualified_two_hours.avg_value","var_id":9401,"required":true},
				{"cell":"B9","source":"limit.limit_l","var_id":9401,"required":true},
				{"cell":"B10","source":"limit.limit_h","var_id":9401,"required":true},
				{"cell":"B11","source":"metric.qualified_two_hours.status","var_id":9401,"required":true},
				{"cell":"B12","source":"param.judgement","required":true},
				{"cell":"B13","source":"variable.unit","var_id":9401,"required":true}
			]
		}
	}`
	request := query.DetectionRunReportRequest{
		TaskID:          task.ID,
		TestNo:          task.TestNo,
		ProjectID:       project.ID,
		ProjectCode:     project.ProjectCode,
		TemplateCode:    "CUSTOMER-CELL-MAP",
		TemplateVersion: 1,
		VarID:           9401,
		VarName:         "supply_air_temp",
		ReportName:      "客户可见单元格验收",
		VariablesJSON:   `[{"var_id":"9401"}]`,
		ParamsJSON:      paramsJSON,
		Status:          "pending",
	}
	if err := db.Create(&request).Error; err != nil {
		t.Fatal(err)
	}
	seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
		{VarID: 9401, VarName: "supply_air_temp", DisplayName: "送风温度", Unit: "C", LimitL: floatPtr(10), LimitH: floatPtr(30), Values: []float64{20, 22, 24}},
	})
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: artifactDir, WaitingDelay: time.Millisecond, RetryDelay: time.Millisecond})
	if _, err := service.EnqueueTask(task.ID, "edge-a", false); err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunDueOnce(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed) != 1 || processed[0].Status != StatusSuccess {
		t.Fatalf("expected customer workbook report success, got %+v", processed)
	}
	workbook := openReportWorkbook(t, processed[0].ArtifactRef)
	defer func() { _ = workbook.Close() }()
	assertCellValue(t, workbook, "Report_Run", "B14", templatePath)
	assertCellValue(t, workbook, "Customer_Report", "A1", "Customer Acceptance Template")
	assertCellValue(t, workbook, "Customer_Report", "B2", task.TestNo)
	assertCellValue(t, workbook, "Customer_Report", "B3", project.ProjectCode)
	assertCellValue(t, workbook, "Customer_Report", "B4", "edge-a")
	assertCellValue(t, workbook, "Customer_Report", "B5", started.Format(time.RFC3339Nano))
	assertCellValue(t, workbook, "Customer_Report", "B6", ended.Format(time.RFC3339Nano))
	assertCellValue(t, workbook, "Customer_Report", "B7", "22")
	assertCellValue(t, workbook, "Customer_Report", "B8", "22")
	assertCellValue(t, workbook, "Customer_Report", "B9", "10")
	assertCellValue(t, workbook, "Customer_Report", "B10", "30")
	assertCellValue(t, workbook, "Customer_Report", "B11", "available")
	assertCellValue(t, workbook, "Customer_Report", "B12", "OK")
	assertCellValue(t, workbook, "Customer_Report", "B13", "C")

	packagePayload := assertReportArtifactPackage(t, processed[0], []int64{9401})
	manifestPackage := readManifestReportPackage(t, processed[0])
	customerVar := packagePayload.Reports[0].Variables[0]
	manifestVar := manifestPackage.Reports[0].Variables[0]
	if packagePayload.Task.TestNo != task.TestNo || manifestPackage.Task.TestNo != task.TestNo {
		t.Fatalf("task identity should match customer visible cells: package=%+v manifest=%+v", packagePayload.Task, manifestPackage.Task)
	}
	if packagePayload.Task.EdgeInstanceID != "edge-a" || manifestPackage.Task.ProjectCode != project.ProjectCode {
		t.Fatalf("project/edge identity should match customer visible cells: package=%+v manifest=%+v", packagePayload.Task, manifestPackage.Task)
	}
	if customerVar.Metrics.FullDetection.AvgValue == nil || *customerVar.Metrics.FullDetection.AvgValue != 22 {
		t.Fatalf("full detection average should match customer B7: %+v", customerVar.Metrics.FullDetection)
	}
	if customerVar.Metrics.QualifiedTwoHours.AvgValue == nil || *customerVar.Metrics.QualifiedTwoHours.AvgValue != 22 || customerVar.Metrics.QualifiedTwoHours.Status != "available" {
		t.Fatalf("qualified two-hour metric should match customer B8/B11: %+v", customerVar.Metrics.QualifiedTwoHours)
	}
	if manifestVar.Limits.LimitL == nil || *manifestVar.Limits.LimitL != 10 || manifestVar.Limits.LimitH == nil || *manifestVar.Limits.LimitH != 30 {
		t.Fatalf("manifest limits should match customer B9/B10: %+v", manifestVar.Limits)
	}
}

func TestServiceEB069NegativeBoundaries(t *testing.T) {
	t.Run("wrong edge is rejected", func(t *testing.T) {
		db := newReportTestDB(t)
		project, task := seedReportTaskWithRequests(t, db, "AC-RPT-WRONG-EDGE", "edge-a", []query.DetectionRunReportRequest{
			{TemplateCode: "PERF", VarID: 9301, VarName: "temp", VariablesJSON: `[{"var_id":"9301"}]`, Status: "pending"},
		})
		ended := *task.EndedAt
		seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
			{VarID: 9301, VarName: "temp", DisplayName: "温度", Unit: "C", LimitL: floatPtr(0), LimitH: floatPtr(30), Values: []float64{10, 12, 14}},
		})
		service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
		if _, err := service.EnqueueTask(task.ID, "edge-b", false); err == nil {
			t.Fatalf("expected edge mismatch error")
		}
	})

	t.Run("missing history or feature keeps jobs waiting", func(t *testing.T) {
		db := newReportTestDB(t)
		project, task := seedReportTaskWithRequests(t, db, "AC-RPT-MISSING-DATA", "edge-a", []query.DetectionRunReportRequest{
			{TemplateCode: "MISSING-HISTORY", VarID: 9311, VarName: "temp", VariablesJSON: `[{"var_id":"9311"}]`, Status: "pending"},
			{TemplateCode: "MISSING-FEATURE", VarID: 9312, VarName: "pressure", VariablesJSON: `[{"var_id":"9312"}]`, Status: "pending"},
		})
		ended := *task.EndedAt
		if err := db.Create(&query.DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, ResultStatus: "ok", HistoryRows: 1, StartedAt: task.StartedAt, EndedAt: task.EndedAt, LastRefreshedAt: ended}).Error; err != nil {
			t.Fatal(err)
		}
		avg := 11.0
		if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: 9311, VarName: "temp", SampleCount: 1, AvgValue: &avg, FirstSampleTime: ended, LastSampleTime: ended}).Error; err != nil {
			t.Fatal(err)
		}
		limitL := 0.0
		limitH := 30.0
		if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 9311, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, LimitL: &limitL, LimitH: &limitH}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 9312, VarName: "pressure", CheckEnabled: true, AlarmEnabled: true, LimitL: &limitL, LimitH: &limitH}).Error; err != nil {
			t.Fatal(err)
		}
		value := 22.0
		if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: 9312, VarName: "pressure", ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: ended}).Error; err != nil {
			t.Fatal(err)
		}
		service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir(), WaitingDelay: time.Millisecond})
		result, err := service.EnqueueTask(task.ID, "edge-a", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Jobs) != 2 {
			t.Fatalf("expected two waiting jobs, got %+v", result.Jobs)
		}
		for _, req := range result.Requests {
			if req.Ready {
				t.Fatalf("missing data request should not be ready: %+v", result.Requests)
			}
		}
		processed, err := service.RunDueOnce(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range processed {
			if job.Status != StatusWaiting || job.ErrorMessage == "" {
				t.Fatalf("missing data should keep job waiting, got %+v", job)
			}
		}
	})

	t.Run("insufficient qualified window is visible in package", func(t *testing.T) {
		db := newReportTestDB(t)
		project, task := seedReportTaskWithRequests(t, db, "AC-RPT-SHORT-WINDOW", "edge-a", []query.DetectionRunReportRequest{
			{TemplateCode: "SHORT-WINDOW", VarID: 9321, VarName: "temp", VariablesJSON: `[{"var_id":"9321"}]`, Status: "pending"},
		})
		ended := *task.EndedAt
		seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
			{VarID: 9321, VarName: "temp", DisplayName: "温度", Unit: "C", LimitL: floatPtr(0), LimitH: floatPtr(30), Offsets: []time.Duration{-30 * time.Minute, 0}, Values: []float64{10, 12}},
		})
		service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir(), WaitingDelay: time.Millisecond})
		if _, err := service.EnqueueTask(task.ID, "edge-a", false); err != nil {
			t.Fatal(err)
		}
		processed, err := service.RunDueOnce(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(processed) != 1 || processed[0].Status != StatusSuccess {
			t.Fatalf("short but synchronized data should generate report, got %+v", processed)
		}
		pkg := assertReportArtifactPackage(t, processed[0], []int64{9321})
		if status := pkg.Reports[0].Variables[0].Metrics.QualifiedTwoHours.Status; status != "insufficient" {
			t.Fatalf("expected insufficient qualified two-hour window, got %s", status)
		}
	})

	t.Run("missing template fallback and single report mapping failure are isolated", func(t *testing.T) {
		db := newReportTestDB(t)
		artifactDir := t.TempDir()
		project, task := seedReportTaskWithRequests(t, db, "AC-RPT-ONE-FAILS", "edge-a", []query.DetectionRunReportRequest{
			{TemplateCode: "MISSING-TEMPLATE", VarID: 9331, VarName: "temp", ReportName: "fallback ok", VariablesJSON: `[{"var_id":"9331"}]`, ParamsJSON: `{"cell_mapping":{"sheet":"OK","items":[{"cell":"A1","source":"metric.avg","var_id":9331,"required":true}]}}`, Status: "pending"},
			{TemplateCode: "BAD-MAPPING", VarID: 9332, VarName: "pressure", ReportName: "mapping fails", VariablesJSON: `[{"var_id":"9332"}]`, ParamsJSON: `{"cell_mapping":{"sheet":"BAD","items":[{"cell":"A1","source":"param.missing.required","required":true}]}}`, Status: "pending"},
		})
		ended := *task.EndedAt
		seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
			{VarID: 9331, VarName: "temp", DisplayName: "温度", Unit: "C", LimitL: floatPtr(0), LimitH: floatPtr(30), Values: []float64{10, 12, 14}},
			{VarID: 9332, VarName: "pressure", DisplayName: "压力", Unit: "Pa", LimitL: floatPtr(30), LimitH: floatPtr(70), Values: []float64{40, 50, 60}},
		})
		service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: artifactDir, MaxAttempts: 1, WaitingDelay: time.Millisecond, RetryDelay: time.Millisecond})
		if _, err := service.EnqueueTask(task.ID, "edge-a", false); err != nil {
			t.Fatal(err)
		}
		processed, err := service.RunDueOnce(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(processed) != 2 {
			t.Fatalf("expected two processed jobs, got %+v", processed)
		}
		var successJob, failedJob MainReportJob
		for _, job := range processed {
			switch job.Status {
			case StatusSuccess:
				successJob = job
			case StatusFailed:
				failedJob = job
			}
		}
		if successJob.ID == 0 || failedJob.ID == 0 {
			t.Fatalf("expected one success and one failed job, got %+v", processed)
		}
		if failedJob.ArtifactRef != "" || failedJob.ErrorMessage == "" {
			t.Fatalf("failed report should not publish artifact and should record error, got %+v", failedJob)
		}
		workbook := openReportWorkbook(t, successJob.ArtifactRef)
		defer func() { _ = workbook.Close() }()
		assertCellValue(t, workbook, "Report_Run", "B14", filepath.Join(artifactDir, DefaultTemplateFileRef))
		assertCellValue(t, workbook, "Default_Report", "A1", "Spindle Default Report Template")
		assertCellValue(t, workbook, "OK", "A1", "12")
	})
}

func TestEnsureSchemaSeedsDefaultReportTemplate(t *testing.T) {
	db := newReportTestDB(t)
	artifactDir := t.TempDir()
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: artifactDir})
	if err := service.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	var template query.ReportTemplate
	if err := db.First(&template, "template_code = ?", DefaultTemplateCode).Error; err != nil {
		t.Fatal(err)
	}
	if !template.Enabled {
		t.Fatalf("default template should be enabled: %+v", template)
	}
	if template.Version != DefaultTemplateVersion || template.FileRef != DefaultTemplateFileRef || template.FileKind != "xlsx" {
		t.Fatalf("default template seed mismatch: %+v", template)
	}
	if strings.TrimSpace(template.ParamsSchemaJSON) == "" {
		t.Fatalf("default template should expose params schema / cell mapping")
	}

	workbook := openReportWorkbook(t, filepath.Join(artifactDir, DefaultTemplateFileRef))
	defer func() { _ = workbook.Close() }()
	assertCellValue(t, workbook, "Default_Report", "A1", "Spindle Default Report Template")
	assertCellValue(t, workbook, "Default_Report", "A3", "test_no")
}

func TestServiceUsesDefaultTemplateWorkbookAndCellMapping(t *testing.T) {
	db := newReportTestDB(t)
	artifactDir := t.TempDir()
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: artifactDir, WaitingDelay: time.Millisecond, RetryDelay: time.Millisecond})
	if err := service.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	defaultTemplatePath := filepath.Join(artifactDir, DefaultTemplateFileRef)
	if _, err := os.Stat(defaultTemplatePath); err != nil {
		t.Fatalf("default template workbook should be created: %v", err)
	}

	project := query.Project{ProjectCode: "AC-RPT-DEFAULT", Name: "Default Template Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	started := ended.Add(-3 * time.Hour)
	task := query.DetectionTask{TestNo: "RUN-EB069-DEFAULT", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &started, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	var defaultParams map[string]any
	if err := json.Unmarshal([]byte(defaultTemplateParamsSchema()), &defaultParams); err != nil {
		t.Fatal(err)
	}
	defaultParams["operator_note"] = "默认模板验收"
	paramsRaw, err := json.Marshal(defaultParams)
	if err != nil {
		t.Fatal(err)
	}
	request := query.DetectionRunReportRequest{
		TaskID:          task.ID,
		TestNo:          task.TestNo,
		ProjectID:       project.ID,
		ProjectCode:     project.ProjectCode,
		TemplateCode:    DefaultTemplateCode,
		TemplateVersion: DefaultTemplateVersion,
		VarID:           9501,
		VarName:         "supply_air_temp",
		ReportName:      "默认模板验收报表",
		VariablesJSON:   `[{"var_id":"9501"}]`,
		ParamsJSON:      string(paramsRaw),
		Status:          "pending",
	}
	if err := db.Create(&request).Error; err != nil {
		t.Fatal(err)
	}
	seedReportTaskSnapshots(t, db, project, task, ended, []reportVarSeed{
		{VarID: 9501, VarName: "supply_air_temp", DisplayName: "送风温度", Unit: "C", LimitL: floatPtr(10), LimitH: floatPtr(30), Values: []float64{16, 18, 20}},
	})

	if _, err := service.EnqueueTask(task.ID, "edge-a", false); err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunDueOnce(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed) != 1 || processed[0].Status != StatusSuccess {
		t.Fatalf("expected default template report success, got %+v", processed)
	}

	workbook := openReportWorkbook(t, processed[0].ArtifactRef)
	defer func() { _ = workbook.Close() }()
	assertCellValue(t, workbook, "Report_Run", "B10", DefaultTemplateCode)
	assertCellValue(t, workbook, "Report_Run", "B11", "1")
	assertCellValue(t, workbook, "Report_Run", "B14", defaultTemplatePath)
	assertCellValue(t, workbook, "Default_Report", "A1", "Spindle Default Report Template")
	assertCellValue(t, workbook, "Default_Report", "B3", task.TestNo)
	assertCellValue(t, workbook, "Default_Report", "B4", project.ProjectCode)
	assertCellValue(t, workbook, "Default_Report", "B5", "edge-a")
	assertCellValue(t, workbook, "Default_Report", "B6", started.Format(time.RFC3339Nano))
	assertCellValue(t, workbook, "Default_Report", "B7", ended.Format(time.RFC3339Nano))
	assertCellValue(t, workbook, "Default_Report", "B8", "18")
	assertCellValue(t, workbook, "Default_Report", "B9", "18")
	assertCellValue(t, workbook, "Default_Report", "B10", "available")
	assertCellValue(t, workbook, "Default_Report", "B11", "10")
	assertCellValue(t, workbook, "Default_Report", "B12", "30")
	assertCellValue(t, workbook, "Default_Report", "B13", "C")
	assertCellValue(t, workbook, "Default_Report", "B14", "默认模板验收")

	packagePayload := assertReportArtifactPackage(t, processed[0], []int64{9501})
	manifestPackage := readManifestReportPackage(t, processed[0])
	packageVar := packagePayload.Reports[0].Variables[0]
	manifestVar := manifestPackage.Reports[0].Variables[0]
	if packagePayload.Reports[0].TemplateCode != DefaultTemplateCode || manifestPackage.Reports[0].TemplateVersion != DefaultTemplateVersion {
		t.Fatalf("default template identity should be preserved: package=%+v manifest=%+v", packagePayload.Reports[0], manifestPackage.Reports[0])
	}
	if packagePayload.Task.TestNo != task.TestNo || manifestPackage.Task.EdgeInstanceID != "edge-a" || manifestPackage.Task.ProjectCode != project.ProjectCode {
		t.Fatalf("default template task identity should match visible cells: package=%+v manifest=%+v", packagePayload.Task, manifestPackage.Task)
	}
	if packageVar.Metrics.FullDetection.AvgValue == nil || *packageVar.Metrics.FullDetection.AvgValue != 18 {
		t.Fatalf("package full average should match Default_Report B8: %+v", packageVar.Metrics.FullDetection)
	}
	if manifestVar.Metrics.QualifiedTwoHours.AvgValue == nil || *manifestVar.Metrics.QualifiedTwoHours.AvgValue != 18 || manifestVar.Metrics.QualifiedTwoHours.Status != "available" {
		t.Fatalf("manifest qualified metric should match Default_Report B9/B10: %+v", manifestVar.Metrics.QualifiedTwoHours)
	}
	if manifestVar.Limits.LimitL == nil || *manifestVar.Limits.LimitL != 10 || manifestVar.Limits.LimitH == nil || *manifestVar.Limits.LimitH != 30 || manifestVar.Unit != "C" {
		t.Fatalf("manifest limits/unit should match Default_Report B11/B12/B13: %+v", manifestVar)
	}
}

func TestServiceArtifactRejectsNotReadyAndPathEscape(t *testing.T) {
	db := newReportTestDB(t)
	artifactDir := t.TempDir()
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: artifactDir})
	pending := MainReportJob{JobKey: "edge-a:1:1", EdgeInstanceID: "edge-a", TaskID: 1, RequestID: 1, Status: StatusPending, MaxAttempts: 3}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Artifact(pending.ID); err != ErrArtifactNotReady {
		t.Fatalf("pending artifact should not be ready, err=%v", err)
	}
	outside := MainReportJob{JobKey: "edge-a:1:2", EdgeInstanceID: "edge-a", TaskID: 1, RequestID: 2, Status: StatusSuccess, MaxAttempts: 3, ArtifactRef: filepath.Join(t.TempDir(), "outside.json"), ArtifactName: "outside.json"}
	if err := os.WriteFile(outside.ArtifactRef, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&outside).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Artifact(outside.ID); err != ErrArtifactUnavailable {
		t.Fatalf("path escape artifact should be unavailable, err=%v", err)
	}
}

func TestScanQualifiedTwoHourWindowResetsOnViolation(t *testing.T) {
	start := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	limitL := 0.0
	limitH := 100.0
	standard := query.DetectionRunStandardItem{VarID: 9001, CheckEnabled: true, LimitL: &limitL, LimitH: &limitH}
	rows := []query.HistoryData{
		historyValue(9001, start, 40),
		historyValue(9001, start.Add(90*time.Minute), 50),
		historyValue(9001, start.Add(100*time.Minute), 150),
		historyValue(9001, start.Add(2*time.Hour), 60),
		historyValue(9001, start.Add(3*time.Hour), 70),
	}
	metric := scanQualifiedTwoHourWindow(rows, standard)
	if metric.Status != "insufficient" {
		t.Fatalf("violation should reset qualified window, got %+v", metric)
	}
}

func TestServiceWaitsForMissingSynchronizedData(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-RPT-WAIT", Name: "Report Wait Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).Truncate(time.Second)
	task := query.DetectionTask{TestNo: "RUN-RPT-WAIT", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusRunning, StartedAt: &started}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&query.DetectionRunReportRequest{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, TemplateCode: "PERF", VarID: 8201, VarName: "temp", VariablesJSON: `[{"var_id":"8201"}]`, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir(), WaitingDelay: time.Millisecond})
	result, err := service.EnqueueTask(task.ID, "edge-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Status != StatusWaiting {
		t.Fatalf("missing data should enqueue waiting job: %+v", result.Jobs)
	}
	processed, err := service.RunDueOnce(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed) != 1 || processed[0].Status != StatusWaiting || processed[0].ErrorMessage == "" {
		t.Fatalf("worker should keep waiting job: %+v", processed)
	}
	events, _, err := service.ListEvents(processed[0].ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !reportEventsContain(events, EventEnqueued) || !reportEventsContain(events, EventStarted) || !reportEventsContain(events, EventWaiting) {
		t.Fatalf("expected enqueue/start/waiting events, got %+v", events)
	}
}

func TestServiceRejectsRunWithoutReportRequest(t *testing.T) {
	db := newReportTestDB(t)
	project := query.Project{ProjectCode: "AC-RPT-NONE", Name: "Report None Project", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	task := query.DetectionTask{TestNo: "RUN-RPT-NONE", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &ended, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, query.NewStationViewQuery(db), Options{ArtifactDir: t.TempDir()})
	if _, err := service.EnqueueTask(task.ID, "edge-a", false); err != ErrReportNotRequested {
		t.Fatalf("expected ErrReportNotRequested, got %v", err)
	}
}

func newReportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&query.Project{},
		&query.DetectionTask{},
		&query.DetectionRunStandardItem{},
		&query.DetectionRunReportRequest{},
		&query.DetectionRunStorageRoute{},
		&query.DetectionRunNote{},
		&query.DetectionRunReport{},
		&query.ReportTemplate{},
		&query.DetectionRunSummary{},
		&query.DetectionRunFeature{},
		&query.HistoryData{},
		&query.DetectionLimitAlarm{},
		&MainReportJob{},
		&MainReportJobEvent{},
		&MainReportNotification{},
		&MainReportNotificationRecipient{},
		&reportNotificationUser{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&reportNotificationUser{ID: 1, Role: "admin", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func reportEventsContain(events []MainReportJobEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func reportNotificationsContain(items []ReportNotificationDTO, title string) bool {
	for _, item := range items {
		if item.Title == title {
			return true
		}
	}
	return false
}

func assertCellValue(t *testing.T, workbook *excelize.File, sheet string, cell string, expected string) {
	t.Helper()
	value, err := workbook.GetCellValue(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	if value != expected {
		t.Fatalf("unexpected %s!%s value: got %q want %q", sheet, cell, value, expected)
	}
}

func openReportWorkbook(t *testing.T, path string) *excelize.File {
	t.Helper()
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("xlsx artifact should be readable: %v", err)
	}
	return workbook
}

func assertReportArtifactPackage(t *testing.T, job MainReportJob, expectedVarIDs []int64) ReportPackage {
	t.Helper()
	if _, err := os.Stat(job.ArtifactRef); err != nil {
		t.Fatalf("xlsx artifact missing: %v", err)
	}
	workbook := openReportWorkbook(t, job.ArtifactRef)
	defer func() { _ = workbook.Close() }()
	packageJSON, err := workbook.GetCellValue("Report_Package", "A2")
	if err != nil {
		t.Fatal(err)
	}
	var packagePayload ReportPackage
	if err := json.Unmarshal([]byte(packageJSON), &packagePayload); err != nil {
		t.Fatalf("report package sheet should contain json package: %v", err)
	}
	if packagePayload.Kind != "main_server_report_package" || len(packagePayload.Reports) != 1 {
		t.Fatalf("unexpected report package payload: %+v", packagePayload)
	}
	if packagePayload.Reports[0].RequestID != job.RequestID || packagePayload.Task.TaskID != job.TaskID {
		t.Fatalf("package identity does not match job: pkg=%+v job=%+v", packagePayload, job)
	}
	if len(packagePayload.Reports[0].Variables) != len(expectedVarIDs) {
		t.Fatalf("unexpected variable count: pkg=%+v want=%+v", packagePayload.Reports[0].Variables, expectedVarIDs)
	}
	seen := map[int64]bool{}
	for _, variable := range packagePayload.Reports[0].Variables {
		seen[variable.VarID] = true
		if variable.Metrics.FullDetection.Status != "available" {
			t.Fatalf("full detection metric should be available for var_id=%d: %+v", variable.VarID, variable.Metrics.FullDetection)
		}
		if variable.Limits.Source != "detection_run_standard_items" {
			t.Fatalf("limit source should be detection_run_standard_items for var_id=%d: %+v", variable.VarID, variable.Limits)
		}
	}
	for _, expected := range expectedVarIDs {
		if !seen[expected] {
			t.Fatalf("expected variable %d in package variables %+v", expected, packagePayload.Reports[0].Variables)
		}
	}
	manifestPath := filepath.Join(filepath.Dir(job.ArtifactRef), fmt.Sprintf("task-%d-request-%d-manifest.json", job.TaskID, job.RequestID))
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest should exist: %v", err)
	}
	var manifest struct {
		ReportPackage ReportPackage `json:"report_package"`
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("manifest should be json: %v", err)
	}
	if manifest.ReportPackage.Kind != "main_server_report_package" || manifest.ReportPackage.Reports[0].RequestID != job.RequestID {
		t.Fatalf("manifest report_package should match request_id=%d: %s", job.RequestID, string(rawManifest))
	}
	return packagePayload
}

func readManifestReportPackage(t *testing.T, job MainReportJob) ReportPackage {
	t.Helper()
	manifestPath := filepath.Join(filepath.Dir(job.ArtifactRef), fmt.Sprintf("task-%d-request-%d-manifest.json", job.TaskID, job.RequestID))
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest should exist: %v", err)
	}
	var manifest struct {
		ReportPackage ReportPackage `json:"report_package"`
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("manifest should be json: %v", err)
	}
	if manifest.ReportPackage.Kind != "main_server_report_package" {
		t.Fatalf("manifest should include report_package: %s", string(rawManifest))
	}
	return manifest.ReportPackage
}

func writeCustomerTemplateWorkbook(t *testing.T, path string) {
	t.Helper()
	workbook := excelize.NewFile()
	defer func() { _ = workbook.Close() }()
	if err := workbook.SetSheetName("Sheet1", "Customer_Report"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"Customer Acceptance Template"},
		{"test_no", "TEMPLATE_PLACEHOLDER"},
		{"project_code", "TEMPLATE_PLACEHOLDER"},
		{"edge_instance_id", "TEMPLATE_PLACEHOLDER"},
		{"started_at", "TEMPLATE_PLACEHOLDER"},
		{"ended_at", "TEMPLATE_PLACEHOLDER"},
		{"full_avg", "TEMPLATE_PLACEHOLDER"},
		{"qualified_two_hour_avg", "TEMPLATE_PLACEHOLDER"},
		{"limit_l", "TEMPLATE_PLACEHOLDER"},
		{"limit_h", "TEMPLATE_PLACEHOLDER"},
		{"qualified_status", "TEMPLATE_PLACEHOLDER"},
		{"judgement", "TEMPLATE_PLACEHOLDER"},
		{"unit", "TEMPLATE_PLACEHOLDER"},
	}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := workbook.SetCellValue("Customer_Report", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}

type reportVarSeed struct {
	VarID       int64
	VarName     string
	DisplayName string
	Unit        string
	LimitL      *float64
	LimitH      *float64
	Offsets     []time.Duration
	Values      []float64
}

func seedReportTaskWithRequests(t *testing.T, db *gorm.DB, projectCode string, edgeInstanceID string, requests []query.DetectionRunReportRequest) (query.Project, query.DetectionTask) {
	t.Helper()
	project := query.Project{ProjectCode: projectCode, Name: projectCode, EdgeInstanceID: edgeInstanceID, Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ended := time.Now().Add(-time.Minute).Truncate(time.Second)
	started := ended.Add(-3 * time.Hour)
	task := query.DetectionTask{TestNo: "RUN-" + projectCode, ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: query.DetectionStatusStopped, StartedAt: &started, EndedAt: &ended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	for i := range requests {
		requests[i].TaskID = task.ID
		requests[i].TestNo = task.TestNo
		requests[i].ProjectID = project.ID
		requests[i].ProjectCode = project.ProjectCode
		if requests[i].Status == "" {
			requests[i].Status = "pending"
		}
		if err := db.Create(&requests[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return project, task
}

func seedReportTaskSnapshots(t *testing.T, db *gorm.DB, project query.Project, task query.DetectionTask, ended time.Time, vars []reportVarSeed) {
	t.Helper()
	if err := db.Create(&query.DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, ResultStatus: "ok", HistoryRows: int64(len(vars) * 3), StartedAt: task.StartedAt, EndedAt: task.EndedAt, LastRefreshedAt: ended}).Error; err != nil {
		t.Fatal(err)
	}
	for index, variable := range vars {
		values := variable.Values
		if len(values) == 0 {
			values = []float64{10, 12, 14}
		}
		offsets := variable.Offsets
		if len(offsets) == 0 {
			offsets = []time.Duration{-2 * time.Hour, -time.Hour, 0}
		}
		if len(offsets) != len(values) {
			t.Fatalf("seed offsets and values length mismatch for var_id=%d", variable.VarID)
		}
		minValue, maxValue, sum := values[0], values[0], 0.0
		for _, value := range values {
			sum += value
			if value < minValue {
				minValue = value
			}
			if value > maxValue {
				maxValue = value
			}
		}
		avgValue := sum / float64(len(values))
		first := ended.Add(offsets[0])
		last := ended.Add(offsets[len(offsets)-1])
		if err := db.Create(&query.DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: project.ID, ProjectCode: project.ProjectCode, VarID: variable.VarID, VarName: variable.VarName, SampleCount: int64(len(values)), AvgValue: &avgValue, MinValue: &minValue, MaxValue: &maxValue, FirstSampleTime: first, LastSampleTime: last}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&query.DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: variable.VarID, VarName: variable.VarName, DisplayName: variable.DisplayName, Unit: variable.Unit, CheckEnabled: true, AlarmEnabled: true, LimitL: variable.LimitL, LimitH: variable.LimitH, SortOrder: index + 1}).Error; err != nil {
			t.Fatal(err)
		}
		for rowIndex, value := range values {
			value := value
			if err := db.Create(&query.HistoryData{GatewayID: 1, Topic: "topic", ProjectID: project.ID, TaskID: task.ID, TestNo: task.TestNo, VarID: variable.VarID, VarName: variable.VarName, ProjectCode: project.ProjectCode, Value: &value, Quality: 1, SourceTime: ended.Add(offsets[rowIndex])}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func historyValue(varID int64, at time.Time, value float64) query.HistoryData {
	return query.HistoryData{VarID: varID, Value: &value, Quality: 1, SourceTime: at}
}
