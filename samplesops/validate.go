package samplesops

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ParseAt parses the RFC3339 timestamp a front door's --at carries and
// normalizes it to UTC.
//
// It lives here rather than in the command so every front door spells the
// accepted format and the refusal the same way. The error matches
// errors.Is against ErrValidation and names --at, because the caller's fix is
// to the argument they typed.
func ParseAt(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("%w: --at is required (RFC3339, e.g. 2026-11-11T09:30:00Z)", ErrValidation)
	}
	at, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: --at %q is not an RFC3339 timestamp", ErrValidation, trimmed)
	}
	return at.UTC(), nil
}

// ParseSince parses --since, where the empty string means "unbounded" and
// anything else must be RFC3339. It shares ParseAt's format and UTC
// normalization but names --since in its refusals.
func ParseSince(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	since, err := ParseAt(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: --since %q is not an RFC3339 timestamp", ErrValidation, strings.TrimSpace(raw))
	}
	return &since, nil
}

// ValidateAddRequest checks everything Add promises to refuse, before any
// transaction is opened, so a refusal cannot be mistaken at the call site for a
// write that rolled back.
func ValidateAddRequest(request AddRequest) error {
	if err := validateKind(request.Kind); err != nil {
		return err
	}
	if request.At.IsZero() {
		return fmt.Errorf("%w: at is required", ErrValidation)
	}
	if len(request.Payload) == 0 {
		return fmt.Errorf("%w: payload is required", ErrValidation)
	}
	if len(request.Payload) > MaxPayloadBytes {
		// No measurement is quoted: a front door that reads with a limiter
		// hands this function a truncated buffer, so the length it sees is a
		// lower bound and reporting it would understate the caller's file.
		return fmt.Errorf("%w: payload is over the %d-byte cap", ErrValidation, MaxPayloadBytes)
	}
	if !json.Valid(request.Payload) {
		return fmt.Errorf("%w: payload is not valid JSON", ErrValidation)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(request.Payload, &object); err != nil || object == nil {
		return fmt.Errorf("%w: payload must be a JSON object", ErrValidation)
	}
	return nil
}

// ValidateListRequest checks everything List promises to refuse.
func ValidateListRequest(request ListRequest) error {
	if err := validateKind(request.Kind); err != nil {
		return err
	}
	if request.Limit < 0 {
		return fmt.Errorf("%w: limit must not be negative (0 means uncapped)", ErrValidation)
	}
	if request.Since != nil && request.Since.IsZero() {
		return fmt.Errorf("%w: since must not be the zero time", ErrValidation)
	}
	return nil
}

// validateKind checks the shape of a kind, not its vocabulary: which kinds a
// producer writes is the producer's business (see the package doc).
func validateKind(kind string) error {
	if strings.TrimSpace(kind) == "" {
		return fmt.Errorf("%w: kind is required", ErrValidation)
	}
	if len(kind) > MaxKindBytes {
		return fmt.Errorf("%w: kind is %d bytes, over the %d-byte cap", ErrValidation, len(kind), MaxKindBytes)
	}
	return nil
}
