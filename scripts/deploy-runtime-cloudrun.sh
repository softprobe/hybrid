#!/usr/bin/env bash
set -euo pipefail

# Deploy hosted Softprobe **Rust** runtime to Cloud Run (single service: OTLP + query + control API).
# Former two-service layout (datalake + Go runtime) is retired.
#
# Docker images: Artifact Registry only — do not default back to gcr.io (legacy Container Registry).
# One-time per project: gcloud artifacts repositories create softprobe \
#   --repository-format=docker --location=us-central1 --project="${PROJECT_ID}"
#
# Optional overrides:
#   PROJECT_ID                 default: cs-poc-sasxbttlzroculpau4u6e2l
#   REGION                     default: us-central1
#   RUNTIME_SERVICE_NAME       default: softprobe-runtime
#   RUNTIME_IMAGE              default: us-central1-docker.pkg.dev/${PROJECT_ID}/softprobe/softprobe-runtime:latest
#   BUILD_RUNTIME_IMAGE        default: 1 (build+push runtime image from ./softprobe-runtime before deploy)
#   SERVICE_ACCOUNT            default: softprobe-runtime@${PROJECT_ID}.iam.gserviceaccount.com
#   VPC_CONNECTOR              default: softprobe-connector
#   VPC_EGRESS                 default: private-ranges-only
#   AUTH_URL                   default: https://auth.softprobe.ai/api/api-key/validate
#   REDIS_HOST                 default: 10.42.202.91
#   REDIS_PORT                 default: 6379
#   CONFIG_FILE                default: config.yaml (env var passed to container)
#   MAX_INSTANCES              default: 100
#   CPU                        default: 1
#   MEMORY                     default: 512Mi
#   SOFTPROBE_API_KEY          required for post-deploy integration tests
#   SKIP_INTEGRATION_TESTS     set to 1 to skip hosted smoke tests
#   RUN_LOCAL_E2E              default: 1 (run local compose e2e gate)
#   RUN_HOSTED_E2E             default: 1 (run hosted e2e gate against deployed URL)
#   LOCAL_E2E_API_KEY          default: dev-public-key (token for local authstub)

PROJECT_ID="${PROJECT_ID:-cs-poc-sasxbttlzroculpau4u6e2l}"
REGION="${REGION:-us-central1}"
RUNTIME_SERVICE_NAME="${RUNTIME_SERVICE_NAME:-softprobe-runtime}"
RUNTIME_IMAGE="${RUNTIME_IMAGE:-us-central1-docker.pkg.dev/${PROJECT_ID}/softprobe/softprobe-runtime:latest}"
BUILD_RUNTIME_IMAGE="${BUILD_RUNTIME_IMAGE:-1}"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT:-softprobe-runtime@${PROJECT_ID}.iam.gserviceaccount.com}"
VPC_CONNECTOR="${VPC_CONNECTOR:-softprobe-connector}"
VPC_EGRESS="${VPC_EGRESS:-private-ranges-only}"
AUTH_URL="${AUTH_URL:-https://auth.softprobe.ai/api/api-key/validate}"
REDIS_HOST="${REDIS_HOST:-10.42.202.91}"
REDIS_PORT="${REDIS_PORT:-6379}"
CONFIG_FILE="${CONFIG_FILE:-config.yaml}"
MAX_INSTANCES="${MAX_INSTANCES:-100}"
CPU="${CPU:-1}"
MEMORY="${MEMORY:-512Mi}"
RUN_LOCAL_E2E="${RUN_LOCAL_E2E:-1}"
RUN_HOSTED_E2E="${RUN_HOSTED_E2E:-1}"
LOCAL_E2E_API_KEY="${LOCAL_E2E_API_KEY:-dev-public-key}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ "${BUILD_RUNTIME_IMAGE}" == "1" ]]; then
  echo "Building softprobe-runtime image in project ${PROJECT_ID}..."
  gcloud builds submit \
    --project="${PROJECT_ID}" \
    --tag "${RUNTIME_IMAGE}" \
    "${REPO_ROOT}/softprobe-runtime"
fi

echo "Deploying ${RUNTIME_SERVICE_NAME} to Cloud Run..."
echo "  project: ${PROJECT_ID}"
echo "  region:  ${REGION}"
echo "  image:   ${RUNTIME_IMAGE}"

gcloud run deploy "${RUNTIME_SERVICE_NAME}" \
  --project="${PROJECT_ID}" \
  --region="${REGION}" \
  --platform=managed \
  --image="${RUNTIME_IMAGE}" \
  --service-account="${SERVICE_ACCOUNT}" \
  --allow-unauthenticated \
  --vpc-connector="${VPC_CONNECTOR}" \
  --vpc-egress="${VPC_EGRESS}" \
  --max-instances="${MAX_INSTANCES}" --cpu="${CPU}" --memory="${MEMORY}" \
  --set-env-vars="CONFIG_FILE=${CONFIG_FILE},SOFTPROBE_LISTEN_ADDR=:8080,SOFTPROBE_AUTH_URL=${AUTH_URL},REDIS_HOST=${REDIS_HOST},REDIS_PORT=${REDIS_PORT},SOFTPROBE_GRPC_DISABLE=1"

echo
RUNTIME_URL="$(gcloud run services describe "${RUNTIME_SERVICE_NAME}" \
  --project="${PROJECT_ID}" \
  --region="${REGION}" \
  --format="value(status.url)")"

echo "Deployment complete."
echo "  runtime: ${RUNTIME_URL}"

if [[ "${SKIP_INTEGRATION_TESTS:-0}" == "1" ]]; then
  echo "Skipping integration tests (SKIP_INTEGRATION_TESTS=1)."
  exit 0
fi

if [[ -z "${SOFTPROBE_API_KEY:-}" ]]; then
  echo "ERROR: SOFTPROBE_API_KEY is required for post-deploy integration tests."
  exit 1
fi

if [[ "${RUN_LOCAL_E2E}" == "1" ]]; then
  echo
  echo "Running local e2e gate (compose runtime/proxy/app/upstream)..."
  (
    cd "${REPO_ROOT}/e2e"
    docker compose -f docker-compose.yaml up --build --wait
    trap 'docker compose -f docker-compose.yaml down || true' EXIT
    SOFTPROBE_API_KEY="${LOCAL_E2E_API_KEY}" go test ./... -count=1
  )
fi

if [[ "${RUN_HOSTED_E2E}" == "1" ]]; then
  echo
  echo "Running hosted e2e gate (official deployed runtime)..."
  (
    cd "${REPO_ROOT}/e2e"
    SOFTPROBE_RUNTIME_URL="${RUNTIME_URL}" \
    SOFTPROBE_API_KEY="${SOFTPROBE_API_KEY}" \
    go test ./hosted -v -count=1
  )
fi
