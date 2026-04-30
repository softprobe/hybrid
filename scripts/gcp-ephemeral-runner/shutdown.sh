#!/bin/bash
# Runs on GCE instance shutdown (MIG scale-in, preemption, manual stop).
# Unregisters the GitHub runner so scale-in does not leave stale offline entries.
set +e
exec >> /var/log/github-ephemeral-shutdown.log 2>&1

log() {
  echo "[$(date -Iseconds)] $*"
}

log "shutdown-script start"

METADATA="http://metadata.google.internal/computeMetadata/v1/instance/attributes"
MD_HEADER="Metadata-Flavor: Google"

github_repo="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/github_repo" 2>/dev/null || true)"
github_pat="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/github_pat" 2>/dev/null || true)"
RUNNER_USER="bill_softprobe_ai"
RUNNER_ROOT="/home/${RUNNER_USER}/actions-runner-ephemeral"

if [[ -z "${github_repo}" || -z "${github_pat}" ]]; then
  log "missing github_repo or github_pat; skipping unregister"
  exit 0
fi

log "Stopping runner listener (if any)"
pkill -u "${RUNNER_USER}" -f "${RUNNER_ROOT}/bin/Runner.Listener run" 2>/dev/null || true
sleep 3
pkill -9 -u "${RUNNER_USER}" -f "${RUNNER_ROOT}/bin/Runner.Listener run" 2>/dev/null || true

remove_token="$(
  curl -fsSL -X POST \
    -H "Authorization: Bearer ${github_pat}" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/${github_repo}/actions/runners/remove-token" 2>/dev/null | jq -r '.token' || true
)"
if [[ -n "${remove_token}" && "${remove_token}" != "null" && -f "${RUNNER_ROOT}/config.sh" ]]; then
  log "Removing runner registration from GitHub"
  su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && ./config.sh remove --token '${remove_token}'" || true
else
  log "No remove token or config.sh; skip config.sh remove"
fi

log "shutdown-script done"
