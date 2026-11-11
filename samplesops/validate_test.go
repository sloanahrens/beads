package samplesops

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseAtNormalizesToUTC(t *testing.T) {
	at, err := ParseAt("2026-11-11T09:30:00+02:00")
	if err != nil {
		t.Fatalf("ParseAt: %v", err)
	}
	if got, want := at.Format(time.RFC3339), "2026-11-11T07:30:00Z"; got != want {
		t.Errorf("ParseAt = %s, want %s", got, want)
	}
}

func TestParseAtRefusalsNameTheField(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing", raw: "", want: "--at"},
		{name: "whitespace only", raw: "  ", want: "--at"},
		{name: "not a timestamp", raw: "yesterday", want: "--at"},
		{name: "date without time", raw: "2026-11-11", want: "--at"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAt(tt.raw)
			assertValidationError(t, err, tt.want)
		})
	}
}

func TestParseSinceTreatsEmptyAsUnbounded(t *testing.T) {
	since, err := ParseSince("")
	if err != nil {
		t.Fatalf("ParseSince(\"\") = %v, want no error", err)
	}
	if since != nil {
		t.Fatalf("ParseSince(\"\") = %v, want nil", since)
	}
}

func TestParseSinceRefusalsNameTheField(t *testing.T) {
	_, err := ParseSince("2026-11-11 09:30")
	assertValidationError(t, err, "--since")
}

func TestValidateAddRequestAcceptsAJSONObject(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
	}{
		{name: "compact", payload: `{"balance":12}`},
		{name: "with whitespace", payload: " {\n  \"balance\": 12\n}\n"},
		{name: "nested with duplicate keys", payload: `{"a":{"b":1},"a":{"b":2}}`},
		{name: "empty object", payload: `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := AddRequest{Kind: "deepseek_balance", At: time.Now(), Payload: []byte(tt.payload)}
			if err := ValidateAddRequest(request); err != nil {
				t.Fatalf("ValidateAddRequest(%s) = %v, want accepted", tt.name, err)
			}
		})
	}
}

func TestValidateAddRequestRefusalsNameTheField(t *testing.T) {
	valid := AddRequest{Kind: "deepseek_balance", At: time.Now(), Payload: []byte(`{"ok":true}`)}
	tooBig := make([]byte, MaxPayloadBytes+1)
	tooBig[0] = '{'
	for i := 1; i < len(tooBig)-1; i++ {
		tooBig[i] = ' '
	}
	tooBig[len(tooBig)-1] = '}'

	tests := []struct {
		name   string
		mutate func(*AddRequest)
		want   string
	}{
		{name: "empty kind", mutate: func(r *AddRequest) { r.Kind = "" }, want: "kind"},
		{name: "whitespace kind", mutate: func(r *AddRequest) { r.Kind = "  " }, want: "kind"},
		{name: "oversize kind", mutate: func(r *AddRequest) { r.Kind = strings.Repeat("k", MaxKindBytes+1) }, want: "kind"},
		{name: "zero at", mutate: func(r *AddRequest) { r.At = time.Time{} }, want: "at"},
		{name: "empty payload", mutate: func(r *AddRequest) { r.Payload = nil }, want: "payload"},
		{name: "oversize payload", mutate: func(r *AddRequest) { r.Payload = tooBig }, want: "payload"},
		{name: "invalid JSON", mutate: func(r *AddRequest) { r.Payload = []byte(`{"a":`) }, want: "payload"},
		{name: "JSON array", mutate: func(r *AddRequest) { r.Payload = []byte(`[1,2]`) }, want: "payload"},
		{name: "JSON null", mutate: func(r *AddRequest) { r.Payload = []byte(`null`) }, want: "payload"},
		{name: "JSON string", mutate: func(r *AddRequest) { r.Payload = []byte(`"balance"`) }, want: "payload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			tt.mutate(&request)
			assertValidationError(t, ValidateAddRequest(request), tt.want)
		})
	}
}

func TestValidateAddRequestAcceptsThePayloadCapExactly(t *testing.T) {
	// A payload of exactly MaxPayloadBytes is accepted: the cap is a maximum,
	// not an exclusive bound.
	payload := make([]byte, MaxPayloadBytes)
	payload[0] = '{'
	for i := 1; i < len(payload)-1; i++ {
		payload[i] = ' '
	}
	payload[len(payload)-1] = '}'
	request := AddRequest{Kind: "usage_record", At: time.Now(), Payload: payload}
	if err := ValidateAddRequest(request); err != nil {
		t.Fatalf("ValidateAddRequest(payload at the cap) = %v, want accepted", err)
	}
}

func TestValidateListRequestRefusalsNameTheField(t *testing.T) {
	zero := time.Time{}
	tests := []struct {
		name    string
		request ListRequest
		want    string
	}{
		{name: "empty kind", request: ListRequest{}, want: "kind"},
		{name: "negative limit", request: ListRequest{Kind: "usage_record", Limit: -1}, want: "limit"},
		{name: "zero since pointer", request: ListRequest{Kind: "usage_record", Since: &zero}, want: "since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertValidationError(t, ValidateListRequest(tt.request), tt.want)
		})
	}
}

func TestValidateListRequestAcceptsUnboundedAndUncapped(t *testing.T) {
	if err := ValidateListRequest(ListRequest{Kind: "claude_plan"}); err != nil {
		t.Fatalf("ValidateListRequest(default) = %v, want accepted", err)
	}
}

// assertValidationError requires err to be a validation refusal whose message
// names field.
func assertValidationError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want a %s refusal", field)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error %q does not name %q", err.Error(), field)
	}
}
