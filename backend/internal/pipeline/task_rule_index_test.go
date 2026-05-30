package pipeline

import (
	"testing"
	"time"

	"spindle-edge/backend/internal/models"
)

func TestTaskRuleIndexEvaluatesOnlyIndexedVariableRules(t *testing.T) {
	index := NewTaskRuleIndex([]models.TaskRule{
		{
			ID:              1,
			Enabled:         true,
			RuleCode:        "start-on-flag",
			TriggerVarID:    100,
			TriggerOperator: models.TaskRuleOperatorEQ,
			TriggerValue:    "1",
			TriggerEdge:     models.TaskRuleEdgeRising,
			ActionType:      models.TaskRuleActionDetectionStart,
		},
		{
			ID:              2,
			Enabled:         true,
			RuleCode:        "other-var",
			TriggerVarID:    200,
			TriggerOperator: models.TaskRuleOperatorEQ,
			TriggerValue:    "1",
			TriggerEdge:     models.TaskRuleEdgeAny,
			ActionType:      models.TaskRuleActionStorageEnable,
		},
	})

	if matches := index.Evaluate(100, 0, 1, true, false); len(matches) != 1 || matches[0].Rule.RuleCode != "start-on-flag" {
		t.Fatalf("expected rising start rule only, got %+v", matches)
	}
	if matches := index.Evaluate(100, 1, 1, false, false); len(matches) != 0 {
		t.Fatalf("unchanged value should not match rising rule: %+v", matches)
	}
	if matches := index.Evaluate(200, 0, 1, true, false); len(matches) != 1 || matches[0].Rule.RuleCode != "other-var" {
		t.Fatalf("expected var-specific rule, got %+v", matches)
	}
}

func TestTaskFlowIndexPriorityCooldownAndHold(t *testing.T) {
	now := time.Date(2026, 5, 30, 11, 40, 0, 0, time.UTC)
	index := NewTaskFlowIndex([]models.TaskFlow{
		{ID: 1, ProjectID: 1, FlowCode: "low", Enabled: true, TriggerType: models.TaskFlowTriggerDataChange, Priority: 1, Vars: []models.TaskFlowVar{{VarID: 100, Role: models.TaskFlowVarRoleWatch}}},
		{ID: 2, ProjectID: 1, FlowCode: "high", Enabled: true, TriggerType: models.TaskFlowTriggerDataChange, Priority: 9, CooldownMS: 1000, Vars: []models.TaskFlowVar{{VarID: 100, Role: models.TaskFlowVarRoleWatch}}},
		{ID: 3, ProjectID: 1, FlowCode: "hold", Enabled: true, TriggerType: models.TaskFlowTriggerDataChange, HoldMS: 500, Priority: 5, Vars: []models.TaskFlowVar{{VarID: 100, Role: models.TaskFlowVarRoleWatch}}},
	})
	matches := index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: 1, TriggerVarID: 100, At: now})
	if len(matches) != 2 || matches[0].FlowCode != "high" || matches[1].FlowCode != "low" {
		t.Fatalf("unexpected first matches: %+v", matches)
	}
	matches = index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: 1, TriggerVarID: 100, At: now.Add(100 * time.Millisecond)})
	if len(matches) != 1 || matches[0].FlowCode != "low" {
		t.Fatalf("cooldown should suppress high priority flow: %+v", matches)
	}
	matches = index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerDataChange, ProjectID: 1, TriggerVarID: 100, At: now.Add(600 * time.Millisecond)})
	if len(matches) != 2 || matches[0].FlowCode != "hold" {
		t.Fatalf("hold flow should become eligible: %+v", matches)
	}
}

func TestTaskFlowIndexScheduleInterval(t *testing.T) {
	now := time.Date(2026, 5, 30, 21, 10, 0, 0, time.UTC)
	index := NewTaskFlowIndex([]models.TaskFlow{
		{ID: 11, ProjectID: 1, FlowCode: "fast", Enabled: true, TriggerType: models.TaskFlowTriggerSchedule, Priority: 5, ScheduleIntervalMS: 500},
		{ID: 12, ProjectID: 1, FlowCode: "fallback-cooldown", Enabled: true, TriggerType: models.TaskFlowTriggerSchedule, Priority: 3, CooldownMS: 1000},
		{ID: 13, ProjectID: 1, FlowCode: "no-interval", Enabled: true, TriggerType: models.TaskFlowTriggerSchedule, Priority: 9},
	})

	matches := index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerSchedule, At: now})
	if len(matches) != 2 || matches[0].FlowCode != "fast" || matches[1].FlowCode != "fallback-cooldown" {
		t.Fatalf("unexpected first schedule matches: %+v", matches)
	}
	matches = index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerSchedule, At: now.Add(200 * time.Millisecond)})
	if len(matches) != 0 {
		t.Fatalf("schedule interval should suppress early matches: %+v", matches)
	}
	matches = index.Match(TaskFlowEvent{TriggerType: models.TaskFlowTriggerSchedule, At: now.Add(600 * time.Millisecond)})
	if len(matches) != 1 || matches[0].FlowCode != "fast" {
		t.Fatalf("expected fast schedule only after 600ms: %+v", matches)
	}
}
