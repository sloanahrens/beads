package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/steveyegge/beads/internal/eventsjournal"
	"github.com/steveyegge/beads/internal/types"
)

// The hot reads a program polls: bd ready --json and bd events tail. Both
// page by keyset under machine mode: the caller hands back the cursor of the
// last row it saw and gets the rows strictly after it, so a row inserted or
// removed between calls never shifts a page the way an offset would.
//
// The functions here take the full candidate set and cut the page; the
// commands and the contract test's in-memory source both call them, so the
// two cannot drift.

// readyCursor is the keyset position after one ready row.
type readyCursor struct {
	Policy   types.SortPolicy `json:"s"`
	Priority int              `json:"p"`
	Created  time.Time        `json:"c"`
	ID       string           `json:"i"`
}

func encodeReadyCursor(policy types.SortPolicy, issue *types.Issue) string {
	b, _ := json.Marshal(readyCursor{Policy: policy, Priority: issue.Priority, Created: issue.CreatedAt.UTC(), ID: issue.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeReadyCursor(s string) (readyCursor, error) {
	var c readyCursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, fmt.Errorf("invalid --after cursor: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" {
		return c, fmt.Errorf("invalid --after cursor")
	}
	return c, nil
}

// readyKeysetPolicy normalizes the sort policy and refuses the one whose
// order is not a stable key: hybrid moves rows across its 48h recency line as
// the clock advances, so a cursor taken now would not describe a position
// later.
func readyKeysetPolicy(p types.SortPolicy) (types.SortPolicy, error) {
	switch p {
	case types.SortPolicyPriority, types.SortPolicyOldest:
		return p, nil
	case "":
		return types.SortPolicyPriority, nil
	}
	return "", fmt.Errorf("--after needs --sort priority or --sort oldest; %q orders by the clock and has no stable keyset", p)
}

// readyKeyLess is the keyset order: (priority, created_at, id) for the
// priority policy, (created_at, id) for oldest. It matches the order the
// ready query returns for those policies.
func readyKeyLess(policy types.SortPolicy, ap int, at time.Time, aid string, bp int, bt time.Time, bid string) bool {
	if policy == types.SortPolicyPriority && ap != bp {
		return ap < bp
	}
	if !at.Equal(bt) {
		return at.Before(bt)
	}
	return aid < bid
}

// readyKeysetPage sorts the full ready set by policy, drops everything at or
// before after (when given), and cuts limit rows (0 = all). next is the
// cursor of the last row returned when more rows follow.
func readyKeysetPage(rows []*types.IssueWithCounts, policy types.SortPolicy, after *readyCursor, limit int) (page []*types.IssueWithCounts, hasMore bool, next string) {
	sorted := make([]*types.IssueWithCounts, 0, len(rows))
	for _, r := range rows {
		if r != nil && r.Issue != nil {
			sorted = append(sorted, r)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].Issue, sorted[j].Issue
		return readyKeyLess(policy, a.Priority, a.CreatedAt, a.ID, b.Priority, b.CreatedAt, b.ID)
	})
	start := 0
	if after != nil {
		start = sort.Search(len(sorted), func(i int) bool {
			x := sorted[i].Issue
			return readyKeyLess(policy, after.Priority, after.Created, after.ID, x.Priority, x.CreatedAt, x.ID)
		})
	}
	rest := sorted[start:]
	if limit > 0 && len(rest) > limit {
		page = rest[:limit]
		hasMore = true
		next = encodeReadyCursor(policy, page[len(page)-1].Issue)
	} else {
		page = rest
	}
	if page == nil {
		page = []*types.IssueWithCounts{}
	}
	return page, hasMore, next
}

// eventsTailData is the machine-mode data member of bd events tail.
type eventsTailData struct {
	Records []eventsjournal.Record `json:"records"`
	// NextSince is the --since for the next call: the last seq returned, or
	// the --since given when nothing was.
	NextSince int64 `json:"next_since"`
}

// journalPage keeps records with seq > since, in seq order, and cuts limit
// of them (0 = all). The caller reads limit+1 rows so a full page can tell
// whether more follow.
func journalPage(rows []eventsjournal.Record, since int64, limit int) (eventsTailData, pageResult) {
	out := eventsTailData{Records: []eventsjournal.Record{}, NextSince: since}
	for _, r := range rows {
		if r.Seq > since {
			out.Records = append(out.Records, r)
		}
	}
	sort.SliceStable(out.Records, func(i, j int) bool { return out.Records[i].Seq < out.Records[j].Seq })
	hasMore := false
	if limit > 0 && len(out.Records) > limit {
		out.Records = out.Records[:limit]
		hasMore = true
	}
	if n := len(out.Records); n > 0 {
		out.NextSince = out.Records[n-1].Seq
	}
	pr := pageResult{
		HasMore:  hasMore,
		Returned: len(out.Records),
		Limit:    limit,
		// The journal has no default cap: any cut is one the caller asked for.
		LimitExplicit: true,
	}
	if hasMore {
		pr.NextCursor = strconv.FormatInt(out.NextSince, 10)
	}
	return out, pr
}
