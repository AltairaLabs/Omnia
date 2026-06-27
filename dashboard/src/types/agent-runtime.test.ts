/**
 * Tests for the small helpers in agent-runtime.ts. Most of the file is
 * type declarations (no runtime); the only executable code that needs
 * coverage is getDefaultProviderRef and isFunctionMode.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect } from "vitest";
import {
  getDefaultProviderRef,
  isFunctionMode,
  type AgentRuntimeSpec,
} from "./agent-runtime";

function mkSpec(overrides: Partial<AgentRuntimeSpec> = {}): AgentRuntimeSpec {
  return {
    promptPackRef: { name: "pack" },
    facades: [{ type: "rest" }],
    ...overrides,
  };
}

describe("isFunctionMode", () => {
  it("returns true when spec.mode === 'function'", () => {
    expect(isFunctionMode(mkSpec({ mode: "function" }))).toBe(true);
  });

  it("returns false when spec.mode === 'agent'", () => {
    expect(isFunctionMode(mkSpec({ mode: "agent" }))).toBe(false);
  });

  it("returns false when spec.mode is unset (legacy default = agent)", () => {
    expect(isFunctionMode(mkSpec())).toBe(false);
  });
});

describe("getDefaultProviderRef", () => {
  it("returns undefined when no providers are configured", () => {
    expect(getDefaultProviderRef(mkSpec())).toBeUndefined();
  });

  it("returns the provider named 'default' when present", () => {
    const ref = getDefaultProviderRef(
      mkSpec({
        providers: [
          { name: "primary", providerRef: { name: "openai-1" } },
          { name: "default", providerRef: { name: "anthropic-1" } },
        ],
      }),
    );
    expect(ref).toEqual({ name: "anthropic-1" });
  });

  it("falls back to the first provider when none is named 'default'", () => {
    const ref = getDefaultProviderRef(
      mkSpec({
        providers: [{ name: "primary", providerRef: { name: "openai-1" } }],
      }),
    );
    expect(ref).toEqual({ name: "openai-1" });
  });
});
