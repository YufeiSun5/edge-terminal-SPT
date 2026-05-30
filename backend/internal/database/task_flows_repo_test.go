package database

import (
	"context"
	"strings"
	"testing"

	"spindle-edge/backend/internal/models"
)

func TestTaskFlowRepositoryCRUDAndSQL(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	flow := models.TaskFlow{
		ProjectID:       1,
		FlowCode:        "flow-1",
		Name:            "Flow 1",
		Enabled:         true,
		TriggerType:     models.TaskFlowTriggerDataChange,
		ConditionScript: "true",
		ActionType:      models.TaskFlowActionJavaScript,
		ActionScript:    "1 + 1",
		Priority:        5,
		Vars: []models.TaskFlowVar{
			{VarID: 100, VarName: "start_flag", Role: models.TaskFlowVarRoleWatch},
			{VarID: 101, VarName: "temp", Role: models.TaskFlowVarRoleRead},
		},
	}
	if err := repo.CreateTaskFlow(&flow); err != nil {
		t.Fatal(err)
	}
	flows, err := repo.LoadEnabledTaskFlows()
	if err != nil || len(flows) != 1 || len(flows[0].Vars) != 2 {
		t.Fatalf("unexpected enabled flows=%+v err=%v", flows, err)
	}
	got, err := repo.GetTaskFlow(flow.ID)
	if err != nil || got.FlowCode != "flow-1" || len(got.Vars) != 2 {
		t.Fatalf("unexpected get flow=%+v err=%v", got, err)
	}
	disabled := false
	updated, err := repo.UpdateTaskFlow(flow.ID, map[string]any{"enabled": disabled, "priority": 9, "project_id": uint(2)}, []models.TaskFlowVar{{VarID: 102, Role: models.TaskFlowVarRoleWatch}}, true)
	if err != nil || updated.Enabled || updated.Priority != 9 || updated.ProjectID != 2 || len(updated.Vars) != 1 || updated.Vars[0].VarID != 102 || updated.Vars[0].ProjectID != 2 {
		t.Fatalf("unexpected updated flow=%+v err=%v", updated, err)
	}
	run := models.TaskFlowRun{FlowID: flow.ID, FlowCode: flow.FlowCode, ProjectID: flow.ProjectID, TriggerType: models.TaskFlowTriggerManual, Status: models.TaskFlowStatusRunning}
	if err := repo.CreateTaskFlowRun(&run); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.QueryTaskFlowSQL(context.Background(), run.ID, flow.ID, "SELECT 1 AS ok", nil, 10); err != nil {
		t.Fatal(err)
	}
	if affected, err := repo.ExecTaskFlowSQL(context.Background(), run.ID, flow.ID, "UPDATE sys_task_flows SET remark = ? WHERE id = ?", []any{"done", flow.ID}); err != nil || affected != 1 {
		t.Fatalf("exec affected=%d err=%v", affected, err)
	}
	if err := repo.FinishTaskFlowRun(run.ID, models.TaskFlowStatusSuccess, `{"ok":true}`, "", `["done"]`, run.StartedAt.Add(1)); err != nil {
		t.Fatal(err)
	}
	var logs []models.TaskFlowSQLLog
	if err := db.Order("id asc").Find(&logs, "run_id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || !strings.Contains(logs[0].SQLText, "SELECT") || logs[1].AffectedRows != 1 {
		t.Fatalf("unexpected sql logs: %+v", logs)
	}
	runLogs, err := repo.ListTaskFlowSQLLogs(run.ID, 10)
	if err != nil || len(runLogs) != 2 {
		t.Fatalf("unexpected listed sql logs=%+v err=%v", runLogs, err)
	}
	flowID := flow.ID
	runs, total, err := repo.ListTaskFlowRuns(TaskFlowRunFilter{FlowID: &flowID, Status: models.TaskFlowStatusSuccess, Limit: 10})
	if err != nil || total != 1 || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("unexpected listed runs=%+v total=%d err=%v", runs, total, err)
	}
	gotRun, err := repo.GetTaskFlowRun(run.ID)
	if err != nil || gotRun.ID != run.ID || gotRun.Status != models.TaskFlowStatusSuccess {
		t.Fatalf("unexpected get run=%+v err=%v", gotRun, err)
	}
	if err := ValidateTaskFlowTrigger(models.TaskFlowTriggerManual); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTaskFlowTrigger("bad"); err == nil {
		t.Fatal("expected invalid trigger")
	}
	if err := repo.DeleteTaskFlow(flow.ID); err != nil {
		t.Fatal(err)
	}
	var varCount int64
	if err := db.Model(&models.TaskFlowVar{}).Where("flow_id = ?", flow.ID).Count(&varCount).Error; err != nil {
		t.Fatal(err)
	}
	if varCount != 0 {
		t.Fatalf("expected task flow vars deleted, got %d", varCount)
	}
	if _, err := repo.GetTaskFlow(flow.ID); err == nil {
		t.Fatal("expected deleted task flow to be missing")
	}
	gotRun, err = repo.GetTaskFlowRun(run.ID)
	if err != nil || gotRun.ID != run.ID {
		t.Fatalf("task flow run history should remain after deleting flow config: %+v err=%v", gotRun, err)
	}
}
