# D6: `bd cook --json` and strict formula decode — implementation plan (be-2gw.1, be-2gw.2)

> For agentic workers: execute with superpowers:executing-plans and
> test-driven-development. Steps use checkbox syntax.

**Goal:** bd is the one formula engine. Machine callers get the cooked step
tree from `bd cook` under `BD_MACHINE`, and a formula that carries keys bd
would silently drop fails to cook with file, key and line.

**Architecture:** Strictness lives in `internal/formula` (decode, locate,
error type, overlay). `cmd/bd` gains a cook-tree builder over the *same*
pipeline pour and wisp use, so the rendered checklist equals the beads by
construction, plus `bd formula lint`. Errors surface as `invalid_args` (27);
a missing formula is `not_found` (20).

**Tech stack:** Go, BurntSushi/toml v1.6 (`toml.Decode` + `MetaData.Undecoded`),
cobra. No Dolt, no Docker for any formula test.

**Spec:** bead be-2gw (NOTES), be-2gw.1, be-2gw.2;
`engdocs/design/d1-machine-surface.md`; deep-review B3-03/B3-04 and
`d6-formula-evidence.md` (operator research tree).

## Global constraints

- Machine envelope and kinds per d1-machine-surface.md; legacy `--json`
  shapes outside machine mode stay unchanged.
- Do not change formula semantics beyond rejecting what was silently dropped.
- Overlays come from ONE directory: config key `formula.overlay-dir`
  (yaml-only; env `BD_FORMULA_OVERLAY_DIR`). Unset means no overlays.
- Output is deterministic: vars sorted by name, steps in cook order.

## File map

| File | Responsibility |
|---|---|
| `internal/formula/strict.go` (new) | `ErrInvalidFormula`, `Problem`, `FormulaError`, strict TOML decode, var-table and gate-type checks |
| `internal/formula/keyline.go` (new) | best-effort TOML key-path to line locator |
| `internal/formula/overlay.go` (new) | overlay file model, strict load, apply (replace/append/skip) |
| `internal/formula/parser.go` | `ParseTOML` strict, `ErrFormulaNotFound`, validation errors typed |
| `cmd/bd/cook.go` | shared `resolveFormulaForCook` pipeline; machine-mode tree output |
| `cmd/bd/cook_tree.go` (new) | `cookTree` shape and builder |
| `cmd/bd/formula_lint.go` (new) | `bd formula lint <path|dir>` |
| `cmd/bd/envelope.go` | classify `ErrInvalidFormula` as invalid_args, `ErrFormulaNotFound` as not_found (shared helper, own commit) |
| `cmd/bd/pour.go`, `wisp.go`, `mol_bond.go` | a broken formula is a one-line error, never a fall-through to proto lookup |
| `cmd/bd/testdata/cook/` (new) | formulas + golden tree file |

## Task 1: strict decode (be-2gw.2)

- [ ] Failing tests in `internal/formula/strict_test.go`: unknown top-level
  key (`[squash]`), unknown step key (`need = [...]`), `gate.condition`
  inside an inline table with the right line, `gate.type = "conditional"`
  rejected, `gate.type = "gh:run"`/`timer`/`bead`/`human`/`mail` and
  `{{var}}` accepted, unknown `[vars.x]` key rejected, error is one line and
  `errors.Is(err, ErrInvalidFormula)`.
- [ ] Implement `DecodeTOMLStrict(data) (*Formula, []Problem, error)`:
  `toml.Decode`, `md.Undecoded()` → `unknown_key` problems; walk `md.Keys()`
  under `vars.<name>.` against the VarDef field set → `unknown_key`; walk steps,
  children and template for gates → `invalid_gate_type`. Lines come from
  `locateKeyLines` (Nth undecoded occurrence ↔ Nth located line).
- [ ] `ParseTOML` returns `*FormulaError` when problems exist; `ParseFile`
  fills `File`. `Validate` failures from `Resolve` wrap `ErrInvalidFormula`
  and render on one line (`required:true` + `default` included).
- [ ] `loadFormula` returns `ErrFormulaNotFound`; `bd cook <path>` falls back
  to `ParseFile` only on not-found, so a broken named formula is reported,
  not masked by "read <name>: no such file".
- [ ] Commit.

## Task 2: one-line failure at pour/wisp/bond

- [ ] Failing test: a pour/wisp resolve of a formula with an unknown key
  returns the strict error (not "not found as formula or proto ID").
- [ ] In pour, wisp, mol bond: `errors.Is(err, formula.ErrInvalidFormula)`
  → `failKind(kindInvalidArgs, ...)`, same branch as `ErrVarValidation`.
- [ ] `errorKindOf`: `ErrInvalidFormula` → invalid_args, `ErrFormulaNotFound`
  → not_found. Separate commit (shared helper).

## Task 3: `bd formula lint <path|dir>`

- [ ] Failing test over a temp dir with one clean and two dirty formulas:
  report lists every problem with file, key, line, kind; exit non-zero.
- [ ] Lint runs `DecodeTOMLStrict` plus `Validate` per `.formula.toml`
  (non-recursive dir scan, sorted). JSON shape:
  `{"files":[{"file","problems":[{"kind","key","line","message"}]}],"checked":N,"failed":M}`.
  Human output: `file:line: key: message`. Problems → exit 1; machine mode
  keeps the report as `data` and sets `error.kind=invalid_args`.

## Task 4: overlays from one directory (be-2gw.1)

- [ ] Failing tests: replace, append (`"\n"` join, as gastown), skip rewires
  `needs` and `depends_on` of dependents to the skipped step's own; unknown
  step is a warning; unknown overlay key is an error; missing file is no-op.
- [ ] `LoadOverlay(dir, name)`, `ApplyOverlay(f, ov) []string`.
- [ ] Config key `formula.overlay-dir` (default "", yaml-only); relative
  paths resolve against the beads dir.

## Task 5: `bd cook --json` tree under BD_MACHINE (be-2gw.1)

- [ ] Extract `resolveFormulaForCook(nameOrPath, searchPaths, vars, overlayDir)`
  from `resolveAndCookFormulaWithVars` (extends → control flow → advice →
  inline expand → compose expand/map → aspects → overlay → conditions →
  standalone expansion). Pour/wisp call it; behavior unchanged when no
  overlay dir is set.
- [ ] `buildCookTree(f, vars, mode)`:
  `{formula, source, type, description, phase, mode, vars:[...sorted],
  unresolved_vars, overlay, warnings, steps:[{id,title,description,notes,type,
  priority,assignee,labels,needs,waits_for,gate,metadata,source_formula,
  source_location,children}]}`; `needs` = depends_on ∪ needs in order, deduped.
  Runtime substitution with defaults + `--var`; `--mode=compile` keeps
  placeholders; `--mode=runtime` with missing vars is invalid_args.
- [ ] runCook: machine mode and no `--persist`/`--dry-run` → tree.
- [ ] Golden test: cook every formula under `cmd/bd/testdata/cook/formulas`
  into `cmd/bd/testdata/cook/tree.golden.json` (`-update` flag regenerates),
  and assert two runs are byte-identical.
- [ ] Capabilities test asserts `cook` and `formula lint` are listed.

## Task 6: docs and gates

- [ ] Add a "D6 cook surface" section to `d1-machine-surface.md`.
- [ ] Gates by exit code: `go build ./...`, `go vet ./...`,
  `make ci-pr-lint`, `go test ./internal/formula/`, the cook/formula tests in
  `./cmd/bd`, `./cmd/bd/protocol` under `gt slot run`.
- [ ] `om review -base origin/main` at midpoint (after Task 3) and end.
