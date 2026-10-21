package reviewerprovider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ClineAdapter invokes the Cline CLI in its headless mode and returns the
// reviewer's final text.
//
// Cline has no print-mode reviewer and no tool-free reviewer subagent the way
// Claude Code has one, so the transport is built from what the runtime
// actually offers: the headless CLI invocation
//
//	cline "<prompt>" --json --system <text> --auto-approve false
//
// (cline/apps/cli/src/commands/program.ts). The reviewer prompt is the
// positional argument because the runtime refuses JSON output mode without a
// prompt argument (cline/apps/cli/src/main.ts), and it is delivered verbatim:
// the provider contract requires the task to begin with the binding marker
// line, so no placeholder or prefix may be prepended to it.
//
// The fresh-reviewer boundary is the scratch area this adapter creates:
// an empty working directory, an isolated configuration root, an isolated data
// directory, and an isolated home. Cline discovers rules, agents, skills,
// hooks, and MCP servers from those roots, so an isolated root means the
// reviewer sees the provider-injected candidate evidence and nothing else --
// no user rules, no project rules, no live tools. The user's real ~/.cline is
// never written: the only thing read from it is the provider settings file
// that carries the credentials the reviewer authenticates with, and it is
// copied into the scratch area so the runtime's own provider-selection
// persistence lands in the copy.
//
// The capture fails closed. A run that produces no final text, an empty final
// text, or a non-zero exit is an unavailable capture: the transport reports
// the failure and never fabricates a verdict.
type ClineAdapter struct {
	// LookPath resolves the Cline binary; it defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Model is optional; an empty assignment preserves the runtime's own
	// selected model.
	Model string
	// commandContext builds the child process; it defaults to
	// exec.CommandContext and is the test seam.
	commandContext func(context.Context, string, ...string) *exec.Cmd
	// homeDir resolves the user's home, the only place the provider settings
	// (credentials) are read from. It defaults to os.UserHomeDir.
	homeDir func() (string, error)
	// scratchDir creates the removable scratch area. It defaults to
	// os.MkdirTemp and is the test seam.
	scratchDir func(pattern string) (string, error)
}

// NewClineAdapter returns an adapter using the Cline binary from PATH.
func NewClineAdapter() *ClineAdapter {
	return &ClineAdapter{
		LookPath:       exec.LookPath,
		commandContext: exec.CommandContext,
		homeDir:        os.UserHomeDir,
		scratchDir:     func(pattern string) (string, error) { return os.MkdirTemp("", pattern) },
	}
}

// clineReviewerSystemPrompt replaces Cline's default system prompt, so the
// reviewer receives the provider task and nothing else. It states the
// tool-free boundary the scratch area already enforces: tool approval is
// denied in a non-TTY run, so the reviewer can never reach the live worktree.
const clineReviewerSystemPrompt = "You are an isolated code reviewer process. Your only instructions are those delivered in the user message. You have no tools: never run commands, never read or write files, and never delegate."

// clineReviewerArguments locks the headless boundary: JSON output so the
// final answer is machine-readable, an explicit system prompt, and
// auto-approval disabled so no tool call can ever be approved in the
// non-interactive capture.
var clineReviewerArguments = []string{
	"--json", "--system", clineReviewerSystemPrompt, "--auto-approve", "false",
}

// clineScrubbedEnvironment lists the Cline environment variables that would
// point the reviewer back at the user's real directories. They are removed
// from the child environment so the isolated config and data roots are the
// only roots the runtime can reach.
var clineScrubbedEnvironment = []string{
	"CLINE_DIR",
	"CLINE_DATA_DIR",
	"CLINE_SANDBOX",
	"CLINE_SANDBOX_DATA_DIR",
	"CLINE_DB_DATA_DIR",
	"CLINE_SESSION_DATA_DIR",
	"CLINE_TEAM_DATA_DIR",
	"CLINE_PROVIDER_SETTINGS_PATH",
	"CLINE_HOOKS_DIR",
	"CLINE_HOOKS_LOG_PATH",
	"CLINE_GLOBAL_SETTINGS_PATH",
	"CLINE_MCP_SETTINGS_PATH",
	"CLINE_CONNECTOR_SETTINGS_PATH",
	"CLINE_CONNECTORS_DB_PATH",
	"CLINE_CRON_DB_PATH",
	"CLINE_TASKS_DB_PATH",
	"CLINE_TOOL_APPROVAL_MODE",
	"CLINE_TOOL_APPROVAL_DIR",
}

// Review runs the Cline CLI in an isolated scratch area and returns the
// reviewer's final text.
func (adapter *ClineAdapter) Review(ctx context.Context, invocation Invocation) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("cline")
	if err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: %w", err)
	}
	// A launcher that replaces the isolated environment would hand the
	// reviewer the user's own hook files to execute. The transport refuses
	// that launcher instead of claiming an isolation it cannot keep.
	if err := checkClineLauncherIsolation(binary); err != nil {
		return nil, err
	}

	scratchDir := adapter.scratchDir
	if scratchDir == nil {
		scratchDir = func(pattern string) (string, error) { return os.MkdirTemp("", pattern) }
	}
	scratch, err := scratchDir("gentle-ai-cline-reviewer-*")
	if err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	workDir := filepath.Join(scratch, "work")
	configDir := filepath.Join(scratch, "config")
	dataDir := filepath.Join(scratch, "data")
	homeDir := filepath.Join(scratch, "home")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: create scratch directory: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: create scratch directory: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: create scratch directory: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: create scratch directory: %w", err)
	}
	if err := adapter.copyProviderSettings(dataDir); err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: %w", err)
	}

	commandContext := adapter.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	arguments := append([]string(nil), clineReviewerArguments...)
	arguments = append(arguments, "--config", configDir, "--data-dir", dataDir)
	if strings.TrimSpace(adapter.Model) != "" {
		arguments = append(arguments, "--model", strings.TrimSpace(adapter.Model))
	}
	prompt := string(invocation.Prompt())
	command := commandContext(ctx, binary, append([]string{prompt}, arguments...)...)
	command.Dir = workDir
	command.Env = clineChildEnvironment(homeDir, configDir, dataDir)
	// The prompt travels as the positional argument, so stdin stays empty and
	// the runtime never waits for piped input.
	command.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("cline reviewer transport failed: %w: %s", err, reviewerTransportFailureDetail(stderr.String(), stdout.String()))
	}
	verdict, err := clineReviewerVerdict(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("cline reviewer transport unavailable: %w", err)
	}
	return verdict, nil
}

// copyProviderSettings copies the user's provider settings -- the credentials
// the reviewer authenticates with -- into the scratch data directory. The
// copy is what the runtime reads and writes: the real file is never written,
// so the runtime's own provider-selection persistence cannot change the
// user's installation. A missing settings file is not an error: the runtime
// may authenticate from the environment instead.
func (adapter *ClineAdapter) copyProviderSettings(dataDir string) error {
	homeDir := adapter.homeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}
	home, err := homeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	source := filepath.Join(home, ".cline", "data", "settings", "providers.json")
	data, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read provider settings: %w", err)
	}
	target := filepath.Join(dataDir, "settings", "providers.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create provider settings directory: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write provider settings copy: %w", err)
	}
	return nil
}
