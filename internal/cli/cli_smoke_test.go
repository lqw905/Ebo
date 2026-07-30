package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lqw905/Ebo/internal/project"
)

func TestCLISmoke(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	git := exec.Command("git", "init")
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}

	runCLI(t, nil, "init", "--agents", "none")
	writePrompt(t, root, project.RootID, rootPrompt)
	writePrompt(t, root, "architecture.identity", identityPrompt)
	writePrompt(t, root, "feature.login", loginPrompt)
	commitAll(t, root, "baseline")

	planOutput := runCLI(t, nil, "plan")
	planID := regexp.MustCompile(`created (plan-[^\s]+)`).FindStringSubmatch(planOutput)
	if len(planID) != 2 {
		t.Fatalf("could not parse plan id from:\n%s", planOutput)
	}
	exportOutput := runCLI(t, nil, "export", planID[1], "--format", "markdown")
	if strings.Contains(exportOutput, "source_edit: allowed") || !strings.Contains(exportOutput, "不授予源码修改权限") {
		t.Fatalf("export must not grant execution permission: %s", exportOutput)
	}
	deniedOutput, deniedCode := runCLIResult(nil, "hook", "pre-write", "--path", "internal/auth/service.go")
	if deniedCode != 1 || !strings.Contains(deniedOutput, "EBO PRE-WRITE: DENIED") || !strings.Contains(deniedOutput, "reason: no_active_task") {
		t.Fatalf("pre-write without active task should be denied, code=%d output=%s", deniedCode, deniedOutput)
	}
	codexDenied := runCLI(t, codexPreToolUseInput("internal/auth/service.go"), "hook", "codex-pre-tool-use")
	if !strings.Contains(codexDenied, `"permissionDecision":"deny"`) || !strings.Contains(codexDenied, "no_active_task") {
		t.Fatalf("Codex adapter should deny an unauthorized apply_patch: %s", codexDenied)
	}
	draftOutput := runCLI(t, nil, "hook", "pre-write", "--path", "drafts/ebo/login/prompt.md")
	if !strings.Contains(draftOutput, "mode: proposal_draft") || !strings.Contains(draftOutput, "source_edit: forbidden") {
		t.Fatalf("Prompt draft should be allowed without opening source gate: %s", draftOutput)
	}
	protectedOutput, protectedCode := runCLIResult(nil, "hook", "pre-write", "--path", ".ebo/tree/project.md")
	if protectedCode != 1 || !strings.Contains(protectedOutput, "reason: protected_control_path") {
		t.Fatalf("direct Ebo control edit should be denied, code=%d output=%s", protectedCode, protectedOutput)
	}
	preexisting := filepath.Join(root, "preexisting.txt")
	if err := os.WriteFile(preexisting, []byte("not authorized yet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	preexistingOutput, preexistingCode := runCLIResult(nil, "next", planID[1])
	if preexistingCode == 0 || !strings.Contains(preexistingOutput, "reason: preexisting_source_changes") {
		t.Fatalf("next must reject source edits made before authorization, code=%d output=%s", preexistingCode, preexistingOutput)
	}
	if err := os.Remove(preexisting); err != nil {
		t.Fatal(err)
	}

	nextOutput := runCLI(t, nil, "next", planID[1])
	if !strings.Contains(nextOutput, "architecture.identity") {
		t.Fatalf("next output = %s", nextOutput)
	}
	if !strings.Contains(nextOutput, "EBO EXECUTION GATE: OPEN") {
		t.Fatalf("next did not open execution gate: %s", nextOutput)
	}
	preWriteOutput := runCLI(t, nil, "hook", "pre-write", "--path", "internal/auth/service.go", "--json")
	if !strings.Contains(preWriteOutput, `"allowed":true`) || !strings.Contains(preWriteOutput, `"gate":"open"`) || !strings.Contains(preWriteOutput, `"path":"internal/auth/service.go"`) {
		t.Fatalf("active pre-write decision = %s", preWriteOutput)
	}
	codexAllowed := runCLI(t, codexPreToolUseInput("internal/auth/service.go"), "hook", "codex-pre-tool-use")
	if strings.TrimSpace(codexAllowed) != "" {
		t.Fatalf("Codex adapter should allow an authorized apply_patch without output: %s", codexAllowed)
	}
	outOfScopeOutput, outOfScopeCode := runCLIResult(nil, "hook", "pre-write", "--path", "internal/payment/service.go")
	if outOfScopeCode != 1 || !strings.Contains(outOfScopeOutput, "reason: path_outside_prompt_scope") {
		t.Fatalf("out-of-scope write should be denied, code=%d output=%s", outOfScopeCode, outOfScopeOutput)
	}
	activePath := filepath.Join(root, ".ebo", "runtime", "active-task.json")
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active task was not created: %v", err)
	}
	activeData, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activePath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	brokenOutput, brokenCode := runCLIResult(nil, "hook", "pre-write", "--path", "internal/auth/service.go")
	if brokenCode != 2 || !strings.Contains(brokenOutput, "reason: active_task_unreadable") {
		t.Fatalf("unreadable active task should be a hook configuration error, code=%d output=%s", brokenCode, brokenOutput)
	}
	if err := os.WriteFile(activePath, activeData, 0o644); err != nil {
		t.Fatal(err)
	}
	repeatedNext := runCLI(t, nil, "next")
	if !strings.Contains(repeatedNext, "architecture.identity") || !strings.Contains(repeatedNext, "EBO EXECUTION GATE: OPEN") {
		t.Fatalf("repeated next should resume the active task: %s", repeatedNext)
	}
	implementation := filepath.Join(root, "implementation.txt")
	if err := os.WriteFile(implementation, []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	openGuard := runCLI(t, nil, "guard", "check")
	if !strings.Contains(openGuard, "guard: pass") || !strings.Contains(openGuard, "EBO EXECUTION GATE: OPEN") {
		t.Fatalf("guard should allow source edits for active task: %s", openGuard)
	}

	runCLI(t, nil, "report", "architecture.identity", "--plan", planID[1], "--result", "failed", "--note", "retry smoke")
	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Fatalf("active task should be cleared after report, stat err=%v", err)
	}
	retryNext := runCLI(t, nil, "next", planID[1])
	if !strings.Contains(retryNext, "architecture.identity") || !strings.Contains(retryNext, "EBO EXECUTION GATE: OPEN") {
		t.Fatalf("failed task should reopen for retry: %s", retryNext)
	}
	runCLI(t, nil, "report", "architecture.identity", "--plan", planID[1], "--result", "passed", "--note", "retry passed")
	secondNext := runCLI(t, nil, "next", planID[1])
	if !strings.Contains(secondNext, "feature.login") || !strings.Contains(secondNext, "EBO EXECUTION GATE: OPEN") {
		t.Fatalf("second next output = %s", secondNext)
	}
	runCLI(t, nil, "report", "feature.login", "--plan", planID[1], "--result", "passed", "--note", "smoke")
	verifyOutput := runCLI(t, nil, "verify", planID[1])
	if !strings.Contains(verifyOutput, "status=completed") {
		t.Fatalf("verify output = %s", verifyOutput)
	}
	scanOutput := runCLI(t, nil, "scan")
	if !strings.Contains(scanOutput, "dirty tasks: 0") || !strings.Contains(scanOutput, "EBO EXECUTION GATE: CLOSED") {
		t.Fatalf("scan output = %s", scanOutput)
	}
	nextEmpty := runCLI(t, nil, "next")
	if !strings.Contains(nextEmpty, "reason: no_executable_task") || !strings.Contains(nextEmpty, "source_edit: forbidden") {
		t.Fatalf("empty next output = %s", nextEmpty)
	}

	guardOutput, guardError := runCLIResult(nil, "guard", "check")
	if guardError == 0 || !strings.Contains(guardOutput, "unauthorized_source_changes") {
		t.Fatalf("guard should reject source changes without active task, code=%d output=%s", guardError, guardOutput)
	}
	gitAddAll(t, root)
	stagedGuard := runCLI(t, nil, "guard", "check", "--staged")
	if !strings.Contains(stagedGuard, "guard: pass") || !strings.Contains(stagedGuard, planID[1]) {
		t.Fatalf("staged guard output = %s", stagedGuard)
	}
}

func TestInitIsolatesEboAndInstallsAgentWorkflow(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	const source = "package example\n"
	sourcePath := filepath.Join(root, "example.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	initOutput := runCLI(t, nil, "init", "--agents", "codex,claude")
	if !strings.Contains(initOutput, "created .ebo/WORKFLOW.md") {
		t.Fatalf("init output does not report workflow creation:\n%s", initOutput)
	}

	for _, path := range []string{
		filepath.Join(root, ".ebo", "config.toml"),
		filepath.Join(root, ".ebo", "WORKFLOW.md"),
		filepath.Join(root, ".ebo", "tree", "project.md"),
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "CLAUDE.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected initialized file %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != source {
		t.Fatalf("source file was changed: %q", data)
	}
	configData, err := os.ReadFile(filepath.Join(root, ".ebo", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), `workflow_file = ".ebo/WORKFLOW.md"`) {
		t.Fatalf("config does not declare workflow file:\n%s", configData)
	}
	gitignoreData, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignoreData), ".ebo/runtime/active-task.json") {
		t.Fatalf(".gitignore does not ignore active task:\n%s", gitignoreData)
	}
	agentData, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".ebo/WORKFLOW.md", "EBO HARD GATE: NO EBO TASK, NO SOURCE EDIT", "只有 ebo next 返回 EBO EXECUTION GATE: OPEN", "ebo hook pre-write --path <目标文件>", "Agent 不得执行"} {
		if !strings.Contains(string(agentData), want) {
			t.Fatalf("AGENTS.md does not contain %q:\n%s", want, agentData)
		}
	}
	workflowData, err := os.ReadFile(filepath.Join(root, ".ebo", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Ebo 项目使用说明",
		"## 0. 最高优先级执行门禁",
		"## 2. Prompt 标记与执行资格",
		"## 4. 执行下一个 Prompt",
		"输入给 Agent",
		"ebo context <prompt-id> --depth 0",
		"## 5. 将对话沉淀为 Prompt 树",
		"ebo add --dir drafts/ebo/<topic>/",
		"## 11. 从已有项目反向抽象 Prompt 树",
	} {
		if !strings.Contains(string(workflowData), want) {
			t.Fatalf("WORKFLOW.md does not contain %q:\n%s", want, workflowData)
		}
	}
}

func TestNextRejectsPlanAfterGitHeadChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	git := exec.Command("git", "init")
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}
	runCLI(t, nil, "init", "--agents", "none")
	writePrompt(t, root, project.RootID, rootPrompt)
	writePrompt(t, root, "architecture.identity", identityPrompt)
	commitAll(t, root, "baseline")
	planOutput := runCLI(t, nil, "plan")
	match := regexp.MustCompile(`created (plan-[^\s]+)`).FindStringSubmatch(planOutput)
	if len(match) != 2 {
		t.Fatalf("could not parse plan id from %s", planOutput)
	}
	if err := os.WriteFile(filepath.Join(root, "head-change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root, "change head")
	output, code := runCLIResult(nil, "next", match[1])
	if code == 0 || !strings.Contains(output, "reason: plan_base_commit_changed") {
		t.Fatalf("next should reject stale plan, code=%d output=%s", code, output)
	}
}

func runCLIResult(in *bytes.Buffer, args ...string) (string, int) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if in == nil {
		in = &bytes.Buffer{}
	}
	code := Execute(args, in, &stdout, &stderr)
	return stdout.String() + stderr.String(), code
}

func commitAll(t *testing.T, root, message string) {
	t.Helper()
	gitAddAll(t, root)
	cmd := exec.Command("git", "-c", "user.name=Ebo Test", "-c", "user.email=ebo@example.invalid", "commit", "-m", message)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, output)
	}
}

func gitAddAll(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, output)
	}
}

func TestApprovalConfirmed(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "y\n", want: true},
		{input: "YES\n", want: true},
		{input: "", want: false},
		{input: "n\n", want: false},
		{input: "sha256:abc\n", want: false},
	}
	for _, test := range tests {
		if got := approvalConfirmed(test.input); got != test.want {
			t.Errorf("approvalConfirmed(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

func runCLI(t *testing.T, in *bytes.Buffer, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if in == nil {
		in = &bytes.Buffer{}
	}
	code := Execute(args, in, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("ebo %s failed with %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), code, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func writePrompt(t *testing.T, root, id, body string) {
	t.Helper()
	path, err := project.NodePathForID(filepath.Join(root, ".ebo", "tree"), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := project.WriteFileAtomic(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const rootPrompt = `---
schema: ebo.prompt/v1
id: project.root
title: Project Root
kind: project
parent:
state:
  spec: approved
  execution: adopted
  sync: in_sync
links:
  references: []
---
## Intent
Root.
`

const identityPrompt = `---
schema: ebo.prompt/v1
id: architecture.identity
title: Identity Architecture
kind: architecture
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
scope:
  allow:
    - internal/auth/**
  deny:
    - internal/auth/secrets/**
links:
  references: []
---
## Intent
Define identity boundaries.
`

const loginPrompt = `---
schema: ebo.prompt/v1
id: feature.login
title: Login
kind: feature
parent: project.root
state:
  spec: approved
  execution: not_started
  sync: dirty
links:
  depends_on:
    - id: architecture.identity
      reason: login follows identity boundaries
---
## Intent
Let users sign in.
`
