package database

import (
	"errors"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

type HistoryFilter struct {
	ProjectID   *uint
	TaskID      *uint
	ProjectCode string
	TestNo      string
	Start       *time.Time
	End         *time.Time
	Limit       int
}

type DetectionStandardFilter struct {
	ProjectID   *uint
	ProjectCode string
	Mode        string
	Enabled     *bool
	Keyword     string
}

type DetectionTaskFilter struct {
	ProjectID *uint
	Status    string
	TestNo    string
	Start     *time.Time
	End       *time.Time
	Limit     int
}

type LimitAlarmFilter struct {
	Scope      string
	ProjectID  *uint
	TaskID     *uint
	TestNo     string
	VarID      *int64
	Status     string
	AlarmType  string
	AlarmLevel string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type StorageRouteFilter struct {
	ProjectID *uint
	VarID     *int64
	Enabled   *bool
}

type StartDetectionOptions struct {
	ProjectID         uint
	TestNo            string
	Mode              string
	StandardID        *uint
	CustomItems       []models.DetectionStandardItem
	ProcessParams     any
	PLCWrites         any
	ReportRequest     any
	LimitCheckEnabled *bool
	EndPolicy         string
	DurationSec       int
	QualifiedHoldMS   int
	OperatorNote      string
	ReportTemplateID  *uint
	StartedByUserID   uint
}

var (
	ErrProjectAlreadyRunning = errors.New("project already has a running detection task")
	ErrReferenced            = errors.New("resource is already referenced")
	ErrEdgeInstanceMismatch  = errors.New("project and gateway edge_instance_id mismatch")
)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}
