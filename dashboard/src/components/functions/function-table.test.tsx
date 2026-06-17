import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { FunctionTable } from "./function-table";
import type { AgentRuntime } from "@/types";

vi.mock("@/hooks/agents", () => ({ useAgentCost: vi.fn(() => ({ data: null })) }));
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

function mkFn(name: string, ns: string): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, namespace: ns, uid: `uid-${name}` },
    spec: {
      mode: "function",
      promptPackRef: { name: "pack" },
      facade: { type: "grpc" as never },
      inputSchema: { type: "object", properties: { q: { type: "string" }, k: { type: "integer" } } },
      outputSchema: { type: "object", properties: { a: { type: "string" } } },
    },
  };
}

describe("FunctionTable", () => {
  it("renders a row per function with a name link to the detail page", () => {
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    const link = screen.getByRole("link", { name: "summarizer" });
    expect(link).toHaveAttribute("href", "/functions/summarizer?namespace=ns-a");
  });

  it("shows the input/output field counts", () => {
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    expect(screen.getByText("2 fields")).toBeInTheDocument();
    expect(screen.getByText("1 field")).toBeInTheDocument();
  });

  it("renders one row per function", () => {
    render(<FunctionTable functions={[mkFn("a", "ns"), mkFn("b", "ns")]} />);
    expect(screen.getByRole("link", { name: "a" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "b" })).toBeInTheDocument();
  });
});
