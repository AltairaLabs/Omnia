#!/usr/bin/env bash
# check-wiring-tests.sh
#
# Enforces the "every service binary has a wiring test" contract from
# CLAUDE.md (Testing Standards section). For every `cmd/*/main.go` (and
# `ee/cmd/*/main.go`) the directory must contain at least one `_test.go`
# file. Without a test in the same dir, an assembled-binary regression
# (missing route registration, missing worker start, missing interceptor)
# only surfaces as a smoke-test failure in-cluster — exactly the failure
# class that gave us issue #1038's wiring audit.
#
# A test file matches if its name ends in `_test.go`. The lint does NOT
# attempt to verify what the test actually asserts on — content-level
# enforcement is the human reviewer's job. The shape "is there a test"
# is the cheap signal that catches binaries with zero coverage.
#
# Exit codes:
#   0  every cmd/main.go has a sibling _test.go
#   1  one or more main.go files lack a sibling test
#
# Usage:
#   bash hack/check-wiring-tests.sh

set -u

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

# Portable across bash 3.2 (macOS default) and bash 4+ — uses temp
# files + line-comparison instead of associative arrays.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Pre-existing gaps that this lint deliberately accepts. The list
# pins the current state so NEW binaries without wiring tests fail,
# while existing gaps are tracked as separate tickets rather than
# blocking unrelated PRs. Removing a gap (by adding a wiring test)
# requires deleting the line below — the lint will tell you to.
#
# To add a wiring test for one of these and remove it from the list:
# see CLAUDE.md → Testing Standards → 'Wiring tests'.
allowlist_file="hack/wiring-tests.allowlist"

# Build the allowlist (sorted, comments + blanks stripped) into a tmp
# file so we can compare it to the current set with `comm`. Plain
# files + sort + comm are portable across bash 3.2 (macOS) and bash
# 4+ (Linux CI) — declare -A is bash-4-only.
allowlist="$tmp/allowed"
if [ -f "$allowlist_file" ]; then
    sed -e 's/#.*//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$allowlist_file" \
        | grep -v '^$' \
        | LC_ALL=C sort -u >"$allowlist"
else
    : >"$allowlist"
fi

# Build the current set of cmd/*/main.go that lack a sibling _test.go.
current="$tmp/current"
: >"$current"
shopt -s nullglob
for main_path in cmd/*/main.go ee/cmd/*/main.go; do
    dir="$(dirname "$main_path")"
    if ! compgen -G "$dir/*_test.go" >/dev/null; then
        echo "$main_path" >>"$current"
    fi
done
shopt -u nullglob
LC_ALL=C sort -o "$current" "$current"

# Set differences. comm -23: lines only in current. comm -13: only in allowlist.
new_gaps="$(comm -23 "$current" "$allowlist")"
stale_allowlist="$(comm -13 "$current" "$allowlist")"

fail=0
if [ -n "$new_gaps" ]; then
    fail=1
    echo "✗ new wiring-test gap (not in $allowlist_file):"
    while IFS= read -r m; do
        echo "  - $m"
    done <<<"$new_gaps"
    echo ""
    echo "Add a wiring test (cmd/<name>/wiring_test.go or main_test.go) that exercises the assembled binary's"
    echo "cross-service contracts: registered routes, started workers, configured providers, etc."
    echo "See CLAUDE.md → Testing Standards → 'Wiring tests (service startup verification)'."
    echo ""
    echo "Why this matters: see issue #1038. Workers wired in main.go that silently never run because the"
    echo "operator doesn't pass the enabling flag pass unit tests — only an assembled-binary test catches them."
fi

if [ -n "$stale_allowlist" ]; then
    fail=1
    echo "✗ stale entries in $allowlist_file (these binaries now have a wiring test — remove them):"
    while IFS= read -r m; do
        echo "  - $m"
    done <<<"$stale_allowlist"
fi

if [ $fail -eq 0 ]; then
    current_count=$(wc -l <"$current" | tr -d '[:space:]')
    if [ "$current_count" = "0" ]; then
        echo "✓ all cmd/*/main.go binaries have a sibling _test.go"
    else
        echo "✓ no new wiring-test gaps ($current_count pre-existing, all allowlisted)"
    fi
    exit 0
fi

exit 1
