package cli

import (
	"bytes"
	"encoding/json"
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

func TestSourceChangeNamesIgnoresEboMetadata(t *testing.T) {
	got := sourceChangeNames([]string{
		".ebo/tree/project.md",
		".prompt/tree/project.md",
		".codex/hooks.json",
		".claude/settings.json",
		"drafts/ebo/login.md",
		"AGENTS.md",
		"CLAUDE.md",
		".gitignore",
		".gitattributes",
		"internal/auth/service.go",
	})
	if len(got) != 1 || got[0] != "internal/auth/service.go" {
		t.Fatalf("sourceChangeNames = %#v", got)
	}
}

func TestPatchTargetPaths(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: internal/auth/service.go\n*** Move to: internal/auth/login.go\n*** Add File: drafts/ebo/login.md\n*** End Patch"
	got := patchTargetPaths(patch)
	want := []string{"internal/auth/service.go", "internal/auth/login.go", "drafts/ebo/login.md"}
	if len(got) != len(want) {
		t.Fatalf("patchTargetPaths = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("patchTargetPaths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func codexPreToolUseInput(path string) *bytes.Buffer {
	payload := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input": map[string]string{
			"command": "*** Begin Patch\n*** Update File: " + path + "\n*** End Patch",
		},
	}
	data, _ := json.Marshal(payload)
	return bytes.NewBuffer(data)
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
