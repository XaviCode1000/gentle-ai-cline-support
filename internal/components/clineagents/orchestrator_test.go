package clineagents

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
)

var updateOrchestratorAsset = flag.Bool("update", false, "rewrite internal/assets/cline/orchestrator.md from the canonical generic asset")

const (
	genericOrchestratorAsset = "generic/orchestrator.md"
	clineOrchestratorAsset   = "cline/orchestrator.md"
)

// TestBuildOrchestratorAsset pins the transformation that turns the canonical
// generic orchestrator into the Cline one: the small-model variant never
// survives, the Cline title and boundary are present, and the Cline memory
// lines name the runtime's no-MCP delegate contract.
func TestBuildOrchestratorAsset(t *testing.T) {
	generic, err := assets.Read(genericOrchestratorAsset)
	if err != nil {
		t.Fatalf("read generic orchestrator: %v", err)
	}
	built, err := BuildOrchestratorAsset([]byte(generic))
	if err != nil {
		t.Fatalf("BuildOrchestratorAsset: %v", err)
	}
	content := string(built)

	for _, unwanted := range []string{
		"section:model-capable",
		"section:model-small",
		"Small Model",
		"# Agent Teams Lite — Orchestrator Instructions",
		"Bind this to the dedicated orchestrator agent or rule only.",
	} {
		if strings.Contains(content, unwanted) {
			t.Errorf("generated Cline orchestrator keeps %q", unwanted)
		}
	}
	for _, wanted := range []string{
		"# Cline — Orchestrator Instructions",
		"Bind this to the Cline orchestrator rule only.",
		"## Cline Delegation Boundary",
		"#### Delegation Triggers",
		"spawn_agent",
		clineWriteContextLine,
		clinePromptAdditionLine,
	} {
		if !strings.Contains(content, wanted) {
			t.Errorf("generated Cline orchestrator is missing %q", wanted)
		}
	}
}

// TestBuildOrchestratorAssetFailsClosed proves the generator never ships a
// partial orchestrator when the canon moves: a missing anchor stops the
// build instead of dropping the section.
func TestBuildOrchestratorAssetFailsClosed(t *testing.T) {
	generic, err := assets.Read(genericOrchestratorAsset)
	if err != nil {
		t.Fatalf("read generic orchestrator: %v", err)
	}
	cases := map[string]func(string) string{
		"missing title anchor": func(s string) string {
			return strings.Replace(s, "# Agent Teams Lite — Orchestrator Instructions", "# Other", 1)
		},
		"missing memory anchor": func(s string) string {
			return strings.Replace(s, genericWriteContextLine, "- Write context: sub-agent saves.", 1)
		},
		"missing model marker": func(s string) string { return strings.Replace(s, "<!-- section:model-capable -->", "", 1) },
		"duplicated model marker": func(s string) string {
			return strings.Replace(s, "<!-- section:model-capable -->", "<!-- section:model-capable -->\n<!-- section:model-capable -->", 1)
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildOrchestratorAsset([]byte(tc(generic))); err == nil {
				t.Fatalf("BuildOrchestratorAsset accepted a generic asset with %s", name)
			}
		})
	}
}

// TestClineOrchestratorAssetInSync keeps the checked-in Cline orchestrator
// byte-identical to the canonical generic asset plus the Cline overlay. The
// generic asset is the source of truth: when upstream moves the canon, this
// test fails and `-update` rebuilds the Cline copy.
func TestClineOrchestratorAssetInSync(t *testing.T) {
	generic, err := assets.Read(genericOrchestratorAsset)
	if err != nil {
		t.Fatalf("read generic orchestrator: %v", err)
	}
	built, err := BuildOrchestratorAsset([]byte(generic))
	if err != nil {
		t.Fatalf("BuildOrchestratorAsset: %v", err)
	}
	checkedIn, err := assets.Read(clineOrchestratorAsset)
	if err != nil {
		t.Fatalf("read cline orchestrator: %v", err)
	}
	if bytes.Equal(built, []byte(checkedIn)) {
		return
	}
	if !*updateOrchestratorAsset {
		t.Fatalf("%s is stale against %s; rerun with -update to rebuild it", clineOrchestratorAsset, genericOrchestratorAsset)
	}
	path := filepath.Join("..", "..", "assets", "cline", "orchestrator.md")
	if err := os.WriteFile(path, built, 0o644); err != nil {
		t.Fatalf("rewrite %s: %v", path, err)
	}
	t.Logf("rebuilt %s from %s", path, genericOrchestratorAsset)
}
