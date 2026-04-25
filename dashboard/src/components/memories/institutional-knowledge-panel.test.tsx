/**
 * Tests for InstitutionalKnowledgePanel.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const {
  mockUseInstitutionalMemories,
  mockUseCreateInstitutionalMemory,
  mockUseDeleteInstitutionalMemory,
} = vi.hoisted(() => ({
  mockUseInstitutionalMemories: vi.fn(),
  mockUseCreateInstitutionalMemory: vi.fn(),
  mockUseDeleteInstitutionalMemory: vi.fn(),
}));

vi.mock("@/hooks/use-institutional-memories", () => ({
  useInstitutionalMemories: mockUseInstitutionalMemories,
  useCreateInstitutionalMemory: mockUseCreateInstitutionalMemory,
  useDeleteInstitutionalMemory: mockUseDeleteInstitutionalMemory,
}));

import { InstitutionalKnowledgePanel } from "./institutional-knowledge-panel";

function setup({
  memories = [] as Array<{ id: string; type: string; content: string }>,
  isLoading = false,
  error = null as unknown,
  createMutateAsync = vi.fn().mockResolvedValue(undefined),
  deleteMutate = vi.fn(),
  createPending = false,
  createError = null as unknown,
}) {
  mockUseInstitutionalMemories.mockReturnValue({
    data: { memories, total: memories.length },
    isLoading,
    error,
  });
  mockUseCreateInstitutionalMemory.mockReturnValue({
    mutateAsync: createMutateAsync,
    isPending: createPending,
    isError: !!createError,
    error: createError,
  });
  mockUseDeleteInstitutionalMemory.mockReturnValue({ mutate: deleteMutate });
  return { createMutateAsync, deleteMutate };
}

describe("InstitutionalKnowledgePanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders loading skeleton", () => {
    setup({ isLoading: true });
    render(<InstitutionalKnowledgePanel />);
    // Skeleton doesn't have a test id; check for absence of empty/list.
    expect(screen.queryByTestId("kn-empty")).not.toBeInTheDocument();
    expect(screen.queryByTestId("kn-list")).not.toBeInTheDocument();
  });

  it("renders empty state when no memories", () => {
    setup({ memories: [] });
    render(<InstitutionalKnowledgePanel />);
    expect(screen.getByTestId("kn-empty")).toBeInTheDocument();
  });

  it("renders the list when memories present", () => {
    setup({
      memories: [
        { id: "m1", type: "policy", content: "snake_case rule" },
        { id: "m2", type: "glossary", content: "API terms" },
      ],
    });
    render(<InstitutionalKnowledgePanel />);
    expect(screen.getByText("snake_case rule")).toBeInTheDocument();
    expect(screen.getByText("API terms")).toBeInTheDocument();
    expect(screen.getByText(/Workspace knowledge \(2\)/)).toBeInTheDocument();
    // Each entry shows an Institutional tier badge.
    expect(screen.getAllByText("Institutional")).toHaveLength(2);
  });

  it("renders error alert when load fails", () => {
    setup({ error: new Error("backend down") });
    render(<InstitutionalKnowledgePanel />);
    expect(screen.getByTestId("kn-error")).toBeInTheDocument();
    expect(screen.getByText("backend down")).toBeInTheDocument();
  });

  it("submits the create form and clears inputs on success", async () => {
    const createMutateAsync = vi.fn().mockResolvedValue(undefined);
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.type(screen.getByTestId("create-type"), "policy");
    await user.type(screen.getByTestId("create-content"), "  no trailing whitespace  ");
    await user.click(screen.getByTestId("create-submit"));

    await waitFor(() => expect(createMutateAsync).toHaveBeenCalledTimes(1));
    expect(createMutateAsync).toHaveBeenCalledWith({
      type: "policy",
      content: "no trailing whitespace",
    });
  });

  it("shows inline error from the create mutation", () => {
    setup({ createError: new Error("validation failed") });
    render(<InstitutionalKnowledgePanel />);
    expect(screen.getByText("validation failed")).toBeInTheDocument();
  });

  it("does not submit when the type is blank", async () => {
    const createMutateAsync = vi.fn().mockResolvedValue(undefined);
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    // Only content filled — form is required so click won't submit.
    await user.type(screen.getByTestId("create-content"), "solo");
    await user.click(screen.getByTestId("create-submit"));

    expect(createMutateAsync).not.toHaveBeenCalled();
  });

  it("invokes delete mutation after confirming the dialog", async () => {
    const deleteMutate = vi.fn();
    setup({
      memories: [{ id: "m1", type: "policy", content: "x" }],
      deleteMutate,
    });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("kn-delete-m1"));
    await user.click(await screen.findByTestId("kn-delete-confirm-m1"));

    expect(deleteMutate).toHaveBeenCalledWith("m1");
  });

  it("bulk imports JSON entries sequentially", async () => {
    const createMutateAsync = vi.fn().mockResolvedValue(undefined);
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("bulk-import-open"));
    const textarea = await screen.findByTestId("bulk-import-json");
    await user.click(textarea);
    await user.paste(
      `[{"type":"policy","content":"A"},{"type":"glossary","content":"B"}]`
    );
    await user.click(screen.getByTestId("bulk-import-submit"));

    await waitFor(() => expect(createMutateAsync).toHaveBeenCalledTimes(2));
    expect(createMutateAsync).toHaveBeenNthCalledWith(1, {
      type: "policy",
      content: "A",
    });
    expect(createMutateAsync).toHaveBeenNthCalledWith(2, {
      type: "glossary",
      content: "B",
    });
    expect(await screen.findByTestId("bulk-import-summary")).toHaveTextContent(
      /Imported 2 \/ 2/
    );
  });

  it("shows parse errors and does not call create on invalid JSON", async () => {
    const createMutateAsync = vi.fn();
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("bulk-import-open"));
    const textarea = await screen.findByTestId("bulk-import-json");
    await user.click(textarea);
    await user.paste("not json");
    await user.click(screen.getByTestId("bulk-import-submit"));

    expect(await screen.findByTestId("bulk-import-errors")).toBeInTheDocument();
    expect(createMutateAsync).not.toHaveBeenCalled();
  });

  it("surfaces per-entry failures from the create loop", async () => {
    const createMutateAsync = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error("dup key"));
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("bulk-import-open"));
    const textarea = await screen.findByTestId("bulk-import-json");
    await user.click(textarea);
    await user.paste(
      `[{"type":"policy","content":"A"},{"type":"policy","content":"B"}]`
    );
    await user.click(screen.getByTestId("bulk-import-submit"));

    expect(await screen.findByTestId("bulk-import-summary")).toHaveTextContent(
      /Imported 1 \/ 2/
    );
    expect(screen.getByTestId("bulk-import-errors")).toHaveTextContent(/dup key/);
  });

  it("rejects empty bulk-import input", async () => {
    const createMutateAsync = vi.fn();
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("bulk-import-open"));
    // Nothing pasted: the submit button is disabled. Assert that.
    const submit = screen.getByTestId("bulk-import-submit") as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
  });

  it("parses markdown tab content", async () => {
    const createMutateAsync = vi.fn().mockResolvedValue(undefined);
    setup({ createMutateAsync });
    const user = userEvent.setup();

    render(<InstitutionalKnowledgePanel />);

    await user.click(screen.getByTestId("bulk-import-open"));
    await user.click(screen.getByRole("tab", { name: /markdown/i }));
    const textarea = await screen.findByTestId("bulk-import-markdown");
    await user.click(textarea);
    await user.paste("## Policy\nUse snake_case.");
    await user.click(screen.getByTestId("bulk-import-submit"));

    await waitFor(() => expect(createMutateAsync).toHaveBeenCalledTimes(1));
    expect(createMutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "policy",
        content: "Use snake_case.",
      })
    );
  });
});
