package pathscope

import (
	"testing"

	"github.com/lqw905/Ebo/internal/document"
)

func TestAllowsUsesDenyBeforeAllow(t *testing.T) {
	scope := document.Scope{
		Allow: []string{"internal/auth/**", "cmd/*.go"},
		Deny:  []string{"internal/auth/secrets/**"},
	}
	tests := []struct {
		path   string
		allow  bool
		reason string
	}{
		{"internal/auth/service.go", true, "path_allowed_by_prompt_scope"},
		{"internal/auth/secrets/key.go", false, "path_denied_by_prompt_scope"},
		{"cmd/main.go", true, "path_allowed_by_prompt_scope"},
		{"cmd/server/main.go", false, "path_outside_prompt_scope"},
		{"README.md", false, "path_outside_prompt_scope"},
	}
	for _, tt := range tests {
		allowed, reason := Allows(scope, tt.path)
		if allowed != tt.allow || reason != tt.reason {
			t.Fatalf("Allows(%q) = %v, %q; want %v, %q", tt.path, allowed, reason, tt.allow, tt.reason)
		}
	}
}

func TestAllowsDefaultsToAllSourcePaths(t *testing.T) {
	allowed, reason := Allows(document.Scope{}, "internal/service.go")
	if !allowed || reason != "prompt_scope_allows_all_source_paths" {
		t.Fatalf("Allows = %v, %q", allowed, reason)
	}
}

func TestDoubleStarMatchesFilesAtAnyDepth(t *testing.T) {
	scope := document.Scope{Allow: []string{"**/*.go"}}
	for _, path := range []string{"main.go", "internal/main.go", "internal/auth/main.go"} {
		if allowed, _ := Allows(scope, path); !allowed {
			t.Fatalf("%q should match **/*.go", path)
		}
	}
}
