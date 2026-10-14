package issueops

import (
	"context"
	"testing"
	"time"
)

func TestNowUsesInjectedClock(t *testing.T) {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	got := Now(WithClock(context.Background(), func() time.Time { return fixed }))
	if !got.Equal(fixed) || got.Location() != time.UTC {
		t.Errorf("Now with injected clock = %v, want %v in UTC", got, fixed)
	}
}

func TestNowDefaultsToWallClock(t *testing.T) {
	before := time.Now()
	got := Now(context.Background())
	if got.Before(before) || time.Since(got) > time.Minute {
		t.Errorf("Now without a clock = %v, want about the current time", got)
	}
}
