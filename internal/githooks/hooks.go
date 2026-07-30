package githooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lqw905/Ebo/internal/gitx"
	"github.com/lqw905/Ebo/internal/project"
)

const (
	startMarker = "# EBO:HOOK:START"
	endMarker   = "# EBO:HOOK:END"
)

func Install(root, executable string) (string, error) {
	path, err := Path(root)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	text := string(data)
	if text != "" && !strings.HasPrefix(text, "#!") {
		return "", fmt.Errorf("existing pre-commit hook is not a shell script: %s", path)
	}
	block := managedBlock(executable)
	start := strings.Index(text, startMarker)
	end := strings.Index(text, endMarker)
	action := "installed"
	var next string
	switch {
	case start >= 0 && end < 0:
		return "", fmt.Errorf("existing pre-commit hook has an incomplete Ebo block")
	case start >= 0 && end >= start:
		end += len(endMarker)
		next = text[:start] + strings.TrimRight(block, "\n") + text[end:]
		action = "updated"
	default:
		next = text
		if next == "" {
			next = "#!/bin/sh\n\n"
		} else if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += "\n" + block
	}
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if next == text {
		return "unchanged", nil
	}
	if err := project.WriteFileAtomic(path, []byte(next), 0o755); err != nil {
		return "", err
	}
	return action, nil
}

func Installed(root string) bool {
	path, err := Path(root)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, startMarker) && strings.Contains(text, endMarker)
}

func Path(root string) (string, error) {
	path, err := gitx.GitPath(root, "hooks/pre-commit")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Clean(path), nil
}

func managedBlock(executable string) string {
	command := strings.ReplaceAll(executable, "\\", "/")
	command = strings.ReplaceAll(command, "'", "'\\''")
	return fmt.Sprintf("%s\n# Blocks staged source changes that are not backed by a completed Ebo plan.\n'%s' guard check --staged || exit $?\n%s\n", startMarker, command, endMarker)
}
