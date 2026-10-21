package reviewerprovider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// This file reduces the Cline runtime's own stdout to the reviewer's final
// text. It is the runtime's output format, not provider-owned review
// semantics: nothing here reads a reviewer binding, a lens, a budget, or an
// admission rule. The reviewer provider boundary stays in the adapter, which
// only invokes the runtime and returns bytes.

// clineRunResult is the runtime's own final report line
// (cline/apps/cli/src/runtime/run-agent.ts): the CLI writes one JSON object
// per stdout line, and the line whose type is "run_result" carries the final
// text of the run and the reason the run ended.
type clineRunResult struct {
	Type         string `json:"type"`
	Text         string `json:"text"`
	FinishReason string `json:"finishReason"`
}

// clineReviewerVerdict extracts the reviewer's final text from the runtime's
// JSON event stream. It fails closed: a stream without a final result, an
// empty final text, or a stream that carries no final result at all cannot
// produce a verdict, and none of them is ever turned into one.
func clineReviewerVerdict(stdout []byte) ([]byte, error) {
	var found bool
	var text string
	var finishReason string
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var result clineRunResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue
		}
		if result.Type != "run_result" {
			continue
		}
		found = true
		text = result.Text
		finishReason = result.FinishReason
	}
	if !found {
		return nil, fmt.Errorf("the Cline run produced no final result")
	}
	// Only a completed run carries a verdict. The runtime reports a failed
	// run the same way it reports a successful one -- a run_result line whose
	// text is the failure -- so the finish reason is what separates a
	// reviewer answer from an error message.
	if finishReason != "completed" {
		return nil, fmt.Errorf("the Cline run ended with %q and produced no verdict", finishReason)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("the Cline run produced an empty final result")
	}
	return []byte(text), nil
}
