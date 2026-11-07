---
title: "bd formula"
description: "Manage workflow formulas"
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --doc formula`.

Manage workflow formulas - the source layer for molecule templates.

Formulas are TOML/JSON files that define workflows with composition rules.
Define formulas, cook them into protos, then pour or wisp them into work.

Search paths (in order):
  1. &lt;resolved-beads-dir&gt;/formulas/ (active project)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/ (user)
  4. $GT_TOWN_ROOT/.beads/formulas/ (shared workspace root, if GT_TOWN_ROOT set)

Commands:
  list    List available formulas from all search paths
  show    Show formula details, steps, and composition rules
  lint    Report keys, gate types and vars a formula would silently drop
  schema  Show the formula schema index (alias: primitives)

Discovering primitives:
  bd formula schema                 # list every declared formula struct
  bd formula schema gate            # show Gate fields, types, and tags
  bd formula primitives gate        # alias; same handler as 'schema'
  examples/formulas/primitives/     # curated, smoke-tested wired fixtures
  docs/workflows/formulas.md          # narrative reference

```
bd formula [command]
```

## bd formula convert

Convert formula files from JSON to TOML format.

TOML format provides better ergonomics:
  - Multi-line strings without \n escaping
  - Human-readable diffs
  - Comments allowed

The convert command reads a .formula.json file and outputs .formula.toml.
The original JSON file is preserved (use --delete to remove it).

Examples:
  bd formula convert shiny              # Convert shiny.formula.json to .toml
  bd formula convert ./my.formula.json  # Convert specific file
  bd formula convert --all              # Convert all JSON formulas
  bd formula convert shiny --delete     # Convert and remove JSON file
  bd formula convert shiny --stdout     # Print TOML to stdout

```
bd formula convert <formula-name|path> [--all] [flags]
```

**Flags:**

```
      --all      Convert all JSON formulas
      --delete   Delete JSON file after conversion
      --stdout   Print TOML to stdout instead of file
```

## bd formula lint

Lint a .formula.toml file, or every .formula.toml directly in a directory,
against the strict decoder bd cooks with. It reports, per file:

  unknown_key        a key bd does not know (it used to be dropped silently)
  invalid_value      a var field of the wrong type (dropped the same way)
  invalid_gate_type  a step gate type no watcher or command resolves
  validation         structural errors, e.g. a var both required and defaulted
  syntax             the file is not valid TOML

A formula that extends another is resolved against its own directory, then
--search-path. Exits 1 when any file has a problem (machine mode: the
report is data and error.kind is invalid_args, exit 27).

Examples:
  bd formula lint ~/gt/.beads/formulas
  bd formula lint mol-shutdown-dance.formula.toml --json

```
bd formula lint <path|dir> [flags]
```

**Flags:**

```
      --search-path strings   Additional paths to search when resolving extends
```

## bd formula list

List all formulas from search paths.

Search paths (in order of priority):
  1. &lt;resolved-beads-dir&gt;/formulas/ (active project - highest priority)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/ (user)
  4. $GT_TOWN_ROOT/.beads/formulas/ (shared workspace root, if GT_TOWN_ROOT set)

Formulas in earlier paths shadow those with the same name in later paths.

To list the declared formula schema structs an agent can write inside a .formula.toml,
use 'bd formula schema' (alias: 'bd formula primitives').

Examples:
  bd formula list
  bd formula list --json
  bd formula list --type workflow
  bd formula list --type convoy

```
bd formula list [flags]
```

**Flags:**

```
      --type string   Filter by type (workflow, expansion, aspect, convoy)
```

## bd formula schema

Show the formula schema index: every exported struct declared
in a .formula.toml/.formula.json, with field names, types, and tags.

The index is generated from internal/formula/types.go via go:generate; the
struct definitions are the source of truth, so this list cannot drift. It is
structural reference, not proof that every declared runtime behavior is wired.

Examples:
  bd formula schema                 # list every declared schema struct
  bd formula schema gate            # show Gate fields
  bd formula primitives step        # alias; shows Step fields
  bd formula schema --json          # machine-readable index

Curated smoke-tested fixtures for wired primitives live in
examples/formulas/primitives/ (with a smoke harness that proves they work).

```
bd formula schema [primitive] [flags]
```

**Aliases:** primitives

## bd formula show

Show detailed information about a formula.

Displays:
  - Formula metadata (name, type, description)
  - Variables with defaults and constraints
  - Steps with dependencies
  - Composition rules (extends, aspects, expansions)
  - Bond points for external composition

To inspect the structure of an individual primitive (e.g. Gate, ExpandRule)
rather than a user-authored formula, use 'bd formula schema &lt;primitive&gt;'.

Examples:
  bd formula show shiny
  bd formula show rule-of-five
  bd formula show security-audit --json

```
bd formula show <formula-name> [flags]
```
