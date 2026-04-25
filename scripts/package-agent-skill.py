#!/usr/bin/env python3
"""Package a Softprobe Agent Skill for GCS publishing."""

from __future__ import annotations

import argparse
import json
import shutil
import sys
import zipfile
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]


def parse_frontmatter(skill_md: Path) -> dict[str, str]:
    text = skill_md.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        raise ValueError(f"{skill_md} must start with YAML frontmatter")

    end = text.find("\n---\n", 4)
    if end == -1:
        raise ValueError(f"{skill_md} has unclosed YAML frontmatter")

    fields: dict[str, str] = {}
    for line in text[4:end].splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if ":" not in line:
            raise ValueError(f"unsupported frontmatter line in {skill_md}: {line!r}")
        key, value = line.split(":", 1)
        fields[key.strip()] = value.strip().strip("\"'")

    return fields


def validate_skill(skill_dir: Path, expected_name: str) -> dict[str, str]:
    skill_md = skill_dir / "SKILL.md"
    if not skill_md.is_file():
        raise ValueError(f"missing required file: {skill_md}")

    fields = parse_frontmatter(skill_md)
    name = fields.get("name")
    description = fields.get("description")
    if name != expected_name:
        raise ValueError(f"frontmatter name must be {expected_name!r}, got {name!r}")
    if not description:
        raise ValueError("frontmatter description is required")
    if not all(c.islower() or c.isdigit() or c == "-" for c in name):
        raise ValueError("frontmatter name must be lowercase hyphen-case")

    return fields


def zip_dir(source_dir: Path, zip_path: Path) -> None:
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(source_dir.rglob("*")):
            if path.is_dir():
                continue
            rel = path.relative_to(source_dir.parent)
            archive.write(path, rel.as_posix())


def package_skill(skill_name: str, version: str, out_dir: Path) -> None:
    source_dir = REPO_ROOT / "agent-skills" / skill_name
    if not source_dir.is_dir():
        raise ValueError(f"skill directory not found: {source_dir}")

    fields = validate_skill(source_dir, skill_name)

    package_root = out_dir / "agent-skills"
    staged_dir = package_root / skill_name
    if staged_dir.exists():
        shutil.rmtree(staged_dir)
    staged_dir.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(source_dir, staged_dir)

    zip_path = out_dir / f"{skill_name}.zip"
    if zip_path.exists():
        zip_path.unlink()
    old_checksum = out_dir / f"{skill_name}.zip.sha256"
    if old_checksum.exists():
        old_checksum.unlink()
    zip_dir(staged_dir, zip_path)
    (out_dir / "version").write_text(f"{version}\n", encoding="utf-8")
    (out_dir / "manifest.json").write_text(
        json.dumps(
            {
                "schemaVersion": 1,
                "name": fields["name"],
                "description": fields["description"],
                "version": version,
                "archive": f"{skill_name}.zip",
            },
            indent=2,
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )

    print(f"packaged {skill_name} {version}")
    print(f"archive: {zip_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("skill_name")
    parser.add_argument("--version", required=True)
    parser.add_argument("--out-dir", default="dist")
    args = parser.parse_args()

    try:
        package_skill(args.skill_name, args.version, REPO_ROOT / args.out_dir)
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
