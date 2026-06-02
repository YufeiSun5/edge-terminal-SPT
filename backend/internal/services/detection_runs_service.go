package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"

	"gorm.io/gorm"
)

type DetectionRunsService struct {
	repo     *database.Repository
	tasks    *pipeline.TaskManager
	tags     *pipeline.TagManager
	channels *pipeline.Channels
	flows    *pipeline.TaskFlowExecutor
}

type AddNoteInput struct {
	TaskID    uint
	NoteType  string
	Content   string
	ActorType string
	ActorID   string
}

type DetectionRunsRuntimeDeps struct {
	Tags     *pipeline.TagManager
	Channels *pipeline.Channels
	Flows    *pipeline.TaskFlowExecutor
}

func NewDetectionRunsService(repo *database.Repository, tasks *pipeline.TaskManager, deps ...DetectionRunsRuntimeDeps) *DetectionRunsService {
	service := &DetectionRunsService{repo: repo, tasks: tasks}
	if len(deps) > 0 {
		service.tags = deps[0].Tags
		service.channels = deps[0].Channels
		service.flows = deps[0].Flows
	}
	return service
}

func (s *DetectionRunsService) Start(opts database.StartDetectionOptions) (*models.DetectionTask, error) {
	task, err := s.repo.StartDetectionTaskWithOptions(opts)
	if err != nil {
		return nil, err
	}
	s.tasks.SetActive(*task)
	s.recordRunEvent(*task, models.DetectionEventRunStarted, "info", "detection run started")
	s.refreshSummary(task.ID)
	s.enqueueStartSnapshots(*task)
	s.evaluateOnStart(*task)
	s.triggerProjectLifecycle(models.TaskFlowTriggerProjectStart, *task)
	return task, nil
}

func (s *DetectionRunsService) enqueueStartSnapshots(task models.DetectionTask) {
	if s.tags == nil || s.channels == nil {
		return
	}
	active, ok := s.tasks.ActiveForProject(task.ProjectID)
	if !ok {
		return
	}
	now := time.Now()
	for _, tag := range s.tags.ForProject(task.ProjectID) {
		if !active.AllowsStore(tag.Config.VarID) {
			continue
		}
		state := tag.RuntimeState()
		if !state.Initialized {
			continue
		}
		storeTask := tag.StoreTaskForTrigger(tag.Config.GatewayID, tag.Config.SourceTopic, active, now, models.StoreTriggerOnStart, true, true)
		if storeTask == nil {
			continue
		}
		select {
		case s.channels.Store <- storeTask:
			tag.MarkStorageRoutesStored(storeTask.StorageRoutes, now)
		default:
			s.channels.RecordDrop("store")
			log.Printf("detection start snapshot dropped task_id=%d var_id=%d: store queue full", task.ID, tag.Config.VarID)
		}
	}
}

func (s *DetectionRunsService) evaluateOnStart(task models.DetectionTask) {
	if s.tags == nil || s.channels == nil || len(task.StandardItems) == 0 {
		return
	}
	now := time.Now()
	for _, item := range task.StandardItems {
		if !item.CheckOnStart {
			continue
		}
		tag, ok := s.tags.Get(item.VarID)
		if !ok {
			continue
		}
		for _, event := range s.tasks.EvaluateLimitAlarm(tag, now, true) {
			select {
			case s.channels.Alarm <- event:
			default:
				s.channels.RecordDrop("alarm")
				log.Printf("detection start alarm dropped task_id=%d var_id=%d: alarm queue full", task.ID, item.VarID)
			}
		}
	}
}

func (s *DetectionRunsService) Stop(taskID uint, reason string) (*models.DetectionTask, error) {
	return s.StopWithEndType(taskID, reason, models.DetectionEndManualStop)
}

func (s *DetectionRunsService) StopWithEndType(taskID uint, reason string, endType string) (*models.DetectionTask, error) {
	task, err := s.repo.StopDetectionTaskWithEndType(taskID, reason, endType)
	if err != nil {
		return nil, err
	}
	s.tasks.Clear(task.ProjectID)
	s.recordRunEvent(*task, models.DetectionEventRunStopped, "info", "detection run stopped")
	summary, ok := s.refreshSummary(task.ID)
	s.refreshFeatures(task.ID)
	if ok {
		s.publishDetectionResult(*task, summary)
	}
	s.triggerProjectLifecycle(models.TaskFlowTriggerProjectEnd, *task)
	return task, nil
}

func (s *DetectionRunsService) AbnormalStop(taskID uint, reason string) (*models.DetectionTask, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("reason is required")
	}
	task, err := s.repo.StopDetectionTaskWithEndType(taskID, reason, models.DetectionEndAbnormalStop)
	if err != nil {
		return nil, err
	}
	s.tasks.Clear(task.ProjectID)
	s.recordRunEvent(*task, models.DetectionEventRunAbnormalStop, "warning", "detection run abnormal stopped")
	summary, ok := s.refreshSummary(task.ID)
	s.refreshFeatures(task.ID)
	if ok {
		s.publishDetectionResult(*task, summary)
	}
	s.triggerProjectLifecycle(models.TaskFlowTriggerProjectEnd, *task)
	return task, nil
}

func (s *DetectionRunsService) Pause(taskID uint, reason string) (*models.DetectionTask, error) {
	task, err := s.repo.PauseDetectionTask(taskID, reason)
	if err != nil {
		return nil, err
	}
	s.tasks.Clear(task.ProjectID)
	s.recordRunEvent(*task, models.DetectionEventRunPaused, "info", "detection run paused")
	s.refreshSummary(task.ID)
	return task, nil
}

func (s *DetectionRunsService) Resume(taskID uint) (*models.DetectionTask, error) {
	task, err := s.repo.ResumeDetectionTask(taskID)
	if err != nil {
		return nil, err
	}
	s.tasks.SetActive(*task)
	s.recordRunEvent(*task, models.DetectionEventRunResumed, "info", "detection run resumed")
	s.refreshSummary(task.ID)
	return task, nil
}

func (s *DetectionRunsService) List(filter database.DetectionTaskFilter) ([]models.DetectionTask, error) {
	return s.repo.ListDetectionTasks(filter)
}

func (s *DetectionRunsService) Current(projectID uint) (models.DetectionTask, error) {
	return s.repo.GetCurrentDetectionTaskForProject(projectID)
}

func (s *DetectionRunsService) Active() []models.ActiveTask {
	return s.tasks.AllActive()
}

func (s *DetectionRunsService) Get(id uint) (models.DetectionTask, error) {
	return s.repo.GetDetectionTask(id)
}

func (s *DetectionRunsService) StorageRoutes(taskID uint) ([]models.DetectionRunStorageRoute, error) {
	if _, err := s.repo.GetDetectionTask(taskID); err != nil {
		return nil, err
	}
	return s.repo.ListRunStorageRoutes(taskID)
}

func (s *DetectionRunsService) ReportRequests(taskID uint) ([]models.DetectionRunReportRequest, error) {
	if _, err := s.repo.GetDetectionTask(taskID); err != nil {
		return nil, err
	}
	return s.repo.ListDetectionRunReportRequests(taskID)
}

func (s *DetectionRunsService) Summary(taskID uint) (models.DetectionRunSummary, error) {
	return s.repo.RefreshDetectionRunSummary(taskID)
}

func (s *DetectionRunsService) Features(taskID uint) ([]models.DetectionRunFeature, error) {
	return s.repo.RefreshDetectionRunFeatures(taskID)
}

func (s *DetectionRunsService) ListEvents(taskID uint, limit int) ([]models.DetectionRunEvent, error) {
	return s.repo.ListDetectionRunEvents(taskID, limit)
}

func (s *DetectionRunsService) AddNote(input AddNoteInput) (models.DetectionRunNote, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return models.DetectionRunNote{}, fmt.Errorf("content is required")
	}
	noteType := strings.TrimSpace(input.NoteType)
	if noteType == "" {
		noteType = "memo"
	}
	note := models.DetectionRunNote{
		TaskID:    input.TaskID,
		NoteType:  noteType,
		Content:   content,
		ActorType: input.ActorType,
		ActorID:   input.ActorID,
	}
	if err := s.repo.CreateDetectionRunNote(&note); err != nil {
		return models.DetectionRunNote{}, err
	}
	return note, nil
}

func (s *DetectionRunsService) ListNotes(taskID uint, limit int) ([]models.DetectionRunNote, error) {
	return s.repo.ListDetectionRunNotes(taskID, limit)
}

func (s *DetectionRunsService) recordRunEvent(task models.DetectionTask, eventType string, level string, message string) {
	event := &models.DetectionRunEvent{
		TaskID:      task.ID,
		TestNo:      task.TestNo,
		ProjectID:   task.ProjectID,
		ProjectCode: task.ProjectCode,
		EventType:   eventType,
		EventLevel:  level,
		Message:     message,
	}
	if err := s.repo.CreateDetectionRunEvent(event); err != nil {
		log.Printf("create detection run event failed task_id=%d event_type=%s err=%v", task.ID, eventType, err)
		return
	}
	s.publishRunEventNotification(task, eventType, level, message)
}

func (s *DetectionRunsService) refreshSummary(taskID uint) (models.DetectionRunSummary, bool) {
	summary, err := s.repo.RefreshDetectionRunSummary(taskID)
	if err != nil {
		log.Printf("refresh detection run summary failed task_id=%d err=%v", taskID, err)
		return models.DetectionRunSummary{}, false
	}
	return summary, true
}

func (s *DetectionRunsService) refreshFeatures(taskID uint) {
	if _, err := s.repo.RefreshDetectionRunFeatures(taskID); err != nil {
		log.Printf("refresh detection run features failed task_id=%d err=%v", taskID, err)
	}
}

func (s *DetectionRunsService) publishRunEventNotification(task models.DetectionTask, eventType string, level string, message string) {
	if s.channels == nil || s.channels.Notify == nil {
		return
	}
	notificationType := detectionEventNotificationType(eventType)
	if notificationType == "" {
		return
	}
	payload := map[string]any{
		"event_type": eventType,
		"status":     task.Status,
		"end_type":   task.EndType,
	}
	notification := models.RuntimeNotificationFromDetectionTask(notificationType, level, task, message, payload)
	select {
	case s.channels.Notify <- notification:
	default:
		s.channels.RecordDrop("notify")
		log.Printf("detection notification dropped type=%s task_id=%d: notify queue full", notificationType, task.ID)
	}
}

func (s *DetectionRunsService) triggerProjectLifecycle(triggerType string, task models.DetectionTask) {
	if s.flows == nil {
		return
	}
	s.flows.Trigger(pipeline.TaskFlowEvent{
		TriggerType: triggerType,
		ProjectID:   task.ProjectID,
		TriggerValue: map[string]any{
			"task_id":      task.ID,
			"project_id":   task.ProjectID,
			"project_code": task.ProjectCode,
			"test_no":      task.TestNo,
			"status":       task.Status,
			"end_type":     task.EndType,
			"trigger_type": triggerType,
		},
		At: time.Now(),
	})
}

func (s *DetectionRunsService) publishDetectionResult(task models.DetectionTask, summary models.DetectionRunSummary) {
	if s.channels == nil || s.channels.Notify == nil {
		return
	}
	var notificationType string
	var level string
	var message string
	switch summary.ResultStatus {
	case models.DetectionSummaryStatusOK:
		notificationType = models.NotificationDetectionResultOK
		level = models.NotificationLevelSuccess
		message = "detection run result ok"
	case models.DetectionSummaryStatusNG:
		notificationType = models.NotificationDetectionResultNG
		level = models.NotificationLevelWarning
		message = "detection run result ng"
	default:
		return
	}
	notification := models.RuntimeNotificationFromDetectionTask(notificationType, level, task, message, map[string]any{
		"result_status":   summary.ResultStatus,
		"history_rows":    summary.HistoryRows,
		"alarm_total":     summary.AlarmTotal,
		"alarm_active":    summary.AlarmActive,
		"alarm_recovered": summary.AlarmRecovered,
		"duration_ms":     summary.DurationMS,
	})
	select {
	case s.channels.Notify <- notification:
	default:
		s.channels.RecordDrop("notify")
		log.Printf("detection result notification dropped task_id=%d status=%s: notify queue full", task.ID, summary.ResultStatus)
	}
}

func detectionEventNotificationType(eventType string) string {
	switch eventType {
	case models.DetectionEventRunStarted:
		return models.NotificationDetectionRunStarted
	case models.DetectionEventRunStopped:
		return models.NotificationDetectionRunStopped
	case models.DetectionEventRunAbnormalStop:
		return models.NotificationDetectionAbnormalStop
	case models.DetectionEventRunPaused:
		return models.NotificationDetectionRunPaused
	case models.DetectionEventRunResumed:
		return models.NotificationDetectionRunResumed
	case models.DetectionEventFeaturesUpdated:
		return models.NotificationDetectionFeatures
	default:
		return ""
	}
}

func HTTPStatusForError(err error) int {
	if status, ok := VariableWriteErrorStatus(err); ok {
		return status
	}
	if errors.Is(err, database.ErrProjectAlreadyRunning) || errors.Is(err, database.ErrReferenced) {
		return 409
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 404
	}
	return 400
}
