/**
 * Tests for Prometheus metadata proxy route.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";

interface ProxyConfig {
  endpoint: string;
  extractParams: (req: { nextUrl: { searchParams: URLSearchParams } }) => { params: Record<string, string> };
}

const state = vi.hoisted(() => ({
  capturedConfig: undefined as ProxyConfig | undefined,
}));

vi.mock("@/lib/prometheus-proxy", () => ({
  createPrometheusProxy: (config: ProxyConfig) => {
    state.capturedConfig = config;
    return vi.fn();
  },
}));

// Import triggers module-level createPrometheusProxy call
import "./route";

describe("prometheus metadata route", () => {
  it("creates proxy with metadata endpoint", () => {
    expect(state.capturedConfig).toBeDefined();
    expect(state.capturedConfig!.endpoint).toBe("metadata");
  });

  it("extracts metric param when provided", () => {
    const request = {
      nextUrl: {
        searchParams: new URLSearchParams({ metric: "omnia_eval_tone" }),
      },
    };
    const result = state.capturedConfig!.extractParams(request);
    expect(result).toEqual({ params: { metric: "omnia_eval_tone" } });
  });

  it("returns empty params when metric is not provided", () => {
    const request = {
      nextUrl: { searchParams: new URLSearchParams() },
    };
    const result = state.capturedConfig!.extractParams(request);
    expect(result).toEqual({ params: {} });
  });
});
