package dolt

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestGateOnce pins the retry-loop contract behind be-pv7(a): the remote-migrate
// gate answers "would migrating fork a shared database?" once per open. A
// retry that follows a half-applied first pass (database now at v1 with
// migrations pending) must not re-ask it and refuse its own bootstrap.
func TestGateOnce(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB // the fake gates never touch it

	t.Run("passes through until it passes once, then never runs again", func(t *testing.T) {
		calls := 0
		gate := gateOnce(func(context.Context, *sql.DB) error {
			calls++
			return nil
		})
		for i := 0; i < 3; i++ {
			if err := gate(ctx, db); err != nil {
				t.Fatalf("call %d: unexpected error %v", i, err)
			}
		}
		if calls != 1 {
			t.Fatalf("underlying gate ran %d times, want 1", calls)
		}
	})

	t.Run("a refusal is returned every time and never latches a pass", func(t *testing.T) {
		refusal := errors.New("refusing to auto-apply 65 pending schema migrations")
		calls := 0
		gate := gateOnce(func(context.Context, *sql.DB) error {
			calls++
			return refusal
		})
		for i := 0; i < 3; i++ {
			if err := gate(ctx, db); !errors.Is(err, refusal) {
				t.Fatalf("call %d: err = %v, want the refusal", i, err)
			}
		}
		if calls != 3 {
			t.Fatalf("underlying gate ran %d times, want 3", calls)
		}
	})

	t.Run("a transient error is retried, then the pass latches", func(t *testing.T) {
		calls := 0
		gate := gateOnce(func(context.Context, *sql.DB) error {
			calls++
			if calls == 1 {
				return errors.New("no root value found in session")
			}
			return nil
		})
		if err := gate(ctx, db); err == nil {
			t.Fatal("first call: want the transient error")
		}
		for i := 0; i < 2; i++ {
			if err := gate(ctx, db); err != nil {
				t.Fatalf("later call %d: %v", i, err)
			}
		}
		if calls != 2 {
			t.Fatalf("underlying gate ran %d times, want 2", calls)
		}
	})

	t.Run("nil gate stays nil", func(t *testing.T) {
		if gateOnce(nil) != nil {
			t.Fatal("gateOnce(nil) must be nil so callers' nil checks still skip the gate")
		}
	})
}
