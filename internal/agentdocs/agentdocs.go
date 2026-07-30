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
此区块由 Ebo 管理。

本项目使用 Ebo 管理 Prompt 树。在规划、生成 Prompt 或修改代码之前，必须先完整阅读并遵守 .ebo/WORKFLOW.md。
EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT.
ebo scan 仅用于查看，永远不授予代码修改权限。只有 ebo next 返回 EBO EXECUTION GATE: OPEN 时才允许修改业务代码，并且只能执行它返回的单个任务。
修改业务代码前必须运行 ebo guard check；只有同时返回 guard: pass 和 EBO EXECUTION GATE: OPEN 才能继续。
如果 ebo next 返回 CLOSED 或没有任务，禁止修改业务代码。用户提出新的代码需求时，只能生成 Prompt 草稿、运行 ebo add 创建 proposal，然后停止等待人工审批。
不得遍历整个 .ebo/tree/ 自行选择任务，不得手工修改 .ebo/tree、.ebo/plans、.ebo/receipts 或 .ebo/runtime 中的控制状态与哈希。
ebo approve 和 ebo apply 属于人工审批步骤，Agent 不得执行。
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
