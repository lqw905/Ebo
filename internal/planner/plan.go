package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lqw905/Ebo/internal/document"
	"github.com/lqw905/Ebo/internal/gitx"
	"github.com/lqw905/Ebo/internal/project"
	"github.com/lqw905/Ebo/internal/tree"
)

const Schema = "ebo.plan/v1"

type Plan struct {
	Schema     string   `json:"schema"`
	ID         string   `json:"id"`
	Root       string   `json:"root,omitempty"`
	Status     string   `json:"status"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	BaseCommit string   `json:"base_commit,omitempty"`
	Nodes      []Node   `json:"nodes"`
	Order      []string `json:"order"`
	Tasks      []Task   `json:"tasks"`
}

type Node struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	ContentHash   string `json:"content_hash"`
	EffectiveHash string `json:"effective_hash"`
}

type Task struct {
	ID            string `json:"id"`
	PromptID      string `json:"prompt_id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	ContentHash   string `json:"content_hash"`
	EffectiveHash string `json:"effective_hash"`
	Result        string `json:"result,omitempty"`
	Note          string `json:"note,omitempty"`
	ReportedAt    string `json:"reported_at,omitempty"`
}

func Create(root string, promptTree *tree.Tree, rootNode string) (*Plan, error) {
	if issues := promptTree.Validate(); len(issues) > 0 {
		return nil, fmt.Errorf("tree invalid: %s", strings.Join(issues, "; "))
	}
	order := promptTree.ExecutionOrder()
	if rootNode != "" {
		if promptTree.Nodes[rootNode] == nil {
			return nil, fmt.Errorf("node %s not found", rootNode)
		}
		order = filterOrderFromRoot(promptTree, order, rootNode)
	}
	effective := promptTree.EffectiveHashes()
	nodes := make([]Node, 0, len(order))
	tasks := make([]Task, 0, len(order))
	for _, id := range order {
		prompt := promptTree.Nodes[id]
		node := Node{
			ID:            id,
			Title:         prompt.Title,
			Kind:          prompt.Kind,
			ContentHash:   document.ContentHash(prompt),
			EffectiveHash: effective[id],
		}
		nodes = append(nodes, node)
		tasks = append(tasks, Task{
			ID:            id,
			PromptID:      id,
			Title:         prompt.Title,
			Kind:          prompt.Kind,
			Status:        "pending",
			ContentHash:   node.ContentHash,
			EffectiveHash: node.EffectiveHash,
		})
	}
	baseCommit := gitx.Head(root)
	if baseCommit == "" {
		return nil, fmt.Errorf("git baseline is missing; commit the initialized project before creating an execution plan")
	}
	now := time.Now().UTC()
	hash := planHash(rootNode, nodes, baseCommit)
	id := fmt.Sprintf("plan-%s-%s", now.Format("20060102-150405"), document.ShortHash(hash, 8))
	status := "planned"
	if len(tasks) == 0 {
		status = "empty"
	}
	return &Plan{
		Schema:     Schema,
		ID:         id,
		Root:       rootNode,
		Status:     status,
		CreatedAt:  now.Format(time.RFC3339),
		UpdatedAt:  now.Format(time.RFC3339),
		BaseCommit: baseCommit,
		Nodes:      nodes,
		Order:      append([]string{}, order...),
		Tasks:      tasks,
	}, nil
}

func Save(root string, plan *Plan) error {
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return project.WriteFileAtomic(Path(root, plan.ID), data, 0o644)
}

func Load(root, id string) (*Plan, error) {
	data, err := os.ReadFile(Path(root, id))
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	if plan.Schema != Schema {
		return nil, fmt.Errorf("%s has unsupported schema %q", id, plan.Schema)
	}
	return &plan, nil
}

func List(root string) ([]*Plan, error) {
	entries, err := os.ReadDir(project.NewPaths(root).PlansDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var plans []*Plan
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		plan, err := Load(root, id)
		if err == nil {
			plans = append(plans, plan)
		}
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].CreatedAt < plans[j].CreatedAt
	})
	return plans, nil
}

func LatestActive(root string) (*Plan, error) {
	plans, err := List(root)
	if err != nil {
		return nil, err
	}
	for i := len(plans) - 1; i >= 0; i-- {
		switch plans[i].Status {
		case "planned", "running", "failed", "blocked":
			return plans[i], nil
		}
	}
	return nil, nil
}

func NextTask(plan *Plan) *Task {
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == "running" {
			return &plan.Tasks[i]
		}
	}
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == "pending" || plan.Tasks[i].Status == "failed" || plan.Tasks[i].Status == "blocked" {
			return &plan.Tasks[i]
		}
	}
	return nil
}

func StartTask(plan *Plan, taskID string) (*Task, error) {
	task, err := FindTask(plan, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case "running":
		plan.Status = "running"
		return task, nil
	case "pending", "failed", "blocked":
		task.Status = "running"
		task.Result = ""
		task.ReportedAt = ""
		plan.Status = "running"
		return task, nil
	default:
		return nil, fmt.Errorf("task %s is %s and cannot be started", task.ID, task.Status)
	}
}

func FindTask(plan *Plan, taskID string) (*Task, error) {
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID || plan.Tasks[i].PromptID == taskID {
			return &plan.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("task %s not found in plan %s", taskID, plan.ID)
}

func Report(root string, plan *Plan, taskID, result, note string) (*Task, error) {
	task, err := FindTask(plan, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != "running" {
		return nil, fmt.Errorf("task %s is %s; run ebo next before reporting it", task.ID, task.Status)
	}
	task.Status = result
	task.Result = result
	task.Note = note
	task.ReportedAt = time.Now().UTC().Format(time.RFC3339)
	plan.Status = statusFromTasks(plan.Tasks)
	if err := Save(root, plan); err != nil {
		return nil, err
	}
	return task, nil
}

func Abort(plan *Plan) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == "running" {
			plan.Tasks[i].Status = "aborted"
			plan.Tasks[i].Result = "aborted"
			plan.Tasks[i].ReportedAt = now
		}
	}
	plan.Status = "aborted"
}

func Path(root, id string) string {
	return filepath.Join(project.NewPaths(root).PlansDir, id+".json")
}

func TaskPackage(plan *Plan, prompt *document.Prompt, task *Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# TASK")
	fmt.Fprintf(&b, "Implement prompt `%s`: %s\n\n", task.PromptID, task.Title)
	fmt.Fprintf(&b, "Plan: `%s`\nTask: `%s`\n\n", plan.ID, task.ID)
	fmt.Fprintln(&b, "# CONTEXT")
	fmt.Fprintf(&b, "Kind: %s\nParent: %s\nContent Hash: %s\nEffective Hash: %s\n\n", prompt.Kind, emptyAsDash(prompt.Parent), task.ContentHash, task.EffectiveHash)
	if strings.TrimSpace(prompt.Body) != "" {
		fmt.Fprintln(&b, strings.TrimSpace(prompt.Body))
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "# WRITE SCOPE")
	if len(prompt.Scope.Allow) == 0 {
		fmt.Fprintln(&b, "- allow: all source paths except protected Ebo, Git, and Agent control files")
	} else {
		for _, pattern := range prompt.Scope.Allow {
			fmt.Fprintf(&b, "- allow: %s\n", pattern)
		}
	}
	for _, pattern := range prompt.Scope.Deny {
		fmt.Fprintf(&b, "- deny: %s\n", pattern)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "# LINKS")
	wroteLinks := false
	for _, typ := range sortedLinkTypes(prompt.Links) {
		for _, link := range prompt.Links[typ] {
			wroteLinks = true
			fmt.Fprintf(&b, "- %s: %s", typ, link.ID)
			if link.Reason != "" {
				fmt.Fprintf(&b, " (%s)", link.Reason)
			}
			fmt.Fprintln(&b)
		}
	}
	if !wroteLinks {
		fmt.Fprintln(&b, "- none")
	}
	fmt.Fprintln(&b, "\n# REPORT CONTRACT")
	fmt.Fprintf(&b, "When finished, run: ebo report %s --plan %s --result passed --note \"...\"\n", task.ID, plan.ID)
	return b.String()
}

func AuthorizedTaskPackage(plan *Plan, prompt *document.Prompt, task *Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "EBO EXECUTION GATE: OPEN")
	fmt.Fprintln(&b, "source_edit: allowed")
	fmt.Fprintf(&b, "plan: %s\ntask: %s\nprompt: %s\n\n", plan.ID, task.ID, task.PromptID)
	b.WriteString(TaskPackage(plan, prompt, task))
	return b.String()
}

func planHash(rootNode string, nodes []Node, baseCommit string) string {
	payload := struct {
		Schema     string `json:"schema"`
		Root       string `json:"root,omitempty"`
		BaseCommit string `json:"base_commit,omitempty"`
		Nodes      []Node `json:"nodes"`
	}{
		Schema:     Schema,
		Root:       rootNode,
		BaseCommit: baseCommit,
		Nodes:      nodes,
	}
	data, _ := json.Marshal(payload)
	return document.SHA256(data)
}

func statusFromTasks(tasks []Task) string {
	if len(tasks) == 0 {
		return "empty"
	}
	allPassed := true
	for _, task := range tasks {
		switch task.Status {
		case "passed":
		case "failed":
			return "failed"
		case "blocked":
			return "blocked"
		default:
			allPassed = false
		}
	}
	if allPassed {
		return "completed"
	}
	return "running"
}

func filterOrderFromRoot(promptTree *tree.Tree, order []string, id string) []string {
	selected := map[string]bool{}
	var mark func(string)
	mark = func(cur string) {
		if selected[cur] {
			return
		}
		selected[cur] = true
		for _, child := range promptTree.Children(cur) {
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

func sortedLinkTypes(links map[string][]document.Link) []string {
	types := make([]string, 0, len(links))
	for typ := range links {
		types = append(types, typ)
	}
	sort.Strings(types)
	return types
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
