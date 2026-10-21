package reviewerprovider

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The Cline runtime resolves some of its roots relative to the user's home:
// the hook files under the documents directory and the legacy agent skills
// directory. A launcher that replaces HOME — a wrapper that pins the user's
// home, which is a common way to expose a CLI to one configuration — would
// hand the reviewer those directories, and the runtime executes the hook
// files it finds there.
//
// The transport cannot repair that from the outside: the override happens
// inside the launcher, after the environment is handed over. So it refuses:
// a launcher that overrides the environment it was given is not a transport
// the reviewer may run under, because the reviewer's isolation would be a
// claim rather than a fact. The transport fails closed with the exact path
// and the exact variables, and the operator can point the reviewer at a
// launcher that honours the environment.

// clineLauncherOverride matches an assignment that replaces one of the roots
// the reviewer depends on. It matches a plain assignment, an export and an
// `env NAME=value` argument, in shell, fish and Python launchers alike.
var clineLauncherOverride = regexp.MustCompile(`(?m)(^|[\s'"])(HOME|USERPROFILE|CLINE_DIR|CLINE_DATA_DIR|CLINE_HOOKS_DIR|CLINE_GLOBAL_SETTINGS_PATH|CLINE_MCP_SETTINGS_PATH|CLINE_PROVIDER_SETTINGS_PATH)\s*=`)

// clineLauncherScriptLimit bounds how much of a launcher is read.
const clineLauncherScriptLimit = 64 * 1024

// checkClineLauncherIsolation refuses a launcher that overrides the isolated
// environment. A native executable cannot do that, so only scripts are
// inspected.
func checkClineLauncherIsolation(binary string) error {
	info, err := os.Stat(binary)
	if err != nil {
		return fmt.Errorf("cline reviewer transport unavailable: inspect launcher %q: %w", binary, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cline reviewer transport unavailable: launcher %q is not a regular file", binary)
	}
	file, err := os.Open(binary)
	if err != nil {
		return fmt.Errorf("cline reviewer transport unavailable: read launcher %q: %w", binary, err)
	}
	defer func() { _ = file.Close() }()
	header := make([]byte, clineLauncherScriptLimit)
	read, err := file.Read(header)
	if err != nil && read == 0 {
		return fmt.Errorf("cline reviewer transport unavailable: read launcher %q: %w", binary, err)
	}
	content := header[:read]
	// A native executable is not a launcher script: its bytes never carry a
	// shell assignment, and the check stops here.
	if !bytes.HasPrefix(bytes.TrimLeft(content, " \t\r\n"), []byte("#!")) {
		return nil
	}
	match := clineLauncherOverride.FindSubmatch(content)
	if match == nil {
		return nil
	}
	return fmt.Errorf(
		"cline reviewer transport unavailable: launcher %q overrides %s, so the reviewer could reach the user's own hook files and skills; point the reviewer at a launcher that honours the environment it is given",
		binary, string(match[2]),
	)
}

// clineLauncherDescription names the launcher for the error messages.
func clineLauncherDescription(binary string) string {
	if strings.TrimSpace(binary) == "" {
		return "cline"
	}
	return binary
}
