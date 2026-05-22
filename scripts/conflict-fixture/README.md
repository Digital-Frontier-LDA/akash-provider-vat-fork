# df-telemetry/ tree-hash guard — conflict fixture

This directory holds the **deliberate-conflict test fixture** that proves the
`df-telemetry/` tree-hash guard actually blocks rebases touching our hook.

## Why it exists

FORK-05 requires the auto-merge pipeline to "refuse to auto-merge when the
directory hash of `df-telemetry/` changes — verified by a deliberate-conflict
test fixture." This fixture is that verification: it lets the guard be exercised
against both a `df-telemetry/`-touching change and a clean one, so a regression
in the guard is caught by CI.

## What it builds

`make-fixture.sh` creates two throwaway **local** branches off `main` (never
pushed, never part of the rebase pipeline):

| Branch | Simulates | Guard must |
|--------|-----------|------------|
| `df-fixture/overlapping` | a rebase that modified a file under `df-telemetry/` | **block** (exit non-zero) |
| `df-fixture/non-overlapping` | a rebase that modified only an upstream file | **allow** (exit zero) |

The script is deterministic and idempotent — it deletes and recreates the
branches each run, and prints the two branch names on stdout.

`clean-fixture.sh` deletes the two fixture branches.

## How the self-test consumes it

`.github/workflows/rebase-guard-selftest.yml`:

1. runs `make-fixture.sh` to build both branches;
2. runs `scripts/df-telemetry-treehash.sh --compare main df-fixture/overlapping`
   and asserts it exits **non-zero** (the guard blocked the hook-touching case);
3. runs `scripts/df-telemetry-treehash.sh --compare main df-fixture/non-overlapping`
   and asserts it exits **zero** (the guard allowed the clean case);
4. runs `clean-fixture.sh`.

A failure of either assertion fails the workflow — that is the executable proof
that FORK-05's deliberate-conflict requirement holds.
