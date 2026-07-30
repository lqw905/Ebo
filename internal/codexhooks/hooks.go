package codexhooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lqw905/Ebo/internal/project"
)

const adapterCommand = "hook codex-pre-tool-use"

func ProjectPath(root string) string {
	return filepath.Join(root, ".codex", "hooks.json")
}

func Install(path string) (string, error) {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse existing Codex hooks %s: %w", path, err)
		}
	}
	before, _ := json.Marshal(root)
	hooks := objectField(root, "hooks")
	groups := arrayField(hooks, "PreToolUse")
	handler := map[string]any{
		"type":           "command",
		"command":        "ebo " + adapterCommand,
		"commandWindows": "ebo " + adapterCommand,
		"timeout":        10,
		"statusMessage":  "Checking Ebo pre-write authorization",
	}
	updated := false
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		handlers, _ := group["hooks"].([]any)
		for i, item := range handlers {
			existing, ok := item.(map[string]any)
			if !ok || !managedHandler(existing) {
				continue
			}
			handlers[i] = handler
			group["hooks"] = handlers
			updated = true
			break
		}
		if updated {
			break
		}
	}
	if !updated {
		groups = append(groups, map[string]any{
			"matcher": "^(apply_patch|Edit|Write)$",
			"hooks":   []any{handler},
		})
	}
	hooks["PreToolUse"] = groups
	root["hooks"] = hooks
	after, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	if string(before) == string(after) {
		return "unchanged", nil
	}
	after, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	after = append(after, '\n')
	if err := project.WriteFileAtomic(path, after, 0o644); err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "installed", nil
	}
	return "updated", nil
}

func Installed(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return false
	}
	hooks, _ := root["hooks"].(map[string]any)
	groups, _ := hooks["PreToolUse"].([]any)
	for _, item := range groups {
		group, _ := item.(map[string]any)
		handlers, _ := group["hooks"].([]any)
		for _, item := range handlers {
			handler, _ := item.(map[string]any)
			if managedHandler(handler) {
				return true
			}
		}
	}
	return false
}

func objectField(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func arrayField(parent map[string]any, key string) []any {
	if value, ok := parent[key].([]any); ok {
		return value
	}
	return nil
}

func managedHandler(handler map[string]any) bool {
	for _, key := range []string{"command", "commandWindows"} {
		if value, ok := handler[key].(string); ok && strings.Contains(value, adapterCommand) {
			return true
		}
	}
	return false
}
