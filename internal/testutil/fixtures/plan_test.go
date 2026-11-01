package fixtures

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// planNow is the fixed clock every plan test passes in, so plans can be compared.
var planNow = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)

func TestPlanLargeConfigShape(t *testing.T) {
	tests := []struct {
		name                   string
		cfg                    DataConfig
		epics, features, tasks int
	}{
		{name: "large", cfg: DefaultLargeConfig(), epics: 1000, features: 3000, tasks: 6000},
		{name: "xlarge", cfg: DefaultXLargeConfig(), epics: 2000, features: 6000, tasks: 12000},
	}
	parentType := map[types.IssueType]types.IssueType{
		types.TypeFeature: types.TypeEpic,
		types.TypeTask:    types.TypeFeature,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := planIssues(tt.cfg, planNow)
			if len(issues) != tt.cfg.TotalIssues {
				t.Fatalf("planned %d issues, want %d", len(issues), tt.cfg.TotalIssues)
			}

			counts := map[types.IssueType]int{}
			for i, issue := range issues {
				counts[issue.IssueType]++
				wantParent, hasParent := parentType[issue.IssueType]
				if !hasParent {
					continue
				}
				parents := 0
				for _, dep := range issue.Dependencies {
					if dep.Type != types.DepParentChild {
						continue
					}
					parents++
					if dep.IssueID != planRef(i) {
						t.Errorf("issue %d: parent link starts at %s", i, dep.IssueID)
					}
					p, err := planRefIndex(dep.DependsOnID)
					if err != nil {
						t.Fatalf("issue %d: %v", i, err)
					}
					if p >= i {
						t.Errorf("issue %d depends on parent %d, which is not created earlier", i, p)
					}
					if issues[p].IssueType != wantParent {
						t.Errorf("issue %d (%s) has parent %d of type %s, want %s",
							i, issue.IssueType, p, issues[p].IssueType, wantParent)
					}
				}
				if parents != 1 {
					t.Errorf("issue %d (%s) has %d parent links, want 1", i, issue.IssueType, parents)
				}
			}

			want := map[types.IssueType]int{
				types.TypeEpic:    tt.epics,
				types.TypeFeature: tt.features,
				types.TypeTask:    tt.tasks,
			}
			if !reflect.DeepEqual(counts, want) {
				t.Errorf("issue types = %v, want %v", counts, want)
			}
		})
	}
}

func TestPlanCrossLinksFormNoCycle(t *testing.T) {
	issues := planIssues(DefaultLargeConfig(), planNow)

	// Independent of blockGraph: a plain DFS that colours nodes as it goes.
	const (
		unvisited = iota
		inProgress
		done
	)
	adj := make(map[int][]int)
	links := 0
	for i, issue := range issues {
		for _, dep := range issue.Dependencies {
			if dep.Type != types.DepBlocks {
				continue
			}
			to, err := planRefIndex(dep.DependsOnID)
			if err != nil {
				t.Fatalf("issue %d: %v", i, err)
			}
			adj[i] = append(adj[i], to)
			links++
		}
	}
	if links == 0 {
		t.Fatal("plan has no cross-links, so the cycle check proves nothing")
	}

	state := make(map[int]int)
	var visit func(n int) bool
	visit = func(n int) bool {
		state[n] = inProgress
		for _, next := range adj[n] {
			switch state[next] {
			case inProgress:
				return false
			case unvisited:
				if !visit(next) {
					return false
				}
			}
		}
		state[n] = done
		return true
	}
	for n := range adj {
		if state[n] == unvisited && !visit(n) {
			t.Fatalf("cross-links close a cycle through issue %d", n)
		}
	}
}

func TestPlanSameSeedSameDataset(t *testing.T) {
	cfg := DefaultLargeConfig()
	first := planIssues(cfg, planNow)
	again := planIssues(cfg, planNow)
	if !reflect.DeepEqual(first, again) {
		t.Fatal("the same seed planned two different datasets")
	}

	other := cfg
	other.RandSeed++
	if reflect.DeepEqual(first, planIssues(other, planNow)) {
		t.Fatal("a different seed planned the same dataset")
	}
}

func TestPlanChunksNeverDependOnLaterChunk(t *testing.T) {
	for _, size := range []int{1, 7, fixtureChunkSize, 10000} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			issues := planIssues(DefaultLargeConfig(), planNow)
			index := make(map[*types.Issue]int, len(issues))
			planned := 0
			for i, issue := range issues {
				index[issue] = i
				planned += len(issue.Dependencies)
			}

			// idOf maps the ID a chunk's real-ID ends name to its plan index.
			idOf := make(map[string]int)
			written := 0
			deferred, err := writePlanInChunks(issues, size, func(chunk []*types.Issue) error {
				chunkEnd := index[chunk[0]] + len(chunk)
				for _, issue := range chunk {
					for _, dep := range issue.Dependencies {
						written++
						for _, end := range []string{dep.IssueID, dep.DependsOnID} {
							target, err := targetOf(end, chunk, index, idOf)
							if err != nil {
								t.Fatalf("%v", err)
							}
							if target >= chunkEnd {
								t.Errorf("chunk [%d,%d) holds a dependency on issue %d, created in a later chunk",
									index[chunk[0]], chunkEnd, target)
							}
						}
					}
				}
				for _, issue := range chunk {
					id := fmt.Sprintf("bd-%d", index[issue])
					issue.ID = id
					idOf[id] = index[issue]
				}
				return nil
			})
			if err != nil {
				t.Fatalf("writePlanInChunks: %v", err)
			}

			for _, dep := range deferred {
				from, err := planRefIndex(dep.IssueID)
				if err != nil {
					t.Fatalf("deferred edge: %v", err)
				}
				to, err := planRefIndex(dep.DependsOnID)
				if err != nil {
					t.Fatalf("deferred edge: %v", err)
				}
				if to/size <= from/size {
					t.Errorf("deferred edge %d -> %d does not wait for a later chunk", from, to)
				}
			}
			if written+len(deferred) != planned {
				t.Errorf("wrote %d and deferred %d dependencies, the plan has %d", written, len(deferred), planned)
			}
		})
	}
}

// targetOf resolves one dependency end seen inside a chunk to the plan index it
// names. A batch ref must name an issue of the chunk; an ID must belong to an
// issue written by an earlier chunk.
func targetOf(end string, chunk []*types.Issue, index map[*types.Issue]int, idOf map[string]int) (int, error) {
	if digits, ok := strings.CutPrefix(end, batchRefPrefix); ok {
		local, err := strconv.Atoi(digits)
		if err != nil || local < 0 || local >= len(chunk) {
			return 0, fmt.Errorf("batch ref %q does not name an issue of this %d-issue chunk", end, len(chunk))
		}
		return index[chunk[local]], nil
	}
	target, ok := idOf[end]
	if !ok {
		return 0, fmt.Errorf("end %q is neither a batch ref nor the ID of an earlier chunk", end)
	}
	return target, nil
}

func TestPlanChunksSplitsRanges(t *testing.T) {
	tests := []struct {
		name    string
		n, size int
		want    [][2]int
		wantErr bool
	}{
		{name: "uneven", n: 7, size: 3, want: [][2]int{{0, 3}, {3, 6}, {6, 7}}},
		{name: "one chunk", n: 4, size: 10, want: [][2]int{{0, 4}}},
		{name: "empty", n: 0, size: 3, want: nil},
		{name: "zero size refused", n: 7, size: 0, wantErr: true},
		{name: "negative size refused", n: 7, size: -1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planChunks(tt.n, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("planChunks(%d, %d) error = %v, wantErr %v", tt.n, tt.size, err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("planChunks(%d, %d) = %v, want %v", tt.n, tt.size, got, tt.want)
			}
		})
	}
}

func TestPlanLocalizeRefusesBadEnds(t *testing.T) {
	tests := []struct {
		name         string
		dep          *types.Dependency
		earlierID    string
		start, end   int
		wantErr      bool
		wantDeferred bool
		wantEnds     [2]string
	}{
		{
			name:      "in-chunk and earlier ends are localized",
			dep:       &types.Dependency{IssueID: planRef(3), DependsOnID: planRef(0)},
			earlierID: "bd-0",
			start:     1, end: 5,
			wantEnds: [2]string{"batch:2", "bd-0"},
		},
		{
			name:  "a later end is deferred",
			dep:   &types.Dependency{IssueID: planRef(2), DependsOnID: planRef(7)},
			start: 0, end: 5,
			wantDeferred: true,
		},
		{
			name:  "a non-plan end is refused",
			dep:   &types.Dependency{IssueID: planRef(3), DependsOnID: "bd-9"},
			start: 1, end: 5,
			wantErr: true,
		},
		{
			name:  "a malformed plan ref is refused",
			dep:   &types.Dependency{IssueID: planRef(3), DependsOnID: "plan:x"},
			start: 1, end: 5,
			wantErr: true,
		},
		{
			name:  "an earlier issue with no ID is refused",
			dep:   &types.Dependency{IssueID: planRef(3), DependsOnID: planRef(0)},
			start: 1, end: 5,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := make([]*types.Issue, 5)
			for i := range issues {
				issues[i] = &types.Issue{Title: fmt.Sprintf("issue %d", i)}
			}
			issues[0].ID = tt.earlierID

			got, ok, err := localizeDependency(issues, tt.dep, tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if ok == tt.wantDeferred {
				t.Fatalf("ok = %v, wantDeferred %v", ok, tt.wantDeferred)
			}
			if ok && [2]string{got.IssueID, got.DependsOnID} != tt.wantEnds {
				t.Errorf("ends = %q, want %q", [2]string{got.IssueID, got.DependsOnID}, tt.wantEnds)
			}
		})
	}
}

func TestPlanEdgesRefuseUnwrittenEnds(t *testing.T) {
	tests := []struct {
		name    string
		issues  []*types.Issue
		dep     *types.Dependency
		want    []issueops.DependencyEdge
		wantErr bool
	}{
		{
			name:   "both ends written",
			issues: []*types.Issue{{ID: "bd-0"}, {ID: "bd-1"}},
			dep:    &types.Dependency{IssueID: planRef(1), DependsOnID: planRef(0), Type: types.DepBlocks},
			want:   []issueops.DependencyEdge{{IssueID: "bd-1", DependsOnID: "bd-0", Type: types.DepBlocks}},
		},
		{
			name:    "end past the plan is refused",
			issues:  []*types.Issue{{ID: "bd-0"}},
			dep:     &types.Dependency{IssueID: planRef(0), DependsOnID: planRef(5), Type: types.DepBlocks},
			wantErr: true,
		},
		{
			name:    "end with no ID is refused",
			issues:  []*types.Issue{{ID: "bd-0"}, {}},
			dep:     &types.Dependency{IssueID: planRef(1), DependsOnID: planRef(0), Type: types.DepBlocks},
			wantErr: true,
		},
		{
			name:    "end that is not a plan ref is refused",
			issues:  []*types.Issue{{ID: "bd-0"}},
			dep:     &types.Dependency{IssueID: "bd-0", DependsOnID: planRef(0), Type: types.DepBlocks},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dependencyEdges(tt.issues, []*types.Dependency{tt.dep})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("edges = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanWriteStopsOnError(t *testing.T) {
	issues := []*types.Issue{{Title: "a"}, {Title: "b"}, {Title: "c"}}
	calls := 0
	wantErr := errors.New("store refused the chunk")
	_, err := writePlanInChunks(issues, 1, func(chunk []*types.Issue) error {
		calls++
		if calls == 2 {
			return wantErr
		}
		for i, issue := range chunk {
			issue.ID = fmt.Sprintf("bd-%d", calls*10+i)
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Errorf("write called %d times, want it to stop at 2", calls)
	}
}

func TestPlanBlockGraphRefusesSelfDuplicateAndCycle(t *testing.T) {
	g := newBlockGraph()
	tests := []struct {
		from, to int
		want     bool
	}{
		{1, 1, false}, // self
		{1, 2, true},
		{1, 2, false}, // repeats an edge
		{2, 1, false}, // closes 1 -> 2 -> 1
		{2, 3, true},
		{3, 1, false}, // closes 1 -> 2 -> 3 -> 1
		{4, 1, true},
	}
	for _, tt := range tests {
		if got := g.add(tt.from, tt.to); got != tt.want {
			t.Errorf("add(%d, %d) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
