package reviewassets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"gopkg.in/yaml.v3"
)

// clineReviewAgentNames is the review and Judgment Day set Cline receives:
// the same lenses, refuter, and judges every other full-tier runtime gets.
var clineReviewAgentNames = []string{
	"jd-fix-agent.yaml",
	"jd-judge-a.yaml",
	"jd-judge-b.yaml",
	"review-readability.yaml",
	"review-refuter.yaml",
	"review-reliability.yaml",
	"review-resilience.yaml",
	"review-risk.yaml",
}

// TestClineReviewAssetsAreRegistered proves the Cline runtime receives the
// standard review lenses and Judgment Day agents, in the file shape its own
// runtime discovers.
func TestClineReviewAssetsAreRegistered(t *testing.T) {
	names := reviewassets.NativeAgentManifest[model.AgentCline]
	if len(names) != len(clineReviewAgentNames) {
		t.Fatalf("Cline review assets = %v, want %v", names, clineReviewAgentNames)
	}
	for _, want := range clineReviewAgentNames {
		if !clineAssetNameIsListed(names, want) {
			t.Errorf("Cline review assets miss %q", want)
		}
	}
	if !model.SupportsReceiptDrivenDevelopment(model.AgentCline) {
		t.Fatal("Cline must be a receipt-driven development runtime to receive review assets")
	}
}

// TestClineReviewAssetsAreInstalledInClineSchema proves every Cline review
// asset is written where the Cline runtime discovers configured agents, and
// that each one is authored in Cline's schema: YAML frontmatter with a name,
// a description, and a tool list, followed by the reviewer body.
func TestClineReviewAssetsAreInstalledInClineSchema(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCline)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatalf("InstallNativeAgents(cline) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("InstallNativeAgents(cline) changed nothing")
	}
	for _, name := range clineReviewAgentNames {
		path := filepath.Join(adapter.SubAgentsDir(home), name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read installed %s: %v", name, err)
		}
		content := string(data)
		if !strings.HasPrefix(content, "---\n") {
			t.Fatalf("%s does not start with a YAML frontmatter block", name)
		}
		end := strings.Index(content, "\n---\n")
		if end < 0 {
			t.Fatalf("%s has no closing frontmatter delimiter", name)
		}
		var frontmatter struct {
			Name        string   `yaml:"name"`
			Description string   `yaml:"description"`
			Tools       []string `yaml:"tools"`
		}
		if err := yaml.Unmarshal([]byte(content[4:end]), &frontmatter); err != nil {
			t.Fatalf("parse %s frontmatter: %v", name, err)
		}
		if frontmatter.Name == "" || frontmatter.Description == "" {
			t.Errorf("%s frontmatter = %+v, want a name and a description", name, frontmatter)
		}
		if frontmatter.Tools == nil {
			t.Errorf("%s frontmatter carries no tool list", name)
		}
		body := strings.TrimSpace(content[end+5:])
		if body == "" {
			t.Errorf("%s has an empty body", name)
		}
		if !strings.Contains(body, "> Cline note:") {
			t.Errorf("%s body carries no Cline invocation note", name)
		}
	}
}

// TestClineReviewAssetsCopyTheCanonicalReviewerBodies proves the Cline assets
// are adapted, not rewritten: every reviewer body is the canonical upstream
// body, so the shared review rules stay byte-identical to every other
// runtime's. The two Judgment Day judges are the documented exception: their
// review ledger contract is the shared one, not the copy that ships in the
// upstream asset.
func TestClineReviewAssetsCopyTheCanonicalReviewerBodies(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCline)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatalf("InstallNativeAgents(cline) error = %v", err)
	}
	for _, name := range clineReviewAgentNames {
		canonical, err := assets.Read("claude/agents/" + strings.TrimSuffix(name, ".yaml") + ".md")
		if err != nil {
			t.Fatalf("read canonical %s: %v", name, err)
		}
		installed, err := os.ReadFile(filepath.Join(adapter.SubAgentsDir(home), name))
		if err != nil {
			t.Fatalf("read installed %s: %v", name, err)
		}
		body := string(installed)
		if strings.HasPrefix(name, "jd-judge-") {
			if !strings.Contains(body, reviewassets.JudgmentDayReviewerContract()) {
				t.Errorf("%s does not carry the shared review ledger contract", name)
			}
			continue
		}
		if !strings.Contains(body, canonicalBody(string(canonical))) {
			t.Errorf("%s does not carry the canonical reviewer body", name)
		}
	}
}

// clineAssetNameIsListed reports whether names contains want.
func clineAssetNameIsListed(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// canonicalBody returns the body of an upstream reviewer asset: everything
// after its closing frontmatter delimiter.
func canonicalBody(content string) string {
	end := strings.Index(content, "\n---\n")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(content[end+5:])
}
