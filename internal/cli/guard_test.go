package cli

import (
	"path/filepath"
	"testing"

	"github.com/lqw905/Ebo/internal/planner"
)

func TestReceiptsCompletePlanRequiresPassedReceiptForEveryTask(t *testing.T) {
	plan := &planner.Plan{
		ID:         "plan-1",
		BaseCommit: "commit-1",
		Tasks: []planner.Task{
			{ID: "task-a", Status: "passed", ContentHash: "content-a", EffectiveHash: "effective-a"},
			{ID: "task-b", Status: "passed", ContentHash: "content-b", EffectiveHash: "effective-b"},
		},
	}
	receipts := []receiptRecord{
		{
			Schema:        "ebo.receipt/v1",
			PlanID:        plan.ID,
			TaskID:        "task-a",
			Result:        "passed",
			BaseCommit:    plan.BaseCommit,
			ContentHash:   "content-a",
			EffectiveHash: "effective-a",
		},
	}
	if receiptsCompletePlan(receipts, plan) {
		t.Fatal("one receipt must not complete a two-task plan")
	}

	receipts = append(receipts, receiptRecord{
		Schema:        "ebo.receipt/v1",
		PlanID:        plan.ID,
		TaskID:        "task-b",
		Result:        "failed",
		BaseCommit:    plan.BaseCommit,
		ContentHash:   "content-b",
		EffectiveHash: "effective-b",
	})
	if receiptsCompletePlan(receipts, plan) {
		t.Fatal("failed receipt must not satisfy staged guard")
	}

	receipts[1].Result = "passed"
	if !receiptsCompletePlan(receipts, plan) {
		t.Fatal("matching passed receipts should complete the plan")
	}

	receipts[1].EffectiveHash = "tampered"
	if receiptsCompletePlan(receipts, plan) {
		t.Fatal("hash-mismatched receipt must not satisfy staged guard")
	}
}

func TestProjectRelativePathUsesProjectRoot(t *testing.T) {
	root := t.TempDir()
	got, err := projectRelativePath(root, filepath.Join("internal", "auth", "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "internal/auth/service.go" {
		t.Fatalf("projectRelativePath = %q", got)
	}
	outside := filepath.Join(filepath.Dir(root), "outside.go")
	if _, err := projectRelativePath(root, outside); err == nil {
		t.Fatal("absolute path outside the project should be rejected")
	}
}
