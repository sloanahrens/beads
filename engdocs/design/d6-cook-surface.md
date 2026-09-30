# D6: bd is the one formula engine — cook tree, strict decode, overlays (be-2gw.1, be-2gw.2)

Decision D6 makes bd the only formula engine: gastown renders its step
checklist from `bd cook` instead of its own parser. This note is the
contract gastown codes against. Machine mode, the envelope and the error
kinds are D1 (`d1-machine-surface.md`).

## `bd cook <name|path>` under machine mode

`BD_MACHINE=1 bd cook <formula> [--var k=v]... [--search-path DIR]... [--mode compile|runtime]`

Without `--persist` or `--dry-run`, `data` is the cooked tree. It comes from
`cookPipeline`, the same function pour, wisp, mol bond and mol seed use, so
the rendered steps and the poured beads cannot diverge. The pipeline runs:
extends, `--var` validation, control flow, advice, inline expansion,
compose expand/map, aspects, the overlay, step conditions, standalone
expansion templates. Non-persist cook opens no database.

```json
{
  "formula": "wf-basic",
  "source": "/abs/path/wf-basic.formula.toml",
  "type": "workflow",
  "description": "Ship api to staging",
  "phase": "",
  "mode": "runtime",
  "vars": [{"name": "env", "description": "Target environment", "required": false,
            "default": "staging", "enum": ["staging", "prod"], "pattern": "",
            "type": "", "value": "staging", "provided": false}],
  "unresolved_vars": [],
  "overlay": {"path": "/rig/formula-overlays/wf-basic.toml"},
  "warnings": ["overlay references unknown step \"gone\" (stale override)"],
  "steps": [
    {"id": "build", "title": "Build api", "description": "", "notes": "",
     "type": "task", "priority": 2, "assignee": "builder", "labels": [],
     "needs": ["design"], "waits_for": "",
     "gate": {"type": "gh:run", "await_id": "ci.yml", "timeout": "1h"},
     "metadata": {}, "source_formula": "wf-basic", "source_location": "steps[1]",
     "children": []}
  ]
}
```

- Steps are in pour order; `children` nest. A child's poured issue id is
  `<root>.<parent id>.<child id>`.
- `needs` is `depends_on` then `needs`, deduplicated.
- `type` is the issue type pour gives the step: `task` by default, `epic` for
  a parent with no declared type. A custom type the database has not
  registered is flattened to `task` at pour time, which the tree cannot see.
- A step `gate` is its own gate issue at pour time (`<root>.gate-<step id>`).
- Runtime mode (default): defaults and `--var` are substituted into title,
  description, notes and gate fields. A var with no value stays a
  placeholder and is listed in `unresolved_vars`. `--mode=runtime` makes
  that an `invalid_args` error. `--mode=compile` keeps every placeholder.
- `vars` is sorted by name. `value` is null in compile mode or without a value.
- `overlay` is null when no overlay applied.

Errors: an invalid formula, overlay or `--var` value is `invalid_args` (27),
with `detail = {file, key, line, problems: [{kind, key, line, message}]}`.
An unknown formula is `not_found` (20). The message is one line.

Outside machine mode `bd cook` keeps printing the resolved formula, as before.

## Strict decode

`ParseTOML` decodes with `toml.Decode` and rejects:

| kind | what |
|---|---|
| `unknown_key` | any key the `Formula` struct does not decode; keys under an unknown table fold into it |
| `unknown_key`, `invalid_value` | a `[vars.<name>]` field outside description/default/required/enum/pattern/type, or of the wrong TOML type |
| `invalid_gate_type` | a step gate whose type is not `gh:run`, `gh:pr` (or `gh:run:`/`gh:pr:` forms), `timer`, `bead`, `human`, `mail`, or a `{{var}}` |
| `validation` | `Formula.Validate` failures after extends, e.g. `required = true` with a `default` |
| `syntax` | not valid TOML |

TOML does not export key positions, so `locateKeyLines` scans the file
itself. A property test checks it against every key toml reports for every
formula in the repo, and it agrees on all 49 town formulas.

pour, wisp, mol bond and mol seed report an invalid formula directly. They
used to fall through to a proto-ID lookup and print "not found". JSON
formulas are still decoded loosely.

## `bd formula lint <path|dir> [--search-path DIR]`

Lints one file or every `.formula.toml` directly in a directory. A formula
that extends another resolves against its own directory, then the search
paths. Output with `--json`:

```json
{"checked": 49, "failed": 26,
 "files": [{"file": "/abs/mol-shutdown-dance.formula.toml",
            "problems": [{"kind": "unknown_key", "key": "steps.gate.condition",
                          "line": 235, "message": "unknown key (bd would silently drop it)"}]}]}
```

The text form prints one `file:line: ✗ [kind] key: message` line per
problem. The command exits 1 when any file has a problem. In machine mode
the report is `data`, `error.kind` is `invalid_args` and the exit is 27.

## Overlays

Overlays are read from one directory: config `formula.overlay-dir`
(yaml-only; env `BD_FORMULA_OVERLAY_DIR`; a relative path resolves against
the beads directory). Unset means no overlays. The file is
`<dir>/<formula>.toml` in gastown's format (`[[step-overrides]]` with
`step_id`, `mode` = replace|append|skip, `description`), decoded strictly.
The overlay applies to top-level steps after every expansion, in pour, wisp
and cook alike, so the poured beads carry the overlay text. `skip` rewires
dependents onto the skipped step's own predecessors. Gastown's rig-then-town
overlay search is replaced by gastown setting this key per rig (D5).

## Migration (staged)

Strict decode is staged, because this bd is installed before gastown cleans
its formulas (gt-fd2cu.3; 26 of 49 town formulas carry dropped keys, among
them every `mol-dog-*` through the dead `[squash]` block).

- Strict now: `bd cook` under machine mode (the gastown renderer path),
  `bd formula lint`, `--strict` on cook/pour/wisp, config `formula.strict=true`.
- Everywhere else (legacy cook, pour, wisp, mol bond/seed, formula list):
  each dropped key or invalid gate type is one stderr warning,
  `Warning: <file>:<line>: <key>: ...`, and the formula cooks as before.
  Validation failures such as required+default were fatal before and stay so.
- `bd capabilities --json` carries this in `notes`.
- The flip to strict everywhere is `formula.StrictDecode = true`, one line,
  once `bd formula lint` is clean over the town.
