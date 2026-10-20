// Package utils provides utility functions for issue ID parsing and resolution.
package utils

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/steveyegge/beads/internal/types"
)

// ErrAmbiguousID is the sentinel wrapped into the error ResolvePartialID
// returns when a partial ID matches more than one issue. Callers use
// errors.Is(err, ErrAmbiguousID) to distinguish "ambiguous" from
// "not found" and surface the candidate list instead of a generic failure.
var ErrAmbiguousID = errors.New("ambiguous issue ID")

type PartialIDResolverStore interface {
	SearchIssues(ctx context.Context, query string, filter types.IssueFilter) ([]*types.Issue, error)
	SearchIssueIDs(ctx context.Context, query string, filter types.IssueFilter) ([]string, error)
	GetConfig(ctx context.Context, key string) (string, error)
}

// parseIssueID ensures an issue ID has the configured prefix.
// If the input already has the prefix (e.g., "bd-a3f8e9"), returns it as-is.
// If the input lacks the prefix (e.g., "a3f8e9"), adds the configured prefix.
// Works with hierarchical IDs too: "a3f8e9.1.2" → "bd-a3f8e9.1.2"
func parseIssueID(input string, prefix string) string {
	if prefix == "" {
		prefix = "bd-"
	}

	if strings.HasPrefix(input, prefix) {
		return input
	}

	return prefix + input
}

// ResolvePartialID resolves a potentially partial issue ID to a full ID.
// Supports:
// - Full IDs: "bd-a3f8e9" or "a3f8e9" → "bd-a3f8e9"
// - Without hyphen: "bda3f8e9" or "wya3f8e9" → "bd-a3f8e9"
// - Partial IDs: "a3f8" → "bd-a3f8e9" (if unique match)
// - Hierarchical: "a3f8e9.1" → "bd-a3f8e9.1"
//
// Returns an error if:
// - No issue found matching the ID
// - Multiple issues match (ambiguous prefix)
func ResolvePartialID(ctx context.Context, store PartialIDResolverStore, input string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("cannot resolve issue ID %q: storage is nil", input)
	}

	// Fast path: Use SearchIssues with exact ID filter (GH#942).
	// This uses the same query path as "bd list --id", ensuring consistency.
	// Previously we used GetIssue which could fail in cases where SearchIssues
	// with filter.IDs succeeded, likely due to subtle query differences.
	exactFilter := types.IssueFilter{IDs: []string{input}}
	if issues, err := store.SearchIssues(ctx, "", exactFilter); err == nil && len(issues) > 0 {
		return issues[0].ID, nil
	}

	// Get the configured prefix
	prefix, err := store.GetConfig(ctx, "issue_prefix")
	if err != nil || prefix == "" {
		prefix = "bd"
	}

	// Ensure prefix has hyphen for ID format
	prefixWithHyphen := prefix
	if !strings.HasSuffix(prefix, "-") {
		prefixWithHyphen = prefix + "-"
	}

	// Build known prefixes from config for deterministic multi-hyphen prefix handling.
	// This avoids relying solely on looksLikePrefixedID heuristics when the repo
	// explicitly declares which prefixes are valid.
	knownPrefixes := []string{strings.TrimSuffix(prefix, "-")}
	if allowed, aErr := store.GetConfig(ctx, "allowed_prefixes"); aErr == nil && allowed != "" {
		for _, p := range strings.Split(allowed, ",") {
			p = strings.TrimSpace(p)
			p = strings.TrimSuffix(p, "-")
			if p != "" {
				knownPrefixes = append(knownPrefixes, p)
			}
		}
	}

	// Normalize input:
	// 1. If it has the full prefix with hyphen (bd-a3f8e9), use as-is
	// 2. If it starts with any known/allowed prefix, use as-is (config-aware cross-prefix)
	// 3. If it has ANY prefix (heuristic fallback), use as-is for cross-prefix lookup
	// 4. Otherwise, add prefix with hyphen (handles both bare hashes and prefix-without-hyphen cases)

	var normalizedID string

	if strings.HasPrefix(input, prefixWithHyphen) {
		// Already has configured prefix with hyphen: "bd-a3f8e9"
		normalizedID = input
	} else if hasKnownPrefix(input, knownPrefixes) {
		// Starts with a known/allowed prefix (e.g., "hacker-news-ko4" when allowed_prefixes includes "hacker-news")
		normalizedID = input
	} else if looksLikePrefixedID(input) {
		// Has a different prefix (e.g., "aap-4ar" when configured prefix is "hq-")
		// Don't prepend configured prefix - use as-is for cross-prefix lookup (GH#1513)
		normalizedID = input
	} else {
		// Bare hash or prefix without hyphen: "a3f8e9", "07b8c8", "bda3f8e9" → all get prefix with hyphen added
		normalizedID = prefixWithHyphen + input
	}

	// Try exact match on normalized ID using SearchIssues (GH#942)
	normalizedFilter := types.IssueFilter{IDs: []string{normalizedID}}
	if issues, err := store.SearchIssues(ctx, "", normalizedFilter); err == nil && len(issues) > 0 {
		return issues[0].ID, nil
	}

	// If exact match failed, search for the hash part. The search is narrow on
	// purpose: it matches ids (and, for bare hashes, titles) only. A text scan
	// over description and notes costs seconds on every miss.
	hashPart := strings.TrimPrefix(normalizedID, prefixWithHyphen)
	searchPart, ok := partialIDSearchPart(hashPart)
	if !ok {
		return "", fmt.Errorf("no issue found matching %q", input)
	}

	// An input that names a configured or allowed prefix is answered by the id
	// prefix index: every candidate starts with normalizedID. A bare hash, or a
	// prefix this repo does not declare, has no prefix to anchor on, so it
	// searches every prefix by id substring (cross-prefix lookup). The loop
	// below decides either way, so the candidate set only has to be a superset.
	var filter types.IssueFilter
	var query string
	if hasKnownPrefix(input, knownPrefixes) {
		filter.IDPrefix = normalizedID
	} else {
		filter.QueryPhrase = true
		query = searchPart
	}

	// Narrow projection: this loop only reads the .ID field, so use the
	// SearchIssueIDs path instead of SearchIssues. Avoids hydrating all
	// 45+ issue columns (including big TEXT fields like description, design,
	// notes, metadata, payload) only to discard them.
	ids, err := store.SearchIssueIDs(ctx, query, filter)
	if err != nil {
		return "", fmt.Errorf("failed to search issues: %w", err)
	}

	exactMatch, matches := matchPartialIDCandidates(input, hashPart, knownPrefixes, ids)

	// A transaction-scoped store does not merge wisps into a nil-Ephemeral
	// search (DoltStore does), so a miss also searches the wisps table with
	// the same narrow filter. Both searches are id-only.
	if exactMatch == "" && len(matches) == 0 {
		ephTrue := true
		filter.Ephemeral = &ephTrue
		if wispIDs, wispErr := store.SearchIssueIDs(ctx, query, filter); wispErr == nil {
			exactMatch, matches = matchPartialIDCandidates(input, hashPart, knownPrefixes, wispIDs)
		}
	}

	// Prefer exact match over substring matches
	if exactMatch != "" {
		return exactMatch, nil
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no issue found matching %q", input)
	}

	// Sort so the ambiguity error lists IDs deterministically. SearchIssues return
	// order is not a contract for ambiguous matches, so sorting by ID pins the same
	// message for every storage implementation.
	sort.Strings(matches)

	if len(matches) > 1 {
		return "", fmt.Errorf("%w: %q matches %d issues: %v\nUse more characters to disambiguate", ErrAmbiguousID, input, len(matches), matches)
	}

	return matches[0], nil
}

// matchPartialIDCandidates picks the exact match (if any) and the leading-prefix
// matches for input among the candidate ids a partial-ID search returned.
func matchPartialIDCandidates(input, hashPart string, knownPrefixes []string, ids []string) (string, []string) {
	var matches []string
	var exactMatch string

	for _, id := range ids {
		// Check for exact full ID match first (case: user typed full ID with different prefix)
		if id == input {
			exactMatch = id
			break
		}

		// Extract hash from each issue using config-aware prefix extraction.
		// This correctly handles multi-hyphen prefixes (e.g., "hacker-news-ko4"
		// yields hash "ko4", not "news-ko4" from naive first-hyphen split).
		var issueHash string
		if p := ExtractIssuePrefixKnown(id, knownPrefixes); p != "" && strings.HasPrefix(id, p+"-") {
			issueHash = id[len(p)+1:]
		} else {
			issueHash = id
		}

		// Wisp IDs are shaped "<prefix>-wisp-<hash>", so issueHash is the
		// composite "wisp-<hash>" for a wisp. Strip the literal "wisp-" infix
		// too, so bare-hash lookups (e.g. "t3st") resolve against the isolated
		// hash, not the full "wisp-t3st" string.
		wispHash := strings.TrimPrefix(issueHash, "wisp-")

		// Check for exact hash match (excluding hierarchical children)
		if issueHash == hashPart || wispHash == hashPart {
			exactMatch = id
			// Don't break - keep searching in case there's a full ID match
		} else if strings.HasPrefix(issueHash, hashPart) || strings.HasPrefix(wispHash, hashPart) {
			// Leading-prefix abbreviation (documented UX, e.g. "a3f8" -> "a3f8e9...").
			// HasPrefix rather than Contains: reject interior-substring matches
			// like "kt8" inside "j0kt8" (GH#4234).
			matches = append(matches, id)
		}
	}

	return exactMatch, matches
}

func partialIDSearchPart(hashPart string) (string, bool) {
	if !looksLikePartialIDHash(hashPart) {
		return "", false
	}
	searchPart := hashPart
	if idx := strings.LastIndex(hashPart, "-"); idx >= 0 && idx < len(hashPart)-1 {
		suffix := hashPart[idx+1:]
		if looksLikePartialIDHash(suffix) {
			searchPart = suffix
		}
	}
	return searchPart, true
}

func looksLikePartialIDHash(input string) bool {
	if input == "" || strings.Contains(input, " ") {
		return false
	}
	for _, c := range input {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// ResolvePartialIDs resolves multiple potentially partial issue IDs.
// Returns the resolved IDs and any errors encountered.
func ResolvePartialIDs(ctx context.Context, store PartialIDResolverStore, inputs []string) ([]string, error) {
	var resolved []string
	for _, input := range inputs {
		fullID, err := ResolvePartialID(ctx, store, input)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, fullID)
	}
	return resolved, nil
}

// looksLikePrefixedID checks if input appears to already have a prefix.
// A prefixed ID has the format "prefix-hash" where prefix is 1-8 lowercase
// letters/numbers and hash is alphanumeric (potentially with dots for hierarchical IDs).
// Examples: "aap-4ar", "bd-a3f8e9", "myproject-abc.1"
func looksLikePrefixedID(input string) bool {
	idx := strings.Index(input, "-")
	if idx <= 0 || idx > 8 {
		// No hyphen, hyphen at start, or prefix too long
		return false
	}

	prefix := input[:idx]
	suffix := input[idx+1:]

	// Prefix must be non-empty lowercase alphanumeric
	for _, c := range prefix {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}

	// Suffix must be non-empty and start with alphanumeric
	if len(suffix) == 0 {
		return false
	}
	first := rune(suffix[0])
	if !((first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		return false
	}

	return true
}

// hasKnownPrefix checks if input starts with any of the known prefixes followed
// by a hyphen. Used to detect already-prefixed input before falling back to the
// looksLikePrefixedID heuristic.
func hasKnownPrefix(input string, knownPrefixes []string) bool {
	for _, p := range knownPrefixes {
		if p != "" && strings.HasPrefix(input, p+"-") {
			return true
		}
	}
	return false
}
