package agentdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lqw905/Ebo/internal/project"
)

func TestUpdateCreatesExecutionProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	action, err := Update(path, project.ModeStrict)
	if err != nil {
		t.Fatal(err)
	}
	if action != "created" {
		t.Fatalf("action = %q, want created", action)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"此区块由 Ebo 管理",
		"必须先完整阅读并遵守 .ebo/WORKFLOW.md",
		"EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT",
		"ebo scan 仅用于查看，永远不授予代码修改权限",
		"只有 ebo next 返回 EBO EXECUTION GATE: OPEN",
		"ebo hook pre-write --path <目标文件>",
		"EBO PRE-WRITE: ALLOWED",
		"只能生成 Prompt 草稿、运行 ebo add 创建 proposal",
		"不得遍历整个 .ebo/tree/",
		"Agent 不得执行",
		"默认只校订用户原话",
		"创作发挥",
		"--request",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed block does not contain %q:\n%s", want, text)
		}
	}
}

func TestSilentBlockDirectsAgentToSelfApprove(t *testing.T) {
	block := Block(project.ModeSilent)
	for _, want := range []string{
		"此区块由 Ebo 管理",
		"EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT",
		"静默模式",
		"依次运行 ebo add、ebo approve、ebo apply、ebo next 和 ebo report",
		"静默模式下 ebo approve 自动通过",
		"静默模式不强制提交",
		"每个计划完成后也不会自动提交",
		"用户想什么时候提交就什么时候提交",
		"只发生在 report 完成后、门禁关闭时",
		"git add -A 与 git commit",
		"默认只校订用户原话",
		"创作发挥",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("silent block does not contain %q:\n%s", want, block)
		}
	}
	for _, banned := range []string{"Agent 不得执行", "停止等待人工审批"} {
		if strings.Contains(block, banned) {
			t.Fatalf("silent block must not contain %q:\n%s", banned, block)
		}
	}
	if strings.Contains(Block(project.ModeStrict), "静默模式") {
		t.Fatalf("strict block must not mention silent mode:\n%s", Block(project.ModeStrict))
	}
}

func TestUpdatePreservesUserContentAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	const userContent = "# Project instructions\n\nKeep this line.\n"
	if err := os.WriteFile(path, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err := Update(path, project.ModeStrict); err != nil || action != "appended" {
		t.Fatalf("first update = %q, %v", action, err)
	}
	if action, err := Update(path, project.ModeStrict); err != nil || action != "updated" {
		t.Fatalf("second update = %q, %v", action, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, userContent) {
		t.Fatalf("user content was not preserved:\n%s", text)
	}
	if strings.Count(text, startMarker) != 1 || strings.Count(text, endMarker) != 1 {
		t.Fatalf("managed block was duplicated:\n%s", text)
	}
}
