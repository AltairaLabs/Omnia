#!/usr/bin/env bash
# check-file-length.sh
#
# Ratcheting file-length guardrail (see issue #1325). golangci-lint and
# SonarCloud gate *function* complexity but nothing gates *file* size, so a
# 2,000-line file made of 60 simple functions sails through clean. This check
# adds the missing axis: files may not exceed a per-language line threshold
# unless they are pinned in the baseline allowlist, and baselined files may only
# grow within a small grace margin (ratchet down only).
#
# Thresholds (non-blank lines):
#   Go        800
#   TS / TSX  500
#
# Excluded entirely:
#   - test files: *_test.go, *.test.ts, *.test.tsx
#   - generated:  *.gen.go, *.pb.go, *zz_generated*, *.d.ts,
#                 dashboard/src/types/generated/**, dashboard/.next/**, **/proto/**
#   - vendored:   promptkit-local/**, **/node_modules/**, .worktrees/**
#
# Exempt (no threshold, never baselined):
#   - api/v1alpha1/*_types.go   (flat CRD struct defs — adding a field shouldn't fail CI)
#   - cmd/**/main.go, ee/cmd/**/main.go  (entrypoints, already Sonar-exempt)
#
# Ratchet: a baselined file fails if it exceeds baseline + max(25 lines, 5%).
# A baselined file that drops to/under the threshold is a "stale" entry and must
# be removed (run --update) so it is gated as a normal file going forward.
#
# Usage:
#   bash hack/check-file-length.sh            # full-repo scan (CI)
#   bash hack/check-file-length.sh --staged   # only staged files (pre-commit)
#   bash hack/check-file-length.sh --update   # regenerate the baseline
#
# Test overrides (used by check-file-length_test.sh):
#   FILE_LENGTH_ROOT      root directory to scan (default: git repo root)
#   FILE_LENGTH_BASELINE  baseline file path (default: <root>/hack/file-length-baseline.txt)

set -u

GO_THRESHOLD=800
TS_THRESHOLD=500
GRACE_FLOOR=25
GRACE_PERCENT=5

mode="all"
case "${1:-}" in
    --update) mode="update" ;;
    --staged) mode="staged" ;;
    --all|"") mode="all" ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
esac

if [ -n "${FILE_LENGTH_ROOT:-}" ]; then
    root="$FILE_LENGTH_ROOT"
else
    root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
baseline_file="${FILE_LENGTH_BASELINE:-$root/hack/file-length-baseline.txt}"

# is_excluded <relpath> -> 0 if the file is outside scope
is_excluded() {
    case "$1" in
        *_test.go|*.test.ts|*.test.tsx) return 0 ;;
        *.gen.go|*.pb.go|*zz_generated*|*.d.ts) return 0 ;;
        dashboard/src/types/generated/*|dashboard/.next/*) return 0 ;;
        */proto/*) return 0 ;;
        promptkit-local/*|*/node_modules/*|.worktrees/*|*/.worktrees/*) return 0 ;;
    esac
    return 1
}

# is_exempt <relpath> -> 0 if the file is exempt from the threshold entirely
is_exempt() {
    case "$1" in
        api/v1alpha1/*_types.go) return 0 ;;
        cmd/*/main.go|ee/cmd/*/main.go) return 0 ;;
    esac
    return 1
}

# threshold_for <relpath>
threshold_for() {
    case "$1" in
        *.go) echo "$GO_THRESHOLD" ;;
        *) echo "$TS_THRESHOLD" ;;
    esac
}

# count_nonblank <file> -> number of non-blank lines
count_nonblank() {
    awk 'NF{c++} END{print c+0}' "$1"
}

# grace_for <baseline_count>
grace_for() {
    local base="$1" pct
    pct=$(( base * GRACE_PERCENT / 100 ))
    if [ "$pct" -gt "$GRACE_FLOOR" ]; then echo "$pct"; else echo "$GRACE_FLOOR"; fi
}

# list_candidate_files -> relpaths of in-scope, non-exempt source files
list_candidate_files() {
    local files rel
    if [ "$mode" = "staged" ]; then
        files="$(git -C "$root" diff --cached --name-only --diff-filter=ACM 2>/dev/null \
            | grep -E '\.(go|ts|tsx)$' || true)"
    else
        files="$(cd "$root" && find . -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) \
            | sed 's#^\./##')"
    fi
    while IFS= read -r rel; do
        [ -n "$rel" ] || continue
        [ -f "$root/$rel" ] || continue
        is_excluded "$rel" && continue
        is_exempt "$rel" && continue
        echo "$rel"
    done <<<"$files"
}

# ---------------------------------------------------------------------------
# --update: regenerate the baseline from the current tree (full scan).
# ---------------------------------------------------------------------------
if [ "$mode" = "update" ]; then
    mode="all"  # candidate listing uses full scan
    tmp="$(mktemp)"
    while IFS= read -r rel; do
        [ -n "$rel" ] || continue
        n="$(count_nonblank "$root/$rel")"
        thr="$(threshold_for "$rel")"
        if [ "$n" -gt "$thr" ]; then
            echo "$n $rel" >>"$tmp"
        fi
    done < <(list_candidate_files)
    LC_ALL=C sort -k2 "$tmp" >"$baseline_file"
    rm -f "$tmp"
    count="$(wc -l <"$baseline_file" | tr -d '[:space:]')"
    echo "✓ regenerated $baseline_file ($count entries)"
    exit 0
fi

# ---------------------------------------------------------------------------
# Load baseline into parallel arrays (bash 3.2 compatible — no associative).
# ---------------------------------------------------------------------------
base_paths=()
base_counts=()
if [ -f "$baseline_file" ]; then
    while IFS= read -r line; do
        line="${line%%#*}"
        line="$(echo "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
        [ -n "$line" ] || continue
        c="${line%% *}"
        p="${line#* }"
        base_paths+=("$p")
        base_counts+=("$c")
    done <"$baseline_file"
fi

baseline_index() {  # echo index of $1 in base_paths, or -1
    local i
    for i in "${!base_paths[@]}"; do
        if [ "${base_paths[$i]}" = "$1" ]; then echo "$i"; return; fi
    done
    echo "-1"
}

new_offenders=""
grew=""
seen_flags=()
for _ in "${base_paths[@]:-}"; do seen_flags+=("0"); done

while IFS= read -r rel; do
    [ -n "$rel" ] || continue
    n="$(count_nonblank "$root/$rel")"
    thr="$(threshold_for "$rel")"
    idx="$(baseline_index "$rel")"
    if [ "$idx" != "-1" ]; then
        seen_flags[idx]="1"
        base="${base_counts[$idx]}"
        grace="$(grace_for "$base")"
        cap=$(( base + grace ))
        if [ "$n" -le "$thr" ]; then
            : # handled as stale below (full mode only)
        elif [ "$n" -gt "$cap" ]; then
            grew="$grew  - $rel ($n lines, baseline $base, cap $cap)"$'\n'
        fi
    else
        if [ "$n" -gt "$thr" ]; then
            new_offenders="$new_offenders  - $rel ($n lines, limit $thr)"$'\n'
        fi
    fi
done < <(list_candidate_files)

# Stale baseline entries (file now <= threshold, or gone). Full scans only —
# a staged scan doesn't see every file, so it can't judge staleness.
stale=""
if [ "$mode" = "all" ]; then
    for i in "${!base_paths[@]}"; do
        rel="${base_paths[$i]}"
        thr="$(threshold_for "$rel")"
        if [ ! -f "$root/$rel" ]; then
            stale="$stale  - $rel (file no longer present)"$'\n'
        elif [ "${seen_flags[$i]}" = "1" ]; then
            n="$(count_nonblank "$root/$rel")"
            if [ "$n" -le "$thr" ]; then
                stale="$stale  - $rel (now $n lines, at/under limit $thr)"$'\n'
            fi
        fi
    done
fi

fail=0
if [ -n "$new_offenders" ]; then
    fail=1
    echo "✗ new files exceed the length threshold (split them, or pin in $baseline_file):"
    printf '%s' "$new_offenders"
    echo ""
    echo "  A file should do one thing. See issue #1325 — split by responsibility"
    echo "  (Go methods on one struct can live in any file in the package)."
fi
if [ -n "$grew" ]; then
    fail=1
    echo "✗ baselined files grew beyond their grace margin (shrink, or split):"
    printf '%s' "$grew"
fi
if [ -n "$stale" ]; then
    fail=1
    echo "✗ stale baseline entries (now at/under the threshold — remove them, run --update):"
    printf '%s' "$stale"
fi

if [ "$fail" -eq 0 ]; then
    n_base="${#base_paths[@]}"
    echo "✓ file-length guardrail passed ($n_base baselined offenders)"
    exit 0
fi
exit 1
