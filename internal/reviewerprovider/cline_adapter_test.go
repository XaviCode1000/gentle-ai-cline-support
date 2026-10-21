package reviewerprovider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// clineFakeCLI writes a fake `cline` binary that records argv, the working
// directory, and the environment it was launched with, then prints the given
// stdout payload.
func clineFakeCLI(t *testing.T, dir, stdout string) string {
	t.Helper()
	record := filepath.Join(dir, "record.txt")
	script := filepath.Join(dir, "cline")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + record + "\n" +
		"printf 'cwd=%s\\n' \"$PWD\" >> " + record + "\n" +
		"printf 'home=%s\\n' \"$HOME\" >> " + record + "\n" +
		"printf 'cline_dir=%s\\n' \"${CLINE_DIR:-}\" >> " + record + "\n" +
		"printf 'provider_settings=%s\\n' \"${CLINE_PROVIDER_SETTINGS_PATH:-}\" >> " + record + "\n" +
		"cat > /dev/null\n" +
		"printf '%s' '" + strings.ReplaceAll(stdout, "'", "'\\''") + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// TestClineAdapterRunsIsolatedHeadlessReviewer pins the fresh-reviewer
// boundary Cline actually offers: the headless CLI invocation
// `cline "<prompt>" --json --system <text> --auto-approve false`, run in an
// empty scratch working directory with an isolated config root, an isolated
// data directory, and an isolated home, so the user's real ~/.cline is never
// read for context and never written.
func TestClineAdapterRunsIsolatedHeadlessReviewer(t *testing.T) {
	binDir := t.TempDir()
	record := filepath.Join(binDir, "record.txt")
	script := clineFakeCLI(t, binDir, `{"type":"run_result","finishReason":"completed","text":"{\"subject_hash\":\"sha256:aa\"}"}`+"\n")

	home := t.TempDir()
	realSettings := filepath.Join(home, ".cline", "data", "settings", "providers.json")
	if err := os.MkdirAll(filepath.Dir(realSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realSettings, []byte(`{"provider":"cline"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := NewClineAdapter()
	adapter.LookPath = func(string) (string, error) { return script, nil }
	adapter.homeDir = func() (string, error) { return home, nil }

	out, err := adapter.Review(context.Background(), NewInvocation([]byte("PROMPT-BYTES")))
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if string(out) != `{"subject_hash":"sha256:aa"}` {
		t.Fatalf("Review() = %q, want the reviewer final text", out)
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"PROMPT-BYTES", "--json", "--system", "--auto-approve", "false", "--config", "--data-dir"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "cline_dir=/\n") {
		t.Error("child inherited a CLINE_DIR from the parent environment")
	}
	if !strings.Contains(got, "cwd=") {
		t.Fatal("child did not run in a scratch working directory")
	}
	if !strings.Contains(got, "home=") {
		t.Fatal("child did not run with an isolated home")
	}
	// The isolated home and data directory must live inside the scratch area,
	// never in the user's real home.
	if !strings.Contains(got, "gentle-ai-cline-reviewer-") {
		t.Errorf("child state was not created inside the scratch directory; got:\n%s", got)
	}
	if strings.Contains(got, home) {
		t.Errorf("child state was created inside the real home %q; got:\n%s", home, got)
	}
	// The real provider settings file is read for credentials, never written.
	settings, err := os.ReadFile(realSettings)
	if err != nil || string(settings) != `{"provider":"cline"}` {
		t.Fatalf("real provider settings changed: %q, %v", settings, err)
	}
}

// TestClineAdapterRemovesItsScratchState proves the scratch state is explicit
// and removable: nothing survives the capture.
func TestClineAdapterRemovesItsScratchState(t *testing.T) {
	binDir := t.TempDir()
	script := clineFakeCLI(t, binDir, `{"type":"run_result","finishReason":"completed","text":"verdict"}`+"\n")
	adapter := NewClineAdapter()
	adapter.LookPath = func(string) (string, error) { return script, nil }
	adapter.homeDir = func() (string, error) { return t.TempDir(), nil }

	var scratch string
	adapter.scratchDir = func(pattern string) (string, error) {
		dir, err := os.MkdirTemp("", pattern)
		if err != nil {
			return "", err
		}
		scratch = dir
		return dir, nil
	}
	if _, err := adapter.Review(context.Background(), NewInvocation([]byte("p"))); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if scratch == "" {
		t.Fatal("adapter did not create a scratch directory")
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch directory survived the capture: %v", err)
	}
}

// TestClineAdapterFailsClosedWithoutAVerdict proves the transport never
// fabricates a reviewer result: a run that produced no final text is an
// unavailable capture, not an empty verdict.
func TestClineAdapterFailsClosedWithoutAVerdict(t *testing.T) {
	for name, stdout := range map[string]string{
		"no run result": `{"type":"run_start"}` + "\n",
		"empty text":    `{"type":"run_result","finishReason":"completed","text":""}` + "\n",
		"not json":      "the reviewer crashed\n",
	} {
		t.Run(name, func(t *testing.T) {
			binDir := t.TempDir()
			script := clineFakeCLI(t, binDir, stdout)
			adapter := NewClineAdapter()
			adapter.LookPath = func(string) (string, error) { return script, nil }
			adapter.homeDir = func() (string, error) { return t.TempDir(), nil }

			out, err := adapter.Review(context.Background(), NewInvocation([]byte("p")))
			if err == nil {
				t.Fatalf("Review() = %q, want an unavailable capture error", out)
			}
			if out != nil {
				t.Fatalf("Review() returned bytes %q with an error", out)
			}
		})
	}
}

// TestClineAdapterFailsClosedOnAFailedRun proves a run that did not complete
// is never captured as a verdict: the runtime reports a failed run the same
// way it reports a successful one, with the failure in the final text, and
// the transport must not hand that text back as a reviewer result.
func TestClineAdapterFailsClosedOnAFailedRun(t *testing.T) {
	for _, finishReason := range []string{"error", "aborted", "max_iterations", "mistake_limit"} {
		t.Run(finishReason, func(t *testing.T) {
			binDir := t.TempDir()
			script := clineFakeCLI(t, binDir, `{"type":"run_result","finishReason":"`+finishReason+`","text":"Unauthorized: re-authenticate"}`+"\n")
			adapter := NewClineAdapter()
			adapter.LookPath = func(string) (string, error) { return script, nil }
			adapter.homeDir = func() (string, error) { return t.TempDir(), nil }

			out, err := adapter.Review(context.Background(), NewInvocation([]byte("x")))
			if err == nil {
				t.Fatalf("Review() = %q, want an unavailable capture error", out)
			}
			if out != nil {
				t.Fatalf("Review() returned bytes %q with an error", out)
			}
		})
	}
}

// TestClineAdapterTransportUnavailable pins the fail-closed answer when the
// Cline binary is absent.
func TestClineAdapterTransportUnavailable(t *testing.T) {
	adapter := NewClineAdapter()
	adapter.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := adapter.Review(context.Background(), NewInvocation([]byte("x"))); err == nil {
		t.Fatal("Review() error = nil, want transport-unavailable error")
	}
}

// TestClineAdapterFailsClosedOnAFailingRun proves a non-zero exit is a
// transport failure, never a verdict.
func TestClineAdapterFailsClosedOnAFailingRun(t *testing.T) {
	binDir := t.TempDir()
	script := filepath.Join(binDir, "cline")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > /dev/null\necho 'not logged in' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := NewClineAdapter()
	adapter.LookPath = func(string) (string, error) { return script, nil }
	adapter.homeDir = func() (string, error) { return t.TempDir(), nil }
	out, err := adapter.Review(context.Background(), NewInvocation([]byte("x")))
	if err == nil || out != nil {
		t.Fatalf("Review() = %q, %v; want a transport failure", out, err)
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("transport failure does not carry the runtime's own reason: %v", err)
	}
}

// TestClineIsARegisteredCaptureRuntime pins the transport classification: Cline
// runs the reviewer itself, inside the capture command.
func TestClineIsARegisteredCaptureRuntime(t *testing.T) {
	if !CapturesInProcess(model.AgentCline) {
		t.Fatal("CapturesInProcess(cline) = false, want true")
	}
	if !RegisteredRuntime(model.AgentCline) {
		t.Fatal("RegisteredRuntime(cline) = false, want true")
	}
}
