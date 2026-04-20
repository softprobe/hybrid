"""End-to-end check that the Python SDK can drive a replay session through the
mesh using `find_in_case` + `mock_outbound`.

Mirrors `e2e/jest-replay/fragment.replay.test.ts`. See `docs/design.md` §3.2.
"""

from __future__ import annotations

import json
from urllib import request as urllib_request

from softprobe import SoftprobeSession


def test_fragment_replay_through_the_mesh(
    replay_session: SoftprobeSession, app_url: str, case_path: str
) -> None:
    replay_session.load_case_from_file(case_path)

    hit = replay_session.find_in_case(
        direction="outbound", method="GET", path="/fragment"
    )

    replay_session.mock_outbound(
        id="fragment-replay",
        priority=100,
        direction="outbound",
        method="GET",
        path="/fragment",
        response=hit.response,
    )

    request = urllib_request.Request(
        f"{app_url}/hello",
        headers={"x-softprobe-session-id": replay_session.id},
    )
    with urllib_request.urlopen(request, timeout=10) as response:
        assert response.status == 200
        body = json.loads(response.read().decode("utf-8"))

    # The SUT composes its response from the mocked /fragment dependency;
    # `dep` coming through proves mock_outbound replaced the live upstream.
    assert body == {"message": "hello", "dep": "ok"}
