package lockfile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lqw905/Ebo/internal/project"
)

type Lock struct {
	path  string
	token string
}

type Info struct {
	Schema    string `json:"schema"`
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
}

func Acquire(root, command string) (*Lock, error) {
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	info := Info{
		Schema:    "ebo.lock/v1",
		Token:     token,
		PID:       os.Getpid(),
		Command:   command,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := Read(root)
		if readErr != nil {
			return nil, fmt.Errorf("ebo project is locked at %s", path)
		}
		return nil, fmt.Errorf("ebo project is locked by pid=%d command=%q since %s", existing.PID, existing.Command, existing.CreatedAt)
	}
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &Lock{path: path, token: token}, nil
}

func Read(root string) (*Info, error) {
	data, err := os.ReadFile(Path(root))
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func Exists(root string) bool {
	_, err := os.Stat(Path(root))
	return err == nil
}

func Path(root string) string {
	return filepath.Join(project.NewPaths(root).LocksDir, "project.lock")
}

func (l *Lock) Release() error {
	data, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return err
	}
	if info.Token != l.token {
		return fmt.Errorf("refusing to release a lock owned by another process")
	}
	return os.Remove(l.path)
}

func randomToken() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
