package planner

import (
	"testing"

	"github.com/lqw905/Ebo/internal/project"
	"github.com/lqw905/Ebo/internal/tree"
)

func TestCreatePlanOrdersDependenciesFirst(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writePrompt(t, root, project.RootID, rootPrompt)
	writePrompt(t, root, "architecture.identity", identityPrompt)
	writePrompt(t, root, "feature.login", loginPrompt)

	promptTree, err := tree.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Create(root, promptTree, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Order) != 2 {
		t.Fatalf("order length = %d, want 2: %#v", len(plan.Order), plan.Order)
	}
	if plan.Order[0] != "architecture.identity" || plan.Order[1] != "feature.login" {
		t.Fatalf("order = %#v", plan.Order)
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

const rootPrompt = `---
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

const identityPrompt = `---
schema: ebo.prompt/v1
id: architecture.identity
title: Identity Architecture
kind: architecture
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent
Define identity boundaries.
`

const loginPrompt = `---
schema: ebo.prompt/v1
id: feature.login
title: Login
kind: feature
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  depends_on:
    - id: architecture.identity
      reason: login follows identity boundaries
---
## Intent
Let users sign in.
`
