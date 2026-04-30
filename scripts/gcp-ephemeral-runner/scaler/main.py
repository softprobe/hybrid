import os
from typing import Any, Dict

import requests
from flask import Flask, jsonify
from google.auth import default
from google.auth.transport.requests import AuthorizedSession

app = Flask(__name__)


def env(name: str, default_value: str = "") -> str:
    value = os.getenv(name, default_value).strip()
    if not value:
        raise RuntimeError(f"Missing required env var: {name}")
    return value


def gh_headers(token: str) -> Dict[str, str]:
    return {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
    }


def count_active_runs(repo: str, token: str) -> int:
    """Approximate repo demand for runner capacity (queued + in_progress)."""
    url = f"https://api.github.com/repos/{repo}/actions/runs"
    total = 0
    for status in ("queued", "in_progress"):
        r = requests.get(
            url,
            headers=gh_headers(token),
            params={"status": status, "per_page": 100},
            timeout=20,
        )
        r.raise_for_status()
        payload = r.json()
        total += int(payload.get("total_count", 0))
    return total


def get_current_target(project: str, zone: str, mig_name: str, session: AuthorizedSession) -> int:
    url = (
        f"https://compute.googleapis.com/compute/v1/projects/{project}"
        f"/zones/{zone}/instanceGroupManagers/{mig_name}"
    )
    r = session.get(url, timeout=20)
    r.raise_for_status()
    body = r.json()
    return int(body.get("targetSize", 0))


def resize_mig(project: str, zone: str, mig_name: str, size: int, session: AuthorizedSession) -> Dict[str, Any]:
    url = (
        f"https://compute.googleapis.com/compute/v1/projects/{project}"
        f"/zones/{zone}/instanceGroupManagers/{mig_name}/resize?size={size}"
    )
    r = session.post(url, timeout=30)
    r.raise_for_status()
    return r.json()


@app.get("/")
def health():
    return jsonify({"ok": True})


@app.post("/scale")
def scale():
    repo = env("GITHUB_REPO")
    token = env("GITHUB_TOKEN")
    project = env("GCP_PROJECT")
    zone = env("MIG_ZONE")
    mig_name = env("MIG_NAME")
    min_runners = int(env("MIN_RUNNERS", "0"))
    max_runners = int(env("MAX_RUNNERS", "3"))

    active_runs = count_active_runs(repo, token)
    desired = min(max(active_runs, min_runners), max_runners)

    creds, _ = default(scopes=["https://www.googleapis.com/auth/cloud-platform"])
    session = AuthorizedSession(creds)
    current = get_current_target(project, zone, mig_name, session)

    operation = None
    if current != desired:
        operation = resize_mig(project, zone, mig_name, desired, session)

    return jsonify(
        {
            "repo": repo,
            "active_runs": active_runs,
            "current_target_size": current,
            "desired_target_size": desired,
            "resized": current != desired,
            "operation": operation,
        }
    )

