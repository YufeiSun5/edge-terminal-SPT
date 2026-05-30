package database

import (
	"strings"
	"time"

	"spindle-edge/backend/internal/models"
)

func (r *Repository) LoadEnabledTaskRules() ([]models.TaskRule, error) {
	var rules []models.TaskRule
	err := r.db.Where("enabled = ?", true).
		Order("trigger_var_id asc, priority desc, id asc").
		Find(&rules).Error
	return rules, err
}

func (r *Repository) CreateTaskRule(rule *models.TaskRule) error {
	now := time.Now()
	normalizeTaskRule(rule)
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return r.db.Create(rule).Error
}

func normalizeTaskRule(rule *models.TaskRule) {
	rule.RuleCode = strings.TrimSpace(rule.RuleCode)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.TriggerOperator = strings.ToLower(strings.TrimSpace(rule.TriggerOperator))
	if rule.TriggerOperator == "" {
		rule.TriggerOperator = models.TaskRuleOperatorEQ
	}
	rule.TriggerEdge = strings.ToLower(strings.TrimSpace(rule.TriggerEdge))
	if rule.TriggerEdge == "" {
		rule.TriggerEdge = models.TaskRuleEdgeAny
	}
	rule.ActionType = strings.ToLower(strings.TrimSpace(rule.ActionType))
}
