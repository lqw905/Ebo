package gitx

import (
	"fmt"
	"os/exec"
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
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
