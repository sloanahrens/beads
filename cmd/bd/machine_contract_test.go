package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/eventsjournal"
	"github.com/steveyegge/beads/internal/types"
)

// hotReadSource is one implementation of the machine-mode hot-read surface:
// bd ready --json and bd events tail. The same contract assertions run
// against every source (runHotReadContract), so the in-memory path and the
// real binary cannot drift apart.
type hotReadSource interface {
	// seed creates n ready issues, one events-journal record each at least.
	seed(t *testing.T, n int)
	ready(t *testing.T, after string, limit int) (decodedEnvelope, int)
	events(t *testing.T, since int64, limit int) (decodedEnvelope, int)
}

// fakeHotReads serves the contract from memory through the same page
// functions the commands use.
type fakeHotReads struct {
	rows    []*types.IssueWithCounts
	records []eventsjournal.Record
}

func (f *fakeHotReads) seed(_ *testing.T, n int) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		f.rows = append(f.rows, &types.IssueWithCounts{Issue: &types.Issue{
			ID:        fmt.Sprintf("fk-%02d", i),
			Priority:  (n - i) % 3,
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}})
		f.records = append(f.records, eventsjournal.Record{Seq: int64(i + 1), Op: "create", IssueID: fmt.Sprintf("fk-%02d", i)})
	}
}

func (f *fakeHotReads) ready(t *testing.T, after string, limit int) (decodedEnvelope, int) {
	env, _, _, code := runMachine(t, func() error {
		var cur *readyCursor
		if after != "" {
			c, err := decodeReadyCursor(after)
			if err != nil {
				return newCLIError(kindInvalidArgs, "%s", err.Error())
			}
			cur = &c
		}
		page, hasMore, next := readyKeysetPage(f.rows, types.SortPolicyPriority, cur, limit)
		return outputJSONPage(page, pageResult{HasMore: hasMore, Returned: len(page), Limit: limit, LimitExplicit: true, NextCursor: next})
	})
	return env, code
}

func (f *fakeHotReads) events(t *testing.T, since int64, limit int) (decodedEnvelope, int) {
	env, _, _, code := runMachine(t, func() error {
		data, page := journalPage(f.records, since, limit)
		return outputJSONPage(data, page)
	})
	return env, code
}

func TestHotReadContract_InMemory(t *testing.T) {
	runHotReadContract(t, &fakeHotReads{})
}

// runHotReadContract is the contract: keyset paging visits every row exactly
// once, in order; every page carries pagination; truncated and next_cursor
// agree; the last page says it is the last.
func runHotReadContract(t *testing.T, src hotReadSource) {
	const n = 5
	src.seed(t, n)

	t.Run("ready pages by keyset", func(t *testing.T) {
		seen := map[string]bool{}
		var order []int
		after := ""
		for pages := 0; ; pages++ {
			if pages > n {
				t.Fatalf("paging did not terminate after %d pages", pages)
			}
			env, code := src.ready(t, after, 2)
			if code != 0 || env.Error != nil {
				t.Fatalf("ready page %d: exit %d error %+v", pages, code, env.Error)
			}
			if env.Pagination == nil {
				t.Fatalf("ready page %d: no pagination", pages)
			}
			var rows []struct {
				ID       string `json:"id"`
				Priority int    `json:"priority"`
			}
			if err := json.Unmarshal(env.Data, &rows); err != nil {
				t.Fatalf("ready data: %v\n%s", err, env.Data)
			}
			if env.Pagination.Returned != len(rows) {
				t.Fatalf("pagination.returned %d != %d rows", env.Pagination.Returned, len(rows))
			}
			if env.Pagination.Truncated != (env.Pagination.NextCursor != "") {
				t.Fatalf("truncated=%v but next_cursor=%q", env.Pagination.Truncated, env.Pagination.NextCursor)
			}
			for _, r := range rows {
				if seen[r.ID] {
					t.Fatalf("id %s served twice", r.ID)
				}
				seen[r.ID] = true
				order = append(order, r.Priority)
			}
			if !env.Pagination.Truncated {
				break
			}
			after = env.Pagination.NextCursor
		}
		if len(seen) != n {
			t.Fatalf("paging visited %d ids, want %d", len(seen), n)
		}
		for i := 1; i < len(order); i++ {
			if order[i] < order[i-1] {
				t.Fatalf("priority order broken across pages: %v", order)
			}
		}
	})

	t.Run("events tail pages by seq", func(t *testing.T) {
		var seqs []int64
		since := int64(0)
		for pages := 0; ; pages++ {
			if pages > 10*n {
				t.Fatalf("events paging did not terminate")
			}
			env, code := src.events(t, since, 2)
			if code != 0 || env.Error != nil || env.Pagination == nil {
				t.Fatalf("events page: exit %d error %+v pagination %+v", code, env.Error, env.Pagination)
			}
			var data struct {
				Records []struct {
					Seq int64 `json:"seq"`
				} `json:"records"`
				NextSince int64 `json:"next_since"`
			}
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("events data: %v\n%s", err, env.Data)
			}
			for _, r := range data.Records {
				seqs = append(seqs, r.Seq)
			}
			if len(data.Records) > 0 && data.NextSince != data.Records[len(data.Records)-1].Seq {
				t.Fatalf("next_since %d is not the last seq", data.NextSince)
			}
			if !env.Pagination.Truncated {
				break
			}
			next, err := strconv.ParseInt(env.Pagination.NextCursor, 10, 64)
			if err != nil || next != data.NextSince {
				t.Fatalf("next_cursor %q does not match next_since %d", env.Pagination.NextCursor, data.NextSince)
			}
			since = next
		}
		if len(seqs) < n {
			t.Fatalf("events visited %d records, want at least %d", len(seqs), n)
		}
		for i := 1; i < len(seqs); i++ {
			if seqs[i] != seqs[i-1]+1 {
				t.Fatalf("seqs not gapless and increasing: %v", seqs)
			}
		}
	})
}

func TestReadyKeysetRefusals(t *testing.T) {
	if _, err := readyKeysetPolicy(types.SortPolicyHybrid); err == nil {
		t.Error("hybrid sort accepted for keyset paging; its order moves with the clock")
	}
	if p, err := readyKeysetPolicy(""); err != nil || p != types.SortPolicyPriority {
		t.Errorf("default policy = %q, %v; want priority", p, err)
	}
	if _, err := decodeReadyCursor("not-a-cursor!"); err == nil {
		t.Error("garbage cursor accepted")
	}
	issue := &types.Issue{ID: "x-1", Priority: 2, CreatedAt: time.Now()}
	c, err := decodeReadyCursor(encodeReadyCursor(types.SortPolicyOldest, issue))
	if err != nil || c.ID != "x-1" || c.Policy != types.SortPolicyOldest || !c.Created.Equal(issue.CreatedAt) {
		t.Errorf("cursor round trip = %+v, %v", c, err)
	}
}
