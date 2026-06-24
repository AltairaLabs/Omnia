import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FunctionTable } from "./function-table";
import { useAgentCost } from "@/hooks/agents";
import { useWorkspace } from "@/contexts/workspace-context";
import type { AgentRuntime } from "@/types";

vi.mock("@/hooks/agents", () => ({ useAgentCost: vi.fn(() => ({ data: null })) }));
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: { name: "demo" } })),
}));
vi.mock("@/hooks/resources", () => ({
  useProvider: vi.fn(() => ({ data: { spec: { type: "anthropic", model: "claude-opus-4-8" } } })),
}));
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
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
      facade: { type: "rest" as never },
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

  it("queries cost by workspace name, not the function namespace (#1572)", () => {
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    // currentWorkspace.name = "demo" (mocked); must be the cost key, not "ns-a".
    expect(vi.mocked(useAgentCost)).toHaveBeenCalledWith("demo", "summarizer");
  });

  it("falls back to an empty cost key when no workspace is selected (#1572)", () => {
    vi.mocked(useWorkspace).mockReturnValueOnce({ currentWorkspace: null } as never);
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    // currentWorkspace null → `currentWorkspace?.name ?? ""` → "" (query disabled).
    expect(vi.mocked(useAgentCost)).toHaveBeenCalledWith("", "summarizer");
  });

  it("renders one row per function", () => {
    render(<FunctionTable functions={[mkFn("a", "ns"), mkFn("b", "ns")]} />);
    expect(screen.getByRole("link", { name: "a" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "b" })).toBeInTheDocument();
  });

  it("resolves the Provider column to the provider type (matching the card)", () => {
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    expect(screen.getByText("anthropic")).toBeInTheDocument();
  });

  it("navigates to the detail page when a row is clicked", () => {
    push.mockClear();
    render(<FunctionTable functions={[mkFn("summarizer", "ns-a")]} />);
    fireEvent.click(screen.getByTestId("function-row"));
    expect(push).toHaveBeenCalledWith("/functions/summarizer?namespace=ns-a");
  });
});
