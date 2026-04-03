/**
 * Tests for MemoryDetailPanel.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryDetailPanel } from "./memory-detail-panel";
import type { MemoryEntity } from "@/lib/data/types";

function makeMemory(overrides: Partial<MemoryEntity> = {}): MemoryEntity {
  return {
    id: "mem-abc-123",
    type: "fact",
    content: "The user prefers dark mode.",
    confidence: 0.85,
    scope: { workspace: "ws-1" },
    createdAt: "2026-01-15T10:00:00Z",
    ...overrides,
  };
}

describe("MemoryDetailPanel", () => {
  const onClose = vi.fn();
  const onDelete = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders panel when memory is non-null", () => {
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.getByTestId("memory-detail-panel")).toBeInTheDocument();
  });

  it("does not render panel content when memory is null", () => {
    render(
      <MemoryDetailPanel memory={null} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.queryByTestId("memory-detail-panel")).not.toBeInTheDocument();
  });

  it("shows memory content", () => {
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.getByText("The user prefers dark mode.")).toBeInTheDocument();
  });

  it("shows category badge", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ metadata: { consent_category: "memory:identity" } })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Identity");
  });

  it("shows Unknown badge when no category metadata", () => {
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Unknown");
  });

  it("shows confidence percentage in description", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ confidence: 0.75 })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByText(/75% confidence/)).toBeInTheDocument();
  });

  it("shows provenance badge when present", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ metadata: { provenance: "user-input" } })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByText("user-input")).toBeInTheDocument();
  });

  it("hides provenance row when absent", () => {
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.queryByText("Provenance")).not.toBeInTheDocument();
  });

  it("shows session link when sessionId is present", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ sessionId: "sess-abc-999" })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    const link = screen.getByTestId("session-link");
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", "/sessions/sess-abc-999");
  });

  it("hides session link when sessionId is absent", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ sessionId: undefined })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.queryByTestId("session-link")).not.toBeInTheDocument();
  });

  it("shows custom metadata excluding consent_category and provenance", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({
          metadata: {
            consent_category: "memory:identity",
            provenance: "user-input",
            source: "chat-session",
            priority: "high",
          },
        })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByText("source")).toBeInTheDocument();
    expect(screen.getByText("chat-session")).toBeInTheDocument();
    expect(screen.getByText("priority")).toBeInTheDocument();
    expect(screen.getByText("high")).toBeInTheDocument();
    // Known keys should not appear in the custom metadata section as dt elements
    const dtElements = screen.queryAllByText("consent_category");
    expect(dtElements).toHaveLength(0);
    const provenanceDt = screen.queryAllByText("provenance");
    expect(provenanceDt).toHaveLength(0);
  });

  it("does not show custom metadata section when only known keys are present", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({
          metadata: {
            consent_category: "memory:identity",
            provenance: "user-input",
          },
        })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.queryByText("Metadata")).not.toBeInTheDocument();
  });

  it("shows delete button", () => {
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    expect(screen.getByTestId("delete-memory-button")).toBeInTheDocument();
  });

  it("shows confirmation dialog when delete button is clicked", async () => {
    const user = userEvent.setup();
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    await user.click(screen.getByTestId("delete-memory-button"));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText("Delete memory?")).toBeInTheDocument();
  });

  it("calls onDelete with memory id when confirm delete is clicked", async () => {
    const user = userEvent.setup();
    render(
      <MemoryDetailPanel
        memory={makeMemory({ id: "mem-abc-123" })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    await user.click(screen.getByTestId("delete-memory-button"));
    await user.click(screen.getByTestId("confirm-delete-button"));
    expect(onDelete).toHaveBeenCalledWith("mem-abc-123");
  });

  it("does not call onDelete when cancel is clicked in confirmation dialog", async () => {
    const user = userEvent.setup();
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    await user.click(screen.getByTestId("delete-memory-button"));
    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onDelete).not.toHaveBeenCalled();
  });

  it("calls onClose when sheet is closed via Escape key", async () => {
    const user = userEvent.setup();
    render(
      <MemoryDetailPanel memory={makeMemory()} onClose={onClose} onDelete={onDelete} />
    );
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows accessedAt when present", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ accessedAt: "2026-01-20T12:00:00Z" })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByText("Last Accessed")).toBeInTheDocument();
  });

  it("hides accessedAt row when absent", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ accessedAt: undefined })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.queryByText("Last Accessed")).not.toBeInTheDocument();
  });

  it("shows expiresAt when present", () => {
    render(
      <MemoryDetailPanel
        memory={makeMemory({ expiresAt: "2026-06-01T00:00:00Z" })}
        onClose={onClose}
        onDelete={onDelete}
      />
    );
    expect(screen.getByText("Expires")).toBeInTheDocument();
  });
});
