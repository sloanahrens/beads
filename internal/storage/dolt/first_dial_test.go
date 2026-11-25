package dolt

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// The fail-fast dial before the MySQL handshake is bounded by
// serverFirstDialTimeout, not a reachability probe's budget: the guest OS
// retransmits a dropped SYN after 1s, so a dial that gives up sooner can never
// observe it (the VM-to-host forwarder drops SYNs beyond roughly 12
// simultaneous handshakes). These tests drive newServerMode through a stubbed
// serverDial, so they touch no network and need no live Dolt server.

// Sentinel errors the stub returns, so each row can prove the returned error
// wraps the dial failure. The timeout one is a real net.Error.
var (
	errFirstDialRefused = errors.New("dial tcp: connect: connection refused")
	errFirstDialTimeout = &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}
)

type firstDialCall struct {
	network string
	address string
	timeout time.Duration
}

// stubServerDial replaces the package dialer for one test and records every
// call it receives.
func stubServerDial(t *testing.T, dialErr error) *[]firstDialCall {
	t.Helper()
	orig := serverDial
	t.Cleanup(func() { serverDial = orig })
	var calls []firstDialCall
	serverDial = func(network, address string, timeout time.Duration) (net.Conn, error) {
		calls = append(calls, firstDialCall{network: network, address: address, timeout: timeout})
		return nil, dialErr
	}
	return &calls
}

// TestFirstDial_FailFast covers the fail-fast dial in newServerMode that runs
// before the MySQL protocol handshake. Every row must make exactly one dial,
// pass serverFirstDialTimeout (more than one SYN retransmit), return the
// unreachable-server error wrapping the dial error, and record exactly one
// breaker failure.
func TestFirstDial_FailFast(t *testing.T) {
	// The suite's TestMain sets BEADS_TEST_MODE=1, which makes
	// initializeServerCircuitBreaker return nil. These rows assert the
	// breaker's failure accounting, so opt into real breaker behavior the way
	// circuit_test.go does; the circuit dir stays the suite-owned temp dir.
	t.Setenv("BEADS_TEST_MODE", "")

	const host = "127.0.0.1"
	const port = 1 // > 0 so the breaker exists; the stub means nothing is dialed
	addr := net.JoinHostPort(host, "1")

	cases := []struct {
		name    string
		dialErr error
	}{
		{name: "refused", dialErr: errFirstDialRefused},
		{name: "timed out", dialErr: errFirstDialTimeout},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database := "test_first_dial_" + strings.ReplaceAll(tc.name, " ", "_")
			// Reset so a repeated run (same suite process, same circuit dir)
			// still observes exactly one recorded failure.
			newCircuitBreaker(host, port, database).Reset()
			calls := stubServerDial(t, tc.dialErr)
			cfg := &Config{Database: database, ServerHost: host, ServerPort: port}

			_, err := newServerMode(context.Background(), cfg)
			if err == nil {
				t.Fatal("newServerMode returned nil error for an unreachable server")
			}
			if !errors.Is(err, tc.dialErr) {
				t.Fatalf("error does not wrap the dial error: %v", err)
			}
			if !strings.Contains(err.Error(), "Dolt server unreachable at "+addr) {
				t.Fatalf("error = %q, want the unreachable-server message for %s", err, addr)
			}

			if len(*calls) != 1 {
				t.Fatalf("dial calls = %d, want 1: %+v", len(*calls), *calls)
			}
			call := (*calls)[0]
			if call.network != "tcp" || call.address != addr {
				t.Errorf("dial = %s %s, want tcp %s", call.network, call.address, addr)
			}
			if call.timeout != serverFirstDialTimeout {
				t.Errorf("dial timeout = %v, want serverFirstDialTimeout (%v)", call.timeout, serverFirstDialTimeout)
			}
			if serverFirstDialTimeout <= time.Second {
				t.Errorf("serverFirstDialTimeout = %v, want more than 1s so a SYN retransmit is observable", serverFirstDialTimeout)
			}

			state := newCircuitBreaker(host, port, database).readState()
			if state.Failures != 1 {
				t.Errorf("breaker failures = %d, want 1", state.Failures)
			}
		})
	}
}
