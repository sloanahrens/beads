//go:build cgo

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/samplesops"
)

// The store-backed round trip for bd sample lives in
// sample_integration_test.go. The tests here stay in the unit tier: every
// refusal happens inside the verb before it opens the samples role, and these
// tests pass the role and the --json source in as seams, so no package-level
// state steers the behaviour under test.

// sampleFake is the samples role a unit test injects. It records the request
// the verb built so the test can assert what the role was asked to do.
type sampleFake struct {
	addCalls    int
	addRequest  samplesops.AddRequest
	addResult   samplesops.AddResult
	addErr      error
	listCalls   int
	listRequest samplesops.ListRequest
	listResult  samplesops.ListResult
	listErr     error
}

func (f *sampleFake) Add(_ context.Context, request samplesops.AddRequest) (samplesops.AddResult, error) {
	f.addCalls++
	f.addRequest = request
	if f.addErr != nil {
		return samplesops.AddResult{}, f.addErr
	}
	return f.addResult, nil
}

func (f *sampleFake) List(_ context.Context, request samplesops.ListRequest) (samplesops.ListResult, error) {
	f.listCalls++
	f.listRequest = request
	if f.listErr != nil {
		return samplesops.ListResult{}, f.listErr
	}
	return f.listResult, nil
}

// openThatMustNotRun fails the test if the verb reaches the role: every
// refusal below is supposed to happen first.
func openThatMustNotRun(t *testing.T) sampleOpen {
	t.Helper()
	return func(string) (samplesops.Samples, error) {
		t.Helper()
		t.Fatal("the verb opened the samples role before refusing")
		return nil, nil
	}
}

func readPayloadReturning(payload []byte) sampleReadPayload {
	return func(string) ([]byte, error) { return payload, nil }
}

// writePayload writes payload under the test's temp dir and returns its path.
// It is shared with the integration tier, which drives the real --json flag.
func writePayload(t *testing.T, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return path
}

func TestSampleAddRefusesBadInputBeforeOpeningTheStore(t *testing.T) {
	oversize := append([]byte(`{"k":"`), make([]byte, samplesops.MaxPayloadBytes)...)
	oversize = append(oversize, []byte(`"}`)...)

	tests := []struct {
		name        string
		kind        string
		at          string
		payload     []byte
		payloadPath string
		want        []string
	}{
		{
			name:        "bad at",
			kind:        "usage_record",
			at:          "yesterday",
			payload:     []byte(`{"ok":true}`),
			payloadPath: "payload.json",
			want:        []string{"--at", "RFC3339"},
		},
		{
			name:        "missing at",
			kind:        "usage_record",
			at:          "",
			payload:     []byte(`{"ok":true}`),
			payloadPath: "payload.json",
			want:        []string{"--at", "required"},
		},
		{
			name:        "non-object payload",
			kind:        "usage_record",
			at:          "2026-11-11T09:30:00Z",
			payload:     []byte(`[1,2,3]`),
			payloadPath: "payload.json",
			want:        []string{"payload", "JSON object"},
		},
		{
			name:        "oversize payload",
			kind:        "usage_record",
			at:          "2026-11-11T09:30:00Z",
			payload:     oversize,
			payloadPath: "payload.json",
			want:        []string{"payload", "65536-byte cap"},
		},
		{
			name:        "empty kind",
			kind:        "",
			at:          "2026-11-11T09:30:00Z",
			payload:     []byte(`{"ok":true}`),
			payloadPath: "payload.json",
			want:        []string{"kind", "required"},
		},
		{
			name:        "empty payload source",
			kind:        "usage_record",
			at:          "2026-11-11T09:30:00Z",
			payload:     []byte(`{"ok":true}`),
			payloadPath: "",
			want:        []string{"--json", "required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &sampleFake{}
			open := openThatMustNotRun(t)
			_, err := sampleAdd(context.Background(), open, readPayloadReturning(tt.payload), tt.kind, tt.at, tt.payloadPath)
			if err == nil {
				t.Fatalf("sampleAdd accepted %s, want a refusal", tt.name)
			}
			if !errors.Is(err, samplesops.ErrValidation) {
				t.Fatalf("sampleAdd error = %v, want errors.Is(err, ErrValidation)", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err.Error(), want)
				}
			}
			if fake.addCalls != 0 {
				t.Errorf("refused add still called the role %d times", fake.addCalls)
			}
		})
	}
}

func TestSampleListRefusesBadInputBeforeOpeningTheStore(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		since string
		limit int
		want  []string
	}{
		{
			name:  "bad since",
			kind:  "usage_record",
			since: "2026-11-11 09:30",
			want:  []string{"--since", "RFC3339"},
		},
		{
			name:  "negative limit",
			kind:  "usage_record",
			limit: -1,
			want:  []string{"limit", "negative"},
		},
		{
			name: "empty kind",
			kind: "",
			want: []string{"kind", "required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &sampleFake{}
			open := openThatMustNotRun(t)
			_, err := sampleList(context.Background(), open, tt.kind, tt.since, tt.limit)
			if err == nil {
				t.Fatalf("sampleList accepted %s, want a refusal", tt.name)
			}
			if !errors.Is(err, samplesops.ErrValidation) {
				t.Fatalf("sampleList error = %v, want errors.Is(err, ErrValidation)", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err.Error(), want)
				}
			}
			if fake.listCalls != 0 {
				t.Errorf("refused list still called the role %d times", fake.listCalls)
			}
		})
	}
}

func TestSampleAddPassesTheParsedRequestToTheRole(t *testing.T) {
	fake := &sampleFake{addResult: samplesops.AddResult{Sample: samplesops.Sample{ID: "smp-1"}}}
	open := func(requirement string) (samplesops.Samples, error) {
		if requirement == "" {
			t.Error("the verb opened the role with an empty direct-access requirement")
		}
		return fake, nil
	}
	payload := []byte(`{"used":1200}`)

	sample, err := sampleAdd(context.Background(), open, readPayloadReturning(payload), "usage_record", "2026-11-11T09:30:00+02:00", "payload.json")
	if err != nil {
		t.Fatalf("sampleAdd: %v", err)
	}
	if sample.ID != "smp-1" {
		t.Errorf("returned sample id = %q, want the role's row", sample.ID)
	}
	if got, want := fake.addRequest.Kind, "usage_record"; got != want {
		t.Errorf("role kind = %q, want %q", got, want)
	}
	if got, want := fake.addRequest.At.Format(time.RFC3339), "2026-11-11T07:30:00Z"; got != want {
		t.Errorf("role at = %s, want the UTC-normalized %s", got, want)
	}
	if got := string(fake.addRequest.Payload); got != string(payload) {
		t.Errorf("role payload = %q, want the bytes as given", got)
	}
}

func TestSampleListPassesTheParsedRequestToTheRole(t *testing.T) {
	fake := &sampleFake{listResult: samplesops.ListResult{Samples: []samplesops.Sample{{ID: "smp-1"}}}}
	open := func(string) (samplesops.Samples, error) { return fake, nil }

	samples, err := sampleList(context.Background(), open, "usage_record", "2026-11-11T00:00:00Z", 5)
	if err != nil {
		t.Fatalf("sampleList: %v", err)
	}
	if len(samples) != 1 || samples[0].ID != "smp-1" {
		t.Errorf("sampleList returned %#v, want the role's rows", samples)
	}
	if fake.listRequest.Since == nil {
		t.Fatal("role since = nil, want the parsed --since")
	}
	if got, want := fake.listRequest.Since.Format(time.RFC3339), "2026-11-11T00:00:00Z"; got != want {
		t.Errorf("role since = %s, want %s", got, want)
	}
	if got, want := fake.listRequest.Limit, 5; got != want {
		t.Errorf("role limit = %d, want %d", got, want)
	}
}

// TestSampleAddReadsAtMostOneBytePastTheCap pins the seam that keeps an
// oversize --json file from being buffered whole: the reader stops one byte
// past the cap, which is exactly what validation needs to refuse it.
func TestSampleAddReadsAtMostOneBytePastTheCap(t *testing.T) {
	source := bytes.Repeat([]byte("x"), 4*samplesops.MaxPayloadBytes)
	payload, err := readSamplePayloadFrom(bytes.NewReader(source))
	if err != nil {
		t.Fatalf("readSamplePayloadFrom: %v", err)
	}
	if got, want := len(payload), samplesops.MaxPayloadBytes+1; got != want {
		t.Fatalf("read %d bytes from an oversize source, want %d", got, want)
	}
}

func TestSampleAddReportsASourceReadFailureSeparately(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errors.New("open payload: permission denied") }
	_, err := sampleAdd(context.Background(), openThatMustNotRun(t), read, "usage_record", "2026-11-11T09:30:00Z", "payload.json")

	var payloadErr *samplePayloadError
	if !errors.As(err, &payloadErr) {
		t.Fatalf("error = %v, want a samplePayloadError so the verb prints the read diagnostic", err)
	}
}

func TestSampleAddReportsAnOpenFailureSeparately(t *testing.T) {
	open := func(string) (samplesops.Samples, error) { return nil, errors.New("no workspace") }
	_, err := sampleAdd(context.Background(), open, readPayloadReturning([]byte(`{"ok":true}`)), "usage_record", "2026-11-11T09:30:00Z", "payload.json")

	var openErr *sampleOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("error = %v, want a sampleOpenError so the verb prints the open diagnostic", err)
	}
}

func TestSampleAddReturnsAStoreFailureAsItIs(t *testing.T) {
	storeFailure := errors.New("connection reset by peer")
	fake := &sampleFake{addErr: storeFailure}
	open := func(string) (samplesops.Samples, error) { return fake, nil }

	_, err := sampleAdd(context.Background(), open, readPayloadReturning([]byte(`{"ok":true}`)), "usage_record", "2026-11-11T09:30:00Z", "payload.json")
	if !errors.Is(err, storeFailure) {
		t.Fatalf("error = %v, want the store failure", err)
	}
	if errors.Is(err, samplesops.ErrValidation) {
		t.Error("a store failure was classified as a validation refusal")
	}
}

// TestSampleCommandDocumentsBothVerbs pins the CLI half of the docs box: the
// parent help lists add and list, and each documents its flags. The docs page
// is generated from this text, so a missing sentence here is a missing page.
func TestSampleCommandDocumentsBothVerbs(t *testing.T) {
	wanted := map[string][]string{
		"add":  {"--kind", "--at", "--json", "RFC3339", "exact bytes given"},
		"list": {"--kind", "--since", "--limit", "newest first", "compact JSON"},
	}
	found := map[string]bool{}
	for _, sub := range sampleCmd.Commands() {
		help, ok := wanted[sub.Name()]
		if !ok {
			continue
		}
		found[sub.Name()] = true
		for _, want := range help {
			if !strings.Contains(sub.Long, want) {
				t.Errorf("bd sample %s help does not mention %q:\n%s", sub.Name(), want, sub.Long)
			}
		}
	}
	for name := range wanted {
		if !found[name] {
			t.Errorf("bd sample has no %q subcommand", name)
		}
	}
}

// TestSampleAddJsonFlagIsThePayloadNotMachineOutput pins the deliberate
// shadowing: add takes --json <file|->, so the inherited bool must not answer
// for it. list keeps the inherited bool, so it declares no local flag.
//
// LocalFlags is what separates the two. Flags also reports the flags inherited
// from the root command once any sibling test has executed the tree, so on
// list it answers with root's bool --json as if list had declared it.
func TestSampleAddJsonFlagIsThePayloadNotMachineOutput(t *testing.T) {
	flag := sampleAddCmd.LocalFlags().Lookup("json")
	if flag == nil {
		t.Fatal("bd sample add has no local --json flag")
	}
	if got := flag.Value.Type(); got != "string" {
		t.Errorf("bd sample add --json is a %s, want a string payload source", got)
	}
	if flag := sampleListCmd.LocalFlags().Lookup("json"); flag != nil {
		t.Errorf("bd sample list declares a local --json flag (%s); it must take the inherited output flag", flag.Value.Type())
	}
}
