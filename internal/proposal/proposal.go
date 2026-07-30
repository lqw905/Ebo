package proposal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lqw905/Ebo/internal/document"
	"github.com/lqw905/Ebo/internal/project"
	"github.com/lqw905/Ebo/internal/tree"
)

const Schema = "ebo.proposal/v1"

type Source struct {
	Kind string
	Path string
	Name string
	Data []byte
}

type Meta struct {
	Schema       string       `json:"schema"`
	ID           string       `json:"id"`
	Status       string       `json:"status"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
	ProposalHash string       `json:"proposal_hash"`
	Sources      []SourceMeta `json:"sources"`
	Nodes        []NodeMeta   `json:"nodes"`
	ApprovedAt   string       `json:"approved_at,omitempty"`
	ApprovedHash string       `json:"approved_hash,omitempty"`
	RejectedAt   string       `json:"rejected_at,omitempty"`
	RejectReason string       `json:"reject_reason,omitempty"`
	AppliedAt    string       `json:"applied_at,omitempty"`
}

type SourceMeta struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	StoredPath string `json:"stored_path"`
	SHA256     string `json:"sha256"`
}

type NodeMeta struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Parent      string `json:"parent,omitempty"`
	ContentHash string `json:"content_hash"`
}

func Create(root string, sources []Source, dryRun bool) (*Meta, error) {
	meta, _, err := buildMeta(sources, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if dryRun {
		return meta, nil
	}
	paths := project.NewPaths(root)
	proposalDir := filepath.Join(paths.ProposalsDir, meta.ID)
	sourceDir := filepath.Join(proposalDir, "sources")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return nil, err
	}
	for i, source := range sources {
		name := source.Name
		if name == "" {
			name = source.Path
		}
		stored := filepath.Join("sources", fmt.Sprintf("%02d-%s", i+1, project.SafeFilename(name)))
		meta.Sources[i].StoredPath = filepath.ToSlash(stored)
		if err := project.WriteFileAtomic(filepath.Join(proposalDir, stored), source.Data, 0o644); err != nil {
			return nil, err
		}
	}
	if err := Save(root, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func Load(root, id string) (*Meta, error) {
	data, err := os.ReadFile(metaPath(root, id))
	if err != nil {
		return nil, err
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.Schema != Schema {
		return nil, fmt.Errorf("%s has unsupported schema %q", id, meta.Schema)
	}
	return &meta, nil
}

func Save(root string, meta *Meta) error {
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return project.WriteFileAtomic(metaPath(root, meta.ID), data, 0o644)
}

func List(root string) ([]*Meta, error) {
	paths := project.NewPaths(root)
	entries, err := os.ReadDir(paths.ProposalsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Meta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := Load(root, entry.Name())
		if err != nil {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}

func RecomputeHash(root string, meta *Meta) (string, error) {
	actual, _, _, err := rebuildFromStored(root, meta)
	if err != nil {
		return "", err
	}
	return actual.ProposalHash, nil
}

func Approve(root, id, hash string) (*Meta, error) {
	meta, err := Load(root, id)
	if err != nil {
		return nil, err
	}
	actual, _, _, err := rebuildFromStored(root, meta)
	if err != nil {
		return nil, err
	}
	if actual.ProposalHash != meta.ProposalHash {
		return nil, fmt.Errorf("proposal content changed: expected %s, got %s", meta.ProposalHash, actual.ProposalHash)
	}
	if hash != meta.ProposalHash {
		return nil, fmt.Errorf("approval hash does not match proposal hash")
	}
	meta.Status = "approved"
	meta.ApprovedAt = time.Now().UTC().Format(time.RFC3339)
	meta.ApprovedHash = hash
	meta.RejectedAt = ""
	meta.RejectReason = ""
	if err := Save(root, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func Reject(root, id, reason string) (*Meta, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("reject reason is required")
	}
	meta, err := Load(root, id)
	if err != nil {
		return nil, err
	}
	meta.Status = "rejected"
	meta.RejectedAt = time.Now().UTC().Format(time.RFC3339)
	meta.RejectReason = reason
	if err := Save(root, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func Apply(root, id string) (*Meta, error) {
	meta, err := Load(root, id)
	if err != nil {
		return nil, err
	}
	if meta.Status != "approved" {
		return nil, fmt.Errorf("proposal %s is %s; approve it first", id, meta.Status)
	}
	actual, prompts, _, err := rebuildFromStored(root, meta)
	if err != nil {
		return nil, err
	}
	if actual.ProposalHash != meta.ProposalHash || meta.ApprovedHash != actual.ProposalHash {
		return nil, fmt.Errorf("approved hash is no longer valid")
	}

	paths := project.NewPaths(root)
	applyRoot := filepath.Join(paths.TmpDir, "apply-"+id+"-"+time.Now().UTC().Format("20060102150405"))
	candidateTree := filepath.Join(applyRoot, "tree")
	backupTree := filepath.Join(applyRoot, "backup-tree")
	if err := os.RemoveAll(applyRoot); err != nil {
		return nil, err
	}
	if err := project.CopyDir(paths.TreeDir, candidateTree); err != nil {
		return nil, err
	}
	for _, prompt := range prompts {
		target, err := project.NodePathForID(candidateTree, prompt.ID)
		if err != nil {
			return nil, err
		}
		approvedData := approvedPromptData(prompt)
		if err := project.WriteFileAtomic(target, approvedData, 0o644); err != nil {
			return nil, err
		}
	}
	candidate, err := tree.Load(candidateTree)
	if err != nil {
		return nil, err
	}
	if issues := candidate.Validate(); len(issues) > 0 {
		return nil, fmt.Errorf("candidate tree is invalid:\n%s", strings.Join(issues, "\n"))
	}
	if err := project.ReplaceDir(paths.TreeDir, candidateTree, backupTree); err != nil {
		return nil, err
	}
	meta.Status = "applied"
	meta.AppliedAt = time.Now().UTC().Format(time.RFC3339)
	if err := Save(root, meta); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(applyRoot)
	return meta, nil
}

func approvedPromptData(prompt *document.Prompt) []byte {
	prompt.State.Spec = "approved"
	prompt.State.Execution = "not_started"
	prompt.State.Sync = "dirty"
	prompt.Hash.Content = ""
	prompt.Hash.Effective = ""
	prompt.Hash.Satisfied = ""
	return document.RenderPrompt(prompt)
}

func buildMeta(sources []Source, now time.Time) (*Meta, []*document.Prompt, error) {
	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("no prompt sources provided")
	}
	prompts := make([]*document.Prompt, 0, len(sources))
	sourceMeta := make([]SourceMeta, 0, len(sources))
	nodeMeta := make([]NodeMeta, 0, len(sources))
	seen := map[string]string{}

	for i, source := range sources {
		if len(source.Data) == 0 {
			return nil, nil, fmt.Errorf("%s is empty", source.Path)
		}
		prompt, err := document.ParsePrompt(source.Data, source.Path)
		if err != nil {
			return nil, nil, err
		}
		if issues := document.ValidateBasic(prompt); len(issues) > 0 {
			return nil, nil, fmt.Errorf("%s", strings.Join(issues, "\n"))
		}
		if old, exists := seen[prompt.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate prompt id %s in %s and %s", prompt.ID, old, source.Path)
		}
		seen[prompt.ID] = source.Path
		prompts = append(prompts, prompt)
		sourceMeta = append(sourceMeta, SourceMeta{
			Kind:   source.Kind,
			Path:   source.Path,
			SHA256: document.SHA256(source.Data),
		})
		nodeMeta = append(nodeMeta, NodeMeta{
			ID:          prompt.ID,
			Title:       prompt.Title,
			Kind:        prompt.Kind,
			Parent:      prompt.Parent,
			ContentHash: document.ContentHash(prompt),
		})
		if sourceMeta[i].Kind == "" {
			sourceMeta[i].Kind = "file"
		}
	}

	hash := proposalHash(sourceMeta, nodeMeta)
	id := fmt.Sprintf("proposal-%s-%s", now.Format("20060102-150405"), document.ShortHash(hash, 8))
	meta := &Meta{
		Schema:       Schema,
		ID:           id,
		Status:       "proposed",
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
		ProposalHash: hash,
		Sources:      sourceMeta,
		Nodes:        nodeMeta,
	}
	return meta, prompts, nil
}

func rebuildFromStored(root string, meta *Meta) (*Meta, []*document.Prompt, [][]byte, error) {
	proposalDir := filepath.Join(project.NewPaths(root).ProposalsDir, meta.ID)
	sources := make([]Source, 0, len(meta.Sources))
	data := make([][]byte, 0, len(meta.Sources))
	for _, source := range meta.Sources {
		if source.StoredPath == "" {
			return nil, nil, nil, fmt.Errorf("proposal %s source has no stored path", meta.ID)
		}
		path := filepath.Join(proposalDir, filepath.FromSlash(source.StoredPath))
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, nil, err
		}
		data = append(data, bytes)
		sources = append(sources, Source{
			Kind: source.Kind,
			Path: source.Path,
			Name: filepath.Base(source.StoredPath),
			Data: bytes,
		})
	}
	actual, prompts, err := buildMeta(sources, parseTimeOrNow(meta.CreatedAt))
	if err != nil {
		return nil, nil, nil, err
	}
	actual.ID = meta.ID
	return actual, prompts, data, nil
}

func proposalHash(sources []SourceMeta, nodes []NodeMeta) string {
	type hashSource struct {
		Kind        string `json:"kind"`
		Path        string `json:"path"`
		SHA256      string `json:"sha256"`
		NodeID      string `json:"node_id"`
		ContentHash string `json:"content_hash"`
	}
	payload := struct {
		Schema  string       `json:"schema"`
		Sources []hashSource `json:"sources"`
	}{Schema: Schema}
	for i, source := range sources {
		payload.Sources = append(payload.Sources, hashSource{
			Kind:        source.Kind,
			Path:        source.Path,
			SHA256:      source.SHA256,
			NodeID:      nodes[i].ID,
			ContentHash: nodes[i].ContentHash,
		})
	}
	data, _ := json.Marshal(payload)
	return document.SHA256(data)
}

func metaPath(root, id string) string {
	return filepath.Join(project.NewPaths(root).ProposalsDir, id, "proposal.json")
}

func parseTimeOrNow(value string) time.Time {
	if value == "" {
		return time.Now().UTC()
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}
