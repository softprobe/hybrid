#!/bin/sh
set -e
# No args → start the HTTP runtime (Kubernetes / compose default).
# Any args → `softprobe` CLI (`--version`, `doctor`, `suite run`, …).
if [ "$#" -eq 0 ]; then
	exec /usr/local/bin/softprobe-runtime
fi
exec /usr/local/bin/softprobe "$@"
