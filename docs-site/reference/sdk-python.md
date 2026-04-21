# Python SDK reference

The `softprobe` PyPI package. Works with Python 3.9+.

```bash
pip install softprobe
```

## Import

```python
from softprobe import Softprobe, SoftprobeSession
from softprobe.types import CapturedHit, CapturedResponse, CaseSpanPredicate, MockRuleSpec, Policy
```

## `Softprobe`

### `Softprobe(base_url=None, timeout=5.0)`

```python
softprobe = Softprobe(base_url="http://127.0.0.1:8080", timeout=5.0)
```

Falls back to `SOFTPROBE_RUNTIME_URL` env var.

### `start_session(mode) → SoftprobeSession`

```python
session = softprobe.start_session(mode="replay")
```

### `attach(session_id) → SoftprobeSession`

Binds to an existing session without an HTTP call.

## `SoftprobeSession`

| Method | JS equivalent |
|---|---|
| `session.id` | `session.id` |
| `session.load_case_from_file(path)` | `loadCaseFromFile` |
| `session.load_case(doc)` | `loadCase` |
| `session.find_in_case(**predicate)` | `findInCase` |
| `session.find_all_in_case(**predicate)` | `findAllInCase` |
| `session.mock_outbound(**spec)` | `mockOutbound` |
| `session.clear_rules()` | `clearRules` |
| `session.set_policy(**kwargs)` | `setPolicy` |
| `session.close()` | `close` |

Semantics and wire shape are identical. Python uses `snake_case`.

### `find_in_case(**predicate)` → `CapturedHit`

```python
hit = session.find_in_case(
    direction="outbound",
    method="POST",
    host_suffix="stripe.com",
    path_prefix="/v1/payment_intents",
)

print(hit.response.status)   # 200
print(hit.response.body)     # '{"id":"pi_123"...}'
```

Accepts the same predicate keys as the JS SDK, with `snake_case`.

### `mock_outbound(**spec)` → None

```python
session.mock_outbound(
    direction="outbound",
    host_suffix="stripe.com",
    path_prefix="/v1/payment_intents",
    response={
        "status": 200,
        "headers": {"content-type": "application/json"},
        "body": '{"id":"pi_test"}',
    },
)
```

`response` can be a dict or a `CapturedResponse` namedtuple. Bodies can be strings or objects (the SDK serializes dicts with `json.dumps`).

## Context manager

Sessions support the context-manager protocol:

```python
with softprobe.start_session(mode="replay") as session:
    session.load_case_from_file("cases/checkout.case.json")
    hit = session.find_in_case(host_suffix="stripe.com")
    session.mock_outbound(host_suffix="stripe.com", response=hit.response)
    # ... run your test
# session.close() is called automatically here
```

## Pytest plugin

Add `softprobe` to your `conftest.py` and get a fixture:

```python
# conftest.py
from softprobe.pytest_plugin import *  # noqa: F403  (re-exports fixtures)
```

```python
def test_checkout(softprobe_session):
    softprobe_session.load_case_from_file("cases/checkout.case.json")
    hit = softprobe_session.find_in_case(host_suffix="stripe.com")
    softprobe_session.mock_outbound(host_suffix="stripe.com", response=hit.response)
    # ...
```

The fixture is module-scoped by default — one session per test module. Override to function scope if needed:

```python
@pytest.fixture(scope="function")
def softprobe_session(softprobe):
    with softprobe.start_session(mode="replay") as s:
        yield s
```

## `run_suite`

Run a `suite.yaml` as pytest-parametrized tests:

```python
# tests/test_checkout_replay.py
from softprobe.suite import run_suite
from tests.hooks import unmask_card, assert_totals

run_suite(
    "suites/checkout.suite.yaml",
    hooks={
        "checkout.unmaskCard": unmask_card,
        "checkout.assertTotalsMatchItems": assert_totals,
    },
)
```

The plugin discovers case files from the glob and registers one parametrized node per case. `pytest -k happy` picks a subset.

## Hook types

```python
from softprobe.hooks import RequestHook, MockResponseHook, BodyAssertHook

def unmask_card(request, env, **_):
    import json
    body = json.loads(request["body"])
    body["card"]["number"] = env.get("TEST_CARD", "4111111111111111")
    return {"body": json.dumps(body)}
```

Signature conventions:
- Hooks take keyword-only arguments.
- Unused keys go into `**_`.
- Return a `dict` with the keys you want to change (same as JS).

## Errors

```python
from softprobe.errors import SoftprobeError, RuntimeError, CaseLookupError

try:
    hit = session.find_in_case(...)
except CaseLookupError as e:
    # e.matches is a list of the offending spans
    print(f"Too many matches: {len(e.matches)}")
```

## Logging

```python
import logging
logging.getLogger("softprobe").setLevel(logging.DEBUG)
```

Or env: `SOFTPROBE_LOG=debug pytest`.

## See also

- [Replay in pytest](/guides/replay-in-pytest) — tutorial
- [HTTP control API](/reference/http-control-api) — wire-level spec
