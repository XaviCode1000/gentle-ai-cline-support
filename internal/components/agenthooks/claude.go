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

// InstallRetainedClaudeHooks installs review and telemetry hooks independently of SDD.
func InstallRetainedClaudeHooks(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentClaudeCode {
		return Result{}, nil
	}
	path := adapter.SettingsPath(homeDir)
	if path == "" {
		return Result{}, nil
	}
	var data []byte
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("hook settings %q is not a regular file", path)
		}
		if data, err = os.ReadFile(path); err != nil {
			return Result{}, err
		}
	}
	root, err := decodeHookSettings(data)
	if err != nil {
		return Result{}, fmt.Errorf("parse Claude settings %q: %w", path, err)
	}
	raw, exists := root["hooks"]
	hooks, ok := raw.(map[string]any)
	if exists && !ok {
		return Result{}, fmt.Errorf("Claude settings %q has unsupported hooks shape: want object", path)
	}
	if !ok {
		hooks = map[string]any{}
	}
	command := fmt.Sprintf("gentle-ai review stop-hook --agent %s", adapter.Agent())
	entries := []struct {
		key, matcher, command string
		timeout               int
		async                 bool
	}{
		{"Stop", "", command, 60, false},
		{"SessionStart", "startup|resume|clear|compact", command, 30, false},
		{"SubagentStop", "", "gentle-ai telemetry runtime claude --json", 5, true},
		{"Stop", "", "gentle-ai telemetry runtime claude --json", 5, true},
	}
	changed := false
	for _, e := range entries {
		raw, exists := hooks[e.key]
		list, ok := raw.([]any)
		if exists && !ok {
			return Result{}, fmt.Errorf("Claude settings %q has unsupported hooks.%s shape: want array", path, e.key)
		}
		if claudeHookListContains(list, e.command) {
			continue
		}
		hook := map[string]any{"type": "command", "command": e.command, "timeout": e.timeout}
		if e.async {
			hook["async"] = true
		}
		hooks[e.key] = append(list, map[string]any{"matcher": e.matcher, "hooks": []any{hook}})
		changed = true
	}
	if !changed {
		return Result{Files: []string{path}}, nil
	}
	out, err := encodeHookSettings(data, root, hooks)
	if err != nil {
		return Result{}, fmt.Errorf("rewrite Claude settings %q: %w", path, err)
	}
	wr, err := filemerge.WriteFileAtomic(path, out, 0644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: wr.Changed, Files: []string{path}}, nil
}

func claudeHookListContains(entries []any, command string) bool {
	for _, entry := range entries {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := item["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooks {
			h, ok := hook.(map[string]any)
			if ok && h["command"] == command {
				return true
			}
		}
	}
	return false
}

// Cline runs its hooks as executable FILES, not as a settings document: the
// runtime looks for a file named after the event in its hooks directory and
// executes it with the workspace as the working directory
// (cline/sdk/packages/core/src/hooks/hook-file-config.ts and
// cline/sdk/packages/shared/src/storage/paths.ts). The Cline hook files below
// therefore never reuse the JSON settings writers above: a JSON rewrite of a
// hook file would corrupt it.
//
// The Cline runtime has no Stop event. It ends a task with a tool call, so
// the review gate is the completion tool call, and the session baseline is
// the task start.
const (
	// clineReviewStopHookTaskStartFile is the Cline event file that records
	// the session's baseline candidate.
	clineReviewStopHookTaskStartFile = "TaskStart.sh"
	// clineReviewStopHookPreToolUseFile is the Cline event file that gates the
	// completion tool call while an unreviewed candidate exists.
	clineReviewStopHookPreToolUseFile = "PreToolUse.sh"
	// clineTelemetryHookTaskCompleteFile is the Cline event file that reports
	// the finished turn to the runtime telemetry.
	clineTelemetryHookTaskCompleteFile = "TaskComplete.sh"
)

// clineReviewStopHookCommand is the command the review hook files run. The
// runtime identity comes from the adapter, never from a literal: only the
// caller can state which runtime is executing (issue #2440).
func clineReviewStopHookCommand(adapter agents.Adapter) string {
	return fmt.Sprintf("gentle-ai review stop-hook --agent %s", adapter.Agent())
}

// clineTelemetryHookCommand is the command the telemetry hook file runs.
func clineTelemetryHookCommand(adapter agents.Adapter) string {
	return fmt.Sprintf("gentle-ai telemetry runtime %s --json", adapter.Agent())
}

// ClineReviewStopHookPaths returns the managed Cline hook files, so backup,
// uninstall, and the retention checks can recognise them.
func ClineReviewStopHookPaths(homeDir string, adapter agents.Adapter) []string {
	dir := ClineHooksDir(homeDir, adapter)
	return []string{
		filepath.Join(dir, clineReviewStopHookTaskStartFile),
		filepath.Join(dir, clineReviewStopHookPreToolUseFile),
		filepath.Join(dir, clineTelemetryHookTaskCompleteFile),
	}
}

// ClineHookCommand returns the command a managed Cline hook file runs.
func ClineHookCommand(name string, adapter agents.Adapter) string {
	switch name {
	case clineTelemetryHookTaskCompleteFile:
		return clineTelemetryHookCommand(adapter)
	default:
		return clineReviewStopHookCommand(adapter)
	}
}

// InstallRetainedClineHooks installs the review and telemetry hook files of
// the Cline runtime.
//
// It fails closed: a hook file that exists with different bytes belongs to
// the user and is never overwritten, and a hook path that is not a regular
// file is refused rather than replaced.
func InstallRetainedClineHooks(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentCline {
		return Result{}, nil
	}
	result := Result{}
	for _, path := range ClineReviewStopHookPaths(homeDir, adapter) {
		body := clineHookBody(filepath.Base(path), adapter)
		written, err := installClineHookFile(path, body)
		if err != nil {
			return Result{Files: result.Files}, err
		}
		if written {
			result.Changed = true
		}
		result.Files = append(result.Files, path)
	}
	return result, nil
}

// clineHookBody is the exact managed content of one Cline hook file. It
// doubles as the ownership marker: a file with different bytes belongs to the
// user.
func clineHookBody(name string, adapter agents.Adapter) []byte {
	return []byte("#!/usr/bin/env bash\n" + ClineHookCommand(name, adapter) + "\n")
}

// IsManagedClineHookFile reports whether the given bytes are the managed
// content of the Cline hook file with that name. It is the ownership check
// the teardown uses: a hook file with any other content belongs to the user
// and is left alone.
func IsManagedClineHookFile(name string, data []byte, adapter agents.Adapter) bool {
	return bytes.Equal(data, clineHookBody(name, adapter))
}

// installClineHookFile writes one managed Cline hook file and reports whether
// it changed anything.
func installClineHookFile(path string, body []byte) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("cline hook %q is not a regular file", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return false, readErr
		}
		if bytes.Equal(data, body) {
			return false, nil
		}
		return false, fmt.Errorf("cline hook %q exists and is not managed by gentle-ai", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	wr, err := filemerge.WriteFileAtomic(path, body, 0o755)
	if err != nil {
		return false, fmt.Errorf("write cline hook %q: %w", path, err)
	}
	return wr.Changed, nil
}
