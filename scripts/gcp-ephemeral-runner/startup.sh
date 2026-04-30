#!/usr/bin/env bash
set -euo pipefail

exec > >(tee -a /var/log/github-ephemeral-startup.log | logger -t gcp-ephemeral-startup) 2>&1

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
idle_timeout_seconds="$(curl -fsS -H "${MD_HEADER}" "${METADATA}/idle_timeout_seconds" || true)"
if [[ -z "${runner_version}" ]]; then
  runner_version="2.334.0"
fi
if [[ -z "${idle_timeout_seconds}" ]]; then
  idle_timeout_seconds="300"
fi

RUNNER_USER="bill_softprobe_ai"
RUNNER_ROOT="/home/${RUNNER_USER}/actions-runner-ephemeral"
RUNNER_URL="https://github.com/${github_repo}"

export DEBIAN_FRONTEND=noninteractive
if command -v docker >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 && command -v unzip >/dev/null 2>&1 && command -v zip >/dev/null 2>&1; then
  log "Runner dependencies already present; skipping apt installation"
else
  log "Installing runner dependencies"
  apt-get update
  apt-get install -y --no-install-recommends \
    ca-certificates curl jq unzip zip git docker.io libicu70 libkrb5-3 zlib1g
fi
systemctl enable docker || true
systemctl start docker || true
usermod -aG docker "${RUNNER_USER}" || true

mkdir -p "${RUNNER_ROOT}"
chown -R "${RUNNER_USER}:${RUNNER_USER}" "${RUNNER_ROOT}"
chown -R "${RUNNER_USER}:${RUNNER_USER}" "/home/${RUNNER_USER}"
chmod 755 "/home/${RUNNER_USER}"
install -d -o "${RUNNER_USER}" -g "${RUNNER_USER}" -m 775 \
  "/home/${RUNNER_USER}/go" \
  "/home/${RUNNER_USER}/.docker" \
  "/home/${RUNNER_USER}/.cache" \
  "/home/${RUNNER_USER}/.npm" \
  "/home/${RUNNER_USER}/.m2"

log "Downloading actions runner ${runner_version}"
runner_tgz="actions-runner-linux-x64-${runner_version}.tar.gz"
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && curl -fsSLo '${runner_tgz}' 'https://github.com/actions/runner/releases/download/v${runner_version}/${runner_tgz}'"
su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && tar xzf '${runner_tgz}'"
log "Installing GitHub runner runtime dependencies"
cd "${RUNNER_ROOT}"
./bin/installdependencies.sh
chown -R "${RUNNER_USER}:${RUNNER_USER}" "${RUNNER_ROOT}"

group_args=()
if [[ -n "${runner_group}" ]]; then
  group_args=(--runnergroup "${runner_group}")
fi

while true; do
  RUNNER_NAME="gcp-ephemeral-$(hostname)"
  existing_pid="$(pgrep -u "${RUNNER_USER}" -f "${RUNNER_ROOT}/bin/Runner.Listener run" | head -n1 || true)"
  if [[ -n "${existing_pid}" ]]; then
    log "Runner listener already running with PID ${existing_pid}; skipping reconfigure"
    runner_pid="${existing_pid}"
  else
  log "Requesting registration token for ${github_repo}"
  registration_token="$(
    curl -fsSL -X POST \
      -H "Authorization: Bearer ${github_pat}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${github_repo}/actions/runners/registration-token" | jq -r '.token'
  )"
  if [[ -z "${registration_token}" || "${registration_token}" == "null" ]]; then
    log "ERROR: failed to get registration token; retry in 15s"
    sleep 15
    continue
  fi

    if [[ ! -f "${RUNNER_ROOT}/.runner" ]]; then
      log "Configuring runner ${RUNNER_NAME}"
      su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && ./config.sh \
        --url '${RUNNER_URL}' \
        --token '${registration_token}' \
        --name '${RUNNER_NAME}' \
        --labels '${runner_labels}' \
        --work '_work' \
        --unattended \
        --replace \
        ${group_args[*]-}"
    else
      log "Runner config exists; keeping current registration metadata"
    fi

    log "Starting reusable runner process for ${RUNNER_NAME}"
    su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && nohup ./run.sh > runner.log 2>&1 & echo \$! > runner.pid"
    sleep 2
    runner_pid="$(cat "${RUNNER_ROOT}/runner.pid" 2>/dev/null || true)"
    if [[ -z "${runner_pid}" ]]; then
      runner_pid="$(pgrep -u "${RUNNER_USER}" -f "${RUNNER_ROOT}/bin/Runner.Listener run" | head -n1 || true)"
    fi
    if [[ -z "${runner_pid}" ]]; then
      log "ERROR: failed to detect runner process; retrying"
      sleep 10
      continue
    fi
  fi

  log "Runner PID ${runner_pid} started; monitoring idle timeout ${idle_timeout_seconds}s"
  last_busy_epoch="$(date +%s)"
  while true; do
    if ! kill -0 "${runner_pid}" 2>/dev/null; then
      log "Runner process ${runner_pid} exited unexpectedly"
      break
    fi

    runner_state="$(
      curl -fsSL \
        -H "Authorization: Bearer ${github_pat}" \
        -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/${github_repo}/actions/runners?per_page=100" \
        | jq -r --arg NAME "${RUNNER_NAME}" '.runners[]? | select(.name == $NAME) | "\(.status) \(.busy)"' \
        | head -n1
    )"

    if [[ -z "${runner_state}" ]]; then
      log "Runner ${RUNNER_NAME} not found in GitHub yet; waiting"
      sleep 10
      continue
    fi

    runner_status="$(echo "${runner_state}" | awk '{print $1}')"
    runner_busy="$(echo "${runner_state}" | awk '{print $2}')"
    now_epoch="$(date +%s)"

    if [[ "${runner_status}" != "online" ]]; then
      log "Runner ${RUNNER_NAME} status=${runner_status}; waiting"
      sleep 10
      continue
    fi

    if [[ "${runner_busy}" == "true" ]]; then
      last_busy_epoch="${now_epoch}"
      sleep 10
      continue
    fi

    idle_for="$((now_epoch - last_busy_epoch))"
    if (( idle_for >= idle_timeout_seconds )); then
      log "Runner ${RUNNER_NAME} idle for ${idle_for}s; recycling VM"
      break
    fi

    sleep 10
  done

  log "Stopping runner process ${runner_pid}"
  kill "${runner_pid}" 2>/dev/null || true
  sleep 2
  kill -9 "${runner_pid}" 2>/dev/null || true

  remove_token="$(
    curl -fsSL -X POST \
      -H "Authorization: Bearer ${github_pat}" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${github_repo}/actions/runners/remove-token" | jq -r '.token' || true
  )"
  if [[ -n "${remove_token}" && "${remove_token}" != "null" ]]; then
    su - "${RUNNER_USER}" -c "cd '${RUNNER_ROOT}' && ./config.sh remove --token '${remove_token}'" || true
  fi

  log "Powering off after idle timeout"
  shutdown -h now
  exit 0
done
