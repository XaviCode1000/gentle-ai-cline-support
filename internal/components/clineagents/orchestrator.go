package clineagents

import (
	"fmt"
	"strings"
)

// The Cline orchestrator asset is generated, never hand-maintained.
//
// It is the canonical generic orchestrator asset plus a Cline-specific
// overlay: the runtime title, the binding line, the Cline Delegation
// Boundary, and the three memory lines that must name the runtime's
// no-MCP delegate contract. Hand-maintaining a copy of the generic asset
// is what let the Cline prompt drift from the canon and ship the
// small-model variant to every Cline session, so the copy is rebuilt from
// the source of truth and a test fails when the two diverge.

const (
	genericModelCapableOpen  = "<!-- section:model-capable -->"
	genericModelCapableClose = "<!-- /section:model-capable -->"
	genericModelSmallOpen    = "<!-- section:model-small -->"
	genericModelSmallClose   = "<!-- /section:model-small -->"
)

const (
	genericTitle    = "# Agent Teams Lite — Orchestrator Instructions\n"
	clineTitle      = "# Cline — Orchestrator Instructions\n"
	genericBinding  = "Bind this to the dedicated orchestrator agent or rule only."
	clineBinding    = "Bind this to the Cline orchestrator rule only."
	oddWorkflowHead = "### Organic Driven Development Is The Default Workflow (MANDATORY)\n"
)

// clineDelegationBoundary is the Cline-specific delivery contract. It
// narrows how delegation is expressed on this runtime and never replaces
// the ODD routing it follows.
const clineDelegationBoundary = `## Cline Delegation Boundary

Cline-specific delivery notes for the ODD protocol above. They narrow how
delegation is expressed on this runtime; they never replace ODD routing.

- Configured agents live in ` + "`~/.cline/agents/`" + ` as YAML files (` + "`name`" + `,
  ` + "`description`" + `, ` + "`tools`" + `, ` + "`skills`" + `, prompt body). Gentle AI owns only the
  files it installs; project-level ` + "`.cline/agents/`" + ` entries shadow
  same-named global ones, so never overwrite them without explicit
  authorization.
- Subagents spawned with ` + "`spawn_agent`" + ` run in a separate context window with
  the mode's built-in tools only. They do not receive MCP servers (Engram,
  Context7, CodeGraph): any memory write or external lookup they need must
  run in the lead, or the subagent must return its result payload, including
  any Key Learnings, for the lead to persist.
- Keep parallel subagent batches small and focused; every subagent shares the
  session budget. Never nest subagents unless a single level cannot express
  the work.
- Approving a ` + "`spawn_agent`" + ` call approves everything that subagent does:
  read the delegated task text before approving.
- Plan-mode subagents do not edit files; Act-mode subagents may. State the
  mode before delegating.
- Global rules under ` + "`~/.cline/rules/`" + ` apply to lead and subagents alike;
  keep agent-specific contracts in the configured-agent YAML, not in global
  rules.
- Managed skills live in ` + "`~/.cline/skills/`" + ` and load on demand; the lead
  resolves skill paths once and passes them explicitly.
- Prefer ` + "`spawn_agent`" + ` and ` + "`subagent_*`" + ` configured agents for delegation; use
  team mode only when the user explicitly asks for persistent multi-session
  teamwork. Team mode keeps a task board across sessions but caps concurrent
  runs at two, gives teammates no MCP tools, resumes under a fragile
  session-or-team key, and stays off in yolo mode.
- Implementation delegates get no surfaces mechanism: inline the exact file
  paths, scope, and acceptance criteria in the delegation text, one level
  only, and expect observed outcomes back, never summaries.

`

// The three memory lines name where a delegate's findings can be persisted.
// Cline delegates have no MCP server, so they return a payload and the lead
// persists it; every other runtime's delegate writes to Engram itself.
const (
	genericWriteContextLine = "- Write context: sub-agent MUST save significant discoveries, decisions, or bug fixes to engram via `mem_save` before returning. Sub-agent has full detail — save before returning, not after."
	clineWriteContextLine   = "- Write context: where the delegate can reach Engram (OpenCode/Claude/Codex agents), the sub-agent MUST save significant discoveries, decisions, or bug fixes via `mem_save` before returning. Cline `spawn_agent` delegates have no MCP access, so they instead return the full result payload including Key Learnings, and the lead or orchestrator persists those findings from the parent session."

	genericPromptAdditionLine = "- Always add to sub-agent prompt: `\"If you make important discoveries, decisions, or fix bugs, save them to engram via mem_save with project: '{project}'.\"`"
	clinePromptAdditionLine   = "- Cline delegation prompt addition: `\"When you find important discoveries, decisions, or fix bugs, include them, including any Key Learnings, in your result payload so the lead can persist them to engram from this session.\"` On runtimes with Engram access use: `\"If you make important discoveries, decisions, or fix bugs, save them to engram via mem_save with project: '{project}'.\"`"
)

// BuildOrchestratorAsset renders the Cline orchestrator asset from the
// canonical generic asset and the Cline overlay.
//
// It mirrors the generic render: the small-model section is removed and both
// model markers are stripped, so the result is the capable variant plus the
// content every model shares. It fails closed: every anchor it replaces must
// be present exactly once, and the generic asset must carry exactly one
// capable and one small model section, so a canon change that moves the
// anchors stops the build instead of silently shipping a partial
// orchestrator.
func BuildOrchestratorAsset(generic []byte) ([]byte, error) {
	content := string(generic)

	_, capableClose, err := modelSectionBounds(content, genericModelCapableOpen, genericModelCapableClose)
	if err != nil {
		return nil, err
	}
	smallOpen, smallClose, err := modelSectionBounds(content, genericModelSmallOpen, genericModelSmallClose)
	if err != nil {
		return nil, err
	}
	if smallOpen < capableClose {
		return nil, fmt.Errorf("clineagents: generic orchestrator model sections are out of order")
	}
	// Drop the small-model section with its markers, exactly as the generic
	// render does when it selects the capable variant.
	content = content[:smallOpen] + content[smallClose:]
	// Strip the capable markers; the section body stays.
	content = strings.Replace(content, genericModelCapableOpen+"\n", "", 1)
	content = strings.Replace(content, "\n"+genericModelCapableClose+"\n", "\n", 1)
	content = strings.TrimPrefix(content, genericModelCapableClose+"\n")
	content = strings.TrimSuffix(content, "\n"+genericModelCapableClose)

	replacements := []struct{ from, to string }{
		{genericTitle, clineTitle},
		{genericBinding, clineBinding},
		{genericWriteContextLine, clineWriteContextLine},
		{genericPromptAdditionLine, clinePromptAdditionLine},
	}
	for _, r := range replacements {
		if count := strings.Count(content, r.from); count != 1 {
			return nil, fmt.Errorf("clineagents: generic orchestrator anchor %q appears %d times, want exactly 1", r.from, count)
		}
		content = strings.Replace(content, r.from, r.to, 1)
	}

	if count := strings.Count(content, oddWorkflowHead); count != 1 {
		return nil, fmt.Errorf("clineagents: generic orchestrator anchor %q appears %d times, want exactly 1", oddWorkflowHead, count)
	}
	content = strings.Replace(content, oddWorkflowHead, clineDelegationBoundary+oddWorkflowHead, 1)

	return []byte(content), nil
}

// modelSectionBounds returns the byte offsets of one model-section marker
// pair, and fails closed when a marker is missing or duplicated.
func modelSectionBounds(content, open, close string) (int, int, error) {
	if strings.Count(content, open) != 1 {
		return 0, 0, fmt.Errorf("clineagents: generic orchestrator marker %q must appear exactly once", open)
	}
	if strings.Count(content, close) != 1 {
		return 0, 0, fmt.Errorf("clineagents: generic orchestrator marker %q must appear exactly once", close)
	}
	start := strings.Index(content, open)
	end := strings.Index(content, close)
	if end < start+len(open) {
		return 0, 0, fmt.Errorf("clineagents: generic orchestrator markers %q and %q are out of order", open, close)
	}
	return start, end + len(close), nil
}
