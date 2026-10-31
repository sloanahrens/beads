package doctor

import (
	"strings"
	"testing"
)

func TestEnrichFreshClone_UsesBootstrapFirstGuidance(t *testing.T) {
	dc := DoctorCheck{Name: "Fresh Clone", Message: "database not found on configured server"}
	enrichment := enrichFreshClone(dc)

	if enrichment.severity != "blocking" {
		t.Fatalf("expected blocking severity, got %q", enrichment.severity)
	}
	if !strings.Contains(enrichment.explanation, "bd bootstrap") {
		t.Fatalf("expected bootstrap guidance, got: %s", enrichment.explanation)
	}
	if len(enrichment.commands) != 1 || enrichment.commands[0] != "bd bootstrap" {
		t.Fatalf("expected bootstrap-first commands, got %#v", enrichment.commands)
	}
	if strings.Contains(enrichment.explanation, "run bd init") {
		t.Fatalf("did not expect init-first guidance, got: %s", enrichment.explanation)
	}
}

func TestEnrichFreshClone_WithSyncRemoteMentionsBootstrapAndFallback(t *testing.T) {
	dc := DoctorCheck{Name: "Fresh Clone", Message: "sync.remote is configured but database not found"}
	enrichment := enrichFreshClone(dc)

	if !strings.Contains(enrichment.explanation, "bd bootstrap") {
		t.Fatalf("expected bootstrap guidance, got: %s", enrichment.explanation)
	}
	if len(enrichment.commands) != 2 || enrichment.commands[0] != "bd bootstrap" {
		t.Fatalf("expected bootstrap-first command list, got %#v", enrichment.commands)
	}
	if !strings.Contains(enrichment.commands[1], "sync.remote") {
		t.Fatalf("expected sync.remote fallback command, got %#v", enrichment.commands)
	}
}

// TestPruneEnrichmentsRecommendRealCommand pins the agent-facing pruning
// advice to a command that exists: these enrichments used to suggest
// 'bd cleanup', which is not a root-level command in this build (cleanup
// lives under 'bd admin cleanup'). 'bd prune --older-than 90d' previews by
// default, matching the "optional and destructive" framing.
func TestPruneEnrichmentsRecommendRealCommand(t *testing.T) {
	enrichments := map[string]agentEnrichment{
		"Large Database":      enrichLargeDatabase(DoctorCheck{Message: "6000 closed issues (threshold: 5000)"}),
		"Stale Closed Issues": enrichStaleClosedIssues(DoctorCheck{Message: "5000 stale closed issues"}),
	}
	for name, enrichment := range enrichments {
		if len(enrichment.commands) == 0 {
			t.Errorf("%s enrichment has no commands", name)
			continue
		}
		for _, cmd := range enrichment.commands {
			if strings.Contains(cmd, "bd cleanup") {
				t.Errorf("%s enrichment recommends nonexistent 'bd cleanup': %q", name, cmd)
			}
		}
		if !strings.HasPrefix(enrichment.commands[0], "bd prune") {
			t.Errorf("%s enrichment should recommend 'bd prune', got %#v", name, enrichment.commands)
		}
	}
}

// TestCursorSettingsEnrichmentDoesNotSuggestNonRepairCommand pins the malformed
// hooks.json advice to its diagnostic: `bd setup cursor` was deleted, and
// `bd prime --hook-json` prints context rather than repairing the file, so the
// enrichment must not offer either as a repair command.
func TestCursorSettingsEnrichmentDoesNotSuggestNonRepairCommand(t *testing.T) {
	enrichment := enrichCursorSettings(DoctorCheck{
		Name:    "Cursor Settings Health",
		Message: "1 malformed hooks file(s)",
	})

	if !strings.Contains(enrichment.explanation, "by hand") {
		t.Errorf("Expected explanation to say the JSON is fixed by hand, got %q", enrichment.explanation)
	}
	for _, cmd := range enrichment.commands {
		if strings.Contains(cmd, "bd setup") || strings.Contains(cmd, "bd prime") {
			t.Errorf("Cursor settings enrichment claims a command that does not repair the hooks file: %q", cmd)
		}
	}
}

// TestCursorIntegrationEnrichmentDescribesManualHookStep pins the Cursor
// integration advice to the manual step: no surviving bd command writes
// .cursor/hooks.json, so offering 'bd prime --hook-json' as the fix would
// claim an install that command cannot perform. The explanation must name the
// file the check reads, the hooks it expects, and the command each hook runs.
func TestCursorIntegrationEnrichmentDescribesManualHookStep(t *testing.T) {
	ad := EnrichForAgent(DoctorCheck{
		Name:    "Cursor Integration",
		Message: "hooks not configured",
	})

	if len(ad.Commands) != 0 {
		t.Errorf("Cursor integration enrichment must not print a command that cannot write the hooks file, got %#v", ad.Commands)
	}
	for _, want := range []string{".cursor/hooks.json", "~/.cursor/hooks.json", "sessionStart", "preCompact", "postToolUse", "bd prime --hook-json", "by hand"} {
		if !strings.Contains(ad.Explanation, want) {
			t.Errorf("Expected Cursor integration explanation to mention %q, got %q", want, ad.Explanation)
		}
	}
	if strings.Contains(ad.Explanation, "bd setup") {
		t.Errorf("Cursor integration explanation suggests deleted 'bd setup': %q", ad.Explanation)
	}
}
