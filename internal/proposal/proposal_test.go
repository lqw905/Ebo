package proposal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lqw905/Ebo/internal/project"
	"github.com/lqw905/Ebo/internal/tree"
)

func TestProposalApproveApplyLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	rootPath, err := project.NodePathForID(project.NewPaths(root).TreeDir, project.RootID)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.WriteFileAtomic(rootPath, []byte(rootPromptFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := Create(root, []Source{{
		Kind: "file",
		Path: "drafts/hello.md",
		Name: "hello.md",
		Data: []byte(`---
schema: ebo.prompt/v1
id: feature.hello
title: Hello
kind: feature
parent: project.root
revision: 1
origin: human
confidence: confirmed
state:
  spec: draft
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent

Say hello.
`),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != "proposed" {
		t.Fatalf("status = %q", meta.Status)
	}
	if _, err := Approve(root, meta.ID, meta.ProposalHash); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, meta.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := tree.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if issues := loaded.Validate(); len(issues) != 0 {
		t.Fatalf("tree issues: %v", issues)
	}
	if loaded.Nodes["feature.hello"] == nil {
		t.Fatal("feature.hello was not applied")
	}
	prompt := loaded.Nodes["feature.hello"]
	if prompt.State.Spec != "approved" {
		t.Fatalf("spec = %q, want approved", prompt.State.Spec)
	}
	if prompt.State.Execution != "not_started" || prompt.State.Sync != "dirty" {
		t.Fatalf("execution state = %q/%q, want not_started/dirty", prompt.State.Execution, prompt.State.Sync)
	}
	if prompt.Hash.Satisfied != "" {
		t.Fatalf("satisfied hash = %q, want empty", prompt.Hash.Satisfied)
	}
	target := filepath.Join(project.NewPaths(root).TreeDir, "feature", "hello.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected applied prompt at %s: %v", target, err)
	}
}

const rootPromptFixture = `---
schema: ebo.prompt/v1
id: project.root
title: Project Root
kind: project
parent:
revision: 1
origin: human
confidence: confirmed
state:
  spec: approved
  execution: adopted
  sync: in_sync
hash:
  satisfied:
links:
  references: []
---
## Intent

Root.
`
