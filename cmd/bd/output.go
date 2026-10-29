package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const JSONSchemaVersion = 1

// PaginationMeta carries truncation context for paginated JSON responses.
// It is included in the envelope under the "pagination" key whenever the
// result set was capped by a --limit.
type PaginationMeta struct {
	Returned  int  `json:"returned"`
	Total     int  `json:"total,omitempty"`
	Truncated bool `json:"truncated"`
	// Limit is the page size that applied (0 = unlimited). Set on the
	// machine-mode envelope only.
	Limit int `json:"limit,omitempty"`
	// NextCursor resumes after the last row returned: pass it back as
	// --after (bd ready) or --since (bd events tail). Set only when more
	// rows exist.
	NextCursor string `json:"next_cursor,omitempty"`
}

func outputJSON(v interface{}) error {
	return outputJSONWithPagination(v, nil)
}

// paginationMetaFor builds the envelope pagination block for a page whose
// HasMore verdict and row count are already known. Returns nil when the page
// was not truncated, so callers can pass the result straight to
// outputJSONWithPagination without an extra branch. Total is left unset
// (omitempty) because list/query pages don't carry a full-count probe the
// way bd ready's does.
func paginationMetaFor(hasMore bool, returned int) *PaginationMeta {
	if !hasMore {
		return nil
	}
	return &PaginationMeta{Returned: returned, Truncated: true}
}

// outputJSONWithPagination emits v in the envelope, with a "pagination" key
// when p is non-nil so programmatic consumers can detect truncation without
// parsing stderr.
func outputJSONWithPagination(v interface{}, p *PaginationMeta) error {
	if machineModeActive() {
		stageMachineData(v, p)
		return nil
	}
	out := map[string]interface{}{
		"schema_version": JSONSchemaVersion,
		"data":           v,
	}
	if p != nil {
		out["pagination"] = p
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encoding JSON: %v", err)
	}
	return nil
}

func outputJSONRaw(v interface{}) error {
	if machineModeActive() {
		stageMachineData(v, nil)
		return nil
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON: %v", err)
	}
	return nil
}

// wrapWithSchemaVersion is the outside-machine-mode shape for every --json
// result: {"schema_version": N, "data": v}.
func wrapWithSchemaVersion(v interface{}) interface{} {
	return map[string]interface{}{
		"schema_version": JSONSchemaVersion,
		"data":           v,
	}
}

func outputJSONError(err error, code string) error {
	if machineModeActive() {
		e := newCLIError(kindInternal, "%s", err.Error())
		if code != "" {
			e.Detail = map[string]any{"code": code}
		}
		recordMachineError(e)
		return &exitError{Code: 1}
	}
	base := map[string]interface{}{
		"error": err.Error(),
	}
	if code != "" {
		base["code"] = code
	}
	encoder := json.NewEncoder(os.Stderr)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(wrapWithSchemaVersion(base))
	return &exitError{Code: 1}
}

// pageResult describes one page of a paged read for outputJSONPage.
type pageResult struct {
	HasMore  bool
	Returned int
	Total    int // 0 when unknown
	// Limit is the page size that applied; LimitExplicit says the caller
	// chose it with --limit rather than inheriting the default.
	Limit         int
	LimitExplicit bool
	NextCursor    string
}

// outputJSONPage emits a paged read. Outside machine mode it is exactly
// outputJSONWithPagination (pagination only when truncated). In machine mode
// pagination is always present, whatever the TTY, and a page cut by the
// DEFAULT limit is a truncated error: the
// caller asked for everything and did not get it. A page cut by an explicit
// --limit is what the caller asked for and exits 0.
func outputJSONPage(v interface{}, page pageResult) error {
	if !machineModeActive() {
		p := paginationMetaFor(page.HasMore, page.Returned)
		if p != nil {
			p.Total = page.Total
			p.NextCursor = page.NextCursor
		}
		return outputJSONWithPagination(v, p)
	}
	p := &PaginationMeta{
		Returned:  page.Returned,
		Total:     page.Total,
		Truncated: page.HasMore,
		Limit:     page.Limit,
	}
	if page.HasMore {
		p.NextCursor = page.NextCursor
	}
	stageMachineData(v, p)
	if page.HasMore && !page.LimitExplicit {
		e := newCLIError(kindTruncated,
			"result cut at the default limit of %d rows; pass --limit 0 for all rows or --limit N to page", page.Limit)
		recordMachineError(e)
		return e
	}
	return nil
}
