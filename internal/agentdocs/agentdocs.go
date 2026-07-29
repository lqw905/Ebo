package agentdocs

import (
	"fmt"
	"os"
	"strings"

	"github.com/lqw905/Ebo/internal/project"
)

const (
	startMarker = "<!-- EBO:START -->"
	endMarker   = "<!-- EBO:END -->"
)

const ManagedBlock = `<!-- EBO:START -->
This block is managed by Ebo.

Ebo manages the project's only Prompt Tree. Do not edit .ebo/tree/ directly.

When generating Prompt Markdown:
1. Run ebo context <node-id>, ebo tree search, or ebo tree graph before drafting.
2. Write generated Markdown outside .ebo/tree/.
3. Every non-root prompt must declare exactly one parent.
4. Identify depends_on, affects, implements, references, and supersedes links, and include a reason for each semantic link.
5. Do not reference unknown prompt IDs. Keep uncertainty in Open Questions instead.
6. Run ebo add only when the user explicitly asks. Never run ebo approve or ebo apply.

When executing code:
1. Run ebo status, ebo scan, and ebo next.
2. Implement only the task package and acceptance criteria Ebo provides.
3. Report results with ebo report. Do not forge prompt state by editing .ebo/tree/.
<!-- EBO:END -->
`

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
		return "", fmt.Errorf("%s has malformed Ebo managed block", path)
	}
	end += len(endMarker)
	next := text[:start] + strings.TrimRight(ManagedBlock, "\n") + text[end:]
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if err := project.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
		return "", err
	}
	return "updated", nil
}
