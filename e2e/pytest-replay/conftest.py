"""Pytest fixtures for the pytest-replay e2e harness.

Runs against the same compose stack as `e2e/jest-replay/`:
- softprobe-runtime on 127.0.0.1:8080
- app (SUT) on 127.0.0.1:8081
- softprobe-proxy (egress listener) on 127.0.0.1:8084
- upstream on 127.0.0.1:8083

The softprobe-python package is expected to be importable (install with
`pip install -e ../../softprobe-python` before running).
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Iterator

import pytest

# Make the local softprobe-python package importable without requiring
# `pip install -e` when running straight from the repo. This keeps the
# harness hermetic and matches how e2e/jest-replay consumes the TS SDK.
_REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(_REPO_ROOT / "softprobe-python"))

from softprobe import Softprobe, SoftprobeSession  # noqa: E402


RUNTIME_URL = os.environ.get("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
APP_URL = os.environ.get("APP_URL", "http://127.0.0.1:8081")
API_KEY = os.environ.get("SOFTPROBE_API_KEY", "")
CASE_PATH = str(
    _REPO_ROOT / "spec" / "examples" / "cases" / "fragment-happy-path.case.json"
)


def _is_reachable(url: str) -> bool:
    import urllib.request
    import urllib.error

    try:
        with urllib.request.urlopen(url, timeout=2) as r:
            return r.status == 200
    except Exception:
        return False


@pytest.fixture(scope="module")
def softprobe() -> Softprobe:
    if not _is_reachable(f"{RUNTIME_URL}/health"):
        pytest.skip(f"softprobe-runtime unreachable at {RUNTIME_URL}")
    return Softprobe(base_url=RUNTIME_URL, api_token=API_KEY or None)


@pytest.fixture()
def replay_session(softprobe: Softprobe) -> Iterator[SoftprobeSession]:
    session = softprobe.start_session(mode="replay")
    try:
        yield session
    finally:
        session.close()


@pytest.fixture()
def app_url() -> str:
    if not _is_reachable(f"{APP_URL}/health"):
        pytest.skip(f"app unreachable at {APP_URL}")
    return APP_URL


@pytest.fixture()
def case_path() -> str:
    return CASE_PATH
