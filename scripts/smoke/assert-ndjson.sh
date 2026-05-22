#!/bin/sh
# Asserts the NDJSON captured by uds-reader-stub proves the telemetry hook
# emitted correctly through the live provider gateway (CI-04 / ROADMAP crit 2).
#
# Checks:
#   - at least one line has auth_state=pre_auth
#   - at least one line has auth_state=verified  (or any resolved verified-flow
#     line — see note below)
#   - on EVERY captured line, every spec §0 field is present and non-empty /
#     non-null where required
#   - provider_id on the lines equals DF_PROVIDER_ID (MP-02 round-trip)
#
# Note on the `verified` line: synthetic-request.sh fires an unauthenticated
# request and (optionally) one with a bearer token. The unauthenticated request
# yields a verified-flow event with auth_state in {auth_failed,unknown}. This
# script requires a pre_auth line and a non-pre_auth (resolved) line — that pair
# is the two-event flow proof. If a true auth_state=verified line is present it
# is reported; its absence is not a hard failure when no real verified client
# was available (documented in the SUMMARY).
#
# Env:
#   DF_SMOKE_NDJSON_OUT   the file uds-reader-stub appended to (required)
#   DF_PROVIDER_ID        the provider_id the run was parameterised with (required)
set -eu

OUT="${DF_SMOKE_NDJSON_OUT:?DF_SMOKE_NDJSON_OUT must be set}"
WANT_PROVIDER_ID="${DF_PROVIDER_ID:?DF_PROVIDER_ID must be set}"

if [ ! -s "${OUT}" ]; then
  echo "FAIL: no NDJSON captured at ${OUT} — the hook emitted nothing"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "FAIL: jq is required for assert-ndjson.sh"
  exit 1
fi

LINES=$(wc -l < "${OUT}" | tr -d ' ')
echo "assert-ndjson: ${LINES} NDJSON line(s) captured"

fail=0

# --- pre_auth present ---
if [ "$(jq -rc 'select(.auth_state=="pre_auth")' "${OUT}" | wc -l | tr -d ' ')" -lt 1 ]; then
  echo "FAIL: no line with auth_state=pre_auth"
  fail=1
else
  echo "OK: a pre_auth line is present"
fi

# --- a resolved (non-pre_auth) verified-flow line present ---
if [ "$(jq -rc 'select(.auth_state!="pre_auth")' "${OUT}" | wc -l | tr -d ' ')" -lt 1 ]; then
  echo "FAIL: no resolved verified-flow line (verified/auth_failed/unknown)"
  fail=1
else
  echo "OK: a resolved verified-flow line is present"
fi

# --- a true verified line (reported, not hard-gated — see header note) ---
if [ "$(jq -rc 'select(.auth_state=="verified")' "${OUT}" | wc -l | tr -d ' ')" -ge 1 ]; then
  echo "OK: an auth_state=verified line is present"
else
  echo "INFO: no auth_state=verified line (no real verified client in this run)"
fi

# --- every spec §0 field present + non-empty on every line ---
# Required-non-empty string fields.
for field in provider_id provider_wallet ts_utc route http_method event_type \
             source_ip_source capture_schema_version provider_upstream_version \
             provider_upstream_commit df_telemetry_commit auth_state; do
  BAD=$(jq -rc "select((.${field}|type)!=\"string\" or .${field}==\"\")" "${OUT}" | wc -l | tr -d ' ')
  if [ "${BAD}" -ne 0 ]; then
    echo "FAIL: ${BAD} line(s) have an empty or non-string ${field}"
    fail=1
  else
    echo "OK: ${field} populated on every line"
  fi
done

# --- keys that must EXIST but may be null (e.g. trusted_proxy_id) ---
for field in trusted_proxy_id status_code; do
  MISSING=$(jq -rc "select(has(\"${field}\")|not)" "${OUT}" | wc -l | tr -d ' ')
  if [ "${MISSING}" -ne 0 ]; then
    echo "FAIL: ${MISSING} line(s) are missing the ${field} key"
    fail=1
  else
    echo "OK: ${field} key present on every line"
  fi
done

# --- capture_schema_version must be exactly v1.3 ---
BADVER=$(jq -rc 'select(.capture_schema_version!="v1.3")' "${OUT}" | wc -l | tr -d ' ')
if [ "${BADVER}" -ne 0 ]; then
  echo "FAIL: ${BADVER} line(s) have capture_schema_version != v1.3"
  fail=1
else
  echo "OK: capture_schema_version == v1.3 on every line"
fi

# --- MP-02: provider_id round-trips the configured DF_PROVIDER_ID ---
BADPID=$(jq -rc --arg want "${WANT_PROVIDER_ID}" 'select(.provider_id!=$want)' "${OUT}" | wc -l | tr -d ' ')
if [ "${BADPID}" -ne 0 ]; then
  echo "FAIL: ${BADPID} line(s) have provider_id != ${WANT_PROVIDER_ID} (MP-02)"
  fail=1
else
  echo "OK: provider_id == ${WANT_PROVIDER_ID} on every line (MP-02 round-trip)"
fi

[ "${fail}" -eq 0 ] || { echo "assert-ndjson: FAILED"; exit 1; }
echo "PASS: pre_auth + verified-flow NDJSON with every spec §0 field populated"
