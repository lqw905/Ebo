package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func Head(root string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func Status(root string) ([]string, error) {
	output, err := run(root, "status", "--short")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(output), nil
}

func Add(root string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := run(root, args...)
	return err
}

func CachedNames(root string) ([]string, error) {
	output, err := run(root, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(output), nil
}

func ChangedNames(root string) ([]string, error) {
	commands := [][]string{
		{"diff", "--name-only"},
		{"diff", "--cached", "--name-only"},
		{"ls-files", "--others", "--exclude-standard"},
	}
	seen := map[string]bool{}
	for _, args := range commands {
		output, err := run(root, args...)
		if err != nil {
			return nil, err
		}
		for _, name := range nonEmptyLines(output) {
			seen[normalizePath(name)] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func GitPath(root, name string) (string, error) {
	output, err := run(root, "rev-parse", "--git-path", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func IndexFile(root, path string) ([]byte, error) {
	output, err := run(root, "show", ":"+normalizePath(path))
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

func Commit(root, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message is required")
	}
	_, err := run(root, "commit", "-m", message)
	return err
}

func run(root string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is not available")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String() + stderr.String())
		return stdout.String(), fmt.Errorf("git %s failed: %s", strings.Join(args, " "), output)
	}
	return stdout.String(), nil
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizePath(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}
