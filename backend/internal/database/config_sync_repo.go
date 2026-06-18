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
	var stationView struct {
		StationViewUpdatedAt    string
		StationViewVersionTotal int64
	}
	if err := r.db.Table("sys_station_view_templates").
		Select("DATE_FORMAT(COALESCE(MAX(updated_at), '1970-01-01 00:00:00'), '%Y-%m-%d %H:%i:%s.%f') AS station_view_updated_at, COALESCE(SUM(version), 0) AS station_view_version_total").
		Scan(&stationView).Error; err != nil {
		return watermark, err
	}
	watermark.StationViewUpdatedAt = parseMySQLDateTime(stationView.StationViewUpdatedAt)
	watermark.StationViewVersionTotal = stationView.StationViewVersionTotal

	var detectionWatermark struct {
		DetectionStandardUpdatedAt    string
		DetectionStandardVersionTotal int64
	}
	detection := r.db.Table("sys_detection_standards").
		Select("DATE_FORMAT(COALESCE(MAX(sys_detection_standards.updated_at), '1970-01-01 00:00:00'), '%Y-%m-%d %H:%i:%s.%f') AS detection_standard_updated_at, COALESCE(SUM(sys_detection_standards.version), 0) AS detection_standard_version_total").
		Joins("LEFT JOIN sys_projects p ON p.id = sys_detection_standards.project_id")
	if edgeInstanceID = strings.TrimSpace(edgeInstanceID); edgeInstanceID != "" {
		detection = detection.Where("(sys_detection_standards.project_id IS NULL OR p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if err := detection.Scan(&detectionWatermark).Error; err != nil {
		return watermark, err
	}
	watermark.DetectionStandardUpdatedAt = parseMySQLDateTime(detectionWatermark.DetectionStandardUpdatedAt)
	watermark.DetectionStandardVersionTotal = detectionWatermark.DetectionStandardVersionTotal

	var taskFlowWatermark struct {
		TaskFlowUpdatedAt    string
		TaskFlowVersionTotal int64
	}
	taskFlows := r.db.Table("sys_task_flows").
		Select("DATE_FORMAT(COALESCE(MAX(sys_task_flows.updated_at), '1970-01-01 00:00:00'), '%Y-%m-%d %H:%i:%s.%f') AS task_flow_updated_at, COALESCE(SUM(sys_task_flows.version), 0) AS task_flow_version_total").
		Joins("LEFT JOIN sys_projects p ON p.id = sys_task_flows.project_id")
	if edgeInstanceID != "" {
		taskFlows = taskFlows.Where("(p.edge_instance_id = ? OR p.edge_instance_id = '' OR p.edge_instance_id IS NULL)", edgeInstanceID)
	}
	if err := taskFlows.Scan(&taskFlowWatermark).Error; err != nil {
		return watermark, err
	}
	watermark.TaskFlowUpdatedAt = parseMySQLDateTime(taskFlowWatermark.TaskFlowUpdatedAt)
	watermark.TaskFlowVersionTotal = taskFlowWatermark.TaskFlowVersionTotal
	return watermark, nil
}

func parseMySQLDateTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
