package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lqw905/Ebo/internal/agentdocs"
	"github.com/lqw905/Ebo/internal/codexhooks"
	"github.com/lqw905/Ebo/internal/document"
	"github.com/lqw905/Ebo/internal/execution"
	"github.com/lqw905/Ebo/internal/githooks"
	"github.com/lqw905/Ebo/internal/gitx"
	"github.com/lqw905/Ebo/internal/lockfile"
	"github.com/lqw905/Ebo/internal/pathscope"
	"github.com/lqw905/Ebo/internal/planner"
	"github.com/lqw905/Ebo/internal/project"
	"github.com/lqw905/Ebo/internal/proposal"
	"github.com/lqw905/Ebo/internal/tree"
	"github.com/lqw905/Ebo/internal/workflowdocs"
)

var (
	Version = "dev"
	Commit  = "none"
	BuiltAt = "unknown"
)

func Execute(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		printHelp(out)
		return 0
	}
	var err error
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(out, "ebo %s commit=%s built=%s\n", Version, Commit, BuiltAt)
	case "help", "--help", "-h":
		printHelp(out)
	case "init":
		err = runInit(args[1:], out, errOut)
	case "doctor":
		err = runDoctor(args[1:], out)
	case "lock":
		err = runLock(args[1:], out)
	case "config":
		err = runConfig(args[1:], out)
	case "add":
		err = runAdd(args[1:], in, out, errOut)
	case "review":
		err = runReview(args[1:], out)
	case "approve":
		err = runApprove(args[1:], in, out)
	case "reject":
		err = runReject(args[1:], out)
	case "apply":
		err = runApply(args[1:], out)
	case "status":
		err = runStatus(args[1:], out)
	case "tree":
		err = runTree(args[1:], out)
	case "context":
		err = runContext(args[1:], out)
	case "scan":
		err = runScan(args[1:], out)
	case "plan":
		err = runPlan(args[1:], out)
	case "next":
		err = runNext(args[1:], out)
	case "export":
		err = runExport(args[1:], out)
	case "report":
		err = runReport(args[1:], out)
	case "verify":
		err = runVerify(args[1:], out)
	case "abort":
		err = runAbort(args[1:], out)
	case "import":
		err = runImport(args[1:], out)
	case "commit":
		err = runCommit(args[1:], out)
	case "guard":
		err = runGuard(args[1:], out)
	case "hook":
		err = runHook(args[1:], in, out)
	case "hooks":
		err = runHooks(args[1:], out)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		var coded interface{ ExitCode() int }
		if errors.As(err, &coded) {
			return coded.ExitCode()
		}
		return 1
	}
	return 0
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `Ebo Runtime CLI

Usage:
  ebo init [--agents codex,claude]
  ebo add (--stdin | --file <path> | --dir <path>) [--dry-run]
  ebo review [proposal-id]
  ebo approve <proposal-id>
  ebo apply <proposal-id>
  ebo tree (list | show <id> | validate | search <text> | graph [--around <id>])
  ebo status
  ebo scan [node-id]
  ebo plan [node-id]
  ebo plan show <plan-id>
  ebo next [plan-id]
  ebo export <plan-id> [--format markdown|json]
  ebo report <task-id> [--plan <plan-id>] --result passed|failed|blocked [--note "..."]
  ebo commit <plan-id> [--dry-run] [--message "..."]
  ebo guard check [--staged]
  ebo hook pre-write --path <file> [--json]
  ebo hook codex-pre-tool-use
  ebo hooks (install | status) [git|codex]
  ebo import <path> [--out <dir>] [--dry-run]
  ebo lock status
  ebo doctor
  ebo version`)
}

func runInit(args []string, out, errOut io.Writer) error {
	fs := newFlagSet("init", errOut)
	agents := fs.String("agents", "codex", "comma-separated agent docs to manage: codex,claude,none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	paths := project.NewPaths(root)
	if err := project.EnsureLayout(root); err != nil {
		return err
	}
	if !fileExists(paths.ConfigFile) {
		config := fmt.Sprintf("schema = \"ebo.config/v1\"\nproject_root = %q\ntree_dir = \".ebo/tree\"\nworkflow_file = \".ebo/WORKFLOW.md\"\nzero_ai = true\n", filepath.ToSlash(root))
		if err := project.WriteFileAtomic(paths.ConfigFile, []byte(config), 0o644); err != nil {
			return err
		}
	}
	workflowAction, err := workflowdocs.Update(paths.WorkflowFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s\n", workflowAction, filepath.ToSlash(filepath.Join(project.DirName, workflowdocs.Filename)))
	rootPrompt, err := project.NodePathForID(paths.TreeDir, project.RootID)
	if err != nil {
		return err
	}
	if !fileExists(rootPrompt) {
		if err := project.WriteFileAtomic(rootPrompt, []byte(defaultRootPrompt()), 0o644); err != nil {
			return err
		}
	}
	if err := ensureGitignore(root); err != nil {
		return err
	}
	for _, agent := range parseAgents(*agents) {
		var path string
		switch agent {
		case "codex":
			path = filepath.Join(root, "AGENTS.md")
		case "claude":
			path = filepath.Join(root, "CLAUDE.md")
		default:
			fmt.Fprintf(errOut, "warning: unknown agent %q ignored\n", agent)
			continue
		}
		action, err := agentdocs.Update(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", action, filepath.Base(path))
	}
	fmt.Fprintf(out, "initialized Ebo project at %s\n", root)
	return nil
}

func runDoctor(args []string, out io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("doctor does not accept arguments")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := project.FindRoot(cwd)
	if err != nil {
		fmt.Fprintln(out, "not initialized: run ebo init first")
		return err
	}
	paths := project.NewPaths(root)
	issues := 0
	check := func(ok bool, label string, detail string) {
		if ok {
			fmt.Fprintf(out, "ok   %s\n", label)
			return
		}
		issues++
		if detail == "" {
			fmt.Fprintf(out, "fail %s\n", label)
		} else {
			fmt.Fprintf(out, "fail %s: %s\n", label, detail)
		}
	}
	check(fileExists(paths.ConfigFile), "config", paths.ConfigFile)
	check(workflowdocs.IsManaged(paths.WorkflowFile), "workflow document", paths.WorkflowFile)
	check(dirExists(paths.TreeDir), "tree directory", paths.TreeDir)
	check(dirExists(paths.ProposalsDir), "proposal directory", paths.ProposalsDir)
	check(gitInside(root), "git repository", "not inside a Git work tree")
	check(gitx.Head(root) != "", "git baseline", "create an initial commit before ebo plan or ebo next")

	t, err := tree.LoadProject(root)
	if err != nil {
		return err
	}
	treeIssues := t.Validate()
	check(len(treeIssues) == 0, "prompt tree", strings.Join(treeIssues, "; "))
	if len(treeIssues) > 0 {
		for _, issue := range treeIssues {
			fmt.Fprintln(out, "  -", issue)
		}
	}
	if active, activeErr := execution.Load(root); activeErr == nil {
		_, _, validationErr := validateActiveTask(root, active)
		check(validationErr == nil, "active task", errorDetail(validationErr))
	} else if errors.Is(activeErr, execution.ErrNoActiveTask) {
		check(true, "active task", "")
	} else {
		check(false, "active task", activeErr.Error())
	}
	if containsManagedBlock(filepath.Join(root, "AGENTS.md")) {
		fmt.Fprintln(out, "ok   AGENTS.md managed block")
	} else {
		fmt.Fprintln(out, "warn AGENTS.md has no Ebo managed block")
	}
	if githooks.Installed(root) {
		fmt.Fprintln(out, "ok   Ebo pre-commit hook")
	} else {
		fmt.Fprintln(out, "warn Ebo pre-commit hook is not installed (optional: ebo hooks install)")
	}
	if info, err := lockfile.Read(root); err == nil {
		fmt.Fprintf(out, "warn project lock exists: pid=%d command=%q since %s\n", info.PID, info.Command, info.CreatedAt)
	} else {
		fmt.Fprintln(out, "ok   project lock")
	}
	if issues > 0 {
		return fmt.Errorf("%d issue(s) found", issues)
	}
	return nil
}

func runLock(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "status" {
		return fmt.Errorf("usage: ebo lock status")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	info, err := lockfile.Read(root)
	if os.IsNotExist(err) {
		fmt.Fprintln(out, "unlocked")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "locked\n")
	fmt.Fprintf(out, "path:    %s\n", lockfile.Path(root))
	fmt.Fprintf(out, "pid:     %d\n", info.PID)
	fmt.Fprintf(out, "command: %s\n", info.Command)
	fmt.Fprintf(out, "since:   %s\n", info.CreatedAt)
	return nil
}

func runConfig(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "get" {
		return fmt.Errorf("only ebo config get is implemented in this MVP slice")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(project.NewPaths(root).ConfigFile)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func runAdd(args []string, in io.Reader, out, errOut io.Writer) error {
	fs := newFlagSet("add", errOut)
	stdin := fs.Bool("stdin", false, "read prompt markdown from stdin")
	file := fs.String("file", "", "read one prompt markdown file")
	dir := fs.String("dir", "", "read prompt markdown files from directory")
	dryRun := fs.Bool("dry-run", false, "validate and preview without creating a proposal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	choices := 0
	for _, enabled := range []bool{*stdin, *file != "", *dir != ""} {
		if enabled {
			choices++
		}
	}
	if choices != 1 {
		return fmt.Errorf("choose exactly one input: --stdin, --file, or --dir")
	}
	var sources []proposal.Source
	switch {
	case *stdin:
		data, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		sources = []proposal.Source{{Kind: "stdin", Path: "stdin", Name: "stdin.md", Data: data}}
	case *file != "":
		source, err := readPromptFile(root, *file)
		if err != nil {
			return err
		}
		sources = []proposal.Source{source}
	case *dir != "":
		sources, err = readPromptDir(root, *dir)
		if err != nil {
			return err
		}
	}
	var meta *proposal.Meta
	if *dryRun {
		meta, err = proposal.Create(root, sources, true)
		if err != nil {
			return err
		}
	} else {
		err = withProjectLock(root, "add", func() error {
			var createErr error
			meta, createErr = proposal.Create(root, sources, false)
			return createErr
		})
		if err != nil {
			return err
		}
	}
	if *dryRun {
		fmt.Fprintf(out, "dry-run ok: %d node(s), proposal hash %s\n", len(meta.Nodes), meta.ProposalHash)
	} else {
		fmt.Fprintf(out, "created %s\n", meta.ID)
		fmt.Fprintf(out, "hash    %s\n", meta.ProposalHash)
		fmt.Fprintf(out, "next    ebo review %s\n", meta.ID)
	}
	for _, node := range meta.Nodes {
		fmt.Fprintf(out, "- %s (%s) parent=%s\n", node.ID, node.Kind, emptyAsDash(node.Parent))
	}
	return nil
}

func runReview(args []string, out io.Writer) error {
	root, err := requireRoot()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		items, err := proposal.List(root)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(out, "no proposals")
			return nil
		}
		for _, item := range items {
			fmt.Fprintf(out, "%s  %s  %s  nodes=%d\n", item.ID, item.Status, item.ProposalHash, len(item.Nodes))
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: ebo review [proposal-id]")
	}
	meta, err := proposal.Load(root, args[0])
	if err != nil {
		return err
	}
	actualHash, err := proposal.RecomputeHash(root, meta)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal: %s\n", meta.ID)
	fmt.Fprintf(out, "status:   %s\n", meta.Status)
	fmt.Fprintf(out, "hash:     %s\n", meta.ProposalHash)
	if actualHash != meta.ProposalHash {
		fmt.Fprintf(out, "warning:  stored content changed; actual hash is %s\n", actualHash)
	}
	fmt.Fprintln(out, "\nsources:")
	for _, source := range meta.Sources {
		fmt.Fprintf(out, "- %s %s %s\n", source.Kind, source.Path, source.SHA256)
	}
	fmt.Fprintln(out, "\nnodes:")
	for _, node := range meta.Nodes {
		fmt.Fprintf(out, "- %s (%s)\n", node.ID, node.Kind)
		fmt.Fprintf(out, "  title: %s\n", node.Title)
		fmt.Fprintf(out, "  parent: %s\n", emptyAsDash(node.Parent))
		fmt.Fprintf(out, "  content: %s\n", node.ContentHash)
	}
	if meta.Status == "approved" {
		fmt.Fprintf(out, "\napproved hash: %s\n", meta.ApprovedHash)
	}
	return nil
}

func runApprove(args []string, in io.Reader, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ebo approve <proposal-id>")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	meta, err := proposal.Load(root, args[0])
	if err != nil {
		return err
	}
	if !isTerminal(in) {
		return fmt.Errorf("approve requires an interactive terminal")
	}
	actualHash, err := proposal.RecomputeHash(root, meta)
	if err != nil {
		return err
	}
	if actualHash != meta.ProposalHash {
		return fmt.Errorf("proposal content changed: expected %s, got %s", meta.ProposalHash, actualHash)
	}
	fmt.Fprintf(out, "Proposal: %s\n", meta.ID)
	fmt.Fprintf(out, "Nodes:    %d\n", len(meta.Nodes))
	for _, node := range meta.Nodes {
		fmt.Fprintf(out, "- %s (%s): %s\n", node.ID, node.Kind, node.Title)
	}
	fmt.Fprintf(out, "Hash:     %s\n", document.ShortHash(meta.ProposalHash, 12))
	fmt.Fprintln(out, "Approve this proposal? [y/N]")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if !approvalConfirmed(line) {
		fmt.Fprintln(out, "approval canceled")
		return nil
	}
	var approved *proposal.Meta
	err = withProjectLock(root, "approve", func() error {
		var approveErr error
		approved, approveErr = proposal.Approve(root, meta.ID)
		return approveErr
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "approved %s at %s\n", approved.ID, approved.ApprovedAt)
	return nil
}

func approvalConfirmed(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func runReject(args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ebo reject <proposal-id> --reason \"...\"")
	}
	id := args[0]
	reason := valueFlag(args[1:], "reason")
	if reason == "" {
		return fmt.Errorf("--reason is required")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	var meta *proposal.Meta
	err = withProjectLock(root, "reject", func() error {
		var rejectErr error
		meta, rejectErr = proposal.Reject(root, id, reason)
		return rejectErr
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "rejected %s\n", meta.ID)
	return nil
}

func runApply(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ebo apply <proposal-id>")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	var meta *proposal.Meta
	err = withProjectLock(root, "apply", func() error {
		var applyErr error
		meta, applyErr = proposal.Apply(root, args[0])
		return applyErr
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "applied %s\n", meta.ID)
	return nil
}

func runStatus(args []string, out io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("status does not accept arguments")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "project: %s\n", root)
	proposals, err := proposal.List(root)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, item := range proposals {
		counts[item.Status]++
	}
	fmt.Fprintf(out, "proposals: proposed=%d approved=%d applied=%d rejected=%d\n", counts["proposed"], counts["approved"], counts["applied"], counts["rejected"])
	t, err := tree.LoadProject(root)
	if err != nil {
		return err
	}
	issues := t.Validate()
	if len(issues) > 0 {
		fmt.Fprintf(out, "tree: invalid (%d issue(s))\n", len(issues))
	} else {
		fmt.Fprintf(out, "tree: ok (%d node(s))\n", len(t.Nodes))
	}
	dirty := t.DirtyNodes()
	fmt.Fprintf(out, "dirty nodes: %d\n", len(dirty))
	for _, id := range dirty {
		fmt.Fprintf(out, "- %s\n", id)
	}
	plans, err := planner.List(root)
	if err != nil {
		return err
	}
	planCounts := map[string]int{}
	for _, item := range plans {
		planCounts[item.Status]++
	}
	fmt.Fprintf(out, "plans: planned=%d running=%d completed=%d failed=%d blocked=%d aborted=%d empty=%d\n", planCounts["planned"], planCounts["running"], planCounts["completed"], planCounts["failed"], planCounts["blocked"], planCounts["aborted"], planCounts["empty"])
	if active, activeErr := execution.Load(root); activeErr == nil {
		if _, _, validationErr := validateActiveTask(root, active); validationErr != nil {
			fmt.Fprintf(out, "gate: invalid (%v)\n", validationErr)
		} else {
			fmt.Fprintf(out, "gate: open plan=%s task=%s prompt=%s\n", active.PlanID, active.TaskID, active.PromptID)
		}
	} else if errors.Is(activeErr, execution.ErrNoActiveTask) {
		fmt.Fprintln(out, "gate: closed")
	} else {
		fmt.Fprintf(out, "gate: invalid (%v)\n", activeErr)
	}
	return nil
}

func runTree(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ebo tree <list|show|validate|search|graph>")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	t, err := tree.LoadProject(root)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return treeList(t, out)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: ebo tree show <node-id>")
		}
		return treeShow(t, args[1], out)
	case "validate":
		issues := t.Validate()
		if len(issues) == 0 {
			fmt.Fprintln(out, "tree ok")
			return nil
		}
		for _, issue := range issues {
			fmt.Fprintln(out, "-", issue)
		}
		return fmt.Errorf("tree invalid: %d issue(s)", len(issues))
	case "search":
		if len(args) != 2 {
			return fmt.Errorf("usage: ebo tree search <text>")
		}
		return treeSearch(t, args[1], out)
	case "graph":
		return treeGraph(t, args[1:], out)
	default:
		return fmt.Errorf("unknown tree command %q", args[0])
	}
}

func treeList(t *tree.Tree, out io.Writer) error {
	effective := t.EffectiveHashes()
	for _, id := range t.IDs() {
		node := t.Nodes[id]
		dirty := "dirty"
		if node.Hash.Satisfied != "" && node.Hash.Satisfied == effective[id] {
			dirty = "in_sync"
		}
		fmt.Fprintf(out, "%s  spec=%s execution=%s sync=%s computed=%s\n", id, node.State.Spec, node.State.Execution, dirty, effective[id])
	}
	return nil
}

func treeShow(t *tree.Tree, id string, out io.Writer) error {
	node := t.Nodes[id]
	if node == nil {
		return fmt.Errorf("node %s not found", id)
	}
	fmt.Fprintf(out, "id:        %s\n", node.ID)
	fmt.Fprintf(out, "title:     %s\n", node.Title)
	fmt.Fprintf(out, "kind:      %s\n", node.Kind)
	fmt.Fprintf(out, "parent:    %s\n", emptyAsDash(node.Parent))
	fmt.Fprintf(out, "state:     spec=%s execution=%s sync=%s\n", node.State.Spec, node.State.Execution, node.State.Sync)
	fmt.Fprintf(out, "file:      %s\n", t.Files[id])
	fmt.Fprintf(out, "content:   %s\n", document.ContentHash(node))
	fmt.Fprintf(out, "effective: %s\n", t.EffectiveHashes()[id])
	if len(node.Links) > 0 {
		fmt.Fprintln(out, "links:")
		for _, typ := range sortedLinkTypes(node.Links) {
			for _, link := range node.Links[typ] {
				if link.Reason == "" {
					fmt.Fprintf(out, "- %s -> %s\n", typ, link.ID)
				} else {
					fmt.Fprintf(out, "- %s -> %s: %s\n", typ, link.ID, link.Reason)
				}
			}
		}
	}
	if strings.TrimSpace(node.Body) != "" {
		fmt.Fprintln(out, "\n--- body ---")
		fmt.Fprintln(out, strings.TrimSpace(node.Body))
	}
	return nil
}

func treeSearch(t *tree.Tree, query string, out io.Writer) error {
	query = strings.ToLower(query)
	matches := 0
	for _, id := range t.IDs() {
		node := t.Nodes[id]
		hay := strings.ToLower(node.ID + "\n" + node.Title + "\n" + node.Body)
		for _, links := range node.Links {
			for _, link := range links {
				hay += "\n" + strings.ToLower(link.ID+" "+link.Reason)
			}
		}
		if strings.Contains(hay, query) {
			matches++
			fmt.Fprintf(out, "%s  %s\n", node.ID, node.Title)
		}
	}
	if matches == 0 {
		fmt.Fprintln(out, "no matches")
	}
	return nil
}

func treeGraph(t *tree.Tree, args []string, out io.Writer) error {
	around := ""
	if len(args) == 1 {
		around = args[0]
	}
	if len(args) == 2 && args[0] == "--around" {
		around = args[1]
	}
	if len(args) > 2 || (len(args) == 2 && args[0] != "--around") {
		return fmt.Errorf("usage: ebo tree graph [node-id] | ebo tree graph --around <node-id>")
	}
	ids := t.IDs()
	if around != "" {
		if t.Nodes[around] == nil {
			return fmt.Errorf("node %s not found", around)
		}
		set := map[string]bool{around: true}
		for _, child := range t.Children(around) {
			set[child] = true
		}
		if parent := t.Nodes[around].Parent; parent != "" {
			set[parent] = true
		}
		for typ, links := range t.Nodes[around].Links {
			_ = typ
			for _, link := range links {
				set[link.ID] = true
			}
		}
		ids = ids[:0]
		for id := range set {
			if t.Nodes[id] != nil {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
	}
	for _, id := range ids {
		node := t.Nodes[id]
		if node.Parent != "" {
			fmt.Fprintf(out, "%s --parent--> %s\n", node.ID, node.Parent)
		} else {
			fmt.Fprintf(out, "%s\n", node.ID)
		}
		for _, typ := range sortedLinkTypes(node.Links) {
			for _, link := range node.Links[typ] {
				fmt.Fprintf(out, "%s --%s--> %s\n", node.ID, typ, link.ID)
			}
		}
	}
	return nil
}

func runContext(args []string, out io.Writer) error {
	nodeID, depth, outPath, err := parseContextArgs(args)
	if err != nil {
		return err
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	t, err := tree.LoadProject(root)
	if err != nil {
		return err
	}
	if t.Nodes[nodeID] == nil {
		return fmt.Errorf("node %s not found", nodeID)
	}
	payload := buildContextPayload(t, nodeID, depth)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if outPath != "" {
		if err := project.WriteFileAtomic(outPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", outPath)
		return nil
	}
	_, err = out.Write(data)
	return err
}

func runScan(args []string, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ebo scan [node-id]")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	t, err := tree.LoadProject(root)
	if err != nil {
		return err
	}
	if issues := t.Validate(); len(issues) > 0 {
		for _, issue := range issues {
			fmt.Fprintln(out, "-", issue)
		}
		return fmt.Errorf("tree invalid")
	}
	order := t.ExecutionOrder()
	if len(args) == 1 {
		order = filterOrderFromRoot(t, order, args[0])
	}
	fmt.Fprintf(out, "dirty tasks: %d\n", len(order))
	for i, id := range order {
		fmt.Fprintf(out, "%d. %s  %s\n", i+1, id, t.Nodes[id].Title)
	}
	if len(order) == 0 {
		printGateClosed(out, "no_executable_task", "create a Prompt proposal and wait for human approval")
	} else {
		printGateClosed(out, "scan_is_informational", "run ebo next to request execution authorization")
	}
	return nil
}

func runPlan(args []string, out io.Writer) error {
	root, err := requireRoot()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			if len(args) != 1 {
				return fmt.Errorf("usage: ebo plan list")
			}
			plans, err := planner.List(root)
			if err != nil {
				return err
			}
			if len(plans) == 0 {
				fmt.Fprintln(out, "no plans")
				return nil
			}
			for _, plan := range plans {
				fmt.Fprintf(out, "%s  %s  tasks=%d  root=%s\n", plan.ID, plan.Status, len(plan.Tasks), emptyAsDash(plan.Root))
			}
			return nil
		case "show":
			if len(args) != 2 {
				return fmt.Errorf("usage: ebo plan show <plan-id>")
			}
			plan, err := planner.Load(root, args[1])
			if err != nil {
				return err
			}
			return printPlan(plan, out)
		}
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: ebo plan [node-id]")
	}
	rootNode := ""
	if len(args) == 1 {
		rootNode = args[0]
	}
	var plan *planner.Plan
	err = withProjectLock(root, "plan", func() error {
		t, err := tree.LoadProject(root)
		if err != nil {
			return err
		}
		var createErr error
		plan, createErr = planner.Create(root, t, rootNode)
		if createErr != nil {
			return createErr
		}
		return planner.Save(root, plan)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s\n", plan.ID)
	fmt.Fprintf(out, "status  %s\n", plan.Status)
	fmt.Fprintf(out, "tasks   %d\n", len(plan.Tasks))
	fmt.Fprintf(out, "next    ebo next %s\n", plan.ID)
	return nil
}

func runNext(args []string, out io.Writer) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: ebo next [plan-id]")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	return withProjectLock(root, "next", func() error {
		if gitx.Head(root) == "" {
			printGateClosed(out, "git_baseline_missing", "commit the initialized project before requesting a task")
			return fmt.Errorf("git baseline is missing")
		}
		t, err := tree.LoadProject(root)
		if err != nil {
			return err
		}
		if issues := t.Validate(); len(issues) > 0 {
			return fmt.Errorf("tree invalid: %s", strings.Join(issues, "; "))
		}
		if active, activeErr := execution.Load(root); activeErr == nil {
			plan, task, err := validateActiveTask(root, active)
			if err != nil {
				printGateClosed(out, "active_task_invalid", "resolve the active task state before editing source code")
				return err
			}
			if len(args) == 1 && args[0] != plan.ID {
				return fmt.Errorf("active task belongs to plan %s, not %s", plan.ID, args[0])
			}
			node := t.Nodes[task.PromptID]
			if err := validateTaskPrompt(t, node, task); err != nil {
				printGateClosed(out, "prompt_changed_after_task_start", "abort or report the active task before continuing")
				return err
			}
			fmt.Fprint(out, planner.AuthorizedTaskPackage(plan, node, task))
			return nil
		} else if !errors.Is(activeErr, execution.ErrNoActiveTask) {
			printGateClosed(out, "active_task_unreadable", "inspect the runtime state, then run ebo abort <plan-id> to recover")
			return activeErr
		}

		var plan *planner.Plan
		if len(args) == 1 {
			plan, err = planner.Load(root, args[0])
			if err != nil {
				return err
			}
		} else {
			plan, err = planner.LatestActive(root)
			if err != nil {
				return err
			}
			if plan == nil {
				plan, err = planner.Create(root, t, "")
				if err != nil {
					return err
				}
			}
		}
		if plan.Status == "aborted" {
			printGateClosed(out, "plan_aborted", "create a new plan after an approved Prompt change")
			return fmt.Errorf("plan %s is aborted", plan.ID)
		}
		task := planner.NextTask(plan)
		if task == nil {
			printGateClosed(out, "no_executable_task", "create a Prompt proposal and wait for human approval")
			return nil
		}
		if plan.BaseCommit != gitx.Head(root) {
			printGateClosed(out, "plan_base_commit_changed", "create a new plan from the current Git HEAD")
			return fmt.Errorf("plan %s is based on %s but HEAD is %s", plan.ID, plan.BaseCommit, gitx.Head(root))
		}
		if plan.Status == "planned" {
			changed, err := gitx.ChangedNames(root)
			if err != nil {
				return err
			}
			if sourceNames := sourceChangeNames(changed); len(sourceNames) > 0 {
				printGateClosed(out, "preexisting_source_changes", "reconcile or commit existing source changes before starting the plan")
				for _, name := range sourceNames {
					fmt.Fprintf(out, "- %s\n", name)
				}
				return fmt.Errorf("%d source change(s) predate the first authorized task", len(sourceNames))
			}
		}
		node := t.Nodes[task.PromptID]
		if err := validateTaskPrompt(t, node, task); err != nil {
			printGateClosed(out, "plan_prompt_changed", "create a new plan for the current Prompt hashes")
			return err
		}
		oldTaskStatus := task.Status
		oldPlanStatus := plan.Status
		task, err = planner.StartTask(plan, task.ID)
		if err != nil {
			return err
		}
		if err := planner.Save(root, plan); err != nil {
			return err
		}
		active := execution.New(plan.ID, task.ID, task.PromptID, task.ContentHash, task.EffectiveHash, plan.BaseCommit)
		if err := execution.Save(root, active); err != nil {
			task.Status = oldTaskStatus
			plan.Status = oldPlanStatus
			_ = planner.Save(root, plan)
			return err
		}
		fmt.Fprint(out, planner.AuthorizedTaskPackage(plan, node, task))
		return nil
	})
}

func runExport(args []string, out io.Writer) error {
	planID, format, outPath, err := parseExportArgs(args)
	if err != nil {
		return err
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	plan, err := planner.Load(root, planID)
	if err != nil {
		return err
	}
	var data []byte
	switch format {
	case "json":
		data, err = json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	case "markdown":
		t, err := tree.LoadProject(root)
		if err != nil {
			return err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Ebo Plan %s\n\n", plan.ID)
		fmt.Fprintln(&b, "> 该文件仅用于查看或传递计划，不授予源码修改权限。执行前必须运行 `ebo next` 并取得 `EBO EXECUTION GATE: OPEN`。")
		fmt.Fprintln(&b)
		for i := range plan.Tasks {
			task := &plan.Tasks[i]
			node := t.Nodes[task.PromptID]
			if node == nil {
				continue
			}
			if i > 0 {
				fmt.Fprint(&b, "\n---\n\n")
			}
			b.WriteString(planner.TaskPackage(plan, node, task))
		}
		data = []byte(b.String())
	default:
		return fmt.Errorf("--format must be markdown or json")
	}
	if outPath != "" {
		if err := project.WriteFileAtomic(outPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", outPath)
		return nil
	}
	_, err = out.Write(data)
	return err
}

func runReport(args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ebo report <task-id> [--plan <plan-id>] --result passed|failed|blocked [--note \"...\"]")
	}
	taskID := args[0]
	result := valueFlag(args[1:], "result")
	note := valueFlag(args[1:], "note")
	planID := valueFlag(args[1:], "plan")
	if result != "passed" && result != "failed" && result != "blocked" {
		return fmt.Errorf("--result must be passed, failed, or blocked")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	return withProjectLock(root, "report", func() error {
		active, err := execution.Load(root)
		if errors.Is(err, execution.ErrNoActiveTask) {
			printGateClosed(out, "no_active_task", "run ebo next before reporting a task")
			return fmt.Errorf("report requires an active task")
		}
		if err != nil {
			printGateClosed(out, "active_task_unreadable", "inspect the runtime state, then run ebo abort <plan-id> to recover")
			return err
		}
		plan, task, err := validateActiveTask(root, active)
		if err != nil {
			printGateClosed(out, "active_task_invalid", "abort the active plan before continuing")
			return err
		}
		if planID != "" && planID != plan.ID {
			return fmt.Errorf("active task belongs to plan %s, not %s", plan.ID, planID)
		}
		if taskID != task.ID && taskID != task.PromptID {
			return fmt.Errorf("active task is %s, not %s", task.ID, taskID)
		}
		if err := tree.ApplyTaskResult(root, tree.TaskResultUpdate{
			PromptID:      task.PromptID,
			Result:        result,
			ContentHash:   task.ContentHash,
			EffectiveHash: task.EffectiveHash,
		}); err != nil {
			return err
		}
		appliedPlanID := plan.ID
		taskID = task.ID
		reportedAt := time.Now().UTC()
		receipt := map[string]string{
			"schema":         "ebo.receipt/v1",
			"plan_id":        appliedPlanID,
			"task_id":        taskID,
			"result":         result,
			"note":           note,
			"base_commit":    active.BaseCommit,
			"content_hash":   active.ContentHash,
			"effective_hash": active.EffectiveHash,
			"created_at":     reportedAt.Format(time.RFC3339Nano),
		}
		data, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		name := fmt.Sprintf("%s-%s.json", project.SafeFilename(taskID), reportedAt.Format("20060102-150405.000000000"))
		path := filepath.Join(project.NewPaths(root).ReceiptsDir, name)
		if err := project.WriteFileAtomic(path, data, 0o644); err != nil {
			return err
		}
		task, err = planner.Report(root, plan, task.ID, result, note)
		if err != nil {
			_ = os.Remove(path)
			return err
		}
		if err := execution.Clear(root); err != nil {
			return err
		}
		fmt.Fprintf(out, "updated %s task %s -> %s\n", plan.ID, task.ID, result)
		fmt.Fprintf(out, "wrote receipt %s\n", path)
		fmt.Fprintln(out, "EBO EXECUTION GATE: CLOSED")
		return nil
	})
}

func runVerify(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ebo verify <plan-id>")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	plan, err := planner.Load(root, args[0])
	if err != nil {
		return err
	}
	pending := 0
	running := 0
	failed := 0
	blocked := 0
	for _, task := range plan.Tasks {
		switch task.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "failed":
			failed++
		case "blocked":
			blocked++
		}
	}
	fmt.Fprintf(out, "plan %s status=%s pending=%d running=%d failed=%d blocked=%d\n", plan.ID, plan.Status, pending, running, failed, blocked)
	if plan.Status != "completed" && plan.Status != "empty" {
		return fmt.Errorf("plan %s is %s; only completed or empty plans verify successfully", plan.ID, plan.Status)
	}
	if pending+running+failed+blocked > 0 {
		return fmt.Errorf("plan is not complete")
	}
	return nil
}

func runAbort(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ebo abort <plan-id>")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	return withProjectLock(root, "abort", func() error {
		plan, err := planner.Load(root, args[0])
		if err != nil {
			return err
		}
		planner.Abort(plan)
		if err := planner.Save(root, plan); err != nil {
			return err
		}
		if active, activeErr := execution.Load(root); activeErr == nil && active.PlanID == plan.ID {
			if err := execution.Clear(root); err != nil {
				return err
			}
		} else if activeErr != nil && !errors.Is(activeErr, execution.ErrNoActiveTask) {
			if err := execution.Clear(root); err != nil {
				return err
			}
			fmt.Fprintln(out, "cleared unreadable active task lease")
		}
		fmt.Fprintf(out, "aborted %s\n", plan.ID)
		fmt.Fprintln(out, "EBO EXECUTION GATE: CLOSED")
		return nil
	})
}

func runCommit(args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ebo commit <plan-id> [--dry-run] [--message \"...\"]")
	}
	planID := args[0]
	dryRun := hasFlag(args[1:], "dry-run")
	message := valueFlag(args[1:], "message")
	if message == "" {
		message = "ebo: complete " + planID
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	return withProjectLock(root, "commit", func() error {
		plan, err := planner.Load(root, planID)
		if err != nil {
			return err
		}
		if plan.Status != "completed" {
			return fmt.Errorf("plan %s is %s; only completed plans can be committed", plan.ID, plan.Status)
		}
		status, err := gitx.Status(root)
		if err != nil {
			return err
		}
		pathspecs := []string{
			".ebo/tree",
			filepath.ToSlash(filepath.Join(".ebo", "plans", plan.ID+".json")),
			".ebo/receipts",
		}
		fmt.Fprintf(out, "plan:    %s\n", plan.ID)
		fmt.Fprintf(out, "message: %s\n", message)
		fmt.Fprintln(out, "ebo paths staged by this command:")
		for _, pathspec := range pathspecs {
			fmt.Fprintf(out, "- %s\n", pathspec)
		}
		if len(status) > 0 {
			fmt.Fprintln(out, "current git status:")
			for _, line := range status {
				fmt.Fprintf(out, "- %s\n", line)
			}
		}
		if dryRun {
			fmt.Fprintln(out, "dry-run: no git add or git commit executed")
			return nil
		}
		if err := gitx.Add(root, pathspecs...); err != nil {
			return err
		}
		staged, err := gitx.CachedNames(root)
		if err != nil {
			return err
		}
		if len(staged) == 0 {
			return fmt.Errorf("nothing is staged for commit")
		}
		if sourceNames := sourceChangeNames(staged); len(sourceNames) > 0 {
			if _, err := stagedCompletedPlan(root, staged); err != nil {
				return fmt.Errorf("staged guard rejected commit: %w", err)
			}
			fmt.Fprintf(out, "guard:  pass (%d staged source change(s))\n", len(sourceNames))
		}
		if err := gitx.Commit(root, message); err != nil {
			return err
		}
		fmt.Fprintf(out, "committed %s\n", plan.ID)
		return nil
	})
}

func runGuard(args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "check" || len(args) > 2 || (len(args) == 2 && args[1] != "--staged") {
		return fmt.Errorf("usage: ebo guard check [--staged]")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	if gitx.Head(root) == "" {
		printGateClosed(out, "git_baseline_missing", "create an initial commit before checking execution authorization")
		return fmt.Errorf("git baseline is missing")
	}
	staged := len(args) == 2
	if staged {
		names, err := gitx.CachedNames(root)
		if err != nil {
			return err
		}
		sourceNames := sourceChangeNames(names)
		if len(sourceNames) == 0 {
			fmt.Fprintln(out, "guard: pass")
			fmt.Fprintln(out, "mode: staged")
			fmt.Fprintln(out, "reason: no_staged_source_changes")
			return nil
		}
		plan, err := stagedCompletedPlan(root, names)
		if err != nil {
			printGateClosed(out, "staged_source_without_completed_plan", "use ebo commit <plan-id> after verification")
			for _, name := range sourceNames {
				fmt.Fprintf(out, "- %s\n", name)
			}
			return err
		}
		fmt.Fprintln(out, "guard: pass")
		fmt.Fprintln(out, "mode: staged")
		fmt.Fprintf(out, "plan: %s\n", plan.ID)
		fmt.Fprintf(out, "source_changes: %d\n", len(sourceNames))
		return nil
	}

	names, err := gitx.ChangedNames(root)
	if err != nil {
		return err
	}
	sourceNames := sourceChangeNames(names)
	active, activeErr := execution.Load(root)
	if activeErr == nil {
		plan, task, err := validateActiveTask(root, active)
		if err != nil {
			printGateClosed(out, "active_task_invalid", "resolve or abort the active task")
			return err
		}
		fmt.Fprintln(out, "guard: pass")
		fmt.Fprintln(out, "EBO EXECUTION GATE: OPEN")
		fmt.Fprintln(out, "source_edit: allowed")
		fmt.Fprintf(out, "plan: %s\ntask: %s\nprompt: %s\n", plan.ID, task.ID, task.PromptID)
		fmt.Fprintf(out, "source_changes: %d\n", len(sourceNames))
		return nil
	}
	if !errors.Is(activeErr, execution.ErrNoActiveTask) {
		printGateClosed(out, "active_task_unreadable", "inspect the runtime state, then run ebo abort <plan-id> to recover")
		return activeErr
	}
	if len(sourceNames) == 0 {
		fmt.Fprintln(out, "guard: pass")
		printGateClosed(out, "no_active_task", "run ebo next before editing source code")
		return nil
	}
	printGateClosed(out, "unauthorized_source_changes", "create and approve a Prompt proposal, then run ebo next")
	for _, name := range sourceNames {
		fmt.Fprintf(out, "- %s\n", name)
	}
	return fmt.Errorf("%d source change(s) exist without an active Ebo task", len(sourceNames))
}

type commandExitError struct {
	code int
	err  error
}

func (e *commandExitError) Error() string {
	return e.err.Error()
}

func (e *commandExitError) Unwrap() error {
	return e.err
}

func (e *commandExitError) ExitCode() int {
	return e.code
}

type preWriteResult struct {
	Schema   string `json:"schema"`
	Hook     string `json:"hook"`
	Allowed  bool   `json:"allowed"`
	Gate     string `json:"gate"`
	Mode     string `json:"mode"`
	Path     string `json:"path,omitempty"`
	Reason   string `json:"reason"`
	Action   string `json:"action,omitempty"`
	PlanID   string `json:"plan_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	PromptID string `json:"prompt_id,omitempty"`
}

func runHook(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ebo hook <pre-write|codex-pre-tool-use>")
	}
	if args[0] == "codex-pre-tool-use" {
		return runCodexPreToolUse(args[1:], in, out)
	}
	if args[0] != "pre-write" {
		return fmt.Errorf("usage: ebo hook <pre-write|codex-pre-tool-use>")
	}
	fs := newFlagSet("hook pre-write", io.Discard)
	target := fs.String("path", "", "project-relative file path that the Agent intends to write")
	jsonOutput := fs.Bool("json", false, "emit a structured hook decision")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*target) == "" {
		return fmt.Errorf("usage: ebo hook pre-write --path <file> [--json]")
	}
	root, err := requireRoot()
	if err != nil {
		return &commandExitError{code: 2, err: err}
	}
	rel, err := projectRelativePath(root, *target)
	if err != nil {
		result := preWriteResult{
			Schema:  "ebo.hook-result/v1",
			Hook:    "pre-write",
			Allowed: false,
			Gate:    "closed",
			Mode:    "source",
			Reason:  "invalid_target_path",
			Action:  "provide a file path inside the Ebo project",
		}
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 1, err: err}
	}
	if protectedPreWritePath(rel) {
		result := preWriteResult{
			Schema:  "ebo.hook-result/v1",
			Hook:    "pre-write",
			Allowed: false,
			Gate:    "closed",
			Mode:    "control",
			Path:    rel,
			Reason:  "protected_control_path",
			Action:  "use the owning Ebo or Git command instead of editing this file directly",
		}
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 1, err: fmt.Errorf("pre-write denied for protected control path %s", rel)}
	}
	if promptDraftPath(rel) {
		result := preWriteResult{
			Schema:  "ebo.hook-result/v1",
			Hook:    "pre-write",
			Allowed: true,
			Gate:    "closed",
			Mode:    "proposal_draft",
			Path:    rel,
			Reason:  "prompt_draft_allowed",
			Action:  "run ebo add to create a proposal, then wait for human approval",
		}
		emitPreWriteResult(out, result, *jsonOutput)
		return nil
	}

	active, err := execution.Load(root)
	if errors.Is(err, execution.ErrNoActiveTask) {
		result := preWriteResult{
			Schema:  "ebo.hook-result/v1",
			Hook:    "pre-write",
			Allowed: false,
			Gate:    "closed",
			Mode:    "source",
			Path:    rel,
			Reason:  "no_active_task",
			Action:  "run ebo next and require EBO EXECUTION GATE: OPEN before writing source code",
		}
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 1, err: fmt.Errorf("pre-write denied: no active Ebo task")}
	}
	if err != nil {
		result := preWriteResult{Schema: "ebo.hook-result/v1", Hook: "pre-write", Allowed: false, Gate: "closed", Mode: "source", Path: rel, Reason: "active_task_unreadable", Action: "inspect the runtime state and abort the affected plan"}
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 2, err: err}
	}
	plan, task, err := validateActiveTask(root, active)
	if err != nil {
		result := preWriteResult{Schema: "ebo.hook-result/v1", Hook: "pre-write", Allowed: false, Gate: "closed", Mode: "source", Path: rel, Reason: "active_task_invalid", Action: "resolve or abort the active plan before writing source code"}
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 2, err: err}
	}
	promptTree, err := tree.LoadProject(root)
	if err != nil {
		return &commandExitError{code: 2, err: err}
	}
	prompt := promptTree.Nodes[task.PromptID]
	allowed, reason := pathscope.Allows(prompt.Scope, rel)
	result := preWriteResult{
		Schema:   "ebo.hook-result/v1",
		Hook:     "pre-write",
		Allowed:  allowed,
		Gate:     "open",
		Mode:     "source",
		Path:     rel,
		Reason:   reason,
		PlanID:   plan.ID,
		TaskID:   task.ID,
		PromptID: task.PromptID,
	}
	if !allowed {
		result.Gate = "closed"
		result.Action = "only write files included by the active Prompt scope"
		emitPreWriteResult(out, result, *jsonOutput)
		return &commandExitError{code: 1, err: fmt.Errorf("pre-write denied: %s is outside the active Prompt scope", rel)}
	}
	emitPreWriteResult(out, result, *jsonOutput)
	return nil
}

type codexPreToolInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

func runCodexPreToolUse(args []string, in io.Reader, out io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: ebo hook codex-pre-tool-use")
	}
	var input codexPreToolInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		emitCodexDeny(out, "Ebo could not parse the Codex PreToolUse input")
		return nil
	}
	if input.HookEventName != "PreToolUse" || input.ToolName != "apply_patch" {
		return nil
	}
	paths := patchTargetPaths(input.ToolInput.Command)
	if len(paths) == 0 {
		emitCodexDeny(out, "Ebo could not determine the target paths in this apply_patch call")
		return nil
	}
	if _, err := requireRoot(); err != nil {
		// The global Codex hook intentionally has no effect outside Ebo projects.
		return nil
	}
	for _, path := range paths {
		var decision bytes.Buffer
		err := runHook([]string{"pre-write", "--path", path, "--json"}, bytes.NewReader(nil), &decision)
		if err == nil {
			continue
		}
		var result preWriteResult
		reason := strings.TrimSpace(decision.String())
		if json.Unmarshal(bytes.TrimSpace(decision.Bytes()), &result) == nil && result.Reason != "" {
			reason = result.Reason
		}
		if reason == "" {
			reason = err.Error()
		}
		emitCodexDeny(out, fmt.Sprintf("Ebo denied writing %s: %s", path, reason))
		return nil
	}
	return nil
}

func patchTargetPaths(patch string) []string {
	prefixes := []string{"*** Add File:", "*** Update File:", "*** Delete File:", "*** Move to:"}
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n") {
		for _, prefix := range prefixes {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			path := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
			break
		}
	}
	return paths
}

func emitCodexDeny(out io.Writer, reason string) {
	payload := map[string]any{
		"hookSpecificOutput": map[string]string{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
	data, _ := json.Marshal(payload)
	fmt.Fprintln(out, string(data))
}

func emitPreWriteResult(out io.Writer, result preWriteResult, jsonOutput bool) {
	if jsonOutput {
		data, _ := json.Marshal(result)
		fmt.Fprintln(out, string(data))
		return
	}
	if result.Allowed {
		fmt.Fprintln(out, "EBO PRE-WRITE: ALLOWED")
	} else {
		fmt.Fprintln(out, "EBO PRE-WRITE: DENIED")
	}
	fmt.Fprintf(out, "EBO EXECUTION GATE: %s\n", strings.ToUpper(result.Gate))
	if result.Mode == "source" && result.Allowed {
		fmt.Fprintln(out, "source_edit: allowed")
	} else {
		fmt.Fprintln(out, "source_edit: forbidden")
	}
	fmt.Fprintf(out, "mode: %s\n", result.Mode)
	if result.Path != "" {
		fmt.Fprintf(out, "path: %s\n", result.Path)
	}
	fmt.Fprintf(out, "reason: %s\n", result.Reason)
	if result.PlanID != "" {
		fmt.Fprintf(out, "plan: %s\ntask: %s\nprompt: %s\n", result.PlanID, result.TaskID, result.PromptID)
	}
	if result.Action != "" {
		fmt.Fprintf(out, "action: %s\n", result.Action)
	}
}

func projectRelativePath(root, target string) (string, error) {
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("target %s is outside the project", target)
	}
	return rel, nil
}

func protectedPreWritePath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	if metadataChangePath(lower) || lower == ".git" || strings.HasPrefix(lower, ".git/") {
		return true
	}
	switch lower {
	case ".gitmodules":
		return true
	default:
		return false
	}
}

func promptDraftPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.HasPrefix(lower, "drafts/") && strings.HasSuffix(lower, ".md")
}

func runHooks(args []string, out io.Writer) error {
	if len(args) < 1 || len(args) > 2 || (args[0] != "install" && args[0] != "status") {
		return fmt.Errorf("usage: ebo hooks <install|status> [git|codex]")
	}
	target := "git"
	if len(args) == 2 {
		target = args[1]
	}
	if target != "git" && target != "codex" {
		return fmt.Errorf("hook target must be git or codex")
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	if target == "codex" {
		path := codexhooks.ProjectPath(root)
		if args[0] == "status" {
			if codexhooks.Installed(path) {
				fmt.Fprintf(out, "installed %s\n", path)
			} else {
				fmt.Fprintf(out, "not installed %s\n", path)
			}
			return nil
		}
		return withProjectLock(root, "hooks install codex", func() error {
			action, err := codexhooks.Install(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s %s\n", action, path)
			fmt.Fprintln(out, "This is a project-local hook. Open /hooks in Codex to review and trust it.")
			return nil
		})
	}
	if args[0] == "status" {
		if githooks.Installed(root) {
			path, _ := githooks.Path(root)
			fmt.Fprintf(out, "installed %s\n", path)
		} else {
			fmt.Fprintln(out, "not installed")
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return withProjectLock(root, "hooks install", func() error {
		action, err := githooks.Install(root, executable)
		if err != nil {
			return err
		}
		path, err := githooks.Path(root)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", action, path)
		return nil
	})
}

func sourceChangeNames(names []string) []string {
	var out []string
	for _, name := range names {
		name = filepath.ToSlash(strings.TrimSpace(name))
		if metadataChangePath(name) || promptDraftPath(name) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func metadataChangePath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	switch {
	case lower == ".ebo", strings.HasPrefix(lower, ".ebo/"):
		return true
	case lower == ".prompt", strings.HasPrefix(lower, ".prompt/"):
		return true
	case lower == ".codex", strings.HasPrefix(lower, ".codex/"):
		return true
	case lower == ".claude", strings.HasPrefix(lower, ".claude/"):
		return true
	}
	switch lower {
	case ".gitignore", ".gitattributes", "agents.md", "claude.md":
		return true
	default:
		return false
	}
}

type receiptRecord struct {
	Schema        string `json:"schema"`
	PlanID        string `json:"plan_id"`
	TaskID        string `json:"task_id"`
	Result        string `json:"result"`
	BaseCommit    string `json:"base_commit"`
	ContentHash   string `json:"content_hash"`
	EffectiveHash string `json:"effective_hash"`
}

func stagedCompletedPlan(root string, stagedNames []string) (*planner.Plan, error) {
	head := gitx.Head(root)
	receipts := map[string][]receiptRecord{}
	for _, name := range stagedNames {
		name = filepath.ToSlash(name)
		if !strings.HasPrefix(name, ".ebo/receipts/") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := gitx.IndexFile(root, name)
		if err != nil {
			continue
		}
		var receipt receiptRecord
		if json.Unmarshal(data, &receipt) == nil && receipt.Schema == "ebo.receipt/v1" && receipt.PlanID != "" {
			receipts[receipt.PlanID] = append(receipts[receipt.PlanID], receipt)
		}
	}
	for _, name := range stagedNames {
		name = filepath.ToSlash(name)
		if !strings.HasPrefix(name, ".ebo/plans/") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := gitx.IndexFile(root, name)
		if err != nil {
			continue
		}
		var plan planner.Plan
		if json.Unmarshal(data, &plan) != nil {
			continue
		}
		filePlanID := strings.TrimSuffix(strings.TrimPrefix(name, ".ebo/plans/"), ".json")
		if !strings.Contains(filePlanID, "/") && filePlanID == plan.ID && plan.Schema == planner.Schema && plan.Status == "completed" && plan.BaseCommit == head && receiptsCompletePlan(receipts[plan.ID], &plan) {
			return &plan, nil
		}
	}
	return nil, fmt.Errorf("staged source changes require a staged completed Ebo plan and receipt based on HEAD %s", head)
}

func receiptsCompletePlan(receipts []receiptRecord, plan *planner.Plan) bool {
	for _, task := range plan.Tasks {
		if task.Status != "passed" {
			return false
		}
		matched := false
		for _, receipt := range receipts {
			if receipt.Schema == "ebo.receipt/v1" && receipt.PlanID == plan.ID && receipt.TaskID == task.ID && receipt.Result == "passed" &&
				receipt.BaseCommit == plan.BaseCommit && receipt.ContentHash == task.ContentHash && receipt.EffectiveHash == task.EffectiveHash {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func runImport(args []string, out io.Writer) error {
	target, outDir, dryRun, err := parseImportArgs(args)
	if err != nil {
		return err
	}
	root, err := requireRoot()
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	files, err := inventoryFiles(root, absTarget)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(out, "would export evidence for %d file(s)\n", len(files))
		return nil
	}
	if outDir == "" {
		outDir = filepath.Join(project.NewPaths(root).RuntimeDir, "import-"+time.Now().UTC().Format("20060102-150405"))
	}
	payload := map[string]any{
		"schema":     "ebo.evidence/v1",
		"root":       filepath.ToSlash(root),
		"target":     filepath.ToSlash(absTarget),
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"files":      files,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(outDir, "evidence.json")
	if err := withProjectLock(root, "import", func() error {
		return project.WriteFileAtomic(path, data, 0o644)
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote evidence package %s\n", path)
	return nil
}

func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

func requireRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return project.FindRoot(cwd)
}

func withProjectLock(root, command string, fn func() error) error {
	lock, err := lockfile.Acquire(root, command)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

func printGateClosed(out io.Writer, reason, action string) {
	fmt.Fprintln(out, "EBO EXECUTION GATE: CLOSED")
	fmt.Fprintln(out, "source_edit: forbidden")
	fmt.Fprintf(out, "reason: %s\n", reason)
	if action != "" {
		fmt.Fprintf(out, "action: %s\n", action)
	}
}

func validateActiveTask(root string, active *execution.ActiveTask) (*planner.Plan, *planner.Task, error) {
	if active == nil {
		return nil, nil, execution.ErrNoActiveTask
	}
	head := gitx.Head(root)
	if head == "" {
		return nil, nil, fmt.Errorf("git baseline is missing")
	}
	if active.BaseCommit != head {
		return nil, nil, fmt.Errorf("active task base commit is %s but HEAD is %s", active.BaseCommit, head)
	}
	plan, err := planner.Load(root, active.PlanID)
	if err != nil {
		return nil, nil, err
	}
	if plan.BaseCommit != active.BaseCommit {
		return nil, nil, fmt.Errorf("active task and plan base commits do not match")
	}
	task, err := planner.FindTask(plan, active.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if task.Status != "running" {
		return nil, nil, fmt.Errorf("active task %s is %s, want running", task.ID, task.Status)
	}
	if task.PromptID != active.PromptID || task.ContentHash != active.ContentHash || task.EffectiveHash != active.EffectiveHash {
		return nil, nil, fmt.Errorf("active task no longer matches plan task %s", task.ID)
	}
	promptTree, err := tree.LoadProject(root)
	if err != nil {
		return nil, nil, err
	}
	if issues := promptTree.Validate(); len(issues) > 0 {
		return nil, nil, fmt.Errorf("tree invalid: %s", strings.Join(issues, "; "))
	}
	if err := validateTaskPrompt(promptTree, promptTree.Nodes[task.PromptID], task); err != nil {
		return nil, nil, err
	}
	return plan, task, nil
}

func validateTaskPrompt(t *tree.Tree, node *document.Prompt, task *planner.Task) error {
	if node == nil {
		return fmt.Errorf("prompt %s not found", task.PromptID)
	}
	if content := document.ContentHash(node); content != task.ContentHash {
		return fmt.Errorf("prompt %s content changed after task start", task.PromptID)
	}
	if effective := t.EffectiveHashes()[task.PromptID]; effective != task.EffectiveHash {
		return fmt.Errorf("prompt %s dependencies changed after task start", task.PromptID)
	}
	return nil
}

func defaultRootPrompt() string {
	return `---
schema: ebo.prompt/v1
id: project.root
title: Project Root
kind: project
parent:
revision: 1
origin: human
confidence: confirmed
state:
  spec: approved
  execution: adopted
  sync: in_sync
hash:
  satisfied:
links:
  references: []
---
## Intent

Capture the project as one logical Prompt Tree.

## Context

This node is the single root. Every non-root prompt must declare a parent that eventually reaches project.root.

## Acceptance

- The tree has exactly one project.root.
- Every non-root prompt has one valid parent.
`
}

func parseAgents(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return nil
	}
	var agents []string
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			agents = append(agents, item)
		}
	}
	return agents
}

func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(data)
	lines := []string{
		".ebo/cache/",
		".ebo/locks/",
		".ebo/tmp/",
		".ebo/runtime/sessions/",
		".ebo/runtime/logs/",
		".ebo/runtime/active-task.json",
	}
	changed := false
	for _, line := range lines {
		if !containsGitignoreLine(text, line) {
			if len(text) > 0 && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			text += line + "\n"
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return project.WriteFileAtomic(path, []byte(text), 0o644)
}

func readPromptFile(root, path string) (proposal.Source, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return proposal.Source{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return proposal.Source{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return proposal.Source{}, fmt.Errorf("refusing to read symlink %s", path)
	}
	if info.Size() > 2*1024*1024 {
		return proposal.Source{}, fmt.Errorf("%s is larger than 2 MiB", path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return proposal.Source{}, err
	}
	return proposal.Source{
		Kind: "file",
		Path: displayPath(root, abs),
		Name: filepath.Base(abs),
		Data: data,
	}, nil
}

func readPromptDir(root, dir string) ([]proposal.Source, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to read symlink %s", path)
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) == ".md" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no Markdown files found in %s", dir)
	}
	if len(paths) > 100 {
		return nil, fmt.Errorf("too many Markdown files: %d > 100", len(paths))
	}
	var total int64
	sources := make([]proposal.Source, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.Size() > 2*1024*1024 {
			return nil, fmt.Errorf("%s is larger than 2 MiB", path)
		}
		total += info.Size()
		if total > 10*1024*1024 {
			return nil, fmt.Errorf("total prompt input is larger than 10 MiB")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sources = append(sources, proposal.Source{
			Kind: "file",
			Path: displayPath(root, path),
			Name: filepath.Base(path),
			Data: data,
		})
	}
	return sources, nil
}

func displayPath(root, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, abs)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

func isTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func valueFlag(args []string, name string) string {
	prefix := "--" + name + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		if arg == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	flagName := "--" + name
	for _, arg := range args {
		if arg == flagName {
			return true
		}
	}
	return false
}

func printPlan(plan *planner.Plan, out io.Writer) error {
	fmt.Fprintf(out, "plan:       %s\n", plan.ID)
	fmt.Fprintf(out, "status:     %s\n", plan.Status)
	fmt.Fprintf(out, "root:       %s\n", emptyAsDash(plan.Root))
	fmt.Fprintf(out, "base:       %s\n", emptyAsDash(plan.BaseCommit))
	fmt.Fprintf(out, "created:    %s\n", plan.CreatedAt)
	fmt.Fprintf(out, "updated:    %s\n", plan.UpdatedAt)
	fmt.Fprintf(out, "tasks:      %d\n", len(plan.Tasks))
	for i, task := range plan.Tasks {
		fmt.Fprintf(out, "%d. %s  %s  %s\n", i+1, task.ID, task.Status, task.Title)
	}
	return nil
}

func parseExportArgs(args []string) (planID, format, outPath string, err error) {
	format = "markdown"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				err = fmt.Errorf("--format requires a value")
				return
			}
			format = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) {
				err = fmt.Errorf("--out requires a value")
				return
			}
			outPath = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "--format=") {
				format = strings.TrimPrefix(args[i], "--format=")
				continue
			}
			if strings.HasPrefix(args[i], "--out=") {
				outPath = strings.TrimPrefix(args[i], "--out=")
				continue
			}
			if strings.HasPrefix(args[i], "-") {
				err = fmt.Errorf("unknown export flag %s", args[i])
				return
			}
			if planID != "" {
				err = fmt.Errorf("export accepts one plan id")
				return
			}
			planID = args[i]
		}
	}
	if planID == "" {
		err = fmt.Errorf("usage: ebo export <plan-id> [--format markdown|json] [--out path]")
		return
	}
	return
}

func parseContextArgs(args []string) (string, int, string, error) {
	depth := 2
	outPath := ""
	nodeID := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--depth":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--depth requires a value")
			}
			var n int
			if _, err := fmt.Sscanf(args[i+1], "%d", &n); err != nil {
				return "", 0, "", fmt.Errorf("--depth must be a number")
			}
			depth = n
			i++
		case "--out":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--out requires a value")
			}
			outPath = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", 0, "", fmt.Errorf("unknown flag %s", args[i])
			}
			if nodeID != "" {
				return "", 0, "", fmt.Errorf("context accepts one node id")
			}
			nodeID = args[i]
		}
	}
	if nodeID == "" {
		return "", 0, "", fmt.Errorf("usage: ebo context <node-id> [--depth 2] [--out path]")
	}
	if depth < 0 {
		depth = 0
	}
	return nodeID, depth, outPath, nil
}

func buildContextPayload(t *tree.Tree, nodeID string, depth int) map[string]any {
	selected := map[string]bool{nodeID: true}
	cur := nodeID
	for i := 0; i < depth; i++ {
		node := t.Nodes[cur]
		if node == nil || node.Parent == "" {
			break
		}
		selected[node.Parent] = true
		cur = node.Parent
	}
	frontier := []string{nodeID}
	for i := 0; i < depth; i++ {
		var next []string
		for _, id := range frontier {
			for _, child := range t.Children(id) {
				if !selected[child] {
					selected[child] = true
					next = append(next, child)
				}
			}
		}
		frontier = next
	}
	for typ, links := range t.Nodes[nodeID].Links {
		_ = typ
		for _, link := range links {
			if t.Nodes[link.ID] != nil {
				selected[link.ID] = true
			}
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		node := t.Nodes[id]
		nodes = append(nodes, map[string]any{
			"id":     node.ID,
			"title":  node.Title,
			"kind":   node.Kind,
			"parent": node.Parent,
			"state":  node.State,
			"links":  node.Links,
			"body":   strings.TrimSpace(node.Body),
		})
	}
	return map[string]any{
		"schema": "ebo.context/v1",
		"root":   project.RootID,
		"focus":  nodeID,
		"depth":  depth,
		"nodes":  nodes,
	}
}

func filterOrderFromRoot(t *tree.Tree, order []string, id string) []string {
	selected := map[string]bool{}
	var mark func(string)
	mark = func(cur string) {
		if selected[cur] {
			return
		}
		selected[cur] = true
		for _, child := range t.Children(cur) {
			mark(child)
		}
	}
	mark(id)
	var out []string
	for _, task := range order {
		if selected[task] {
			out = append(out, task)
		}
	}
	return out
}

func parseImportArgs(args []string) (target, outDir string, dryRun bool, err error) {
	target = "."
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--out":
			if i+1 >= len(args) {
				err = fmt.Errorf("--out requires a value")
				return
			}
			outDir = args[i+1]
			i++
		case "--history-depth":
			if i+1 >= len(args) {
				err = fmt.Errorf("--history-depth requires a value")
				return
			}
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				err = fmt.Errorf("unknown import flag %s", args[i])
				return
			}
			target = args[i]
		}
	}
	return
}

func inventoryFiles(root, target string) ([]map[string]any, error) {
	var files []map[string]any
	err := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".ebo", "node_modules", "dist", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, map[string]any{
			"path":   displayPath(root, path),
			"size":   info.Size(),
			"sha256": document.SHA256(data),
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i]["path"].(string) < files[j]["path"].(string)
	})
	return files, err
}

func sortedLinkTypes(links map[string][]document.Link) []string {
	types := make([]string, 0, len(links))
	for typ := range links {
		types = append(types, typ)
	}
	sort.Strings(types)
	return types
}

func gitInside(root string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

func containsManagedBlock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "<!-- EBO:START -->") && strings.Contains(text, "<!-- EBO:END -->")
}

func containsGitignoreLine(text, line string) bool {
	for _, existing := range strings.Split(text, "\n") {
		if strings.TrimSpace(existing) == line {
			return true
		}
	}
	return false
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
