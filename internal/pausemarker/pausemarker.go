// Package pausemarker reads the Dolt pause marker gt writes and answers the
// one question a write path asks before touching the store: may I write now?
//
// The marker is a JSON file of the shape {actor, reason, until} at a path the
// caller supplies; the town daemon writes it at ~/gt/daemon/dolt.pause while
// Dolt is being restarted, migrated, or otherwise kept down on purpose. Read
// reports none, active, or expired; Wait blocks while the marker is active and
// gives up with a *PausedError when its cap is reached first. Both read only
// that file, so both answer while Dolt itself is hung - the whole point of a
// marker Dolt does not have to serve.
//
// Everything is injected: the marker path, the clock, the poll interval and
// the wait cap. The package keeps no process state, writes nothing, and
// depends only on the standard library.
//
// This package is the reader alone. Wiring it into machine-mode writes, and
// writing the marker itself, are separate work (epic be-c94, policy item D3).
package pausemarker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// State is what the marker says about writing right now.
type State string

const (
	// None: there is no marker file. Writing is allowed.
	None State = "none"
	// Active: a marker exists and its until is still ahead. Writing is paused.
	Active State = "active"
	// Expired: a marker exists but its until has passed. Writing is allowed
	// again.
	Expired State = "expired"
)

// Status is a marker read at one instant: how it bears on writing now, plus
// the fields it carries. Actor, Reason and Until are zero for State None.
type Status struct {
	State  State
	Actor  string
	Reason string
	Until  time.Time
}

// KindStoreUnavailable is the machine-mode failure kind a caller reports when
// Wait gives up: the same string cmd/bd puts in its envelope. cmd/bd is
// package main, so an internal package cannot import the constant from it.
//
// Note for the wiring bead: cmd/bd's errorKindOf classifies by concrete type,
// so a *PausedError reaching it lands on kindInternal until errorKindOf grows
// a case for it (errors.As-able *PausedError -> KindStoreUnavailable with
// QualifierPaused in detail). Kind and Qualifier below exist to make that case
// a two-line addition rather than a re-derivation.
const KindStoreUnavailable = "store_unavailable"

// QualifierPaused says why the store is unavailable. A paused store is down on
// purpose and comes back when the marker expires; an unreachable one is a
// fault. A caller reports KindStoreUnavailable qualified by this, and PausedError
// exposes it so the two never have to be told apart by parsing a message.
const QualifierPaused = "paused"

// Clock is the time source Read and Wait consult. It is injected so a test can
// drive time by hand and never sleep for real; neither Read nor Wait calls
// time.Now or time.Sleep itself.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// SystemClock is the Clock backed by the time package.
type SystemClock struct{}

// Now reports the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep blocks for d.
func (SystemClock) Sleep(d time.Duration) { time.Sleep(d) }

// DefaultPollInterval is how often Wait re-reads the marker when the caller
// sets no interval. It exists only to keep a zero interval from spinning; the
// re-read it paces is what lets a pause lifted early end the wait early.
const DefaultPollInterval = 250 * time.Millisecond

// marker is the on-disk shape. gt owns what is written; until is RFC 3339.
type marker struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Until  string `json:"until"`
}

// Read reports whether writes are paused by the marker at path, as of clock's
// Now. A nil clock means the system clock.
//
// A missing file is State None: no marker means no pause. Anything else that
// cannot be read as a marker - a permission error, malformed JSON, an until
// that is not RFC 3339 - is an error, never a silent None. A caller that
// cannot tell whether the store is paused must not read silence as permission
// to write, so an empty path is an error too: it can only come from a caller
// that never set the marker path, and answering None there would be exactly
// that mistake.
func Read(path string, clock Clock) (Status, error) {
	if path == "" {
		return Status{}, errors.New("pausemarker: read: empty marker path")
	}
	if clock == nil {
		clock = SystemClock{}
	}

	data, err := os.ReadFile(path) // #nosec G304 -- path is the caller-supplied marker location, not user input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Status{State: None}, nil
		}
		return Status{}, fmt.Errorf("pausemarker: read %s: %w", path, err)
	}

	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Status{}, fmt.Errorf("pausemarker: parse %s: %w", path, err)
	}
	until, err := time.Parse(time.RFC3339, m.Until)
	if err != nil {
		return Status{}, fmt.Errorf("pausemarker: parse %s: until %q: %w", path, m.Until, err)
	}

	status := Status{Actor: m.Actor, Reason: m.Reason, Until: until}
	// The pause is over at its until, not after it: an until equal to now is
	// expired.
	if until.After(clock.Now()) {
		status.State = Active
	} else {
		status.State = Expired
	}
	return status, nil
}

// Wait blocks until the marker at path stops being active, or until cap
// elapses, whichever comes first.
//
// It returns nil when there is no marker, when the marker has already expired,
// and when the marker expires or is cleared while it waits; it returns
// *PausedError when the marker is still active at the cap. A cap of zero
// therefore does not wait at all: an active marker at entry is refused
// immediately.
//
// poll is how long to sleep between re-reads; zero or negative uses
// DefaultPollInterval. A nil clock means the system clock. A marker that
// cannot be read stops the wait with that error, so Wait never mistakes an
// unreadable marker for a lifted pause.
func Wait(path string, clock Clock, poll, cap time.Duration) error {
	if clock == nil {
		clock = SystemClock{}
	}
	if poll <= 0 {
		poll = DefaultPollInterval
	}

	deadline := clock.Now().Add(cap)
	for {
		status, err := Read(path, clock)
		if err != nil {
			return err
		}
		if status.State != Active {
			return nil
		}
		remaining := deadline.Sub(clock.Now())
		if remaining <= 0 {
			return &PausedError{Actor: status.Actor, Reason: status.Reason, Until: status.Until}
		}
		sleep := poll
		if remaining < sleep {
			sleep = remaining // last sleep: land on the deadline, not past it
		}
		clock.Sleep(sleep)
	}
}

// PausedError reports that a wait for the pause marker to clear reached its cap
// with the marker still active. It carries the marker's fields so the caller
// can report who paused the store, why, and until when.
type PausedError struct {
	// Actor is the marker's actor field: who asked for the pause.
	Actor string
	// Reason is the marker's reason field: why.
	Reason string
	// Until is the marker's until field: when the pause was meant to end.
	Until time.Time
}

// Error describes the refusal.
func (e *PausedError) Error() string {
	msg := fmt.Sprintf("pausemarker: store unavailable (%s): Dolt paused until %s",
		QualifierPaused, e.Until.Format(time.RFC3339))
	if e.Actor != "" {
		msg += " by " + e.Actor
	}
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// Kind reports the machine-mode failure kind, KindStoreUnavailable.
func (e *PausedError) Kind() string { return KindStoreUnavailable }

// Qualifier reports why the store is unavailable, QualifierPaused.
func (e *PausedError) Qualifier() string { return QualifierPaused }
