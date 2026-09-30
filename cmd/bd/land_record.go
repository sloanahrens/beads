package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/ui"
	"github.com/steveyegge/beads/issueops"
)

// Metadata keys and the label the landing record owns (be-u20). They live in
// the issue's metadata document, so `bd show --json` returns them as
// metadata.landing and metadata.landing_rejection with no schema change.
const (
	landingMetadataKey          = "landing"
	landingRejectionMetadataKey = "landing_rejection"
	landingReworkLabel          = "rework"
)

// landingRecord is what a successful landing writes to metadata.landing.
type landingRecord struct {
	PatchID      string   `json:"patch_id"`
	LandedCommit string   `json:"landed_commit"`
	GateResult   string   `json:"gate_result"`
	OmVerdict    *string  `json:"om_verdict"`
	OmScore      *float64 `json:"om_score"`
	Route        string   `json:"route"`
	RecordedAt   string   `json:"recorded_at"`
	RecordedBy   string   `json:"recorded_by"`
}

// landingRejection is what a rejection writes to metadata.landing_rejection.
type landingRejection struct {
	Kind             string          `json:"kind"`
	GateTail         string          `json:"gate_tail"`
	OmFindings       json.RawMessage `json:"om_findings"`
	ConflictingFiles []string        `json:"conflicting_files"`
	RecordedAt       string          `json:"recorded_at"`
	RecordedBy       string          `json:"recorded_by"`
}

var landRecordCmd = &cobra.Command{
	Use:     "land-record <id>",
	GroupID: "issues",
	Short:   "Record a landing or a landing rejection on a work bead",
	Long: `Record the outcome of landing a work bead's branch, in one write.

A landing writes metadata.landing = {patch_id, landed_commit, gate_result,
om_verdict, om_score, route, recorded_at, recorded_by} and removes the
"rework" label:

  bd land-record <id> --patch-id P --landed-commit C --gate-result pass \
      --om-verdict approve --om-score 8.5 --route merge-queue

A rejection writes metadata.landing_rejection = {kind, gate_tail,
om_findings, conflicting_files, recorded_at, recorded_by} and adds the
"rework" label:

  bd land-record <id> --reject --kind gate_failed --gate-tail-file tail.txt \
      --om-findings '[...]' --conflicting-file a.go --conflicting-file b.go

The metadata edit and the label edit land in one transaction. Read the record
back with bd show <id> --json (.metadata.landing, .metadata.landing_rejection).
Each call replaces the previous record of the same kind.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		CheckReadonly("land-record")
		evt := metrics.NewCommandEvent("land-record")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		patch, summary, err := landRecordPatch(cmd)
		if err != nil {
			return err
		}
		if store == nil {
			if err := ensureStoreActive(); err != nil {
				return handleClassifiedRespectJSON(err)
			}
		}

		ctx := rootCtx
		result, err := resolveAndGetIssueForMutation(ctx, store, args[0])
		if err != nil {
			if result != nil {
				result.Close()
			}
			return handleClassifiedRespectJSON(err)
		}
		defer result.Close()
		if result.Issue == nil {
			return failKind(kindNotFound, "issue %s not found", args[0])
		}

		ops, err := writeOps(result.Store)
		if err != nil {
			return handleClassifiedRespectJSON(err)
		}
		opsCtx, err := issueOpsContext(ctx)
		if err != nil {
			return handleClassifiedRespectJSON(err)
		}
		updated, err := ops.Update(opsCtx, issueops.UpdateRequest{
			Actor:   actor,
			IssueID: result.ResolvedID,
			Patch:   patch,
		})
		if err != nil {
			return handleClassifiedRespectJSON(fmt.Errorf("recording %s on %s: %w", summary, result.ResolvedID, err))
		}
		commandDidWrite.Store(true)

		if jsonOutput {
			issue := updated.Issue
			if issue != nil {
				issue.Dependencies = nil
			}
			return outputJSON(issue)
		}
		fmt.Printf("%s Recorded %s on %s\n", ui.RenderPass("✓"), summary, result.ResolvedID)
		return nil
	},
}

// landRecordPatch validates the flags and builds the one update that carries
// the record and its label edit. Every refusal is invalid_args.
func landRecordPatch(cmd *cobra.Command) (issueops.IssuePatch, string, error) {
	flags := cmd.Flags()
	reject, _ := flags.GetBool("reject")
	landingFlags := []string{"patch-id", "landed-commit", "gate-result", "om-verdict", "om-score", "route"}
	rejectionFlags := []string{"kind", "gate-tail", "gate-tail-file", "om-findings", "conflicting-file"}
	now := time.Now().UTC().Format(time.RFC3339)

	if !reject {
		for _, f := range rejectionFlags {
			if flags.Changed(f) {
				return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "--%s belongs to a rejection; add --reject", f)
			}
		}
		var missing []string
		for _, f := range []string{"patch-id", "landed-commit", "gate-result", "route"} {
			if v, _ := flags.GetString(f); strings.TrimSpace(v) == "" {
				missing = append(missing, "--"+f)
			}
		}
		if len(missing) > 0 {
			return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "a landing record needs %s", strings.Join(missing, ", "))
		}
		rec := landingRecord{RecordedAt: now, RecordedBy: actor}
		rec.PatchID, _ = flags.GetString("patch-id")
		rec.LandedCommit, _ = flags.GetString("landed-commit")
		rec.GateResult, _ = flags.GetString("gate-result")
		rec.Route, _ = flags.GetString("route")
		if flags.Changed("om-verdict") {
			v, _ := flags.GetString("om-verdict")
			rec.OmVerdict = &v
		}
		if flags.Changed("om-score") {
			v, _ := flags.GetFloat64("om-score")
			rec.OmScore = &v
		}
		raw, err := json.Marshal(rec)
		if err != nil {
			return issueops.IssuePatch{}, "", failKind(kindInternal, "encode landing record: %v", err)
		}
		return issueops.IssuePatch{
			Metadata: issueops.MetadataPatch{Set: map[string]json.RawMessage{landingMetadataKey: raw}},
			Labels:   issueops.LabelPatch{Remove: []string{landingReworkLabel}},
		}, "landing", nil
	}

	for _, f := range landingFlags {
		if flags.Changed(f) {
			return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "--%s belongs to a landing; drop --reject", f)
		}
	}
	rej := landingRejection{RecordedAt: now, RecordedBy: actor, ConflictingFiles: []string{}}
	rej.Kind, _ = flags.GetString("kind")
	if strings.TrimSpace(rej.Kind) == "" {
		return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "a rejection needs --kind")
	}
	if flags.Changed("gate-tail") && flags.Changed("gate-tail-file") {
		return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "--gate-tail and --gate-tail-file are mutually exclusive")
	}
	rej.GateTail, _ = flags.GetString("gate-tail")
	if path, _ := flags.GetString("gate-tail-file"); path != "" {
		tail, err := readLandRecordFile(path)
		if err != nil {
			return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "--gate-tail-file: %v", err)
		}
		rej.GateTail = tail
	}
	rej.OmFindings = json.RawMessage("null")
	if findings, _ := flags.GetString("om-findings"); findings != "" {
		if !json.Valid([]byte(findings)) {
			return issueops.IssuePatch{}, "", failKind(kindInvalidArgs, "--om-findings must be valid JSON")
		}
		rej.OmFindings = json.RawMessage(findings)
	}
	if files, _ := flags.GetStringArray("conflicting-file"); len(files) > 0 {
		rej.ConflictingFiles = files
	}
	raw, err := json.Marshal(rej)
	if err != nil {
		return issueops.IssuePatch{}, "", failKind(kindInternal, "encode landing rejection: %v", err)
	}
	return issueops.IssuePatch{
		Metadata: issueops.MetadataPatch{Set: map[string]json.RawMessage{landingRejectionMetadataKey: raw}},
		Labels:   issueops.LabelPatch{Add: []string{landingReworkLabel}},
	}, "landing rejection", nil
}

// readLandRecordFile reads a flag's file argument; "-" reads stdin.
func readLandRecordFile(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path) // #nosec G304 -- caller-named input file
	return string(b), err
}

func init() {
	f := landRecordCmd.Flags()
	f.String("patch-id", "", "Landing: git patch-id of the landed change")
	f.String("landed-commit", "", "Landing: commit that landed on the target branch")
	f.String("gate-result", "", "Landing: quality-gate result (e.g. pass)")
	f.String("om-verdict", "", "Landing: om review verdict")
	f.Float64("om-score", 0, "Landing: om review score")
	f.String("route", "", "Landing: how the change landed (e.g. merge-queue, direct)")
	f.Bool("reject", false, "Record a landing rejection instead of a landing (adds the rework label)")
	f.String("kind", "", "Rejection: what failed (e.g. gate_failed, om_rejected, conflict)")
	f.String("gate-tail", "", "Rejection: tail of the failing gate output")
	f.String("gate-tail-file", "", "Rejection: read the gate tail from a file (- for stdin)")
	f.String("om-findings", "", "Rejection: om findings as a JSON value")
	f.StringArray("conflicting-file", nil, "Rejection: a file that conflicted (repeatable)")
	rootCmd.AddCommand(landRecordCmd)
}
