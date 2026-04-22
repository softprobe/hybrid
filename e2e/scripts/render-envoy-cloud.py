#!/usr/bin/env python3
"""Render e2e/envoy.cloud.yaml from template + env vars.

Required env:
  SOFTPROBE_RUNTIME_URL  e.g. https://softprobe-runtime-....run.app
  SOFTPROBE_API_KEY      Hosted API key used by the WASM proxy auth path
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from urllib.parse import urlparse


def fail(msg: str) -> None:
    print(f"render-envoy-cloud: {msg}", file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    repo = Path(__file__).resolve().parents[1]
    tmpl_path = repo / "envoy.cloud.tmpl.yaml"
    out_path = repo / "envoy.cloud.yaml"

    runtime_url = os.environ.get("SOFTPROBE_RUNTIME_URL", "").strip()
    api_key = os.environ.get("SOFTPROBE_API_KEY", "").strip()
    if not runtime_url:
        fail("SOFTPROBE_RUNTIME_URL is required")
    if not api_key:
        fail("SOFTPROBE_API_KEY is required")

    parsed = urlparse(runtime_url)
    if parsed.scheme not in {"https", "http"}:
        fail("SOFTPROBE_RUNTIME_URL must start with http:// or https://")
    if not parsed.hostname:
        fail("SOFTPROBE_RUNTIME_URL host is missing")
    if parsed.path not in {"", "/"}:
        fail("SOFTPROBE_RUNTIME_URL must be a base URL without a path")

    host = parsed.hostname
    if parsed.port:
        port = parsed.port
    else:
        port = 443 if parsed.scheme == "https" else 80
    cluster = f"outbound|{port}||{host}"

    transport_socket = ""
    if parsed.scheme == "https":
        transport_socket = f"""
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          sni: "{host}" """

    text = tmpl_path.read_text(encoding="utf-8")
    text = text.replace("__SP_BACKEND_URL__", runtime_url)
    text = text.replace("__SP_PUBLIC_KEY__", api_key)
    text = text.replace("__SP_BACKEND_HOST__", host)
    text = text.replace("__SP_BACKEND_PORT__", str(port))
    text = text.replace("__SP_BACKEND_CLUSTER__", cluster)
    text = text.replace("__SP_TRANSPORT_SOCKET__", transport_socket)
    out_path.write_text(text, encoding="utf-8")
    print(f"wrote {out_path}")


if __name__ == "__main__":
    main()
