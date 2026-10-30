//go:build integration || e2e

package main

import (
	"encoding/json"
	"fmt"
)

// unmarshalEnvelope decodes the "data" member of a --json reply into v. The
// envelope {"schema_version": N, "data": ...} is the only --json shape, so a
// reply without both members is an error here, never decoded as flat JSON.
func unmarshalEnvelope(raw []byte, v any) error {
	var env struct {
		SchemaVersion *int            `json:"schema_version"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if env.SchemaVersion == nil || len(env.Data) == 0 {
		return fmt.Errorf("not a --json envelope (want schema_version and data): %.200s", raw)
	}
	return json.Unmarshal(env.Data, v)
}
