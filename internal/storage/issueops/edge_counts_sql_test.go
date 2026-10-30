package issueops

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
	publicops "github.com/steveyegge/beads/issueops"
)

// recordingDBTX captures every query it is asked to run, so a test can pin the
// SQL the production body actually sent rather than re-deriving it.
type recordingDBTX struct {
	DBTX
	queries []string
}

func (r *recordingDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	r.queries = append(r.queries, query)
	return r.DBTX.QueryContext(ctx, query, args...)
}

// outerFilters returns the WHERE filters of the SELECT that wraps the derived
// table a dependency query is built from: everything between the derived
// table's ") d" close and the GROUP BY, or "" when that SELECT has no WHERE.
func outerFilters(normalized string) string {
	const derivedClose = ") d"
	at := strings.Index(normalized, derivedClose)
	if at < 0 {
		return ""
	}
	rest := normalized[at+len(derivedClose):]
	if group := strings.LastIndex(rest, " GROUP BY "); group >= 0 {
		rest = rest[:group]
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "WHERE"))
}

// expectEdgeCountReads queues the two existence probes and the two per-plane
// count reads ExecuteEdgeCount issues for anchors, and returns the recorder that
// captured their SQL.
func expectEdgeCountReads(t *testing.T, anchors []string, request publicops.EdgeCountRequest) (*recordingDBTX, sqlmock.Sqlmock) {
	t.Helper()
	tx, mock := newBindCheckedDB(t)
	rec := &recordingDBTX{DBTX: tx}

	idArgs := make([]driver.Value, len(anchors))
	for i, id := range anchors {
		idArgs[i] = id
	}
	for _, table := range []string{"issues", "wisps"} {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM " + table + " WHERE id IN")).
			WithArgs(idArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}

	countArgs := make([]driver.Value, 0, len(anchors)*len(sqlbuild.DepTargetColumns)+len(request.Types)+1)
	if request.Direction == publicops.EdgeDirectionOut {
		countArgs = append(countArgs, idArgs...)
	} else {
		// The inbound target predicate is one seek per typed target column, so
		// the batch binds once per branch, in DepTargetColumns order.
		for range sqlbuild.DepTargetColumns {
			countArgs = append(countArgs, idArgs...)
		}
	}
	for _, depType := range request.Types {
		countArgs = append(countArgs, string(depType))
	}
	if request.Direction == publicops.EdgeDirectionIn && request.Status != "" {
		countArgs = append(countArgs, request.Status)
	}
	// Each count read matches on the table it scans, not on the predicate it
	// happens to emit, so a predicate regression reaches the assertions in the
	// test rather than stopping at the mock.
	for _, plane := range edgeCountPlanes {
		mock.ExpectQuery(regexp.QuoteMeta("FROM " + plane.dependencies)).
			WithArgs(countArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"anchor", "n"}))
	}
	return rec, mock
}

// TestEdgeCountsInboundSQLSeeksTypedTargets drives the production count and pins
// the SQL of both planes' inbound reads: the batch predicate is one bare
// typed-column seek per branch, with no COALESCE in any WHERE, while the
// projection and GROUP BY keep the COALESCE target expression.
func TestEdgeCountsInboundSQLSeeksTypedTargets(t *testing.T) {
	anchors := []string{"bd-1", "bd-2"}
	const wantSelect = "SELECT COALESCE(d.depends_on_issue_id, d.depends_on_wisp_id, d.depends_on_external) AS anchor, COUNT(*) AS n FROM ("
	const wantGroupBy = "GROUP BY COALESCE(d.depends_on_issue_id, d.depends_on_wisp_id, d.depends_on_external)"

	cases := []struct {
		name     string
		request  publicops.EdgeCountRequest
		wantType string
	}{
		{
			name:    "unnarrowed",
			request: publicops.EdgeCountRequest{Direction: publicops.EdgeDirectionIn, IDs: anchors},
		},
		{
			name: "type and status narrowed",
			request: publicops.EdgeCountRequest{
				Direction: publicops.EdgeDirectionIn,
				IDs:       anchors,
				Types:     []types.DependencyType{types.DepBlocks},
				Status:    string(types.StatusOpen),
			},
			wantType: "d.type IN (?)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, mock := expectEdgeCountReads(t, anchors, tc.request)
			if _, err := ExecuteEdgeCount(context.Background(), rec, tc.request); err != nil {
				t.Fatalf("ExecuteEdgeCount: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
			if n := len(rec.queries); n != 4 {
				t.Fatalf("ran %d queries, want two probes and two count reads", n)
			}

			for i, plane := range edgeCountPlanes {
				got := normalizeSQLSpace(rec.queries[2+i])

				// The answer's columns and grouping are what they were: anchor
				// by the typed-column COALESCE, n by COUNT(*).
				if !strings.HasPrefix(got, wantSelect) {
					t.Errorf("SELECT list changed for %s:\n got: %s\nwant prefix: %s", plane.dependencies, got, wantSelect)
				}
				if !strings.HasSuffix(got, wantGroupBy) {
					t.Errorf("GROUP BY changed for %s:\n got: %s\nwant suffix: %s", plane.dependencies, got, wantGroupBy)
				}

				// Property: the target is resolved as an expression in exactly the
				// two pinned places (projection and GROUP BY); no filter or join
				// key anywhere else may spell it as a COALESCE.
				rest := strings.ReplaceAll(got, wantSelect, "")
				rest = strings.ReplaceAll(rest, wantGroupBy, "")
				if strings.Contains(rest, "COALESCE") {
					t.Errorf("%s resolves the target as COALESCE outside the projection and GROUP BY:\n%s",
						plane.dependencies, got)
				}
				if where := outerFilters(got); strings.Contains(where, "COALESCE") {
					t.Errorf("%s WHERE still predicates on the COALESCE target:\n%s", plane.dependencies, got)
				}
				for _, col := range sqlbuild.DepTargetColumns {
					if want := col + " IN (?,?)"; !strings.Contains(got, want) {
						t.Errorf("%s target predicate does not seek %q:\n%s", plane.dependencies, want, got)
					}
				}

				where := outerFilters(got)
				if tc.wantType == "" {
					if where != "" {
						t.Errorf("%s unfiltered inbound count filters on %q:\n%s", plane.dependencies, where, got)
					}
				} else if !strings.Contains(where, tc.wantType) {
					t.Errorf("%s WHERE does not carry the type filter %q:\n%s", plane.dependencies, tc.wantType, got)
				}

				if tc.request.Status == "" {
					if strings.Contains(got, "JOIN") {
						t.Errorf("%s unnarrowed count joins the dependent's plane for nothing:\n%s", plane.dependencies, got)
					}
				} else if want := fmt.Sprintf("JOIN %s s ON s.id = d.issue_id", plane.sources); !strings.Contains(got, want) {
					t.Errorf("%s status narrowing does not join the DEPENDENT's own plane (%q):\n%s",
						plane.dependencies, want, got)
				}
			}
		})
	}
}

// TestEdgeCountsOutboundSQLKeepsTheSourceKey pins the outbound read, which was
// already keyed by the source column: it keeps its single-column predicate and
// binds the batch once.
func TestEdgeCountsOutboundSQLKeepsTheSourceKey(t *testing.T) {
	anchors := []string{"bd-1", "bd-2"}
	request := publicops.EdgeCountRequest{
		Direction: publicops.EdgeDirectionOut,
		IDs:       anchors,
		Types:     []types.DependencyType{types.DepBlocks},
	}

	rec, mock := expectEdgeCountReads(t, anchors, request)
	if _, err := ExecuteEdgeCount(context.Background(), rec, request); err != nil {
		t.Fatalf("ExecuteEdgeCount: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	for _, plane := range []string{"dependencies", "wisp_dependencies"} {
		var found bool
		for _, query := range rec.queries {
			got := normalizeSQLSpace(query)
			if !strings.Contains(got, "FROM "+plane+" d WHERE") {
				continue
			}
			found = true
			want := "SELECT d.issue_id AS anchor, COUNT(*) AS n FROM " + plane +
				" d WHERE d.issue_id IN (?,?) AND d.type IN (?) GROUP BY d.issue_id"
			if got != want {
				t.Errorf("%s outbound count changed:\n got: %s\nwant: %s", plane, got, want)
			}
		}
		if !found {
			t.Errorf("no outbound count ran against %s", plane)
		}
	}
}
