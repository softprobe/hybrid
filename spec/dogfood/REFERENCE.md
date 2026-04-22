# Dogfood reference build

## What this is

The dogfood suite captures real CLI interactions and replays them against new
builds to detect regressions. The **reference build** is the pinned commit
that produced the golden case files in `spec/examples/cases/`.

## Current reference

```
main @ cfa4d2a
```

Post PD5.3a (released Go module tags), the reference will be a released tag
(`v0.5.0` or later) rather than a bare `main` SHA.

## Invariant

> A case refresh must land in a PR that contains **no** changes to
> `softprobe-runtime/`, `softprobe-go/`, `softprobe-js/`, `softprobe-python/`,
> or `softprobe-java/`. Case files record protocol behavior at a point in time;
> mixing behavioral changes and case refreshes makes regressions ambiguous.

CI enforces this by running `make capture-refresh --dry-run` and failing if
both code and case changes appear in the same PR.

## Refreshing the reference

1. Run `make capture-refresh` on a clean checkout of `main`.
2. Inspect the diff in `spec/examples/cases/`.
3. Open a PR containing only the case diff; no code changes.
4. Merge. Update `main @ <new-sha>` above.
