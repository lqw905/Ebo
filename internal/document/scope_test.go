package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptScopeRoundTripAndContentHash(t *testing.T) {
	data := []byte(`---
schema: ebo.prompt/v1
id: feature.login
title: Login
kind: feature
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
scope:
  allow:
    - internal/auth/**
    - cmd/*.go
  deny:
    - internal/auth/secrets/**
links:
  references: []
---
## Intent

Implement login.
`)
	prompt, err := ParsePrompt(data, "login.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Scope.Allow) != 2 || prompt.Scope.Allow[0] != "internal/auth/**" || len(prompt.Scope.Deny) != 1 {
		t.Fatalf("scope = %#v", prompt.Scope)
	}
	if issues := ValidateBasic(prompt); len(issues) > 0 {
		t.Fatalf("validation issues = %v", issues)
	}
	rendered := RenderPrompt(prompt)
	parsedAgain, err := ParsePrompt(rendered, "rendered.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(RenderPrompt(parsedAgain), rendered) {
		t.Fatalf("scope render is not stable:\n%s", rendered)
	}
	withScope := ContentHash(prompt)
	prompt.Scope = Scope{}
	withoutScope := ContentHash(prompt)
	if withScope == withoutScope {
		t.Fatal("scope must participate in the Prompt content hash")
	}
}

func TestPromptScopeRejectsOutsideTraversal(t *testing.T) {
	prompt := &Prompt{Source: "bad.md", Schema: PromptSchema, ID: "feature.bad", Title: "Bad", Kind: "feature", Scope: Scope{Allow: []string{"../secrets/**"}}}
	issues := ValidateBasic(prompt)
	if !strings.Contains(strings.Join(issues, "\n"), "cannot traverse outside") {
		t.Fatalf("validation issues = %v", issues)
	}
}
