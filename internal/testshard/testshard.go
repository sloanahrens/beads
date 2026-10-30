// Package testshard splits a package's top-level tests into shards so that an
// integration suite past the CI alarm can run as several jobs that each stay
// under it (be-wr0k).
//
// A shard is named "i/n" (index i of n, both counted from 0 for i and from 1
// for n): the selected tests are those whose name hashes to i modulo n. There
// is no committed manifest to go stale, so a test added later lands in exactly
// one shard without anyone updating a list; the cost is that the split is
// balanced by count only, not by wall time. A caller reads the whole assignment
// from a selection function rather than a file:
//
//	index, count, err := testshard.ParseSpec(os.Getenv("BD_TEST_SHARD"))
//	names, err := testshard.TestNames(".")
//	flag.Set("test.run", testshard.Pattern(names, index, count))
//
// The package is pure: no process, no Docker, no clock. Selection is a hash of
// the name, so the answer depends on nothing but its arguments.
package testshard

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// testPrefix is the prefix the go tool requires of a top-level test function.
const testPrefix = "Test"

// ParseSpec parses a shard spec of the form "i/n" with 0 <= i < n and n >= 1.
//
// It is strict: whitespace, a sign and any other decoration are errors, so
// "-0/3" is refused rather than read as shard 0. Callers pass user input, and
// a spec that is silently reinterpreted runs a different set of tests than the
// one that was asked for.
func ParseSpec(spec string) (index, count int, err error) {
	indexPart, countPart, ok := strings.Cut(spec, "/")
	if !ok {
		return 0, 0, fmt.Errorf("shard spec %q: want <index>/<count>", spec)
	}
	if strings.Contains(countPart, "/") {
		return 0, 0, fmt.Errorf("shard spec %q: want <index>/<count>", spec)
	}
	index, err = parseNonNegative(indexPart)
	if err != nil {
		return 0, 0, fmt.Errorf("shard spec %q: %w", spec, err)
	}
	count, err = parseNonNegative(countPart)
	if err != nil {
		return 0, 0, fmt.Errorf("shard spec %q: %w", spec, err)
	}
	if count < 1 {
		return 0, 0, fmt.Errorf("shard spec %q: count must be at least 1", spec)
	}
	if index >= count {
		return 0, 0, fmt.Errorf("shard spec %q: index must be less than count", spec)
	}
	return index, count, nil
}

// parseNonNegative reads a run of ASCII digits. strconv.Atoi alone would also
// accept "+1" and "-0", which ParseSpec means to refuse.
func parseNonNegative(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%q is not a decimal number", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a decimal number", s)
	}
	return n, nil
}

// Selects reports whether the test called name belongs to shard index of
// count. The assignment is a FNV-32a hash of the name modulo count, so it is
// stable across processes and independent of the names handed to it.
//
// Out-of-range arguments select nothing rather than panicking on the modulo.
func Selects(name string, index, count int) bool {
	if count < 1 || index < 0 || index >= count {
		return false
	}
	return int(hashName(name)%uint32(count)) == index
}

func hashName(name string) uint32 {
	h := fnv.New32a()
	// fnv.Write never returns an error.
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}

// TestNames lists the top-level test functions declared in the *_test.go files
// directly inside dir: sorted, without duplicates, and without the methods,
// helpers, benchmarks and fuzz targets that are not selectable by -test.run.
//
// A name is a test when it starts with "Test" and the next character is not a
// lower-case letter, which is the go tool's own rule (cmd/go and
// go/doc/comment agree): "Testing" and "Testhelper" are helpers, "Test_V1" and
// "TestMain" are not. TestMain is dropped anyway because the go tool runs it
// unconditionally, whatever -test.run says, so counting it in a shard would
// claim a selection the shard does not make.
//
// The signature is not checked. A function that looks like a test but takes
// the wrong arguments is a compile or vet error in the package, so it can never
// be silently skipped; matching a signature textually would instead skip tests
// that spell testing.T some other way (an import alias, a dot import) and lose
// them from every shard. Build constraints are not evaluated either: a name
// declared only in a file excluded from this build simply never matches the
// pattern, which leaves the partition intact.
func TestNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir, err)
	}
	seen := make(map[string]struct{})
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !isTestName(fn.Name.Name) {
				continue
			}
			if _, dup := seen[fn.Name.Name]; dup {
				continue
			}
			seen[fn.Name.Name] = struct{}{}
			names = append(names, fn.Name.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// isTestName reports whether name is a top-level test the go tool selects by
// -test.run, TestMain aside.
func isTestName(name string) bool {
	if name == "TestMain" || !strings.HasPrefix(name, testPrefix) {
		return false
	}
	if len(name) == len(testPrefix) {
		// A bare "Test" is run by the go tool, with an empty suffix.
		return true
	}
	first, _ := utf8.DecodeRuneInString(name[len(testPrefix):])
	return !unicode.IsLower(first)
}

// Pattern returns a -test.run pattern that matches exactly the names Selects
// assigns to shard index of count, and nothing else. The names are quoted, so a
// name holding a regexp metacharacter matches itself rather than widening the
// pattern.
//
// A shard with no names gets "^$", which matches nothing: passing "" to
// -test.run would run every test instead of none.
func Pattern(names []string, index, count int) string {
	var selected []string
	for _, name := range names {
		if Selects(name, index, count) {
			selected = append(selected, regexp.QuoteMeta(name))
		}
	}
	if len(selected) == 0 {
		return "^$"
	}
	return "^(" + strings.Join(selected, "|") + ")$"
}
