package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DirName    = ".ebo"
	ConfigName = "config.toml"
	RootID     = "project.root"
)

var ErrNotInitialized = errors.New("ebo project is not initialized")

type Paths struct {
	Root         string
	EboDir       string
	ConfigFile   string
	WorkflowFile string
	TreeDir      string
	ProposalsDir string
	PlansDir     string
	ReceiptsDir  string
	RuntimeDir   string
	ActiveTask   string
	CacheDir     string
	LocksDir     string
	TmpDir       string
}

func NewPaths(root string) Paths {
	ebo := filepath.Join(root, DirName)
	return Paths{
		Root:         root,
		EboDir:       ebo,
		ConfigFile:   filepath.Join(ebo, ConfigName),
		WorkflowFile: filepath.Join(ebo, "WORKFLOW.md"),
		TreeDir:      filepath.Join(ebo, "tree"),
		ProposalsDir: filepath.Join(ebo, "proposals"),
		PlansDir:     filepath.Join(ebo, "plans"),
		ReceiptsDir:  filepath.Join(ebo, "receipts"),
		RuntimeDir:   filepath.Join(ebo, "runtime"),
		ActiveTask:   filepath.Join(ebo, "runtime", "active-task.json"),
		CacheDir:     filepath.Join(ebo, "cache"),
		LocksDir:     filepath.Join(ebo, "locks"),
		TmpDir:       filepath.Join(ebo, "tmp"),
	}
}

func FindRoot(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(cur, DirName, ConfigName)) {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrNotInitialized
		}
		cur = parent
	}
}

func Initialized(root string) bool {
	return fileExists(filepath.Join(root, DirName, ConfigName))
}

func EnsureLayout(root string) error {
	paths := NewPaths(root)
	for _, dir := range []string{
		paths.EboDir,
		paths.TreeDir,
		paths.ProposalsDir,
		paths.PlansDir,
		paths.ReceiptsDir,
		paths.RuntimeDir,
		paths.CacheDir,
		paths.LocksDir,
		paths.TmpDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if !dirExists(src) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to copy symlink %s", path)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		dest, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dest, source); err != nil {
			_ = dest.Close()
			return err
		}
		return dest.Close()
	})
}

func ReplaceDir(target, candidate, backup string) error {
	if !dirExists(candidate) {
		return fmt.Errorf("candidate directory does not exist: %s", candidate)
	}
	moved := false
	if dirExists(target) {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		moved = true
	}
	if err := os.Rename(candidate, target); err != nil {
		if moved {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if moved {
		_ = os.RemoveAll(backup)
	}
	return nil
}

var idSegmentRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func NodePathForID(treeRoot, id string) (string, error) {
	if id == RootID {
		return filepath.Join(treeRoot, "project.md"), nil
	}
	parts := strings.Split(id, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("empty prompt id")
	}
	for _, part := range parts {
		if !idSegmentRE.MatchString(part) {
			return "", fmt.Errorf("invalid prompt id segment %q in %q", part, id)
		}
	}
	targetParts := append([]string{treeRoot}, parts...)
	return filepath.Join(targetParts...) + ".md", nil
}

func SafeFilename(name string) string {
	name = filepath.Base(name)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "prompt.md"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "prompt.md"
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
