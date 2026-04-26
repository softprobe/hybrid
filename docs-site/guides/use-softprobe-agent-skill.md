# Use the Softprobe agent skill

Install the Softprobe Test Writer skill in Claude Code or Cursor so your coding agent knows the official Softprobe replay workflow and can look up the right documentation while writing tests.

The skill is published from the Softprobe GCS bucket with the rest of the public artifacts:

```bash
SKILL_VERSION="$(curl -fsSL https://storage.googleapis.com/softprobe-published-files/agent/skills/softprobe-test-writer/version)"
curl -fsSLO "https://storage.googleapis.com/softprobe-published-files/agent/skills/${SKILL_VERSION}/softprobe-test-writer.zip"
```

## Claude Code

Install for your user account:

```bash
mkdir -p ~/.claude/skills
unzip -o softprobe-test-writer.zip -d ~/.claude/skills
```

Or install for one project:

```bash
mkdir -p .claude/skills
unzip -o softprobe-test-writer.zip -d .claude/skills
```

Use it directly:

```text
/softprobe-test-writer help me add a replay test for cases/checkout.case.json
```

## Cursor

Install for one project:

```bash
mkdir -p .cursor/skills
unzip -o softprobe-test-writer.zip -d .cursor/skills
```

Then ask Cursor Agent to use the Softprobe Test Writer skill when adding or debugging replay tests.

## What The Skill Contains

The archive expands to:

```text
softprobe-test-writer/
  SKILL.md
  references/
    docs-map.md
```

`SKILL.md` tells the agent to load **canonical context** first, then deeper docs as needed:

- **Canonical (always use first):** [`https://docs.softprobe.dev/ai-context.md`](https://docs.softprobe.dev/ai-context.md) — single markdown file with workflow rules, headers, and CLI surface. Source in repo: `docs-site/public/ai-context.md`.
- **Task index:** `references/docs-map.md` maps common tasks to official pages on `docs.softprobe.dev` (CLI reference, language guides, schemas).

When Softprobe behavior or docs change, maintainers update `ai-context.md` alongside the change so agents outside this repository stay accurate after the next docs deploy.

## Next

- [Replay in a Jest test](/guides/replay-in-jest) for the Jest replay flow.
- [Replay in pytest](/guides/replay-in-pytest) for Python tests.
- [Troubleshooting](/guides/troubleshooting) when mocks are not hit.
