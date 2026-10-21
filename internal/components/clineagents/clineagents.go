// Package clineagents owns the Cline configured-agent surface: it installs
// nothing today and owns the retirement of the superseded ODD verifier set.
//
// Cline holds the native receipt-driven-development class, so its review
// lifecycle is served by the standard review lenses and Judgment Day agents
// installed through internal/components/reviewassets. The lead-invoked ODD
// verifier agents (challenge-assumption and the verify-* roles) shipped while
// Cline had no provider harness are retired: with the harness in place, the
// runtime would otherwise carry two competing review vocabularies for the
// same job (verify-reliability against review-reliability).
//
// Retirement is ownership-checked: a file is removed only when its bytes
// match a variant Gentle AI shipped. A user's own agent under the same name
// is preserved.
package clineagents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// managedAgents is the single source of truth for the configured agents
// Gentle AI installs for Cline. It is empty: Cline's review agents are
// installed by reviewassets, and this component owns the retirement of the
// superseded ODD verifier set. Keep the embedded directory, install, backup,
// and upgrade inventory derived from it.
var managedAgents = []string{}

// AgentSupported reports whether Gentle AI installs configured agents for
// the runtime. Only Cline: every other file-subagent runtime is served by
// reviewassets (RDD) or ships nothing.
func AgentSupported(agent model.AgentID) bool { return agent == model.AgentCline }

// FileNames lists the configured-agent files installed for the runtime, so
// backup and verification contracts match the installer exactly.
func FileNames(agent model.AgentID) []string {
	if !AgentSupported(agent) {
		return nil
	}
	return append([]string(nil), managedAgents...)
}

// retiredAgents maps retired configured-agent filenames to the SHA-256 hex
// digests of every byte variant Gentle AI ever shipped for them. Install
// removes a retired file only when its bytes match a known shipped variant;
// anything else (including a user's own agent under the same name) is left
// untouched.
// The lead-invoked ODD verifiers are retired in favour of the native review
// class: the review lenses and Judgment Day agents now own Cline's review
// lifecycle, and shipping both sets would leave two competing review
// vocabularies in one runtime.
var retiredAgents = map[string][]string{
	"challenge-assumption.yaml": {"8fa41a1857b23831cf68c3a03faec02459052f8067f7b90e56eb9808e11c35da"},
	"verify-clarity.yaml":       {"04c9b676771b95951a34eaab1d8d0f3e97b2980b4f6b7576cb2e9a1f433f21a2"},
	"verify-reliability.yaml":   {"a8dbcb899245f9abb2f9e12e67e27dc6706f3a7e7d6c1bd8a71081674d29d056"},
	"verify-resilience.yaml":    {"addf06a11307a9f95a67e1f5ee8740cb700def12d041421644cfd9b0dd9ce928"},
	"verify-security.yaml":      {"c5a4bcb02e8d1d37107f3d85870e7f4fc59cbb882b9935ad288b6317602ceb58"},
	"review-readability.yaml":   {"fb54aa6a97c6e803d2aa0c5350423197750ff189e64835acd702638231cb28a6", "a4c3c8573ee196a2c26f4d014e3bfdd2f57dbad4f6866926bbb3c1085d8d5081"},
	"review-refuter.yaml":       {"cefb908bfb55ceaa70ca810b68ed7d28c1496bff8bf87725d5e5ade7cc1bce3e", "cb45985f5bf3fa51c16e1ebbbe566c09ac777ba204586d1e47ae59a8ac21eb92"},
	"review-reliability.yaml":   {"6114073a44be9aed13566c790d405e6d3744b35adc970d23d15f7bee7787f721", "a701a1129b1d5eedec90118349de79e5334283890a88705661e79228d26aff3c"},
	"review-resilience.yaml":    {"56ee2ef77bf9c04f95c139c311924a1a6fb3c1354a4290ce49726b7aa39b7550", "b701d97deb296d19cc6129523dd043571aeeb8436bc029602c671df432d9bb4a"},
	"review-risk.yaml":          {"a5466a0212fd8bc82543854e871e38aec8d4efb6c584c9e48e68ec9f4c772b71", "2866bac1ccb69ab9152850e12d8045d8173eb3ad74515cd98c8ddd104c82716d"},
}

// RetiredFileNames lists retired configured-agent filenames for the runtime,
// so backup and verification contracts snapshot exactly what the installer
// may remove.
func RetiredFileNames(agent model.AgentID) []string {
	if !AgentSupported(agent) {
		return nil
	}
	names := make([]string, 0, len(retiredAgents))
	for name := range retiredAgents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ownsRetiredFile reports whether data matches a byte variant Gentle AI
// shipped for the retired filename.
func ownsRetiredFile(name string, data []byte) bool {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	for _, want := range retiredAgents[name] {
		if got == want {
			return true
		}
	}
	return false
}

type InstallResult struct {
	Changed bool
	Files   []string
}

// Install writes the embedded Cline configured agents into the adapter's
// subagents directory, creating it when needed. A file is written only
// when its bytes differ. It fails closed on an unsupported agent, an empty
// directory, or a missing embedded asset, and never touches user-owned
// files outside the managed set.
func Install(home string, adapter agents.Adapter) (InstallResult, error) {
	if !AgentSupported(adapter.Agent()) {
		return InstallResult{}, fmt.Errorf("clineagents: unsupported agent %q", adapter.Agent())
	}
	if adapter.EmbeddedSubAgentsDir() == "" {
		return InstallResult{}, fmt.Errorf("clineagents: adapter %q exposes no embedded agents directory", adapter.Agent())
	}
	dir := adapter.SubAgentsDir(home)
	if dir == "" {
		return InstallResult{}, fmt.Errorf("clineagents: adapter %q exposes no subagents directory", adapter.Agent())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create cline agents directory: %w", err)
	}
	var result InstallResult

	// Read every input before touching the target so a missing or unreadable
	// embedded asset leaves the previous installation untouched. Retiring
	// is only safe after all inputs are in memory.
	desired := make(map[string]string, len(managedAgents))
	for _, name := range managedAgents {
		content, err := assets.Read(adapter.EmbeddedSubAgentsDir() + "/" + name)
		if err != nil {
			return InstallResult{}, fmt.Errorf("read embedded cline agent %s: %w", name, err)
		}
		desired[name] = content
	}
	// Retire previously shipped agents first: a renamed managed set must
	// not leave stale tools behind. Only Gentle AI-owned bytes are
	// removed; user files under the same name are preserved. Retirement
	// runs after the embedded inputs have been read, so a read failure cannot
	// leave a partial install behind.
	for _, name := range RetiredFileNames(adapter.Agent()) {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return result, fmt.Errorf("inspect retired cline agent %s: %w", name, err)
		}
		if !ownsRetiredFile(name, data) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return result, fmt.Errorf("remove retired cline agent %s: %w", name, err)
		}
		result.Changed = true
		result.Files = append(result.Files, path)
	}
	names := append([]string(nil), managedAgents...)
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		res, err := filemerge.WriteFileAtomic(path, []byte(desired[name]), 0o644)
		if err != nil {
			return result, fmt.Errorf("write cline agent %s: %w", name, err)
		}
		if res.Changed {
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}

// EmbeddedNames lists the agent files actually embedded under the
// adapter's embedded directory. Tests pin it equal to FileNames so a
// dropped or renamed asset fails instead of silently narrowing coverage.
func EmbeddedNames(adapter agents.Adapter) ([]string, error) {
	entries, err := fs.ReadDir(assets.FS, adapter.EmbeddedSubAgentsDir())
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
