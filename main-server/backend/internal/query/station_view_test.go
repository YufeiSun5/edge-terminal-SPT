package query

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStationViewEffectiveReadsSyncedTables(t *testing.T) {
	db := newStationViewTestDB(t)
	query := NewStationViewQuery(db)

	project := Project{ProjectCode: "AC-SV-01", Name: "Station 1", DisplayName: "工位一", ModelName: "KFR", EdgeInstanceID: "edge-a", Enabled: true}
	otherProject := Project{ProjectCode: "AC-SV-02", Name: "Station 2", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherProject).Error; err != nil {
		t.Fatal(err)
	}
	limitL := 10.0
	limitH := 20.0
	tags := []TagConfig{
		{VarID: 22, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "humidity", VarGroup: "air", DisplayName: "湿度", Unit: "%", DecimalPlaces: 1, Enabled: true},
		{VarID: 11, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", VarGroup: "air", DisplayName: "温度", DisplayNameEN: "Temperature", Unit: "C", DecimalPlaces: 2, DefaultLimitL: &limitL, DefaultLimitH: &limitH, DefaultAlarmEnabled: true, Enabled: true},
		{VarID: 99, ProjectID: &otherProject.ID, ProjectCode: otherProject.ProjectCode, VarName: "temp", VarGroup: "air", DisplayName: "其他项目温度", Enabled: true},
	}
	if err := db.Create(&tags).Error; err != nil {
		t.Fatal(err)
	}
	template := StationViewTemplate{TemplateUID: "tpl-project", TemplateCode: "TPL-PROJECT", Name: "Project template", Version: 2, Status: StationViewStatusPublished, OwnerScope: "main_server"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	regions := []StationViewRegion{
		{TemplateUID: template.TemplateUID, RegionKey: "left", RegionType: "metric_grid", SortOrder: 10, Enabled: true},
		{TemplateUID: template.TemplateUID, RegionKey: "right", RegionType: "inspection_table", SortOrder: 20, Enabled: true},
	}
	if err := db.Create(&regions).Error; err != nil {
		t.Fatal(err)
	}
	items := []StationViewItem{
		{TemplateUID: template.TemplateUID, RegionKey: "left", ItemUID: "left-temp", ItemType: "metric_card", BindingType: StationViewBindingVarName, BindingKey: "temp", SortOrder: 10, Visible: true},
		{TemplateUID: template.TemplateUID, RegionKey: "right", ItemUID: "right-run", ItemType: "inspection_row", BindingType: StationViewBindingRunItems, SortOrder: 20, Visible: true},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	assignment := StationViewAssignment{TemplateUID: template.TemplateUID, TargetType: StationViewTargetProject, TargetKey: project.ProjectCode, Priority: 10, Enabled: true}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute)
	task := DetectionTask{TestNo: "SV-RUN", ProjectID: project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Status: DetectionStatusPaused, StartedAt: &started}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	runItems := []DetectionRunStandardItem{
		{TaskID: task.ID, TestNo: task.TestNo, VarID: 33, VarName: "pressure", DisplayName: "压力", Unit: "Pa", DecimalPlaces: 0, CheckEnabled: true, AlarmEnabled: true, SortOrder: 30},
		{TaskID: task.ID, TestNo: task.TestNo, VarID: 11, VarName: "temp", DisplayName: "运行温度", Unit: "C", DecimalPlaces: 2, CheckEnabled: true, AlarmEnabled: true, SortOrder: 10},
	}
	if err := db.Create(&runItems).Error; err != nil {
		t.Fatal(err)
	}

	effective, err := query.Effective(project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if effective.EdgeInstanceID != "edge-a" || effective.Template.TemplateCode != "TPL-PROJECT" {
		t.Fatalf("unexpected effective context/template: %+v", effective)
	}
	if len(effective.Items) != 2 || effective.Items[0].ResolvedBindings[0].VarID != 11 {
		t.Fatalf("var_name should resolve inside current project only: %+v", effective.Items)
	}
	if runBindings := effective.Items[1].ResolvedBindings; len(runBindings) != 2 || runBindings[0].VarID != 11 || runBindings[1].VarID != 33 {
		t.Fatalf("run bindings should use current paused task sorted by sort_order: %+v", runBindings)
	}
	if got := strings.Join(effective.WSSubscription.VarIDs, ","); got != "11,33" {
		t.Fatalf("unexpected ws var ids: %s", got)
	}
	if _, err := query.Effective(project.ID, "edge-b"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("edge mismatch should not leak project, got %v", err)
	}
}

func TestStationViewEffectiveReturnsSyncNotReadyWithoutSyncedTemplate(t *testing.T) {
	db := newStationViewTestDB(t)
	query := NewStationViewQuery(db)
	project := Project{ProjectCode: "AC-NO-TPL", Name: "No Template", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Effective(project.ID, "edge-a"); !errors.Is(err, ErrStationViewSyncNotReady) {
		t.Fatalf("expected sync not ready, got %v", err)
	}
}

func TestSyncedProjectsAndCurrentDetectionRun(t *testing.T) {
	db := newStationViewTestDB(t)
	query := NewStationViewQuery(db)
	projectA := Project{ProjectCode: "AC-A", EdgeInstanceID: "edge-a", Name: "Project A", Enabled: true}
	projectB := Project{ProjectCode: "AC-B", EdgeInstanceID: "edge-b", Name: "Project B", Enabled: true}
	legacyProject := Project{ProjectCode: "AC-LEGACY", Name: "Legacy Project", Enabled: true}
	for _, project := range []*Project{&projectA, &projectB, &legacyProject} {
		if err := db.Create(project).Error; err != nil {
			t.Fatal(err)
		}
	}
	if projectA.ID == 0 {
		t.Fatal("project ID was not assigned")
	}
	projects, err := query.ListProjects("edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].ProjectCode != "AC-A" || projects[1].ProjectCode != "AC-LEGACY" {
		t.Fatalf("expected edge-a plus legacy projects, got %+v", projects)
	}

	started := time.Now().Add(-time.Minute)
	task := DetectionTask{
		TestNo:            "RUN-A",
		ProjectID:         projectA.ID,
		ProjectCode:       projectA.ProjectCode,
		Mode:              "standard",
		Status:            DetectionStatusRunning,
		StandardCode:      "STD-A",
		LimitCheckEnabled: true,
		StartedAt:         &started,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	item := DetectionRunStandardItem{TaskID: task.ID, TestNo: task.TestNo, VarID: 9001, VarName: "temp", DisplayName: "温度", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true, SortOrder: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	route := DetectionRunStorageRoute{TaskID: task.ID, TestNo: task.TestNo, ProjectID: projectA.ID, VarID: item.VarID, RouteCode: "default", StorageTarget: "wide_table", StorageTable: "rt_project_1_data", ColumnName: "temp", ColumnType: "DOUBLE"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	request := DetectionRunReportRequest{TaskID: task.ID, TestNo: task.TestNo, ProjectID: projectA.ID, ProjectCode: projectA.ProjectCode, VarID: item.VarID, VarName: item.VarName, VariablesJSON: `[{"var_id":"9001"}]`, ParamsJSON: `{"area":1.2}`, Status: "pending"}
	if err := db.Create(&request).Error; err != nil {
		t.Fatal(err)
	}
	summary := DetectionRunSummary{TaskID: task.ID, TestNo: task.TestNo, ProjectID: projectA.ID, ProjectCode: projectA.ProjectCode, ResultStatus: "running", StartedAt: task.StartedAt, LastRefreshedAt: time.Now()}
	if err := db.Create(&summary).Error; err != nil {
		t.Fatal(err)
	}
	avg := 12.3
	feature := DetectionRunFeature{TaskID: task.ID, TestNo: task.TestNo, ProjectID: projectA.ID, ProjectCode: projectA.ProjectCode, VarID: item.VarID, VarName: item.VarName, SampleCount: 3, AvgValue: &avg, FirstSampleTime: started, LastSampleTime: time.Now()}
	if err := db.Create(&feature).Error; err != nil {
		t.Fatal(err)
	}
	event := DetectionRunEvent{TaskID: task.ID, TestNo: task.TestNo, ProjectID: projectA.ID, ProjectCode: projectA.ProjectCode, EventType: "started", EventLevel: "info", OccurredAt: started}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}

	current, err := query.CurrentDetectionRun(projectA.ID, "edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != task.ID || len(current.StandardItems) != 1 || len(current.StorageRoutes) != 1 || len(current.ReportRequests) != 1 {
		t.Fatalf("current run should include synced snapshots: %+v", current)
	}
	if _, err := query.CurrentDetectionRun(projectA.ID, "edge-b"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("edge mismatch should not return current run, got %v", err)
	}
	tasks, limit, err := query.ListDetectionRuns(DetectionRunFilter{ProjectID: &projectA.ID}, "edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if limit != 200 || len(tasks) != 1 || tasks[0].ID != task.ID || len(tasks[0].StandardItems) != 1 || len(tasks[0].ReportRequests) != 0 {
		t.Fatalf("list should include lightweight run snapshots only: limit=%d tasks=%+v", limit, tasks)
	}
	if _, _, err := query.ListDetectionRuns(DetectionRunFilter{ProjectID: &projectA.ID}, "edge-b"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("edge mismatch should not list project runs, got %v", err)
	}
	detail, err := query.GetDetectionRun(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ReportRequests) != 1 || len(detail.StorageRoutes) != 1 {
		t.Fatalf("detail should include report requests and storage routes: %+v", detail)
	}
	if got, err := query.DetectionRunSummary(task.ID); err != nil || got.TaskID != task.ID {
		t.Fatalf("summary mismatch: %+v err=%v", got, err)
	}
	if got, err := query.DetectionRunFeatures(task.ID); err != nil || len(got) != 1 || got[0].VarID != item.VarID {
		t.Fatalf("features mismatch: %+v err=%v", got, err)
	}
	if got, limit, err := query.DetectionRunEvents(task.ID, 0); err != nil || limit != 200 || len(got) != 1 || got[0].EventType != "started" {
		t.Fatalf("events mismatch: limit=%d %+v err=%v", limit, got, err)
	}
}

func newStationViewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&Project{},
		&TagConfig{},
		&StationViewTemplate{},
		&StationViewRegion{},
		&StationViewItem{},
		&StationViewAssignment{},
		&DetectionTask{},
		&DetectionRunStandardItem{},
		&DetectionRunStorageRoute{},
		&DetectionRunNote{},
		&DetectionRunReport{},
		&DetectionRunReportRequest{},
		&DetectionRunEvent{},
		&DetectionRunSummary{},
		&DetectionRunFeature{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}
