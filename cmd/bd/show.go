package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/debug"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
	"github.com/steveyegge/beads/issueops"
)

// showNotFoundHint explains that an unresolved issue ID is not proof the ID
// never existed: bd delete and wisp purge both remove a row (and its
// events) from the live tables with no trace left behind, so a deleted or
// purged ID looks identical to one that was never created. bd history can
// sometimes tell the two apart via Dolt's commit history, but that scan is
// real, sometimes-slow work (and wisps are never committed to history at
// all), so it's offered as a pointer rather than run automatically here
// (ga-m6inyb).
func showNotFoundHint(id string) string {
	return fmt.Sprintf("this ID may have never existed, or may reference a deleted/purged record with no trace left in the live database — try 'bd history %s'", id)
}

var showCmd = &cobra.Command{
	Use:           "show [id...] [--id=<id>...] [--current]",
	Aliases:       []string{"view"},
	GroupID:       "issues",
	Short:         "Show issue details",
	Args:          cobra.ArbitraryArgs, // Allow zero positional args when --id is used
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		showThread, _ := cmd.Flags().GetBool("thread")
		shortMode, _ := cmd.Flags().GetBool("short")
		longMode, _ := cmd.Flags().GetBool("long")
		showRefs, _ := cmd.Flags().GetBool("refs")
		showChildren, _ := cmd.Flags().GetBool("children")
		asOfRef, _ := cmd.Flags().GetString("as-of")
		idFlags, _ := cmd.Flags().GetStringArray("id")
		localTime, _ := cmd.Flags().GetBool("local-time")
		watchMode, _ := cmd.Flags().GetBool("watch")
		currentMode, _ := cmd.Flags().GetBool("current")
		includeDepends, _ := cmd.Flags().GetBool("include-dependents")
		includeComments, _ := cmd.Flags().GetBool("include-comments")
		briefDeps, _ := cmd.Flags().GetBool("brief-deps")
		ctx := rootCtx

		// Helper to format timestamp based on --local-time flag
		formatTime := func(t time.Time) string {
			if localTime {
				t = t.Local()
			}
			return t.Format("2006-01-02 15:04")
		}

		// Merge --id flag values with positional args
		// This allows IDs that look like flags (e.g., --xyz or gt--abc) to be passed safely
		args = append(args, idFlags...)

		if currentMode {
			if len(args) > 0 {
				return HandleErrorRespectJSON("--current cannot be combined with explicit issue IDs")
			}
			currentID := resolveCurrentIssueID(ctx)
			if currentID == "" {
				return HandleErrorRespectJSON("no current issue found (no in-progress, hooked, or recently touched issues)")
			}
			args = []string{currentID}
		}

		if len(args) == 0 {
			return HandleErrorRespectJSON("at least one issue ID is required (use positional args, --id flag, or --current)")
		}

		if asOfRef != "" {
			return showIssueAsOf(ctx, args, asOfRef, shortMode)
		}

		if watchMode {
			if err := machineRefusesInteractive("bd show --watch"); err != nil {
				return err
			}
			if err := ensureDirectMode("watch mode requires direct database access"); err != nil {
				return HandleErrorRespectJSON("%v", err)
			}
			if len(args) != 1 {
				return HandleErrorRespectJSON("watch mode requires exactly one issue ID")
			}
			watchIssue(ctx, args[0])
			return nil
		}

		if showThread {
			if len(args) > 0 {
				result, err := resolveAndGetIssueWithRouting(ctx, store, args[0])
				if result != nil {
					defer result.Close()
				}
				if err == nil && result != nil && result.ResolvedID != "" {
					return showMessageThread(ctx, result.ResolvedID, jsonOutput)
				}
			}
		}

		if showRefs {
			return showIssueRefs(ctx, args, jsonOutput)
		}

		if showChildren {
			return showIssueChildren(ctx, args, jsonOutput, shortMode)
		}

		// The JSON detail view for a list of ids is answered from one store
		// read per database the ids live in, not one read per id (be-2wkz).
		if jsonOutput && !shortMode {
			return showIssuesJSON(ctx, args, includeDepends, includeComments, briefDeps, currentMode)
		}

		// Direct mode - use routed resolution for cross-repo lookups
		foundCount := 0
		// Ids that could not be shown, with why. Machine mode reports them
		// in error.ids (not_found, route_unreachable, ...).
		var showFailures []idOutcome
		for idx, id := range args {
			// Resolve and get issue with routing (e.g., gt-xyz routes to another rig)
			result, err := resolveAndGetIssueWithRouting(ctx, store, id)
			if err != nil {
				if result != nil {
					result.Close()
				}
				showFailures = append(showFailures, reportShowFailure(id, err))
				continue
			}
			if result == nil || result.Issue == nil {
				if result != nil {
					result.Close()
				}
				showFailures = append(showFailures, reportShowMissing(id))
				continue
			}
			issue := result.Issue
			issueStore := result.Store // Use the store that contains this issue
			// Note: result.Close() called at end of loop iteration
			foundCount++

			if shortMode {
				fmt.Println(formatShortIssue(issue))
				result.Close()
				continue
			}

			if idx > 0 {
				fmt.Println("\n" + ui.RenderMuted(strings.Repeat("─", 60)))
				fmt.Printf("\n%s\n", formatIssueHeader(issue))
			} else {
				fmt.Printf("%s\n", formatIssueHeader(issue))
			}

			// Metadata: Owner · Type | Created · Updated
			fmt.Println(formatIssueMetadata(issue))

			// Content sections — always show DESCRIPTION header so the user
			// can distinguish "empty" from "hidden" (GH#3336).
			if issue.Description != "" {
				fmt.Print(formatShowSection("DESCRIPTION", issue.Description))
			} else {
				fmt.Printf("\n%s\n  %s\n", ui.RenderBold("DESCRIPTION"), ui.RenderMuted("(none)"))
			}
			if issue.Design != "" {
				fmt.Print(formatShowSection("DESIGN", issue.Design))
			}
			if issue.Notes != "" {
				fmt.Print(formatShowSection("NOTES", issue.Notes))
			}
			if issue.AcceptanceCriteria != "" {
				fmt.Print(formatShowSection("ACCEPTANCE CRITERIA", issue.AcceptanceCriteria))
			}

			// Show labels
			labels, _ := issueStore.GetLabels(ctx, issue.ID) // Best effort: show issue even if label fetch fails
			if len(labels) > 0 {
				fmt.Printf("\n%s %s\n", ui.RenderBold("LABELS:"), strings.Join(labels, ", "))
			}

			// Show custom metadata (GH#1406)
			if metaStr := formatIssueCustomMetadata(issue); metaStr != "" {
				fmt.Printf("\n%s\n", metaStr)
			}

			// Collect related issues from both directions for deduplication
			// (relates-to is bidirectional, so we merge and show once)
			relatedSeen := make(map[string]*types.IssueWithDependencyMetadata)

			// Show dependencies - grouped by dependency type for clarity
			depsWithMeta, _ := issueStore.GetDependenciesWithMetadata(ctx, issue.ID) // Best effort: show issue even if deps unavailable
			for _, sec := range groupDepSections(depsWithMeta, true, relatedSeen) {
				printDepSection(sec)
			}

			// Show dependents - grouped by dependency type for clarity
			dependentsWithMeta, _ := issueStore.GetDependentsWithMetadata(ctx, issue.ID) // Best effort: show issue even if dependents unavailable
			for _, sec := range groupDepSections(dependentsWithMeta, false, relatedSeen) {
				printDepSection(sec)
				if sec.Type == types.DepParentChild && issue.IssueType == types.TypeEpic {
					printEpicChildProgress(sec.Deps)
				}
			}

			printRelatedSection(relatedSeen)

			// Show comments
			comments, _ := issueStore.GetIssueComments(ctx, issue.ID) // Best effort: show issue even if comments unavailable
			if len(comments) > 0 {
				fmt.Printf("\n%s\n", ui.RenderBold("COMMENTS"))
				for _, comment := range comments {
					fmt.Printf("  %s %s\n", ui.RenderMuted(formatTime(comment.CreatedAt)), comment.Author)
					// TrimRight drops trailing newlines in the stored text, preventing extra blank lines
					for _, line := range strings.Split(strings.TrimRight(comment.Text, "\n"), "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
			}

			// Long mode: show all extended fields
			if longMode {
				fmt.Print(formatIssueLongExtras(issue, formatTime))
			}

			fmt.Println()
			result.Close() // Close routed storage after each iteration
		}

		if jsonOutput {
			// Only --json --short reaches this point: --short printed its
			// lines above and the detail views are showIssuesJSON's, so there
			// is nothing to marshal. This is the answer that combination has
			// always given, kept as it was.
			if machineModeActive() {
				return batchError(0, showFailures)
			}
			return HandleErrorWithHintRespectJSON("no issues found matching the provided IDs",
				"some IDs may reference deleted/purged records with no trace left in the live database — try 'bd history <id>' to check")
		} else if foundCount > 0 {
			maybeShowTip(store, stdoutIsTerminal())
		} else {
			if len(args) > 0 {
				SetLastTouchedID(args[0])
			}
			return SilentExit()
		}

		if len(args) > 0 {
			SetLastTouchedID(args[0])
		}
		if machineModeActive() {
			// Some ids shown, some not: partial, with the misses named.
			return batchError(foundCount, showFailures)
		}
		return nil
	},
}

func init() {
	showCmd.Flags().Bool("thread", false, "Show full conversation thread (for messages)")
	showCmd.Flags().Bool("short", false, "Show compact one-line output per issue")
	showCmd.Flags().Bool("long", false, "Show all available fields (extended metadata, agent identity, gate fields, etc.)")
	showCmd.Flags().Bool("refs", false, "Show issues that reference this issue (reverse lookup)")
	showCmd.Flags().Bool("children", false, "Show only the children of this issue")
	showCmd.Flags().String("as-of", "", "Show issue as it existed at a specific commit hash or branch (requires Dolt)")
	showCmd.Flags().StringArray("id", nil, "Issue ID (use for IDs that look like flags, e.g., --id=gt--xyz)")
	showCmd.Flags().Bool("local-time", false, "Show timestamps in local time instead of UTC")
	showCmd.Flags().BoolP("watch", "w", false, "Watch for changes and auto-refresh display")
	showCmd.Flags().Bool("current", false, "Show the currently active issue (in-progress, hooked, or last touched)")
	showCmd.Flags().Bool("include-dependents", false, "Stream full dependent issues in JSON output (--json only; may be slow on hub beads)")
	showCmd.Flags().Bool("include-comments", false, "Stream full comment bodies in JSON output (--json only; may be slow on issues with many comments)")
	showCmd.Flags().Bool("brief-deps", false, "Reduce each dependency to its identity fields in JSON output (--json only; drops description, design, notes and acceptance criteria)")
	showCmd.ValidArgsFunction = issueIDCompletion
	rootCmd.AddCommand(showCmd)
}

// showGetManyRequest carries the show flags onto the reader contract's batch
// form. Extracted so the hop is reachable from a test: a dropped flag here is
// invisible to every other test, and it would change the answer for every id in
// the list at once.
func showGetManyRequest(ids []string, includeDependents, includeComments, briefDeps bool) issueops.GetManyRequest {
	return issueops.GetManyRequest{
		IDs:               ids,
		IncludeDependents: includeDependents,
		IncludeComments:   includeComments,
		BriefDeps:         briefDeps,
	}
}

// showBatchGroup is one store and the ids resolution landed in it.
type showBatchGroup struct {
	store storage.DoltStorage
	ids   []string
}

// appendToShowBatchGroup adds id to the group for its store, appending a group
// on first sight.
//
// Grouping by store is what turns "one detail read per id" into "one detail
// read per database": every id resolved against the local store shares it, and
// a prefix- or auto-routed lookup answers with a store of its own, so it gets a
// group of its own — slower, but never wrong.
func appendToShowBatchGroup(groups []showBatchGroup, store storage.DoltStorage, id string) []showBatchGroup {
	for i := range groups {
		if sameStoreHandle(groups[i].store, store) {
			groups[i].ids = append(groups[i].ids, id)
			return groups
		}
	}
	return append(groups, showBatchGroup{store: store, ids: []string{id}})
}

// sameStoreHandle reports whether two handles name the same store. Interface
// equality panics when a dynamic type is not comparable, so the guard keeps a
// value-typed backend from turning `bd show` into a panic; every real backend
// is a pointer and compares by identity.
func sameStoreHandle(a, b storage.DoltStorage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	t := reflect.TypeOf(a)
	if t != reflect.TypeOf(b) || !t.Comparable() {
		return false
	}
	return a == b
}

// showResolution is one requested id's answer from resolution, before the
// detail read: either a canonical id and the store that holds it, or the
// per-id failure `bd show` reports for it.
type showResolution struct {
	requested string
	canonical string
	store     storage.DoltStorage
	failure   idOutcome
	failed    bool
}

// exactLocalIDs answers which of ids the local store holds EXACTLY, in one
// query set. An id it leaves out is not decided by this read — it still goes
// through resolveAndGetIssueWithRouting — so a miss here can only cost a
// slower lookup, never change an answer.
//
// A store that cannot answer (no ExactIDStore capability, or a failed read)
// yields nil, which sends every id down the per-id resolver: that is the same
// read this fast path stands in for, and it reports a store failure on the id
// that hits it instead of dropping the answer.
func exactLocalIDs(ctx context.Context, s storage.DoltStorage, ids []string) map[string]struct{} {
	if len(ids) == 0 || s == nil {
		return nil
	}
	resolver, ok := s.(storage.ExactIDStore)
	if !ok {
		return nil
	}
	set, err := resolver.ExactIssueIDs(ctx, ids)
	if err != nil {
		// Not fatal, and not silent: the per-id resolver is the read this
		// batch replaces, and it reports the store failure on the id that hits
		// it, so the command still answers honestly — just slower. The debug
		// line leaves a trace for a show that got slow for no visible reason.
		debug.Logf("bd show: exact id lookup failed, resolving per id: %v", err)
		return nil
	}
	return set
}

// showExactIDSet is exactLocalIDs as showIssuesJSON asks it: a --current pick
// is not run through it, because that id came out of a store read of its own
// and keeps resolving exactly as it did before this fast path existed.
func showExactIDSet(ctx context.Context, s storage.DoltStorage, ids []string, currentPick bool) map[string]struct{} {
	if currentPick {
		return nil
	}
	return exactLocalIDs(ctx, s, ids)
}

// resolveShowIDs resolves every requested id for `bd show --json`, in the
// order the caller named them.
//
// exactLocal is the set exactLocalIDs answered with: an id in it names a row
// the local store holds under that very id, and routing is only consulted
// after the local store answers not-found (resolveAndGetIssueWithRoutingAccess
// tries the local store first, unconditionally), so such an id would have come
// back from the resolver as itself, out of the local store. Taking it as its
// own canonical id therefore skips only the lookup, not the answer.
//
// EVERY OTHER ID takes resolveAndGetIssueWithRouting exactly as before: fuzzy
// and partial ids, a prefix routed to another rig, an auto-routed contributor
// project, and a --current pick all keep the affordances and the failure
// outcomes they have today. Prefix routing is not re-implemented here; the
// fast path only covers the case where there is no routing decision to make.
//
// The returned slice is parallel to ids, and the second return holds the
// routed results opened on the way, which the caller must Close.
func resolveShowIDs(ctx context.Context, localStore storage.DoltStorage, ids []string, exactLocal map[string]struct{}) ([]showResolution, []*RoutedResult) {
	resolutions := make([]showResolution, 0, len(ids))
	var open []*RoutedResult
	for _, id := range ids {
		if _, ok := exactLocal[id]; ok {
			resolutions = append(resolutions, showResolution{requested: id, canonical: id, store: localStore})
			continue
		}
		result, err := resolveAndGetIssueWithRouting(ctx, localStore, id)
		if err != nil {
			if result != nil {
				result.Close()
			}
			resolutions = append(resolutions, showResolution{
				requested: id,
				failure:   reportShowFailure(id, err),
				failed:    true,
			})
			continue
		}
		if result == nil || result.Issue == nil {
			if result != nil {
				result.Close()
			}
			resolutions = append(resolutions, showResolution{
				requested: id,
				failure:   reportShowMissing(id),
				failed:    true,
			})
			continue
		}
		open = append(open, result)
		resolutions = append(resolutions, showResolution{
			requested: id,
			canonical: result.Issue.ID,
			store:     result.Store,
		})
	}
	return resolutions, open
}

// showIssuesJSON answers `bd show --json` for a list of ids.
//
// RESOLUTION STAYS IN THIS PACKAGE: fuzzy ids, --current's pick and
// prefix/cross-repo routing are interactive affordances that can answer with a
// different issue than the caller named, which has no place on a contract an
// unattended client also calls. What this function changes is what resolution
// COSTS: ids the local store holds exactly are settled by one batched
// existence lookup for the whole list (exactLocalIDs) instead of one store
// call each, and only the ids that are not exactly local go through the
// per-id resolver (be-mfu5). Every id is then answered by one
// issueops.Reader.GetMany per store they resolved into, so a list of a
// thousand ids costs one query set per chunk instead of ten statements an id
// (be-2wkz).
//
// The output keeps the caller's order and the per-id failure outcome: an id
// that does not resolve (or that the batch no longer holds) is reported on
// stderr and named again in error.ids under machine mode, and the ids that did
// answer are still printed.
func showIssuesJSON(ctx context.Context, args []string, includeDependents, includeComments, briefDeps, currentPick bool) error {
	foundCount := 0
	var showFailures []idOutcome

	// resolved holds the canonical ids in the order the caller named them, so
	// the batch answer can be put back into request order however the reads
	// were grouped.
	resolved := make([]string, 0, len(args))
	var groups []showBatchGroup

	exactLocal := showExactIDSet(ctx, store, args, currentPick)
	resolutions, open := resolveShowIDs(ctx, store, args, exactLocal)
	// A routed lookup hands back a store it opened for that id, and only
	// Close releases it — so the batch read below must happen while the
	// results are still open. The local store's Close is a no-op, which is
	// what keeps this from holding anything for the ordinary case.
	defer func() {
		for _, result := range open {
			result.Close()
		}
	}()
	for _, r := range resolutions {
		if r.failed {
			showFailures = append(showFailures, r.failure)
			continue
		}
		foundCount++
		resolved = append(resolved, r.canonical)
		groups = appendToShowBatchGroup(groups, r.store, r.canonical)
	}

	detailsByID := make(map[string]*issueops.IssueDetails, len(resolved))
	for _, group := range groups {
		rd, rerr := group.store.IssueReader()
		if rerr != nil {
			return HandleErrorRespectJSON("%v", rerr)
		}
		answer, rerr := rd.GetMany(ctx, showGetManyRequest(group.ids, includeDependents, includeComments, briefDeps))
		if rerr != nil {
			return HandleErrorRespectJSON("%v", rerr)
		}
		for _, details := range answer.Details {
			detailsByID[details.ID] = details
		}
	}

	allDetails := make([]interface{}, 0, len(resolved))
	for _, id := range resolved {
		details, ok := detailsByID[id]
		if !ok {
			// Resolution found this row and the batch did not: it went away
			// between the two reads. It keeps the per-id miss outcome rather
			// than failing the ids that did answer.
			showFailures = append(showFailures, reportShowMissing(id))
			continue
		}
		allDetails = append(allDetails, details)
	}

	if len(allDetails) > 0 {
		if jerr := outputJSON(allDetails); jerr != nil {
			return jerr
		}
	} else {
		if machineModeActive() {
			return batchError(0, showFailures)
		}
		return HandleErrorWithHintRespectJSON("no issues found matching the provided IDs",
			"some IDs may reference deleted/purged records with no trace left in the live database — try 'bd history <id>' to check")
	}

	if len(args) > 0 {
		SetLastTouchedID(args[0])
	}
	if machineModeActive() {
		// Some ids shown, some not: partial, with the misses named.
		return batchError(foundCount, showFailures)
	}
	return nil
}

// reportShowFailure prints one id's resolution failure and returns its
// machine-readable outcome. Both `bd show` routes report this way so a miss and
// a failure cannot read differently depending on which one answered.
func reportShowFailure(id string, err error) idOutcome {
	var unreachable *routeUnreachableError
	switch {
	case errors.As(err, &unreachable):
		// UNKNOWN, not a definite negative: say which database could not be
		// asked (B1-05).
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	case isNotFoundErr(err):
		fmt.Fprintf(os.Stderr, "Issue %s not found\n", id)
		fmt.Fprintf(os.Stderr, "Hint: %s\n", showNotFoundHint(id))
	default:
		fmt.Fprintf(os.Stderr, "Error fetching %s: %v\n", id, err)
	}
	return idOutcome{ID: id, Kind: errorKindOf(err), Message: err.Error()}
}

// reportShowMissing is a resolver that found NO row and no error — the "not
// found" outcome in its own right.
func reportShowMissing(id string) idOutcome {
	fmt.Fprintf(os.Stderr, "Issue %s not found\n", id)
	fmt.Fprintf(os.Stderr, "Hint: %s\n", showNotFoundHint(id))
	return idOutcome{ID: id, Kind: kindNotFound, Message: fmt.Sprintf("issue %s not found", id)}
}
