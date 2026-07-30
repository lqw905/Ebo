package workflowdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCreatesChineseWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
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
		"# Ebo 项目使用说明",
		"每个 Prompt Markdown 都必须",
		"输入给 Agent",
		"只执行 ebo next 返回的单个任务",
		"ebo context <prompt-id> --depth 0",
		"## 5. 将对话沉淀为 Prompt 树",
		"只有用户明确要求",
		"ebo add --dry-run --dir drafts/ebo/<topic>/",
		"Agent 创建 proposal 后必须停止",
		"人工审阅并加入 Prompt 树",
		"反向抽象 Prompt 树",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow does not contain %q:\n%s", want, text)
		}
	}
	if !IsManaged(path) {
		t.Fatal("workflow should be recognized as managed")
	}
}

func TestUpdatePreservesContentOutsideManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	const prefix = "# 项目补充规则\n\n必须保留。\n\n"
	const suffix = "\n\n## 团队约定\n\n也必须保留。\n"
	initial := prefix + strings.TrimRight(ManagedBlock, "\n") + suffix
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	action, err := Update(path)
	if err != nil {
		t.Fatal(err)
	}
	if action != "unchanged" {
		t.Fatalf("action = %q, want unchanged", action)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
		t.Fatalf("content outside managed block was not preserved:\n%s", text)
	}
}
