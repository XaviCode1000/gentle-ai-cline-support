package reviewerprovider

import (
	"os"
	"path/filepath"
	"strings"
)

// This file builds the reviewer process' environment. It is runtime plumbing,
// not provider-owned review semantics: the adapter itself only invokes the
// runtime and returns bytes.

// clineChildEnvironment builds the child environment: the isolated home
// replaces the user's home, every Cline variable that would point the
// runtime back at the user's real directories is removed, and every root the
// runtime resolves from an environment variable is pinned to the scratch.
//
// Pinning the roots matters as much as the home: the runtime resolves the
// hooks and skills directories relative to HOME, and a launcher that replaces
// HOME would hand the reviewer the user's own hook files to execute. The
// explicit roots below are read by the runtime before the home-relative
// fallbacks, so they keep the reviewer inside the scratch even when the
// home cannot be replaced.
func clineChildEnvironment(homeDir, configDir, dataDir string) []string {
	scrubbed := make(map[string]struct{}, len(clineScrubbedEnvironment))
	for _, name := range clineScrubbedEnvironment {
		scrubbed[name] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+16)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, drop := scrubbed[name]; drop {
			continue
		}
		if name == "HOME" || name == "USERPROFILE" {
			continue
		}
		environment = append(environment, entry)
	}
	pinned := [][2]string{
		{"HOME", homeDir},
		{"USERPROFILE", homeDir},
		{"CLINE_DIR", configDir},
		{"CLINE_DATA_DIR", dataDir},
		{"CLINE_SESSION_DATA_DIR", filepath.Join(dataDir, "sessions")},
		{"CLINE_TEAM_DATA_DIR", filepath.Join(dataDir, "teams")},
		{"CLINE_CONNECTOR_DATA_DIR", filepath.Join(dataDir, "connectors")},
		{"CLINE_DB_DATA_DIR", filepath.Join(dataDir, "db")},
		{"CLINE_PROVIDER_SETTINGS_PATH", filepath.Join(dataDir, "settings", "providers.json")},
		{"CLINE_GLOBAL_SETTINGS_PATH", filepath.Join(configDir, "settings", "global-settings.json")},
		{"CLINE_MCP_SETTINGS_PATH", filepath.Join(configDir, "settings", "cline_mcp_settings.json")},
		{"CLINE_CONNECTOR_SETTINGS_PATH", filepath.Join(configDir, "settings", "connectors.json")},
		{"CLINE_CONNECTORS_DB_PATH", filepath.Join(dataDir, "db", "connectors.db")},
		{"CLINE_CRON_DB_PATH", filepath.Join(dataDir, "db", "cron.db")},
		{"CLINE_TASKS_DB_PATH", filepath.Join(dataDir, "db", "tasks.db")},
	}
	for _, entry := range pinned {
		environment = append(environment, entry[0]+"="+entry[1])
	}
	return environment
}
