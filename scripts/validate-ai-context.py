#!/usr/bin/env python3
"""Validate docs-site/public/ai-context.md for agent consumption (required sections + date)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
AI_CONTEXT = REPO_ROOT / "docs-site" / "public" / "ai-context.md"

REQUIRED_HEADINGS = [
    "## Product model",
    "## CLI and SDK commands",
    "## Header and session rules",
    "## Troubleshooting pointers",
]

LAST_UPDATED = re.compile(r"Last updated:\s*(\d{4}-\d{2}-\d{2})\b", re.IGNORECASE)


def main() -> int:
    if not AI_CONTEXT.is_file():
        print(f"error: missing {AI_CONTEXT}", file=sys.stderr)
        return 1

    text = AI_CONTEXT.read_text(encoding="utf-8")
    if not LAST_UPDATED.search(text):
        print(
            "error: ai-context.md must contain 'Last updated: YYYY-MM-DD'",
            file=sys.stderr,
        )
        return 1

    missing = [h for h in REQUIRED_HEADINGS if h not in text]
    if missing:
        for h in missing:
            print(f"error: missing required heading {h!r}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
