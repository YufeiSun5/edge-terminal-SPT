package pipeline

import (
	"sync"

	"spindle-edge/backend/internal/models"
)

type gatewayTopicKey struct {
	gatewayID int
	topic     string
}

type TagManager struct {
	mu             sync.RWMutex
	tags           map[int64]*models.Tag
	byName         map[string]*models.Tag
	byGatewayTopic map[gatewayTopicKey][]*models.Tag
	byProject      map[uint][]*models.Tag
}

func NewTagManager() *TagManager {
	return &TagManager{
		tags:           make(map[int64]*models.Tag),
		byName:         make(map[string]*models.Tag),
		byGatewayTopic: make(map[gatewayTopicKey][]*models.Tag),
		byProject:      make(map[uint][]*models.Tag),
	}
}

func (tm *TagManager) Load(configs []models.TagConfig) {
	next := make(map[int64]*models.Tag, len(configs))
	nextByName := make(map[string]*models.Tag, len(configs))
	nextByGatewayTopic := make(map[gatewayTopicKey][]*models.Tag, len(configs))
	nextByProject := make(map[uint][]*models.Tag, len(configs))

	tm.mu.RLock()
	old := tm.tags
	tm.mu.RUnlock()

	for _, cfg := range configs {
		if !runtimeEligible(cfg) {
			continue
		}
		if existing, ok := old[cfg.VarID]; ok {
			existing.Config = cfg
			next[cfg.VarID] = existing
			nextByName[cfg.VarName] = existing
			addGatewayTopicIndex(nextByGatewayTopic, existing)
			addProjectIndex(nextByProject, existing)
			continue
		}
		tag := models.NewTag(cfg)
		next[cfg.VarID] = tag
		nextByName[cfg.VarName] = tag
		addGatewayTopicIndex(nextByGatewayTopic, tag)
		addProjectIndex(nextByProject, tag)
	}

	tm.mu.Lock()
	tm.tags = next
	tm.byName = nextByName
	tm.byGatewayTopic = nextByGatewayTopic
	tm.byProject = nextByProject
	tm.mu.Unlock()
}

func (tm *TagManager) Upsert(configs []models.TagConfig) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, cfg := range configs {
		if !runtimeEligible(cfg) {
			delete(tm.tags, cfg.VarID)
			tm.rebuildIndexesLocked()
			continue
		}
		if existing, ok := tm.tags[cfg.VarID]; ok {
			existing.Config = cfg
			tm.byName[cfg.VarName] = existing
			tm.rebuildIndexesLocked()
			continue
		}
		tag := models.NewTag(cfg)
		tm.tags[cfg.VarID] = tag
		tm.byName[cfg.VarName] = tag
		addGatewayTopicIndex(tm.byGatewayTopic, tag)
		addProjectIndex(tm.byProject, tag)
	}
}

func (tm *TagManager) All() []*models.Tag {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tags := make([]*models.Tag, 0, len(tm.tags))
	for _, tag := range tm.tags {
		tags = append(tags, tag)
	}
	return tags
}

func (tm *TagManager) Get(varID int64) (*models.Tag, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tag, ok := tm.tags[varID]
	return tag, ok
}

func (tm *TagManager) ForMessage(gatewayID int, topic string) []*models.Tag {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tags, ok := tm.byGatewayTopic[gatewayTopicKey{gatewayID: gatewayID, topic: topic}]; ok {
		return append([]*models.Tag(nil), tags...)
	}
	tags := make([]*models.Tag, 0, len(tm.tags))
	for _, tag := range tm.tags {
		tags = append(tags, tag)
	}
	return tags
}

func (tm *TagManager) ForProject(ProjectID uint) []*models.Tag {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tags := tm.byProject[ProjectID]
	return append([]*models.Tag(nil), tags...)
}

func (tm *TagManager) Snapshots() []models.TagSnapshot {
	tags := tm.All()
	items := make([]models.TagSnapshot, 0, len(tags))
	for _, tag := range tags {
		items = append(items, tag.Snapshot())
	}
	return items
}

func (tm *TagManager) SnapshotsForProject(ProjectID uint) []models.TagSnapshot {
	tags := tm.ForProject(ProjectID)
	items := make([]models.TagSnapshot, 0, len(tags))
	for _, tag := range tags {
		items = append(items, tag.Snapshot())
	}
	return items
}

func (tm *TagManager) Count() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.tags)
}

func (tm *TagManager) rebuildIndexesLocked() {
	tm.byName = make(map[string]*models.Tag, len(tm.tags))
	tm.byGatewayTopic = make(map[gatewayTopicKey][]*models.Tag, len(tm.tags))
	tm.byProject = make(map[uint][]*models.Tag, len(tm.tags))
	for _, tag := range tm.tags {
		tm.byName[tag.Config.VarName] = tag
		addGatewayTopicIndex(tm.byGatewayTopic, tag)
		addProjectIndex(tm.byProject, tag)
	}
}

func runtimeEligible(cfg models.TagConfig) bool {
	return cfg.Enabled && cfg.ProjectID != nil
}

func addGatewayTopicIndex(index map[gatewayTopicKey][]*models.Tag, tag *models.Tag) {
	if tag.Config.SourceTopic == "" {
		return
	}
	key := gatewayTopicKey{gatewayID: tag.Config.GatewayID, topic: tag.Config.SourceTopic}
	index[key] = append(index[key], tag)
}

func addProjectIndex(index map[uint][]*models.Tag, tag *models.Tag) {
	if tag.Config.ProjectID == nil {
		return
	}
	index[*tag.Config.ProjectID] = append(index[*tag.Config.ProjectID], tag)
}
