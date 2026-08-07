#!/bin/sh
# df-telemetry/ tree-hash guard primitive.
#
# Usage:
#   df-telemetry-treehash.sh <ref>             -> prints the df-telemetry/ tree SHA at <ref>
#   df-telemetry-treehash.sh --compare <a> <b> -> exit 0 if the df-telemetry/ tree
#                                                 SHA is equal at both refs, exit 1
#                                                 if it differs
#
# Git content-addresses every directory as a tree object, so comparing
# `git rev-parse <ref>:df-telemetry` between two refs is an exact, rename-aware
# equality check (RESEARCH "Don't Hand-Roll": never diff file lists by hand).
#
# REF SEMANTICS (the subtle part). The auto-merge guard compares the PRE-rebase
# fork state against the POST-rebase fork state — NOT against the upstream base
# tag. The upstream base (df-base/<tag>) never contains df-telemetry/, so a
# compare against it would always report "changed". auto-merge.yaml therefore
# resolves:
#   <a> = the PR's base branch (origin/main, the fork before the rebase)
#   <b> = the PR head (the rebased branch)
# Both have df-telemetry/; a difference means the rebase modified our hook and a
# human must review (FORK-05/FORK-06/CI-03).
set -eu

if [ "${1:-}" = "--compare" ]; then
  if [ "$#" -lt 3 ]; then
    echo "usage: df-telemetry-treehash.sh --compare <ref-a> <ref-b>" >&2
    exit 2
  fi
  A=$(git rev-parse "${2}:df-telemetry" 2>/dev/null || echo "MISSING_A")
  B=$(git rev-parse "${3}:df-telemetry" 2>/dev/null || echo "MISSING_B")
  if [ "$A" = "$B" ] && [ "$A" != "MISSING_A" ]; then
    echo "df-telemetry/ unchanged ($A) -> safe to auto-merge if CI green"
    exit 0
  fi
  echo "df-telemetry/ CHANGED ($A != $B) -> HUMAN REVIEW REQUIRED"
  exit 1
fi

git rev-parse "${1:-HEAD}:df-telemetry"
