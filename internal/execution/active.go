package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lqw905/Ebo/internal/project"
)

const Schema = "ebo.active-task/v1"

var ErrNoActiveTask = errors.New("no active Ebo task")

type ActiveTask struct {
	Schema        string `json:"schema"`
	PlanID        string `json:"plan_id"`
	TaskID        string `json:"task_id"`
	PromptID      string `json:"prompt_id"`
	ContentHash   string `json:"content_hash"`
	EffectiveHash string `json:"effective_hash"`
	BaseCommit    string `json:"base_commit"`
	StartedAt     string `json:"started_at"`
}

func New(planID, taskID, promptID, contentHash, effectiveHash, baseCommit string) *ActiveTask {
	return &ActiveTask{
		Schema:        Schema,
		PlanID:        planID,
		TaskID:        taskID,
		PromptID:      promptID,
		ContentHash:   contentHash,
		EffectiveHash: effectiveHash,
		BaseCommit:    baseCommit,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

func Save(root string, active *ActiveTask) error {
	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return project.WriteFileAtomic(project.NewPaths(root).ActiveTask, data, 0o644)
}

func Load(root string) (*ActiveTask, error) {
	data, err := os.ReadFile(project.NewPaths(root).ActiveTask)
	if os.IsNotExist(err) {
		return nil, ErrNoActiveTask
	}
	if err != nil {
		return nil, err
	}
	var active ActiveTask
	if err := json.Unmarshal(data, &active); err != nil {
		return nil, err
	}
	if active.Schema != Schema {
		return nil, errors.New("unsupported active task schema")
	}
	required := map[string]string{
		"plan_id":        active.PlanID,
		"task_id":        active.TaskID,
		"prompt_id":      active.PromptID,
		"content_hash":   active.ContentHash,
		"effective_hash": active.EffectiveHash,
		"base_commit":    active.BaseCommit,
		"started_at":     active.StartedAt,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("active task is missing %s", name)
		}
	}
	return &active, nil
}

func Clear(root string) error {
	err := os.Remove(project.NewPaths(root).ActiveTask)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
