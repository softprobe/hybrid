# Replay in pytest

The Python flow mirrors [Replay in Jest](/guides/replay-in-jest) — the same case file, the same control API, the same `findInCase` + `mockOutbound` pattern. If you've read the Jest guide, skim this for the Python-specific bits.

## 1. Install the SDK

::: warning Not yet on PyPI
`softprobe` is not yet published to PyPI. Until it is, install `pytest` from
PyPI and consume `softprobe` from source in the
[softprobe monorepo](https://github.com/softprobe/softprobe) (`softprobe-python/`)
— see that package's `README.md` for the editable-install recipe.
:::

```bash
# Planned — not yet published.
pip install softprobe pytest
```

## 2. The minimum working test

```python
# tests/test_checkout_replay.py
import os
from pathlib import Path
import pytest
import urllib.request

from softprobe import Softprobe

RUNTIME_URL = os.environ.get("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
APP_URL = os.environ.get("APP_URL", "http://127.0.0.1:8082")

softprobe = Softprobe(base_url=RUNTIME_URL)


@pytest.fixture(scope="module")
def session():
    s = softprobe.start_session(mode="replay")
    s.load_case_from_file(Path(__file__).parent.parent / "cases" / "checkout-happy-path.case.json")

    hit = s.find_in_case(
        direction="outbound",
        method="POST",
        host_suffix="stripe.com",
        path_prefix="/v1/payment_intents",
    )

    s.mock_outbound(
        direction="outbound",
        method="POST",
        host_suffix="stripe.com",
        path_prefix="/v1/payment_intents",
        response=hit.response,
    )

    yield s
    s.close()


def test_charges_the_captured_card(session):
    req = urllib.request.Request(
        f"{APP_URL}/checkout",
        method="POST",
        data=b'{"amount": 1000, "currency": "usd"}',
        headers={
            "content-type": "application/json",
            "x-softprobe-session-id": session.id,
        },
    )
    with urllib.request.urlopen(req) as res:
        assert res.status == 200
        body = res.read().decode()
        assert '"status": "paid"' in body
```

## 3. Run it

```bash
pytest tests/test_checkout_replay.py -v
```

Expected:

```
tests/test_checkout_replay.py::test_charges_the_captured_card PASSED    [100%]
```

## API parity with Jest

| JavaScript | Python |
|---|---|
| `new Softprobe({ baseUrl })` | `Softprobe(base_url=...)` |
| `softprobe.startSession({ mode: 'replay' })` | `softprobe.start_session(mode="replay")` |
| `session.loadCaseFromFile(path)` | `session.load_case_from_file(path)` |
| `session.findInCase({ direction, method, hostSuffix })` | `session.find_in_case(direction=..., method=..., host_suffix=...)` |
| `session.mockOutbound({ ..., response })` | `session.mock_outbound(..., response=...)` |
| `session.clearRules()` | `session.clear_rules()` |
| `session.setPolicy({ externalHttp: 'strict' })` | `session.set_policy(external_http="strict")` |
| `session.close()` | `session.close()` |

Python uses `snake_case`; semantics and HTTP wire shape are identical.

## Mutating a captured response

```python
import json

hit = session.find_in_case(direction="outbound", host_suffix="stripe.com")
body = json.loads(hit.response.body)
body["source"]["card"]["number"] = os.environ.get("TEST_CARD", "4111111111111111")

session.mock_outbound(
    host_suffix="stripe.com",
    response={
        "status": hit.response.status,
        "headers": hit.response.headers,
        "body": json.dumps(body),
    },
)
```

## Parametrizing over captured scenarios

If you have a folder of case files, you can drive each one as a pytest parameter:

```python
import glob
from pathlib import Path

CASE_DIR = Path(__file__).parent.parent / "cases"
CASE_FILES = sorted(glob.glob(str(CASE_DIR / "checkout-*.case.json")))


@pytest.mark.parametrize("case_path", CASE_FILES)
def test_replay_case(case_path):
    s = softprobe.start_session(mode="replay")
    try:
        s.load_case_from_file(case_path)
        hit = s.find_in_case(direction="outbound", host_suffix="stripe.com")
        s.mock_outbound(host_suffix="stripe.com", response=hit.response)

        req = urllib.request.Request(
            f"{APP_URL}/checkout",
            method="POST",
            data=b'{"amount": 1000}',
            headers={"x-softprobe-session-id": s.id},
        )
        with urllib.request.urlopen(req) as res:
            assert res.status == 200
    finally:
        s.close()
```

For 100s of cases, prefer [`softprobe suite run`](/guides/run-a-suite-at-scale) over pytest parametrization — it's faster and designed for the scale case.

## Using a pytest plugin for `conftest.py`

The SDK ships a small pytest plugin that handles fixture wiring. In `conftest.py`:

```python
from softprobe.pytest_plugin import softprobe_session  # re-exports a fixture
```

Then in your test:

```python
def test_charges_the_captured_card(softprobe_session):
    softprobe_session.load_case_from_file("cases/checkout-happy-path.case.json")
    hit = softprobe_session.find_in_case(host_suffix="stripe.com")
    softprobe_session.mock_outbound(host_suffix="stripe.com", response=hit.response)
    # ...
```

The plugin handles create / close + env var inference so you don't repeat boilerplate per file.

## Running in parallel

`pytest-xdist` works with Softprobe out of the box — each worker creates its own session. Set `-n auto`:

```bash
pytest -n auto
```

## Next

- [Run a suite at scale](/guides/run-a-suite-at-scale) — for hundreds or thousands of cases.
- [Write a hook](/guides/write-a-hook) — for PII masking or custom assertions shared with CLI-driven suites.
- [Python SDK reference](/reference/sdk-python) — complete API.
