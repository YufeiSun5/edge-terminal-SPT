package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type DetectionStandard struct {
	ID               uint                    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	StandardCode     string                  `gorm:"column:standard_code" json:"standard_code"`
	Name             string                  `gorm:"column:name" json:"name"`
	DisplayName      string                  `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN    string                  `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA    string                  `gorm:"column:display_name_ja" json:"display_name_ja"`
	ProjectID        *uint                   `gorm:"column:project_id" json:"project_id,omitempty"`
	ProjectCode      string                  `gorm:"column:project_code" json:"project_code"`
	Mode             string                  `gorm:"column:mode" json:"mode"`
	ReportTemplateID *uint                   `gorm:"column:report_template_id" json:"report_template_id,omitempty"`
	Version          int                     `gorm:"column:version" json:"version"`
	Enabled          bool                    `gorm:"column:enabled" json:"enabled"`
	Remark           string                  `gorm:"column:remark" json:"remark"`
	CreatedAt        time.Time               `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time               `gorm:"column:updated_at" json:"updated_at"`
	Items            []DetectionStandardItem `gorm:"-" json:"items,omitempty"`
}

func (DetectionStandard) TableName() string { return "sys_detection_standards" }

type DetectionStandardItem struct {
	ID              uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	StandardID      uint      `gorm:"column:standard_id" json:"standard_id"`
	VarID           int64     `gorm:"column:var_id" json:"var_id"`
	VarName         string    `gorm:"column:var_name" json:"var_name"`
	DisplayName     string    `gorm:"column:display_name" json:"display_name"`
	DisplayNameEN   string    `gorm:"column:display_name_en" json:"display_name_en"`
	DisplayNameJA   string    `gorm:"column:display_name_ja" json:"display_name_ja"`
	CheckEnabled    bool      `gorm:"column:check_enabled" json:"check_enabled"`
	AlarmEnabled    bool      `gorm:"column:alarm_enabled" json:"alarm_enabled"`
	StoreEnabled    bool      `gorm:"column:store_enabled" json:"store_enabled"`
	CheckCycleMS    int       `gorm:"column:check_cycle_ms" json:"check_cycle_ms"`
	CheckOnStart    bool      `gorm:"column:check_on_start" json:"check_on_start"`
	Required        bool      `gorm:"column:required" json:"required"`
	CheckMethod     string    `gorm:"column:check_method" json:"check_method"`
	TargetValue     string    `gorm:"column:target_value" json:"target_value"`
	LimitLL         *float64  `gorm:"column:limit_ll" json:"limit_ll,omitempty"`
	LimitL          *float64  `gorm:"column:limit_l" json:"limit_l,omitempty"`
	LimitH          *float64  `gorm:"column:limit_h" json:"limit_h,omitempty"`
	LimitHH         *float64  `gorm:"column:limit_hh" json:"limit_hh,omitempty"`
	LimitDeadband   float64   `gorm:"column:limit_deadband" json:"limit_deadband"`
	ViolationHoldMS int       `gorm:"column:violation_hold_ms" json:"violation_hold_ms"`
	RecoverHoldMS   int       `gorm:"column:recover_hold_ms" json:"recover_hold_ms"`
	QualityPolicy   string    `gorm:"column:quality_policy" json:"quality_policy"`
	Unit            string    `gorm:"column:unit" json:"unit"`
	DecimalPlaces   int       `gorm:"column:decimal_places" json:"decimal_places"`
	SortOrder       int       `gorm:"column:sort_order" json:"sort_order"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardItem) TableName() string { return "sys_detection_standard_items" }

func (i DetectionStandardItem) MarshalJSON() ([]byte, error) {
	type alias DetectionStandardItem
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(i),
		VarIDText: strconv.FormatInt(i.VarID, 10),
	})
}

type DetectionStandardFavorite struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"column:user_id" json:"user_id"`
	StandardID uint      `gorm:"column:standard_id" json:"standard_id"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardFavorite) TableName() string { return "sys_detection_standard_favorites" }

type DetectionStandardRecent struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"column:user_id" json:"user_id"`
	StandardID uint      `gorm:"column:standard_id" json:"standard_id"`
	ProjectID  uint      `gorm:"column:project_id" json:"project_id"`
	LastUsedAt time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	UseCount   int       `gorm:"column:use_count" json:"use_count"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (DetectionStandardRecent) TableName() string { return "sys_detection_standard_recents" }

type DetectionStandardFilter struct {
	ProjectID   *uint
	ProjectCode string
	Mode        string
	Enabled     *bool
	Keyword     string
}

func (q *StationViewQuery) ListDetectionStandards(filter DetectionStandardFilter, edgeInstanceID string) ([]DetectionStandard, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, err
		}
	}
	stmt := q.detectionStandardsForEdge(edgeInstanceID)
	if filter.ProjectID != nil {
		stmt = stmt.Where("sys_detection_standards.project_id = ?", *filter.ProjectID)
	}
	if strings.TrimSpace(filter.ProjectCode) != "" {
		stmt = stmt.Where("sys_detection_standards.project_code = ?", strings.TrimSpace(filter.ProjectCode))
	}
	if strings.TrimSpace(filter.Mode) != "" {
		stmt = stmt.Where("sys_detection_standards.mode = ?", strings.TrimSpace(filter.Mode))
	}
	if filter.Enabled != nil {
		stmt = stmt.Where("sys_detection_standards.enabled = ?", *filter.Enabled)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		stmt = stmt.Where(
			"sys_detection_standards.standard_code LIKE ? OR sys_detection_standards.name LIKE ? OR sys_detection_standards.display_name LIKE ? OR sys_detection_standards.display_name_en LIKE ? OR sys_detection_standards.display_name_ja LIKE ?",
			like, like, like, like, like,
		)
	}
	var standards []DetectionStandard
	err := stmt.Order("sys_detection_standards.id asc").Find(&standards).Error
	return standards, err
}

func (q *StationViewQuery) GetDetectionStandard(id uint, edgeInstanceID string) (DetectionStandard, error) {
	var standard DetectionStandard
	if err := q.detectionStandardsForEdge(edgeInstanceID).First(&standard, "sys_detection_standards.id = ?", id).Error; err != nil {
		return standard, err
	}
	var items []DetectionStandardItem
	if err := q.db.Where("standard_id = ?", id).Order("sort_order asc, id asc").Find(&items).Error; err != nil {
		return standard, err
	}
	standard.Items = items
	return standard, nil
}

func (q *StationViewQuery) ListFavoriteDetectionStandards(userID uint, edgeInstanceID string) ([]DetectionStandard, error) {
	if userID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var standards []DetectionStandard
	err := q.detectionStandardsForEdge(edgeInstanceID).
		Joins("JOIN sys_detection_standard_favorites f ON f.standard_id = sys_detection_standards.id AND f.user_id = ?", userID).
		Order("f.updated_at desc, f.id desc").
		Find(&standards).Error
	return standards, err
}

func (q *StationViewQuery) ListRecentDetectionStandards(userID uint, projectID *uint, limit int, edgeInstanceID string) ([]DetectionStandard, int, error) {
	if projectID != nil {
		if _, err := q.projectForEdge(*projectID, edgeInstanceID); err != nil {
			return nil, 0, err
		}
	}
	limit = normalizedDetectionStandardRecentLimit(limit)
	stmt := q.detectionStandardsForEdge(edgeInstanceID).
		Joins("JOIN sys_detection_standard_recents r ON r.standard_id = sys_detection_standards.id").
		Where("r.user_id IN ?", []uint{0, userID})
	if projectID != nil {
		stmt = stmt.Where("r.project_id = ?", *projectID)
	}
	var standards []DetectionStandard
	err := stmt.Order("r.last_used_at desc, r.id desc").Limit(limit).Find(&standards).Error
	return standards, limit, err
}

func (q *StationViewQuery) detectionStandardsForEdge(edgeInstanceID string) *gorm.DB {
	stmt := q.db.Model(&DetectionStandard{}).
		Joins("LEFT JOIN sys_projects p ON p.id = sys_detection_standards.project_id")
	edgeInstanceID = strings.TrimSpace(edgeInstanceID)
	if edgeInstanceID != "" {
		stmt = stmt.Where("(sys_detection_standards.project_id IS NULL OR p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	return stmt
}

func normalizedDetectionStandardRecentLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
