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

	planOutput := runCLI(t, nil, "plan")
	planID := regexp.MustCompile(`created (plan-[^\s]+)`).FindStringSubmatch(planOutput)
	if len(planID) != 2 {
		t.Fatalf("could not parse plan id from:\n%s", planOutput)
	}

	nextOutput := runCLI(t, nil, "next", planID[1])
	if !strings.Contains(nextOutput, "architecture.identity") {
		t.Fatalf("next output = %s", nextOutput)
	}

	runCLI(t, nil, "report", "architecture.identity", "--plan", planID[1], "--result", "passed", "--note", "smoke")
	runCLI(t, nil, "report", "feature.login", "--plan", planID[1], "--result", "passed", "--note", "smoke")
	verifyOutput := runCLI(t, nil, "verify", planID[1])
	if !strings.Contains(verifyOutput, "status=completed") {
		t.Fatalf("verify output = %s", verifyOutput)
	}
	scanOutput := runCLI(t, nil, "scan")
	if !strings.Contains(scanOutput, "dirty tasks: 0") {
		t.Fatalf("scan output = %s", scanOutput)
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
	agentData, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".ebo/WORKFLOW.md", "只执行 ebo next 返回的单个任务", "Agent 不得执行"} {
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
		"## 2. Prompt 标记与执行资格",
		"## 4. 执行下一个 Prompt",
		"输入给 Agent",
		"ebo context <prompt-id> --depth 0",
		"## 10. 从已有项目反向抽象 Prompt 树",
	} {
		if !strings.Contains(string(workflowData), want) {
			t.Fatalf("WORKFLOW.md does not contain %q:\n%s", want, workflowData)
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
