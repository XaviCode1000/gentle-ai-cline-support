// Package clineagents installs Gentle AI's lead-invoked configured agents
// for Cline into the runtime's subagents directory.
//
// These agents are NOT part of receipt-driven development: Cline has no
// provider subprocess harness, so the Cline lead invokes them with pasted
// binding plus context, and they return payloads. The review lenses carry
// no tools at all (Cline subagents never receive MCP servers anyway),
// exactly like their upstream provider-harness counterparts, which run
// without execution tools against provider-injected immutable context.
package clineagents

import (
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
// Gentle AI installs for Cline. Keep the embedded directory, install,
// backup, and upgrade inventory derived from it.
var managedAgents = []string{
	"review-readability.yaml",
	"review-refuter.yaml",
	"review-reliability.yaml",
	"review-resilience.yaml",
	"review-risk.yaml",
}

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
	// Read every input before touching the target so a missing embedded
	// asset never leaves a partial install behind.
	desired := make(map[string]string, len(managedAgents))
	for _, name := range managedAgents {
		content, err := assets.Read(adapter.EmbeddedSubAgentsDir() + "/" + name)
		if err != nil {
			return InstallResult{}, fmt.Errorf("read embedded cline agent %s: %w", name, err)
		}
		desired[name] = content
	}
	var result InstallResult
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
