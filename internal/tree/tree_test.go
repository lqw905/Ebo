package tree

import (
	"testing"

	"github.com/lqw905/Ebo/internal/document"
	"github.com/lqw905/Ebo/internal/project"
)

func TestApplyTaskResultMarksPassedPromptInSync(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writePrompt(t, root, project.RootID, rootPromptFixture)
	writePrompt(t, root, "feature.hello", helloPromptFixture)

	promptTree, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	prompt := promptTree.Nodes["feature.hello"]
	contentHash := document.ContentHash(prompt)
	effectiveHash := promptTree.EffectiveHashes()["feature.hello"]

	err = ApplyTaskResult(root, TaskResultUpdate{
		PromptID:      "feature.hello",
		Result:        "passed",
		ContentHash:   contentHash,
		EffectiveHash: effectiveHash,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	prompt = updated.Nodes["feature.hello"]
	if prompt.State.Execution != "passed" {
		t.Fatalf("execution = %q", prompt.State.Execution)
	}
	if prompt.State.Sync != "in_sync" {
		t.Fatalf("sync = %q", prompt.State.Sync)
	}
	if prompt.Hash.Satisfied != effectiveHash {
		t.Fatalf("satisfied hash = %q, want %q", prompt.Hash.Satisfied, effectiveHash)
	}
	if dirty := updated.DirtyNodes(); len(dirty) != 0 {
		t.Fatalf("dirty = %#v", dirty)
	}
}

func TestApplyTaskResultRejectsStalePlanHash(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writePrompt(t, root, project.RootID, rootPromptFixture)
	writePrompt(t, root, "feature.hello", helloPromptFixture)

	err := ApplyTaskResult(root, TaskResultUpdate{
		PromptID:      "feature.hello",
		Result:        "passed",
		ContentHash:   "sha256:stale",
		EffectiveHash: "sha256:stale",
	})
	if err == nil {
		t.Fatal("expected stale hash error")
	}
}

func TestStats(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writePrompt(t, root, project.RootID, rootPromptFixture)
	writePrompt(t, root, "architecture.draft", draftPromptFixture)
	writePrompt(t, root, "feature.waiting", waitingPromptFixture)
	writePrompt(t, root, "feature.hello", helloPromptFixture)
	writePrompt(t, root, "feature.hello.speak", childPromptFixture)

	promptTree, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	s := promptTree.Stats()
	if s.Nodes != 5 {
		t.Fatalf("nodes = %d, want 5", s.Nodes)
	}
	if s.MaxDepth != 2 {
		t.Fatalf("max_depth = %d, want 2", s.MaxDepth)
	}
	if s.DepthCount[0] != 1 || s.DepthCount[1] != 3 || s.DepthCount[2] != 1 {
		t.Fatalf("depth_count = %v, want {0:1, 1:3, 2:1}", s.DepthCount)
	}
	if s.Dirty != 4 || s.InSync != 0 {
		t.Fatalf("dirty/in_sync = %d/%d, want 4/0", s.Dirty, s.InSync)
	}
	if s.BodyBytes <= 0 {
		t.Fatalf("body_bytes = %d, want > 0", s.BodyBytes)
	}

	// After a passing result, the node moves from dirty to in_sync.
	hello := promptTree.Nodes["feature.hello"]
	if err := ApplyTaskResult(root, TaskResultUpdate{
		PromptID:      "feature.hello",
		Result:        "passed",
		ContentHash:   document.ContentHash(hello),
		EffectiveHash: promptTree.EffectiveHashes()["feature.hello"],
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	after := updated.Stats()
	if after.Dirty != 3 || after.InSync != 1 {
		t.Fatalf("after pass dirty/in_sync = %d/%d, want 3/1", after.Dirty, after.InSync)
	}
}

func TestExecutionOrderRequiresApprovalAndReadyDependencies(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writePrompt(t, root, project.RootID, rootPromptFixture)
	writePrompt(t, root, "architecture.draft", draftPromptFixture)
	writePrompt(t, root, "feature.waiting", waitingPromptFixture)
	writePrompt(t, root, "feature.hello", helloPromptFixture)

	promptTree, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if issues := promptTree.Validate(); len(issues) != 0 {
		t.Fatalf("tree issues: %v", issues)
	}
	if dirty := promptTree.DirtyNodes(); len(dirty) != 3 {
		t.Fatalf("dirty = %#v, want all three changed prompts", dirty)
	}
	order := promptTree.ExecutionOrder()
	if len(order) != 1 || order[0] != "feature.hello" {
		t.Fatalf("execution order = %#v, want only approved prompt with ready dependencies", order)
	}
}

func writePrompt(t *testing.T, root, id, body string) {
	t.Helper()
	path, err := project.NodePathForID(project.NewPaths(root).TreeDir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.WriteFileAtomic(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const rootPromptFixture = `---
schema: ebo.prompt/v1
id: project.root
title: Project Root
kind: project
parent:
state:
  spec: approved
  execution: adopted
  sync: in_sync
links:
  references: []
---
## Intent
Root.
`

const helloPromptFixture = `---
schema: ebo.prompt/v1
id: feature.hello
title: Hello
kind: feature
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent
Say hello.
`

const draftPromptFixture = `---
schema: ebo.prompt/v1
id: architecture.draft
title: Draft Architecture
kind: architecture
parent: project.root
state:
  spec: draft
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent
Remain pending until a human approves it.
`

const waitingPromptFixture = `---
schema: ebo.prompt/v1
id: feature.waiting
title: Waiting Feature
kind: feature
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  depends_on:
    - id: architecture.draft
      reason: the feature requires the draft architecture
---
## Intent
Wait for the dependency to be approved.
`

const childPromptFixture = `---
schema: ebo.prompt/v1
id: feature.hello.speak
title: Speak
kind: task
parent: feature.hello
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent
Nested below feature.hello to exercise depth counting.
`
