package storage

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

func StartWorkers(count int, batchSize int, channels *pipeline.Channels, repo *database.Repository) {
	if batchSize <= 0 {
		batchSize = 100
	}
	if count <= 0 {
		count = 1
	}
	bus := newStorageBus(repo, batchSize, 200*time.Millisecond)
	go bus.Run(channels.Store)
	log.Printf("storage bus started: dispatchers=%d batch_size=%d", count, batchSize)
}

type StorageBus struct {
	repo          *database.Repository
	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	buckets map[string]chan *models.StoreTask
}

func newStorageBus(repo *database.Repository, batchSize int, flushInterval time.Duration) *StorageBus {
	return &StorageBus{
		repo:          repo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		buckets:       make(map[string]chan *models.StoreTask),
	}
}

func (b *StorageBus) Run(input <-chan *models.StoreTask) {
	for task := range input {
		for _, bucketed := range splitStoreTaskByBucket(task) {
			b.bucket(bucketKey(bucketed)) <- bucketed
		}
	}
	b.closeBuckets()
}

func (b *StorageBus) bucket(key string) chan *models.StoreTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.buckets[key]; ok {
		return ch
	}
	ch := make(chan *models.StoreTask, max(1024, b.batchSize*4))
	b.buckets[key] = ch
	go b.bucketWorker(key, ch)
	return ch
}

func (b *StorageBus) closeBuckets() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, ch := range b.buckets {
		close(ch)
		delete(b.buckets, key)
	}
}

func (b *StorageBus) bucketWorker(key string, input <-chan *models.StoreTask) {
	batch := make([]*models.StoreTask, 0, b.batchSize)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case task, ok := <-input:
			if !ok {
				flush(key, batch, b.repo)
				return
			}
			batch = append(batch, task)
			if len(batch) >= b.batchSize {
				flush(key, batch, b.repo)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				flush(key, batch, b.repo)
				batch = batch[:0]
			}
		}
	}
}

func splitStoreTaskByBucket(task *models.StoreTask) []*models.StoreTask {
	if task == nil {
		return nil
	}
	if len(task.StorageRoutes) == 0 {
		return []*models.StoreTask{task}
	}
	byKey := make(map[string][]models.DetectionRunStorageRoute)
	order := make([]string, 0, len(task.StorageRoutes))
	for _, route := range task.StorageRoutes {
		if route.StorageTarget == models.StorageTargetNone {
			continue
		}
		key := routeBucketKey(task.ProjectID, route.StorageTable)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], route)
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]*models.StoreTask, 0, len(order))
	for idx, key := range order {
		copyTask := *task
		copyTask.StorageRoutes = append([]models.DetectionRunStorageRoute(nil), byKey[key]...)
		copyTask.SkipHistoryRow = task.SkipHistoryRow || idx > 0
		out = append(out, &copyTask)
	}
	return out
}

func bucketKey(task *models.StoreTask) string {
	if task == nil {
		return "nil"
	}
	if len(task.StorageRoutes) == 0 {
		return routeBucketKey(task.ProjectID, "rt_history_data")
	}
	return routeBucketKey(task.ProjectID, task.StorageRoutes[0].StorageTable)
}

func routeBucketKey(projectID uint, table string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		table = "rt_history_data"
	}
	return fmt.Sprintf("project:%d/table:%s", projectID, table)
}

func flush(key string, batch []*models.StoreTask, repo *database.Repository) {
	if len(batch) == 0 {
		return
	}
	if err := repo.InsertHistoryBatch(batch); err != nil {
		log.Printf("[storage-bus %s] insert history batch failed count=%d err=%v", key, len(batch), err)
	}
}
