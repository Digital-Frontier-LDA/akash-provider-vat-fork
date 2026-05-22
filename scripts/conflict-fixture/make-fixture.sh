#!/bin/sh
# Builds the two deliberate-conflict test fixtures for the df-telemetry/
# tree-hash guard self-test (FORK-05 verification).
#
# Both fixtures are throwaway LOCAL branches off main — they are never pushed
# and never enter the rebase pipeline. They exist only so rebase-guard-
# selftest.yml can point the tree-hash guard at them:
#
#   df-fixture/overlapping     — a commit that edits a file UNDER df-telemetry/
#                                ("the rebase touched our hook"). The guard
#                                MUST block this (exit non-zero).
#   df-fixture/non-overlapping — a commit that edits an upstream-tracked file
#                                OUTSIDE df-telemetry/ ("the rebase did not
#                                touch our hook"). The guard MUST allow this
#                                (exit zero).
#
# Deterministic and idempotent: the branches are deleted and recreated each run.
set -eu

OVERLAP_BRANCH="df-fixture/overlapping"
CLEAN_BRANCH="df-fixture/non-overlapping"

START_REF=$(git rev-parse --abbrev-ref HEAD)

# Recreate from a clean slate.
git branch -D "${OVERLAP_BRANCH}" 2>/dev/null || true
git branch -D "${CLEAN_BRANCH}" 2>/dev/null || true

# --- Overlapping fixture: modifies df-telemetry/ ---
git checkout -q -b "${OVERLAP_BRANCH}" main
printf '\n// fixture: deliberate df-telemetry/ change for the guard self-test\n' \
  >> df-telemetry/doc.go
git add df-telemetry/doc.go
git -c user.name=df-fixture -c user.email=df-fixture@local \
  commit -q -m "fixture: simulate a rebase that touched df-telemetry/"

# --- Non-overlapping fixture: modifies an upstream file only ---
git checkout -q -b "${CLEAN_BRANCH}" main
# README.md is upstream-tracked and outside df-telemetry/.
printf '\n<!-- fixture: non-overlapping change for the guard self-test -->\n' \
  >> README.md
git add README.md
git -c user.name=df-fixture -c user.email=df-fixture@local \
  commit -q -m "fixture: simulate a rebase that did NOT touch df-telemetry/"

git checkout -q "${START_REF}"

# Print the two ref names so the self-test workflow can consume them.
echo "${OVERLAP_BRANCH}"
echo "${CLEAN_BRANCH}"
