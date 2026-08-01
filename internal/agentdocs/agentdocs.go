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

const strictBlock = `<!-- EBO:START -->
此区块由 Ebo 管理。

本项目使用 Ebo 管理 Prompt 树。在规划、生成 Prompt 或修改代码之前，必须先完整阅读并遵守 .ebo/WORKFLOW.md。
EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT.
ebo scan 仅用于查看，永远不授予代码修改权限。只有 ebo next 返回 EBO EXECUTION GATE: OPEN 时才允许修改业务代码，并且只能执行它返回的单个任务。
每次调用 Write、Edit、Patch 或其他文件写入工具前，必须先运行 ebo hook pre-write --path <目标文件>；只有命令退出码为 0 且返回 EBO PRE-WRITE: ALLOWED 才能写入。
drafts/**/*.md 可以在 CLOSED 状态下用于生成 proposal 草稿，但这不授予业务源码修改权限。
如果 ebo next 返回 CLOSED 或没有任务，禁止修改业务代码。用户提出新的代码需求时，只能生成 Prompt 草稿、运行 ebo add 创建 proposal，然后停止等待人工审批。
生成 Prompt 时默认只校订用户原话（错别字、语序、不通顺），不得增删语义或补充用户没说的内容；只有用户明确要求“生成/优化 prompt 或设计文档”时才允许创作发挥，且必须用 ebo add --request 记录用户原要求，供 review 对照核验。
不得遍历整个 .ebo/tree/ 自行选择任务，不得手工修改 .ebo/tree、.ebo/plans、.ebo/receipts 或 .ebo/runtime 中的控制状态与哈希。
ebo approve 和 ebo apply 属于人工审批步骤，Agent 不得执行。
<!-- EBO:END -->
`

const silentBlock = `<!-- EBO:START -->
此区块由 Ebo 管理。

本项目使用 Ebo 管理 Prompt 树。在规划、生成 Prompt 或修改代码之前，必须先完整阅读并遵守 .ebo/WORKFLOW.md。
EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT.
ebo scan 仅用于查看，永远不授予代码修改权限。只有 ebo next 返回 EBO EXECUTION GATE: OPEN 时才允许修改业务代码，并且只能执行它返回的单个任务。
每次调用 Write、Edit、Patch 或其他文件写入工具前，必须先运行 ebo hook pre-write --path <目标文件>；只有命令退出码为 0 且返回 EBO PRE-WRITE: ALLOWED 才能写入。
drafts/**/*.md 可以在 CLOSED 状态下用于生成 proposal 草稿，但这不授予业务源码修改权限。
当前项目为静默模式：用户提出新的代码需求时，Agent 应生成 Prompt 草稿并依次运行 ebo add、ebo approve、ebo apply、ebo next 和 ebo report，不需要请求用户逐条批准。
静默模式下 ebo approve 自动通过，不再要求交互确认；Agent 仍必须遵守哈希校验、scope 与执行门禁。
静默模式不强制提交。仅当用户明确要求时才提交，且只发生在 report 完成后、门禁关闭时；任务中途提交会使当前活动任务失效。用户要求提交时，直接运行 git add -A 与 git commit，.ebo 记录会一并提交。
生成 Prompt 时默认只校订用户原话（错别字、语序、不通顺），不得增删语义或补充用户没说的内容；只有用户明确要求“生成/优化 prompt 或设计文档”时才允许创作发挥，且必须用 ebo add --request 记录用户原要求，供 review 对照核验。
如果 ebo next 返回 CLOSED 或没有任务，禁止修改业务代码。
不得遍历整个 .ebo/tree/ 自行选择任务，不得手工修改 .ebo/tree、.ebo/plans、.ebo/receipts 或 .ebo/runtime 中的控制状态与哈希。
<!-- EBO:END -->
`

// Block returns the managed agent instruction block for the given project mode.
func Block(mode string) string {
	if mode == project.ModeSilent {
		return silentBlock
	}
	return strictBlock
}

func Update(path, mode string) (string, error) {
	block := Block(mode)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := project.WriteFileAtomic(path, []byte(block), 0o644); err != nil {
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
		next += "\n" + block
		if err := project.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
			return "", err
		}
		return "appended", nil
	}
	if end < start {
		return "", fmt.Errorf("%s has malformed Ebo managed block", path)
	}
	end += len(endMarker)
	next := text[:start] + strings.TrimRight(block, "\n") + text[end:]
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if err := project.WriteFileAtomic(path, []byte(next), 0o644); err != nil {
		return "", err
	}
	return "updated", nil
}
