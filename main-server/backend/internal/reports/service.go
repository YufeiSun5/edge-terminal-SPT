package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/query"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const (
	StatusPending = "pending"
	StatusWaiting = "waiting"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"

	DefaultTemplateCode    = "SPINDLE_DEFAULT_REPORT"
	DefaultTemplateName    = "Spindle Default Report"
	DefaultTemplateVersion = 1
	DefaultTemplateFileRef = "templates/default-report-template.xlsx"

	EventEnqueued  = "enqueued"
	EventStarted   = "started"
	EventWaiting   = "waiting"
	EventSucceeded = "succeeded"
	EventFailed    = "failed"
	EventRetried   = "retried"
)

var (
	ErrReportNotRequested  = errors.New("report not requested")
	ErrJobNotRetryable     = errors.New("report job is not retryable")
	ErrArtifactNotReady    = errors.New("report artifact is not ready")
	ErrArtifactUnavailable = errors.New("report artifact is unavailable")
)

type MainReportJob struct {
	ID              uint64     `gorm:"column:id;primaryKey" json:"id"`
	JobKey          string     `gorm:"column:job_key;size:191;uniqueIndex" json:"job_key"`
	EdgeInstanceID  string     `gorm:"column:edge_instance_id;size:64;index" json:"edge_instance_id"`
	TaskID          uint       `gorm:"column:task_id;index" json:"task_id"`
	RequestID       uint64     `gorm:"column:request_id;index" json:"request_id"`
	TestNo          string     `gorm:"column:test_no;size:128" json:"test_no"`
	ProjectID       uint       `gorm:"column:project_id;index" json:"project_id"`
	ProjectCode     string     `gorm:"column:project_code;size:64" json:"project_code"`
	TemplateID      *uint      `gorm:"column:template_id" json:"template_id,omitempty"`
	TemplateCode    string     `gorm:"column:template_code;size:128" json:"template_code"`
	TemplateVersion int        `gorm:"column:template_version" json:"template_version"`
	ReportName      string     `gorm:"column:report_name;size:255" json:"report_name"`
	Status          string     `gorm:"column:status;size:32;index" json:"status"`
	ReadinessStatus string     `gorm:"column:readiness_status;size:32" json:"readiness_status"`
	Attempts        int        `gorm:"column:attempts" json:"attempts"`
	MaxAttempts     int        `gorm:"column:max_attempts" json:"max_attempts"`
	NextRunAt       *time.Time `gorm:"column:next_run_at;index" json:"next_run_at,omitempty"`
	LockedAt        *time.Time `gorm:"column:locked_at" json:"locked_at,omitempty"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt      *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	LastCheckedAt   *time.Time `gorm:"column:last_checked_at" json:"last_checked_at,omitempty"`
	ArtifactRef     string     `gorm:"column:artifact_ref;size:512" json:"artifact_ref"`
	ArtifactName    string     `gorm:"column:artifact_name;size:255" json:"artifact_name"`
	ErrorMessage    string     `gorm:"column:error_message;type:text" json:"error_message"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (MainReportJob) TableName() string { return "main_report_jobs" }

type MainReportJobEvent struct {
	ID        uint64    `gorm:"column:id;primaryKey" json:"id"`
	JobID     uint64    `gorm:"column:job_id;index" json:"job_id"`
	EventType string    `gorm:"column:event_type;size:64;index" json:"event_type"`
	Level     string    `gorm:"column:level;size:32" json:"level"`
	Message   string    `gorm:"column:message;size:512" json:"message"`
	Payload   string    `gorm:"column:payload;type:text" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (MainReportJobEvent) TableName() string { return "main_report_job_events" }

func (event MainReportJobEvent) MarshalJSON() ([]byte, error) {
	type alias MainReportJobEvent
	payload := json.RawMessage(`{}`)
	if strings.TrimSpace(event.Payload) != "" && json.Valid([]byte(event.Payload)) {
		payload = json.RawMessage(event.Payload)
	}
	return json.Marshal(struct {
		alias
		Payload json.RawMessage `json:"payload"`
	}{
		alias:   alias(event),
		Payload: payload,
	})
}

type Options struct {
	ArtifactDir             string
	DefaultTemplateCode     string
	DefaultTemplateVersion  int
	DefaultTemplateFileRef  string
	DefaultTemplateRequired bool
	MaxAttempts             int
	WaitingDelay            time.Duration
	RetryDelay              time.Duration
	WorkerBatchSize         int
}

type Service struct {
	db      *gorm.DB
	query   *query.StationViewQuery
	options Options
}

type EnqueueResult struct {
	Jobs      []MainReportJob                `json:"jobs"`
	Readiness query.ReportReadiness          `json:"readiness"`
	Requests  []query.ReportRequestReadiness `json:"requests"`
}

type JobFilter struct {
	Status         string
	TaskID         *uint
	EdgeInstanceID string
	Limit          int
	Offset         int
}

func NewService(db *gorm.DB, stationQuery *query.StationViewQuery, options Options) *Service {
	if options.ArtifactDir == "" {
		options.ArtifactDir = filepath.Join("data", "reports")
	}
	if options.DefaultTemplateCode == "" {
		options.DefaultTemplateCode = DefaultTemplateCode
	}
	if options.DefaultTemplateVersion <= 0 {
		options.DefaultTemplateVersion = DefaultTemplateVersion
	}
	if options.DefaultTemplateFileRef == "" {
		options.DefaultTemplateFileRef = DefaultTemplateFileRef
	}
	options.DefaultTemplateRequired = true
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	if options.WaitingDelay <= 0 {
		options.WaitingDelay = 30 * time.Second
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = 30 * time.Second
	}
	if options.WorkerBatchSize <= 0 {
		options.WorkerBatchSize = 5
	}
	return &Service{db: db, query: stationQuery, options: options}
}

func (s *Service) EnsureSchema() error {
	if err := s.db.AutoMigrate(&query.ReportTemplate{}, &MainReportJob{}, &MainReportJobEvent{}, &MainReportNotification{}, &MainReportNotificationRecipient{}); err != nil {
		return err
	}
	_, _, err := s.ensureDefaultReportTemplate()
	return err
}

func (s *Service) EnqueueTask(taskID uint, edgeInstanceID string, force bool) (EnqueueResult, error) {
	readiness, err := s.query.ReportReadiness(taskID, edgeInstanceID)
	if err != nil {
		return EnqueueResult{}, err
	}
	if readiness.OverallStatus == query.ReportReadinessNotRequested {
		return EnqueueResult{Readiness: readiness}, ErrReportNotRequested
	}
	requestByID := map[uint64]query.ReportRequestReadiness{}
	for _, request := range readiness.Requests {
		requestByID[request.RequestID] = request
	}
	now := time.Now()
	jobs := make([]MainReportJob, 0, len(readiness.ReportItems))
	for _, request := range readiness.ReportItems {
		job, err := s.enqueueRequest(readiness, request, requestByID[request.ID], edgeInstanceID, force, now)
		if err != nil {
			return EnqueueResult{}, err
		}
		jobs = append(jobs, job)
	}
	return EnqueueResult{Jobs: jobs, Readiness: readiness, Requests: readiness.Requests}, nil
}

func (s *Service) enqueueRequest(readiness query.ReportReadiness, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness, edgeInstanceID string, force bool, now time.Time) (MainReportJob, error) {
	key := jobKey(edgeInstanceID, request.TaskID, request.ID)
	var existing MainReportJob
	if err := s.db.First(&existing, "job_key = ?", key).Error; err == nil {
		if force && existing.Status != StatusRunning {
			updates := map[string]any{
				"status":           initialStatus(requestReadiness.Ready, readiness.OverallStatus),
				"readiness_status": readiness.OverallStatus,
				"next_run_at":      &now,
				"error_message":    "",
				"artifact_ref":     "",
				"artifact_name":    "",
				"finished_at":      nil,
				"locked_at":        nil,
			}
			if err := s.db.Model(&MainReportJob{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return MainReportJob{}, err
			}
			job, err := s.GetJob(existing.ID)
			if err != nil {
				return MainReportJob{}, err
			}
			s.recordEvent(job.ID, EventRetried, "info", "report job was force requeued", map[string]any{
				"status":           job.Status,
				"readiness_status": job.ReadinessStatus,
				"force":            true,
			})
			return job, nil
		}
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return MainReportJob{}, err
	}
	nextRunAt := now
	job := MainReportJob{
		JobKey:          key,
		EdgeInstanceID:  strings.TrimSpace(edgeInstanceID),
		TaskID:          request.TaskID,
		RequestID:       request.ID,
		TestNo:          request.TestNo,
		ProjectID:       request.ProjectID,
		ProjectCode:     request.ProjectCode,
		TemplateID:      request.TemplateID,
		TemplateCode:    request.TemplateCode,
		TemplateVersion: request.TemplateVersion,
		ReportName:      request.ReportName,
		Status:          initialStatus(requestReadiness.Ready, readiness.OverallStatus),
		ReadinessStatus: readiness.OverallStatus,
		MaxAttempts:     s.options.MaxAttempts,
		NextRunAt:       &nextRunAt,
	}
	if err := s.db.Create(&job).Error; err != nil {
		return MainReportJob{}, err
	}
	s.recordEvent(job.ID, EventEnqueued, "info", "report job was enqueued", map[string]any{
		"status":           job.Status,
		"readiness_status": job.ReadinessStatus,
		"task_id":          job.TaskID,
		"request_id":       job.RequestID,
	})
	return job, nil
}

func initialStatus(requestReady bool, overallStatus string) string {
	if overallStatus == query.ReportReadinessReady && requestReady {
		return StatusPending
	}
	return StatusWaiting
}

func (s *Service) GetJob(id uint64) (MainReportJob, error) {
	var job MainReportJob
	if err := s.db.First(&job, "id = ?", id).Error; err != nil {
		return job, err
	}
	return job, nil
}

func (s *Service) ListJobs(filter JobFilter) ([]MainReportJob, int64, int, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	stmt := s.db.Model(&MainReportJob{})
	if strings.TrimSpace(filter.Status) != "" {
		stmt = stmt.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	if filter.TaskID != nil {
		stmt = stmt.Where("task_id = ?", *filter.TaskID)
	}
	if strings.TrimSpace(filter.EdgeInstanceID) != "" {
		stmt = stmt.Where("edge_instance_id = ?", strings.TrimSpace(filter.EdgeInstanceID))
	}
	var total int64
	if err := stmt.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	var jobs []MainReportJob
	if err := stmt.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&jobs).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	return jobs, total, limit, offset, nil
}

func (s *Service) ListEvents(jobID uint64, limit int) ([]MainReportJobEvent, int, error) {
	if _, err := s.GetJob(jobID); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var events []MainReportJobEvent
	if err := s.db.Where("job_id = ?", jobID).Order("id asc").Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, limit, nil
}

func (s *Service) RetryJob(id uint64) (MainReportJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return job, err
	}
	if job.Status == StatusRunning || job.Status == StatusSuccess {
		return job, ErrJobNotRetryable
	}
	now := time.Now()
	if err := s.db.Model(&MainReportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":        StatusPending,
		"next_run_at":   &now,
		"error_message": "",
		"locked_at":     nil,
		"finished_at":   nil,
	}).Error; err != nil {
		return MainReportJob{}, err
	}
	updated, err := s.GetJob(id)
	if err != nil {
		return MainReportJob{}, err
	}
	s.recordEvent(updated.ID, EventRetried, "info", "report job was requeued manually", map[string]any{
		"status":   updated.Status,
		"attempts": updated.Attempts,
	})
	return updated, nil
}

func (s *Service) Artifact(id uint64) (string, string, string, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return "", "", "", err
	}
	if job.Status != StatusSuccess || strings.TrimSpace(job.ArtifactRef) == "" {
		return "", "", "", ErrArtifactNotReady
	}
	baseDir, err := filepath.Abs(s.options.ArtifactDir)
	if err != nil {
		return "", "", "", err
	}
	artifactPath, err := filepath.Abs(job.ArtifactRef)
	if err != nil {
		return "", "", "", err
	}
	if artifactPath != baseDir && !strings.HasPrefix(artifactPath, baseDir+string(os.PathSeparator)) {
		return "", "", "", ErrArtifactUnavailable
	}
	info, err := os.Stat(artifactPath)
	if err != nil || info.IsDir() {
		return "", "", "", ErrArtifactUnavailable
	}
	name := strings.TrimSpace(job.ArtifactName)
	if name == "" {
		name = filepath.Base(artifactPath)
	}
	return artifactPath, name, artifactContentType(name), nil
}

func (s *Service) RunDueOnce(ctx context.Context, limit int) ([]MainReportJob, error) {
	if limit <= 0 {
		limit = s.options.WorkerBatchSize
	}
	now := time.Now()
	var jobs []MainReportJob
	if err := s.db.
		Where("status IN ? AND (next_run_at IS NULL OR next_run_at <= ?)", []string{StatusPending, StatusWaiting, StatusFailed}, now).
		Where("attempts < max_attempts OR status = ?", StatusWaiting).
		Order("next_run_at asc, id asc").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		return nil, err
	}
	processed := make([]MainReportJob, 0, len(jobs))
	for _, job := range jobs {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		current, err := s.ProcessJob(job.ID)
		if err != nil {
			log.Printf("[report-worker] process job failed id=%d err=%v", job.ID, err)
			continue
		}
		processed = append(processed, current)
	}
	return processed, nil
}

func (s *Service) ProcessJob(id uint64) (MainReportJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return job, err
	}
	if job.Status == StatusRunning || job.Status == StatusSuccess {
		return job, nil
	}
	now := time.Now()
	if err := s.db.Model(&MainReportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":     StatusRunning,
		"locked_at":  &now,
		"started_at": &now,
	}).Error; err != nil {
		return MainReportJob{}, err
	}
	s.recordEvent(job.ID, EventStarted, "info", "report job processing started", map[string]any{
		"attempts": job.Attempts,
	})
	readiness, err := s.query.ReportReadiness(job.TaskID, job.EdgeInstanceID)
	if err != nil {
		return s.markFailed(job, "readiness query failed: "+err.Error())
	}
	requestReadiness, request, found := findRequest(readiness, job.RequestID)
	checkedAt := time.Now()
	if !found {
		return s.markFailed(job, "report request is no longer synchronized")
	}
	if readiness.OverallStatus != query.ReportReadinessReady || !requestReadiness.Ready {
		next := checkedAt.Add(s.options.WaitingDelay)
		message := "report data is not synchronized yet"
		if readiness.OverallStatus == query.ReportReadinessNotRequested {
			message = "report request is no longer requested"
		}
		if err := s.db.Model(&MainReportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":           StatusWaiting,
			"readiness_status": readiness.OverallStatus,
			"last_checked_at":  &checkedAt,
			"next_run_at":      &next,
			"locked_at":        nil,
			"error_message":    message,
		}).Error; err != nil {
			return MainReportJob{}, err
		}
		s.recordEvent(job.ID, EventWaiting, "warning", message, map[string]any{
			"readiness_status": readiness.OverallStatus,
			"request_ready":    requestReadiness.Ready,
			"next_run_at":      next.Format(time.RFC3339Nano),
		})
		return s.GetJob(job.ID)
	}
	artifacts, err := s.writeArtifacts(job, readiness, request, requestReadiness)
	if err != nil {
		return s.markFailed(job, "write report artifact failed: "+err.Error())
	}
	finished := time.Now()
	if err := s.db.Model(&MainReportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status":           StatusSuccess,
		"readiness_status": readiness.OverallStatus,
		"last_checked_at":  &checkedAt,
		"finished_at":      &finished,
		"locked_at":        nil,
		"next_run_at":      nil,
		"artifact_ref":     artifacts.ExcelRef,
		"artifact_name":    artifacts.ExcelName,
		"error_message":    "",
	}).Error; err != nil {
		return MainReportJob{}, err
	}
	s.recordEvent(job.ID, EventSucceeded, "success", "report artifact was generated", map[string]any{
		"artifact_ref":     artifacts.ExcelRef,
		"artifact_name":    artifacts.ExcelName,
		"manifest_ref":     artifacts.ManifestRef,
		"manifest_name":    artifacts.ManifestName,
		"readiness_status": readiness.OverallStatus,
	})
	return s.GetJob(job.ID)
}

func (s *Service) markFailed(job MainReportJob, message string) (MainReportJob, error) {
	now := time.Now()
	next := now.Add(s.options.RetryDelay)
	attempts := job.Attempts + 1
	status := StatusFailed
	if attempts < job.MaxAttempts {
		status = StatusPending
	}
	if err := s.db.Model(&MainReportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status":          status,
		"attempts":        attempts,
		"next_run_at":     &next,
		"locked_at":       nil,
		"last_checked_at": &now,
		"error_message":   message,
	}).Error; err != nil {
		return MainReportJob{}, err
	}
	level := "warning"
	if status == StatusFailed {
		level = "error"
	}
	s.recordEvent(job.ID, EventFailed, level, message, map[string]any{
		"status":       status,
		"attempts":     attempts,
		"max_attempts": job.MaxAttempts,
		"next_run_at":  next.Format(time.RFC3339Nano),
	})
	return s.GetJob(job.ID)
}

func (s *Service) recordEvent(jobID uint64, eventType string, level string, message string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}
	event := MainReportJobEvent{
		JobID:     jobID,
		EventType: eventType,
		Level:     level,
		Message:   message,
		Payload:   string(raw),
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&event).Error; err != nil {
		log.Printf("[report-worker] record event failed job_id=%d type=%s err=%v", jobID, eventType, err)
		return
	}
	if err := s.recordNotificationForEvent(event); err != nil {
		log.Printf("[report-worker] record notification failed job_id=%d type=%s event_id=%d err=%v", jobID, eventType, event.ID, err)
	}
}

func findRequest(readiness query.ReportReadiness, requestID uint64) (query.ReportRequestReadiness, query.DetectionRunReportRequest, bool) {
	var requestReadiness query.ReportRequestReadiness
	for _, item := range readiness.Requests {
		if item.RequestID == requestID {
			requestReadiness = item
			break
		}
	}
	for _, request := range readiness.ReportItems {
		if request.ID == requestID {
			return requestReadiness, request, true
		}
	}
	return requestReadiness, query.DetectionRunReportRequest{}, false
}

type artifactSet struct {
	ManifestRef  string
	ManifestName string
	ExcelRef     string
	ExcelName    string
}

func (s *Service) writeArtifacts(job MainReportJob, readiness query.ReportReadiness, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness) (artifactSet, error) {
	qualifiedMetrics, err := s.qualifiedTwoHourMetrics(job, readiness, requestReadiness)
	if err != nil {
		return artifactSet{}, err
	}
	reportPackage, err := buildReportPackage(job, readiness, request, requestReadiness, qualifiedMetrics)
	if err != nil {
		return artifactSet{}, err
	}
	manifestRef, manifestName, err := s.writeManifest(job, readiness, request, requestReadiness, reportPackage)
	if err != nil {
		return artifactSet{}, err
	}
	excelRef, excelName, err := s.writeExcel(job, readiness, request, requestReadiness, reportPackage, manifestRef)
	if err != nil {
		return artifactSet{}, err
	}
	return artifactSet{ManifestRef: manifestRef, ManifestName: manifestName, ExcelRef: excelRef, ExcelName: excelName}, nil
}

func (s *Service) qualifiedTwoHourMetrics(job MainReportJob, readiness query.ReportReadiness, requestReadiness query.ReportRequestReadiness) (map[int64]ReportMetricWindow, error) {
	taskID := readiness.Task.ID
	if taskID == 0 {
		taskID = job.TaskID
	}
	standardByVar := make(map[int64]query.DetectionRunStandardItem, len(readiness.Task.StandardItems))
	for _, item := range readiness.Task.StandardItems {
		standardByVar[item.VarID] = item
	}
	result := make(map[int64]ReportMetricWindow, len(requestReadiness.RequiredVarIDs))
	for _, varIDText := range requestReadiness.RequiredVarIDs {
		varID, err := strconv.ParseInt(strings.TrimSpace(varIDText), 10, 64)
		if err != nil || varID <= 0 {
			continue
		}
		standard := standardByVar[varID]
		if standard.VarID == 0 {
			result[varID] = ReportMetricWindow{Status: "missing_standard", Message: "qualified two-hour window requires detection standard snapshot"}
			continue
		}
		if !standard.CheckEnabled {
			result[varID] = ReportMetricWindow{Status: "disabled", Message: "detection check is disabled for this variable"}
			continue
		}
		filter := query.HistoryFilter{
			TaskID:      &taskID,
			VarID:       &varID,
			ProjectID:   &readiness.Task.ProjectID,
			ProjectCode: readiness.Task.ProjectCode,
			TestNo:      readiness.Task.TestNo,
			Limit:       10000,
		}
		rows, _, err := s.query.QueryHistoryData(filter, job.EdgeInstanceID)
		if err != nil {
			return nil, err
		}
		result[varID] = scanQualifiedTwoHourWindow(rows, standard)
	}
	return result, nil
}

func (s *Service) writeManifest(job MainReportJob, readiness query.ReportReadiness, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness, reportPackage ReportPackage) (string, string, error) {
	if err := os.MkdirAll(s.options.ArtifactDir, 0o755); err != nil {
		return "", "", err
	}
	name := fmt.Sprintf("task-%d-request-%d-manifest.json", job.TaskID, job.RequestID)
	path := filepath.Join(s.options.ArtifactDir, name)
	payload := map[string]any{
		"kind":              "main_server_report_manifest",
		"generated_at":      time.Now().Format(time.RFC3339Nano),
		"edge_instance_id":  job.EdgeInstanceID,
		"task":              readiness.Task,
		"request":           request,
		"request_readiness": requestReadiness,
		"counts":            readiness.Counts,
		"checks":            readiness.Checks,
		"summary":           readiness.Summary,
		"features":          readiness.Features,
		"warnings":          readiness.Warnings,
		"report_package":    reportPackage,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", "", err
	}
	return path, name, nil
}

func (s *Service) writeExcel(job MainReportJob, readiness query.ReportReadiness, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness, reportPackage ReportPackage, manifestRef string) (string, string, error) {
	if err := os.MkdirAll(s.options.ArtifactDir, 0o755); err != nil {
		return "", "", err
	}
	file, templateSource := s.openTemplateWorkbook(request)
	defer func() { _ = file.Close() }()

	if err := writeRunSheet(file, job, readiness, request, templateSource, manifestRef); err != nil {
		return "", "", err
	}
	if err := writeRequestSheet(file, request, requestReadiness); err != nil {
		return "", "", err
	}
	if err := writeChecksSheet(file, readiness); err != nil {
		return "", "", err
	}
	if err := writeFeaturesSheet(file, readiness.Features); err != nil {
		return "", "", err
	}
	if err := writeManifestSheet(file, manifestRef); err != nil {
		return "", "", err
	}
	if err := writeReportPackageSheet(file, reportPackage); err != nil {
		return "", "", err
	}
	if err := applyCellMapping(file, reportPackage); err != nil {
		return "", "", err
	}

	name := fmt.Sprintf("task-%d-request-%d-%s.xlsx", job.TaskID, job.RequestID, safeArtifactName(firstNonEmpty(job.ReportName, request.ReportName, request.TemplateCode, "report")))
	path := filepath.Join(s.options.ArtifactDir, name)
	if err := file.SaveAs(path); err != nil {
		return "", "", err
	}
	return path, name, nil
}

func (s *Service) openTemplateWorkbook(request query.DetectionRunReportRequest) (*excelize.File, string) {
	template, ok := s.findReportTemplate(request)
	if !ok {
		defaultTemplate, source, err := s.ensureDefaultReportTemplate()
		if err == nil {
			template = defaultTemplate
			ok = true
			if strings.TrimSpace(source) != "" {
				log.Printf("[report-worker] report template missing code=%s request_id=%d using default template source=%s", request.TemplateCode, request.ID, source)
			}
		} else {
			log.Printf("[report-worker] ensure default report template failed request_id=%d err=%v", request.ID, err)
		}
	}
	if ok {
		if path, ok := s.resolveTemplatePath(template.FileRef); ok {
			file, err := excelize.OpenFile(path)
			if err == nil {
				return file, path
			}
			log.Printf("[report-worker] open report template failed file_ref=%s path=%s err=%v", template.FileRef, path, err)
		}
	}
	return excelize.NewFile(), "generated_default_missing_template"
}

func (s *Service) ensureDefaultReportTemplate() (query.ReportTemplate, string, error) {
	fileRef := strings.TrimSpace(s.options.DefaultTemplateFileRef)
	if fileRef == "" {
		fileRef = DefaultTemplateFileRef
	}
	if _, ok := s.resolveTemplatePath(fileRef); !ok {
		path := filepath.Join(s.options.ArtifactDir, fileRef)
		if filepath.IsAbs(fileRef) {
			path = fileRef
		}
		if err := writeDefaultReportTemplateWorkbook(path); err != nil {
			return query.ReportTemplate{}, "", err
		}
	}
	code := strings.TrimSpace(s.options.DefaultTemplateCode)
	if code == "" {
		code = DefaultTemplateCode
	}
	version := s.options.DefaultTemplateVersion
	if version <= 0 {
		version = DefaultTemplateVersion
	}
	var template query.ReportTemplate
	if err := s.db.First(&template, "template_code = ?", code).Error; err == nil {
		updates := map[string]any{
			"file_ref":           fileRef,
			"file_kind":          "xlsx",
			"version":            version,
			"enabled":            true,
			"params_schema_json": defaultTemplateParamsSchema(),
			"remark":             "system default report template for main-server report worker",
			"updated_at":         time.Now(),
		}
		if strings.TrimSpace(template.Name) == "" {
			updates["name"] = DefaultTemplateName
		}
		if strings.TrimSpace(template.DisplayName) == "" {
			updates["display_name"] = "默认检测报表模板"
		}
		if err := s.db.Model(&query.ReportTemplate{}).Where("id = ?", template.ID).Updates(updates).Error; err != nil {
			return query.ReportTemplate{}, "", err
		}
		if err := s.db.First(&template, "id = ?", template.ID).Error; err != nil {
			return query.ReportTemplate{}, "", err
		}
		return template, "", nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return query.ReportTemplate{}, "", err
	}
	now := time.Now()
	template = query.ReportTemplate{
		TemplateCode:     code,
		Name:             DefaultTemplateName,
		DisplayName:      "默认检测报表模板",
		FileRef:          fileRef,
		FileKind:         "xlsx",
		Version:          version,
		ParamsSchemaJSON: defaultTemplateParamsSchema(),
		Enabled:          true,
		Remark:           "system default report template for main-server report worker",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.db.Create(&template).Error; err != nil {
		return query.ReportTemplate{}, "", err
	}
	return template, "created", nil
}

func writeDefaultReportTemplateWorkbook(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	sheet := "Default_Report"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	rows := [][]any{
		{"Spindle Default Report Template"},
		{"field", "value"},
		{"test_no", ""},
		{"project_code", ""},
		{"edge_instance_id", ""},
		{"started_at", ""},
		{"ended_at", ""},
		{"full_detection_avg", ""},
		{"qualified_two_hours_avg", ""},
		{"qualified_two_hours_status", ""},
		{"limit_l", ""},
		{"limit_h", ""},
		{"unit", ""},
		{"operator_note", ""},
	}
	if err := writeRows(file, sheet, rows); err != nil {
		return err
	}
	return file.SaveAs(path)
}

func defaultTemplateParamsSchema() string {
	return `{"cell_mapping":{"version":1,"sheet":"Default_Report","items":[{"cell":"B3","source":"task.test_no"},{"cell":"B4","source":"task.project_code"},{"cell":"B5","source":"task.edge_instance_id"},{"cell":"B6","source":"task.started_at"},{"cell":"B7","source":"task.ended_at"},{"cell":"B8","source":"metric.avg"},{"cell":"B9","source":"metric.qualified_two_hours.avg_value"},{"cell":"B10","source":"metric.qualified_two_hours.status"},{"cell":"B11","source":"limit.limit_l"},{"cell":"B12","source":"limit.limit_h"},{"cell":"B13","source":"variable.unit"},{"cell":"B14","source":"param.operator_note"}]}}`
}

func (s *Service) resolveTemplatePath(fileRef string) (string, bool) {
	fileRef = strings.TrimSpace(fileRef)
	if fileRef == "" {
		return "", false
	}
	candidates := []string{fileRef}
	if !filepath.IsAbs(fileRef) {
		candidates = append(candidates,
			filepath.Join(s.options.ArtifactDir, fileRef),
			filepath.Join(filepath.Dir(s.options.ArtifactDir), fileRef),
		)
	}
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
			return cleaned, true
		}
	}
	return "", false
}

func (s *Service) findReportTemplate(request query.DetectionRunReportRequest) (query.ReportTemplate, bool) {
	var template query.ReportTemplate
	if request.TemplateID != nil && *request.TemplateID > 0 {
		if err := s.db.First(&template, "id = ? AND enabled = ?", *request.TemplateID, true).Error; err == nil {
			return template, true
		}
	}
	if strings.TrimSpace(request.TemplateCode) != "" {
		if err := s.db.First(&template, "template_code = ? AND enabled = ?", strings.TrimSpace(request.TemplateCode), true).Error; err == nil {
			return template, true
		}
	}
	return query.ReportTemplate{}, false
}

func writeRunSheet(file *excelize.File, job MainReportJob, readiness query.ReportReadiness, request query.DetectionRunReportRequest, templateSource string, manifestRef string) error {
	sheet := "Report_Run"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	rows := [][]any{
		{"field", "value"},
		{"generated_at", time.Now().Format(time.RFC3339Nano)},
		{"edge_instance_id", job.EdgeInstanceID},
		{"task_id", job.TaskID},
		{"request_id", job.RequestID},
		{"test_no", job.TestNo},
		{"project_id", job.ProjectID},
		{"project_code", job.ProjectCode},
		{"template_id", request.TemplateID},
		{"template_code", request.TemplateCode},
		{"template_version", request.TemplateVersion},
		{"report_name", firstNonEmpty(job.ReportName, request.ReportName)},
		{"readiness_status", readiness.OverallStatus},
		{"template_source", templateSource},
		{"manifest_ref", manifestRef},
	}
	if readiness.Summary != nil {
		rows = append(rows,
			[]any{"result_status", readiness.Summary.ResultStatus},
			[]any{"duration_ms", readiness.Summary.DurationMS},
			[]any{"history_rows", readiness.Summary.HistoryRows},
			[]any{"alarm_total", readiness.Summary.AlarmTotal},
			[]any{"alarm_active", readiness.Summary.AlarmActive},
			[]any{"alarm_recovered", readiness.Summary.AlarmRecovered},
		)
	}
	return writeRows(file, sheet, rows)
}

func writeRequestSheet(file *excelize.File, request query.DetectionRunReportRequest, requestReadiness query.ReportRequestReadiness) error {
	sheet := "Report_Request"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	rows := [][]any{
		{"field", "value"},
		{"request_id", request.ID},
		{"status", request.Status},
		{"var_id", request.VarID},
		{"var_name", request.VarName},
		{"display_name", request.DisplayName},
		{"display_name_en", request.DisplayNameEN},
		{"display_name_ja", request.DisplayNameJA},
		{"required_var_ids", strings.Join(requestReadiness.RequiredVarIDs, ",")},
		{"history_rows", requestReadiness.HistoryRows},
		{"alarm_rows", requestReadiness.AlarmRows},
		{"ready", requestReadiness.Ready},
		{"variables_json", prettyJSON(request.VariablesJSON, "[]")},
		{"params_json", prettyJSON(request.ParamsJSON, "{}")},
		{"ext_1", request.Ext1},
		{"ext_2", request.Ext2},
		{"ext_3", request.Ext3},
	}
	return writeRows(file, sheet, rows)
}

func writeChecksSheet(file *excelize.File, readiness query.ReportReadiness) error {
	sheet := "Readiness_Checks"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	rows := [][]any{{"name", "status", "count", "message"}}
	for _, check := range readiness.Checks {
		rows = append(rows, []any{check.Name, check.Status, check.Count, check.Message})
	}
	if len(readiness.Warnings) > 0 {
		rows = append(rows, []any{})
		rows = append(rows, []any{"warnings"})
		for _, warning := range readiness.Warnings {
			rows = append(rows, []any{warning})
		}
	}
	return writeRows(file, sheet, rows)
}

func writeFeaturesSheet(file *excelize.File, features []query.DetectionRunFeature) error {
	sheet := "Features"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	rows := [][]any{{"var_id", "var_name", "sample_count", "avg_value", "min_value", "max_value", "first_sample_time", "last_sample_time"}}
	for _, feature := range features {
		rows = append(rows, []any{
			strconvFormatInt(feature.VarID),
			feature.VarName,
			feature.SampleCount,
			floatValue(feature.AvgValue),
			floatValue(feature.MinValue),
			floatValue(feature.MaxValue),
			feature.FirstSampleTime.Format(time.RFC3339Nano),
			feature.LastSampleTime.Format(time.RFC3339Nano),
		})
	}
	return writeRows(file, sheet, rows)
}

func writeManifestSheet(file *excelize.File, manifestRef string) error {
	sheet := "Manifest_JSON"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	raw, err := os.ReadFile(manifestRef)
	if err != nil {
		return err
	}
	return writeRows(file, sheet, [][]any{{"manifest_json"}, {string(raw)}})
}

func writeReportPackageSheet(file *excelize.File, reportPackage ReportPackage) error {
	sheet := "Report_Package"
	if err := ensureSheet(file, sheet); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(reportPackage, "", "  ")
	if err != nil {
		return err
	}
	rows := [][]any{{"report_package_json"}, {string(raw)}}
	if len(reportPackage.Reports) > 0 {
		rows = append(rows, []any{}, []any{"request_id", "var_id", "var_name", "avg_value", "min_value", "max_value", "sample_count", "limit_l", "limit_h", "limit_source"})
		for _, variable := range reportPackage.Reports[0].Variables {
			rows = append(rows, []any{
				reportPackage.Reports[0].RequestID,
				variable.VarIDText,
				variable.VarName,
				floatValue(variable.Metrics.FullDetection.AvgValue),
				floatValue(variable.Metrics.FullDetection.MinValue),
				floatValue(variable.Metrics.FullDetection.MaxValue),
				variable.Metrics.FullDetection.SampleCount,
				floatValue(variable.Limits.LimitL),
				floatValue(variable.Limits.LimitH),
				variable.Limits.Source,
			})
		}
	}
	return writeRows(file, sheet, rows)
}

func scanQualifiedTwoHourWindow(rows []query.HistoryData, standard query.DetectionRunStandardItem) ReportMetricWindow {
	if len(rows) == 0 {
		return ReportMetricWindow{Status: "missing_history", Message: "no synchronized history rows for qualified two-hour window"}
	}
	var current qualifiedWindowAccumulator
	var best qualifiedWindowAccumulator
	for _, row := range rows {
		if !historyRowQualified(row, standard) {
			current = qualifiedWindowAccumulator{}
			continue
		}
		value := 0.0
		if row.Value != nil {
			value = *row.Value
		}
		current.add(row.SourceTime, value)
		if current.duration() >= 2*time.Hour && current.SampleCount > best.SampleCount {
			best = current
		}
	}
	if best.SampleCount == 0 {
		if current.SampleCount > 0 {
			return current.toMetric("insufficient", "qualified samples exist but the continuous window is shorter than two hours")
		}
		return ReportMetricWindow{Status: "insufficient", Message: "no continuous qualified samples were found"}
	}
	return best.toMetric("available", "continuous qualified window is at least two hours")
}

type qualifiedWindowAccumulator struct {
	SampleCount int64
	Sum         float64
	Min         float64
	Max         float64
	First       time.Time
	Last        time.Time
}

func (w *qualifiedWindowAccumulator) add(sampleTime time.Time, value float64) {
	if w.SampleCount == 0 {
		w.Min = value
		w.Max = value
		w.First = sampleTime
	} else {
		if value < w.Min {
			w.Min = value
		}
		if value > w.Max {
			w.Max = value
		}
	}
	w.Last = sampleTime
	w.SampleCount++
	w.Sum += value
}

func (w qualifiedWindowAccumulator) duration() time.Duration {
	if w.SampleCount <= 0 {
		return 0
	}
	return w.Last.Sub(w.First)
}

func (w qualifiedWindowAccumulator) toMetric(status string, message string) ReportMetricWindow {
	if w.SampleCount == 0 {
		return ReportMetricWindow{Status: status, Message: message}
	}
	avg := w.Sum / float64(w.SampleCount)
	minValue := w.Min
	maxValue := w.Max
	first := w.First
	last := w.Last
	return ReportMetricWindow{
		Status:          status,
		SampleCount:     w.SampleCount,
		AvgValue:        &avg,
		MinValue:        &minValue,
		MaxValue:        &maxValue,
		FirstSampleTime: &first,
		LastSampleTime:  &last,
		Message:         message,
	}
}

func historyRowQualified(row query.HistoryData, standard query.DetectionRunStandardItem) bool {
	if row.Value == nil {
		return false
	}
	if !qualityAllowed(row.Quality, standard.QualityPolicy) {
		return false
	}
	value := *row.Value
	if standard.LimitLL != nil && value < *standard.LimitLL {
		return false
	}
	if standard.LimitL != nil && value < *standard.LimitL {
		return false
	}
	if standard.LimitH != nil && value > *standard.LimitH {
		return false
	}
	if standard.LimitHH != nil && value > *standard.LimitHH {
		return false
	}
	if standard.LimitLL == nil && standard.LimitL == nil && standard.LimitH == nil && standard.LimitHH == nil {
		return false
	}
	return true
}

func qualityAllowed(quality int, policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "good_only", "good":
		return quality > 0
	case "ignore", "ignore_quality", "any":
		return true
	default:
		return quality > 0
	}
}

func ensureSheet(file *excelize.File, sheet string) error {
	index, err := file.GetSheetIndex(sheet)
	if err != nil {
		return err
	}
	if index >= 0 {
		return nil
	}
	_, err = file.NewSheet(sheet)
	return err
}

func writeRows(file *excelize.File, sheet string, rows [][]any) error {
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				return err
			}
			if err := file.SetCellValue(sheet, cell, excelValue(value)); err != nil {
				return err
			}
		}
	}
	return nil
}

func excelValue(value any) any {
	switch typed := value.(type) {
	case *uint:
		if typed == nil {
			return ""
		}
		return *typed
	case *float64:
		return floatValue(typed)
	default:
		return typed
	}
}

func prettyJSON(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return raw
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return raw
	}
	return string(pretty)
}

func floatValue(value *float64) any {
	if value == nil {
		return ""
	}
	return *value
}

func safeArtifactName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "report"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	safe := strings.Trim(builder.String(), "_")
	if safe == "" {
		return "report"
	}
	if len(safe) > 80 {
		return safe[:80]
	}
	return safe
}

func strconvFormatInt(value int64) string {
	return fmt.Sprintf("%d", value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func artifactContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return "application/json"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".xls":
		return "application/vnd.ms-excel"
	default:
		return "application/octet-stream"
	}
}

func jobKey(edgeInstanceID string, taskID uint, requestID uint64) string {
	return fmt.Sprintf("%s:%d:%d", strings.TrimSpace(edgeInstanceID), taskID, requestID)
}

func StartWorker(ctx context.Context, service *Service, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := service.RunDueOnce(ctx, service.options.WorkerBatchSize); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("[report-worker] run failed err=%v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
