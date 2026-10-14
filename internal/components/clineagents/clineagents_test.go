package clineagents

import (
	"crypto/sha256"
	"encoding/hex"
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
		if !strings.Contains(body, "One pass only") {
			t.Fatalf("%s: body lost the single-pass bound", name)
		}
		for _, banned := range []string{"{{", "sdd", "openspec", "claude", "kiro", "askuserquestion", "frozen", "ledger", "refuter", "receipt", "RDD", "RAR", "native review", "review-integration", "judgment-day", "No findings."} {
			if strings.Contains(strings.ToLower(content), banned) {
				t.Fatalf("%s: banned %q present", name, banned)
			}
		}
	}
}

func TestRetiredOwnedRemovedForeignPreserved(t *testing.T) {
	adapter := cline.NewAdapter()
	home := t.TempDir()
	// A user-owned file under a retired name must survive install.
	foreign := []byte("---\nname: review-risk\ndescription: mine\ntools: []\n---\n\nhello\n")
	foreignPath := filepath.Join(adapter.SubAgentsDir(home), "review-risk.yaml")
	if err := os.MkdirAll(adapter.SubAgentsDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignPath, foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	// A genuine previously shipped variant must be retired. Rebuild its
	// exact bytes from the embedded upstream source plus the documented
	// transform both shipped variants applied.
	raw, err := assets.Read("claude/agents/review-reliability.md")
	if err != nil {
		t.Fatalf("read upstream lens: %v", err)
	}
	raw = strings.Replace(raw, "model: {{CLAUDE_MODEL}}\n", "", 1)
	raw = strings.Replace(raw, "{{CLAUDE_EFFORT_FRONTMATTER}}\n", "", 1)
	raw = strings.Replace(raw, "tools: []", "tools: [read_files]", 1)
	// Rebuild the exact shipped bytes the lensfix transform produced:
	// frontmatter (drops + tools), invocation note, body. The hash
	// assertion below pins this reconstruction to a known shipped variant;
	// a mismatch fails loudly instead of silently weakening the test.
	parts := strings.SplitN(raw, "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("unexpected frontmatter shape")
	}
	note := "> Cline note: the Cline lead invokes you with the review target pasted in the\n> task text, or with a file path for you to read yourself with read_files.\n> That supplied-or-read target is your entire scope: never edit, execute,\n> delegate, or fetch anything else.\n\n"
	raw = "---\n" + parts[1] + "---\n" + note + parts[2]
	sum := sha256.Sum256([]byte(raw))
	if got, want := hex.EncodeToString(sum[:]), "a701a1129b1d5eedec90118349de79e5334283890a88705661e79228d26aff3c"; got != want {
		t.Fatalf("reconstruction hash = %s, want shipped variant %s", got, want)
	}
	ownedPath := filepath.Join(adapter.SubAgentsDir(home), "review-reliability.yaml")
	if err := os.WriteFile(ownedPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Install(home, adapter)
	if err != nil {
		t.Fatalf("Install error = %v", err)
	}
	if !res.Changed {
		t.Fatal("Install reported no change, want managed writes plus one retirement")
	}
	if _, err := os.Stat(foreignPath); err != nil {
		t.Fatalf("user-owned retired-name file was removed: %v", err)
	}
	if _, err := os.Stat(ownedPath); !os.IsNotExist(err) {
		t.Fatalf("owned retired file still present")
	}
	for _, name := range FileNames(model.AgentCline) {
		if _, err := os.Stat(filepath.Join(adapter.SubAgentsDir(home), name)); err != nil {
			t.Fatalf("managed file %s missing: %v", name, err)
		}
	}
}
