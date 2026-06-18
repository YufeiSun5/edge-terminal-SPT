package reports

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/query"
)

const downloadPackageContentType = "application/zip"

type DownloadPackageInput struct {
	TaskID         uint     `json:"task_id"`
	EdgeInstanceID string   `json:"edge_instance_id,omitempty"`
	Keys           []string `json:"keys,omitempty"`
	Operator       string   `json:"operator,omitempty"`
}

type DownloadPackageResult struct {
	Name        string
	ContentType string
	Data        []byte
}

type downloadManifest struct {
	Kind          string                  `json:"kind"`
	Version       int                     `json:"version"`
	TaskID        uint                    `json:"task_id"`
	TestNo        string                  `json:"test_no,omitempty"`
	GeneratedAt   string                  `json:"generated_at"`
	Operator      string                  `json:"operator,omitempty"`
	RequestedKeys []string                `json:"requested_keys"`
	Included      []downloadManifestEntry `json:"included"`
	Skipped       []downloadManifestEntry `json:"skipped,omitempty"`
}

type downloadManifestEntry struct {
	Key    string `json:"key"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (s *Service) BuildDownloadPackage(ctx context.Context, input DownloadPackageInput) (DownloadPackageResult, error) {
	if input.TaskID == 0 {
		return DownloadPackageResult{}, fmt.Errorf("task_id is required")
	}
	task, err := s.query.GetDetectionRun(input.TaskID)
	if err != nil {
		return DownloadPackageResult{}, err
	}
	if strings.TrimSpace(input.EdgeInstanceID) != "" {
		if _, err := s.query.DetectionRunEdgeInstanceID(input.TaskID, input.EdgeInstanceID); err != nil {
			return DownloadPackageResult{}, err
		}
	}
	keys := normalizeDownloadKeys(input.Keys)
	manifest := downloadManifest{
		Kind:          "main_server_history_download_package",
		Version:       1,
		TaskID:        input.TaskID,
		TestNo:        task.TestNo,
		GeneratedAt:   time.Now().Format(time.RFC3339Nano),
		Operator:      strings.TrimSpace(input.Operator),
		RequestedKeys: keys,
		Included:      []downloadManifestEntry{},
		Skipped:       []downloadManifestEntry{},
	}
	buffer := &bytes.Buffer{}
	zipWriter := zip.NewWriter(buffer)

	if err := addJSONToZip(zipWriter, &manifest, "task", "snapshots/task.json", task); err != nil {
		_ = zipWriter.Close()
		return DownloadPackageResult{}, err
	}

	if hasAnyKey(keys, "config-standard", "config-limits") {
		if hasKey(keys, "config-standard") {
			if err := addJSONToZip(zipWriter, &manifest, "config-standard", "snapshots/standard-items.json", task.StandardItems); err != nil {
				_ = zipWriter.Close()
				return DownloadPackageResult{}, err
			}
		}
		if hasKey(keys, "config-limits") {
			if err := addJSONToZip(zipWriter, &manifest, "config-limits", "snapshots/limit-snapshot.json", buildDownloadLimitSnapshot(task.StandardItems)); err != nil {
				_ = zipWriter.Close()
				return DownloadPackageResult{}, err
			}
		}
	}
	if hasKey(keys, "config-routes") {
		routes, err := s.query.DetectionRunStorageRoutes(input.TaskID)
		if err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
		if err := addJSONToZip(zipWriter, &manifest, "config-routes", "snapshots/storage-routes.json", routes); err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
	}
	if hasKey(keys, "config-reports") {
		requests, err := s.query.DetectionRunReportRequests(input.TaskID)
		if err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
		if err := addJSONToZip(zipWriter, &manifest, "config-reports", "snapshots/report-requests.json", requests); err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
	}
	if hasKey(keys, "alarms-events") {
		events, _, err := s.query.DetectionRunEvents(input.TaskID, 1000)
		if err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
		if err := addJSONToZip(zipWriter, &manifest, "alarms-events", "events/task-events.json", events); err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
	}
	if hasKey(keys, "alarms-records") {
		alarms, _, _, _, err := s.query.ListLimitAlarms(query.LimitAlarmFilter{Scope: query.AlarmScopeDetection, TaskID: &input.TaskID, Limit: 1000}, input.EdgeInstanceID)
		if err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
		if err := addJSONToZip(zipWriter, &manifest, "alarms-records", "events/alarm-records.json", alarms); err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
	}
	if hasAnyKey(keys, "data-raw", "data-filtered", "data-table") {
		rows, _, err := s.query.QueryHistoryData(query.HistoryFilter{TaskID: &input.TaskID, Limit: 10000}, input.EdgeInstanceID)
		if err != nil {
			_ = zipWriter.Close()
			return DownloadPackageResult{}, err
		}
		if hasKey(keys, "data-raw") {
			if err := addHistoryCSVToZip(zipWriter, &manifest, "data-raw", "data/history-raw.csv", rows); err != nil {
				_ = zipWriter.Close()
				return DownloadPackageResult{}, err
			}
		}
		if hasKey(keys, "data-filtered") {
			if err := addHistoryCSVToZip(zipWriter, &manifest, "data-filtered", "data/history-filtered.csv", rows); err != nil {
				_ = zipWriter.Close()
				return DownloadPackageResult{}, err
			}
		}
		if hasKey(keys, "data-table") {
			if err := addHistoryCSVToZip(zipWriter, &manifest, "data-table", "data/history-table.csv", rows); err != nil {
				_ = zipWriter.Close()
				return DownloadPackageResult{}, err
			}
		}
	}
	if hasKey(keys, "data-chart") {
		manifest.Skipped = append(manifest.Skipped, downloadManifestEntry{
			Key:    "data-chart",
			Reason: "standalone chart png export is not available yet; chart images are embedded in generated report xlsx artifacts",
		})
	}
	if err := s.addSelectedReportArtifacts(ctx, zipWriter, &manifest, keys); err != nil {
		_ = zipWriter.Close()
		return DownloadPackageResult{}, err
	}
	if err := addJSONToZip(zipWriter, &manifest, "download-manifest", "download-manifest.json", manifest); err != nil {
		_ = zipWriter.Close()
		return DownloadPackageResult{}, err
	}
	if err := zipWriter.Close(); err != nil {
		return DownloadPackageResult{}, err
	}
	name := fmt.Sprintf("task-%d-download-package.zip", input.TaskID)
	if strings.TrimSpace(task.TestNo) != "" {
		name = fmt.Sprintf("task-%d-%s-download-package.zip", input.TaskID, safePackageName(task.TestNo))
	}
	return DownloadPackageResult{Name: name, ContentType: downloadPackageContentType, Data: buffer.Bytes()}, nil
}

func (s *Service) addSelectedReportArtifacts(ctx context.Context, zipWriter *zip.Writer, manifest *downloadManifest, keys []string) error {
	reportJobIDs := selectedReportJobIDs(keys)
	if len(reportJobIDs) == 0 {
		return nil
	}
	for _, jobID := range reportJobIDs {
		job, err := s.GetJob(jobID)
		if err != nil {
			return err
		}
		path, name, _, err := s.Artifact(jobID)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return ErrArtifactUnavailable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		target := "reports/" + safePackageName(firstNonEmpty(name, job.ArtifactName, fmt.Sprintf("report-job-%d.xlsx", job.ID)))
		if err := addBytesToZip(zipWriter, target, raw); err != nil {
			return err
		}
		manifest.Included = append(manifest.Included, downloadManifestEntry{Key: fmt.Sprintf("report-job-%d", job.ID), Path: target})
		events, _, err := s.ListEvents(job.ID, 200)
		if err != nil {
			return err
		}
		if err := addJSONToZip(zipWriter, manifest, fmt.Sprintf("report-job-%d-events", job.ID), fmt.Sprintf("reports/report-job-%d-events.json", job.ID), events); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDownloadKeys(keys []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || key == "reports-empty" || seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return []string{"data-raw", "data-filtered", "data-chart", "data-table", "alarms-records", "alarms-events", "config-standard", "config-limits", "config-reports", "config-routes"}
	}
	return normalized
}

func selectedReportJobIDs(keys []string) []uint64 {
	ids := make([]uint64, 0)
	for _, key := range keys {
		if !strings.HasPrefix(key, "report-job-") {
			continue
		}
		id, err := strconv.ParseUint(strings.TrimPrefix(key, "report-job-"), 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func hasKey(keys []string, target string) bool {
	for _, key := range keys {
		if key == target {
			return true
		}
	}
	return false
}

func hasAnyKey(keys []string, targets ...string) bool {
	for _, target := range targets {
		if hasKey(keys, target) {
			return true
		}
	}
	return false
}

func addJSONToZip(zipWriter *zip.Writer, manifest *downloadManifest, key string, path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := addBytesToZip(zipWriter, path, raw); err != nil {
		return err
	}
	if manifest != nil && key != "download-manifest" {
		manifest.Included = append(manifest.Included, downloadManifestEntry{Key: key, Path: path})
	}
	return nil
}

func addHistoryCSVToZip(zipWriter *zip.Writer, manifest *downloadManifest, key string, path string, rows []query.HistoryData) error {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"id", "task_id", "test_no", "project_id", "project_code", "var_id", "var_name", "value", "str_value", "quality", "source_time"}); err != nil {
		return err
	}
	for _, row := range rows {
		value := ""
		if row.Value != nil {
			value = strconv.FormatFloat(*row.Value, 'f', -1, 64)
		}
		strValue := ""
		if row.StrValue != nil {
			strValue = *row.StrValue
		}
		if err := writer.Write([]string{
			strconv.FormatUint(row.ID, 10),
			strconv.FormatUint(uint64(row.TaskID), 10),
			row.TestNo,
			strconv.FormatUint(uint64(row.ProjectID), 10),
			row.ProjectCode,
			strconv.FormatInt(row.VarID, 10),
			row.VarName,
			value,
			strValue,
			strconv.Itoa(row.Quality),
			row.SourceTime.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	if err := addBytesToZip(zipWriter, path, buffer.Bytes()); err != nil {
		return err
	}
	manifest.Included = append(manifest.Included, downloadManifestEntry{Key: key, Path: path})
	return nil
}

func addBytesToZip(zipWriter *zip.Writer, path string, raw []byte) error {
	header := &zip.FileHeader{
		Name:   filepath.ToSlash(path),
		Method: zip.Deflate,
	}
	header.SetModTime(time.Now())
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(raw)
	return err
}

func buildDownloadLimitSnapshot(items []query.DetectionRunStandardItem) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"var_id":            item.VarID,
			"var_id_text":       strconv.FormatInt(item.VarID, 10),
			"var_name":          item.VarName,
			"display_name":      item.DisplayName,
			"unit":              item.Unit,
			"check_enabled":     item.CheckEnabled,
			"alarm_enabled":     item.AlarmEnabled,
			"limit_ll":          item.LimitLL,
			"limit_l":           item.LimitL,
			"limit_h":           item.LimitH,
			"limit_hh":          item.LimitHH,
			"limit_deadband":    item.LimitDeadband,
			"violation_hold_ms": item.ViolationHoldMS,
			"recover_hold_ms":   item.RecoverHoldMS,
			"quality_policy":    item.QualityPolicy,
		})
	}
	return result
}

func safePackageName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "download"
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = replacer.Replace(value)
	value = strings.Join(strings.Fields(value), "_")
	if value == "" {
		return "download"
	}
	return value
}
