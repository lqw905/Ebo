package execution

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/lqw905/Ebo/internal/project"
)

func TestActiveTaskLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	active := New("plan-1", "task-1", "feature.login", "content", "effective", "commit")
	if err := Save(root, active); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PlanID != active.PlanID || loaded.TaskID != active.TaskID || loaded.Schema != Schema {
		t.Fatalf("loaded active task = %#v", loaded)
	}
	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); !errors.Is(err, ErrNoActiveTask) {
		t.Fatalf("load after clear = %v, want ErrNoActiveTask", err)
	}
}

func TestLoadRejectsIncompleteActiveTask(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project.NewPaths(root).ActiveTask, []byte(`{"schema":"ebo.active-task/v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Load error = %v, want missing field error", err)
	}
}
