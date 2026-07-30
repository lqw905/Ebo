package codexhooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreservesExistingHooksAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	existing := map[string]any{
		"description": "keep me",
		"hooks": map[string]any{
			"Stop": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo done"}}}},
		},
	}
	data, _ := json.Marshal(existing)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err := Install(path, `C:\Tools\ebo.exe`); err != nil || action != "updated" {
		t.Fatalf("Install = %q, %v", action, err)
	}
	if !Installed(path) {
		t.Fatal("Codex hook should be installed")
	}
	if action, err := Install(path, `C:\Tools\ebo.exe`); err != nil || action != "unchanged" {
		t.Fatalf("second Install = %q, %v", action, err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(updated, []byte("keep me")) || !bytes.Contains(updated, []byte("echo done")) || !bytes.Contains(updated, []byte(adapterCommand)) {
		t.Fatalf("existing hooks were not preserved:\n%s", updated)
	}
}
