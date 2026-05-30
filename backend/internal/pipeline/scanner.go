package pipeline

import (
	"log"
	"time"
)

func StartCycleScanner(channels *Channels, tags *TagManager, tasks *TaskManager) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for now := range ticker.C {
			for _, tag := range tags.All() {
				enqueueAlarmEvents(channels, tasks.EvaluateLimitAlarm(tag, now, false), -1)
				task := buildStoreTaskForTrigger(tag, tasks, 0, "cycle", now, "on_cycle", false, false)
				if task == nil {
					continue
				}
				select {
				case channels.Store <- task:
					tag.MarkStorageRoutesStored(task.StorageRoutes, now)
				default:
					log.Printf("[scanner] store queue full, drop var_id=%d", task.VarID)
				}
			}
		}
	}()
}
