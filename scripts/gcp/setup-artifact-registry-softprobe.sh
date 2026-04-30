#!/usr/bin/env bash
# One-time (or idempotent) setup: Docker repo "softprobe" + cleanup = keep 10 newest versions
# per image package. Requires: gcloud, roles/artifactregistry.admin (or equivalent).
# Org policy is applied in GCP; this file is the source of truth for the JSON policy.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICY="${SCRIPT_DIR}/artifact-registry-cleanup-policy.json"
LOCATION="${AR_LOCATION:-us-central1}"
REPO="${AR_REPOSITORY:-softprobe}"

PROJECTS=(
  "cs-poc-sasxbttlzroculpau4u6e2l"
)

for PROJECT_ID in "${PROJECTS[@]}"; do
  echo "=== Project ${PROJECT_ID} ==="
  if gcloud artifacts repositories describe "${REPO}" \
      --project="${PROJECT_ID}" --location="${LOCATION}" &>/dev/null; then
    echo "Repository ${REPO} already exists."
  else
    gcloud artifacts repositories create "${REPO}" \
      --project="${PROJECT_ID}" \
      --repository-format=docker \
      --location="${LOCATION}" \
      --description="Softprobe container images (runtime, datalake, etc.)"
  fi
  gcloud artifacts repositories set-cleanup-policies "${REPO}" \
    --project="${PROJECT_ID}" \
    --location="${LOCATION}" \
    --policy="${POLICY}"
  echo "Cleanup policies applied."
done
