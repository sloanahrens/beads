package conformance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/samplesops"
)

// This file holds the contract every implementation of samplesops.Samples must
// satisfy. Each case asserts what samplesops/samples.go PROMISES rather than
// what any one backend happens to do today.
//
// THE VOTE COUNT. Two wirings — the server-backed store and the unit-of-work
// provider — and ONE body between them: both reach AddInTx / ListInTx in
// internal/storage/samplesops, the store through withRetryTx/withReadTx and the
// unit of work through its statement runner. So this is one reading plus an
// engine check, and the cases assert the ANSWER'S SHAPE (ordering, the exact
// payload text, the typed refusal) rather than message text, because what a
// per-leg failure would actually be here is a wrapper that loses the
// transaction or drops a request field.
//
// SEEDING DISCIPLINE: every case namespaces its kind with the fixture prefix
// and its own case slug. Kinds are workspace-global strings in one table, the
// fixture is built once per role, and several cases run in order against it, so
// an unscoped list would be an assertion about a sibling case's rows.
//
// WHAT THIS CONTRACT DELIBERATELY DOES NOT PIN: that two concurrent writers
// land distinct ids. id is a random UUID minted in Go and the table's primary
// key is where a collision would surface, which the engine guarantees; a
// concurrent case here would buy a flaky test rather than a contract.

// SamplesFixture supplies adapter-specific access for the samples assertions.
type SamplesFixture struct {
	// KindPrefix namespaces every kind the cases write, so several of them can
	// share one database.
	KindPrefix string
	Samples    samplesops.Samples
}

// samplesKind returns a workspace-unique kind for one case.
func samplesKind(fixture SamplesFixture, slug string) string {
	return fixture.KindPrefix + "-" + slug
}

// samplesAt returns a fixed event time offset from the epoch anchor the cases
// share, so ordering assertions do not depend on the wall clock.
func samplesAt(seconds int) time.Time {
	return time.Date(2026, time.November, 11, 9, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
}

func addSample(t *testing.T, ctx context.Context, fixture SamplesFixture, kind string, at time.Time, payload string) samplesops.Sample {
	t.Helper()
	result, err := fixture.Samples.Add(ctx, samplesops.AddRequest{Kind: kind, At: at, Payload: []byte(payload)})
	if err != nil {
		t.Fatalf("Add(%s, %s): %v", kind, at.Format(time.RFC3339), err)
	}
	return result.Sample
}

func listSamples(t *testing.T, ctx context.Context, fixture SamplesFixture, request samplesops.ListRequest) []samplesops.Sample {
	t.Helper()
	result, err := fixture.Samples.List(ctx, request)
	if err != nil {
		t.Fatalf("List(%+v): %v", request, err)
	}
	return result.Samples
}

// RunSamplesAddReturnsTheStoredRow pins the Add result's promise that the row
// it answers with is the row it stored.
func RunSamplesAddReturnsTheStoredRow(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	kind := samplesKind(fixture, "add-result")
	payload := "{\n  \"z\": 1,\n  \"a\": 2\n}"
	at := samplesAt(1)

	sample := addSample(t, ctx, fixture, kind, at, payload)

	if sample.ID == "" {
		t.Error("Add returned a sample with no id")
	}
	if sample.Kind != kind {
		t.Errorf("Add returned kind %q, want %q", sample.Kind, kind)
	}
	if !sample.At.Equal(at) {
		t.Errorf("Add returned at %s, want %s", sample.At.Format(time.RFC3339Nano), at.Format(time.RFC3339Nano))
	}
	if sample.At.Location() != time.UTC {
		t.Errorf("Add returned at in %s, want UTC", sample.At.Location())
	}
	if sample.Payload != payload {
		t.Errorf("Add returned payload %q, want the exact text stored (%q)", sample.Payload, payload)
	}
	if sample.CreatedAt.IsZero() {
		t.Error("Add returned a zero created_at")
	}
}

// RunSamplesListByKindIsNewestFirstAndScoped pins the role's two list promises
// at once: one kind's rows come back newest first, and another kind's rows are
// not in the answer.
func RunSamplesListByKindIsNewestFirstAndScoped(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	kind := samplesKind(fixture, "ordering")
	other := samplesKind(fixture, "ordering-other")
	addSample(t, ctx, fixture, kind, samplesAt(10), `{"row":"oldest-whitespace"}`)
	addSample(t, ctx, fixture, kind, samplesAt(30), `{"row":"newest"}`)
	addSample(t, ctx, fixture, kind, samplesAt(20), `{"row":"middle"}`)
	addSample(t, ctx, fixture, other, samplesAt(40), `{"row":"other-kind"}`)

	samples := listSamples(t, ctx, fixture, samplesops.ListRequest{Kind: kind})

	if len(samples) != 3 {
		t.Fatalf("List(%s) returned %d rows, want 3: %+v", kind, len(samples), samples)
	}
	got := []string{samples[0].Payload, samples[1].Payload, samples[2].Payload}
	want := []string{`{"row":"newest"}`, `{"row":"middle"}`, `{"row":"oldest-whitespace"}`}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d payload = %q, want %q (newest first)", i, got[i], want[i])
		}
	}
	if !samples[0].At.After(samples[1].At) || !samples[1].At.After(samples[2].At) {
		t.Errorf("at is not strictly descending: %s, %s, %s",
			samples[0].At, samples[1].At, samples[2].At)
	}
}

// RunSamplesListSinceIsInclusive pins ListRequest.Since's inclusive bound: a
// consumer re-reading from the last At it saw must not miss a row stamped at
// that instant.
func RunSamplesListSinceIsInclusive(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	kind := samplesKind(fixture, "since")
	addSample(t, ctx, fixture, kind, samplesAt(50), `{"row":"before"}`)
	addSample(t, ctx, fixture, kind, samplesAt(60), `{"row":"at"}`)
	addSample(t, ctx, fixture, kind, samplesAt(70), `{"row":"after"}`)

	since := samplesAt(60)
	samples := listSamples(t, ctx, fixture, samplesops.ListRequest{Kind: kind, Since: &since})

	if len(samples) != 2 {
		t.Fatalf("List(since=%s) returned %d rows, want 2: %+v", since.Format(time.RFC3339), len(samples), samples)
	}
	if samples[0].Payload != `{"row":"after"}` || samples[1].Payload != `{"row":"at"}` {
		t.Errorf("List(since) = [%s, %s], want the at-or-after rows newest first", samples[0].Payload, samples[1].Payload)
	}
}

// RunSamplesListLimitKeepsTheNewestRows pins Limit's meaning: it takes from the
// front of the newest-first order, not an arbitrary prefix of the table.
func RunSamplesListLimitKeepsTheNewestRows(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	kind := samplesKind(fixture, "limit")
	addSample(t, ctx, fixture, kind, samplesAt(80), `{"row":"old"}`)
	addSample(t, ctx, fixture, kind, samplesAt(90), `{"row":"new"}`)

	samples := listSamples(t, ctx, fixture, samplesops.ListRequest{Kind: kind, Limit: 1})

	if len(samples) != 1 {
		t.Fatalf("List(limit=1) returned %d rows, want 1", len(samples))
	}
	if samples[0].Payload != `{"row":"new"}` {
		t.Errorf("List(limit=1) = %q, want the newest row", samples[0].Payload)
	}
}

// RunSamplesListOfAKindWithNoRowsIsEmpty pins the empty answer: no error, an
// empty slice.
func RunSamplesListOfAKindWithNoRowsIsEmpty(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	samples := listSamples(t, ctx, fixture, samplesops.ListRequest{Kind: samplesKind(fixture, "never-written")})
	if len(samples) != 0 {
		t.Fatalf("List of an unwritten kind returned %d rows, want 0", len(samples))
	}
}

// RunSamplesAddRefusesAnInvalidRequest pins every refusal Add promises, and
// that a refusal writes nothing.
func RunSamplesAddRefusesAnInvalidRequest(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	kind := samplesKind(fixture, "refusals")
	oversize := strings.Repeat(" ", samplesops.MaxPayloadBytes+1)

	tests := []struct {
		name    string
		request samplesops.AddRequest
		want    string
	}{
		{name: "empty kind", request: samplesops.AddRequest{Kind: "", At: samplesAt(100), Payload: []byte(`{"ok":true}`)}, want: "kind"},
		{name: "zero at", request: samplesops.AddRequest{Kind: kind, Payload: []byte(`{"ok":true}`)}, want: "at"},
		{name: "empty payload", request: samplesops.AddRequest{Kind: kind, At: samplesAt(100), Payload: nil}, want: "payload"},
		{name: "oversize payload", request: samplesops.AddRequest{Kind: kind, At: samplesAt(100), Payload: []byte(oversize)}, want: "payload"},
		{name: "invalid JSON", request: samplesops.AddRequest{Kind: kind, At: samplesAt(100), Payload: []byte(`{"a":`)}, want: "payload"},
		{name: "non-object JSON", request: samplesops.AddRequest{Kind: kind, At: samplesAt(100), Payload: []byte(`[1,2]`)}, want: "payload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fixture.Samples.Add(ctx, tt.request)
			if err == nil {
				t.Fatalf("Add(%s) = nil error, want a refusal", tt.name)
			}
			if !errors.Is(err, samplesops.ErrValidation) {
				t.Fatalf("Add(%s) error = %v, want errors.Is(err, samplesops.ErrValidation)", tt.name, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Add(%s) error %q does not name %q", tt.name, err.Error(), tt.want)
			}
		})
	}

	if samples := listSamples(t, ctx, fixture, samplesops.ListRequest{Kind: kind}); len(samples) != 0 {
		t.Fatalf("a refused Add wrote %d rows, want none: %+v", len(samples), samples)
	}
}

// RunSamplesListRefusesANegativeLimit pins the one list refusal the role names.
func RunSamplesListRefusesANegativeLimit(t *testing.T, ctx context.Context, fixture SamplesFixture) {
	t.Helper()
	_, err := fixture.Samples.List(ctx, samplesops.ListRequest{Kind: samplesKind(fixture, "negative-limit"), Limit: -1})
	if err == nil {
		t.Fatal("List(limit=-1) = nil error, want a refusal")
	}
	if !errors.Is(err, samplesops.ErrValidation) {
		t.Fatalf("List(limit=-1) error = %v, want errors.Is(err, samplesops.ErrValidation)", err)
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("List(limit=-1) error %q does not name the limit", err.Error())
	}
}
