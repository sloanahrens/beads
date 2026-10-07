package scripts_test

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// cmdBdStoreHelpers are the cmd/bd test helpers that open a real Dolt store.
// Each one ends in dolt.New against the shared test server, so a test that
// reaches one of them can only skip in the unit tier, which starts no server
// (engdocs/TESTING.md, "The Two Tiers"). newParityEnv is on the list because
// it wires a parityStore over newTestStore.
//
// The scan seeds its call graph from these names and follows calls to
// package-local functions, so it catches the test that reaches newTestStore
// through a helper of its own, not just the direct caller.
var cmdBdStoreHelpers = []string{
	"newTestStore",
	"newTestStoreIsolatedDB",
	"newTestStoreWithPrefix",
	"newParityEnv",
}

// cmdBdStoreGrandfathered lists the store-opening cmd/bd tests that predate
// this check and still compile in the unit tier. It is empty: be-vyu retagged
// cmd/bd's store-opening files wholesale, and be-7w3 moved the last three —
// which a census keyed on skip messages missed, because skipIfNoDolt reports
// "Dolt test server not running" rather than "Dolt test server not available",
// so their tests skipped silently in the unit tier.
//
// The list stays as the way to land a change to this check ahead of the retag
// it flags. Add an entry only for a test whose file is being retagged in a
// follow-up, and delete it there.
var cmdBdStoreGrandfathered = map[string]bool{}

// TestCmdBdStoreTestsCarryTheIntegrationTag fails when a cmd/bd test that
// needs a Dolt server is compiled in the unit tier. TESTING.md calls the tier
// "enforced at runtime", but the store helpers check testDoltServerPort first
// and call testutil.SkipOrFailUnavailable, which in the unit tier is a plain
// t.Skip: the tier pays for the compile and the skip, and nothing says so.
// This check is static on purpose — the unit tier has no server to fail
// against.
//
// Only test entry points count. The helpers themselves, and the harnesses
// that wrap them, stay untagged so both tiers keep compiling — that is the
// shape be-vyu left cmd/bd in.
func TestCmdBdStoreTestsCarryTheIntegrationTag(t *testing.T) {
	scan, err := scanUnitTierStoreOpeners(cmdBdTestSources(t))
	if err != nil {
		t.Fatalf("scan cmd/bd test files: %v", err)
	}

	if len(scan.Helpers) != len(cmdBdStoreHelpers) {
		t.Fatalf("unit tier defines %v of %v; a helper renamed or moved behind the integration tag leaves this check with nothing to resolve",
			scan.Helpers, cmdBdStoreHelpers)
	}

	var unexpected []string
	for _, violation := range scan.Violations {
		if !cmdBdStoreGrandfathered[violation] {
			unexpected = append(unexpected, violation)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf(`cmd/bd tests reach a Dolt store helper but compile in the unit tier:
  %s

Add "integration" to the file's build constraint (//go:build cgo && integration),
or move the store-opening tests into a sibling <name>_integration_test.go and
leave the unit tests and shared harness behind (engdocs/TESTING.md, "The Two Tiers").`,
			strings.Join(unexpected, "\n  "))
	}
}

// TestUnitTierStoreScanFiresOnAnUntaggedStoreTest is the negative fixture: a
// test that opens a store through a helper of its own, with no integration
// tag, must be reported — otherwise the check above passes for the wrong
// reason.
func TestUnitTierStoreScanFiresOnAnUntaggedStoreTest(t *testing.T) {
	scan, err := scanUnitTierStoreOpeners(untaggedStoreTestFixture())
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if want := []string{"store_test.go: TestOpensStoreThroughLocalHelper"}; !slices.Equal(scan.Violations, want) {
		t.Errorf("violations = %v, want %v; the tagged and the store-free test in the fixture must stay quiet", scan.Violations, want)
	}
}

// untaggedStoreTestFixture mirrors cmd/bd's shape: the helpers live in an
// untagged harness file, one test reaches them through a helper of its own,
// one is tagged, and one never touches a store.
func untaggedStoreTestFixture() map[string]string {
	return map[string]string{
		"test_helpers_test.go": `package main

import "testing"

func newTestStore(t *testing.T, dbPath string) {}

func newTestStoreIsolatedDB(t *testing.T, dbPath string, prefix string) {}

func newTestStoreWithPrefix(t *testing.T, dbPath string, prefix string) {}

func newParityEnv(t *testing.T) {}

func helperThatOpensStore(t *testing.T, dbPath string) {
	newTestStore(t, dbPath)
}
`,
		"store_test.go": `package main

import "testing"

func TestOpensStoreThroughLocalHelper(t *testing.T) {
	helperThatOpensStore(t, "/tmp/db")
}
`,
		"tagged_integration_test.go": `//go:build cgo && integration

package main

import "testing"

func TestOpensStore(t *testing.T) {
	newParityEnv(t)
}
`,
		"envelope_test.go": `package main

import "testing"

func TestStoreFreeEnvelope(t *testing.T) {
	if false {
		t.Error("never")
	}
}
`,
	}
}

// unitTierFunc is one package-local function declaration, as the unit tier
// compiles it: the bare-name calls in its body.
type unitTierFunc struct {
	calls []string
}

// unitTierStoreScan is what scanUnitTierStoreOpeners found in a test package.
type unitTierStoreScan struct {
	// Violations are "<file>: <Test>" for every test entry point that compiles
	// without the integration tag and reaches a store helper, sorted.
	Violations []string
	// Helpers are the cmdBdStoreHelpers that a unit-tier file defines.
	Helpers []string
}

// scanUnitTierStoreOpeners resolves the call graph of a Go test package.
// sources maps a file name to its contents; a file whose build constraint
// excludes it from a run without the integration tag is not part of the unit
// tier and is ignored entirely, because a unit-tier test can only call what
// the unit tier compiles.
//
// Only package-local functions called by bare name are followed, which is how
// cmd/bd's harness is called: a call through a variable or an interface
// method has no statically resolvable callee. Only //go:build constraints are
// read, so a legacy "// +build integration" line would read as untagged.
func scanUnitTierStoreOpeners(sources map[string]string) (unitTierStoreScan, error) {
	fset := token.NewFileSet()
	unit := map[string][]unitTierFunc{}
	var entries []string

	for name, src := range sources {
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			return unitTierStoreScan{}, fmt.Errorf("parse %s: %w", name, err)
		}
		if !compilesWithoutIntegrationTag(src) {
			continue
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			entry := isTestEntryPoint(fd)
			unit[fd.Name.Name] = append(unit[fd.Name.Name], unitTierFunc{calls: calledNames(fd)})
			if entry {
				entries = append(entries, name+": "+fd.Name.Name)
			}
		}
	}

	scan := unitTierStoreScan{}
	for _, helper := range cmdBdStoreHelpers {
		if len(unit[helper]) > 0 {
			scan.Helpers = append(scan.Helpers, helper)
		}
	}

	for _, entry := range entries {
		_, testFunc, _ := strings.Cut(entry, ": ")
		if reachesStoreHelper(testFunc, unit, map[string]int{}) {
			scan.Violations = append(scan.Violations, entry)
		}
	}
	slices.Sort(scan.Violations)
	return scan, nil
}

// reachesStoreHelper reports whether a unit-tier function reaches a store
// helper, memoizing into state (1 = in progress, 2 = yes, 3 = no) so a call
// cycle among the helpers terminates.
func reachesStoreHelper(name string, unit map[string][]unitTierFunc, state map[string]int) bool {
	switch state[name] {
	case 2:
		return true
	case 1, 3:
		return false
	}
	state[name] = 1

	reaches := slices.Contains(cmdBdStoreHelpers, name)
	for _, fn := range unit[name] {
		for _, callee := range fn.calls {
			if _, defined := unit[callee]; !defined {
				continue
			}
			if reachesStoreHelper(callee, unit, state) {
				reaches = true
				break
			}
		}
	}
	if reaches {
		state[name] = 2
	} else {
		state[name] = 3
	}
	return reaches
}

// compilesWithoutIntegrationTag reports whether a file with this source is
// part of the unit tier. It evaluates the file's own //go:build constraint
// with every tag but "integration" satisfied, so `//go:build cgo`,
// `//go:build integration` and `//go:build cgo && integration` all land where
// the tier puts them.
func compilesWithoutIntegrationTag(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//go:build ") {
			continue
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			// A malformed constraint is a compile error the build reports;
			// treat the file as unit tier so this scan still resolves it.
			return true
		}
		return expr.Eval(func(tag string) bool { return tag != "integration" })
	}
	return true
}

// isTestEntryPoint mirrors the go test naming rule: Test/Benchmark/Fuzz, then
// a name that does not start lowercase, then that tier's single testing
// parameter. A helper whose name starts with Test but whose signature is not
// testing's is not run as a test, so it is not an entry point here either.
func isTestEntryPoint(fd *ast.FuncDecl) bool {
	var param string
	switch {
	case strings.HasPrefix(fd.Name.Name, "Benchmark"):
		param = "B"
	case strings.HasPrefix(fd.Name.Name, "Fuzz"):
		param = "F"
	case strings.HasPrefix(fd.Name.Name, "Test"):
		param = "T"
		if fd.Name.Name == "TestMain" {
			param = "M"
		}
	default:
		return false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(fd.Name.Name, "Benchmark"), "Test")
	rest = strings.TrimPrefix(rest, "Fuzz")
	if rest == "" || (rest[0] >= 'a' && rest[0] <= 'z') {
		return false
	}
	params := fd.Type.Params
	if params == nil || len(params.List) != 1 {
		return false
	}
	star, ok := params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing" && sel.Sel.Name == param
}

// calledNames returns the bare-name calls in a function body.
func calledNames(fd *ast.FuncDecl) []string {
	if fd.Body == nil {
		return nil
	}
	var out []string
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok {
				out = append(out, id.Name)
			}
		}
		return true
	})
	return out
}

// cmdBdTestSources reads cmd/bd's test files, keyed by base name.
func cmdBdTestSources(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "cmd", "bd", "*_test.go"))
	if err != nil {
		t.Fatalf("glob cmd/bd test files: %v", err)
	}
	if len(paths) < 100 {
		t.Fatalf("found only %d cmd/bd test files; the glob is wrong", len(paths))
	}
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		sources[filepath.Base(path)] = readRepoFile(t, path)
	}
	return sources
}
