package samplesops

import (
	"context"
	"time"
)

// MaxPayloadBytes is the largest payload the plane accepts, measured on the
// bytes as given. It is 64 KiB, and the table column is LONGTEXT rather than
// TEXT precisely because TEXT stops at 65535 — one byte below this cap.
const MaxPayloadBytes = 64 << 10

// MaxKindBytes is the largest kind the plane accepts. The table column is
// VARCHAR(64), so a longer value is refused before the database sees it rather
// than arriving as a truncation or an engine error.
const MaxKindBytes = 64

// Sample is one stored observation.
//
// At is the event time the producer reported, converted to UTC; CreatedAt is
// the ingest time the plane stamped. Both are stored with microsecond
// precision, so a caller that passes nanoseconds reads back a truncated At.
// Payload is the exact text that was stored: see the package doc.
type Sample struct {
	ID        string
	Kind      string
	At        time.Time
	Payload   string
	CreatedAt time.Time
}

// AddRequest appends one sample.
type AddRequest struct {
	// Kind names the producer's observation type. Required; at most
	// MaxKindBytes bytes.
	Kind string
	// At is the event time. Required (a zero value is refused): a sample with
	// no time cannot be ordered, and defaulting it to "now" would hide a
	// caller's bug behind a plausible-looking row.
	At time.Time
	// Payload is the JSON object text to store, verbatim. Required; a JSON
	// object; at most MaxPayloadBytes bytes.
	Payload []byte
}

// AddResult is the row Add appended.
type AddResult struct {
	Sample Sample
}

// ListRequest reads the samples of one kind, newest first.
type ListRequest struct {
	// Kind selects the rows. Required.
	Kind string
	// Since, when set, keeps rows whose At is at or after it (INCLUSIVE), so a
	// caller that re-reads from the last At it saw does not miss a row
	// stamped at that instant. Nil means unbounded.
	Since *time.Time
	// Limit caps the rows returned. Zero means uncapped; a negative value is
	// refused rather than treated as zero, because "--limit -1" almost always
	// means the caller computed a page size from an empty result.
	Limit int
}

// ListResult is the rows List read, newest first.
type ListResult struct {
	Samples []Sample
}

// Samples is the workspace's generic samples plane.
//
// ORDER. List answers newest first by At, so a caller polling a feed reads the
// latest row first. Rows sharing an At are ordered by ingest time and then id,
// which makes the order total: two calls over an unchanged table return the
// same sequence.
//
// PAYLOAD FIDELITY. A payload lists back byte-for-byte as it was added. The
// plane validates its shape at write time and never re-encodes it, so key
// order, whitespace and duplicate keys survive a round trip.
//
// REFUSALS ARE CLASSIFIED. A request that cannot be satisfied as written is
// refused with an error matching errors.Is against ErrValidation; the message
// names the field that failed. A storage failure is returned as itself and
// never as a validation error, so a front door can tell a caller's mistake
// from the database's.
//
// A write that returns has been committed by the implementation; the surface
// has no separate commit call. Implementations never mutate caller-owned
// values.
type Samples interface {
	// Add appends one sample and returns the stored row.
	Add(ctx context.Context, request AddRequest) (AddResult, error)

	// List reads one kind's samples, newest first, honoring Since and Limit.
	// A kind with no rows answers an empty slice and no error.
	List(ctx context.Context, request ListRequest) (ListResult, error)
}
