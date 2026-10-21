package reviewerprovider

import (
	"os"
	"strings"
)

// This file builds the reviewer process' environment. It is runtime plumbing,
// not provider-owned review semantics: the adapter itself only invokes the
// runtime and returns bytes.

// clineChildEnvironment builds the child environment: the isolated home
// replaces the user's home, and every Cline variable that would point the
// runtime back at the user's real directories is removed.
func clineChildEnvironment(homeDir string) []string {
	scrubbed := make(map[string]struct{}, len(clineScrubbedEnvironment))
	for _, name := range clineScrubbedEnvironment {
		scrubbed[name] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+1)
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
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}
