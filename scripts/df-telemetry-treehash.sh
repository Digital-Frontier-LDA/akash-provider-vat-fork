#!/bin/sh
# Prints the Git tree object SHA of df-telemetry/ at a given ref (default HEAD).
# Git content-addresses every directory as a tree object; comparing tree SHAs
# is exact and rename-aware. Plan 05's auto-merge.yml diffs the pre-rebase fork
# state's df-telemetry tree SHA against the rebased HEAD's — any difference
# means a human must review (FORK-05/FORK-06/CI-03).
#
# Note: at df-base/<tag> the df-telemetry/ directory does NOT exist (it was
# added by the fork after the upstream base). The guard treats "tree missing
# at one ref" as "changed -> human review". Plan 05 extends this script with
# a --compare mode and compares the PRE-rebase fork branch against the
# POST-rebase fork branch (both of which have df-telemetry/).
set -eu
REF="${1:-HEAD}"
git rev-parse "${REF}:df-telemetry"
