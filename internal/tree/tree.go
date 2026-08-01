package tree

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lqw905/Ebo/internal/document"
	"github.com/lqw905/Ebo/internal/project"
)

type Tree struct {
	Root       string
	Nodes      map[string]*document.Prompt
	Files      map[string]string
	LoadErrors []string
}

type TaskResultUpdate struct {
	PromptID      string
	Result        string
	ContentHash   string
	EffectiveHash string
}

func Load(root string) (*Tree, error) {
	t := &Tree{
		Root:  root,
		Nodes: map[string]*document.Prompt{},
		Files: map[string]string{},
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return t, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.LoadErrors = append(t.LoadErrors, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		prompt, err := document.ParsePrompt(data, path)
		if err != nil {
			t.LoadErrors = append(t.LoadErrors, err.Error())
			return nil
		}
		if old, exists := t.Files[prompt.ID]; exists {
			t.LoadErrors = append(t.LoadErrors, fmt.Sprintf("duplicate prompt id %s in %s and %s", prompt.ID, old, path))
			return nil
		}
		t.Nodes[prompt.ID] = prompt
		t.Files[prompt.ID] = path
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func LoadProject(root string) (*Tree, error) {
	return Load(project.NewPaths(root).TreeDir)
}

func ApplyTaskResult(root string, update TaskResultUpdate) error {
	t, err := LoadProject(root)
	if err != nil {
		return err
	}
	if issues := t.Validate(); len(issues) > 0 {
		return fmt.Errorf("tree invalid: %s", strings.Join(issues, "; "))
	}
	prompt := t.Nodes[update.PromptID]
	if prompt == nil {
		return fmt.Errorf("prompt %s not found", update.PromptID)
	}
	effective := t.EffectiveHashes()
	actualContent := document.ContentHash(prompt)
	actualEffective := effective[update.PromptID]
	if update.ContentHash != "" && update.ContentHash != actualContent {
		return fmt.Errorf("prompt %s content changed since plan was created: expected %s, got %s", update.PromptID, update.ContentHash, actualContent)
	}
	if update.EffectiveHash != "" && update.EffectiveHash != actualEffective {
		return fmt.Errorf("prompt %s effective hash changed since plan was created: expected %s, got %s", update.PromptID, update.EffectiveHash, actualEffective)
	}
	prompt.Hash.Content = actualContent
	prompt.Hash.Effective = actualEffective
	switch update.Result {
	case "passed":
		prompt.State.Execution = "passed"
		prompt.State.Sync = "in_sync"
		prompt.Hash.Satisfied = actualEffective
	case "failed":
		prompt.State.Execution = "failed"
		prompt.State.Sync = "dirty"
	case "blocked":
		prompt.State.Execution = "blocked"
		prompt.State.Sync = "dirty"
	default:
		return fmt.Errorf("unsupported task result %q", update.Result)
	}
	data := document.RenderPrompt(prompt)
	if _, err := document.ParsePrompt(data, t.Files[update.PromptID]); err != nil {
		return err
	}
	return project.WriteFileAtomic(t.Files[update.PromptID], data, 0o644)
}

func (t *Tree) Validate() []string {
	issues := append([]string{}, t.LoadErrors...)
	if len(t.Nodes) == 0 {
		return append(issues, "prompt tree is empty")
	}
	root, hasRoot := t.Nodes[project.RootID]
	if !hasRoot {
		issues = append(issues, "prompt tree must contain project.root")
	} else if strings.TrimSpace(root.Parent) != "" {
		issues = append(issues, "project.root must not declare a parent")
	}
	for id, prompt := range t.Nodes {
		issues = append(issues, document.ValidateBasic(prompt)...)
		if id != project.RootID {
			if prompt.Parent == "" {
				issues = append(issues, fmt.Sprintf("%s must declare parent", id))
			} else if _, exists := t.Nodes[prompt.Parent]; !exists {
				issues = append(issues, fmt.Sprintf("%s parent %s does not exist", id, prompt.Parent))
			}
		}
		for typ, links := range prompt.Links {
			for _, link := range links {
				if _, exists := t.Nodes[link.ID]; !exists {
					issues = append(issues, fmt.Sprintf("%s %s link target %s does not exist", id, typ, link.ID))
				}
			}
		}
	}
	issues = append(issues, t.validateParentCycles()...)
	issues = append(issues, t.validateDependsOnCycles()...)
	return dedupe(issues)
}

func (t *Tree) IDs() []string {
	ids := make([]string, 0, len(t.Nodes))
	for id := range t.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (t *Tree) Children(parent string) []string {
	var ids []string
	for id, prompt := range t.Nodes {
		if prompt.Parent == parent {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (t *Tree) EffectiveHashes() map[string]string {
	out := map[string]string{}
	visiting := map[string]bool{}
	var calc func(id string) string
	calc = func(id string) string {
		if hash, ok := out[id]; ok {
			return hash
		}
		prompt := t.Nodes[id]
		if prompt == nil {
			return ""
		}
		if visiting[id] {
			return document.ContentHash(prompt)
		}
		visiting[id] = true
		deps := make([]string, 0)
		for _, link := range prompt.Links["depends_on"] {
			if _, exists := t.Nodes[link.ID]; exists {
				deps = append(deps, link.ID+":"+calc(link.ID))
			}
		}
		sort.Strings(deps)
		payload := struct {
			Content string   `json:"content"`
			Depends []string `json:"depends,omitempty"`
		}{
			Content: document.ContentHash(prompt),
			Depends: deps,
		}
		data, _ := json.Marshal(payload)
		hash := document.SHA256(data)
		out[id] = hash
		delete(visiting, id)
		return hash
	}
	for _, id := range t.IDs() {
		calc(id)
	}
	return out
}

func (t *Tree) DirtyNodes() []string {
	effective := t.EffectiveHashes()
	var dirty []string
	for _, id := range t.IDs() {
		prompt := t.Nodes[id]
		if prompt.ID == project.RootID {
			continue
		}
		if prompt.Hash.Satisfied == "" || prompt.Hash.Satisfied != effective[id] || prompt.State.Execution == "failed" || prompt.State.Execution == "blocked" {
			dirty = append(dirty, id)
		}
	}
	return dirty
}

// Stats summarizes the shape and health of a prompt tree.
type Stats struct {
	Nodes      int
	MaxDepth   int
	DepthCount map[int]int
	InSync     int
	Dirty      int
	BodyBytes  int
}

// Stats reports tree shape (counts, depth histogram, body size) and health
// (in_sync vs dirty). InSync counts non-root nodes whose satisfied hash still
// matches their effective hash and whose execution state is not failed/blocked,
// mirroring DirtyNodes.
func (t *Tree) Stats() Stats {
	ids := t.IDs()
	dirtySet := map[string]bool{}
	for _, id := range t.DirtyNodes() {
		dirtySet[id] = true
	}
	depths := t.depthsFromRoot()

	st := Stats{
		Nodes:      len(ids),
		DepthCount: map[int]int{},
		Dirty:      len(dirtySet),
	}
	for _, id := range ids {
		node := t.Nodes[id]
		st.BodyBytes += len(node.Body)
		d, ok := depths[id]
		if !ok {
			d = depthByParent(t, id)
		}
		st.DepthCount[d]++
		if d > st.MaxDepth {
			st.MaxDepth = d
		}
		if id == project.RootID {
			continue
		}
		if !dirtySet[id] {
			st.InSync++
		}
	}
	return st
}

func (t *Tree) depthsFromRoot() map[string]int {
	depths := map[string]int{}
	if _, ok := t.Nodes[project.RootID]; !ok {
		return depths
	}
	depths[project.RootID] = 0
	seen := map[string]bool{project.RootID: true}
	frontier := []string{project.RootID}
	for len(frontier) > 0 {
		var next []string
		for _, id := range frontier {
			for _, child := range t.Children(id) {
				if seen[child] {
					continue
				}
				seen[child] = true
				depths[child] = depths[id] + 1
				next = append(next, child)
			}
		}
		frontier = next
	}
	return depths
}

func depthByParent(t *Tree, id string) int {
	d := 0
	seen := map[string]bool{}
	cur := id
	for {
		node := t.Nodes[cur]
		if node == nil || node.Parent == "" {
			return d
		}
		if seen[cur] {
			return d
		}
		seen[cur] = true
		cur = node.Parent
		d++
	}
}

func (t *Tree) ExecutionOrder() []string {
	allDirty := map[string]bool{}
	for _, id := range t.DirtyNodes() {
		allDirty[id] = true
	}
	readyMemo := map[string]bool{}
	checked := map[string]bool{}
	checking := map[string]bool{}
	var ready func(string) bool
	ready = func(id string) bool {
		if !allDirty[id] {
			return true
		}
		if checked[id] {
			return readyMemo[id]
		}
		if checking[id] {
			return false
		}
		checking[id] = true
		prompt := t.Nodes[id]
		isReady := prompt != nil && prompt.State.Spec == "approved"
		if isReady {
			for _, link := range prompt.Links["depends_on"] {
				if allDirty[link.ID] && !ready(link.ID) {
					isReady = false
					break
				}
			}
		}
		delete(checking, id)
		checked[id] = true
		readyMemo[id] = isReady
		return isReady
	}
	dirtySet := map[string]bool{}
	for id := range allDirty {
		if ready(id) {
			dirtySet[id] = true
		}
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	var order []string
	var visit func(string)
	visit = func(id string) {
		if visited[id] || visiting[id] {
			return
		}
		visiting[id] = true
		prompt := t.Nodes[id]
		if prompt != nil {
			deps := make([]string, 0)
			for _, link := range prompt.Links["depends_on"] {
				if dirtySet[link.ID] {
					deps = append(deps, link.ID)
				}
			}
			sort.Strings(deps)
			for _, dep := range deps {
				visit(dep)
			}
		}
		delete(visiting, id)
		visited[id] = true
		if dirtySet[id] {
			order = append(order, id)
		}
	}
	ids := make([]string, 0, len(dirtySet))
	for id := range dirtySet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		visit(id)
	}
	return order
}

func (t *Tree) validateParentCycles() []string {
	var issues []string
	for id := range t.Nodes {
		seen := map[string]bool{}
		cur := id
		for cur != "" && cur != project.RootID {
			if seen[cur] {
				issues = append(issues, fmt.Sprintf("parent cycle detected at %s", cur))
				break
			}
			seen[cur] = true
			prompt := t.Nodes[cur]
			if prompt == nil {
				break
			}
			cur = prompt.Parent
		}
	}
	return issues
}

func (t *Tree) validateDependsOnCycles() []string {
	var issues []string
	color := map[string]int{}
	var stack []string
	var visit func(string)
	visit = func(id string) {
		if color[id] == 2 {
			return
		}
		if color[id] == 1 {
			issues = append(issues, fmt.Sprintf("depends_on cycle detected: %s -> %s", strings.Join(stack, " -> "), id))
			return
		}
		color[id] = 1
		stack = append(stack, id)
		prompt := t.Nodes[id]
		if prompt != nil {
			deps := make([]string, 0)
			for _, link := range prompt.Links["depends_on"] {
				deps = append(deps, link.ID)
			}
			sort.Strings(deps)
			for _, dep := range deps {
				if _, exists := t.Nodes[dep]; exists {
					visit(dep)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = 2
	}
	for _, id := range t.IDs() {
		visit(id)
	}
	return issues
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
