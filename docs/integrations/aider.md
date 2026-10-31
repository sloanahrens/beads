---
title: Aider
description: Set up beads with Aider's human-in-the-loop workflow, where the AI suggests bd commands you approve with /run
---

How to use beads with Aider.

[Aider](https://aider.chat/) is a human-in-the-loop AI pair programming tool: unlike autonomous agents such as [Claude Code](/integrations/claude-code), it doesn't run shell commands on its own. The beads integration works with that design — the AI **suggests** `bd` commands, and you confirm each one with aider's `/run` command.

## Setup

### Prerequisites

- beads installed and initialized in your project (see [Installation](/getting-started/installation)); run `bd init` if there's no `.beads/` directory yet
- aider installed: `pip install aider-chat` (or `pipx install aider-chat`)

### Quick Setup

Two files give aider the beads context. First, `.aider.conf.yml` loads the beads instructions into aider's read-only context:

```yaml
# Beads Issue Tracking Integration for Aider

# Load Beads workflow instructions for the AI
# This file is marked read-only and cached for efficiency
read:
  - .aider/BEADS.md
```

Then write `.aider/BEADS.md`, the workflow rules the AI follows. Run `bd init` for the canonical section in `AGENTS.md` and copy it in, or use this minimal version:

```markdown
This project uses bd (beads) for issue tracking.

- Suggest commands for me to run with /run; do not assume you can run them.
- Use `bd ready` to find work, `bd create` for new issues, and `bd show <id>` for details.
- Track all work in bd — never markdown TODO lists.
- Suggest `bd dolt push` at the end of a session.
```

You own both files: edit `.aider/BEADS.md` freely to add project-specific instructions.

Aider has no session-start hook here, so inject context yourself at session start (see [Workflow](#workflow)).

### Verify Setup

Start `aider` in the project and ask what work is ready. It should suggest `bd ready`.

### Remove the Integration

Delete `.aider.conf.yml` and `.aider/BEADS.md`. Aider reads no other beads files.

## Workflow

### Start Session

```bash
# Aider will have access to issues via .aider.conf.yml
aider

# Or manually inject context
bd prime | aider --message-file -
```

### Inside Aider

Aider's own commands start with `/` (`/run`, `/add`, `/help`); anything else is a message to the AI. The AI suggests bd commands, and you execute the ones you approve with `/run`:

```text
You: What issues are ready to work on?

Aider: Let me check the available work. Run:
/run bd ready

You: Let's work on bd-42

Aider: To claim it, run:
/run bd update bd-42 --claim
```

To give the AI full bd context mid-session, run `/run bd prime` — the AI reads the output and picks up the complete workflow guide.

### During Work

Use bd commands alongside aider:

```bash
# In another terminal or after exiting aider
bd create "Found bug during work" --deps discovered-from:bd-42 --json
bd update bd-42 --claim
bd ready

# Link an already-created issue as discovered work
bd dep add bd-77 bd-42 --type discovered-from
```

### End Session

```bash
bd dolt push
```

## Best Practices

1. **Keep issues visible** - Use `bd prime` to inject issue context
2. **Push regularly** - Run `bd dolt push` after significant changes
3. **Use discovered-from** - Track issues found during work
4. **Document context** - Include descriptions in issues
5. **Aider commits, bd pushes** - Aider auto-commits your code changes; issue data moves separately with `bd dolt push`

## Example Workflow

```bash
# 1. Check ready work
bd ready

# 2. Start aider with issue context
aider --message "Working on bd-42: Fix auth bug"

# 3. Work in aider...

# 4. Create discovered issues
bd create "Found related bug" --deps discovered-from:bd-42 --json

# 5. Complete and push
bd close bd-42 --reason "Fixed"
bd dolt push
```

## Troubleshooting

### Config not loading

```bash
# Check config exists and points at the instructions file
cat .aider.conf.yml
ls .aider/BEADS.md
```

Aider reads `.aider.conf.yml` at startup, so restart aider (`/exit`, then `aider`) after editing it.

### Issues not visible

```bash
# Use bd prime to inject issue context
bd prime | aider --message-file -

# Or check database health
bd doctor
```

## See Also

- [Claude Code](/integrations/claude-code)
- [IDE Setup](/getting-started/ide-setup)
- [Quickstart](/getting-started/quickstart)
- [Aider documentation](https://aider.chat/docs/)
- [AGENTS.md](https://github.com/gastownhall/beads/blob/main/AGENTS.md) - the full bd agent workflow guide
