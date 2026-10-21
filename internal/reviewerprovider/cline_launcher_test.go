package reviewerprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckClineLauncherIsolationRefusesAnEnvironmentOverride proves the
// transport refuses a launcher that replaces the isolated environment: such a
// launcher would hand the reviewer the user's own hook files to execute.
func TestCheckClineLauncherIsolationRefusesAnEnvironmentOverride(t *testing.T) {
	cases := map[string]struct {
		content  string
		wantErr  bool
		variable string
	}{
		"pins the home":            {content: "#!/usr/bin/env bash\nexec env HOME=/home/user cline \"$@\"\n", wantErr: true, variable: "HOME"},
		"pins the config root":     {content: "#!/usr/bin/env bash\nexport CLINE_DIR=/home/user/.cline\n", wantErr: true, variable: "CLINE_DIR"},
		"pins the hooks directory": {content: "#!/bin/sh\nCLINE_HOOKS_DIR=/home/user/.cline/hooks\n", wantErr: true, variable: "CLINE_HOOKS_DIR"},
		"pins the provider file":   {content: "#!/usr/bin/env bash\nCLINE_PROVIDER_SETTINGS_PATH=/home/user/p.json\n", wantErr: true, variable: "CLINE_PROVIDER_SETTINGS_PATH"},
		"honours the environment":  {content: "#!/usr/bin/env bash\nexec bun \"$ROOT/apps/cli/src/index.ts\" \"$@\"\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cline")
			if err := os.WriteFile(path, []byte(tc.content), 0o755); err != nil {
				t.Fatal(err)
			}
			err := checkClineLauncherIsolation(path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("checkClineLauncherIsolation accepted a launcher that overrides the environment")
				}
				if !strings.Contains(err.Error(), tc.variable) {
					t.Fatalf("error %q does not name the overridden variable %q", err, tc.variable)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkClineLauncherIsolation refused a launcher that honours the environment: %v", err)
			}
		})
	}
}

// TestCheckClineLauncherIsolationAcceptsANativeBinary proves the guard only
// inspects launcher scripts: a native executable cannot replace the
// environment it is given.
func TestCheckClineLauncherIsolationAcceptsANativeBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cline")
	// An ELF-like header: the bytes are not a script, so no assignment can
	// be read out of them.
	if err := os.WriteFile(path, []byte("\x7fELF\x02\x01\x01\x00HOME=/home/user\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkClineLauncherIsolation(path); err != nil {
		t.Fatalf("checkClineLauncherIsolation refused a native binary: %v", err)
	}
}

// TestClineChildEnvironmentPinsEveryRoot proves the child environment pins
// every root the runtime resolves from the environment, and leaves no
// approval or path variable pointing at the user's directories.
func TestClineChildEnvironmentPinsEveryRoot(t *testing.T) {
	t.Setenv("HOME", "/home/user")
	t.Setenv("CLINE_TOOL_APPROVAL_MODE", "always")
	t.Setenv("CLINE_TOOL_APPROVAL_DIR", "/home/user/.cline/approvals")
	t.Setenv("CLINE_DIR", "/home/user/.cline")

	environment := clineChildEnvironment("/scratch/home", "/scratch/config", "/scratch/data")
	values := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	for name, want := range map[string]string{
		"HOME":                         "/scratch/home",
		"CLINE_DIR":                    "/scratch/config",
		"CLINE_DATA_DIR":               "/scratch/data",
		"CLINE_PROVIDER_SETTINGS_PATH": "/scratch/data/settings/providers.json",
		"CLINE_GLOBAL_SETTINGS_PATH":   "/scratch/config/settings/global-settings.json",
		"CLINE_MCP_SETTINGS_PATH":      "/scratch/config/settings/cline_mcp_settings.json",
	} {
		if got := values[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"CLINE_TOOL_APPROVAL_MODE", "CLINE_TOOL_APPROVAL_DIR"} {
		if _, present := values[name]; present {
			t.Errorf("%s survived in the reviewer environment", name)
		}
	}
}
