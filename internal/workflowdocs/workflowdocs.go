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
	modeMarker  = "{{MODE_BANNER}}"
)

//go:embed WORKFLOW.zh-CN.md
var managedWorkflow string

// silentBanner is prepended to the workflow document in silent mode. It
// overrides every "stop and wait for human approval" instruction below, so an
// agent following the document in a silent project auto-runs the pipeline.
const silentBanner = "\n> **当前模式：静默（silent）。** 用户提出新的代码需求后，Agent 应自行依次运行 ebo add、ebo approve、ebo apply、ebo next 和 ebo report；本工作流中所有「停止等待人工审批」「Agent 不得运行 ebo approve / ebo apply」的说明，在静默模式下改为 Agent 自动执行。\n> 提交时机：静默模式不强制提交，每个计划完成后也不会自动提交。第 12 节的 ebo commit 流程与所有「计划完成后提交」的说明，在静默模式下都改为「仅在用户明确要求时才提交」；用户想什么时候提交就什么时候提交，可以在任意多个计划之间累积不提交。提交只发生在 report 完成后、门禁关闭时；任务中途提交会使活动任务失效。用户要求提交时使用普通 git add -A 与 git commit，.ebo 记录会随提交一并保存。"

// Block returns the managed workflow document for the given project mode. The
// strict document is unchanged; silent mode adds a banner that redefines the
// approval steps for the agent.
func Block(mode string) string {
	if mode == project.ModeSilent {
		return strings.Replace(managedWorkflow, modeMarker, silentBanner, 1)
	}
	return strings.Replace(managedWorkflow, modeMarker, "", 1)
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
		return "", fmt.Errorf("%s has malformed Ebo workflow block", path)
	}
	end += len(endMarker)
	next := text[:start] + strings.TrimRight(block, "\n") + text[end:]
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
