/**
 * Tests for the owner-only bulk session purge dialog.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const { mockMutate, mockState } = vi.hoisted(() => ({
  mockMutate: vi.fn(),
  mockState: { isPending: false, isError: false, error: null as Error | null },
}));

vi.mock("@/hooks/use-session-mutations", () => ({
  usePurgeSessions: () => ({
    mutate: mockMutate,
    isPending: mockState.isPending,
    isError: mockState.isError,
    error: mockState.error,
  }),
}));

import { PurgeSessionsDialog } from "./purge-sessions-dialog";

/** Select an option from a Radix Select by clicking its trigger then the option. */
async function selectOption(trigger: HTMLElement, optionText: string) {
  fireEvent.click(trigger);
  const option = await screen.findByRole("option", { name: optionText });
  fireEvent.click(option);
}

function openDialog() {
  render(<PurgeSessionsDialog agentNames={["support-agent", "code-agent"]} />);
  fireEvent.click(screen.getByTestId("purge-sessions-open"));
}

describe("PurgeSessionsDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState.isPending = false;
    mockState.isError = false;
    mockState.error = null;
  });

  it("renders a trigger button", () => {
    render(<PurgeSessionsDialog agentNames={[]} />);
    expect(screen.getByTestId("purge-sessions-open")).toBeInTheDocument();
  });

  it("opens with the default scope summary (all agents, everything)", () => {
    openDialog();
    expect(screen.getByRole("heading", { name: "Purge sessions" })).toBeInTheDocument();
    // The summary line names the default scope in lowercase.
    expect(screen.getByText("all agents")).toBeInTheDocument();
    expect(screen.getByText("everything")).toBeInTheDocument();
  });

  it("purges with an undefined scope by default", () => {
    openDialog();
    fireEvent.click(screen.getByTestId("purge-confirm"));
    expect(mockMutate).toHaveBeenCalledWith(
      { agent: undefined, before: undefined },
      expect.any(Object)
    );
  });

  it("scopes the purge to the chosen agent and age cutoff", async () => {
    openDialog();
    await selectOption(screen.getByTestId("purge-agent"), "support-agent");
    await selectOption(screen.getByTestId("purge-age"), "Older than 7 days");

    fireEvent.click(screen.getByTestId("purge-confirm"));

    expect(mockMutate).toHaveBeenCalledWith(
      { agent: "support-agent", before: expect.any(String) },
      expect.any(Object)
    );
  });

  it("shows the deleted count after a successful purge", () => {
    mockMutate.mockImplementation((_scope, opts) => opts?.onSuccess?.(3));
    openDialog();
    fireEvent.click(screen.getByTestId("purge-confirm"));
    expect(screen.getByText("Deleted 3 sessions.")).toBeInTheDocument();
  });

  it("uses singular wording for a single deleted session", () => {
    mockMutate.mockImplementation((_scope, opts) => opts?.onSuccess?.(1));
    openDialog();
    fireEvent.click(screen.getByTestId("purge-confirm"));
    expect(screen.getByText("Deleted 1 session.")).toBeInTheDocument();
  });

  it("surfaces a purge error", () => {
    mockState.isError = true;
    mockState.error = new Error("backend exploded");
    openDialog();
    expect(screen.getByText("Purge failed")).toBeInTheDocument();
    expect(screen.getByText("backend exploded")).toBeInTheDocument();
  });

  it("disables the confirm button while purging", () => {
    mockState.isPending = true;
    openDialog();
    expect(screen.getByTestId("purge-confirm")).toBeDisabled();
    expect(screen.getByText("Purging…")).toBeInTheDocument();
  });
});
