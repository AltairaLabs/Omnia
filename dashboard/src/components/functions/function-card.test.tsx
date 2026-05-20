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
      invocationRecording: { state: "enabled" },
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

  it("shows Recording badge when invocationRecording.state is enabled", () => {
    render(<FunctionCard fn={mkFn()} />);
    expect(screen.getByText("Recording")).toBeInTheDocument();
  });

  it("shows Ephemeral badge when recording is disabled or unset", () => {
    render(
      <FunctionCard
        fn={mkFn({
          spec: {
            ...mkFn().spec,
            invocationRecording: { state: "disabled" },
          },
        })}
      />,
    );
    expect(screen.getByText("Ephemeral")).toBeInTheDocument();
  });

  it("counts top-level schema properties on each side", () => {
    render(<FunctionCard fn={mkFn()} />);
    // 2 input properties, 1 output property
    expect(screen.getByText("2 fields")).toBeInTheDocument();
    expect(screen.getByText("1 field")).toBeInTheDocument();
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
});
