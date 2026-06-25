package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"spindle-edge/backend/internal/pipeline"
)

const (
	RuntimeDraftNamespaceStationDetectionPreload = "station.detection_preload"
	RuntimeDraftScopeProject                     = "project"
)

var (
	ErrRuntimeDraftNotFound         = errors.New("runtime draft not found")
	ErrRuntimeDraftRevisionConflict = errors.New("runtime draft revision conflict")
	ErrRuntimeDraftTooLarge         = errors.New("runtime draft is too large")
	ErrRuntimeDraftProjectRunning   = errors.New("project is running")
	ErrRuntimeDraftStale            = errors.New("runtime draft is stale")
)

type RuntimeDraftPolicy struct {
	Namespace                 string
	ScopeType                 string
	MaxBytes                  int
	DefaultTTL                time.Duration
	RequireExpectedRevision   bool
	RejectWriteWhenProjectRun bool
}

type RuntimeDraft struct {
	Namespace string          `json:"namespace"`
	ScopeType string          `json:"scope_type"`
	ScopeID   string          `json:"scope_id"`
	Revision  int64           `json:"revision"`
	UpdatedAt time.Time       `json:"updated_at"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type RuntimeDraftPutInput struct {
	Namespace        string
	ScopeType        string
	ScopeID          string
	ExpectedRevision *int64
	TTLSec           int
	Data             any
}

type RuntimeDraftClearInput struct {
	Namespace        string
	ScopeType        string
	ScopeID          string
	ExpectedRevision *int64
}

type RuntimeDraftService struct {
	mu       sync.RWMutex
	tasks    *pipeline.TaskManager
	now      func() time.Time
	policies map[string]RuntimeDraftPolicy
	data     map[runtimeDraftKey]RuntimeDraft
}

type runtimeDraftKey struct {
	namespace string
	scopeType string
	scopeID   string
}

func NewRuntimeDraftService(tasks *pipeline.TaskManager) *RuntimeDraftService {
	service := &RuntimeDraftService{
		tasks: tasks,
		now:   time.Now,
		policies: map[string]RuntimeDraftPolicy{
			RuntimeDraftNamespaceStationDetectionPreload: {
				Namespace:                 RuntimeDraftNamespaceStationDetectionPreload,
				ScopeType:                 RuntimeDraftScopeProject,
				MaxBytes:                  512 * 1024,
				DefaultTTL:                24 * time.Hour,
				RequireExpectedRevision:   true,
				RejectWriteWhenProjectRun: true,
			},
		},
		data: map[runtimeDraftKey]RuntimeDraft{},
	}
	return service
}

func (s *RuntimeDraftService) Get(namespace, scopeType, scopeID string) (RuntimeDraft, error) {
	policy, key, err := s.resolveKey(namespace, scopeType, scopeID)
	if err != nil {
		return RuntimeDraft{}, err
	}
	_ = policy
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	draft, ok := s.data[key]
	if !ok {
		return RuntimeDraft{}, ErrRuntimeDraftNotFound
	}
	if runtimeDraftExpired(draft, now) {
		delete(s.data, key)
		return RuntimeDraft{}, ErrRuntimeDraftNotFound
	}
	return cloneRuntimeDraft(draft), nil
}

func (s *RuntimeDraftService) Put(input RuntimeDraftPutInput) (RuntimeDraft, error) {
	policy, key, err := s.resolveKey(input.Namespace, input.ScopeType, input.ScopeID)
	if err != nil {
		return RuntimeDraft{}, err
	}
	if policy.RequireExpectedRevision && input.ExpectedRevision == nil {
		return RuntimeDraft{}, fmt.Errorf("expected_revision is required")
	}
	if policy.RejectWriteWhenProjectRun && s.projectRunning(key.scopeID) {
		return RuntimeDraft{}, ErrRuntimeDraftProjectRunning
	}
	raw, err := normalizeRuntimeDraftData(input.Data)
	if err != nil {
		return RuntimeDraft{}, err
	}
	if policy.MaxBytes > 0 && len(raw) > policy.MaxBytes {
		return RuntimeDraft{}, ErrRuntimeDraftTooLarge
	}
	now := s.now()
	var expiresAt *time.Time
	ttl := policy.DefaultTTL
	if input.TTLSec > 0 {
		ttl = time.Duration(input.TTLSec) * time.Second
	}
	if ttl > 0 {
		value := now.Add(ttl)
		expiresAt = &value
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.data[key]
	if exists && runtimeDraftExpired(current, now) {
		delete(s.data, key)
		exists = false
		current = RuntimeDraft{}
	}
	currentRevision := int64(0)
	if exists {
		currentRevision = current.Revision
	}
	if input.ExpectedRevision != nil && *input.ExpectedRevision != currentRevision {
		return RuntimeDraft{}, ErrRuntimeDraftRevisionConflict
	}
	next := RuntimeDraft{
		Namespace: key.namespace,
		ScopeType: key.scopeType,
		ScopeID:   key.scopeID,
		Revision:  currentRevision + 1,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
		Data:      raw,
	}
	s.data[key] = next
	return cloneRuntimeDraft(next), nil
}

func (s *RuntimeDraftService) Clear(input RuntimeDraftClearInput) error {
	policy, key, err := s.resolveKey(input.Namespace, input.ScopeType, input.ScopeID)
	if err != nil {
		return err
	}
	if policy.RequireExpectedRevision && input.ExpectedRevision == nil {
		return fmt.Errorf("expected_revision is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.data[key]
	if !exists || runtimeDraftExpired(current, s.now()) {
		delete(s.data, key)
		return ErrRuntimeDraftNotFound
	}
	if input.ExpectedRevision != nil && *input.ExpectedRevision != current.Revision {
		return ErrRuntimeDraftRevisionConflict
	}
	delete(s.data, key)
	return nil
}

func (s *RuntimeDraftService) ClearIfRevision(namespace, scopeType, scopeID string, revision int64) {
	_, key, err := s.resolveKey(namespace, scopeType, scopeID)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.data[key]
	if !exists || current.Revision != revision {
		return
	}
	delete(s.data, key)
}

func (s *RuntimeDraftService) resolveKey(namespace, scopeType, scopeID string) (RuntimeDraftPolicy, runtimeDraftKey, error) {
	namespace = strings.TrimSpace(namespace)
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	policy, ok := s.policies[namespace]
	if !ok {
		return RuntimeDraftPolicy{}, runtimeDraftKey{}, fmt.Errorf("runtime draft namespace is not allowed")
	}
	if scopeType == "" {
		scopeType = policy.ScopeType
	}
	if scopeType != policy.ScopeType {
		return RuntimeDraftPolicy{}, runtimeDraftKey{}, fmt.Errorf("runtime draft scope_type must be %s", policy.ScopeType)
	}
	if scopeID == "" {
		return RuntimeDraftPolicy{}, runtimeDraftKey{}, fmt.Errorf("runtime draft scope_id is required")
	}
	return policy, runtimeDraftKey{namespace: namespace, scopeType: scopeType, scopeID: scopeID}, nil
}

func (s *RuntimeDraftService) projectRunning(scopeID string) bool {
	if s == nil || s.tasks == nil {
		return false
	}
	projectID, err := strconv.ParseUint(strings.TrimSpace(scopeID), 10, 64)
	if err != nil || projectID == 0 {
		return false
	}
	_, ok := s.tasks.ActiveForProject(uint(projectID))
	return ok
}

func normalizeRuntimeDraftData(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("runtime draft data is invalid JSON")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`{}`), nil
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func runtimeDraftExpired(draft RuntimeDraft, now time.Time) bool {
	return draft.ExpiresAt != nil && !draft.ExpiresAt.After(now)
}

func cloneRuntimeDraft(draft RuntimeDraft) RuntimeDraft {
	draft.Data = append(json.RawMessage(nil), draft.Data...)
	return draft
}
