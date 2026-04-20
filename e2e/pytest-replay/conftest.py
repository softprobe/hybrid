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
CASE_PATH = str(
    _REPO_ROOT / "spec" / "examples" / "cases" / "fragment-happy-path.case.json"
)


@pytest.fixture(scope="module")
def softprobe() -> Softprobe:
    return Softprobe(base_url=RUNTIME_URL)


@pytest.fixture()
def replay_session(softprobe: Softprobe) -> Iterator[SoftprobeSession]:
    session = softprobe.start_session(mode="replay")
    try:
        yield session
    finally:
        session.close()


@pytest.fixture()
def app_url() -> str:
    return APP_URL


@pytest.fixture()
def case_path() -> str:
    return CASE_PATH
