package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodedEnvelope mirrors the machine-mode envelope with every key required.
type decodedEnvelope struct {
	SchemaVersion   *int             `json:"schema_version"`
	ContractVersion *int             `json:"contract_version"`
	Data            json.RawMessage  `json:"data"`
	Pagination      *PaginationMeta  `json:"pagination"`
	Error           *decodedEnvError `json:"error"`
}

type decodedEnvError struct {
	Kind    string         `json:"kind"`
	Message string         `json:"message"`
	IDs     []idOutcome    `json:"ids"`
	Detail  map[string]any `json:"detail"`
}

// runMachine drives one simulated machine-mode command: fn plays the
// command body (it may stage data, print, record errors) and its return is
// what cobra would hand main. It returns stdout, stderr and the exit code.
func runMachine(t *testing.T, fn func() error) (decodedEnvelope, string, string, int) {
	t.Helper()
	withMachineMode(t, true)
	applyMachineCommandSetup() // what the root pre-run does: JSON on
	oldOut := machineOut
	machineOut = &machineOutput{done: make(chan struct{})}
	close(machineOut.done)
	t.Cleanup(func() { machineOut = oldOut })

	// Capture what the body prints to os.Stdout the way the pipe would.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	realStdout := os.Stdout
	os.Stdout = w
	cmdErr := fn()
	os.Stdout = realStdout
	_ = w.Close()
	var captured bytes.Buffer
	_, _ = captured.ReadFrom(r)

	var stdout, stderr bytes.Buffer
	code := writeMachineEnvelope(&stdout, &stderr, machineOut, captured.Bytes(), cmdErr)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout.String())
	}
	for _, k := range []string{"schema_version", "contract_version", "data", "pagination", "error"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("envelope missing key %q: %s", k, stdout.String())
		}
	}
	var env decodedEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if *env.SchemaVersion != JSONSchemaVersion || *env.ContractVersion != JSONContractVersion {
		t.Fatalf("versions = %d/%d", *env.SchemaVersion, *env.ContractVersion)
	}
	return env, stdout.String(), stderr.String(), code
}

func TestEnvelope_SuccessCarriesLegacyDataShape(t *testing.T) {
	env, _, _, code := runMachine(t, func() error {
		return outputJSON([]map[string]string{{"id": "bd-1"}})
	})
	if code != 0 || env.Error != nil {
		t.Fatalf("code=%d error=%+v", code, env.Error)
	}
	if string(bytes.TrimSpace(env.Data)) == "null" || !strings.Contains(string(env.Data), `"bd-1"`) {
		t.Fatalf("data = %s", env.Data)
	}
	if env.Pagination != nil {
		t.Fatalf("pagination on a non-paged read: %+v", env.Pagination)
	}
}

func TestEnvelope_ErrorHelpersNeverWriteStdout(t *testing.T) {
	env, stdout, _, code := runMachine(t, func() error {
		jsonOutput = true
		return HandleErrorRespectJSON("boom %d", 7)
	})
	if code != 1 || env.Error == nil || env.Error.Kind != string(kindInternal) || env.Error.Message != "boom 7" {
		t.Fatalf("code=%d error=%+v", code, env.Error)
	}
	if strings.Count(stdout, "schema_version") != 1 {
		t.Fatalf("stdout carries more than the envelope:\n%s", stdout)
	}
}

func TestEnvelope_ProseOnStdoutGoesToStderr(t *testing.T) {
	env, stdout, stderr, code := runMachine(t, func() error {
		fmt.Println("✓ Created issue bd-1")
		return outputJSON(map[string]string{"id": "bd-1"})
	})
	if code != 0 || env.Error != nil {
		t.Fatalf("code=%d err=%+v", code, env.Error)
	}
	if strings.Contains(stdout, "Created issue") || !strings.Contains(stderr, "Created issue") {
		t.Fatalf("prose not moved to stderr:\nstdout=%s\nstderr=%s", stdout, stderr)
	}
}

func TestEnvelope_UnmigratedJSONIsCapturedAsData(t *testing.T) {
	env, _, _, _ := runMachine(t, func() error {
		fmt.Println(`{"seq":1}`)
		fmt.Println(`{"seq":2}`)
		return nil
	})
	var rows []map[string]int
	if err := json.Unmarshal(env.Data, &rows); err != nil || len(rows) != 2 || rows[1]["seq"] != 2 {
		t.Fatalf("JSON lines not captured as an array: %s (%v)", env.Data, err)
	}
}

func TestEnvelope_PartialBatchExitsNonZeroWithIDs(t *testing.T) {
	env, _, _, code := runMachine(t, func() error {
		_ = outputJSON([]map[string]string{{"id": "bd-1"}})
		return batchError(1, []idOutcome{{ID: "bd-2", Kind: kindRefused, Message: "blocked by bd-3"}})
	})
	if code != 22 || env.Error == nil || env.Error.Kind != "partial" {
		t.Fatalf("code=%d error=%+v", code, env.Error)
	}
	if len(env.Error.IDs) != 1 || env.Error.IDs[0].ID != "bd-2" || env.Error.IDs[0].Kind != kindRefused {
		t.Fatalf("ids = %+v", env.Error.IDs)
	}
	if !strings.Contains(string(env.Data), "bd-1") {
		t.Fatalf("partial batch dropped the successes: %s", env.Data)
	}
}

func TestBatchErrorKinds(t *testing.T) {
	withMachineMode(t, true)
	cases := []struct {
		name      string
		succeeded int
		failures  []idOutcome
		want      errorKind
		code      int
	}{
		{"none failed", 2, nil, "", 0},
		{"all refused", 0, []idOutcome{{ID: "a", Kind: kindRefused}}, kindRefused, 21},
		{"all missing", 0, []idOutcome{{ID: "a", Kind: kindNotFound}, {ID: "b", Kind: kindNotFound}}, kindNotFound, 20},
		{"some ok", 1, []idOutcome{{ID: "a", Kind: kindNotFound}}, kindPartial, 22},
		{"guards keep 13", 1, []idOutcome{{ID: "a", Kind: kindGuardNotHeld}}, kindGuardNotHeld, 13},
		{"mixed takes first", 0, []idOutcome{{ID: "a", Kind: kindNotFound}, {ID: "b", Kind: kindRefused}}, kindNotFound, 20},
	}
	for _, tc := range cases {
		err := batchError(tc.succeeded, tc.failures)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: err = %v, want nil", tc.name, err)
			}
			continue
		}
		var ce *cliError
		if !errors.As(err, &ce) || ce.Kind != tc.want || ce.ExitCode() != tc.code {
			t.Errorf("%s: got %+v, want kind %s exit %d", tc.name, err, tc.want, tc.code)
		}
	}
}

func TestCLIErrorLegacyExitCodes(t *testing.T) {
	withMachineMode(t, false)
	if c := batchError(1, []idOutcome{{ID: "a", Kind: kindRefused}}).(*cliError).ExitCode(); c != 1 {
		t.Errorf("legacy partial exit = %d, want 1", c)
	}
	if c := (&cliError{Kind: kindGuardNotHeld}).ExitCode(); c != 13 {
		t.Errorf("legacy guard exit = %d, want 13", c)
	}
	if code, ok := exitCodeFromError(fmt.Errorf("wrapped: %w", &cliError{Kind: kindNotFound})); !ok || code != 1 {
		t.Errorf("exitCodeFromError(cliError) = %d, %v", code, ok)
	}
}

func TestEnvelope_ExitCodeTableIsTotalAndDistinct(t *testing.T) {
	seen := map[int]errorKind{}
	for _, k := range []errorKind{kindInternal, kindInvalidArgs, kindNotFound, kindRefused, kindPartial,
		kindTruncated, kindRouteUnreachable, kindStoreUnavailable, kindSchemaSkew, kindGuardNotHeld} {
		code, ok := errorKindExitCodes[k]
		if !ok {
			t.Fatalf("kind %s has no exit code", k)
		}
		if other, dup := seen[code]; dup {
			t.Fatalf("kinds %s and %s share exit %d", k, other, code)
		}
		seen[code] = k
	}
	if errorKindExitCodes[kindGuardNotHeld] != 13 {
		t.Fatal("guard_not_held must keep exit 13")
	}
}

func TestEnvelope_UntypedFailures(t *testing.T) {
	env, _, _, code := runMachine(t, func() error { return errors.New(`unknown command "shwo" for "bd"`) })
	if code != 27 || env.Error.Kind != "invalid_args" {
		t.Fatalf("unknown command: code=%d err=%+v", code, env.Error)
	}
	env, _, _, code = runMachine(t, func() error { return &exitError{Code: ExitGuardMismatch} })
	if code != 13 || env.Error.Kind != "guard_not_held" {
		t.Fatalf("exit 13: code=%d err=%+v", code, env.Error)
	}
	env, _, _, code = runMachine(t, func() error { return SilentExit() })
	if code != 1 || env.Error == nil || env.Error.Kind != "internal" {
		t.Fatalf("silent exit: code=%d err=%+v", code, env.Error)
	}
}

func TestEnvelope_StoreUnavailableAndSchemaSkew(t *testing.T) {
	env, _, _, code := runMachine(t, func() error {
		return failKind(kindStoreUnavailable, "failed to open database: %v", errors.New("connection refused"))
	})
	if code != 25 || env.Error.Kind != "store_unavailable" || !strings.Contains(env.Error.Message, "connection refused") {
		t.Fatalf("code=%d err=%+v", code, env.Error)
	}
}

func TestOutputJSONPage(t *testing.T) {
	rows := []string{"a", "b"}
	t.Run("default limit cut is truncated", func(t *testing.T) {
		env, _, _, code := runMachine(t, func() error {
			return outputJSONPage(rows, pageResult{HasMore: true, Returned: 2, Limit: 2})
		})
		if code != 23 || env.Error == nil || env.Error.Kind != "truncated" {
			t.Fatalf("code=%d err=%+v", code, env.Error)
		}
		if env.Pagination == nil || !env.Pagination.Truncated || env.Pagination.Returned != 2 {
			t.Fatalf("pagination = %+v", env.Pagination)
		}
		if !strings.Contains(string(env.Data), `"a"`) {
			t.Fatalf("truncated page dropped its rows: %s", env.Data)
		}
	})
	t.Run("explicit limit cut is a page", func(t *testing.T) {
		env, _, _, code := runMachine(t, func() error {
			return outputJSONPage(rows, pageResult{HasMore: true, Returned: 2, Limit: 2, LimitExplicit: true, NextCursor: "c1"})
		})
		if code != 0 || env.Error != nil || !env.Pagination.Truncated || env.Pagination.NextCursor != "c1" {
			t.Fatalf("code=%d err=%+v pag=%+v", code, env.Error, env.Pagination)
		}
	})
	t.Run("complete result still carries pagination", func(t *testing.T) {
		env, _, _, code := runMachine(t, func() error {
			return outputJSONPage(rows, pageResult{Returned: 2, Limit: 50})
		})
		if code != 0 || env.Pagination == nil || env.Pagination.Truncated || env.Pagination.NextCursor != "" {
			t.Fatalf("code=%d pag=%+v", code, env.Pagination)
		}
	})
}

// TestRouteUnreachableIsNotNotFound pins B1-05: a matched prefix route whose
// database cannot be asked is route_unreachable, never "not found".
func TestRouteUnreachableIsNotNotFound(t *testing.T) {
	town := t.TempDir()
	townBeads := filepath.Join(town, ".beads")
	rigBeads := filepath.Join(town, "om", "mayor", "rig", ".beads")
	for _, d := range []string{townBeads, rigBeads} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"),
		[]byte("{\"prefix\":\"hq-\",\"path\":\".\"}\n{\"prefix\":\"om-\",\"path\":\"om/mayor/rig\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The town's metadata names no dolt_database, so the hq- route cannot
	// be opened.
	if err := os.WriteFile(filepath.Join(townBeads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	oldDB := dbPath
	dbPath = filepath.Join(rigBeads, "embeddeddolt")
	t.Cleanup(func() { dbPath = oldDB })

	_, err := resolveViaPrefixRoutingWithAccess(context.Background(), "hq-abc", false)
	var re *routeUnreachableError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v (%T), want *routeUnreachableError", err, err)
	}
	if re.Prefix != "hq-" || re.ID != "hq-abc" {
		t.Fatalf("route error = %+v", re)
	}
	if isNotFoundErr(err) || errorKindOf(err) != kindRouteUnreachable {
		t.Fatalf("route failure classified as %s", errorKindOf(err))
	}

	// An id whose prefix has no route is still an ordinary miss.
	_, err = resolveViaPrefixRoutingWithAccess(context.Background(), "zz-abc", false)
	if errors.As(err, &re) {
		t.Fatalf("unrouted prefix reported as unreachable: %v", err)
	}
}

// TestUnknownIDIsNotFound pins be-2bc: comments add and dep add report an id
// that resolves to no issue as not_found (exit 20), the kind bd show uses,
// not internal. The errors are the chains those commands build.
func TestUnknownIDIsNotFound(t *testing.T) {
	resolverMiss := errors.New(`no issue found matching "gt-nosuch"`)
	cases := map[string]func() error{
		"comments add resolve error": func() error {
			return handleClassifiedRespectJSON(fmt.Errorf("resolving %s: %w", "gt-nosuch", resolverMiss))
		},
		"comments add nil result": func() error {
			return handleNotFoundRespectJSON("issue %s not found", "gt-nosuch")
		},
		"dep add source": func() error {
			return handleClassifiedRespectJSON(fmt.Errorf("resolving issue ID %s: %w", "gt-nosuch", resolverMiss))
		},
		"dep add target": func() error {
			return handleClassifiedRespectJSON(fmt.Errorf("resolving dependency ID %s: %w", "gt-nosuch",
				fmt.Errorf("resolving issue ID %s: %w", "gt-nosuch", resolverMiss)))
		},
	}
	for name, fn := range cases {
		env, _, _, code := runMachine(t, fn)
		if code != 20 || env.Error == nil || env.Error.Kind != "not_found" || !strings.Contains(env.Error.Message, "gt-nosuch") {
			t.Errorf("%s: code=%d err=%+v, want not_found (20)", name, code, env.Error)
		}
	}
}
