//go:build cgo

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// doltNewGuardFiles are the test files whose dolt.New calls must create their
// database. In server mode dolt.New refuses to CREATE DATABASE unless
// Config.CreateIfMissing is set, and these tests open a fresh path each time
// (be-vf7n). A call that is not a fresh create must be listed in
// doltNewReopenSites instead.
var doltNewGuardFiles = []string{
	"config_test.go",
	"mol_integration_test.go",
	"mol_ready_gated_test.go",
	"rename_prefix_integration_test.go",
	"rename_prefix_repair_integration_test.go",
	"sql_integration_test.go",
	"update_metadata_integration_test.go",
	"contributor_routing_e2e_test.go",
}

// doltNewReopenSites names the enclosing test functions that deliberately open
// a database that must already exist. It is empty: every guarded call creates
// its database.
var doltNewReopenSites = map[string]bool{}

// doltNewViolations returns one message per dolt.New call in src whose config
// literal sets Path without CreateIfMissing, or whose config is not a literal
// the guard can read. It also returns the number of dolt.New calls it saw.
func doltNewViolations(fset *token.FileSet, filename string, src any) (violations []string, calls int, err error) {
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || doltNewReopenSites[fn.Name.Name] {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isDoltNewCall(call) {
				return true
			}
			calls++
			where := filename + ": " + fn.Name.Name + " at " + fset.Position(call.Pos()).String()
			if len(call.Args) < 2 {
				violations = append(violations, where+": dolt.New call has no config argument")
				return true
			}
			lit := doltConfigLiteral(call.Args[1])
			if lit == nil {
				violations = append(violations, where+": dolt.New config is not a &dolt.Config{...} literal, so CreateIfMissing cannot be checked")
				return true
			}
			hasPath, hasCreate := false, false
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "Path":
					hasPath = true
				case "CreateIfMissing":
					hasCreate = true
				}
			}
			if hasPath && !hasCreate {
				violations = append(violations, where+": dolt.Config sets Path but not CreateIfMissing: true")
			}
			return true
		})
	}
	return violations, calls, nil
}

// isDoltNewCall reports whether call is dolt.New(...).
func isDoltNewCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "New" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "dolt"
}

// doltConfigLiteral returns the composite literal of a &dolt.Config{...}
// argument, or nil when the argument has any other shape.
func doltConfigLiteral(arg ast.Expr) *ast.CompositeLit {
	unary, ok := arg.(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil
	}
	lit, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	typ, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || typ.Sel.Name != "Config" {
		return nil
	}
	if pkg, ok := typ.X.(*ast.Ident); !ok || pkg.Name != "dolt" {
		return nil
	}
	return lit
}

// TestDoltNewInTestsCreatesItsDatabase fails when a guarded test file opens a
// dolt store with a Path and no CreateIfMissing, which would leave the test
// database missing on the shared server. It parses the files and needs no
// Docker or Dolt server.
func TestDoltNewInTestsCreatesItsDatabase(t *testing.T) {
	fset := token.NewFileSet()
	var all []string
	calls := 0
	for _, name := range doltNewGuardFiles {
		violations, n, err := doltNewViolations(fset, name, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if n == 0 {
			t.Errorf("%s: no dolt.New calls found; the guard list is stale", name)
		}
		calls += n
		all = append(all, violations...)
	}
	if calls == 0 {
		t.Fatal("no dolt.New calls found in any guarded file")
	}
	if len(all) > 0 {
		t.Errorf("%d dolt.New call(s) would open a database that does not exist:\n  %s",
			len(all), strings.Join(all, "\n  "))
	}
}

// TestDoltNewGuardDetectsMissingFlag checks the guard's detector on inline
// source, so a regression in the detector cannot make the file scan pass
// vacuously.
func TestDoltNewGuardDetectsMissingFlag(t *testing.T) {
	tests := map[string]struct {
		src           string
		wantViolation int
	}{
		"path without flag is flagged": {
			src: `package p
func TestX() { dolt.New(ctx, &dolt.Config{Path: dbPath}) }`,
			wantViolation: 1,
		},
		"path with flag passes": {
			src: `package p
func TestX() { dolt.New(ctx, &dolt.Config{Path: dbPath, CreateIfMissing: true}) }`,
			wantViolation: 0,
		},
		"non-literal config is flagged": {
			src: `package p
func TestX() { dolt.New(ctx, cfg) }`,
			wantViolation: 1,
		},
		"unrelated config literal is ignored": {
			src: `package p
func TestX() { routing.New(ctx, &routing.Config{Path: p}) }`,
			wantViolation: 0,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			violations, _, err := doltNewViolations(token.NewFileSet(), "inline_test.go", tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(violations) != tc.wantViolation {
				t.Errorf("violations = %d, want %d: %v", len(violations), tc.wantViolation, violations)
			}
		})
	}
}
