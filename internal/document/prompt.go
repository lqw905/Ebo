package document

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const PromptSchema = "ebo.prompt/v1"

type Prompt struct {
	Schema     string
	ID         string
	Title      string
	Kind       string
	Parent     string
	Revision   int
	Origin     string
	Confidence string
	State      State
	Hash       Hashes
	Links      map[string][]Link
	Body       string
	Source     string
	Raw        []byte
}

type State struct {
	Spec      string
	Execution string
	Sync      string
}

type Hashes struct {
	Content   string
	Effective string
	Satisfied string
}

type Link struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

var promptIDRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(\.[A-Za-z][A-Za-z0-9_-]*)*$`)

func ParsePrompt(data []byte, source string) (*Prompt, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s is not valid UTF-8", source)
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = normalizeNewlines(text)
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("%s must start with YAML front matter", source)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("%s has no closing front matter marker", source)
	}

	p := &Prompt{
		Source: source,
		Raw:    data,
		Links:  map[string][]Link{},
		Body:   strings.Join(lines[end+1:], "\n"),
	}

	var section string
	var linkType string
	linkIndex := -1

	for lineNo, raw := range lines[1:end] {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := leadingSpaces(raw)
		trimmed := strings.TrimSpace(raw)
		key, value, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			return nil, fmt.Errorf("%s front matter line %d is not supported by the MVP parser: %s", source, lineNo+2, trimmed)
		}

		switch {
		case indent == 0:
			section = ""
			linkType = ""
			linkIndex = -1
			if value == "" && strings.HasSuffix(trimmed, ":") {
				section = key
				continue
			}
			if err := setTopLevel(p, key, value); err != nil {
				return nil, fmt.Errorf("%s front matter line %d: %w", source, lineNo+2, err)
			}
		case section == "state" && indent >= 2:
			setState(&p.State, key, value)
		case section == "hash" && indent >= 2:
			setHash(&p.Hash, key, value)
		case section == "links" && indent == 2:
			linkType = key
			linkIndex = -1
			if _, exists := p.Links[linkType]; !exists {
				p.Links[linkType] = nil
			}
		case section == "links" && indent >= 4 && linkType != "":
			if strings.HasPrefix(trimmed, "- ") {
				link := Link{Type: linkType}
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				if rest != "" {
					k, v, ok := splitYAMLKeyValue(rest)
					if !ok {
						return nil, fmt.Errorf("%s front matter line %d has unsupported link syntax", source, lineNo+2)
					}
					setLinkField(&link, k, v)
				}
				p.Links[linkType] = append(p.Links[linkType], link)
				linkIndex = len(p.Links[linkType]) - 1
				continue
			}
			if linkIndex < 0 {
				return nil, fmt.Errorf("%s front matter line %d has link field without list item", source, lineNo+2)
			}
			links := p.Links[linkType]
			setLinkField(&links[linkIndex], key, value)
			p.Links[linkType] = links
		default:
			return nil, fmt.Errorf("%s front matter line %d has unsupported indentation", source, lineNo+2)
		}
	}

	defaultState(&p.State)
	return p, nil
}

func ValidateBasic(p *Prompt) []string {
	var issues []string
	if p.Schema != PromptSchema {
		issues = append(issues, fmt.Sprintf("%s: schema must be %s", p.Source, PromptSchema))
	}
	if !promptIDRE.MatchString(p.ID) {
		issues = append(issues, fmt.Sprintf("%s: invalid id %q", p.Source, p.ID))
	}
	if strings.TrimSpace(p.Title) == "" {
		issues = append(issues, fmt.Sprintf("%s: title is required", p.Source))
	}
	if strings.TrimSpace(p.Kind) == "" {
		issues = append(issues, fmt.Sprintf("%s: kind is required", p.Source))
	}
	for typ, links := range p.Links {
		if !KnownLinkType(typ) {
			issues = append(issues, fmt.Sprintf("%s: unknown link type %q", p.Source, typ))
			continue
		}
		for _, link := range links {
			if !promptIDRE.MatchString(link.ID) {
				issues = append(issues, fmt.Sprintf("%s: invalid %s link target %q", p.Source, typ, link.ID))
			}
			if RequiresReason(typ) && strings.TrimSpace(link.Reason) == "" {
				issues = append(issues, fmt.Sprintf("%s: %s link to %s requires reason", p.Source, typ, link.ID))
			}
		}
	}
	return issues
}

func KnownLinkType(typ string) bool {
	switch typ {
	case "depends_on", "affects", "implements", "references", "supersedes":
		return true
	default:
		return false
	}
}

func RequiresReason(typ string) bool {
	switch typ {
	case "depends_on", "affects", "implements", "supersedes":
		return true
	default:
		return false
	}
}

func ContentHash(p *Prompt) string {
	links := make([]Link, 0)
	for typ, items := range p.Links {
		for _, item := range items {
			links = append(links, Link{Type: typ, ID: item.ID, Reason: item.Reason})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].Type != links[j].Type {
			return links[i].Type < links[j].Type
		}
		if links[i].ID != links[j].ID {
			return links[i].ID < links[j].ID
		}
		return links[i].Reason < links[j].Reason
	})
	canonical := struct {
		Schema     string `json:"schema"`
		ID         string `json:"id"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
		Parent     string `json:"parent,omitempty"`
		Revision   int    `json:"revision,omitempty"`
		Origin     string `json:"origin,omitempty"`
		Confidence string `json:"confidence,omitempty"`
		Links      []Link `json:"links,omitempty"`
		Body       string `json:"body"`
	}{
		Schema:     p.Schema,
		ID:         p.ID,
		Title:      p.Title,
		Kind:       p.Kind,
		Parent:     p.Parent,
		Revision:   p.Revision,
		Origin:     p.Origin,
		Confidence: p.Confidence,
		Links:      links,
		Body:       normalizeNewlines(p.Body),
	}
	data, _ := json.Marshal(canonical)
	return SHA256(data)
}

func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ShortHash(hash string, n int) string {
	hash = strings.TrimPrefix(hash, "sha256:")
	if len(hash) <= n {
		return hash
	}
	return hash[:n]
}

func RenderPrompt(p *Prompt) []byte {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	writeScalar(&b, "schema", p.Schema)
	writeScalar(&b, "id", p.ID)
	writeScalar(&b, "title", p.Title)
	writeScalar(&b, "kind", p.Kind)
	writeScalar(&b, "parent", p.Parent)
	if p.Revision != 0 {
		fmt.Fprintf(&b, "revision: %d\n", p.Revision)
	}
	writeScalar(&b, "origin", p.Origin)
	writeScalar(&b, "confidence", p.Confidence)
	fmt.Fprintln(&b, "state:")
	writeIndentedScalar(&b, "spec", p.State.Spec)
	writeIndentedScalar(&b, "execution", p.State.Execution)
	writeIndentedScalar(&b, "sync", p.State.Sync)
	if p.Hash.Content != "" || p.Hash.Effective != "" || p.Hash.Satisfied != "" {
		fmt.Fprintln(&b, "hash:")
		writeIndentedScalar(&b, "content", p.Hash.Content)
		writeIndentedScalar(&b, "effective", p.Hash.Effective)
		writeIndentedScalar(&b, "satisfied", p.Hash.Satisfied)
	}
	fmt.Fprintln(&b, "links:")
	linkTypes := make([]string, 0, len(p.Links))
	for typ := range p.Links {
		linkTypes = append(linkTypes, typ)
	}
	sort.Strings(linkTypes)
	if len(linkTypes) == 0 {
		fmt.Fprintln(&b, "  references: []")
	}
	for _, typ := range linkTypes {
		links := p.Links[typ]
		if len(links) == 0 {
			fmt.Fprintf(&b, "  %s: []\n", typ)
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", typ)
		for _, link := range links {
			fmt.Fprintf(&b, "    - id: %s\n", yamlScalar(link.ID))
			if link.Reason != "" {
				fmt.Fprintf(&b, "      reason: %s\n", yamlScalar(link.Reason))
			}
		}
	}
	fmt.Fprintln(&b, "---")
	body := strings.TrimLeft(normalizeNewlines(p.Body), "\n")
	b.WriteString(body)
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func writeScalar(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, yamlScalar(value))
}

func writeIndentedScalar(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "  %s: %s\n", key, yamlScalar(value))
}

func yamlScalar(value string) string {
	value = strings.ReplaceAll(normalizeNewlines(value), "\n", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, "#[]{}") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "&") || strings.HasPrefix(value, "*") {
		value = strings.ReplaceAll(value, "'", "''")
		return "'" + value + "'"
	}
	return value
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

func splitYAMLKeyValue(s string) (string, string, bool) {
	key, value, ok := strings.Cut(s, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, parseScalar(value), true
}

func parseScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" || value == "~" {
		return ""
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return ""
	}
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func setTopLevel(p *Prompt, key, value string) error {
	switch key {
	case "schema":
		p.Schema = value
	case "id":
		p.ID = value
	case "title":
		p.Title = value
	case "kind":
		p.Kind = value
	case "parent":
		p.Parent = value
	case "revision":
		if value == "" {
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("revision must be a number")
		}
		p.Revision = n
	case "origin":
		p.Origin = value
	case "confidence":
		p.Confidence = value
	default:
		return fmt.Errorf("unknown top-level field %q", key)
	}
	return nil
}

func setState(state *State, key, value string) {
	switch key {
	case "spec":
		state.Spec = value
	case "execution":
		state.Execution = value
	case "sync":
		state.Sync = value
	}
}

func setHash(hash *Hashes, key, value string) {
	switch key {
	case "content":
		hash.Content = value
	case "effective":
		hash.Effective = value
	case "satisfied":
		hash.Satisfied = value
	}
}

func setLinkField(link *Link, key, value string) {
	switch key {
	case "id":
		link.ID = value
	case "reason":
		link.Reason = value
	}
}

func defaultState(state *State) {
	if state.Spec == "" {
		state.Spec = "draft"
	}
	if state.Execution == "" {
		state.Execution = "not_started"
	}
	if state.Sync == "" {
		state.Sync = "dirty"
	}
}
