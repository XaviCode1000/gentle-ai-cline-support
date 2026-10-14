package clineagents

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestAgentSupportedOnlyCline(t *testing.T) {
	if !AgentSupported(model.AgentCline) {
		t.Fatal("AgentSupported(cline) = false, want true")
	}
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentKimi, model.AgentPi} {
		if AgentSupported(agent) {
			t.Fatalf("AgentSupported(%s) = true, want false", agent)
		}
	}
}

func TestFileNamesMatchEmbedded(t *testing.T) {
	adapter := cline.NewAdapter()
	embedded, err := EmbeddedNames(adapter)
	if err != nil {
		t.Fatalf("EmbeddedNames error = %v", err)
	}
	if !reflect.DeepEqual(embedded, FileNames(model.AgentCline)) {
		t.Fatalf("embedded %v != managed %v", embedded, FileNames(model.AgentCline))
	}
}

func TestInstallWritesAndIsIdempotent(t *testing.T) {
	adapter := cline.NewAdapter()
	home := t.TempDir()
	first, err := Install(home, adapter)
	if err != nil {
		t.Fatalf("Install error = %v", err)
	}
	if !first.Changed || len(first.Files) != len(FileNames(model.AgentCline)) {
		t.Fatalf("first Install = %+v, want all %d files changed", first, len(FileNames(model.AgentCline)))
	}
	for _, name := range FileNames(model.AgentCline) {
		if _, err := os.Stat(filepath.Join(adapter.SubAgentsDir(home), name)); err != nil {
			t.Fatalf("installed file %s missing: %v", name, err)
		}
	}
	second, err := Install(home, adapter)
	if err != nil {
		t.Fatalf("second Install error = %v", err)
	}
	if second.Changed || len(second.Files) != 0 {
		t.Fatalf("second Install = %+v, want no changes", second)
	}
}

func TestInstallRejectsUnsupportedAgent(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("NewAdapter(claude) error = %v", err)
	}
	if _, err := Install(t.TempDir(), adapter); err == nil {
		t.Fatal("Install(claude) = nil, want unsupported-agent error")
	}
}

func TestEmbeddedAgentYAMLContracts(t *testing.T) {
	for _, name := range FileNames(model.AgentCline) {
		content, err := assets.Read("cline/agents/" + name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		base := strings.TrimSuffix(name, ".yaml")
		if !strings.HasPrefix(content, "---\n") {
			t.Fatalf("%s: missing frontmatter open", name)
		}
		head, body, ok := strings.Cut(content[4:], "\n---\n")
		if !ok || strings.TrimSpace(body) == "" {
			t.Fatalf("%s: missing frontmatter close or empty body", name)
		}
		fields := map[string]string{}
		for _, line := range strings.Split(head, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				t.Fatalf("%s: malformed frontmatter line %q", name, line)
			}
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		if fields["name"] != base {
			t.Fatalf("%s: name = %q, want %q", name, fields["name"], base)
		}
		if strings.Trim(fields["description"], "\"") == "" {
			t.Fatalf("%s: empty description", name)
		}
		if fields["tools"] != "[read_files]" {
			t.Fatalf("%s: tools = %q, want read-only [read_files] (reviewers run without execution tools)", name, fields["tools"])
		}
		if !strings.Contains(body, "BLOCKER") {
			t.Fatalf("%s: body lost the shared severity vocabulary", name)
		}
		for _, banned := range []string{"{{", "sdd", "openspec", "claude", "askuserquestion"} {
			if strings.Contains(strings.ToLower(content), banned) {
				t.Fatalf("%s: banned %q present", name, banned)
			}
		}
	}
}
