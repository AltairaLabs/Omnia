#!/usr/bin/env bash
# Script to fetch the latest PromptPack schema and embed it in the codebase.
# Run this during development or CI to update the embedded schema.

set -euo pipefail

SCHEMA_URL="${SCHEMA_URL:-https://promptpack.org/schema/latest/promptpack.schema.json}"
OUTPUT_FILE="${OUTPUT_FILE:-internal/schema/promptpack.schema.json}"

echo "Fetching PromptPack schema from: ${SCHEMA_URL}"

# Fetch schema with error handling
if ! curl -sSfL "${SCHEMA_URL}" -o "${OUTPUT_FILE}"; then
    echo "ERROR: Failed to fetch schema from ${SCHEMA_URL}"
    exit 1
fi

# Validate it's valid JSON
if ! jq empty "${OUTPUT_FILE}" 2>/dev/null; then
    echo "ERROR: Downloaded file is not valid JSON"
    rm -f "${OUTPUT_FILE}"
    exit 1
fi

# Extract version for logging
VERSION=$(jq -r '.version // "unknown"' "${OUTPUT_FILE}")
echo "Successfully fetched PromptPack schema version: ${VERSION}"
echo "Saved to: ${OUTPUT_FILE}"
