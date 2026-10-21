package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestClineSupersededLensIsReplacedNotLost proves the Cline upgrade path:
// the lens bytes the superseded ODD-era set shipped are retired first, and
// the native review installer then writes the current lens. The opposite
// order left the runtime with no lens at all: the native installer only
// rewrites bytes it owns, so it skipped the unowned file and the retirement
// then removed it.
func TestClineSupersededLensIsReplacedNotLost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	superseded, err := os.ReadFile(filepath.Join("testdata", "cline-superseded-lens-review-risk.yaml"))
	if err != nil {
		t.Fatalf("read superseded lens fixture: %v", err)
	}
	agentsDir := filepath.Join(home, ".cline", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lensPath := filepath.Join(agentsDir, "review-risk.yaml")
	if err := os.WriteFile(lensPath, superseded, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunInstall([]string{"--agent", "cline"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall(cline) error = %v", err)
	}

	installed, err := os.ReadFile(lensPath)
	if err != nil {
		t.Fatalf("the review lens is missing after the upgrade: %v", err)
	}
	if bytes.Equal(installed, superseded) {
		t.Fatal("the superseded lens bytes are still installed")
	}
	if !strings.Contains(string(installed), "name: review-risk") {
		t.Fatalf("the installed file is not the review lens:\n%s", installed)
	}

	// The superseded ODD verifier set is gone.
	for _, name := range []string{"challenge-assumption.yaml", "verify-clarity.yaml", "verify-reliability.yaml", "verify-resilience.yaml", "verify-security.yaml"} {
		if _, err := os.Stat(filepath.Join(agentsDir, name)); err == nil {
			t.Errorf("the superseded verifier %s is still installed", name)
		}
	}
}

// TestClineUserOwnedAgentSurvivesTheUpgrade proves a file the user wrote
// under a managed name is preserved: the retirement removes only the bytes
// Gentle AI shipped, and the native installer skips a file it does not own.
func TestClineUserOwnedAgentSurvivesTheUpgrade(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	agentsDir := filepath.Join(home, ".cline", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := []byte("---\nname: review-risk\ndescription: mine\ntools: []\n---\n\nmy own reviewer\n")
	path := filepath.Join(agentsDir, "review-risk.yaml")
	if err := os.WriteFile(path, userFile, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunInstall([]string{"--agent", "cline"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall(cline) error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the user file was removed: %v", err)
	}
	if !bytes.Equal(after, userFile) {
		t.Fatalf("the user file changed:\n%s", after)
	}
}
