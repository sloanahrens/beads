//go:build cgo && integration

package embeddeddolt_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/types"
)

func TestPristineEmbeddedDoltFixtureRelocatesPrefixes(t *testing.T) {
	template := pristineEmbeddedDoltTemplateForTest(t)
	templateDigest := directoryDigest(t, template.beadsDir)

	fast := newPristineEmbeddedDoltFixture(t, "test")
	if got := directoryDigest(t, fast.beadsDir); got != templateDigest {
		t.Fatalf("test-prefix clone digest = %s, want pristine template %s", got, templateDigest)
	}
	closeEmbeddedDoltStore(t, fast.store)

	alpha := newPristineEmbeddedDoltFixture(t, "alpha")
	beta := newPristineEmbeddedDoltFixture(t, "beta")
	for _, fixture := range []*pristineEmbeddedDoltFixture{alpha, beta} {
		if _, err := os.Stat(filepath.Join(fixture.dataDir, fixture.database)); err != nil {
			t.Fatalf("%s physical database directory: %v", fixture.database, err)
		}
		if _, err := os.Stat(filepath.Join(fixture.dataDir, pristineEmbeddedDoltDatabase)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s clone still contains baseline database directory: %v", fixture.database, err)
		}
		prefix, err := fixture.store.GetConfig(t.Context(), "issue_prefix")
		if err != nil || prefix != fixture.database {
			t.Fatalf("%s issue_prefix = %q, %v; want %q", fixture.database, prefix, err, fixture.database)
		}
		db, cleanup, err := embeddeddolt.OpenSQL(t.Context(), fixture.dataDir, fixture.database, "main")
		if err != nil {
			t.Fatalf("open raw SQL for %s: %v", fixture.database, err)
		}
		var rawPrefix string
		err = db.QueryRowContext(t.Context(), "SELECT value FROM config WHERE `key` = ?", "issue_prefix").Scan(&rawPrefix)
		if cleanupErr := cleanup(); cleanupErr != nil && err == nil {
			err = cleanupErr
		}
		if err != nil || rawPrefix != fixture.database {
			t.Fatalf("raw issue_prefix for %s = %q, %v; want %q", fixture.database, rawPrefix, err, fixture.database)
		}
	}

	issue := &types.Issue{Title: "only alpha has this", Status: types.StatusOpen, IssueType: types.TypeTask}
	if err := alpha.store.CreateIssue(t.Context(), issue, "test"); err != nil {
		t.Fatalf("create alpha issue: %v", err)
	}
	if !strings.HasPrefix(issue.ID, "alpha-") {
		t.Fatalf("generated alpha issue ID = %q, want alpha prefix", issue.ID)
	}
	if err := alpha.store.Commit(t.Context(), "commit alpha issue"); err != nil {
		t.Fatalf("commit alpha issue: %v", err)
	}
	if _, err := beta.store.GetIssue(t.Context(), issue.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("beta GetIssue(%q) error = %v, want ErrNotFound", issue.ID, err)
	}
	closeEmbeddedDoltStore(t, alpha.store)
	closeEmbeddedDoltStore(t, beta.store)

	alphaReopened, err := embeddeddolt.Open(context.Background(), alpha.beadsDir, "alpha", "main")
	if err != nil {
		t.Fatalf("reopen alpha: %v", err)
	}
	if _, err := alphaReopened.GetIssue(t.Context(), issue.ID); err != nil {
		t.Fatalf("reopened alpha GetIssue(%q): %v", issue.ID, err)
	}
	closeEmbeddedDoltStore(t, alphaReopened)

	betaReopened, err := embeddeddolt.Open(context.Background(), beta.beadsDir, "beta", "main")
	if err != nil {
		t.Fatalf("reopen beta: %v", err)
	}
	if _, err := betaReopened.GetIssue(t.Context(), issue.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("reopened beta GetIssue(%q) error = %v, want ErrNotFound", issue.ID, err)
	}
	closeEmbeddedDoltStore(t, betaReopened)

	if got := directoryDigest(t, template.beadsDir); got != templateDigest {
		t.Fatalf("template digest changed after relocated clone mutations: got %s, want %s", got, templateDigest)
	}

	reusedDestination := filepath.Join(t.TempDir(), ".beads")
	if err := clonePristineEmbeddedDoltTemplate(template, reusedDestination); err != nil {
		t.Fatalf("first clone into reusable destination: %v", err)
	}
	if err := clonePristineEmbeddedDoltTemplate(template, reusedDestination); err == nil {
		t.Fatal("clone into an existing destination succeeded")
	}
}
