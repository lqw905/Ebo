package workflowdocs

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/lqw905/Ebo/internal/project"
)

const (
	Filename    = "WORKFLOW.md"
	startMarker = "<!-- EBO:WORKFLOW:START -->"
	endMarker   = "<!-- EBO:WORKFLOW:END -->"
)

//go:embed WORKFLOW.zh-CN.md
var ManagedBlock string

func Update(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := project.WriteFileAtomic(path, []byte(ManagedBlock), 0o644); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}

	text := string(data)
	start := strings.Index(text, startMarker)
	end := strings.Index(text, endMarker)
	if start >= 0 && end < 0 {
		return "", fmt.Errorf("%s contains %s without %s", path, startMarker, endMarker)
	}
	if start < 0 {
		next := text
		if len(next) > 0 && !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += "\n" + ManagedBlock
		if err := project.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
			return "", err
		}
		return "appended", nil
	}
	if end < start {
		return "", fmt.Errorf("%s has malformed Ebo workflow block", path)
	}
	end += len(endMarker)
	next := text[:start] + strings.TrimRight(ManagedBlock, "\n") + text[end:]
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if next == text {
		return "unchanged", nil
	}
	if err := project.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
		return "", err
	}
	return "updated", nil
}

func IsManaged(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, startMarker) && strings.Contains(text, endMarker)
}
