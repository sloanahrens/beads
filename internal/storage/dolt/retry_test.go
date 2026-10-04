package dolt

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cenkalti/backoff/v4"
	mysql "github.com/go-sql-driver/mysql"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "driver bad connection",
			err:      errors.New("driver: bad connection"),
			expected: true,
		},
		{
			name:     "Driver Bad Connection (case insensitive)",
			err:      errors.New("Driver: Bad Connection"),
			expected: true,
		},
		{
			name:     "invalid connection",
			err:      errors.New("invalid connection"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write: broken pipe"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read: connection reset by peer"),
			expected: true,
		},
		{
			name:     "connection refused - retryable (server restart)",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "database is read only - retryable",
			err:      errors.New("cannot update manifest: database is read only"),
			expected: true,
		},
		{
			name:     "Database Is Read Only (case insensitive)",
			err:      errors.New("Database Is Read Only"),
			expected: true,
		},
		{
			name:     "lost connection - retryable (MySQL error 2013)",
			err:      errors.New("Error 2013: Lost connection to MySQL server during query"),
			expected: true,
		},
		{
			name:     "server gone away - retryable (MySQL error 2006)",
			err:      errors.New("Error 2006: MySQL server has gone away"),
			expected: true,
		},
		{
			name:     "i/o timeout - retryable",
			err:      errors.New("read tcp 127.0.0.1:3307: i/o timeout"),
			expected: true,
		},
		{
			name:     "unknown database - retryable (catalog race GH-1851)",
			err:      errors.New("Error 1049 (42000): Unknown database 'beads_test'"),
			expected: true,
		},
		{
			name:     "Unknown Database (case insensitive)",
			err:      errors.New("Unknown Database 'beads_test'"),
			expected: true,
		},
		{
			name:     "no root value found in session",
			err:      errors.New("Error 1105 (HY000): no root value found in session"),
			expected: true,
		},
		{
			name:     "typed no root value found in session",
			err:      &mysql.MySQLError{Number: 1105, Message: "no root value found in session"},
			expected: true,
		},
		{
			name:     "typed database is read only",
			err:      &mysql.MySQLError{Number: 1105, Message: "cannot update manifest: database is read only"},
			expected: true,
		},
		{
			name:     "untyped Dolt merge conflict does not enter general retry loop",
			err:      errors.New("dolt commit: Error 1105 (HY000): Merge conflict detected, @autocommit transaction rolled back"),
			expected: false,
		},
		{
			name:     "untyped 1105 could not resolve initial root",
			err:      errors.New("Error 1105 (HY000): could not resolve initial root for database `beads_x`"),
			expected: true,
		},
		{
			name:     "typed 1105 could not resolve initial root",
			err:      &mysql.MySQLError{Number: 1105, Message: "could not resolve initial root for database beads_x"},
			expected: true,
		},
		{
			name:     "typed 1105 with connection-like wording is not retryable",
			err:      &mysql.MySQLError{Number: 1105, Message: "connection lost while validating commit"},
			expected: false,
		},
		{
			name:     "syntax error - not retryable",
			err:      errors.New("Error 1064: You have an error in your SQL syntax"),
			expected: false,
		},
		{
			name:     "table not found - not retryable",
			err:      errors.New("Error 1146: Table 'beads.foo' doesn't exist"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	store := &DoltStore{}

	callCount := 0
	err := store.withRetry(context.Background(), func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call on success, got %d", callCount)
	}
}

func TestWithRetry_RetryOnBadConnection(t *testing.T) {
	store := &DoltStore{}

	callCount := 0
	err := store.withRetry(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return errors.New("driver: bad connection")
		}
		return nil // Success on 3rd attempt
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries + success), got %d", callCount)
	}
}

func TestWithRetry_RetryOnUnknownDatabase(t *testing.T) {
	// Simulates the GH-1851 race: "Unknown database" is transient after CREATE DATABASE
	store := &DoltStore{}

	callCount := 0
	err := store.withRetry(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return errors.New("Error 1049 (42000): Unknown database 'beads_test'")
		}
		return nil // Catalog caught up on 3rd attempt
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries + success), got %d", callCount)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	store := &DoltStore{}

	callCount := 0
	err := store.withRetry(context.Background(), func() error {
		callCount++
		return errors.New("syntax error in SQL")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
}

func TestWithRetry_DoesNotReplayDoltMergeConflict(t *testing.T) {
	store := &DoltStore{}

	callCount := 0
	err := store.withRetry(context.Background(), func() error {
		callCount++
		return errors.New("dolt commit: Error 1105 (HY000): Merge conflict detected, @autocommit transaction rolled back")
	})

	if err == nil {
		t.Fatal("withRetry() error = nil, want the definite rollback error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call at the general retry boundary, got %d", callCount)
	}
}

func TestRetryVersionCommit(t *testing.T) {
	t.Parallel()
	conflict := fmt.Errorf("dolt commit: %w", &mysql.MySQLError{
		Number:   1213,
		SQLState: [5]byte{'4', '0', '0', '0', '1'},
		Message:  "serialization failure: this transaction conflicts with a committed transaction from another client, try restarting transaction",
	})
	tests := []struct {
		name      string
		results   []error // commit's result per call; the last repeats
		wantErr   error
		wantCalls int
	}{
		{name: "succeeds first time", results: []error{nil}, wantCalls: 1},
		{name: "conflict then success", results: []error{conflict, nil}, wantCalls: 2},
		{name: "always conflicts", results: []error{conflict}, wantErr: conflict, wantCalls: 4},
		{name: "other error not retried", results: []error{errors.New("boom")}, wantErr: errors.New("boom"), wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			err := retryVersionCommit(context.Background(),
				backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 3),
				func() error {
					calls++
					return tt.results[min(calls, len(tt.results))-1]
				})
			if calls != tt.wantCalls {
				t.Errorf("commit called %d times, want %d", calls, tt.wantCalls)
			}
			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("err = %v, want nil", err)
			case tt.wantErr != nil && (err == nil || err.Error() != tt.wantErr.Error()):
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
