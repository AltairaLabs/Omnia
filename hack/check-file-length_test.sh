#!/usr/bin/env bash
# Tests for hack/check-file-length.sh
#
# Each test builds a throwaway fixture tree, points the checker at it via
# FILE_LENGTH_ROOT / FILE_LENGTH_BASELINE, and asserts exit code + output.
#
# Run: bash hack/check-file-length_test.sh

set -u

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/check-file-length.sh"

pass=0
fail=0

fail_test() {
    fail=$((fail + 1))
    echo "  ✗ FAIL: $1"
}
pass_test() {
    pass=$((pass + 1))
    echo "  ✓ $1"
}

# make_file <path> <non_blank_lines>
# Writes a file with the given number of non-blank lines plus a couple of blank
# lines (to prove blanks are not counted).
make_file() {
    local path="$1" n="$2" i
    mkdir -p "$(dirname "$path")"
    : >"$path"
    echo "" >>"$path"
    for ((i = 1; i <= n; i++)); do
        echo "line $i" >>"$path"
    done
    echo "" >>"$path"
}

# run_checker <root> <baseline> [args...] -> sets RC and OUT
run_checker() {
    local root="$1" baseline="$2"
    shift 2
    OUT="$(FILE_LENGTH_ROOT="$root" FILE_LENGTH_BASELINE="$baseline" bash "$checker" "$@" 2>&1)"
    RC=$?
}

# ---------------------------------------------------------------------------
# Test 1: clean tree (all files under threshold) -> exit 0
# ---------------------------------------------------------------------------
test_clean_tree() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/small.go" 100
    make_file "$root/dashboard/src/small.ts" 100
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "clean tree passes"; else
        fail_test "clean tree should pass (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 2: new Go file over 800, not baselined -> exit 1, names the file
# ---------------------------------------------------------------------------
test_new_go_offender() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/big.go" 900
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -ne 0 ] && echo "$OUT" | grep -q "internal/big.go"; then
        pass_test "new Go offender fails and is named"
    else
        fail_test "new Go offender should fail naming the file (rc=$RC): $OUT"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 3: new TS file over 500, not baselined -> exit 1
# ---------------------------------------------------------------------------
test_new_ts_offender() {
    local root; root="$(mktemp -d)"
    make_file "$root/dashboard/src/big.tsx" 600
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -ne 0 ] && echo "$OUT" | grep -q "dashboard/src/big.tsx"; then
        pass_test "new TS offender fails (threshold 500)"
    else
        fail_test "new TS offender should fail (rc=$RC): $OUT"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 4: baselined file within grace -> exit 0
#   baseline 900, grace = max(25, 5%) = 45 -> 940 allowed
# ---------------------------------------------------------------------------
test_baselined_within_grace() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/big.go" 930
    printf '900 internal/big.go\n' >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "baselined file within grace passes"; else
        fail_test "baselined within grace should pass (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 5: baselined file beyond grace -> exit 1
#   baseline 900, grace 45 -> 945 is the cap; 960 fails
# ---------------------------------------------------------------------------
test_baselined_beyond_grace() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/big.go" 960
    printf '900 internal/big.go\n' >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -ne 0 ] && echo "$OUT" | grep -q "internal/big.go"; then
        pass_test "baselined file beyond grace fails"
    else
        fail_test "baselined beyond grace should fail (rc=$RC): $OUT"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 6: grace uses 5% when that exceeds the 25-line floor
#   baseline 2000, 5% = 100 -> 2100 allowed, 2101 fails
# ---------------------------------------------------------------------------
test_grace_percentage() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/huge.go" 2100
    printf '2000 internal/huge.go\n' >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    local rc_ok=$RC
    make_file "$root/internal/huge.go" 2101
    run_checker "$root" "$root/baseline.txt"
    if [ "$rc_ok" -eq 0 ] && [ "$RC" -ne 0 ]; then
        pass_test "grace is max(25, 5%) — 5% applied for large files"
    else
        fail_test "5% grace boundary wrong (2100 rc=$rc_ok, 2101 rc=$RC): $OUT"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 7: baselined file now under threshold -> stale, exit 1
# ---------------------------------------------------------------------------
test_stale_baseline_entry() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/shrunk.go" 400
    printf '900 internal/shrunk.go\n' >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -ne 0 ] && echo "$OUT" | grep -qi "stale\|remove"; then
        pass_test "stale baseline entry (now under threshold) fails"
    else
        fail_test "stale baseline entry should fail asking removal (rc=$RC): $OUT"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 8: exempt files (_types.go, main.go) over threshold -> ignored
# ---------------------------------------------------------------------------
test_exempt_files() {
    local root; root="$(mktemp -d)"
    make_file "$root/api/v1alpha1/agentruntime_types.go" 1500
    make_file "$root/cmd/operator/main.go" 1500
    make_file "$root/ee/cmd/worker/main.go" 1500
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "exempt _types.go and main.go are ignored"; else
        fail_test "exempt files should be ignored (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 9: test files over threshold -> ignored
# ---------------------------------------------------------------------------
test_test_files_ignored() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/store_test.go" 2000
    make_file "$root/dashboard/src/foo.test.ts" 2000
    make_file "$root/dashboard/src/foo.test.tsx" 2000
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "test files are ignored"; else
        fail_test "test files should be ignored (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 10: generated files over threshold -> ignored
# ---------------------------------------------------------------------------
test_generated_files_ignored() {
    local root; root="$(mktemp -d)"
    make_file "$root/api/v1alpha1/zz_generated.deepcopy.go" 3000
    make_file "$root/internal/foo.pb.go" 3000
    make_file "$root/internal/foo.gen.go" 3000
    make_file "$root/dashboard/src/lib/api/schema.d.ts" 3000
    make_file "$root/dashboard/src/types/generated/types.ts" 3000
    make_file "$root/dashboard/.next/types/validator.ts" 3000
    make_file "$root/dashboard/src/lib/proto/runtime/v1/runtime.ts" 3000
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "generated files are ignored"; else
        fail_test "generated files should be ignored (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 11: promptkit-local and node_modules ignored
# ---------------------------------------------------------------------------
test_vendored_ignored() {
    local root; root="$(mktemp -d)"
    make_file "$root/promptkit-local/sdk/options.go" 3000
    make_file "$root/dashboard/node_modules/pkg/index.ts" 3000
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "promptkit-local and node_modules ignored"; else
        fail_test "vendored trees should be ignored (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 12: blank lines are not counted
#   make_file writes N non-blank + 2 blank; a 900-non-blank Go file with a
#   baseline of exactly 900 must pass (blanks would push it over grace... no,
#   over baseline by 0). Use threshold edge: 800 non-blank exactly = at
#   threshold, not over -> not a new offender -> pass.
# ---------------------------------------------------------------------------
test_blank_lines_not_counted() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/edge.go" 800   # exactly at threshold, plus blanks
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt"
    if [ "$RC" -eq 0 ]; then pass_test "blank lines not counted (800 non-blank = at threshold)"; else
        fail_test "file at exactly 800 non-blank should pass (rc=$RC): $OUT"; fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Test 13: --update regenerates the baseline
# ---------------------------------------------------------------------------
test_update_regenerates_baseline() {
    local root; root="$(mktemp -d)"
    make_file "$root/internal/big.go" 900
    make_file "$root/dashboard/src/big.tsx" 600
    make_file "$root/internal/small.go" 100
    : >"$root/baseline.txt"
    run_checker "$root" "$root/baseline.txt" --update
    # After update, both offenders recorded, small file not, and a re-check passes.
    if grep -q "internal/big.go" "$root/baseline.txt" \
        && grep -q "dashboard/src/big.tsx" "$root/baseline.txt" \
        && ! grep -q "internal/small.go" "$root/baseline.txt"; then
        run_checker "$root" "$root/baseline.txt"
        if [ "$RC" -eq 0 ]; then
            pass_test "--update records offenders; re-check passes"
        else
            fail_test "--update baseline should make re-check pass (rc=$RC): $OUT"
        fi
    else
        fail_test "--update did not record the right entries: $(cat "$root/baseline.txt")"
    fi
    rm -rf "$root"
}

# ---------------------------------------------------------------------------
echo "Running check-file-length.sh tests"
test_clean_tree
test_new_go_offender
test_new_ts_offender
test_baselined_within_grace
test_baselined_beyond_grace
test_grace_percentage
test_stale_baseline_entry
test_exempt_files
test_test_files_ignored
test_generated_files_ignored
test_vendored_ignored
test_blank_lines_not_counted
test_update_regenerates_baseline

echo ""
echo "Passed: $pass  Failed: $fail"
[ "$fail" -eq 0 ]
