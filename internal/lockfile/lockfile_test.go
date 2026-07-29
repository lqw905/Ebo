package lockfile

import (
	"testing"

	"github.com/lqw905/Ebo/internal/project"
)

func TestAcquireRejectsConcurrentLock(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(root, "first")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	if _, err := Acquire(root, "second"); err == nil {
		t.Fatal("expected concurrent lock error")
	}
	info, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Command != "first" {
		t.Fatalf("command = %q", info.Command)
	}
}

func TestReleaseAllowsReacquire(t *testing.T) {
	root := t.TempDir()
	if err := project.EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(root, "first")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(root, "second")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
}
