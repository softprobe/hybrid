#!/usr/bin/env bash
set -euo pipefail

exec > >(tee -a /var/log/github-ephemeral-startup.log | logger -t gcp-ephemeral-startup) 2>&1

log() {
  echo "[$(date -Iseconds)] $*"
}

METADATA="http://metadata.google.internal/computeMetadata/v1/instance/attributes"
MD_HEADER="Metadata-Flavor: Google"

metadata_get_required() {
  local key="$1"
  curl -fsS -H "${MD_HEADER}" "${METADATA}/${key}"
}

metadata_get_optional() {
  local key="$1"
  local body_file
  body_file="$(mktemp)"
  local code
  code="$(curl -sS -o "${body_file}" -w "%{http_code}" -H "${MD_HEADER}" "${METADATA}/${key}" || true)"
  if [[ "${code}" == "200" ]]; then
    cat "${body_file}"
  fi
  rm -f "${body_file}"
}

github_repo="$(metadata_get_required github_repo)"
github_pat="$(metadata_get_required github_pat)"
runner_labels="$(metadata_get_required runner_labels)"
runner_group="$(metadata_get_optional runner_group)"
runner_version="$(metadata_get_optional runner_version)"
idle_timeout_seconds="$(metadata_get_optional idle_timeout_seconds)"
if [[ -z "${runner_version}" ]]; then
  runner_version="2.334.0"
fi
if [[ -z "${idle_timeout_seconds}" ]]; then
  idle_timeout_seconds="300"
fi

RUNNER_USER="bill_softprobe_ai"
RUNNER_ROOT="/home/${RUNNER_USER}/actions-runner-ephemeral"
RUNNER_URL="https://github.com/${github_repo}"

log "Stopping legacy baked-in runner services (if present)"
for svc in $(systemctl list-units --type=service --all --no-legend 'actions.runner.*.service' 2>/dev/null | awk '{print $1}'); do
  systemctl stop "${svc}" || true
  systemctl disable "${svc}" || true
done
pkill -f '/actions-runner/bin/Runner.Listener run' || true
pkill -f '/actions-runner-org/bin/Runner.Listener run' || true

log "Using pre-baked runner image; skipping package install/bootstrap"
if [[ ! -x "${RUNNER_ROOT}/config.sh" || ! -x "${RUNNER_ROOT}/run.sh" || ! -x "${RUNNER_ROOT}/bin/Runner.Listener" ]]; then
  log "ERROR: pre-baked runner binaries not found at ${RUNNER_ROOT}"
  exit 1
fi

mkdir -p "${RUNNER_ROOT}"
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
  recycle_reason=""
  missing_in_github_checks=0
  while true; do
    if ! kill -0 "${runner_pid}" 2>/dev/null; then
      log "Runner process ${runner_pid} exited unexpectedly"
      recycle_reason="runner-exited"
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
      missing_in_github_checks="$((missing_in_github_checks + 1))"
      if (( missing_in_github_checks >= 6 )); then
        log "Runner ${RUNNER_NAME} missing from GitHub for 60s; forcing re-registration"
        recycle_reason="registration-lost"
        break
      fi
      log "Runner ${RUNNER_NAME} not found in GitHub yet; waiting (${missing_in_github_checks}/6)"
      sleep 10
      continue
    fi
    missing_in_github_checks=0

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
      recycle_reason="idle-timeout"
      break
    fi

    sleep 10
  done

  if [[ "${recycle_reason}" == "registration-lost" || "${recycle_reason}" == "runner-exited" ]]; then
    log "Runner recycle reason is '${recycle_reason}'; cleaning local state and re-registering"
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
    rm -f "${RUNNER_ROOT}/.runner" "${RUNNER_ROOT}/.credentials" "${RUNNER_ROOT}/.credentials_rsaparams"
    sleep 5
    continue
  fi

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
  systemctl poweroff --force --no-wall || shutdown -P now || halt -p || true
  exit 0
done
