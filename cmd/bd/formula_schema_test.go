package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/formula"
)

func TestFormulaSchemaList_HumanOutput(t *testing.T) {
	prevJSON := jsonOutput
	jsonOutput = false
	defer func() { jsonOutput = prevJSON }()

	out := captureStdout(t, func() error {
		return runFormulaSchemaList()
	})

	for _, prim := range formula.Primitives {
		if !strings.Contains(out, prim.Name) {
			t.Errorf("primitive %s missing from list output", prim.Name)
		}
	}
	if !strings.Contains(out, "examples/formulas/primitives/") {
		t.Error("list output must point users at examples/formulas/primitives/")
	}
}

func TestFormulaSchemaList_JSON(t *testing.T) {
	prevJSON := jsonOutput
	jsonOutput = true
	defer func() { jsonOutput = prevJSON }()

	out := captureStdout(t, func() error {
		return runFormulaSchemaList()
	})

	// The envelope wraps the slice of primitives under "data".
	var envelope struct {
		Data []formula.PrimitiveDoc `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	got := envelope.Data
	if len(got) != len(formula.Primitives) {
		t.Errorf("got %d primitives, want %d", len(got), len(formula.Primitives))
	}
}

func TestFormulaSchemaShow_RendersFields(t *testing.T) {
	prevJSON := jsonOutput
	jsonOutput = false
	defer func() { jsonOutput = prevJSON }()

	out := captureStdout(t, func() error {
		return runFormulaSchemaShow("step")
	})

	for _, want := range []string{"Step", "children", "needs", "gate", "[]*Step"} {
		if !strings.Contains(out, want) {
			t.Errorf("step output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestFormulaSchemaShow_AliasResolution(t *testing.T) {
	prevJSON := jsonOutput
	jsonOutput = false
	defer func() { jsonOutput = prevJSON }()

	for _, input := range []string{"expand", "ExpandRule", "expand_rule"} {
		out := captureStdout(t, func() error {
			return runFormulaSchemaShow(input)
		})
		if !strings.Contains(out, "ExpandRule") {
			t.Errorf("%q did not resolve to ExpandRule; output:\n%s", input, out)
		}
		if !strings.Contains(out, "target") {
			t.Errorf("%q output missing target field", input)
		}
	}
}

func TestFormulaSchemaCmd_AliasRegistered(t *testing.T) {
	// `bd formula primitives` must route to the same handler as `bd formula schema`.
	found := false
	for _, alias := range formulaSchemaCmd.Aliases {
		if alias == "primitives" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("formulaSchemaCmd.Aliases = %v, want to contain \"primitives\"", formulaSchemaCmd.Aliases)
	}
}

func TestFormulaSchemaShow_JSON(t *testing.T) {
	prevJSON := jsonOutput
	jsonOutput = true
	defer func() { jsonOutput = prevJSON }()

	out := captureStdout(t, func() error {
		return runFormulaSchemaShow("step")
	})

	// Single-primitive output is the envelope's "data" object. Decode into a
	// generic map and assert the payload fields are present.
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	got, _ := envelope["data"].(map[string]interface{})
	if got == nil {
		t.Fatalf("no data object in JSON output: %s", out)
	}
	if got["name"] != "Step" {
		t.Errorf("name = %v, want Step", got["name"])
	}
	fields, ok := got["fields"].([]interface{})
	if !ok || len(fields) == 0 {
		t.Fatalf("fields missing or empty in JSON output: %v", got["fields"])
	}
}
