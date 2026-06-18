package database

import (
	"errors"
	"testing"

	"spindle-edge/backend/internal/models"
)

func TestRepositoryDetectionPlansLifecycle(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	if err := db.Create(&models.DetectionPlan{
		PlanNo:         "PLAN-1",
		SourceSystem:   "mes",
		ExternalPlanID: "MES-1",
		FactoryNo:      "FAC-1",
		DeviceModel:    "M-A",
		TestItemCode:   "cooling",
		TestItemName:   "制冷测试",
		TestSequence:   1,
		Mode:           "standard",
		StandardCode:   "STD-A",
		Status:         models.DetectionPlanStatusPending,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.DetectionPlan{
		PlanNo:         "PLAN-2",
		SourceSystem:   "mes",
		ExternalPlanID: "MES-2",
		FactoryNo:      "FAC-1",
		DeviceModel:    "M-A",
		TestItemCode:   "heating",
		TestItemName:   "制热测试",
		TestSequence:   2,
		Mode:           "standard",
		StandardCode:   "STD-B",
		Status:         models.DetectionPlanStatusStarted,
	}).Error; err != nil {
		t.Fatal(err)
	}
	plans, total, err := repo.ListDetectionPlans(DetectionPlanFilter{Status: models.DetectionPlanStatusPending, FactoryNo: "FAC-1"})
	if err != nil || total != 1 || len(plans) != 1 || plans[0].PlanNo != "PLAN-1" {
		t.Fatalf("unexpected pending plans len=%d total=%d err=%v plans=%+v", len(plans), total, err, plans)
	}
	starting, err := repo.MarkDetectionPlanStarting(plans[0].ID)
	if err != nil || starting.Status != models.DetectionPlanStatusStarting {
		t.Fatalf("mark starting got=%+v err=%v", starting, err)
	}
	if _, err := repo.MarkDetectionPlanStarting(plans[0].ID); !errors.Is(err, ErrDetectionPlanNotPending) {
		t.Fatalf("expected not pending error, got %v", err)
	}
	ownerProjectID := uint(7)
	started, err := repo.MarkDetectionPlanStarted(DetectionPlanStartedUpdate{
		PlanID:              plans[0].ID,
		TaskID:              99,
		OwnerEdgeInstanceID: "edge-local",
		OwnerProjectID:      ownerProjectID,
		OwnerProjectCode:    "AC-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != models.DetectionPlanStatusStarted || started.StartedTaskID == nil || *started.StartedTaskID != 99 || started.OwnerProjectID == nil || *started.OwnerProjectID != ownerProjectID {
		t.Fatalf("unexpected started plan: %+v", started)
	}
}
