---
title: Gemini CLI
description: Set up beads for Gemini CLI with a SessionStart hook that runs bd prime and GEMINI.md workflow guidance
---

Use Beads with Gemini CLI through a `SessionStart` hook and `GEMINI.md`
guidance.

Gemini CLI requires hook stdout to be valid JSON, so the hook runs
`bd prime --hook-json`, which wraps the workflow context in the envelope Gemini
expects. Add it to your Gemini settings — `~/.gemini/settings.json` for a
global install, `.gemini/settings.json` for a project-local one:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "bd prime --hook-json" }
        ]
      }
    ]
  }
}
```

Then write the instruction layer. Run `bd init` for the managed `AGENTS.md`
section and copy it into `GEMINI.md`, or pass the filename directly:

```bash
bd init --agents-file GEMINI.md
```

Already initialized? `bd init` refuses to rerun, so copy the beads section from
`AGENTS.md` into `GEMINI.md`, or paste the snippet `bd onboard` prints.

## Verify

```bash
bd prime --hook-json    # Prints the JSON envelope the hook returns
```

Restart Gemini CLI after changing settings, then start a session and ask what
work is ready.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
