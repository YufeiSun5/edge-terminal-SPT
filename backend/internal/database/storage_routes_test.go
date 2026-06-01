package database

import (
	"errors"
	"testing"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

func TestNormalizeStorageColumnNameAndValidation(t *testing.T) {
	cases := map[string]string{
		"Temp In":       "temp_in",
		"123_pressure":  "v_123_pressure",
		"温度入口":          "var_42",
		"fan-speed.rpm": "fan_speed_rpm",
		"already_valid": "already_valid",
	}
	for input, want := range cases {
		if got := NormalizeStorageColumnName(input, "fallback", 42); got != want {
			t.Fatalf("NormalizeStorageColumnName(%q)=%q want=%q", input, got, want)
		}
	}
	if err := ValidateStorageIdentifier("valid_name_1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStorageIdentifier("1_bad"); err == nil {
		t.Fatal("expected invalid identifier error")
	}
	if StorageColumnTypeForDataType("STRING") != "TEXT" || StorageColumnTypeForDataType("BOOL") != "TINYINT(1)" || StorageColumnTypeForDataType("INT") != "BIGINT" || StorageColumnTypeForDataType("FLOAT") != "DOUBLE" {
		t.Fatal("unexpected storage column type mapping")
	}
}

func TestInsertWideHistoryBatchUpsertsDynamicColumns(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	ProjectID := uint(3)
	tempRoute := models.DetectionRunStorageRoute{
		TaskID:        77,
		TestNo:        "T-WIDE",
		ProjectID:     ProjectID,
		VarID:         700,
		RouteID:       1,
		RouteCode:     "route-temp",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  ProjectWideTableName(ProjectID),
		ColumnName:    "temp_in",
		ColumnType:    "DOUBLE",
		QueryAlias:    "Temp In",
	}
	statusRoute := models.DetectionRunStorageRoute{
		TaskID:        77,
		TestNo:        "T-WIDE",
		ProjectID:     ProjectID,
		VarID:         701,
		RouteID:       2,
		RouteCode:     "route-status",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  ProjectWideTableName(ProjectID),
		ColumnName:    "status_text",
		ColumnType:    "TEXT",
		QueryAlias:    "Status Text",
	}
	if err := db.Create(&[]models.DetectionRunStorageRoute{tempRoute, statusRoute}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.EnsureProjectWideTable(ProjectID, []models.DetectionRunStorageRoute{tempRoute, statusRoute}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 5, 30, 9, 0, 0, 123000000, time.UTC)
	if err := repo.InsertWideHistoryBatch([]*models.StoreTask{
		{
			ProjectID:     ProjectID,
			TaskID:        77,
			TestNo:        "T-WIDE",
			VarID:         700,
			ProjectCode:   "D-3",
			Value:         23.5,
			Timestamp:     at,
			StorageRoutes: []models.DetectionRunStorageRoute{tempRoute},
		},
		{
			ProjectID:     ProjectID,
			TaskID:        77,
			TestNo:        "T-WIDE",
			VarID:         701,
			ProjectCode:   "D-3",
			StrValue:      "OK",
			IsString:      true,
			Timestamp:     at,
			StorageRoutes: []models.DetectionRunStorageRoute{statusRoute},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var rowCount int64
	if err := db.Table(ProjectWideTableName(ProjectID)).
		Where("task_id = ? AND sample_bucket_ms = ?", 77, at.UnixMilli()).
		Count(&rowCount).Error; err != nil {
		t.Fatal(err)
	}
	if rowCount != 1 {
		t.Fatalf("expected one merged wide row, got %d", rowCount)
	}
	var got struct {
		TempIn     float64
		StatusText string
	}
	if err := db.Table(ProjectWideTableName(ProjectID)).
		Select("temp_in, status_text").
		Where("task_id = ? AND sample_bucket_ms = ?", 77, at.UnixMilli()).
		Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.TempIn != 23.5 || got.StatusText != "OK" {
		t.Fatalf("expected merged wide values, got %+v", got)
	}
	taskID := uint(77)
	historyRows, err := repo.QueryHistoryData(HistoryFilter{TaskID: &taskID})
	if err != nil {
		t.Fatal(err)
	}
	if len(historyRows) != 2 {
		t.Fatalf("expected two history rows reconstructed from wide row, got %d", len(historyRows))
	}
	byVarID := map[int64]models.HistoryData{}
	for _, row := range historyRows {
		byVarID[row.VarID] = row
	}
	if byVarID[700].Value == nil || *byVarID[700].Value != 23.5 || byVarID[700].VarName != "Temp In" {
		t.Fatalf("unexpected temp history row: %+v", byVarID[700])
	}
	if byVarID[701].StrValue == nil || *byVarID[701].StrValue != "OK" || byVarID[701].VarName != "Status Text" {
		t.Fatalf("unexpected status history row: %+v", byVarID[701])
	}
	if err := repo.InsertWideHistoryBatch([]*models.StoreTask{{
		ProjectID:     ProjectID,
		TaskID:        77,
		TestNo:        "T-WIDE",
		VarID:         700,
		ProjectCode:   "D-3",
		Value:         24.25,
		Timestamp:     at,
		StorageRoutes: []models.DetectionRunStorageRoute{tempRoute},
	}}); err != nil {
		t.Fatal(err)
	}
	var updatedTemp float64
	if err := db.Table(ProjectWideTableName(ProjectID)).
		Select("temp_in").
		Where("task_id = ? AND sample_bucket_ms = ?", 77, at.UnixMilli()).
		Scan(&updatedTemp).Error; err != nil {
		t.Fatal(err)
	}
	if updatedTemp != 24.25 {
		t.Fatalf("expected wide upsert value 24.25, got %v", updatedTemp)
	}
}

func TestQueryWideHistoryDataHandlesLegacyProjectTableWithoutIdentityColumns(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	projectID := uint(9)
	tableName := ProjectWideTableName(projectID)
	route := models.DetectionRunStorageRoute{
		TaskID:        88,
		TestNo:        "T-LEGACY-WIDE",
		ProjectID:     projectID,
		VarID:         8801,
		RouteID:       1,
		RouteCode:     "legacy-temp",
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  tableName,
		ColumnName:    "temp_in",
		ColumnType:    "DOUBLE",
		QueryAlias:    "Temp In",
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE ` + quoteIdentifier(db.Name(), tableName) + ` (
id INTEGER PRIMARY KEY AUTOINCREMENT,
task_id INTEGER NOT NULL,
test_no TEXT DEFAULT '',
sample_time DATETIME NOT NULL,
sample_bucket_ms INTEGER NOT NULL,
temp_in DOUBLE NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	if err := db.Table(tableName).Create(map[string]any{
		"task_id":          88,
		"test_no":          "T-LEGACY-WIDE",
		"sample_time":      at,
		"sample_bucket_ms": at.UnixMilli(),
		"temp_in":          31.5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.EnsureProjectWideTable(projectID, []models.DetectionRunStorageRoute{route}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(tableName, "project_id") || !db.Migrator().HasColumn(tableName, "project_code") {
		t.Fatal("expected legacy wide table identity columns to be added")
	}
	rows, err := repo.QueryHistoryData(HistoryFilter{ProjectID: &projectID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one history row reconstructed from legacy wide table, got %d", len(rows))
	}
	if rows[0].ProjectID != projectID || rows[0].VarID != route.VarID || rows[0].Value == nil || *rows[0].Value != 31.5 {
		t.Fatalf("unexpected legacy history row: %+v", rows[0])
	}
}

func TestDefaultStorageRouteAndProjectWideTableSchema(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	Project := &models.Project{ProjectCode: "D-ROUTE", Name: "Route Project", Enabled: true}
	if err := repo.CreateProject(Project); err != nil {
		t.Fatal(err)
	}
	tag := models.TagConfig{
		VarID:       700,
		GatewayID:   1,
		SourcePath:  "temp.in",
		RawName:     "Temp In",
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		VarName:     "Temp In",
		DisplayName: "Temperature In",
		JSONPath:    "temp.in",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	route, err := repo.EnsureDefaultStorageRouteForTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	if route == nil || route.StorageTarget != models.StorageTargetWideTable || route.StorageTable != ProjectWideTableName(Project.ID) || route.ColumnName != "temp_in" || route.CycleMS != 0 || route.StoreOnStart || route.Enabled {
		t.Fatalf("unexpected default route: %+v", route)
	}
	route.Enabled = true
	route.TriggerMode = models.StoreTriggerOnCycle
	route.CycleMS = 3000
	route.StoreOnStart = true
	runRoute := models.DetectionRunStorageRoute{
		TaskID:        9,
		TestNo:        "T-ROUTE",
		ProjectID:     Project.ID,
		VarID:         tag.VarID,
		RouteID:       route.ID,
		RouteCode:     route.RouteCode,
		StorageTarget: route.StorageTarget,
		StorageTable:  route.StorageTable,
		ColumnName:    route.ColumnName,
		ColumnType:    route.ColumnType,
	}
	if err := repo.EnsureProjectWideTable(Project.ID, []models.DetectionRunStorageRoute{runRoute}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(ProjectWideTableName(Project.ID)) {
		t.Fatal("expected Project wide table")
	}
	for _, column := range []string{"task_id", "sample_bucket_ms", "temp_in"} {
		if !db.Migrator().HasColumn(ProjectWideTableName(Project.ID), column) {
			t.Fatalf("expected column %s", column)
		}
	}
	if err := repo.EnsureProjectWideTable(Project.ID, []models.DetectionRunStorageRoute{runRoute}); err != nil {
		t.Fatal(err)
	}
}

func TestStorageRouteSkipsUnassignedAndRejectsInvalidSchema(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	unassigned := models.TagConfig{VarID: 800, VarName: "temp", DataType: "FLOAT"}
	route, err := repo.EnsureDefaultStorageRouteForTag(unassigned)
	if err != nil {
		t.Fatal(err)
	}
	if route != nil {
		t.Fatalf("unassigned variable should not create route: %+v", route)
	}
	customTable := "custom_project_data"
	if err := repo.EnsureProjectWideTable(1, []models.DetectionRunStorageRoute{{
		ProjectID:     1,
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  customTable,
		ColumnName:    "custom_value",
		ColumnType:    "DOUBLE",
	}}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(customTable) || !db.Migrator().HasColumn(customTable, "custom_value") {
		t.Fatal("expected custom storage table and column")
	}
	if err := repo.EnsureProjectWideTable(1, []models.DetectionRunStorageRoute{{
		ProjectID:     1,
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  ProjectWideTableName(1),
		ColumnName:    "bad-name",
		ColumnType:    "DOUBLE",
	}}); err == nil {
		t.Fatal("expected invalid column error")
	}
}

func TestCreateStorageRouteAllowsCustomTableAndRouteOwnedTiming(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	Project := &models.Project{ProjectCode: "D-CUSTOM", Name: "Custom Project", Enabled: true}
	if err := repo.CreateProject(Project); err != nil {
		t.Fatal(err)
	}
	tag := models.TagConfig{
		VarID:       901,
		GatewayID:   1,
		SourcePath:  "flow",
		RawName:     "Flow",
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		VarName:     "Flow",
		JSONPath:    "flow",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		Enabled:     true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	route := &models.StorageRoute{
		ProjectID:     Project.ID,
		VarID:         tag.VarID,
		StorageTarget: models.StorageTargetWideTable,
		StorageTable:  "custom_flow_data",
		ColumnName:    "flow_value",
		TriggerMode:   models.StoreTriggerOnCycle,
		CycleMS:       5000,
		StoreOnStart:  true,
		Enabled:       true,
	}
	if err := repo.CreateStorageRoute(route); err != nil {
		t.Fatal(err)
	}
	if route.ColumnType != "DOUBLE" || route.RouteCode == "" {
		t.Fatalf("expected route defaults, got %+v", route)
	}
	updated, err := repo.UpdateStorageRoute(route.ID, map[string]interface{}{"cycle_ms": 10000, "deadband": 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CycleMS != 10000 || updated.Deadband != 0.5 || updated.StorageTable != "custom_flow_data" {
		t.Fatalf("unexpected updated route: %+v", updated)
	}
	if err := repo.EnsureProjectWideTable(Project.ID, []models.DetectionRunStorageRoute{{
		TaskID:        1,
		ProjectID:     Project.ID,
		VarID:         tag.VarID,
		RouteID:       route.ID,
		StorageTarget: route.StorageTarget,
		StorageTable:  route.StorageTable,
		ColumnName:    route.ColumnName,
		ColumnType:    route.ColumnType,
	}}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("custom_flow_data") || !db.Migrator().HasColumn("custom_flow_data", "flow_value") {
		t.Fatal("expected custom storage table schema")
	}
	if err := repo.DeleteStorageRoute(route.ID + 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing storage route delete to return record not found, got %v", err)
	}
}
