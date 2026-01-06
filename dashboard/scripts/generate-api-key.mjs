#!/usr/bin/env node
/**
 * Generate API key for Kubernetes Secret.
 *
 * This script generates a new API key with bcrypt hash for use with
 * the file-based API key store. The output can be added to a K8s Secret.
 *
 * Usage:
 *   node scripts/generate-api-key.mjs --user=<user-id> --name=<key-name> [options]
 *
 * Options:
 *   --user=<id>        User ID (required)
 *   --name=<name>      Key name (required)
 *   --role=<role>      User role: admin, editor, viewer (default: viewer)
 *   --expires=<days>   Days until expiration (0 = never, default: 90)
 *   --json             Output full JSON entry for keys.json
 *
 * Example:
 *   node scripts/generate-api-key.mjs --user=john --name="CI Pipeline" --role=editor
 *   node scripts/generate-api-key.mjs --user=system --name="Monitoring" --role=viewer --expires=0
 */

import { randomBytes, createHash } from "crypto";
import bcrypt from "bcryptjs";

const API_KEY_PREFIX = "omnia_sk_";
const BCRYPT_ROUNDS = 10;

function parseArgs() {
  const args = {};
  for (const arg of process.argv.slice(2)) {
    if (arg.startsWith("--")) {
      const [key, value] = arg.slice(2).split("=");
      args[key] = value ?? true;
    }
  }
  return args;
}

function generateKey() {
  // Generate 32 random bytes -> 64 hex chars
  const randomPart = randomBytes(32).toString("hex");
  return `${API_KEY_PREFIX}${randomPart}`;
}

function generateId() {
  // Short random ID for the key entry
  return randomBytes(8).toString("hex");
}

async function main() {
  const args = parseArgs();

  // Validate required args
  if (!args.user) {
    console.error("Error: --user is required");
    console.error("Usage: node scripts/generate-api-key.mjs --user=<id> --name=<name>");
    process.exit(1);
  }

  if (!args.name) {
    console.error("Error: --name is required");
    console.error("Usage: node scripts/generate-api-key.mjs --user=<id> --name=<name>");
    process.exit(1);
  }

  const role = args.role || "viewer";
  if (!["admin", "editor", "viewer"].includes(role)) {
    console.error("Error: --role must be one of: admin, editor, viewer");
    process.exit(1);
  }

  const expiresDays = parseInt(args.expires ?? "90", 10);

  // Generate the key
  const key = generateKey();
  const keyId = generateId();
  const keyPrefix = key.slice(0, 20) + "...";

  // Hash with bcrypt
  console.error("Generating key...");
  const keyHash = await bcrypt.hash(key, BCRYPT_ROUNDS);

  const now = new Date();
  const expiresAt =
    expiresDays > 0
      ? new Date(now.getTime() + expiresDays * 24 * 60 * 60 * 1000).toISOString()
      : null;

  const entry = {
    id: keyId,
    userId: args.user,
    name: args.name,
    keyPrefix,
    keyHash,
    role,
    expiresAt,
    createdAt: now.toISOString(),
  };

  if (args.json) {
    // Output full JSON for keys.json
    console.log(JSON.stringify(entry, null, 2));
  } else {
    // Human-readable output
    console.log("");
    console.log("=".repeat(60));
    console.log("API KEY GENERATED");
    console.log("=".repeat(60));
    console.log("");
    console.log("IMPORTANT: Save this key now. It cannot be retrieved later.");
    console.log("");
    console.log("API Key:");
    console.log(`  ${key}`);
    console.log("");
    console.log("Key Details:");
    console.log(`  ID:       ${keyId}`);
    console.log(`  User ID:  ${args.user}`);
    console.log(`  Name:     ${args.name}`);
    console.log(`  Role:     ${role}`);
    console.log(`  Expires:  ${expiresAt || "Never"}`);
    console.log("");
    console.log("Add this entry to your keys.json:");
    console.log("-".repeat(60));
    console.log(JSON.stringify(entry, null, 2));
    console.log("-".repeat(60));
    console.log("");
    console.log("To use in Kubernetes Secret:");
    console.log("  1. Add the entry to your keys.json file");
    console.log("  2. Create/update the Secret:");
    console.log("     kubectl create secret generic omnia-api-keys \\");
    console.log("       --from-file=keys.json=/path/to/keys.json \\");
    console.log("       --dry-run=client -o yaml | kubectl apply -f -");
    console.log("");
  }
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
