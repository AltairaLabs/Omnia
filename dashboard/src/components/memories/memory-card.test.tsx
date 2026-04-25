/**
 * Tests for MemoryCard.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryCard } from "./memory-card";
import type { MemoryEntity } from "@/lib/data/types";

function makeMemory(overrides: Partial<MemoryEntity> = {}): MemoryEntity {
  return {
    id: "mem-1",
    type: "fact",
    content: "The user prefers dark mode.",
    confidence: 0.85,
    scope: { workspace: "ws-1" },
    createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(), // 2h ago
    ...overrides,
  };
}

describe("MemoryCard", () => {
  it("renders truncated content when content is long", () => {
    const longContent = "A".repeat(150);
    render(<MemoryCard memory={makeMemory({ content: longContent })} />);
    const truncated = screen.getByText(/A{100}\.\.\./);
    expect(truncated).toBeInTheDocument();
  });

  it("renders full content without truncation when short", () => {
    render(<MemoryCard memory={makeMemory()} />);
    expect(screen.getByText("The user prefers dark mode.")).toBeInTheDocument();
  });

  it("shows category badge", () => {
    render(
      <MemoryCard
        memory={makeMemory({ metadata: { consent_category: "memory:identity" } })}
      />
    );
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Identity");
  });

  it("shows Unknown badge when category metadata is absent", () => {
    render(<MemoryCard memory={makeMemory()} />);
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Unknown");
  });

  it("shows confidence bar with correct width", () => {
    render(<MemoryCard memory={makeMemory({ confidence: 0.75 })} />);
    const bar = screen.getByTestId("confidence-bar");
    expect(bar).toHaveStyle({ width: "75%" });
  });

  it("shows relative timestamp", () => {
    render(<MemoryCard memory={makeMemory()} />);
    expect(screen.getByText("2h ago")).toBeInTheDocument();
  });

  it("shows 'just now' for a very recent memory", () => {
    render(
      <MemoryCard
        memory={makeMemory({ createdAt: new Date().toISOString() })}
      />
    );
    expect(screen.getByText("just now")).toBeInTheDocument();
  });

  it("shows minutes ago for a memory created minutes ago", () => {
    const createdAt = new Date(Date.now() - 30 * 60 * 1000).toISOString();
    render(<MemoryCard memory={makeMemory({ createdAt })} />);
    expect(screen.getByText("30m ago")).toBeInTheDocument();
  });

  it("shows days ago for older memories", () => {
    const createdAt = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString();
    render(<MemoryCard memory={makeMemory({ createdAt })} />);
    expect(screen.getByText("3d ago")).toBeInTheDocument();
  });

  it("expands on click to show full content", () => {
    const longContent = "B".repeat(150);
    render(<MemoryCard memory={makeMemory({ content: longContent })} />);

    // Full content not visible in expanded section before click
    const trigger = screen.getByTestId("memory-card").querySelector("[data-state]");
    fireEvent.click(trigger!);

    // After expand, full content appears in the expanded area
    const fullContentEl = screen.getAllByText(longContent);
    expect(fullContentEl.length).toBeGreaterThan(0);
  });

  it("shows type and confidence in expanded view", () => {
    render(<MemoryCard memory={makeMemory({ type: "preference", confidence: 0.9 })} />);

    const trigger = screen.getByTestId("memory-card").querySelector("[data-state]");
    fireEvent.click(trigger!);

    expect(screen.getByText("preference")).toBeInTheDocument();
    expect(screen.getByText("90%")).toBeInTheDocument();
  });

  it("shows session ID in expanded view when present", () => {
    render(
      <MemoryCard memory={makeMemory({ sessionId: "sess-abc" })} />
    );

    const trigger = screen.getByTestId("memory-card").querySelector("[data-state]");
    fireEvent.click(trigger!);

    expect(screen.getByText("sess-abc")).toBeInTheDocument();
  });

  it("does not show session row when sessionId is absent", () => {
    render(<MemoryCard memory={makeMemory({ sessionId: undefined })} />);

    const trigger = screen.getByTestId("memory-card").querySelector("[data-state]");
    fireEvent.click(trigger!);

    expect(screen.queryByText("Session:")).not.toBeInTheDocument();
  });

  it("shows the tier badge when tier is set", () => {
    render(<MemoryCard memory={makeMemory({ tier: "user" })} />);
    expect(screen.getByText("User")).toBeInTheDocument();
  });

  it("does not show a tier badge when tier is absent (legacy mock)", () => {
    render(<MemoryCard memory={makeMemory({ tier: undefined })} />);
    expect(screen.queryByText("User")).not.toBeInTheDocument();
    expect(screen.queryByText("Agent")).not.toBeInTheDocument();
    expect(screen.queryByText("Institutional")).not.toBeInTheDocument();
  });
});
