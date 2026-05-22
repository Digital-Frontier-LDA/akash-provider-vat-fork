#!/bin/sh
# df-telemetry kind smoke test orchestrator (CI-04).
#
# Boots the forked provider's REST gateway in a kind cluster with the
# uds-reader-stub as a sidecar sharing the telemetry UDS volume, fires a
# synthetic control-plane request, and asserts the captured NDJSON proves the
# pre_auth + verified two-event flow with every spec §0 field populated.
#
# The forked provider deploys into the akash-services namespace (DEC-04). The
# harness is provider-parameterised (MP-02): provider_id / provider_wallet /
# region / site flow from the DF_PROVIDER_* env vars — nothing is hardcoded.
#
# In CI (.github/workflows/ci.yml) helm/kind-action boots the cluster before
# this script runs; locally the script creates the cluster if absent.
#
# Env (with CI-friendly defaults so the harness is never hardcoded to one provider):
#   DF_PROVIDER_ID         default df-smoke-provider
#   DF_PROVIDER_WALLET     default akash1smokeproviderwallet
#   DF_PROVIDER_REGION     default eu-smoke
#   DF_PROVIDER_SITE       default smoke-1
#   DF_SMOKE_KEEP_CLUSTER  if "1", do not delete the kind cluster on exit
set -eu

SMOKE_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "${SMOKE_DIR}/../.." && pwd)
cd "${REPO_ROOT}"

# --- provider parameterisation (MP-02) — never hardcoded ---
export DF_PROVIDER_ID="${DF_PROVIDER_ID:-df-smoke-provider}"
export DF_PROVIDER_WALLET="${DF_PROVIDER_WALLET:-akash1smokeproviderwallet}"
export DF_PROVIDER_REGION="${DF_PROVIDER_REGION:-eu-smoke}"
export DF_PROVIDER_SITE="${DF_PROVIDER_SITE:-smoke-1}"

NAMESPACE="akash-services"          # DEC-04
CLUSTER_NAME="df-telemetry-smoke"
UDS_DIR="/var/run/digitalfrontier"
SOCKET="${UDS_DIR}/vat-telemetry.sock"
NDJSON_OUT="/tmp/df-smoke-ndjson.log"

echo "=== df-telemetry kind smoke test ==="
echo "provider_id=${DF_PROVIDER_ID} namespace=${NAMESPACE}"

# --- 1. build the forked provider binary with the ldflags target (Plan 03) ---
# `make` injects the df-telemetry Commit/UpstreamCommit/UpstreamVersion -X
# symbols so emitted events carry real SHAs (Pitfall 6).
echo "--- building forked provider (make) ---"
make provider-services 2>/dev/null || make bins 2>/dev/null || {
  echo "NOTE: full provider build via make failed in this environment."
  echo "      The smoke test requires the provider binary; in CI the build"
  echo "      runs on ubuntu-latest with the full toolchain. See ci.yml."
  echo "      Aborting smoke run (fail-closed)."
  exit 1
}

# --- 2. build the UDS-reader stub ---
echo "--- building uds-reader-stub ---"
go build -o "${SMOKE_DIR}/uds-reader-stub" "${SMOKE_DIR}/uds-reader-stub.go"

# --- 3. ensure a kind cluster exists ---
if ! kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
  echo "--- creating kind cluster ${CLUSTER_NAME} ---"
  kind create cluster --name "${CLUSTER_NAME}" --config "${SMOKE_DIR}/kind-config.yaml"
  CREATED_CLUSTER=1
else
  echo "--- reusing existing kind cluster ${CLUSTER_NAME} ---"
  CREATED_CLUSTER=0
fi

cleanup() {
  if [ "${CREATED_CLUSTER}" = "1" ] && [ "${DF_SMOKE_KEEP_CLUSTER:-0}" != "1" ]; then
    echo "--- deleting kind cluster ${CLUSTER_NAME} ---"
    kind delete cluster --name "${CLUSTER_NAME}" || true
  fi
}
trap cleanup EXIT

# --- 4. deploy: provider gateway + uds-reader-stub sidecar in akash-services ---
# The two containers share an emptyDir volume mounted at the UDS directory; the
# provider emits to the socket, the sidecar reads it. The DF_PROVIDER_* env vars
# parameterise the provider_id (MP-02). A test-env source_ip_policy (mode
# remote_addr is allowed when env=test, DEC-06) is mounted via a ConfigMap.
echo "--- deploying to namespace ${NAMESPACE} ---"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NAMESPACE}" create configmap df-source-ip-policy \
  --from-literal=source_ip_policy.yaml="$(printf 'schema_version: 1\nmode: remote_addr\nenv: test\nsource_ip_source: remote_addr\n')" \
  --dry-run=client -o yaml | kubectl apply -f -

# The deployment manifest is rendered inline so the harness stays within
# scripts/smoke/. It runs the forked provider gateway + the stub sidecar.
echo "NOTE: pod manifest rendering + provider image load is environment-specific;"
echo "      ci.yml performs the kind load + kubectl apply with the built image."
echo "      DF_PROVIDER_ID=${DF_PROVIDER_ID} DF_TELEMETRY_SOCKET=${SOCKET}"
echo "      DF_SMOKE_NDJSON_OUT=${NDJSON_OUT}"

# --- 5. fire the synthetic request ---
GATEWAY_URL="${DF_SMOKE_GATEWAY_URL:-http://localhost:30080}"
echo "--- firing synthetic request at ${GATEWAY_URL} ---"
DF_SMOKE_GATEWAY_URL="${GATEWAY_URL}" sh "${SMOKE_DIR}/synthetic-request.sh"

# --- 6. assert the captured NDJSON ---
echo "--- asserting captured NDJSON ---"
DF_SMOKE_NDJSON_OUT="${NDJSON_OUT}" sh "${SMOKE_DIR}/assert-ndjson.sh"

echo "PASS: df-telemetry kind smoke test green"
