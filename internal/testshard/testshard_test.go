package testshard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestParseSpec(t *testing.T) {
	valid := []struct {
		spec  string
		index int
		count int
	}{
		{spec: "0/1", index: 0, count: 1},
		{spec: "0/3", index: 0, count: 3},
		{spec: "2/3", index: 2, count: 3},
	}
	for _, tt := range valid {
		t.Run("valid "+tt.spec, func(t *testing.T) {
			index, count, err := ParseSpec(tt.spec)
			if err != nil {
				t.Fatalf("ParseSpec(%q) error = %v, want nil", tt.spec, err)
			}
			if index != tt.index || count != tt.count {
				t.Fatalf("ParseSpec(%q) = %d, %d, want %d, %d", tt.spec, index, count, tt.index, tt.count)
			}
		})
	}

	// The last entry is all digits, so it reaches strconv rather than the
	// digit check: a count too large for an int is still an error.
	invalid := []string{"", "x", "1", "a/b", "2/2", "-1/3", "0/0", "1/0", "1/", "/3", "0/3/1", "1 /3", "+0/3", "-0/3", "999999999999999999999999/3"}
	for _, spec := range invalid {
		t.Run("invalid "+fmt.Sprintf("%q", spec), func(t *testing.T) {
			index, count, err := ParseSpec(spec)
			if err == nil {
				t.Fatalf("ParseSpec(%q) = %d, %d, want an error", spec, index, count)
			}
		})
	}
}

func TestSelectsPartitionsEveryName(t *testing.T) {
	names := make([]string, 200)
	for i := range names {
		names[i] = fmt.Sprintf("TestGenerated%d", i)
	}
	for _, count := range []int{1, 2, 3, 5} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			for _, name := range names {
				selected := 0
				for index := 0; index < count; index++ {
					if Selects(name, index, count) {
						selected++
					}
				}
				if selected != 1 {
					t.Fatalf("%s is selected by %d of %d shards, want exactly 1", name, selected, count)
				}
				for index := 0; index < count; index++ {
					first := Selects(name, index, count)
					if second := Selects(name, index, count); second != first {
						t.Fatalf("Selects(%s, %d, %d) = %v then %v, want a stable answer", name, index, count, first, second)
					}
				}
			}
		})
	}
}

func TestTestNames(t *testing.T) {
	dir := t.TempDir()
	alphaTest := `package fixture

import "testing"

func Test(t *testing.T) {}
func TestAlpha(t *testing.T) {}
func TestBeta(t *testing.T) {}
func Testlower(t *testing.T) {}
func Testing(t *testing.T) {}
func testUnexported(t *testing.T) {}
func helper(t *testing.T) {}
func BenchmarkAlpha(b *testing.B) {}
func FuzzAlpha(f *testing.F) {}
func TestMain(m *testing.M) {}
func (s suite) TestMethod(t *testing.T) {}
`
	betaTest := `package fixture

import "testing"

func TestGamma(t *testing.T) {}
func TestAlpha(t *testing.T) {}
`
	nonTest := `package fixture

import "testing"

func TestInNonTestFile(t *testing.T) {}
`
	files := map[string]string{
		"alpha_test.go": alphaTest,
		"beta_test.go":  betaTest,
		"fixture.go":    nonTest,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", name, err)
		}
	}

	names, err := TestNames(dir)
	if err != nil {
		t.Fatalf("TestNames() error = %v, want nil", err)
	}
	// A bare "Test" is run by the go tool too, so it is listed first.
	want := []string{"Test", "TestAlpha", "TestBeta", "TestGamma"}
	if !slices.Equal(names, want) {
		t.Fatalf("TestNames() = %v, want %v", names, want)
	}
}

func TestTestNamesReportsUnparsableFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken_test.go"), []byte("package fixture\n\nfunc TestAlpha(t *testing.T) {\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	names, err := TestNames(dir)
	if err == nil {
		t.Fatalf("TestNames() = %v, want an error for an unparsable test file", names)
	}
}

func TestTestNamesReportsUnreadableDir(t *testing.T) {
	names, err := TestNames(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("TestNames() = %v, want an error for a directory that is not there", names)
	}
}

func TestPatternSelectsExactlyTheShard(t *testing.T) {
	// Metacharacters, a prefix/superstring pair and an underscore cover the
	// ways an unquoted or unanchored pattern would over-match.
	names := []string{
		"TestAlpha",
		"TestAlphaBeta",
		"TestBeta",
		"TestDot.Name",
		"TestPlus+Name",
		"TestGroup(One)",
		"Test_V1",
		"TestMain",
	}
	for index := 0; index < 3; index++ {
		pattern := Pattern(names, index, 3)
		re := regexp.MustCompile(pattern)
		selected := 0
		for _, name := range names {
			want := Selects(name, index, 3)
			if got := re.MatchString(name); got != want {
				t.Fatalf("shard %d: pattern %q matches %q = %v, want %v", index, pattern, name, got, want)
			}
			if !want {
				continue
			}
			selected++
			for _, longer := range []string{name + "Extra", name + "_suffix"} {
				if re.MatchString(longer) {
					t.Fatalf("shard %d: pattern %q matches the superstring %q", index, pattern, longer)
				}
			}
		}
		if selected == 0 {
			if pattern != "^$" {
				t.Fatalf("shard %d selects nothing but Pattern = %q, want %q", index, pattern, "^$")
			}
			if re.MatchString("TestAlpha") {
				t.Fatalf("the empty-shard pattern %q matches TestAlpha", pattern)
			}
		}
	}
}

func TestPatternOfNoNames(t *testing.T) {
	pattern := Pattern(nil, 0, 3)
	if pattern != "^$" {
		t.Fatalf("Pattern(nil, 0, 3) = %q, want %q", pattern, "^$")
	}
	re := regexp.MustCompile(pattern)
	if re.MatchString("TestAlpha") {
		t.Fatalf("the empty pattern %q matches TestAlpha", pattern)
	}
}
