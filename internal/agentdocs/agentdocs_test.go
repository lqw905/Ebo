package agentdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCreatesExecutionProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	action, err := Update(path)
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
		"只有同时返回 guard: pass 和 EBO EXECUTION GATE: OPEN",
		"只能生成 Prompt 草稿、运行 ebo add 创建 proposal",
		"不得遍历整个 .ebo/tree/",
		"Agent 不得执行",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed block does not contain %q:\n%s", want, text)
		}
	}
}

func TestUpdatePreservesUserContentAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	const userContent = "# Project instructions\n\nKeep this line.\n"
	if err := os.WriteFile(path, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err := Update(path); err != nil || action != "appended" {
		t.Fatalf("first update = %q, %v", action, err)
	}
	if action, err := Update(path); err != nil || action != "updated" {
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
