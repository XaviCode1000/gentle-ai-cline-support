package clineagents

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"gopkg.in/yaml.v3"
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

// TestFileNamesMatchEmbedded pins the ownership split: this component
// installs nothing for Cline, and the review assets in the embedded
// directory belong to reviewassets, which installs the native review class.
func TestFileNamesMatchEmbedded(t *testing.T) {
	adapter := cline.NewAdapter()
	embedded, err := EmbeddedNames(adapter)
	if err != nil {
		t.Fatalf("EmbeddedNames error = %v", err)
	}
	managed := FileNames(model.AgentCline)
	if len(managed) != 0 {
		t.Fatalf("managed Cline agents = %v, want none: the review class is installed by reviewassets", managed)
	}
	for _, name := range managed {
		if !slices.Contains(embedded, name) {
			t.Fatalf("managed agent %s is not embedded", name)
		}
	}
	if len(embedded) == 0 {
		t.Fatal("embedded Cline agents are empty: the review assets are missing")
	}
}

// TestInstallWritesNothingAndIsIdempotent proves the Cline installer writes
// no configured agent of its own: the review class is reviewassets' job.
func TestInstallWritesNothingAndIsIdempotent(t *testing.T) {
	adapter := cline.NewAdapter()
	home := t.TempDir()
	first, err := Install(home, adapter)
	if err != nil {
		t.Fatalf("Install error = %v", err)
	}
	if first.Changed {
		t.Fatalf("first Install = %+v, want no changes", first)
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

// TestEmbeddedAgentYAMLContracts pins the Cline schema of the embedded
// review agents: frontmatter with name, description and tools, then the
// reviewer body. The bodies carry the native review contract, so the ODD
// verifier bans of the retired set do not apply here.
func TestEmbeddedAgentYAMLContracts(t *testing.T) {
	adapter := cline.NewAdapter()
	names, err := EmbeddedNames(adapter)
	if err != nil {
		t.Fatalf("EmbeddedNames error = %v", err)
	}
	for _, name := range names {
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
		var fields struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(head), &fields); err != nil {
			t.Fatalf("%s: frontmatter is not valid YAML: %v", name, err)
		}
		if fields.Name != base {
			t.Fatalf("%s: name = %q, want %q", name, fields.Name, base)
		}
		if strings.TrimSpace(fields.Description) == "" {
			t.Fatalf("%s: empty description", name)
		}
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(head), &raw); err != nil {
			t.Fatalf("%s: frontmatter is not valid YAML: %v", name, err)
		}
		if _, ok := raw["tools"]; !ok {
			t.Fatalf("%s: missing tools", name)
		}
		if !strings.Contains(body, "Cline note") {
			t.Fatalf("%s: body lost the Cline invocation note", name)
		}
		for _, banned := range []string{"{{", "openspec", "askuserquestion"} {
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
