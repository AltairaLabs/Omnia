import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import bcrypt from "bcryptjs";
import { FileApiKeyStore } from "./file-store";

const KEY = "omnia_sk_filetest_abcdefghijklmnopqrstuvwxyz012345";
let dir: string;
let file: string;

beforeEach(async () => {
  dir = mkdtempSync(join(tmpdir(), "omnia-keys-"));
  file = join(dir, "keys.json");
  const keyHash = await bcrypt.hash(KEY, 10);
  writeFileSync(file, JSON.stringify({
    keys: [{
      id: "k1", userId: "u1", name: "scoped", keyPrefix: "omnia_sk_filetes...",
      keyHash, role: "editor", expiresAt: null, createdAt: "2026-01-01T00:00:00Z",
      workspaces: ["demo"],
    }],
  }));
});

afterEach(() => rmSync(dir, { recursive: true, force: true }));

describe("FileApiKeyStore workspaces", () => {
  it("returns the workspaces allowlist parsed from the file", async () => {
    const store = new FileApiKeyStore(file, { watch: false });
    const found = await store.findByKey(KEY);
    expect(found?.workspaces).toEqual(["demo"]);
  });
});
