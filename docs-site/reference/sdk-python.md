# Python SDK reference

The `softprobe` package. Works with Python 3.9+.

::: warning Not yet on PyPI
The `pip install softprobe` command below refers to a **planned** PyPI
release and is not wired to this repository today. Consume the package from
source via the [hybrid monorepo](https://github.com/softprobe/hybrid)
(`softprobe-python/`) — see its `README.md` for details.
:::

```bash
# Planned — not yet published.
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
softprobe = Softprobe(base_url="https://runtime.softprobe.dev", timeout=5.0)
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

**Raises `CaseLookupError`** if **zero** or **more than one** spans match — ambiguity surfaces at authoring time. The exception's `.matches` attribute lists the offending spans so you can narrow the predicate. Use `find_all_in_case` when multiple matches are expected.

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

::: info Merge on the client, replace on the wire
The runtime's `POST /v1/sessions/{id}/rules` **replaces** the whole rules document. The SDK keeps a local merged list so consecutive `mock_outbound()` calls accumulate. Call `session.clear_rules()` to reset the SDK-side list.
:::

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

All SDK exceptions inherit from `softprobe.errors.SoftprobeError`.

### Error catalog

| Condition | Exception | Typical cause | Recovery |
|---|---|---|---|
| **Runtime unreachable** | `RuntimeError` (cause: `ConnectionError` / `Timeout`) | Runtime not running, wrong `base_url`, firewall | Start the runtime; `softprobe doctor` |
| **Unknown session** | `RuntimeError` with `.status == 404` | Session closed, wrong id | Start a fresh session |
| **Strict miss** (proxy returns error to app) | Not an SDK error — surfaces as a Python HTTP client exception inside the SUT | Missing `mock_outbound` or wrong predicate | Add the rule; see [Debug strict miss](/guides/troubleshooting#_403-forbidden-on-outbound-under-strict-policy) |
| **Invalid rule payload** | `RuntimeError` with `.status == 400` | Rule body doesn't validate against [rule-schema](/reference/rule-schema) | Fix the spec; most fields validated client-side |
| **`find_in_case` zero matches** | `CaseLookupError` with `len(e.matches) == 0` | Predicate too narrow; capture didn't include hop | Relax predicate; re-capture |
| **`find_in_case` multiple matches** | `CaseLookupError` with `len(e.matches) > 1` | Predicate too broad | Narrow predicate; use `find_all_in_case` |

### Example

```python
from softprobe.errors import SoftprobeError, RuntimeError, CaseLookupError, CaseLoadError

try:
    hit = session.find_in_case(direction="outbound", host_suffix="stripe.com")
except CaseLookupError as e:
    print(f"findInCase: {len(e.matches)} matches:",
          [m.span_id for m in e.matches])
    raise
except RuntimeError as e:
    print(f"runtime {e.status} at {e.url}: {e.body}")
    raise
except CaseLoadError as e:
    print(f"case load failed: {e.path}: {e}")
    raise
```

### Class hierarchy

| Class | Extends | When raised |
|---|---|---|
| `SoftprobeError` | `Exception` | Base class; catch to catch everything |
| `RuntimeError` | `SoftprobeError` | Runtime returned non-2xx. Attributes: `status`, `body`, `url` |
| `CaseLookupError` | `SoftprobeError` | `find_in_case` saw 0 or >1 matches. Attribute: `matches: list[Span]` |
| `CaseLoadError` | `SoftprobeError` | `load_case_from_file` failed to parse / validate. Attribute: `path` |

## Logging

```python
import logging
logging.getLogger("softprobe").setLevel(logging.DEBUG)
```

Or env: `SOFTPROBE_LOG=debug pytest`.

## See also

- [Replay in pytest](/guides/replay-in-pytest) — tutorial
- [HTTP control API](/reference/http-control-api) — wire-level spec
