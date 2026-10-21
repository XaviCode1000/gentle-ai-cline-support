package agenthooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestClineSkillRegistryHookIsAFile pins the Cline hook contract: the
// runtime executes a file named after the event in its hooks directory, so
// gentle-ai writes that file instead of merging a settings document.
func TestClineSkillRegistryHookIsAFile(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCline)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	first, err := InstallSkillRegistry(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first install: %+v %v", first, err)
	}
	path := ClineSkillRegistryHookPath(home, adapter)
	if filepath.Base(path) != "UserPromptSubmit.sh" {
		t.Fatalf("hook file %q is not the Cline event file", path)
	}
	if filepath.Dir(path) != filepath.Join(adapter.GlobalConfigDir(home), "hooks") {
		t.Fatalf("hook file %q is not inside the runtime hooks directory", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(data), "gentle-ai skill-registry refresh") {
		t.Fatalf("hook does not refresh the skill registry: %s", data)
	}
	if !strings.Contains(string(data), "|| true") {
		t.Fatalf("hook can fail the session: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("hook is not executable: %s", info.Mode())
	}
	second, err := InstallSkillRegistry(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second install: %+v %v", second, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatalf("repeat changed bytes: %v", err)
	}
}

// TestClineSkillRegistryHookKeepsUserHook proves gentle-ai never replaces a
// hook file it does not own.
func TestClineSkillRegistryHookKeepsUserHook(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCline)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := ClineSkillRegistryHookPath(home, adapter)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	userHook := "#!/usr/bin/env bash\necho mine\n"
	if err := os.WriteFile(path, []byte(userHook), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSkillRegistry(home, adapter); err == nil {
		t.Fatal("install overwrote a user-owned hook file")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != userHook {
		t.Fatalf("user hook changed: %v %s", err, after)
	}
}

// TestClineSkillRegistryHookIsNotASettingsMerge proves the Cline path never
// writes a settings document: the runtime would not read it.
func TestClineSkillRegistryHookIsNotASettingsMerge(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCline)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if _, err := InstallSkillRegistry(home, adapter); err != nil {
		t.Fatalf("install: %v", err)
	}
	settings := adapter.SettingsPath(home)
	if _, err := os.Stat(settings); err == nil {
		t.Fatalf("cline install wrote a settings document at %q", settings)
	}
}
