//go:build integration
// +build integration

package beads_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

// Hash IDs are derived from the issue's content (title, description, creator,
// creation time) and a nonce, and the mint loop only retries on a collision
// inside one database. These tests check that invariant on Dolt-backed stores:
// nothing is shared between clones through git, so there is no sync to test.

// hashIDPattern is the id space and length rule for generated hash ids.
var hashIDPattern = regexp.MustCompile(`^test-[0-9a-z]{3,8}$`)

// hashIDTestTime is a fixed creation time, so two creations share it unless a
// test changes it on purpose.
var hashIDTestTime = time.Date(2026, time.October, 30, 9, 0, 0, 0, time.UTC)

// allowNewDatabase is what bd init asks of scripts that run it against a shared
// test server already serving other databases (store.go: REFUSED: bd init would
// create a new database).
var allowNewDatabase = map[string]string{"BD_ALLOW_NEW_DATABASE": "1"}

// getBDPath returns the test bd binary path
func getBDPath() string {
	if testBDBinary := os.Getenv("BEADS_TEST_BD_BINARY"); testBDBinary != "" {
		return testBDBinary
	}
	// Fallback for non-TestMain runs
	return "./bd"
}

func hasDoltTestPort() bool {
	return os.Getenv("BEADS_DOLT_PORT") != ""
}

func requireHashIDIntegration(t *testing.T) string {
	t.Helper()
	if !hasDoltTestPort() {
		t.Skip("skipping: Dolt test container not available")
	}
	bdPath, err := filepath.Abs(getBDPath())
	if err != nil {
		t.Fatalf("resolve bd binary path: %v", err)
	}
	if _, err := os.Stat(bdPath); err != nil {
		t.Fatalf("bd binary not found at %s", bdPath)
	}
	return bdPath
}

// openHashIDStore opens an empty Dolt database at path. Under the test server
// (BEADS_TEST_MODE=1) dolt.New names the database after a hash of path, so
// two paths are two independent databases.
func openHashIDStore(t *testing.T, path string) *dolt.DoltStore {
	t.Helper()
	ctx := context.Background()
	store, err := dolt.New(ctx, &dolt.Config{Path: path, CreateIfMissing: true})
	if err != nil {
		t.Fatalf("dolt.New(%s) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// CRITICAL (bd-166): Set issue_prefix to prevent "database not initialized" errors
	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set issue_prefix: %v", err)
	}
	return store
}

// createHashIDIssue creates one issue and returns the id the store minted for it.
// It returns an error rather than failing the test, so goroutines can call it.
func createHashIDIssue(store *dolt.DoltStore, title, description, actor string, createdAt time.Time) (string, error) {
	issue := &types.Issue{
		Title:       title,
		Description: description,
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeTask,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := store.CreateIssue(context.Background(), issue, actor); err != nil {
		return "", err
	}
	return issue.ID, nil
}

// TestHashIDs_IdenticalContentGivesTheSameID verifies that the same content,
// creator and creation time mint the same id in two independent databases, and
// that changing only the creation time or only the title changes the id.
func TestHashIDs_IdenticalContentGivesTheSameID(t *testing.T) {
	testutil.RequireDoltContainer(t)
	t.Parallel()
	root := testutil.TempDirInMemory(t)

	first := openHashIDStore(t, filepath.Join(root, "first"))
	second := openHashIDStore(t, filepath.Join(root, "second"))

	idFirst, err := createHashIDIssue(first, "Same title", "Same description", "alice", hashIDTestTime)
	if err != nil {
		t.Fatalf("create in first database: %v", err)
	}
	idSecond, err := createHashIDIssue(second, "Same title", "Same description", "alice", hashIDTestTime)
	if err != nil {
		t.Fatalf("create in second database: %v", err)
	}
	if idFirst != idSecond {
		t.Fatalf("identical content gave different ids: %q and %q; want the same id", idFirst, idSecond)
	}

	t.Run("CreatedAtChangesTheID", func(t *testing.T) {
		store := openHashIDStore(t, filepath.Join(root, "later"))
		id, err := createHashIDIssue(store, "Same title", "Same description", "alice", hashIDTestTime.Add(time.Second))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if id == idFirst {
			t.Errorf("same content at a different creation time gave id %q; want a different id", id)
		}
	})

	t.Run("TitleChangesTheID", func(t *testing.T) {
		store := openHashIDStore(t, filepath.Join(root, "retitled"))
		id, err := createHashIDIssue(store, "Other title", "Same description", "alice", hashIDTestTime)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if id == idFirst {
			t.Errorf("different title at the same creation time gave id %q; want a different id", id)
		}
	})
}

// TestHashIDs_IndependentDatabasesDoNotCollide creates three different issues
// concurrently, one per independent database, and expects three distinct ids
// that each fit the id space and length rule.
func TestHashIDs_IndependentDatabasesDoNotCollide(t *testing.T) {
	testutil.RequireDoltContainer(t)
	t.Parallel()
	root := testutil.TempDirInMemory(t)

	titles := []string{"Writer one", "Writer two", "Writer three"}
	stores := make([]*dolt.DoltStore, len(titles))
	for i := range titles {
		stores[i] = openHashIDStore(t, filepath.Join(root, fmt.Sprintf("writer-%d", i)))
	}

	ids := make([]string, len(titles))
	errs := make([]error, len(titles))
	var wg sync.WaitGroup
	for i := range titles {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = createHashIDIssue(stores[i], titles[i], "", "alice", hashIDTestTime)
		}(i)
	}
	wg.Wait()

	seen := make(map[string]int, len(ids))
	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("writer %d create failed: %v", i, errs[i])
		}
		if !hashIDPattern.MatchString(id) {
			t.Errorf("writer %d id %q does not match %s", i, id, hashIDPattern)
		}
		if j, dup := seen[id]; dup {
			t.Errorf("writers %d and %d minted the same id %q", j, i, id)
		}
		seen[id] = i
	}
}

// TestHashIDs_ReimportOfIdenticalContentIsADedupe exports one issue from a bd
// workspace and imports the same file twice into a fresh workspace. Each import
// must leave exactly one issue, with the exported id.
func TestHashIDs_ReimportOfIdenticalContentIsADedupe(t *testing.T) {
	bdPath := requireHashIDIntegration(t)
	t.Parallel()
	root := testutil.TempDirInMemory(t)

	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	exportFile := filepath.Join(root, "issues.jsonl")
	for _, dir := range []string{source, target} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create workspace %s: %v", dir, err)
		}
		runHashIDBD(t, bdPath, dir, allowNewDatabase, "init", "--quiet", "--prefix", "test", "--database", hashIDDatabaseName(dir))
	}

	id := createHashIDBDIssue(t, bdPath, source, "Reimport me")
	runHashIDBD(t, bdPath, source, nil, "export", "-o", exportFile)

	for i := 1; i <= 2; i++ {
		runHashIDBD(t, bdPath, target, nil, "import", "-i", exportFile)

		ids := listHashIDBDIssues(t, bdPath, target)
		if len(ids) != 1 || ids[0] != id {
			t.Fatalf("after import %d: want exactly one issue %q, got %v", i, id, ids)
		}
	}
}

// hashIDDatabaseName returns the Dolt database name for a workspace at dir. Both
// workspaces of the re-import test use the prefix "test", which bd init turns
// into the same database name, so each directory gets its own name instead.
// It hashes the path the way openHashIDStore's database names do.
func hashIDDatabaseName(dir string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(dir)) // hash.Hash.Write never returns an error
	return fmt.Sprintf("hashids_%x", h.Sum64())
}

// TestHashIDDatabaseName checks that two workspace directories get two database
// names, and that one directory always gets the same name.
func TestHashIDDatabaseName(t *testing.T) {
	source := hashIDDatabaseName("/work/source")
	target := hashIDDatabaseName("/work/target")
	if source == target {
		t.Errorf("two directories share database name %q; want distinct names", source)
	}
	if again := hashIDDatabaseName("/work/source"); again != source {
		t.Errorf("same directory gave database name %q then %q; want one name", source, again)
	}
}

// runHashIDBD runs the bd binary in dir and fails the test on a non-zero exit.
// env is added to the inherited environment.
func runHashIDBD(t *testing.T, bdPath, dir string, env map[string]string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bdPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd %v failed in %s: %v\nOutput: %s", args, dir, err, out)
	}
	return string(out)
}

// createHashIDBDIssue creates one issue through bd and returns its id.
func createHashIDBDIssue(t *testing.T, bdPath, dir, title string) string {
	t.Helper()
	out := runHashIDBD(t, bdPath, dir, nil, "create", title, "-t", "task", "-p", "1", "--json")
	start := strings.Index(out, "{")
	if start == -1 {
		t.Fatalf("bd create --json printed no JSON object: %s", out)
	}
	id := hashIDFromCreateReply(out[start:])
	if id == "" {
		t.Fatalf("bd create --json returned no id: %s", out)
	}
	return id
}

// hashIDFromCreateReply returns the id in a bd create --json reply, or "" if
// there is none. Older bd prints the id at the top level; the plain envelope
// and the five-key contract document print it under "data". Input that is not
// a JSON object gives "".
func hashIDFromCreateReply(reply string) string {
	var top struct {
		ID   string          `json:"id"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(reply), &top); err != nil {
		return ""
	}
	if top.ID != "" {
		return top.ID
	}
	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(top.Data, &data); err != nil {
		return ""
	}
	return data.ID
}

// TestHashIDFromCreateReply covers each reply shape bd create --json has printed.
func TestHashIDFromCreateReply(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{
			name:  "top-level id",
			reply: `{"id":"test-abc","title":"Reimport me"}`,
			want:  "test-abc",
		},
		{
			name:  "plain envelope",
			reply: `{"data":{"id":"test-3xu","title":"Reimport me"},"schema_version":1}`,
			want:  "test-3xu",
		},
		{
			name:  "five-key contract document",
			reply: `{"schema_version":1,"ok":true,"data":{"id":"test-k7q"},"error":null,"warnings":[]}`,
			want:  "test-k7q",
		},
		{
			name:  "empty object",
			reply: `{}`,
			want:  "",
		},
		{
			name:  "malformed JSON",
			reply: `{"data":{"id":"test-x`,
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hashIDFromCreateReply(tt.reply); got != tt.want {
				t.Errorf("hashIDFromCreateReply(%s) = %q; want %q", tt.reply, got, tt.want)
			}
		})
	}
}

// listHashIDBDIssues returns the ids of the issues bd lists in dir.
func listHashIDBDIssues(t *testing.T, bdPath, dir string) []string {
	t.Helper()
	out := runHashIDBD(t, bdPath, dir, map[string]string{"BD_NO_AUTO_IMPORT": "1"}, "list", "--json")
	start := strings.Index(out, "[")
	if start == -1 {
		t.Fatalf("bd list --json printed no JSON array: %s", out)
	}
	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out[start:]), &issues); err != nil {
		t.Fatalf("parse bd list output: %v\nOutput: %s", err, out)
	}
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}
