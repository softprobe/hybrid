#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

AJV=(npx -y ajv-cli@5)

schema_files=()
while IFS= read -r file; do
  schema_files+=("$file")
done < <(find spec/schemas -maxdepth 1 -name '*.json' | sort)

for schema in "${schema_files[@]}"; do
  refs=()
  for ref in "${schema_files[@]}"; do
    if [[ "$ref" != "$schema" ]]; then
      refs+=(-r "$ref")
    fi
  done
  "${AJV[@]}" compile -s "$schema" "${refs[@]}" --spec=draft2020 >/dev/null
done

while IFS= read -r example; do
  base="$(basename "$example")"
  case "$base" in
    *.case.json)
      "${AJV[@]}" validate -s spec/schemas/case.schema.json -r spec/schemas/case-trace.schema.json -d "$example" --spec=draft2020 >/dev/null
      ;;
    *.rule.json)
      "${AJV[@]}" validate -s spec/schemas/rule.schema.json -d "$example" --spec=draft2020 >/dev/null
      ;;
    *-trace.json)
      "${AJV[@]}" validate -s spec/schemas/case-trace.schema.json -d "$example" --spec=draft2020 >/dev/null
      ;;
  esac
done < <(find spec/examples -type f -name '*.json' | sort)

echo "spec validation passed"
