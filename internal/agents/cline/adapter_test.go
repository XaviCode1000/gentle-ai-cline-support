package cline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

const testHome = "/tmp/home"

func TestDetect(t *testing.T) {
	tests := []struct {
		name            string
		stat            statResult
		lookPathErr     error
		wantInstalled   bool
		wantConfigPath  string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name:            "config directory found",
			stat:            statResult{isDir: true},
			wantInstalled:   true,
			wantConfigPath:  filepath.Join(testHome, ".cline"),
			wantConfigFound: true,
		},
		{
			name:            "config missing",
			stat:            statResult{err: os.ErrNotExist},
			lookPathErr:     errors.New("not found"),
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".cline"),
			wantConfigFound: false,
		},
		{
			name:            "config exists but is a file not a dir",
			stat:            statResult{isDir: false},
			wantInstalled:   false,
			wantConfigPath:  filepath.Join(testHome, ".cline"),
			wantConfigFound: false,
		},
		{
			name:    "stat error bubbles up",
			stat:    statResult{err: errors.New("permission denied")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{
				statPath: func(string) statResult { return tt.stat },
				lookPath: func(string) (string, error) {
					if tt.lookPathErr != nil {
						return "", tt.lookPathErr
					}
					return "/usr/local/bin/cline", nil
				},
			}
			installed, _, configPath, configFound, err := a.Detect(context.Background(), testHome)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if installed != tt.wantInstalled {
				t.Fatalf("Detect() installed = %v, want %v", installed, tt.wantInstalled)
			}
			if configPath != tt.wantConfigPath {
				t.Fatalf("Detect() configPath = %q, want %q", configPath, tt.wantConfigPath)
			}
			if configFound != tt.wantConfigFound {
				t.Fatalf("Detect() configFound = %v, want %v", configFound, tt.wantConfigFound)
			}
		})
	}
}

func TestConfigPaths(t *testing.T) {
	a := NewAdapter()
	home := testHome

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".cline") {
		t.Fatalf("GlobalConfigDir() = %q", got)
	}
	if got := a.SystemPromptDir(home); got != filepath.Join(home, ".cline", "rules") {
		t.Fatalf("SystemPromptDir() = %q", got)
	}
	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".cline", "rules", "gentle-ai.md") {
		t.Fatalf("SystemPromptFile() = %q", got)
	}
	if got := a.SkillsDir(home); got != filepath.Join(home, ".cline", "skills") {
		t.Fatalf("SkillsDir() = %q", got)
	}
	if got := a.SettingsPath(home); got != filepath.Join(home, ".cline", "data", "settings", "global-settings.json") {
		t.Fatalf("SettingsPath() = %q", got)
	}
	if got := a.MCPConfigPath(home, ""); got != filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json") {
		t.Fatalf("MCPConfigPath() = %q", got)
	}
	if got := a.SubAgentsDir(home); got != filepath.Join(home, ".cline", "agents") {
		t.Fatalf("SubAgentsDir() = %q", got)
	}
	if got := a.EmbeddedSubAgentsDir(); got != "" {
		t.Fatalf("EmbeddedSubAgentsDir() = %q, want empty (no shipped projection yet)", got)
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyMarkdownSections {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyMarkdownSections", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestAgentIdentity(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentCline {
		t.Fatalf("Agent() = %v, want %v", got, model.AgentCline)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want %v", got, model.TierFull)
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if !a.SupportsSkills() {
		t.Fatal("Cline should support skills")
	}
	if !a.SupportsSystemPrompt() {
		t.Fatal("Cline should support system prompt")
	}
	if !a.SupportsMCP() {
		t.Fatal("Cline should support MCP")
	}
	if !a.SupportsSubAgents() {
		t.Fatal("Cline should project configured-agent sub-agents")
	}
	if a.SubAgentsDir(testHome) == "" {
		t.Fatal("Cline SubAgentsDir() must resolve the configured-agent directory")
	}
	// Slash commands stay false for the base delivery: Cline's /-invocation
	// targets skills (use_skill), not standalone command files like OpenCode.
	if a.SupportsOutputStyles() {
		t.Fatal("Cline should NOT support output styles")
	}
}

func TestInstallCommandResolvesNpm(t *testing.T) {
	a := NewAdapter()

	cmds, err := a.InstallCommand(system.PlatformProfile{OS: "linux", NpmWritable: true})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}
	if len(cmds) == 0 {
		t.Fatal("InstallCommand() returned no commands")
	}
	joined := ""
	for _, c := range cmds {
		for _, part := range c {
			joined += part + " "
		}
	}
	if joined == "" || len(cmds[0]) == 0 {
		t.Fatal("InstallCommand() returned empty command")
	}
	found := false
	for _, c := range cmds {
		for _, part := range c {
			if part == "cline@latest" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("InstallCommand() should reference cline@latest, got %v", cmds)
	}
}
