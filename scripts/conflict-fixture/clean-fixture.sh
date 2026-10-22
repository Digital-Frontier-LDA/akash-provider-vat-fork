#!/bin/sh
# Deletes the throwaway fixture branches created by make-fixture.sh.
# Safe to run when the branches do not exist.
set -eu

git branch -D df-fixture/overlapping 2>/dev/null || true
git branch -D df-fixture/non-overlapping 2>/dev/null || true
echo "conflict-fixture branches removed"
