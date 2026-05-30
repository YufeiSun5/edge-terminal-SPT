package discovery

import (
	"encoding/json"
	"hash/fnv"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"
)

type Worker struct {
	repo *database.Repository
	tags *pipeline.TagManager
	seen sync.Map
}

func Start(channels *pipeline.Channels, repo *database.Repository, tags *pipeline.TagManager) {
	worker := &Worker{repo: repo, tags: tags}
	go worker.run(channels)
	log.Println("discovery worker started")
}

func (w *Worker) run(channels *pipeline.Channels) {
	for msg := range channels.Discovery {
		discoveredTags, err := w.discover(msg.GatewayID, msg.Topic, msg.Payload)
		if err != nil {
			log.Printf("[discovery] parse failed gateway=%d topic=%s err=%v", msg.GatewayID, msg.Topic, err)
			continue
		}
		if len(discoveredTags) == 0 {
			continue
		}
		if err := w.repo.UpsertDiscoveredTags(discoveredTags); err != nil {
			log.Printf("[discovery] upsert failed gateway=%d count=%d err=%v", msg.GatewayID, len(discoveredTags), err)
			continue
		}
		runtimeTags, err := w.repo.LoadTags()
		if err != nil {
			log.Printf("[discovery] runtime reload failed gateway=%d err=%v", msg.GatewayID, err)
			continue
		}
		w.tags.Load(runtimeTags)
		log.Printf("[discovery] discovered gateway=%d count=%d runtime_tags=%d", msg.GatewayID, len(discoveredTags), len(runtimeTags))
	}
}

func (w *Worker) discover(gatewayID int, topic string, payload []byte) ([]models.TagConfig, error) {
	if kio.LooksLikePayload(payload) {
		return w.discoverKIO(gatewayID, topic, payload)
	}

	var root interface{}
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, err
	}

	leaves := make([]leaf, 0, 128)
	flatten("", root, &leaves)

	now := time.Now()
	tags := make([]models.TagConfig, 0, len(leaves))
	for _, item := range leaves {
		if item.Path == "" {
			continue
		}
		cacheKey := strconv.Itoa(gatewayID) + ":" + item.Path
		if _, loaded := w.seen.LoadOrStore(cacheKey, struct{}{}); loaded {
			continue
		}

		tags = append(tags, models.TagConfig{
			VarID:       stableVarID(gatewayID, item.Path),
			GatewayID:   gatewayID,
			SourceTopic: topic,
			SourcePath:  item.Path,
			SourceType:  models.TagSourceMQTT,
			RawName:     item.Name,
			VarName:     sanitizeVarName(item.Path),
			DisplayName: item.Name,
			JSONPath:    item.Path,
			DataType:    item.DataType,
			ScaleFactor: 1,
			Discovered:  true,
			Enabled:     false,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return tags, nil
}

func (w *Worker) discoverKIO(gatewayID int, topic string, payload []byte) ([]models.TagConfig, error) {
	defs, err := kio.DiscoverVariables(payload)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tags := make([]models.TagConfig, 0, len(defs))
	for _, item := range defs {
		cacheKey := strconv.Itoa(gatewayID) + ":" + item.SourcePath
		if _, loaded := w.seen.LoadOrStore(cacheKey, struct{}{}); loaded {
			continue
		}

		tags = append(tags, models.TagConfig{
			VarID:       stableVarID(gatewayID, item.SourcePath),
			GatewayID:   gatewayID,
			SourceTopic: topic,
			SourcePath:  item.SourcePath,
			SourceType:  models.TagSourceMQTT,
			RawName:     item.Name,
			VarName:     sanitizeVarName(item.Name),
			DisplayName: item.Name,
			JSONPath:    item.JSONPath,
			DataType:    item.DataType,
			ScaleFactor: 1,
			Discovered:  true,
			Enabled:     false,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return tags, nil
}

type leaf struct {
	Path     string
	Name     string
	DataType string
}

func flatten(prefix string, value interface{}, leaves *[]leaf) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flatten(path, child, leaves)
		}
	case []interface{}:
		for index, child := range typed {
			path := strconv.Itoa(index)
			if prefix != "" {
				path = prefix + "." + path
			}
			flatten(path, child, leaves)
		}
	default:
		if value == nil {
			return
		}
		*leaves = append(*leaves, leaf{
			Path:     prefix,
			Name:     lastPathPart(prefix),
			DataType: inferDataType(value),
		})
	}
}

func inferDataType(value interface{}) string {
	switch value.(type) {
	case bool:
		return "BOOL"
	case float64:
		return "FLOAT"
	case string:
		return "STRING"
	default:
		return "STRING"
	}
}

func stableVarID(gatewayID int, path string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strconv.Itoa(gatewayID)))
	_, _ = hash.Write([]byte(":"))
	_, _ = hash.Write([]byte(path))
	return int64(hash.Sum64() & 0x7fffffffffffffff)
}

func lastPathPart(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func sanitizeVarName(path string) string {
	name := strings.NewReplacer(".", "_", "-", "_", " ", "_").Replace(path)
	if name == "" {
		return "unnamed"
	}
	return name
}
