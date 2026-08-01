package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "empty", data: "", want: ModeStrict},
		{name: "unrelated", data: "schema = \"ebo.config/v1\"\nzero_ai = true\n", want: ModeStrict},
		{name: "explicit strict", data: "mode = \"strict\"\n", want: ModeStrict},
		{name: "silent", data: "mode = \"silent\"\n", want: ModeSilent},
		{name: "unquoted silent", data: "mode = silent\n", want: ModeSilent},
		{name: "invalid falls back to strict", data: "mode = \"loud\"\n", want: ModeStrict},
		{name: "ignores other mode-like key", data: "mode_flag = \"silent\"\n", want: ModeStrict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseMode([]byte(test.data)); got != test.want {
				t.Errorf("ParseMode(%q) = %q, want %q", test.data, got, test.want)
			}
		})
	}
}

func TestSetModeRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, DirName, ConfigName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema = \"ebo.config/v1\"\nzero_ai = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if mode, err := ReadMode(root); err != nil || mode != ModeStrict {
		t.Fatalf("default mode = %q, err = %v", mode, err)
	}
	if err := SetMode(root, ModeSilent); err != nil {
		t.Fatal(err)
	}
	if mode, err := ReadMode(root); err != nil || mode != ModeSilent {
		t.Fatalf("mode after SetMode(silent) = %q, err = %v", mode, err)
	}
	if err := SetMode(root, ModeStrict); err != nil {
		t.Fatal(err)
	}
	if mode, err := ReadMode(root); err != nil || mode != ModeStrict {
		t.Fatalf("mode after SetMode(strict) = %q, err = %v", mode, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "mode") != 1 {
		t.Fatalf("config should contain exactly one mode line:\n%s", data)
	}
}

func TestSetModeRejectsInvalidMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, DirName, ConfigName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("zero_ai = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetMode(root, "loud"); err == nil {
		t.Fatal("SetMode should reject an invalid mode")
	}
}
