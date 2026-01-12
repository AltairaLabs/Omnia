#!/bin/bash
# Generate TypeScript types from Protocol Buffer definitions.
# This ensures type definitions are shared between Go backend and TypeScript frontend.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASHBOARD_DIR="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$DASHBOARD_DIR/../api/proto"
OUT_DIR="$DASHBOARD_DIR/src/lib/proto"

# Find the ts-proto plugin
TS_PROTO_PLUGIN="$DASHBOARD_DIR/node_modules/.bin/protoc-gen-ts_proto"
if [ ! -x "$TS_PROTO_PLUGIN" ]; then
  echo "Error: ts-proto plugin not found. Run 'npm install' first."
  exit 1
fi

# Ensure output directory exists
mkdir -p "$OUT_DIR"

echo "Generating TypeScript from proto files..."

# Generate runtime types
protoc \
  --plugin="protoc-gen-ts_proto=$TS_PROTO_PLUGIN" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt=esModuleInterop=true \
  --ts_proto_opt=outputServices=false \
  --ts_proto_opt=oneof=unions \
  --ts_proto_opt=useOptionals=messages \
  --ts_proto_opt=exportCommonSymbols=false \
  --ts_proto_opt=snakeToCamel=keys \
  --proto_path="$PROTO_DIR" \
  "$PROTO_DIR/runtime/v1/runtime.proto"

# Generate tools types
protoc \
  --plugin="protoc-gen-ts_proto=$TS_PROTO_PLUGIN" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt=esModuleInterop=true \
  --ts_proto_opt=outputServices=false \
  --ts_proto_opt=oneof=unions \
  --ts_proto_opt=useOptionals=messages \
  --ts_proto_opt=exportCommonSymbols=false \
  --ts_proto_opt=snakeToCamel=keys \
  --proto_path="$PROTO_DIR" \
  "$PROTO_DIR/tools/v1/tools.proto"

echo "Generated TypeScript types in $OUT_DIR"
echo "Files:"
ls -la "$OUT_DIR"
