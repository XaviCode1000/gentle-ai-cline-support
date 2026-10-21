package agenthooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Cline runs its hooks as executable FILES, not as a settings document: the
// runtime looks for a file named after the event in its hooks directory and
// executes it with the workspace as the working directory
// (cline/sdk/packages/core/src/hooks/hook-file-config.ts and
// cline/sdk/packages/shared/src/storage/paths.ts). The Claude and Codex
// writers below merge JSON settings instead, so Cline needs its own path and
// must never reuse their helpers: a JSON rewrite of a hook file would
// corrupt it.
//
// The command shape stays the same as every other runtime: refresh the skill
// registry for the current project, quietly, and never fail the session.

const (
	// clineSkillRegistryHookFile is the event file the Cline runtime executes
	// when the user submits a prompt.
	clineSkillRegistryHookFile = "UserPromptSubmit.sh"
	// clineSkillRegistryCommand resolves the project directory from the
	// process working directory, which the Cline runtime sets to the
	// workspace before running the hook.
	clineSkillRegistryCommand = `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true`
	// clineSkillRegistryHookBody is the exact managed content. It doubles as
	// the ownership marker: a file with different bytes belongs to the user.
	clineSkillRegistryHookBody = "#!/usr/bin/env bash\n" + clineSkillRegistryCommand + "\n"
)

// ClineHooksDir returns the directory the Cline runtime reads hook files
// from, for the runtime's global configuration root.
func ClineHooksDir(homeDir string, adapter agents.Adapter) string {
	return filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks")
}

// ClineSkillRegistryHookPath returns the managed skill-registry hook file
// for the Cline runtime.
func ClineSkillRegistryHookPath(homeDir string, adapter agents.Adapter) string {
	return filepath.Join(ClineHooksDir(homeDir, adapter), clineSkillRegistryHookFile)
}

// ClineSkillRegistryHookCommand returns the command the managed Cline hook
// runs, so backup, uninstall and the retention checks can recognise it.
func ClineSkillRegistryHookCommand() string { return clineSkillRegistryCommand }

// IsManagedClineSkillRegistryHook reports whether the given bytes are the
// managed Cline hook. It is the ownership check the teardown uses: a hook
// file with any other content belongs to the user and is left alone.
func IsManagedClineSkillRegistryHook(data []byte) bool {
	return bytes.Equal(data, []byte(clineSkillRegistryHookBody))
}

// installClineSkillRegistryHook writes the managed Cline hook file.
//
// It fails closed: a hook file that exists with different bytes is the
// user's own hook and is never overwritten, and a hook path that is not a
// regular file is refused rather than replaced.
func installClineSkillRegistryHook(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentCline {
		return Result{}, nil
	}
	path := ClineSkillRegistryHookPath(homeDir, adapter)
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("cline hook %q is not a regular file", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, readErr
		}
		if bytes.Equal(data, []byte(clineSkillRegistryHookBody)) {
			return Result{Files: []string{path}}, nil
		}
		return Result{}, fmt.Errorf("cline hook %q exists and is not managed by gentle-ai", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, err
	}
	wr, err := filemerge.WriteFileAtomic(path, []byte(clineSkillRegistryHookBody), 0o755)
	if err != nil {
		return Result{}, fmt.Errorf("write cline hook %q: %w", path, err)
	}
	return Result{Changed: wr.Changed, Files: []string{path}}, nil
}
