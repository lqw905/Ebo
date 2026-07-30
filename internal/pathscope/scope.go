package pathscope

import (
	"regexp"
	"strings"

	"github.com/lqw905/Ebo/internal/document"
)

func Allows(scope document.Scope, target string) (bool, string) {
	target = normalize(target)
	for _, pattern := range scope.Deny {
		if matches(pattern, target) {
			return false, "path_denied_by_prompt_scope"
		}
	}
	if len(scope.Allow) == 0 {
		return true, "prompt_scope_allows_all_source_paths"
	}
	for _, pattern := range scope.Allow {
		if matches(pattern, target) {
			return true, "path_allowed_by_prompt_scope"
		}
	}
	return false, "path_outside_prompt_scope"
}

func matches(pattern, target string) bool {
	pattern = normalize(pattern)
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	re, err := regexp.Compile(globRegex(pattern))
	return err == nil && re.MatchString(target)
}

func globRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 3
				} else {
					b.WriteString(".*")
					i += 2
				}
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return b.String()
}

func normalize(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "./")
	return value
}
