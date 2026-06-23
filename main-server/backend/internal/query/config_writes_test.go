package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateDetectionStandardValidatesProjectVariableAndStableHash(t *testing.T) {
	db := newConfigWriteTestDB(t)
	q := NewStationViewQuery(db)
	project := Project{ProjectCode: "AC-CFG", Name: "Config Project", ProjectGroup: "AC", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := TagConfig{VarID: 9101, GatewayID: 1, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", DisplayName: "温度", Unit: "C", DecimalPlaces: 1, Enabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	limitL := 10.0
	limitH := 20.0
	standard, err := q.CreateDetectionStandard(&DetectionStandard{
		StandardCode: "CFG-IMPORT-1",
		Name:         "Imported Config",
		ProjectID:    &project.ID,
		ProjectCode:  project.ProjectCode,
		Mode:         "standard",
		Enabled:      true,
	}, []DetectionStandardItem{{
		VarID:           tag.VarID,
		CheckEnabled:    true,
		AlarmEnabled:    true,
		StoreEnabled:    true,
		CheckCycleMS:    3000,
		CheckOnStart:    true,
		LimitL:          &limitL,
		LimitH:          &limitH,
		ViolationHoldMS: 3000,
		RecoverHoldMS:   3000,
		SortOrder:       1,
	}}, SyncWriteMeta{EdgeInstanceID: "edge-a", UpdatedByUser: "admin", UpdatedByNode: "main-server"})
	if err != nil {
		t.Fatal(err)
	}
	if standard.ProjectCode != project.ProjectCode || standard.ProjectGroup != project.ProjectGroup || standard.EdgeInstanceID != "edge-a" || standard.SyncScope != "edge" || standard.ConfigHash == "" {
		t.Fatalf("unexpected standard: %+v", standard)
	}
	if len(standard.Items) != 1 {
		t.Fatalf("expected one item: %+v", standard.Items)
	}
	item := standard.Items[0]
	if item.VarName != tag.VarName || item.DisplayName != tag.DisplayName || item.Unit != tag.Unit || item.DecimalPlaces != 2 {
		t.Fatalf("item should be hydrated from tag config: %+v", item)
	}
	expected := expectedDetectionStandardHash(standard, standard.Items)
	if standard.ConfigHash != expected {
		t.Fatalf("config_hash mismatch: got %s want %s", standard.ConfigHash, expected)
	}
}

func TestCreateDetectionStandardRejectsWrongProjectVariable(t *testing.T) {
	db := newConfigWriteTestDB(t)
	q := NewStationViewQuery(db)
	project := Project{ProjectCode: "AC-A", Name: "A", EdgeInstanceID: "edge-a", Enabled: true}
	other := Project{ProjectCode: "AC-B", Name: "B", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&TagConfig{VarID: 9201, GatewayID: 1, ProjectID: &other.ID, ProjectCode: other.ProjectCode, VarName: "temp", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := q.CreateDetectionStandard(&DetectionStandard{
		StandardCode: "CFG-WRONG-VAR",
		Name:         "Wrong Var",
		ProjectID:    &project.ID,
		ProjectCode:  project.ProjectCode,
		Mode:         "standard",
		Enabled:      true,
	}, []DetectionStandardItem{{VarID: 9201, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true}}, SyncWriteMeta{EdgeInstanceID: "edge-a"})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected project variable validation error, got %v", err)
	}
}

func TestCreateDetectionStandardRejectsDisabledVariable(t *testing.T) {
	db := newConfigWriteTestDB(t)
	q := NewStationViewQuery(db)
	project := Project{ProjectCode: "AC-DISABLED", Name: "Disabled", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&TagConfig{VarID: 9301, GatewayID: 1, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", Enabled: false}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := q.CreateDetectionStandard(&DetectionStandard{
		StandardCode: "CFG-DISABLED-VAR",
		Name:         "Disabled Var",
		ProjectID:    &project.ID,
		ProjectCode:  project.ProjectCode,
		Mode:         "standard",
		Enabled:      true,
	}, []DetectionStandardItem{{VarID: 9301, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true}}, SyncWriteMeta{EdgeInstanceID: "edge-a"})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled variable validation error, got %v", err)
	}
}

func TestUpdateDetectionStandardHydratesProjectGroupOnProjectChange(t *testing.T) {
	db := newConfigWriteTestDB(t)
	q := NewStationViewQuery(db)
	project := Project{ProjectCode: "AC-UPD", Name: "Update Project", ProjectGroup: "AC", EdgeInstanceID: "edge-a", Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	tag := TagConfig{VarID: 9401, GatewayID: 1, ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", DisplayName: "温度", Enabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	standard := DetectionStandard{
		StandardCode: "CFG-UPDATE-PROJECT",
		Name:         "Update Project",
		Mode:         "standard",
		Version:      1,
		Enabled:      true,
	}
	if err := db.Create(&standard).Error; err != nil {
		t.Fatal(err)
	}
	updated, err := q.UpdateDetectionStandard(standard.ID, map[string]any{
		"project_id":   project.ID,
		"project_code": project.ProjectCode,
	}, &[]DetectionStandardItem{{VarID: tag.VarID, CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true}}, SyncWriteMeta{EdgeInstanceID: "edge-a"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProjectID == nil || *updated.ProjectID != project.ID || updated.ProjectGroup != project.ProjectGroup || updated.ConfigHash == "" {
		t.Fatalf("project group should be hydrated on project change: %+v", updated)
	}
}

func TestDeleteDetectionStandardRemovesItems(t *testing.T) {
	db := newConfigWriteTestDB(t)
	q := NewStationViewQuery(db)
	standard := DetectionStandard{StandardCode: "CFG-DELETE", Name: "Delete", Mode: "standard", Version: 1, Enabled: true}
	if err := db.Create(&standard).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&DetectionStandardItem{StandardID: standard.ID, VarID: 9501, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteDetectionStandard(standard.ID); err != nil {
		t.Fatal(err)
	}
	var standardCount int64
	if err := db.Model(&DetectionStandard{}).Where("id = ?", standard.ID).Count(&standardCount).Error; err != nil {
		t.Fatal(err)
	}
	var itemCount int64
	if err := db.Model(&DetectionStandardItem{}).Where("standard_id = ?", standard.ID).Count(&itemCount).Error; err != nil {
		t.Fatal(err)
	}
	if standardCount != 0 || itemCount != 0 {
		t.Fatalf("standard and items should be deleted, standards=%d items=%d", standardCount, itemCount)
	}
	if err := q.DeleteDetectionStandard(standard.ID); err == nil {
		t.Fatal("expected deleting a missing standard to fail")
	}
}

func newConfigWriteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Project{}, &TagConfig{}, &DetectionStandard{}, &DetectionStandardItem{}, &DetectionStandardFavorite{}, &DetectionStandardRecent{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func expectedDetectionStandardHash(standard DetectionStandard, items []DetectionStandardItem) string {
	type hashItem struct {
		VarID           int64    `json:"var_id"`
		VarName         string   `json:"var_name"`
		DisplayName     string   `json:"display_name"`
		DisplayNameEN   string   `json:"display_name_en"`
		DisplayNameJA   string   `json:"display_name_ja"`
		CheckEnabled    bool     `json:"check_enabled"`
		AlarmEnabled    bool     `json:"alarm_enabled"`
		StoreEnabled    bool     `json:"store_enabled"`
		CheckCycleMS    int      `json:"check_cycle_ms"`
		CheckOnStart    bool     `json:"check_on_start"`
		Required        bool     `json:"required"`
		CheckMethod     string   `json:"check_method"`
		TargetValue     string   `json:"target_value"`
		LimitLL         *float64 `json:"limit_ll"`
		LimitL          *float64 `json:"limit_l"`
		LimitH          *float64 `json:"limit_h"`
		LimitHH         *float64 `json:"limit_hh"`
		LimitDeadband   float64  `json:"limit_deadband"`
		ViolationHoldMS int      `json:"violation_hold_ms"`
		RecoverHoldMS   int      `json:"recover_hold_ms"`
		QualityPolicy   string   `json:"quality_policy"`
		Unit            string   `json:"unit"`
		DecimalPlaces   int      `json:"decimal_places"`
		SortOrder       int      `json:"sort_order"`
	}
	payload := struct {
		StandardCode     string     `json:"standard_code"`
		Name             string     `json:"name"`
		DisplayName      string     `json:"display_name"`
		DisplayNameEN    string     `json:"display_name_en"`
		DisplayNameJA    string     `json:"display_name_ja"`
		ProjectCode      string     `json:"project_code"`
		ProjectGroup     string     `json:"project_group"`
		Mode             string     `json:"mode"`
		ReportTemplateID *uint      `json:"report_template_id"`
		Version          int        `json:"version"`
		Enabled          bool       `json:"enabled"`
		Items            []hashItem `json:"items"`
	}{
		StandardCode:     strings.TrimSpace(standard.StandardCode),
		Name:             strings.TrimSpace(standard.Name),
		DisplayName:      strings.TrimSpace(standard.DisplayName),
		DisplayNameEN:    strings.TrimSpace(standard.DisplayNameEN),
		DisplayNameJA:    strings.TrimSpace(standard.DisplayNameJA),
		ProjectCode:      strings.TrimSpace(standard.ProjectCode),
		ProjectGroup:     strings.TrimSpace(standard.ProjectGroup),
		Mode:             strings.TrimSpace(standard.Mode),
		ReportTemplateID: standard.ReportTemplateID,
		Version:          standard.Version,
		Enabled:          standard.Enabled,
		Items:            make([]hashItem, 0, len(items)),
	}
	sorted := append([]DetectionStandardItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SortOrder != sorted[j].SortOrder {
			return sorted[i].SortOrder < sorted[j].SortOrder
		}
		if sorted[i].VarID != sorted[j].VarID {
			return sorted[i].VarID < sorted[j].VarID
		}
		return sorted[i].VarName < sorted[j].VarName
	})
	for _, item := range sorted {
		if item.CheckMethod == "" {
			item.CheckMethod = "numeric_range"
		}
		if item.QualityPolicy == "" {
			item.QualityPolicy = "ignore_bad"
		}
		if item.DecimalPlaces == 0 {
			item.DecimalPlaces = 2
		}
		payload.Items = append(payload.Items, hashItem{
			VarID:           item.VarID,
			VarName:         strings.TrimSpace(item.VarName),
			DisplayName:     strings.TrimSpace(item.DisplayName),
			DisplayNameEN:   strings.TrimSpace(item.DisplayNameEN),
			DisplayNameJA:   strings.TrimSpace(item.DisplayNameJA),
			CheckEnabled:    item.CheckEnabled,
			AlarmEnabled:    item.AlarmEnabled,
			StoreEnabled:    item.StoreEnabled,
			CheckCycleMS:    item.CheckCycleMS,
			CheckOnStart:    item.CheckOnStart,
			Required:        item.Required,
			CheckMethod:     strings.TrimSpace(item.CheckMethod),
			TargetValue:     strings.TrimSpace(item.TargetValue),
			LimitLL:         item.LimitLL,
			LimitL:          item.LimitL,
			LimitH:          item.LimitH,
			LimitHH:         item.LimitHH,
			LimitDeadband:   item.LimitDeadband,
			ViolationHoldMS: item.ViolationHoldMS,
			RecoverHoldMS:   item.RecoverHoldMS,
			QualityPolicy:   strings.TrimSpace(item.QualityPolicy),
			Unit:            strings.TrimSpace(item.Unit),
			DecimalPlaces:   item.DecimalPlaces,
			SortOrder:       item.SortOrder,
		})
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
