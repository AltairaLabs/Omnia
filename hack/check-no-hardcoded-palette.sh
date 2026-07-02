#!/usr/bin/env bash
# check-no-hardcoded-palette.sh
#
# Enforces the white-label design-token contract (#1690): dashboard UI colors
# must come from design tokens (bg-success, text-warning, bg-category-4,
# text-muted-foreground, …) so the UI re-themes for SI-partner brands — never
# from hardcoded Tailwind palette shades (bg-green-100, text-red-600,
# dark:bg-amber-900/30, …), which are frozen and ignore the active brand.
#
# The ~1,000-hit long tail was migrated in #1690; this guard prevents *new*
# hardcoded palette classes from creeping back into tokenized files.
#
# A file violates the rule if it contains any Tailwind color-utility class of
# the form <prefix>-<palette>-<shade>, e.g.:
#   bg-green-100  text-red-600  border-amber-500  dark:bg-blue-900/30
#   ring-purple-400  fill-slate-700  hover:text-yellow-500
#
# Exit codes:
#   0  no new violations
#   1  one or more non-allowlisted files contain a hardcoded palette class
#
# Allowlist: files with intentional, non-themeable palette usage are pinned in
# `hack/no-hardcoded-palette.allowlist` (third-party vendor/framework brand
# swatches, skeuomorphic affordances, fixed-dark canvas chrome). Removing a
# file from that list (by tokenizing it) requires deleting its line — the lint
# will tell you if an allowlist entry is stale.
#
# Test files (*.test.ts, *.test.tsx) are NOT scanned: they legitimately assert
# on class strings, including allowlisted brand colors.
#
# Usage:
#   bash hack/check-no-hardcoded-palette.sh

set -u

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

allowlist_file="hack/no-hardcoded-palette.allowlist"

allowlist="$tmp/allowed"
if [ -f "$allowlist_file" ]; then
    sed -e 's/#.*//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$allowlist_file" \
        | grep -v '^$' \
        | LC_ALL=C sort -u >"$allowlist"
else
    : >"$allowlist"
fi

# Tailwind palette color-utility pattern: <prefix>-<palette>-<shade>. Variants
# (dark:, hover:, etc.) and opacity suffixes (/30) are matched by the surrounding
# grep since we only need to detect presence of the core token.
prefix='(bg|text|border|ring|fill|stroke|from|to|via|divide|outline|decoration|accent|caret)'
palette='(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)'
shade='(50|100|200|300|400|500|600|700|800|900|950)'
pattern="${prefix}-${palette}-${shade}"

current="$tmp/current"
# List non-test dashboard source files that contain a hardcoded palette class.
grep -rlE "$pattern" dashboard/src \
    --include='*.ts' --include='*.tsx' 2>/dev/null \
    | grep -v -e '\.test\.ts$' -e '\.test\.tsx$' \
    | LC_ALL=C sort -u >"$current" || true
[ -f "$current" ] || : >"$current"

new_violations="$(comm -23 "$current" "$allowlist")"
stale_allowlist="$(comm -13 "$current" "$allowlist")"

fail=0
if [ -n "$new_violations" ]; then
    fail=1
    echo "✗ hardcoded Tailwind palette classes in tokenized files (not in $allowlist_file):"
    while IFS= read -r f; do
        echo "  - $f"
        grep -nE "$pattern" "$f" | sed 's/^/      /' | head -8
    done <<<"$new_violations"
    echo ""
    echo "Dashboard colors must use design tokens so the UI re-themes under white-label"
    echo "branding. Map palette shades to tokens:"
    echo "  green→success  red→destructive  amber/yellow/orange→warning  blue→info"
    echo "  gray/slate/zinc→muted-foreground/muted/border"
    echo "  categorical sets (node/provider/category)→category-N via @/lib/colors/category"
    echo "  status→getStatusClasses() from @/lib/colors/status"
    echo ""
    echo "If this is intentional non-themeable identity (third-party vendor/framework brand"
    echo "color, skeuomorphic affordance, fixed-dark canvas chrome), add the file to"
    echo "  $allowlist_file  with a comment explaining why."
fi

if [ -n "$stale_allowlist" ]; then
    fail=1
    echo "✗ stale entries in $allowlist_file (these no longer contain palette classes — remove them):"
    while IFS= read -r f; do
        echo "  - $f"
    done <<<"$stale_allowlist"
fi

if [ $fail -eq 0 ]; then
    allow_count=$(wc -l <"$allowlist" | tr -d '[:space:]')
    echo "✓ no new hardcoded palette classes ($allow_count file(s) allowlisted as intentional identity)"
    exit 0
fi

exit 1
