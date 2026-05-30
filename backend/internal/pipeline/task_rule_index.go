package pipeline

import (
	"strconv"
	"strings"

	"spindle-edge/backend/internal/models"
)

type TaskRuleMatch struct {
	Rule models.TaskRule
}

type TaskRuleIndex struct {
	byVar map[int64][]models.TaskRule
}

func NewTaskRuleIndex(rules []models.TaskRule) TaskRuleIndex {
	index := TaskRuleIndex{byVar: make(map[int64][]models.TaskRule)}
	for _, rule := range rules {
		if !rule.Enabled || rule.TriggerVarID == 0 {
			continue
		}
		index.byVar[rule.TriggerVarID] = append(index.byVar[rule.TriggerVarID], rule)
	}
	return index
}

func (idx TaskRuleIndex) Evaluate(varID int64, oldValue float64, newValue float64, changed bool, first bool) []TaskRuleMatch {
	rules := idx.byVar[varID]
	if len(rules) == 0 {
		return nil
	}
	matches := make([]TaskRuleMatch, 0, len(rules))
	for _, rule := range rules {
		if !taskRuleEdgeMatches(rule.TriggerEdge, oldValue, newValue, changed, first) {
			continue
		}
		if !taskRuleConditionMatches(rule.TriggerOperator, rule.TriggerValue, newValue) {
			continue
		}
		matches = append(matches, TaskRuleMatch{Rule: rule})
	}
	return matches
}

func taskRuleEdgeMatches(edge string, oldValue float64, newValue float64, changed bool, first bool) bool {
	switch strings.ToLower(strings.TrimSpace(edge)) {
	case "", models.TaskRuleEdgeAny:
		return first || changed
	case models.TaskRuleEdgeRising:
		return !first && oldValue <= 0 && newValue > 0
	case models.TaskRuleEdgeFalling:
		return !first && oldValue > 0 && newValue <= 0
	default:
		return false
	}
}

func taskRuleConditionMatches(operator string, expectedRaw string, actual float64) bool {
	expected, err := strconv.ParseFloat(strings.TrimSpace(expectedRaw), 64)
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "", models.TaskRuleOperatorEQ:
		return actual == expected
	case models.TaskRuleOperatorNE:
		return actual != expected
	case models.TaskRuleOperatorGT:
		return actual > expected
	case models.TaskRuleOperatorGE:
		return actual >= expected
	case models.TaskRuleOperatorLT:
		return actual < expected
	case models.TaskRuleOperatorLE:
		return actual <= expected
	default:
		return false
	}
}
