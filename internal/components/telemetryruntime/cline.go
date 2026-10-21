package telemetryruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/telemetry"
)

// clineMaxBytes bounds the one hook payload this runtime reads. A hook
// reports a small event, never an unbounded stream.
const clineMaxBytes = 1 << 20

// clineHook is the Cline runtime's hook payload
// (cline/sdk/packages/shared/src/hooks/events.ts). Only the fields that
// describe the observed turn cross this boundary: ids, workspace roots, tool
// input, tool output, prompts, and error text stay in the runtime.
type clineHook struct {
	HookName       string  `json:"hookName"`
	AgentID        string  `json:"agent_id"`
	ParentAgentID  *string `json:"parent_agent_id"`
	Iteration      *int    `json:"iteration"`
	SessionContext struct {
		RootSessionID string `json:"rootSessionId"`
	} `json:"sessionContext"`
}

// clineAgentClasses maps the Cline configured agents that carry a known
// review class onto the runtime telemetry vocabulary. Every other agent stays
// unknown: the runtime never invents a class.
var clineAgentClasses = map[string]string{
	"review-risk":        "review-risk",
	"review-readability": "review-readability",
	"review-reliability": "review-reliability",
	"review-resilience":  "review-resilience",
	"review-refuter":     "review-refuter",
	"review-validator":   "review-validator",
	"jd-judge-a":         "jd-judge-a",
	"jd-judge-b":         "jd-judge-b",
	"jd-fix-agent":       "jd-fix-agent",
}

// clineUnavailableToken is the observation shape for a metric the runtime
// does not report: the count is unavailable, never zero.
var clineUnavailableToken = json.RawMessage(`{"reported":0,"unavailable":1,"unsupported":0,"sum":0}`)

// clineUnsupportedToken is the observation shape for a metric the runtime
// cannot measure at all.
var clineUnsupportedToken = json.RawMessage(`{"reported":0,"unavailable":0,"unsupported":1,"sum":0}`)

// SendCline handles one Cline hook in memory and sends at most once. Source
// paths, ids, tool data, and file contents never enter the aggregate or an
// error message.
//
// The Cline hook payload carries no model, no effort, and no token usage, so
// the row reports exactly what the runtime proved: one finished turn, an
// unknown model, an unavailable effort, and unavailable token counts.
func SendCline(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) string {
	decision, err := sendCline(ctx, home, getenv, input, client)
	if err != nil {
		return "discarded"
	}
	return decision
}

func sendCline(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) (string, error) {
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	data, err := io.ReadAll(io.LimitReader(input, clineMaxBytes+1))
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	if err != nil || len(data) > clineMaxBytes {
		return "", errEnvelope
	}
	var hook clineHook
	if err := json.Unmarshal(data, &hook); err != nil {
		return "", errEnvelope
	}
	row, ok := clineRow(hook)
	if !ok {
		return "ignored", nil
	}
	batch, err := json.Marshal(telemetry.RuntimeBatch{
		Schema: telemetry.RuntimeSchema, Registry: json.RawMessage("1"), Host: "cline", Rows: []telemetry.RuntimeRow{row},
	})
	if err != nil || len(batch) > telemetry.RuntimeMaxBytes {
		return "", errEnvelope
	}
	return telemetry.SendRuntime(ctx, home, getenv, bytes.NewReader(batch), client), nil
}

// clineRow projects one Cline hook event onto the runtime telemetry row. It
// reports nothing the runtime did not observe: the model is unknown because
// the hook carries none, the effort is unavailable for the same reason, and
// the token counts are unavailable rather than zero.
func clineRow(hook clineHook) (telemetry.RuntimeRow, bool) {
	switch hook.HookName {
	case "agent_end":
	case "agent_error":
	default:
		return telemetry.RuntimeRow{}, false
	}
	row := telemetry.RuntimeRow{
		Model:           telemetry.RuntimeModel{Provider: "unknown", ID: "unknown"},
		ModelEvidence:   "unknown",
		AgentKind:       "orchestrator",
		AgentClass:      "orchestrator",
		SelectedEffort:  "unavailable",
		EffectiveEffort: "unavailable",
		Launches:        json.RawMessage("1"),
		Responses:       json.RawMessage("null"),
		Input:           clineUnavailableToken,
		Output:          clineUnavailableToken,
		CacheRead:       clineUnavailableToken,
		CacheCreation:   clineUnavailableToken,
		ReasoningTokens: clineUnsupportedToken,
		TotalTokens:     clineUnavailableToken,
		ErrorCategory:   "none",
		Duration:        telemetry.RuntimeDuration{Kind: "unavailable", MeasuredCount: json.RawMessage("0"), SumMS: json.RawMessage("null")},
	}
	if hook.HookName == "agent_error" {
		row.ErrorCategory = "unknown"
	}
	// A turn with a parent agent is a subagent turn; the runtime reports the
	// parent/child relation, not the class, so the class is only named when
	// the agent id is a known review agent.
	if hook.ParentAgentID != nil && *hook.ParentAgentID != "" {
		row.AgentKind = "custom"
		row.AgentClass = "unknown"
		if class, known := clineAgentClasses[strings.TrimSpace(hook.AgentID)]; known {
			row.AgentClass = class
		}
	}
	return row, true
}
