package formula

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func overlayFormula() *Formula {
	return &Formula{Formula: "mol-p", Steps: []*Step{
		{ID: "a", Title: "A", Description: "do a"},
		{ID: "b", Title: "B", Description: "do b", Needs: []string{"a"}},
		{ID: "c", Title: "C", Needs: []string{"b"}, DependsOn: []string{"b"}},
	}}
}

func TestApplyOverlay_Modes(t *testing.T) {
	f := overlayFormula()
	warnings := ApplyOverlay(f, &Overlay{StepOverrides: []StepOverride{
		{StepID: "a", Mode: OverlayReplace, Description: "new a"},
		{StepID: "b", Mode: OverlayAppend, Description: "more b"},
		{StepID: "zzz", Mode: OverlayAppend, Description: "stale"},
	}})
	if f.Steps[0].Description != "new a" || f.Steps[1].Description != "do b\nmore b" {
		t.Fatalf("replace/append wrong: %q %q", f.Steps[0].Description, f.Steps[1].Description)
	}
	if len(warnings) != 1 || warnings[0] != `overlay references unknown step "zzz" (stale override)` {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestApplyOverlay_SkipRewiresNeedsAndDependsOn(t *testing.T) {
	f := overlayFormula()
	ApplyOverlay(f, &Overlay{StepOverrides: []StepOverride{{StepID: "b", Mode: OverlaySkip}}})
	if len(f.Steps) != 2 || f.Steps[1].ID != "c" {
		t.Fatalf("skip did not remove b: %+v", f.Steps)
	}
	if !reflect.DeepEqual(f.Steps[1].Needs, []string{"a"}) || !reflect.DeepEqual(f.Steps[1].DependsOn, []string{"a"}) {
		t.Fatalf("c must inherit b's predecessors: needs=%v depends_on=%v", f.Steps[1].Needs, f.Steps[1].DependsOn)
	}
}

func TestLoadOverlay(t *testing.T) {
	dir := t.TempDir()
	if ov, err := LoadOverlay(dir, "mol-none"); ov != nil || err != nil {
		t.Fatalf("missing overlay: %v %v", ov, err)
	}
	if ov, err := LoadOverlay("", "mol-none"); ov != nil || err != nil {
		t.Fatalf("no overlay dir: %v %v", ov, err)
	}
	good := "[[step-overrides]]\nstep_id = \"a\"\nmode = \"append\"\ndescription = \"x\"\n"
	if err := os.WriteFile(filepath.Join(dir, "mol-p.toml"), []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadOverlay(dir, "mol-p")
	if err != nil || ov == nil || len(ov.StepOverrides) != 1 || ov.Path != filepath.Join(dir, "mol-p.toml") {
		t.Fatalf("load: %+v %v", ov, err)
	}
	for name, body := range map[string]string{
		"typo":     "[[step-overrides]]\nstep_id = \"a\"\nmode = \"append\"\ndescripton = \"x\"\n",
		"badmode":  "[[step-overrides]]\nstep_id = \"a\"\nmode = \"prepend\"\n",
		"noid":     "[[step-overrides]]\nmode = \"skip\"\n",
		"toplevel": "extra = 1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOverlay(dir, name); !errors.Is(err, ErrInvalidFormula) {
			t.Errorf("%s: want ErrInvalidFormula, got %v", name, err)
		}
	}
}
