package pausemarker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// base is the instant every test starts from. Markers are built relative to it,
// and the fake clock stays there unless the code under test asks it to move.
var base = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fakeClock is a Clock that advances only when Sleep is called, so a test sees
// exactly how long the code meant to wait without waiting for it.
type fakeClock struct {
	now     time.Time
	onSleep func()
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
	if c.onSleep != nil {
		c.onSleep()
	}
}

// markerBody renders the marker file's JSON shape. The values are test
// constants, so no JSON escaping is needed.
func markerBody(actor, reason string, until time.Time) string {
	return `{"actor":"` + actor + `","reason":"` + reason + `","until":"` + until.Format(time.RFC3339) + `"}`
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRead(t *testing.T) {
	active := base.Add(30 * time.Minute)
	expired := base.Add(-time.Minute)

	tests := []struct {
		name       string
		create     bool // write body to the marker path
		mkdir      bool // make the marker path a directory instead
		body       string
		wantState  State
		wantActor  string
		wantReason string
		wantUntil  time.Time
		wantErr    string // substring the error must carry; empty means no error
	}{
		{
			name:      "missing file is none",
			wantState: None,
		},
		{
			name:       "active marker",
			create:     true,
			body:       markerBody("gt-daemon", "dolt restart", active),
			wantState:  Active,
			wantActor:  "gt-daemon",
			wantReason: "dolt restart",
			wantUntil:  active,
		},
		{
			name:       "expired marker",
			create:     true,
			body:       markerBody("gt-daemon", "dolt restart", expired),
			wantState:  Expired,
			wantActor:  "gt-daemon",
			wantReason: "dolt restart",
			wantUntil:  expired,
		},
		{
			name:       "until equal to now is expired",
			create:     true,
			body:       markerBody("gt-daemon", "dolt restart", base),
			wantState:  Expired,
			wantActor:  "gt-daemon",
			wantReason: "dolt restart",
			wantUntil:  base,
		},
		{
			name:       "unknown fields are ignored",
			create:     true,
			body:       `{"actor":"gt-daemon","reason":"dolt restart","until":"` + active.Format(time.RFC3339) + `","note":"added later"}`,
			wantState:  Active,
			wantActor:  "gt-daemon",
			wantReason: "dolt restart",
			wantUntil:  active,
		},
		{
			name:    "malformed json is an error, never none",
			create:  true,
			body:    `{"actor":`,
			wantErr: "parse",
		},
		{
			name:    "empty file is an error, never none",
			create:  true,
			body:    "",
			wantErr: "parse",
		},
		{
			name:    "json that is not an object is an error",
			create:  true,
			body:    `["gt-daemon"]`,
			wantErr: "parse",
		},
		{
			name:    "missing until is an error, never none",
			create:  true,
			body:    `{"actor":"gt-daemon","reason":"dolt restart"}`,
			wantErr: "until",
		},
		{
			name:    "until without a zone is an error",
			create:  true,
			body:    `{"until":"2026-03-01T12:30:00"}`,
			wantErr: "until",
		},
		{
			name:    "unreadable marker is an error, never none",
			mkdir:   true,
			wantErr: "pausemarker: read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dolt.pause")
			switch {
			case tt.mkdir:
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("mkdir %s: %v", path, err)
				}
			case tt.create:
				writeRaw(t, path, tt.body)
			}

			got, err := Read(path, &fakeClock{now: base})

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Read() = %+v, nil; want an error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Read() error = %q, want it to contain %q", err, tt.wantErr)
				}
				if got.State == None {
					t.Errorf("Read() state = %q with an error; an unreadable marker must not read as no pause", got.State)
				}
				return
			}
			if err != nil {
				t.Fatalf("Read() error = %v, want nil", err)
			}
			if got.State != tt.wantState {
				t.Errorf("Read() state = %q, want %q", got.State, tt.wantState)
			}
			if got.Actor != tt.wantActor {
				t.Errorf("Read() actor = %q, want %q", got.Actor, tt.wantActor)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Read() reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if !got.Until.Equal(tt.wantUntil) {
				t.Errorf("Read() until = %s, want %s", got.Until, tt.wantUntil)
			}
		})
	}
}

func TestNilClockUsesTheSystemClock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")

	status, err := Read(path, nil)
	if err != nil || status.State != None {
		t.Fatalf("Read(missing, nil) = %+v, %v; want none, nil", status, err)
	}

	writeRaw(t, path, markerBody("gt-daemon", "dolt restart", time.Now().Add(time.Hour)))
	status, err = Read(path, nil)
	if err != nil || status.State != Active {
		t.Fatalf("Read(active, nil) = %+v, %v; want active, nil", status, err)
	}
}

func TestWaitReturnsNilWithoutWaiting(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name:  "no marker",
			setup: func(*testing.T, string) {},
		},
		{
			name: "expired marker",
			setup: func(t *testing.T, path string) {
				writeRaw(t, path, markerBody("gt-daemon", "dolt restart", base.Add(-time.Minute)))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dolt.pause")
			tt.setup(t, path)
			clock := &fakeClock{now: base}

			if err := Wait(path, clock, time.Second, time.Minute); err != nil {
				t.Fatalf("Wait() = %v, want nil", err)
			}
			if !clock.now.Equal(base) {
				t.Errorf("clock moved to %s; an unpaused store must not wait", clock.now)
			}
		})
	}
}

func TestWaitReturnsNilWhenMarkerExpiresWhileWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	until := base.Add(20 * time.Second)
	writeRaw(t, path, markerBody("gt-daemon", "dolt restart", until))
	clock := &fakeClock{now: base}

	if err := Wait(path, clock, 5*time.Second, time.Hour); err != nil {
		t.Fatalf("Wait() = %v, want nil once the marker expires", err)
	}
	if !clock.now.Equal(until) {
		t.Errorf("wait ended at %s, want the marker's until %s", clock.now, until)
	}
}

func TestWaitReturnsNilWhenMarkerIsClearedWhileWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	writeRaw(t, path, markerBody("gt-daemon", "dolt restart", base.Add(time.Hour)))
	clock := &fakeClock{now: base}
	clock.onSleep = func() {
		clock.onSleep = nil // clear the marker once, on the first poll
		if err := os.Remove(path); err != nil {
			t.Errorf("remove marker: %v", err)
		}
	}

	if err := Wait(path, clock, 5*time.Second, time.Hour); err != nil {
		t.Fatalf("Wait() = %v, want nil once the marker is gone", err)
	}
	if want := base.Add(5 * time.Second); !clock.now.Equal(want) {
		t.Errorf("wait ended at %s, want one poll interval after base (%s)", clock.now, want)
	}
}

func TestWaitRefusesWhenMarkerOutlastsTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	until := base.Add(time.Hour)
	writeRaw(t, path, markerBody("gt-daemon", "dolt upgrade", until))
	clock := &fakeClock{now: base}

	err := Wait(path, clock, 3*time.Second, 10*time.Second)

	var paused *PausedError
	if !errors.As(err, &paused) {
		t.Fatalf("Wait() = %v (%T), want a *PausedError", err, err)
	}
	if paused.Actor != "gt-daemon" {
		t.Errorf("PausedError.Actor = %q, want %q", paused.Actor, "gt-daemon")
	}
	if paused.Reason != "dolt upgrade" {
		t.Errorf("PausedError.Reason = %q, want %q", paused.Reason, "dolt upgrade")
	}
	if !paused.Until.Equal(until) {
		t.Errorf("PausedError.Until = %s, want %s", paused.Until, until)
	}
	if got := paused.Kind(); got != KindStoreUnavailable {
		t.Errorf("PausedError.Kind() = %q, want %q", got, KindStoreUnavailable)
	}
	if got := paused.Qualifier(); got != QualifierPaused {
		t.Errorf("PausedError.Qualifier() = %q, want %q", got, QualifierPaused)
	}
	if msg := paused.Error(); !strings.Contains(msg, QualifierPaused) || !strings.Contains(msg, "gt-daemon") {
		t.Errorf("PausedError.Error() = %q, want it to name the qualifier and the actor", msg)
	}
	if want := base.Add(10 * time.Second); !clock.now.Equal(want) {
		t.Errorf("wait ended at %s, want the cap (%s)", clock.now, want)
	}
}

func TestWaitWithZeroCapRefusesImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	writeRaw(t, path, markerBody("gt-daemon", "dolt restart", base.Add(time.Hour)))
	clock := &fakeClock{now: base}

	err := Wait(path, clock, time.Second, 0)

	var paused *PausedError
	if !errors.As(err, &paused) {
		t.Fatalf("Wait() = %v (%T), want a *PausedError", err, err)
	}
	if !clock.now.Equal(base) {
		t.Errorf("clock moved to %s; a zero cap must not sleep", clock.now)
	}
}

func TestWaitWithZeroPollStillTerminates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	until := base.Add(time.Second)
	writeRaw(t, path, markerBody("gt-daemon", "dolt restart", until))
	clock := &fakeClock{now: base}

	if err := Wait(path, clock, 0, time.Minute); err != nil {
		t.Fatalf("Wait() = %v, want nil once the marker expires", err)
	}
	if !clock.now.Equal(until) {
		t.Errorf("wait ended at %s, want the marker's until %s", clock.now, until)
	}
}

func TestWaitDoesNotReadAnUnreadableMarkerAsALiftedPause(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dolt.pause")
	writeRaw(t, path, `{"actor":`)

	err := Wait(path, &fakeClock{now: base}, time.Second, time.Minute)

	if err == nil {
		t.Fatal("Wait() = nil for a malformed marker; it must not be read as a lifted pause")
	}
	var paused *PausedError
	if errors.As(err, &paused) {
		t.Errorf("Wait() = %v, want a read error rather than a pause refusal", err)
	}
}
