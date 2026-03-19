#!/usr/bin/env bash
#
# check-log-pii.sh — Detect high-risk PII patterns in Go log statements.
#
# Scans Go source files (excluding vendor, promptkit-local, and test files) for
# log key names that are likely to leak PII (message content, user identifiers,
# credentials, IP addresses). Maintains an allowlist for safe alternatives
# (e.g., "contentLength" vs "content").
#
# Exit 0 if clean, exit 1 if violations found.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

# High-risk log key patterns — these should never appear as bare keys in
# structured log calls because they typically carry PII or sensitive data.
RISKY_PATTERNS=(
    '"content"'
    '"body"'
    '"arguments"'
    '"email"'
    '"password"'
    '"token"'
    '"authorization"'
    '"userID"'
    '"userEmail"'
    '"remoteAddr"'
    '"raw"'
)

# Safe alternatives that look like risky patterns but are fine.
ALLOWLIST_PATTERNS=(
    '"contentType"'
    '"contentLength"'
    '"bodySize"'
    '"tokenCount"'
    '"tokenUsage"'
    '"argKeys"'
    '"userHash"'
    '"originHash"'
    '"urlHash"'
    '"clientHash"'
)

violations=0
violation_files=""

for pattern in "${RISKY_PATTERNS[@]}"; do
    # Search for risky patterns in log.Info(), log.Error(), log.V().Info() calls.
    # Use grep to find lines containing both a log call and the risky pattern.
    while IFS= read -r file; do
        [ -z "$file" ] && continue

        # Read matching lines from the file
        while IFS= read -r line; do
            [ -z "$line" ] && continue

            # Check if this line contains a log call
            if ! echo "$line" | grep -qE 'log\.(V\([0-9]\)\.)?Info\(|log\.Error\(|\.log\.(V\([0-9]\)\.)?Info\(|\.log\.Error\(|s\.log\.(V\([0-9]\)\.)?Info\(|s\.log\.Error\('; then
                continue
            fi

            # Check if any allowlist pattern is present — if so, skip
            skip=false
            for allow in "${ALLOWLIST_PATTERNS[@]}"; do
                if echo "$line" | grep -qF "$allow"; then
                    skip=true
                    break
                fi
            done
            if $skip; then
                continue
            fi

            # This is a genuine violation
            violations=$((violations + 1))
            echo "  VIOLATION: $file"
            echo "    Pattern: $pattern"
            echo "    Line: $(echo "$line" | sed 's/^[[:space:]]*//')"
            echo ""
            violation_files="$violation_files $file"
        done < <(grep -n "$pattern" "$file" 2>/dev/null || true)
    done < <(find "$REPO_ROOT" -name '*.go' \
        -not -path '*/vendor/*' \
        -not -path '*/promptkit-local/*' \
        -not -path '*_test.go' \
        -not -path '*/testdata/*' \
        2>/dev/null || true)
done

if [ $violations -gt 0 ]; then
    echo "========================================="
    echo "PII log check: $violations violation(s) found"
    echo "========================================="
    echo ""
    echo "The following log key patterns may leak PII in structured logs."
    echo "Replace them with safe alternatives from pkg/logging:"
    echo ""
    echo "  \"content\"    -> \"contentLength\", logging.ContentLength(content)"
    echo "  \"body\"       -> \"bodySize\", len(body)"
    echo "  \"arguments\"  -> \"argKeys\", logging.SafeMapKeys(args)"
    echo "  \"email\"      -> \"userHash\", logging.HashID(email)"
    echo "  \"userID\"     -> \"userHash\", logging.HashID(userID)"
    echo "  \"remoteAddr\" -> \"clientHash\", logging.HashID(remoteAddr)"
    echo "  \"token\"      -> (do not log tokens)"
    echo "  \"password\"   -> (do not log passwords)"
    echo "  \"raw\"        -> \"contentLength\", logging.ContentLength(raw)"
    echo ""
    exit 1
fi

echo "PII log check: clean (no violations found)"
exit 0
