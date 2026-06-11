/**
 * Tests for the shared operator API client helpers.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("node:fs/promises", () => {
  const readFile = vi.fn();
  return { readFile, default: { readFile } };
});

import { operatorAuthToken } from "./operator-client";
import { readFile } from "node:fs/promises";

beforeEach(() => {
  vi.clearAllMocks();
  delete process.env.OPERATOR_TOOL_TEST_TOKEN;
});

describe("operatorAuthToken", () => {
  it("returns OPERATOR_TOOL_TEST_TOKEN when that env var is set", async () => {
    process.env.OPERATOR_TOOL_TEST_TOKEN = "explicit-test-token";
    const token = await operatorAuthToken();
    expect(token).toBe("explicit-test-token");
    expect(readFile).not.toHaveBeenCalled();
  });

  it("returns null when token file cannot be read and no env var is set", async () => {
    vi.mocked(readFile).mockRejectedValue(new Error("ENOENT: no such file or directory"));
    const token = await operatorAuthToken();
    expect(token).toBeNull();
  });
});
