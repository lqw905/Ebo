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
		Data: []byte(helloPromptFixture),
	}}, "实现登录", false)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != "proposed" {
		t.Fatalf("status = %q", meta.Status)
	}
	approved, err := Approve(root, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.ApprovedHash != meta.ProposalHash {
		t.Fatalf("approved hash = %q, want %q", approved.ApprovedHash, meta.ProposalHash)
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
	proposalDir := filepath.Join(project.NewPaths(root).ProposalsDir, meta.ID)
	if _, err := os.Stat(proposalDir); !os.IsNotExist(err) {
		t.Fatalf("proposal directory must be removed after apply: %v", err)
	}
}

func TestProposalBindsUserRequest(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	meta, err := Create(root, []Source{{
		Kind: "file",
		Path: "drafts/hello.md",
		Name: "hello.md",
		Data: []byte(helloPromptFixture),
	}}, "实现登录", false)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Request != "实现登录" {
		t.Fatalf("request = %q, want 实现登录", meta.Request)
	}
	loaded, err := Load(root, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Request != "实现登录" {
		t.Fatalf("stored request = %q", loaded.Request)
	}
	// Tampering with the stored request must invalidate the proposal hash.
	loaded.Request = "改成别的要求"
	if err := Save(root, loaded); err != nil {
		t.Fatal(err)
	}
	actual, err := RecomputeHash(root, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if actual == loaded.ProposalHash {
		t.Fatal("changed request must change the proposal hash")
	}
	if _, err := Approve(root, meta.ID); err == nil {
		t.Fatal("expected approve to reject a tampered request")
	}
}

func TestApproveRejectsChangedProposalContent(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	meta, err := Create(root, []Source{{
		Kind: "file",
		Path: "drafts/hello.md",
		Name: "hello.md",
		Data: []byte(helloPromptFixture),
	}}, "实现登录", false)
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(project.NewPaths(root).ProposalsDir, meta.ID, filepath.FromSlash(meta.Sources[0].StoredPath))
	if err := os.WriteFile(stored, []byte(rootPromptFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(root, meta.ID); err == nil {
		t.Fatal("expected changed proposal content to be rejected")
	}
}

func TestApplyRejectsContentChangedAfterApproval(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	meta, err := Create(root, []Source{{
		Kind: "file",
		Path: "drafts/hello.md",
		Name: "hello.md",
		Data: []byte(helloPromptFixture),
	}}, "实现登录", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(root, meta.ID); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(project.NewPaths(root).ProposalsDir, meta.ID, filepath.FromSlash(meta.Sources[0].StoredPath))
	if err := os.WriteFile(stored, []byte(rootPromptFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, meta.ID); err == nil {
		t.Fatal("expected changed approved content to be rejected during apply")
	}
	if _, err := os.Stat(filepath.Join(project.NewPaths(root).ProposalsDir, meta.ID)); err != nil {
		t.Fatalf("failed apply must leave the proposal in place: %v", err)
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

const helloPromptFixture = `---
schema: ebo.prompt/v1
id: feature.hello
title: Hello
kind: feature
parent: project.root
state:
  spec: draft
  execution: not_started
  sync: dirty
links:
  references: []
---
## Intent
Say hello.
`
