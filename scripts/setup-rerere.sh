#!/bin/sh
# Enables git rerere repo-wide. rerere CANNOT be path-scoped by Git config
# (RESEARCH Pitfall 4) — df-telemetry/ is protected instead by the
# tree-hash auto-merge guard (scripts/df-telemetry-treehash.sh), which is
# the real enforcement of FORK-06/CI-03.
set -eu
git config rerere.enabled true
git config rerere.autoupdate false   # never auto-stage a rerere resolution
echo "rerere enabled; autoupdate OFF — df-telemetry/ changes stay human-gated via the tree-hash guard"
