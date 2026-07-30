package agentdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCreatesExecutionProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	action, err := Update(path)
	if err != nil {
		t.Fatal(err)
	}
	if action != "created" {
		t.Fatalf("action = %q, want created", action)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"canonical, version-controlled Prompt Tree is .ebo/tree/",
		"Ebo decides execution eligibility",
		"Never browse or load every file in .ebo/tree/",
		"ebo next",
		"only the single task package returned by ebo next as executable",
		"ebo context <prompt-id> --depth 0",
		"Increase depth only when the task explicitly requires",
		"spec state is not approved",
		"satisfied hash still matches their effective hash",
		"waiting on an unready dependency",
		"Failed or blocked prompts remain eligible for retry",
		"Do not preload unrelated branches",
		"ebo verify <plan-id>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed block does not contain %q:\n%s", want, text)
		}
	}
}

func TestUpdatePreservesUserContentAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	const userContent = "# Project instructions\n\nKeep this line.\n"
	if err := os.WriteFile(path, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err := Update(path); err != nil || action != "appended" {
		t.Fatalf("first update = %q, %v", action, err)
	}
	if action, err := Update(path); err != nil || action != "updated" {
		t.Fatalf("second update = %q, %v", action, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, userContent) {
		t.Fatalf("user content was not preserved:\n%s", text)
	}
	if strings.Count(text, startMarker) != 1 || strings.Count(text, endMarker) != 1 {
		t.Fatalf("managed block was duplicated:\n%s", text)
	}
}
