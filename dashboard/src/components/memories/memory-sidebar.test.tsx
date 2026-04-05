/**
 * Tests for MemorySidebar.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { MemoryEntity } from "@/lib/data/types";

const { mockUseAuth, mockUseMemories } = vi.hoisted(() => ({
  mockUseAuth: vi.fn(),
  mockUseMemories: vi.fn(),
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: mockUseAuth,
}));

vi.mock("@/hooks/use-memories", () => ({
  useMemories: mockUseMemories,
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: { name: "test-ws" }, workspaces: [], setCurrentWorkspace: vi.fn(), isLoading: false, error: null, refetch: vi.fn() }),
}));

// next/link works fine in jsdom but needs a router mock in some setups
vi.mock("next/link", () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}));

import { MemorySidebar } from "./memory-sidebar";

function makeMemory(id: string, content: string): MemoryEntity {
  return {
    id,
    type: "fact",
    content,
    confidence: 0.8,
    scope: { workspace: "ws-1" },
    createdAt: new Date().toISOString(),
  };
}

function setup(memories: MemoryEntity[] = [], isLoading = false) {
  mockUseAuth.mockReturnValue({
    user: { id: "user-1" },
    isAuthenticated: true,
  });
  mockUseMemories.mockReturnValue({
    data: { memories, total: memories.length },
    isLoading,
  });
}

function setupAnonymous() {
  mockUseAuth.mockReturnValue({
    user: { id: "anon", provider: "anonymous" },
    isAuthenticated: false,
  });
  mockUseMemories.mockReturnValue({
    data: { memories: [], total: 0 },
    isLoading: false,
  });
}

describe("MemorySidebar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders sidebar when open is true", () => {
    setup();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(screen.getByTestId("memory-sidebar")).toBeInTheDocument();
  });

  it("does not render sidebar content when open is false", () => {
    setup();
    render(<MemorySidebar agentName="my-agent" open={false} onClose={vi.fn()} />);
    expect(screen.queryByTestId("memory-sidebar")).not.toBeInTheDocument();
  });

  it("shows Agent Memories title", () => {
    setup();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(screen.getByText("Agent Memories")).toBeInTheDocument();
  });

  it("shows loading skeletons when isLoading is true", () => {
    setup([], true);
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    // Skeletons are rendered as divs; verify no memory cards or empty state
    expect(screen.queryByText("No memories yet")).not.toBeInTheDocument();
    expect(screen.queryByTestId("memory-card")).not.toBeInTheDocument();
  });

  it("shows empty state when no memories", () => {
    setup([]);
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(screen.getByText("No memories yet")).toBeInTheDocument();
  });

  it("shows memory cards when memories exist", () => {
    setup([
      makeMemory("m1", "User prefers dark mode"),
      makeMemory("m2", "User is based in NYC"),
    ]);
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(screen.getByText("User prefers dark mode")).toBeInTheDocument();
    expect(screen.getByText("User is based in NYC")).toBeInTheDocument();
  });

  it("shows View all memories link", () => {
    setup();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(screen.getByTestId("view-all-memories")).toBeInTheDocument();
    expect(screen.getByTestId("view-all-memories")).toHaveAttribute("href", "/memories");
  });

  it("shows anonymous notice for anonymous users", () => {
    setupAnonymous();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(
      screen.getByTestId("memory-sidebar-anonymous-notice")
    ).toBeInTheDocument();
    expect(screen.getByText(/Memories require sign-in/i)).toBeInTheDocument();
    expect(screen.queryByText("No memories yet")).not.toBeInTheDocument();
  });

  it("does not fetch memories for anonymous users", () => {
    setupAnonymous();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={vi.fn()} />);
    expect(mockUseMemories).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false })
    );
  });

  it("calls onClose when sheet is closed", async () => {
    const user = userEvent.setup();
    setup();
    const onClose = vi.fn();
    render(<MemorySidebar agentName="my-agent" open={true} onClose={onClose} />);

    // Click the close button rendered by the Sheet component
    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);
    expect(onClose).toHaveBeenCalled();
  });
});
