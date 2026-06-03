package query

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type StorageRoute struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ProjectID     uint      `gorm:"column:project_id" json:"project_id"`
	VarID         int64     `gorm:"column:var_id" json:"var_id"`
	RouteCode     string    `gorm:"column:route_code" json:"route_code"`
	StorageTarget string    `gorm:"column:storage_target" json:"storage_target"`
	StorageTable  string    `gorm:"column:table_name" json:"table_name"`
	ColumnName    string    `gorm:"column:column_name" json:"column_name"`
	ColumnType    string    `gorm:"column:column_type" json:"column_type"`
	FormFieldKey  string    `gorm:"column:form_field_key" json:"form_field_key"`
	QueryAlias    string    `gorm:"column:query_alias" json:"query_alias"`
	TriggerMode   string    `gorm:"column:trigger_mode" json:"trigger_mode"`
	CycleMS       int       `gorm:"column:cycle_ms" json:"cycle_ms"`
	Deadband      float64   `gorm:"column:deadband" json:"deadband"`
	StoreOnStart  bool      `gorm:"column:store_on_start" json:"store_on_start"`
	Enabled       bool      `gorm:"column:enabled" json:"enabled"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (StorageRoute) TableName() string { return "sys_storage_routes" }

func (r StorageRoute) MarshalJSON() ([]byte, error) {
	type alias StorageRoute
	return json.Marshal(struct {
		alias
		VarIDText string `json:"var_id_text"`
	}{
		alias:     alias(r),
		VarIDText: strconv.FormatInt(r.VarID, 10),
	})
}

type StorageRouteFilter struct {
	ProjectID *uint
	VarID     *int64
	Enabled   *bool
}

func (q *StationViewQuery) ListStorageRoutes(filter StorageRouteFilter, edgeInstanceID string) ([]StorageRoute, error) {
	if filter.ProjectID != nil {
		if _, err := q.projectForEdge(*filter.ProjectID, edgeInstanceID); err != nil {
			return nil, err
		}
	}
	stmt := q.db.Model(&StorageRoute{}).
		Joins("LEFT JOIN sys_projects p ON p.id = sys_storage_routes.project_id")
	if edgeInstanceID = strings.TrimSpace(edgeInstanceID); edgeInstanceID != "" {
		stmt = stmt.Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if filter.ProjectID != nil {
		stmt = stmt.Where("sys_storage_routes.project_id = ?", *filter.ProjectID)
	}
	if filter.VarID != nil {
		stmt = stmt.Where("sys_storage_routes.var_id = ?", *filter.VarID)
	}
	if filter.Enabled != nil {
		stmt = stmt.Where("sys_storage_routes.enabled = ?", *filter.Enabled)
	}
	var routes []StorageRoute
	err := stmt.Order("sys_storage_routes.project_id asc, sys_storage_routes.var_id asc, sys_storage_routes.id asc").Find(&routes).Error
	return routes, err
}
