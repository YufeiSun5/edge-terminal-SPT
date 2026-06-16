package services

import (
	"log"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/pipeline"
)

type ConfigSyncWatcher struct {
	repo           *database.Repository
	flows          *pipeline.TaskFlowExecutor
	edgeInstanceID string
	interval       time.Duration
	last           database.ConfigSyncWatermark
}

func NewConfigSyncWatcher(repo *database.Repository, flows *pipeline.TaskFlowExecutor, edgeInstanceID string, interval time.Duration) *ConfigSyncWatcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &ConfigSyncWatcher{repo: repo, flows: flows, edgeInstanceID: edgeInstanceID, interval: interval}
}

func (w *ConfigSyncWatcher) Start() {
	if w == nil || w.repo == nil {
		return
	}
	current, err := w.repo.ConfigSyncWatermark(w.edgeInstanceID)
	if err == nil {
		w.last = current
	}
	go w.loop()
}

func (w *ConfigSyncWatcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		current, err := w.repo.ConfigSyncWatermark(w.edgeInstanceID)
		if err != nil {
			log.Printf("config sync watcher watermark failed: %v", err)
			continue
		}
		if current.TaskFlowUpdatedAt != w.last.TaskFlowUpdatedAt || current.TaskFlowVersionTotal != w.last.TaskFlowVersionTotal {
			flows, err := w.repo.LoadEnabledTaskFlowsForEdge(w.edgeInstanceID)
			if err != nil {
				log.Printf("config sync watcher reload task flows failed: %v", err)
			} else if w.flows != nil {
				w.flows.Load(flows)
				log.Printf("config sync watcher reloaded task flows: %d", len(flows))
			}
		}
		if current.StationViewUpdatedAt != w.last.StationViewUpdatedAt || current.StationViewVersionTotal != w.last.StationViewVersionTotal {
			log.Printf("config sync watcher observed station-view change")
		}
		if current.DetectionStandardUpdatedAt != w.last.DetectionStandardUpdatedAt || current.DetectionStandardVersionTotal != w.last.DetectionStandardVersionTotal {
			log.Printf("config sync watcher observed detection-standard change")
		}
		w.last = current
	}
}
