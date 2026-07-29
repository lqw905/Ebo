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
