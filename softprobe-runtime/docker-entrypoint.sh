#!/bin/sh
set -e
# Route `docker run … --version` / `version` to the CLI binary; otherwise
# start the HTTP runtime (default).
case "${1:-}" in
--version|version)
	exec /usr/local/bin/softprobe "$@"
	;;
*)
	exec /usr/local/bin/softprobe-runtime
	;;
esac
