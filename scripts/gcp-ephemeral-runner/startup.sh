#!/usr/bin/env bash
set -euo pipefail

log() {
  echo "[$(date -Iseconds)] $*"
}

METADATA="http://metadata.google.internal/computeMetadata/v1/instance/attributes"
MD_HEADER="Metadata-Flavor: Google"

github_repo="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/github_repo")"
github_pat="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/github_pat")"
runner_labels="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/runner_labels")"
runner_group="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/runner_group" || true)"
runner_version="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/runner_version" || true)"
if [[ -z "${runner_version}" ]]; then
  runner_version="2.325.0"
fi

RUNNER_USER="bill_softprobe_ai"
RUNNER_ROOT="/home/${RUNNER_USER}/actions-runner-ephemeral"
RUNNER_NAME="gcp-ephemeral-$(hostname)-$(date +%s)"

export DEBIAN_FRONTEND=noninteractive
log "Installing runner dependencies"
apt-get update
apt-get install -y --no-install-recommends \
  ca-certificates curl jq unzip zip git docker.io
systemctl enable docker
systemctl start docker
usermod -aG docker "${RUNNER_USER}" || true

mkdir -p "${RUNNER_ROOT}"
chown -R "${RUNNER_USER}:${RUNNER_USER}" "${RUNNER_ROOT}"

log "Downloading actions runner ${runner_version}"
runner_tgz="actions-runner-linux-x64-${runner_version}.tar.gz"
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && curl -fsSLo '${runner_tgz}' 'https://github.com/actions/runner/releases/download/v${runner_version}/${runner_tgz}'"
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && tar xzf '${runner_tgz}'"

log "Requesting registration token for ${github_repo}"
registration_token="$(
  curl -fsSL -X POST \
    -H "Authorization: Bearer ${github_pat}" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/${github_repo}/actions/runners/registration-token" | jq -r '.token'
)"
if [[ -z "${registration_token}" || "${registration_token}" == "null" ]]; then
  log "ERROR: failed to get registration token"
  exit 1
fi

group_args=()
if [[ -n "${runner_group}" ]]; then
  group_args=(--runnergroup "${runner_group}")
fi

log "Configuring ephemeral runner ${RUNNER_NAME}"
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && ./config.sh \
  --url 'https://github.com/${github_repo}' \
  --token '${registration_token}' \
  --name '${RUNNER_NAME}' \
  --labels '${runner_labels}' \
  --work '_work' \
  --unattended \
  --ephemeral \
  --disableupdate \
  ${group_args[*]-}"

log "Running runner (single job, then terminate VM)"
set +e
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && ./run.sh"
rc=$?
set -e
log "Runner exited with code ${rc}; powering off instance"
shutdown -h now
