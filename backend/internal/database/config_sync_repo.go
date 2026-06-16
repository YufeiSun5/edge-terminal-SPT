package database

import (
	"strings"
	"time"
)

type ConfigSyncWatermark struct {
	StationViewUpdatedAt          time.Time
	StationViewVersionTotal       int64
	DetectionStandardUpdatedAt    time.Time
	DetectionStandardVersionTotal int64
	TaskFlowUpdatedAt             time.Time
	TaskFlowVersionTotal          int64
}

func (r *Repository) ConfigSyncWatermark(edgeInstanceID string) (ConfigSyncWatermark, error) {
	var watermark ConfigSyncWatermark
	if err := r.db.Table("sys_station_view_templates").
		Select("COALESCE(MAX(updated_at), '1970-01-01 00:00:00') AS station_view_updated_at, COALESCE(SUM(version), 0) AS station_view_version_total").
		Scan(&watermark).Error; err != nil {
		return watermark, err
	}
	detection := r.db.Table("sys_detection_standards").
		Select("COALESCE(MAX(sys_detection_standards.updated_at), '1970-01-01 00:00:00') AS detection_standard_updated_at, COALESCE(SUM(sys_detection_standards.version), 0) AS detection_standard_version_total").
		Joins("LEFT JOIN sys_projects p ON p.id = sys_detection_standards.project_id")
	if edgeInstanceID = strings.TrimSpace(edgeInstanceID); edgeInstanceID != "" {
		detection = detection.Where("(sys_detection_standards.project_id IS NULL OR p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if err := detection.Scan(&watermark).Error; err != nil {
		return watermark, err
	}
	taskFlows := r.db.Table("sys_task_flows").
		Select("COALESCE(MAX(sys_task_flows.updated_at), '1970-01-01 00:00:00') AS task_flow_updated_at, COALESCE(SUM(sys_task_flows.version), 0) AS task_flow_version_total").
		Joins("LEFT JOIN sys_projects p ON p.id = sys_task_flows.project_id")
	if edgeInstanceID != "" {
		taskFlows = taskFlows.Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if err := taskFlows.Scan(&watermark).Error; err != nil {
		return watermark, err
	}
	return watermark, nil
}
