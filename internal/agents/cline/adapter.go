// Adapter implements agents.Adapter for the Cline coding agent
// (VS Code extension, CLI, desktop app and SDK share one config root).
//
// Config path summary (cross-platform, always under home):
//   - Detection: `cline` binary on PATH (npm package `cline`, verified 3.0.62)
//     and/or the shared config root ~/.cline/ (IDE extensions create it too)
//   - Rules / system prompt: ~/.cline/rules/
//   - ~/Cline/Rules + ~/Documents/Cline/Rules (compat search paths —
//     see resolveGlobalRulesConfigPaths in @cline/shared); gentle-ai owns
//     rules/gentle-ai.md inside ~/.cline/rules
//   - Skills: ~/.cline/skills/ (SKILL.md Agent-Skills spec)
//   - MCP (authoritative, CLI-verified): ~/.cline/data/settings/cline_mcp_settings.json
//     (~/.cline/mcp.json is a legacy/compat surface only)
//   - Sub-agents (configured agents): ~/.cline/agents/*.yaml|yml
//     → exposed to the model as subagent_<name> tools
//   - Settings: ~/.cline/data/settings/global-settings.json
//
// Detection: CLI sessions share the same root, so the ~/.cline directory
// existing is enough; the `cline` binary on PATH is reported independently.
// No binary appears on PATH for IDE-only installs (desktop/VS Code).
package cline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type statResult struct {
	isDir bool
	err   error
}

type Adapter struct {
	statPath func(string) statResult
	lookPath func(string) (string, error)
	resolver installcmd.Resolver
}

func NewAdapter() *Adapter {
	return &Adapter{statPath: defaultStat, lookPath: exec.LookPath}
}

// --- Identity ---

func (a *Adapter) Agent() model.AgentID    { return model.AgentCline }
func (a *Adapter) Tier() model.SupportTier { return model.TierFull }

// --- Detection ---

// Detect reports the `cline` binary on PATH and the shared config root
// ~/.cline independently; the config directory drives install detection,
// because the binary is absent on IDE-only installs — neither the VS Code
// extension nor the desktop app puts a `cline` executable on PATH — and such an
// install is still configured. That makes the first return value mean "the
// config root is a directory" here, where every other adapter uses it to mean
// "the binary is on PATH". Cline is the deliberate exception; do not converge
// it, and do not read it as a precedent for the others.
func (a *Adapter) Detect(_ context.Context, homeDir string) (bool, string, string, bool, error) {
	lookPath := a.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	configPath := a.GlobalConfigDir(homeDir)
	stat := a.statPath(configPath)
	if stat.err != nil {
		if os.IsNotExist(stat.err) {
			if binaryPath, lookErr := lookPath("cline"); lookErr == nil {
				return true, binaryPath, configPath, false, nil
			}
			return false, "", configPath, false, nil
		}
		return false, "", "", false, stat.err
	}
	binaryPath, lookErr := lookPath("cline")
	if lookErr != nil {
		binaryPath = ""
	}
	return stat.isDir, binaryPath, configPath, stat.isDir, nil
}

// --- Installation ---

func (a *Adapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.MustForAgent(model.AgentCline)
}

// InstallCommand resolves the pinned npm install sequence for the `cline`
// package (same resolver cohort as Claude Code / Kilocode). A desktop-only
// host without npm still gets a printed command, never a silent skip.
func (a *Adapter) InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	resolver := a.resolver
	if resolver == nil {
		resolver = installcmd.NewResolver()
	}
	return resolver.ResolveAgentInstall(profile, a.Agent())
}

// --- Config paths ---

// GlobalConfigDir returns ~/.cline, the single config root shared by the
// VS Code extension, the CLI, the desktop app and the SDK.
func (a *Adapter) GlobalConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ".cline")
}

// SystemPromptDir returns the global rules directory, the first entry of
// Cline's global rules search paths (resolveGlobalRulesConfigPaths).
func (a *Adapter) SystemPromptDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "rules")
}

// SystemPromptFile returns the rules file gentle-ai owns. Cline combines
// every *.md file under ~/.cline/rules/ into its prompt, so gentle-ai
// injects its sections via StrategyMarkdownSections markers without
// clobbering other user rules.
func (a *Adapter) SystemPromptFile(homeDir string) string {
	return filepath.Join(a.SystemPromptDir(homeDir), "gentle-ai.md")
}

// SkillsDir returns the global skills directory (SKILL.md Agent-Skills spec).
func (a *Adapter) SkillsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "skills")
}

// SettingsPath returns the platform-independent global settings file.
func (a *Adapter) SettingsPath(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "data", "settings", "global-settings.json")
}

// --- Config strategies ---

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}

// --- MCP ---

// MCPConfigPath returns the authoritative MCP servers config file, verified
// against the running CLI (`cline config mcp --json` reports every server
// with path ~/.cline/data/settings/cline_mcp_settings.json).
// ~/.cline/mcp.json exists as a legacy/compat surface and is intentionally
// left unmanaged (documented, not synchronized).
func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "data", "settings", "cline_mcp_settings.json")
}

// --- Optional capabilities ---

func (a *Adapter) SupportsOutputStyles() bool {
	return a.CapabilityManifest().Features.OutputStyles
}
func (a *Adapter) OutputStyleDir(_ string) string { return "" }
func (a *Adapter) SupportsSlashCommands() bool {
	return a.CapabilityManifest().Features.SlashCommands
}
func (a *Adapter) CommandsDir(_ string) string { return "" }
func (a *Adapter) SupportsSubAgents() bool {
	return a.CapabilityManifest().Features.FileSubAgents
}

// SubAgentsDir returns ~/.cline/agents, where Cline discovers configured
// agents (*.yaml/*.yml) and exposes each one to the model as a
// subagent_<name> tool. Workspace definitions (<workspace>/.cline/agents)
// shadow same-named global ones.
func (a *Adapter) SubAgentsDir(homeDir string) string {
	return filepath.Join(a.GlobalConfigDir(homeDir), "agents")
}

// EmbeddedSubAgentsDir returns the canonical Cline configured-agent
// definitions (review lenses). The clineagents projection copies these
// files verbatim, so they are authored directly in Cline's schema:
// frontmatter {name, description, tools} followed by the reviewer prompt
// as the document body. Keep clineagents.FileNames as the source of
// truth for the set.
func (a *Adapter) EmbeddedSubAgentsDir() string { return "cline/agents" }
func (a *Adapter) SupportsSkills() bool {
	return a.CapabilityManifest().Features.Skills
}
func (a *Adapter) SupportsSystemPrompt() bool {
	return a.CapabilityManifest().Features.SystemPrompt
}
func (a *Adapter) SupportsMCP() bool {
	return a.CapabilityManifest().Features.MCP
}

func defaultStat(path string) statResult {
	info, err := os.Stat(path)
	if err != nil {
		return statResult{err: err}
	}
	return statResult{isDir: info.IsDir()}
}
