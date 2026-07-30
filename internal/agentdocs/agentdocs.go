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

Ebo keeps project intent isolated under .ebo/. The canonical, version-controlled Prompt Tree is .ebo/tree/. Do not store prompts beside source code and do not edit .ebo/tree/ directly.

When generating Prompt Markdown:
1. Run ebo context <node-id>, ebo tree search, or ebo tree graph before drafting.
2. Write generated Markdown outside .ebo/tree/.
3. Every non-root prompt must declare exactly one parent.
4. Identify depends_on, affects, implements, references, and supersedes links, and include a reason for each semantic link.
5. Do not reference unknown prompt IDs. Keep uncertainty in Open Questions instead.
6. Give the Markdown to the user, or run ebo add --file <path> only when the user explicitly asks.
7. Never run ebo approve or ebo apply. Those commands require human review and approval.

When executing code:
1. Ebo decides execution eligibility. Never browse or load every file in .ebo/tree/ to choose work yourself.
2. Run ebo status and ebo scan at the start of the task, then run ebo next to open or continue the active execution plan.
3. Ignore project.root, prompts whose spec state is not approved, prompts whose satisfied hash still matches their effective hash, and prompts waiting on an unready dependency. Failed or blocked prompts remain eligible for retry.
4. Treat only the single task package returned by ebo next as executable. If ebo next returns no task, stop without loading Prompt files.
5. After ebo next selects a task, run ebo context <prompt-id> --depth 0 before changing code. This loads the selected Prompt and its direct semantic links without expanding unrelated tree branches. Increase depth only when the task explicitly requires broader hierarchy context.
6. Implement that one task package and follow its acceptance criteria. Do not preload unrelated branches of the Prompt Tree.
7. Report the result with the exact ebo report command from the task package, then run ebo verify <plan-id>.
8. Do not forge prompt state or hashes by editing .ebo/tree/.
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
