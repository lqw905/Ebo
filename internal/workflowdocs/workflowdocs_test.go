package workflowdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lqw905/Ebo/internal/project"
)

func TestUpdateCreatesChineseWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
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
		"# Ebo 项目使用说明",
		"EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT",
		"每个 Prompt Markdown 都必须",
		"输入给 Agent",
		"只有 `ebo next` 输出 `EBO EXECUTION GATE: OPEN`",
		"ebo guard check",
		"ebo hook pre-write --path <目标文件>",
		"退出码 `0` 表示本次写入允许",
		"ebo hooks install codex",
		"打开 `/hooks`",
		"scope.allow",
		"ebo commit` 本身也会执行同等校验",
		"ebo context <prompt-id> --depth 0",
		"## 5. 将对话沉淀为 Prompt 树",
		"直接提出新的代码变更，而当前没有 OPEN 任务",
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

func TestSilentModeAddsBanner(t *testing.T) {
	strict := Block(project.ModeStrict)
	silent := Block(project.ModeSilent)
	if strings.Contains(strict, "当前模式：静默") {
		t.Fatalf("strict workflow must not contain silent banner:\n%s", strict)
	}
	for _, want := range []string{
		"当前模式：静默（silent）",
		"依次运行 ebo add、ebo approve、ebo apply、ebo next 和 ebo report",
		"改为 Agent 自动执行",
		"提交时机：静默模式不强制提交",
		"只发生在 report 完成后、门禁关闭时",
		"普通 git add -A 与 git commit",
	} {
		if !strings.Contains(silent, want) {
			t.Fatalf("silent workflow does not contain %q:\n%s", want, silent)
		}
	}
	if strings.Contains(silent, "{{MODE_BANNER}}") {
		t.Fatalf("silent workflow leaked the mode marker:\n%s", silent)
	}
}

func TestUpdatePreservesContentOutsideManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	const prefix = "# 项目补充规则\n\n必须保留。\n\n"
	const suffix = "\n\n## 团队约定\n\n也必须保留。\n"
	initial := prefix + strings.TrimRight(Block(project.ModeStrict), "\n") + suffix
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	action, err := Update(path, project.ModeStrict)
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
