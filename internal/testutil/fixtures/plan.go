package fixtures

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// planRefPrefix marks a dependency end that names an issue of the plan by its
// position in the plan. The writer replaces it with a batch ref or with the
// issue's ID before the dependency is written.
const planRefPrefix = "plan:"

// batchRefPrefix is the form CheckBatchDependencyRefs and
// ResolveBatchDependencyRefs in internal/storage/issueops read for a dependency
// end that names issues[index] of the same CreateIssues call. Their prefix
// constant is unexported, so the literal is repeated here.
const batchRefPrefix = "batch:"

func planRef(index int) string {
	return planRefPrefix + strconv.Itoa(index)
}

func planRefIndex(end string) (int, error) {
	digits, ok := strings.CutPrefix(end, planRefPrefix)
	if !ok {
		return 0, fmt.Errorf("dependency end %q is not a plan ref", end)
	}
	index, err := strconv.Atoi(digits)
	if err != nil || index < 0 {
		return 0, fmt.Errorf("dependency end %q is not a plan ref", end)
	}
	return index, nil
}

// planIssues returns the issues of cfg in creation order: epics, then features,
// then tasks, so every feature's epic and every task's feature comes earlier in
// the list. Labels are set on each issue. Every dependency names its ends with
// plan refs; a cross-link is held by the task it starts from.
//
// The draws from rng happen in the order the per-issue generator always made
// them, so a seed keeps its dataset. now is the clock for every timestamp.
func planIssues(cfg DataConfig, now time.Time) []*types.Issue {
	rng := rand.New(rand.NewSource(cfg.RandSeed)) // #nosec G404 -- deterministic math/rand used for repeatable fixture data

	numEpics := int(float64(cfg.TotalIssues) * cfg.EpicRatio)
	numFeatures := int(float64(cfg.TotalIssues) * cfg.FeatureRatio)
	numTasks := cfg.TotalIssues - numEpics - numFeatures

	issues := make([]*types.Issue, 0, cfg.TotalIssues)
	epicIdx := make([]int, 0, numEpics)
	featureIdx := make([]int, 0, numFeatures)
	taskIdx := make([]int, 0, numTasks)

	for i := 0; i < numEpics; i++ {
		title := epicTitles[i%len(epicTitles)]
		issue := drawIssue(rng, cfg, now, types.TypeEpic, cfg.MaxEpicAgeDays,
			fmt.Sprintf("%s (Epic %d)", title, i), fmt.Sprintf("Epic for %s", title))
		for j := 0; j < rng.Intn(3)+1; j++ {
			issue.Labels = append(issue.Labels, commonLabels[rng.Intn(len(commonLabels))])
		}
		epicIdx = append(epicIdx, len(issues))
		issues = append(issues, issue)
	}

	for i := 0; i < numFeatures; i++ {
		parent := issues[epicIdx[i%len(epicIdx)]]
		issue := drawIssue(rng, cfg, now, types.TypeFeature, cfg.MaxFeatureAgeDays,
			fmt.Sprintf("%s (Feature %d)", featureTitles[i%len(featureTitles)], i),
			fmt.Sprintf("Feature under %s", parent.Title))
		issue.Dependencies = append(issue.Dependencies, parentDependency(len(issues), epicIdx[i%len(epicIdx)], now))
		for j := 0; j < rng.Intn(3)+1; j++ {
			issue.Labels = append(issue.Labels, commonLabels[rng.Intn(len(commonLabels))])
		}
		featureIdx = append(featureIdx, len(issues))
		issues = append(issues, issue)
	}

	for i := 0; i < numTasks; i++ {
		parentIndex := featureIdx[i%len(featureIdx)]
		parent := issues[parentIndex]
		issue := drawIssue(rng, cfg, now, types.TypeTask, cfg.MaxTaskAgeDays,
			fmt.Sprintf("%s (Task %d)", taskTitles[i%len(taskTitles)], i),
			fmt.Sprintf("Task under %s", parent.Title))
		issue.Dependencies = append(issue.Dependencies, parentDependency(len(issues), parentIndex, now))
		for j := 0; j < rng.Intn(2)+1; j++ {
			issue.Labels = append(issue.Labels, commonLabels[rng.Intn(len(commonLabels))])
		}
		taskIdx = append(taskIdx, len(issues))
		issues = append(issues, issue)
	}

	// Cross-links between tasks. A link that repeats an edge or would close a
	// cycle is dropped here. The store refuses a cycle, and one refused edge
	// fails the whole batch of cross-links that AddDependencies writes.
	numCrossLinks := int(float64(numTasks) * cfg.CrossLinkRatio)
	blocks := newBlockGraph()
	for i := 0; i < numCrossLinks; i++ {
		from := taskIdx[rng.Intn(len(taskIdx))]
		to := taskIdx[rng.Intn(len(taskIdx))]
		if !blocks.add(from, to) {
			continue
		}
		issues[from].Dependencies = append(issues[from].Dependencies, &types.Dependency{
			IssueID:     planRef(from),
			DependsOnID: planRef(to),
			Type:        types.DepBlocks,
			CreatedAt:   now,
			CreatedBy:   "fixture",
		})
	}

	return issues
}

// drawIssue builds the fields every issue type shares. The draws happen in
// field order: status, priority, assignee, created time, then the closed time
// of a closed issue.
func drawIssue(rng *rand.Rand, cfg DataConfig, now time.Time, issueType types.IssueType, maxAgeDays int, title, description string) *types.Issue {
	issue := &types.Issue{
		Title:       title,
		Description: description,
		Status:      randomStatus(rng, cfg.OpenRatio),
		Priority:    randomPriority(rng),
		IssueType:   issueType,
		Assignee:    commonAssignees[rng.Intn(len(commonAssignees))],
		CreatedAt:   randomTime(rng, now, maxAgeDays),
		UpdatedAt:   now,
	}
	if issue.Status == types.StatusClosed {
		closedAt := randomTime(rng, now, cfg.MaxClosedAgeDays)
		issue.ClosedAt = &closedAt
	}
	return issue
}

func parentDependency(childIndex, parentIndex int, now time.Time) *types.Dependency {
	return &types.Dependency{
		IssueID:     planRef(childIndex),
		DependsOnID: planRef(parentIndex),
		Type:        types.DepParentChild,
		CreatedAt:   now,
		CreatedBy:   "fixture",
	}
}

// blockGraph holds the blocking edges planned so far, by issue index.
type blockGraph struct {
	edges map[int]map[int]bool
}

func newBlockGraph() *blockGraph {
	return &blockGraph{edges: make(map[int]map[int]bool)}
}

// add records that from depends on to. It refuses a self edge, an edge already
// present, and an edge that would close a cycle, and reports whether it added one.
func (g *blockGraph) add(from, to int) bool {
	if from == to || g.edges[from][to] || g.reaches(to, from) {
		return false
	}
	if g.edges[from] == nil {
		g.edges[from] = make(map[int]bool)
	}
	g.edges[from][to] = true
	return true
}

// reaches reports whether start depends on target through planned edges.
func (g *blockGraph) reaches(start, target int) bool {
	seen := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == target {
			return true
		}
		for next := range g.edges[node] {
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return false
}

// planChunks splits n issues into consecutive [start, end) ranges of at most
// size issues each.
func planChunks(n, size int) ([][2]int, error) {
	if size < 1 {
		return nil, fmt.Errorf("chunk size must be at least 1, got %d", size)
	}
	var ranges [][2]int
	for start := 0; start < n; start += size {
		ranges = append(ranges, [2]int{start, min(start+size, n)})
	}
	return ranges, nil
}

// localizeDependency returns a copy of dep for the chunk issues[start:end]. An
// end inside the chunk becomes a batch ref, and an end in an earlier chunk
// becomes that issue's ID, which the store has assigned by now. ok is false
// when an end names an issue at or after end: the dependency then waits for
// the final pass, once that issue exists.
func localizeDependency(issues []*types.Issue, dep *types.Dependency, start, end int) (*types.Dependency, bool, error) {
	local := *dep
	for _, field := range []*string{&local.IssueID, &local.DependsOnID} {
		index, err := planRefIndex(*field)
		if err != nil {
			return nil, false, err
		}
		switch {
		case index >= end:
			return nil, false, nil
		case index >= start:
			*field = batchRefPrefix + strconv.Itoa(index-start)
		default:
			if issues[index].ID == "" {
				return nil, false, fmt.Errorf("issue %d has no ID yet, but a dependency in a later chunk names it", index)
			}
			*field = issues[index].ID
		}
	}
	return &local, true, nil
}

// writePlanInChunks hands issues to write in consecutive chunks of at most size.
// Before each chunk is written, the dependencies of its issues are localized,
// so that the chunk never names an issue created in a later chunk. It returns
// the dependencies that wait for the final pass, still in plan refs.
func writePlanInChunks(issues []*types.Issue, size int, write func(chunk []*types.Issue) error) ([]*types.Dependency, error) {
	ranges, err := planChunks(len(issues), size)
	if err != nil {
		return nil, err
	}
	var deferred []*types.Dependency
	for _, r := range ranges {
		start, end := r[0], r[1]
		for _, issue := range issues[start:end] {
			var local []*types.Dependency
			for _, dep := range issue.Dependencies {
				ld, ok, err := localizeDependency(issues, dep, start, end)
				if err != nil {
					return nil, err
				}
				if !ok {
					deferred = append(deferred, dep)
					continue
				}
				local = append(local, ld)
			}
			issue.Dependencies = local
		}
		if err := write(issues[start:end]); err != nil {
			return nil, err
		}
	}
	return deferred, nil
}
