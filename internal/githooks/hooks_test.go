package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPreservesExistingShellHook(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}
	path, err := Path(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const existing = "#!/bin/sh\necho existing\n"
	if err := os.WriteFile(path, []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	action, err := Install(root, `C:\Program Files\Ebo\ebo.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if action != "installed" {
		t.Fatalf("action = %q, want installed", action)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"echo existing", startMarker, `'C:/Program Files/Ebo/ebo.exe' guard check --staged`, endMarker} {
		if !strings.Contains(text, want) {
			t.Fatalf("hook does not contain %q:\n%s", want, text)
		}
	}
	if !Installed(root) {
		t.Fatal("hook should be recognized as installed")
	}
	if action, err := Install(root, `C:\Program Files\Ebo\ebo.exe`); err != nil || action != "unchanged" {
		t.Fatalf("second install = %q, %v, want unchanged", action, err)
	}
}
