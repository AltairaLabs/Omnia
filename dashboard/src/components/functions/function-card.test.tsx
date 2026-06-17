/**
 * Tests for FunctionCard.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { FunctionCard } from "./function-card";
import type { AgentRuntime } from "@/types";

vi.mock("next/link", () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// Cost is read from the same Prometheus-backed hook the agent cards use.
vi.mock("@/hooks/agents", () => ({
  useAgentCost: vi.fn(() => ({ data: { totalCost: 0, timeSeries: [] } })),
}));
vi.mock("@/hooks/resources", () => ({
  useProvider: vi.fn(() => ({ data: { spec: { type: "anthropic", model: "claude-opus-4-8" } } })),
}));

function mkFn(overrides: Partial<AgentRuntime> = {}): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "summarizer",
      namespace: "ns-a",
      uid: "uid-1",
    },
    spec: {
      mode: "function",
      promptPackRef: { name: "summarizer-pack" },
      facade: { type: "grpc" as never },
      inputSchema: {
        type: "object",
        properties: { q: { type: "string" }, k: { type: "integer" } },
      },
      outputSchema: {
        type: "object",
        properties: { a: { type: "string" } },
      },
    },
    ...overrides,
  };
}

describe("FunctionCard", () => {
  it("renders the function name + namespace", () => {
    render(<FunctionCard fn={mkFn()} />);
    expect(screen.getByText("summarizer")).toBeInTheDocument();
    expect(screen.getByText("ns-a")).toBeInTheDocument();
  });

  it("counts top-level schema properties on each side", () => {
    render(<FunctionCard fn={mkFn()} />);
    // 2 input properties, 1 output property
    expect(screen.getByText("2 fields")).toBeInTheDocument();
    expect(screen.getByText("1 field")).toBeInTheDocument();
  });

  it("renders the 24h cost section (same as agent cards)", () => {
    render(<FunctionCard fn={mkFn()} />);
    expect(screen.getByText("Cost (24h)")).toBeInTheDocument();
  });

  it("shows the resolved provider and model", () => {
    render(<FunctionCard fn={mkFn()} />);
    expect(screen.getByText("anthropic")).toBeInTheDocument();
    expect(screen.getByText(/opus-4-8/)).toBeInTheDocument();
  });

  it("renders 0 fields when schemas are missing or malformed", () => {
    const fn = mkFn({
      spec: {
        ...mkFn().spec,
        inputSchema: undefined,
        outputSchema: { type: "array" }, // no .properties
      },
    });
    render(<FunctionCard fn={fn} />);
    // Both should read "0 fields"
    const zeros = screen.getAllByText("0 fields");
    expect(zeros).toHaveLength(2);
  });

  it("links to the detail page with the namespace in the query string", () => {
    render(<FunctionCard fn={mkFn()} />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/functions/summarizer?namespace=ns-a");
  });

  it("defaults to 'default' namespace in the URL when metadata.namespace is unset", () => {
    const fn = mkFn();
    fn.metadata.namespace = undefined;
    render(<FunctionCard fn={fn} />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/functions/summarizer?namespace=default");
  });

  it("renders the MCP badge when spec.facade.mcp.enabled is true", () => {
    const fn = mkFn({
      spec: {
        ...mkFn().spec,
        facade: { type: "grpc" as never, mcp: { enabled: true } },
      },
    });
    render(<FunctionCard fn={fn} />);
    expect(screen.getByTestId("mcp-badge")).toBeInTheDocument();
    expect(screen.getByText("MCP")).toBeInTheDocument();
  });

  it("hides the MCP badge when MCP is not enabled", () => {
    render(<FunctionCard fn={mkFn()} />);
    expect(screen.queryByTestId("mcp-badge")).not.toBeInTheDocument();
  });

  it("hides the MCP badge when MCP block is present but disabled", () => {
    const fn = mkFn({
      spec: {
        ...mkFn().spec,
        facade: { type: "grpc" as never, mcp: { enabled: false } },
      },
    });
    render(<FunctionCard fn={fn} />);
    expect(screen.queryByTestId("mcp-badge")).not.toBeInTheDocument();
  });
});
