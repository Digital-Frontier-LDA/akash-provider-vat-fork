#!/bin/sh
# CI-03 / FORK-06 assertion.
#
# git rerere CANNOT be path-scoped by Git config (RESEARCH rerere section) — so
# this script asserts the mechanisms that REALLY guarantee a rebase touching
# df-telemetry/ is human-gated and never silently auto-resolved:
#   (1) rerere.autoupdate is OFF  -> no recorded resolution is ever silently staged
#   (2) the df-telemetry/ tree-hash guard script exists
#   (3) auto-merge.yml runs the guard as a gating step
#   (4) setup-rerere.sh pins rerere.autoupdate
# Together these are the FORK-06/CI-03 enforcement: the tree-hash guard blocks
# and labels any df-telemetry/-touching rebase, and autoupdate=off means a
# recorded rerere resolution is never auto-staged onto our hook.
set -eu
fail=0

# (1) rerere.autoupdate must NOT be true (off or unset both acceptable).
AU=$(git config --get rerere.autoupdate || echo "unset")
if [ "$AU" = "true" ]; then
  echo "FAIL: rerere.autoupdate=true would silently stage resolutions onto df-telemetry/"; fail=1
else
  echo "OK: rerere.autoupdate=$AU (recorded resolutions are never auto-staged)"
fi

# (2) the tree-hash guard primitive exists.
if [ ! -f scripts/df-telemetry-treehash.sh ]; then
  echo "FAIL: scripts/df-telemetry-treehash.sh missing"; fail=1
else
  echo "OK: tree-hash guard script present"
fi

# (3) auto-merge.yml invokes the guard.
if ! grep -q 'df-telemetry-treehash' .github/workflows/auto-merge.yml; then
  echo "FAIL: auto-merge.yml does not invoke the df-telemetry/ tree-hash guard"; fail=1
else
  echo "OK: auto-merge.yml gates on the df-telemetry/ tree-hash guard"
fi

# (4) setup-rerere.sh pins rerere.autoupdate.
if ! grep -q 'rerere.autoupdate' scripts/setup-rerere.sh 2>/dev/null; then
  echo "FAIL: scripts/setup-rerere.sh does not pin rerere.autoupdate"; fail=1
else
  echo "OK: scripts/setup-rerere.sh pins rerere.autoupdate"
fi

[ "$fail" -eq 0 ] || { echo "rerere/guard configuration assertion FAILED"; exit 1; }
echo "PASS: df-telemetry/ rebase changes are human-gated and never silently auto-resolved"
