#!/usr/bin/env python3
"""
Grade assertions for Softprobe skill evals.
Usage: python3 grade.py <iteration_dir>
"""
import json
import os
import sys


def check_assertion(assertion, eval_dir):
    check = assertion["check"]
    passed = False
    evidence = ""

    if check == "file_exists":
        path = assertion["file"]
        passed = os.path.exists(path)
        evidence = f"{'EXISTS' if passed else 'MISSING'}: {path}"

    elif check == "file_contains":
        path = assertion["file"]
        pattern = assertion["pattern"]
        if os.path.exists(path):
            content = open(path).read()
            passed = pattern in content
            evidence = f"{'FOUND' if passed else 'NOT FOUND'} '{pattern}' in {path}"
        else:
            evidence = f"File missing: {path}"

    elif check == "file_ordering":
        # Check that pattern_before appears before pattern_after in the file
        path = assertion["file"]
        pattern_before = assertion["pattern_before"]
        pattern_after = assertion["pattern_after"]
        if os.path.exists(path):
            content = open(path).read()
            idx_before = content.find(pattern_before)
            idx_after = content.find(pattern_after)
            if idx_before == -1:
                evidence = f"'{pattern_before}' not found in {path}"
            elif idx_after == -1:
                evidence = f"'{pattern_after}' not found in {path}"
            else:
                passed = idx_before < idx_after
                evidence = (f"'{pattern_before}' at pos {idx_before} "
                           f"{'BEFORE' if passed else 'AFTER'} "
                           f"'{pattern_after}' at pos {idx_after}")
        else:
            evidence = f"File missing: {path}"

    elif check == "file_not_contains":
        path = assertion["file"]
        pattern = assertion["pattern"]
        if os.path.exists(path):
            content = open(path).read()
            passed = pattern not in content
            evidence = f"{'NOT PRESENT (good)' if passed else 'FOUND (bad)'} '{pattern}' in {path}"
        else:
            passed = True
            evidence = f"File missing (treated as pass — no bad import possible): {path}"

    elif check == "json_contains":
        path = assertion["file"]
        json_path = assertion["path"]
        pattern = assertion["pattern"]
        if os.path.exists(path):
            try:
                data = json.load(open(path))
                keys = json_path.split(".")
                val = data
                for k in keys:
                    val = val[k]
                passed = pattern in str(val)
                evidence = f"scripts.start = '{val}' — '{pattern}' {'FOUND' if passed else 'NOT FOUND'}"
            except Exception as e:
                evidence = f"Error reading {path}: {e}"
        else:
            evidence = f"File missing: {path}"

    elif check == "output_contains_one_of":
        patterns = assertion["patterns"]
        outfile = os.path.join(eval_dir, assertion["file"])
        if os.path.exists(outfile):
            content = open(outfile).read().lower()
            for p in patterns:
                if p.lower() in content:
                    passed = True
                    evidence = f"Found '{p}' in {outfile}"
                    break
            if not passed:
                evidence = f"None of {patterns} found in {outfile}"
        else:
            evidence = f"Output file missing: {outfile}"

    return {
        "text": assertion["description"],
        "passed": passed,
        "evidence": evidence,
    }


def grade_run(eval_dir, run_name):
    meta_path = os.path.join(eval_dir, "eval_metadata.json")
    if not os.path.exists(meta_path):
        return None
    meta = json.load(open(meta_path))
    run_dir = os.path.join(eval_dir, run_name)

    results = []
    for assertion in meta.get("assertions", []):
        result = check_assertion(assertion, run_dir)
        results.append(result)

    passed = sum(1 for r in results if r["passed"])
    total = len(results)

    grading = {
        "eval_id": meta["eval_id"],
        "eval_name": meta["eval_name"],
        "run": run_name,
        "pass_rate": passed / total if total > 0 else 0,
        "passed": passed,
        "total": total,
        "expectations": results,
    }

    out_path = os.path.join(run_dir, "grading.json")
    with open(out_path, "w") as f:
        json.dump(grading, f, indent=2)
    print(f"  {run_name}: {passed}/{total} passed → {out_path}")
    return grading


def main():
    iteration_dir = sys.argv[1] if len(sys.argv) > 1 else "."
    evals = ["eval-fresh-bootstrap", "eval-capture-replay", "eval-import-fix"]
    runs = ["with_skill", "without_skill"]

    all_results = []
    for eval_name in evals:
        eval_dir = os.path.join(iteration_dir, eval_name)
        if not os.path.isdir(eval_dir):
            print(f"  SKIP (not found): {eval_dir}")
            continue
        print(f"\n{eval_name}:")
        for run in runs:
            result = grade_run(eval_dir, run)
            if result:
                all_results.append(result)

    print("\n--- Summary ---")
    for r in all_results:
        bar = "█" * r["passed"] + "░" * (r["total"] - r["passed"])
        print(f"  {r['eval_name']:30s} {r['run']:15s} {r['passed']}/{r['total']} [{bar}]")


if __name__ == "__main__":
    main()
