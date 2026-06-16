package services

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"spindle-edge/backend/internal/database"
)

const (
	RuntimeSettingDetectionConfigReadyTimeoutMS  = "detection_config_ready_timeout_ms"
	RuntimeSettingDetectionConfigReadyIntervalMS = "detection_config_ready_interval_ms"
)

const (
	defaultDetectionConfigReadyTimeoutMS  = 60000
	defaultDetectionConfigReadyIntervalMS = 5000
)

type RuntimeSettingsService struct {
	repo *database.Repository
	mu   sync.RWMutex
	data map[string]string
}

type RuntimeSettingsView struct {
	DetectionConfigReadyTimeoutMS  int `json:"detection_config_ready_timeout_ms"`
	DetectionConfigReadyIntervalMS int `json:"detection_config_ready_interval_ms"`
}

type RuntimeSettingsUpdate struct {
	DetectionConfigReadyTimeoutMS  *int `json:"detection_config_ready_timeout_ms"`
	DetectionConfigReadyIntervalMS *int `json:"detection_config_ready_interval_ms"`
}

func NewRuntimeSettingsService(repo *database.Repository) *RuntimeSettingsService {
	return &RuntimeSettingsService{repo: repo, data: map[string]string{}}
}

func (s *RuntimeSettingsService) Load() error {
	if s == nil || s.repo == nil {
		return nil
	}
	if err := s.repo.CreateRuntimeSettingIfMissing(RuntimeSettingDetectionConfigReadyTimeoutMS, strconv.Itoa(defaultDetectionConfigReadyTimeoutMS), "Detection config readiness wait timeout in milliseconds."); err != nil {
		return err
	}
	if err := s.repo.CreateRuntimeSettingIfMissing(RuntimeSettingDetectionConfigReadyIntervalMS, strconv.Itoa(defaultDetectionConfigReadyIntervalMS), "Detection config readiness retry interval in milliseconds."); err != nil {
		return err
	}
	values, err := s.repo.ListRuntimeSettings()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.data = values
	s.mu.Unlock()
	return nil
}

func (s *RuntimeSettingsService) DetectionConfigWaitTimeout() time.Duration {
	return time.Duration(s.intValue(RuntimeSettingDetectionConfigReadyTimeoutMS, defaultDetectionConfigReadyTimeoutMS)) * time.Millisecond
}

func (s *RuntimeSettingsService) DetectionConfigWaitInterval() time.Duration {
	value := s.intValue(RuntimeSettingDetectionConfigReadyIntervalMS, defaultDetectionConfigReadyIntervalMS)
	if value <= 0 {
		value = defaultDetectionConfigReadyIntervalMS
	}
	return time.Duration(value) * time.Millisecond
}

func (s *RuntimeSettingsService) View() RuntimeSettingsView {
	return RuntimeSettingsView{
		DetectionConfigReadyTimeoutMS:  s.intValue(RuntimeSettingDetectionConfigReadyTimeoutMS, defaultDetectionConfigReadyTimeoutMS),
		DetectionConfigReadyIntervalMS: s.intValue(RuntimeSettingDetectionConfigReadyIntervalMS, defaultDetectionConfigReadyIntervalMS),
	}
}

func (s *RuntimeSettingsService) Update(input RuntimeSettingsUpdate) (RuntimeSettingsView, error) {
	if s == nil || s.repo == nil {
		return RuntimeSettingsView{}, nil
	}
	if input.DetectionConfigReadyTimeoutMS != nil {
		if *input.DetectionConfigReadyTimeoutMS <= 0 {
			return RuntimeSettingsView{}, fmt.Errorf("detection_config_ready_timeout_ms must be positive")
		}
		if err := s.repo.UpsertRuntimeSetting(RuntimeSettingDetectionConfigReadyTimeoutMS, strconv.Itoa(*input.DetectionConfigReadyTimeoutMS), "Detection config readiness wait timeout in milliseconds."); err != nil {
			return RuntimeSettingsView{}, err
		}
	}
	if input.DetectionConfigReadyIntervalMS != nil {
		if *input.DetectionConfigReadyIntervalMS <= 0 {
			return RuntimeSettingsView{}, fmt.Errorf("detection_config_ready_interval_ms must be positive")
		}
		if err := s.repo.UpsertRuntimeSetting(RuntimeSettingDetectionConfigReadyIntervalMS, strconv.Itoa(*input.DetectionConfigReadyIntervalMS), "Detection config readiness retry interval in milliseconds."); err != nil {
			return RuntimeSettingsView{}, err
		}
	}
	if err := s.Load(); err != nil {
		return RuntimeSettingsView{}, err
	}
	return s.View(), nil
}

func (s *RuntimeSettingsService) intValue(key string, fallback int) int {
	if s == nil {
		return fallback
	}
	s.mu.RLock()
	raw := s.data[key]
	s.mu.RUnlock()
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
