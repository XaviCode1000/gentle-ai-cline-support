package telemetryruntime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/telemetry"
)

// TestClineRowReportsOnlyWhatTheHookProves proves the Cline row never claims
// more than the runtime's hook payload proves: an unknown model, an
// unavailable effort, and unavailable token counts instead of zeros.
func TestClineRowReportsOnlyWhatTheHookProves(t *testing.T) {
	row, ok := clineRow(clineHook{HookName: "agent_end", AgentID: "main"})
	if !ok {
		t.Fatal("agent_end produced no row")
	}
	if row.Model != (telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"}) || row.ModelEvidence != "unknown" {
		t.Fatalf("model = %#v, evidence = %q", row.Model, row.ModelEvidence)
	}
	if row.SelectedEffort != "unavailable" || row.EffectiveEffort != "unavailable" {
		t.Fatalf("effort = %q/%q, want unavailable", row.SelectedEffort, row.EffectiveEffort)
	}
	if string(row.Launches) != "1" || string(row.Responses) != "null" {
		t.Fatalf("launches = %s, responses = %s", row.Launches, row.Responses)
	}
	if string(row.Input) != `{"reported":0,"unavailable":1,"unsupported":0,"sum":0}` {
		t.Fatalf("input tokens = %s, want an unavailable observation", row.Input)
	}
	if string(row.ReasoningTokens) != `{"reported":0,"unavailable":0,"unsupported":1,"sum":0}` {
		t.Fatalf("reasoning tokens = %s, want an unsupported observation", row.ReasoningTokens)
	}
	if row.AgentKind != "orchestrator" || row.AgentClass != "orchestrator" {
		t.Fatalf("agent = %q/%q, want the orchestrator", row.AgentKind, row.AgentClass)
	}
	if row.ErrorCategory != "none" {
		t.Fatalf("error category = %q", row.ErrorCategory)
	}
}

// TestClineRowNamesAKnownReviewAgent proves a subagent turn is attributed to
// the review agent the runtime actually ran, and to nothing else.
func TestClineRowNamesAKnownReviewAgent(t *testing.T) {
	parent := "main"
	row, ok := clineRow(clineHook{HookName: "agent_end", AgentID: "review-risk", ParentAgentID: &parent})
	if !ok {
		t.Fatal("agent_end produced no row")
	}
	if row.AgentKind != "custom" || row.AgentClass != "review-risk" {
		t.Fatalf("agent = %q/%q, want the review-risk class", row.AgentKind, row.AgentClass)
	}
	unknown := "some-other-agent"
	row, _ = clineRow(clineHook{HookName: "agent_end", AgentID: unknown, ParentAgentID: &parent})
	if row.AgentClass != "unknown" {
		t.Fatalf("agent class = %q, want unknown", row.AgentClass)
	}
}

// TestClineRowIgnoresEventsWithoutATurn proves only the events that end a
// turn produce a row.
func TestClineRowIgnoresEventsWithoutATurn(t *testing.T) {
	for _, hookName := range []string{"agent_start", "tool_call", "tool_result", "prompt_submit", "session_shutdown", ""} {
		t.Run(hookName, func(t *testing.T) {
			if _, ok := clineRow(clineHook{HookName: hookName}); ok {
				t.Fatalf("%s produced a row", hookName)
			}
		})
	}
}

// TestClineRowMarksAFailedTurn proves a failed turn is reported as an
// unknown error category, never as a clean turn.
func TestClineRowMarksAFailedTurn(t *testing.T) {
	row, ok := clineRow(clineHook{HookName: "agent_error", AgentID: "main"})
	if !ok {
		t.Fatal("agent_error produced no row")
	}
	if row.ErrorCategory != "unknown" {
		t.Fatalf("error category = %q, want unknown", row.ErrorCategory)
	}
}

// TestClineHookPayloadKeepsSourceDataOut proves the Cline hook struct never
// carries the runtime's source data: no workspace root, no tool input, no
// prompt.
func TestClineHookPayloadKeepsSourceDataOut(t *testing.T) {
	payload := `{"hookName":"agent_end","taskId":"task-1","agent_id":"main","parent_agent_id":null,"workspaceRoots":["/home/user/private"],"userPromptSubmit":{"prompt":"PRIVATE_PROMPT"},"tool_call":{"name":"read_files","input":{"path":"/home/user/private/file.ts"}}}`
	var hook clineHook
	if err := json.Unmarshal([]byte(payload), &hook); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(hook)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"PRIVATE_PROMPT", "/home/user/private", "read_files", "task-1"} {
		if strings.Contains(string(encoded), private) {
			t.Errorf("Cline hook keeps source data %q: %s", private, encoded)
		}
	}
}
