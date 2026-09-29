package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/schema"
)

// JSONContractVersion versions the machine-mode envelope and the data shapes
// inside it. Bump it on any breaking change to either; callers refuse to run
// against a contract version they do not know. See
// engdocs/design/d1-machine-surface.md.
const JSONContractVersion = 1

// errorKind is the typed failure class a machine caller branches on.
type errorKind string

const (
	kindInternal         errorKind = "internal"
	kindInvalidArgs      errorKind = "invalid_args"
	kindNotFound         errorKind = "not_found"
	kindRefused          errorKind = "refused"
	kindPartial          errorKind = "partial"
	kindTruncated        errorKind = "truncated"
	kindRouteUnreachable errorKind = "route_unreachable"
	kindStoreUnavailable errorKind = "store_unavailable"
	kindSchemaSkew       errorKind = "schema_skew"
	kindGuardNotHeld     errorKind = "guard_not_held"
)

// errorKindExitCodes is the machine-mode exit code for each kind. internal
// exits 1 unless the command set its own code. guard_not_held keeps the 13
// bd update has always used.
var errorKindExitCodes = map[errorKind]int{
	kindInternal:         1,
	kindGuardNotHeld:     ExitGuardMismatch,
	kindNotFound:         20,
	kindRefused:          21,
	kindPartial:          22,
	kindTruncated:        23,
	kindRouteUnreachable: 24,
	kindStoreUnavailable: 25,
	kindSchemaSkew:       26,
	kindInvalidArgs:      27,
}

func kindExitCode(k errorKind) int {
	if code, ok := errorKindExitCodes[k]; ok {
		return code
	}
	return 1
}

// idOutcome is one id's failure inside a batch.
type idOutcome struct {
	ID      string    `json:"id"`
	Kind    errorKind `json:"kind"`
	Message string    `json:"message"`
}

// envelopeError is the error member of the envelope.
type envelopeError struct {
	Kind    errorKind      `json:"kind"`
	Message string         `json:"message"`
	IDs     []idOutcome    `json:"ids,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// envelope is the one JSON document machine mode writes to stdout. Every key
// is always present so a caller can decode it into a fixed struct.
type envelope struct {
	SchemaVersion   int             `json:"schema_version"`
	ContractVersion int             `json:"contract_version"`
	Data            any             `json:"data"`
	Pagination      *PaginationMeta `json:"pagination"`
	Error           *envelopeError  `json:"error"`
}

// cliError is a typed command failure. In machine mode its kind picks the
// exit code; outside it the exit is 1, or 13 for guard_not_held.
// Whoever builds one outside machine mode has already told the user what
// went wrong on stderr, so main exits with the code and prints nothing more.
type cliError struct {
	Kind    errorKind
	Message string
	IDs     []idOutcome
	Detail  map[string]any
}

func (e *cliError) Error() string { return e.Message }

// ExitCode is the process exit status for this error in the current mode.
func (e *cliError) ExitCode() int {
	if machineModeActive() {
		return kindExitCode(e.Kind)
	}
	if e.Kind == kindGuardNotHeld {
		return ExitGuardMismatch
	}
	return 1
}

func (e *cliError) envelopeError() *envelopeError {
	return &envelopeError{Kind: e.Kind, Message: e.Message, IDs: e.IDs, Detail: e.Detail}
}

func newCLIError(kind errorKind, format string, args ...any) *cliError {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &cliError{Kind: kind, Message: msg}
}

// failKind reports a typed failure. Outside machine mode it behaves exactly
// like HandleError (text on stderr, exit 1), so legacy callers see no change;
// in machine mode the message goes into the envelope and the kind sets the
// exit code.
func failKind(kind errorKind, format string, args ...any) error {
	e := newCLIError(kind, format, args...)
	if !machineModeActive() {
		fmt.Fprintf(os.Stderr, "Error: %s\n", e.Message)
	}
	return e
}

// handleClassifiedRespectJSON reports a resolver or store error. Outside
// machine mode it is HandleErrorRespectJSON("%v", err), unchanged; in machine
// mode the error keeps its kind (not_found, route_unreachable, ...).
func handleClassifiedRespectJSON(err error) error {
	if machineModeActive() {
		return &cliError{Kind: errorKindOf(err), Message: err.Error()}
	}
	return HandleErrorRespectJSON("%v", err)
}

// handleClassified is handleClassifiedRespectJSON for call sites that use
// HandleError (text on stderr even under --json) outside machine mode.
func handleClassified(err error) error {
	if machineModeActive() {
		return &cliError{Kind: errorKindOf(err), Message: err.Error()}
	}
	return HandleError("%v", err)
}

// errorKindOf classifies an error returned by a store or resolver.
func errorKindOf(err error) errorKind {
	var ce *cliError
	var re *routeUnreachableError
	var skew *schema.SchemaSkewError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &ce):
		return ce.Kind
	case errors.As(err, &re):
		return kindRouteUnreachable
	case errors.As(err, &skew):
		return kindSchemaSkew
	case isGuardMismatch(err):
		return kindGuardNotHeld
	case isNotFoundErr(err):
		return kindNotFound
	case errors.Is(err, storage.ErrValidation):
		return kindInvalidArgs
	case errors.Is(err, storage.ErrAlreadyClaimed), errors.Is(err, storage.ErrNotClaimable),
		errors.Is(err, storage.ErrCloseBlocked), errors.Is(err, storage.ErrCloseOpenChildren),
		errors.Is(err, storage.ErrAlreadyExists), errors.Is(err, storage.ErrPrefixMismatch):
		return kindRefused
	}
	return kindInternal
}

// batchError turns a batch's per-id failures into one error, or nil when
// nothing failed. Every failure a guard refusal keeps guard_not_held (the
// 13 contract bd update has always had); any success makes it partial;
// otherwise all-one-kind reports that kind and mixed failures report the
// first failure's kind. Outside machine mode it exits 1 (13 when every
// failure is a guard refusal), the rule bd update has always had.
func batchError(succeeded int, failures []idOutcome) error {
	if len(failures) == 0 {
		return nil
	}
	kind := failures[0].Kind
	allGuard := true
	for _, f := range failures {
		if f.Kind != kindGuardNotHeld {
			allGuard = false
		}
	}
	switch {
	case allGuard:
		kind = kindGuardNotHeld
	case succeeded > 0:
		kind = kindPartial
	}
	msg := fmt.Sprintf("%d of %d ids failed", len(failures), succeeded+len(failures))
	return &cliError{Kind: kind, Message: msg, IDs: failures}
}

// ---------------------------------------------------------------------------
// Machine-mode output: staging, stdout capture, and the one writer.
// ---------------------------------------------------------------------------

// machineOutput holds what a command produced until main writes the envelope.
type machineOutput struct {
	mu         sync.Mutex
	realStdout *os.File
	pipeW      *os.File
	captured   bytes.Buffer
	done       chan struct{}
	staged     []any
	pagination *PaginationMeta
	recorded   *cliError
}

var machineOut *machineOutput

// machineDrainTimeout bounds the wait for the stdout pipe to drain after
// the command returns. A detached child that inherited stdout can hold the
// pipe open forever; the envelope must not wait on it.
const machineDrainTimeout = 2 * time.Second

// beginMachineCapture points os.Stdout at a pipe so nothing a command prints
// reaches the caller's stdout except the envelope. Idempotent.
func beginMachineCapture() {
	if machineOut != nil {
		return
	}
	out := &machineOutput{realStdout: os.Stdout, done: make(chan struct{})}
	r, w, err := os.Pipe()
	if err != nil {
		// Without a pipe nothing is captured, but the envelope is still the
		// only thing the staging path writes.
		close(out.done)
		machineOut = out
		return
	}
	out.pipeW = w
	os.Stdout = w
	go func() {
		defer close(out.done)
		buf := make([]byte, 32*1024)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				out.mu.Lock()
				out.captured.Write(buf[:n])
				out.mu.Unlock()
			}
			if rerr != nil {
				_ = r.Close()
				return
			}
		}
	}()
	machineOut = out
}

func currentMachineOutput() *machineOutput {
	if machineOut == nil {
		machineOut = &machineOutput{realStdout: os.Stdout, done: make(chan struct{})}
		close(machineOut.done)
	}
	return machineOut
}

// stageMachineData records a JSON value a command produced. The first value
// becomes data; a command that emits several gets them as an array.
func stageMachineData(v any, p *PaginationMeta) {
	out := currentMachineOutput()
	out.mu.Lock()
	defer out.mu.Unlock()
	out.staged = append(out.staged, v)
	if p != nil {
		out.pagination = p
	}
}

// recordMachineError keeps the first error a helper reported, so the
// envelope names the root cause rather than a cascade.
func recordMachineError(e *cliError) {
	out := currentMachineOutput()
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.recorded == nil {
		out.recorded = e
	}
}

// finishMachineMode restores stdout, writes the envelope, and returns the
// exit code. Called once from main.
func finishMachineMode(err error) int {
	out := currentMachineOutput()
	stdout := out.realStdout
	if out.pipeW != nil {
		os.Stdout = stdout
		_ = out.pipeW.Close()
		select {
		case <-out.done:
		case <-time.After(machineDrainTimeout):
			// Something still holds stdout (a detached child). Anything it
			// writes from here on is dropped; say so rather than lose it
			// silently.
			fmt.Fprintf(os.Stderr, "bd: warning: stdout still held open %s after the command finished; later output is dropped\n", machineDrainTimeout)
		}
	}
	out.mu.Lock()
	captured := append([]byte(nil), out.captured.Bytes()...)
	out.mu.Unlock()
	return writeMachineEnvelope(stdout, os.Stderr, out, captured, err)
}

// writeMachineEnvelope builds and writes the envelope. Separated from
// finishMachineMode so tests can drive it with buffers.
func writeMachineEnvelope(stdout, stderr io.Writer, out *machineOutput, captured []byte, err error) int {
	var data any
	switch len(out.staged) {
	case 0:
		if v, ok := parseCapturedJSON(captured); ok {
			data = v
			captured = nil
		}
	case 1:
		data = out.staged[0]
	default:
		data = out.staged
	}
	if len(bytes.TrimSpace(captured)) > 0 {
		// Prose or a second document: never on stdout in machine mode.
		_, _ = stderr.Write(captured)
	}

	ce, code := machineFailure(out.recorded, err)
	env := envelope{
		SchemaVersion:   JSONSchemaVersion,
		ContractVersion: JSONContractVersion,
		Data:            data,
		Pagination:      out.pagination,
	}
	if ce != nil {
		env.Error = ce.envelopeError()
	}
	if werr := emitEnvelope(stdout, env); werr != nil {
		fmt.Fprintf(stderr, "Error: writing JSON envelope: %v\n", werr)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// machineFailure resolves the error the envelope reports and the exit code.
func machineFailure(recorded *cliError, err error) (*cliError, int) {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce, kindExitCode(ce.Kind)
	}
	if err == nil {
		if recorded != nil {
			return recorded, kindExitCode(recorded.Kind)
		}
		return nil, 0
	}
	legacy, hasLegacy := exitCodeFromError(err)
	if recorded != nil {
		code := kindExitCode(recorded.Kind)
		if recorded.Kind == kindInternal && hasLegacy && legacy != 0 {
			code = legacy
		}
		return recorded, code
	}
	if hasLegacy {
		if legacy == ExitGuardMismatch {
			return newCLIError(kindGuardNotHeld, "guard not held"), legacy
		}
		return newCLIError(kindInternal, "command failed with exit status %d; details are on stderr", legacy), legacy
	}
	if isCobraUsageError(err) {
		ce := newCLIError(kindInvalidArgs, "%s", err.Error())
		return ce, kindExitCode(kindInvalidArgs)
	}
	return newCLIError(kindInternal, "%s", err.Error()), 1
}

// isCobraUsageError recognizes the errors cobra raises before any command
// code runs (unknown command, unknown flag), which reach main untyped.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	for _, prefix := range []string{"unknown command", "unknown flag", "unknown shorthand flag", "flag needs an argument", "invalid argument"} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}

// parseCapturedJSON turns what an unmigrated command printed into data: one
// JSON document, or JSON lines as an array.
func parseCapturedJSON(b []byte) (any, bool) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return nil, false
	}
	var v any
	if json.Unmarshal(trimmed, &v) == nil {
		return v, true
	}
	var rows []any
	sc := bufio.NewScanner(bytes.NewReader(trimmed))
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var row any
		if json.Unmarshal(line, &row) != nil {
			return nil, false
		}
		rows = append(rows, row)
	}
	if sc.Err() != nil || len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

// emitEnvelope is the only writer of machine-mode JSON.
func emitEnvelope(w io.Writer, env envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
