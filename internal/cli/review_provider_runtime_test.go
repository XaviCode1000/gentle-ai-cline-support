package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
)

// TestClineReviewAdapterIsResolvedForTheClineRuntime proves the compiled
// transport dispatcher hands Cline its own adapter: the Cline runtime is
// never routed to another runtime's adapter, and never refused for lack of
// one.
func TestClineReviewAdapterIsResolvedForTheClineRuntime(t *testing.T) {
	for _, role := range []string{reviewProviderRoleLens, reviewProviderRoleRefuter, reviewProviderRoleTargetedValidator} {
		t.Run(role, func(t *testing.T) {
			adapter, err := reviewProviderAdapter(role, model.AgentCline)
			if err != nil {
				t.Fatalf("reviewProviderAdapter(%q, cline) error = %v", role, err)
			}
			if _, ok := adapter.(*reviewerprovider.ClineAdapter); !ok {
				t.Fatalf("reviewProviderAdapter(%q, cline) = %T, want *reviewerprovider.ClineAdapter", role, adapter)
			}
		})
	}
}

// TestClineReviewAdapterRunsTheReviewerInProcess proves Cline is classified
// as a runtime that captures the reviewer itself, inside the capture command.
func TestClineReviewAdapterRunsTheReviewerInProcess(t *testing.T) {
	if !reviewProviderCaptureRuntime(model.AgentCline) {
		t.Fatal("reviewProviderCaptureRuntime(cline) = false, want true")
	}
}
