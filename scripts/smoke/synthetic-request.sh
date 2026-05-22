#!/bin/sh
# Fires synthetic control-plane requests at the forked provider's REST gateway.
#
# The gateway requires verified auth (mTLS / JWT) for the `verified` event to
# resolve to auth_state=verified. Standing up a real verified client inside the
# smoke test is heavy, so this driver fires TWO requests:
#
#   1. an UNAUTHENTICATED manifest_submit-shaped request — exercises the
#      pre_auth event and a verified event with auth_state=auth_failed/unknown;
#   2. a request carrying the smoke test's synthetic bearer token — exercises
#      the auth path. (In CI the token is a fixture; whether it resolves to
#      auth_state=verified depends on the gateway's verifier — assert-ndjson.sh
#      checks for a `verified` line across all captured records.)
#
# Both requests still produce a fully-populated pre_auth event, which is the
# load-bearing assertion. event_type resolves to manifest_submit via the
# PUT …/manifest route.
#
# Env:
#   DF_SMOKE_GATEWAY_URL   base URL of the provider REST gateway (required)
#   DF_SMOKE_DSEQ          deployment sequence to use in the path (default 1)
#   DF_SMOKE_BEARER        synthetic bearer token for request 2 (optional)
set -eu

URL="${DF_SMOKE_GATEWAY_URL:?DF_SMOKE_GATEWAY_URL must be set}"
DSEQ="${DF_SMOKE_DSEQ:-1}"
BEARER="${DF_SMOKE_BEARER:-}"

MANIFEST_PATH="/deployment/${DSEQ}/manifest"

echo "synthetic-request: PUT ${URL}${MANIFEST_PATH} (unauthenticated)"
curl -ksS -o /dev/null -w 'request 1 -> HTTP %{http_code}\n' \
  -X PUT "${URL}${MANIFEST_PATH}" \
  -H 'Content-Type: application/json' \
  --data '{}' || true

if [ -n "${BEARER}" ]; then
  echo "synthetic-request: PUT ${URL}${MANIFEST_PATH} (with bearer token)"
  curl -ksS -o /dev/null -w 'request 2 -> HTTP %{http_code}\n' \
    -X PUT "${URL}${MANIFEST_PATH}" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${BEARER}" \
    --data '{}' || true
fi

echo "synthetic-request: done"
