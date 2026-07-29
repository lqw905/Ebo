package document

import (
	"strings"
	"testing"
)

func TestParsePromptWithLinks(t *testing.T) {
	prompt, err := ParsePrompt([]byte(`---
schema: ebo.prompt/v1
id: feature.login
title: Login
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
  depends_on:
    - id: architecture.identity
      reason: login needs identity boundaries
  references: []
---
## Intent

Let users sign in.
`), "login.md")
	if err != nil {
		t.Fatal(err)
	}
	if prompt.ID != "feature.login" {
		t.Fatalf("ID = %q", prompt.ID)
	}
	if got := prompt.Links["depends_on"][0].Reason; !strings.Contains(got, "identity") {
		t.Fatalf("reason = %q", got)
	}
	if issues := ValidateBasic(prompt); len(issues) != 0 {
		t.Fatalf("unexpected validation issues: %v", issues)
	}
	if hash := ContentHash(prompt); !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("content hash = %q", hash)
	}
}

func TestValidateBasicRequiresSemanticReason(t *testing.T) {
	prompt, err := ParsePrompt([]byte(`---
schema: ebo.prompt/v1
id: feature.login
title: Login
kind: feature
parent: project.root
links:
  depends_on:
    - id: architecture.identity
---
## Intent
`), "login.md")
	if err != nil {
		t.Fatal(err)
	}
	issues := ValidateBasic(prompt)
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
	if !strings.Contains(strings.Join(issues, "\n"), "requires reason") {
		t.Fatalf("issues = %v", issues)
	}
}

func TestRenderPromptRoundTrip(t *testing.T) {
	prompt, err := ParsePrompt([]byte(`---
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
`), "hello.md")
	if err != nil {
		t.Fatal(err)
	}
	prompt.State.Execution = "passed"
	prompt.Hash.Satisfied = "sha256:abc"

	rendered := RenderPrompt(prompt)
	parsed, err := ParsePrompt(rendered, "rendered.md")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.State.Execution != "passed" {
		t.Fatalf("execution = %q", parsed.State.Execution)
	}
	if parsed.Hash.Satisfied != "sha256:abc" {
		t.Fatalf("satisfied = %q", parsed.Hash.Satisfied)
	}
	if !strings.Contains(parsed.Body, "Say hello.") {
		t.Fatalf("body = %q", parsed.Body)
	}
}
