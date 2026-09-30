package formula

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// Every key the TOML library sees must be placed by the line locator, with
// the same number of occurrences, across every real formula in the repo.
// A drift here means strict-decode errors would point at the wrong line.
func TestLocateKeyLines_AgreesWithTOMLKeys(t *testing.T) {
	var files []string
	for _, pattern := range []string{
		"../../examples/formulas/*.formula.toml",
		"../../examples/formulas/primitives/*.formula.toml",
		"../../.beads/formulas/*.formula.toml",
		"../../cmd/bd/testdata/cook/formulas/*.formula.toml",
	} {
		m, _ := filepath.Glob(pattern)
		files = append(files, m...)
	}
	if len(files) < 5 {
		t.Fatalf("found only %d formula files", len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		md, err := toml.Decode(string(data), &v)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		want := map[string]int{}
		for _, k := range md.Keys() {
			want[strings.Join(k, ".")]++
		}
		got := locateKeyLines(data)
		for k, n := range want {
			if len(got[k]) != n {
				t.Errorf("%s: key %s: located %d times, toml has %d", filepath.Base(f), k, len(got[k]), n)
			}
		}
	}
}

func TestLocateKeyLines_EdgeCases(t *testing.T) {
	src := `a."b.c" = 1 # comment = x
[t]
s = '''
k = not a key
'''
arr = [
  { x = 1, y = { z = "}" } },
  "str with ] and = ",
]
[[t.u]]
w = """a \""" b"""
`
	got := locateKeyLines([]byte(src))
	want := map[string][]int{
		"a": {1}, "a.b.c": {1}, "t": {2}, "t.s": {3}, "t.arr": {6},
		"t.arr.x": {7}, "t.arr.y": {7}, "t.arr.y.z": {7}, "t.u": {10}, "t.u.w": {11},
	}
	for k, lines := range want {
		if len(got[k]) != len(lines) || (len(lines) > 0 && got[k][0] != lines[0]) {
			t.Errorf("%s: got %v want %v", k, got[k], lines)
		}
	}
	if _, bad := got["t.k"]; bad {
		t.Errorf("key inside a multi-line literal string was located: %v", got)
	}
}
