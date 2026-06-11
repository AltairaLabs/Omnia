import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const { mockMutate, mockToast } = vi.hoisted(() => ({
  mockMutate: vi.fn(),
  mockToast: vi.fn(),
}));

vi.mock("@/hooks/use-embedding-dimension", () => ({
  useChangeEmbeddingDimension: () => ({ mutate: mockMutate, isPending: false }),
}));
vi.mock("@/hooks/core", () => ({ useToast: () => ({ toast: mockToast }) }));

import { AdminTab } from "./admin-tab";

function dimensionInput() {
  return screen.getByLabelText(/new dimension/i);
}
function triggerButton() {
  return screen.getByRole("button", { name: /record dimension change/i });
}

describe("AdminTab", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockToast.mockReset();
  });

  it("disables the trigger until a valid dimension is entered", () => {
    render(<AdminTab workspaceName="ws-1" />);
    expect(triggerButton()).toBeDisabled();

    fireEvent.change(dimensionInput(), { target: { value: "768" } });
    expect(triggerButton()).toBeEnabled();
  });

  it("keeps the trigger disabled for an out-of-range dimension", () => {
    render(<AdminTab workspaceName="ws-1" />);
    fireEvent.change(dimensionInput(), { target: { value: "5000" } });
    expect(triggerButton()).toBeDisabled();
  });

  it("records consent and toasts success on confirm", async () => {
    mockMutate.mockImplementation((_dim: number, opts: { onSuccess?: () => void }) => {
      opts.onSuccess?.();
    });
    render(<AdminTab workspaceName="ws-1" />);
    fireEvent.change(dimensionInput(), { target: { value: "768" } });
    fireEvent.click(triggerButton());

    const confirm = await screen.findByRole("button", { name: /record consent/i });
    fireEvent.click(confirm);

    expect(mockMutate).toHaveBeenCalledWith(768, expect.any(Object));
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Consent recorded" })
    );
  });

  it("toasts a destructive error when the mutation fails", async () => {
    mockMutate.mockImplementation(
      (_dim: number, opts: { onError?: (e: Error) => void }) => {
        opts.onError?.(new Error("backend down"));
      }
    );
    render(<AdminTab workspaceName="ws-1" />);
    fireEvent.change(dimensionInput(), { target: { value: "768" } });
    fireEvent.click(triggerButton());

    const confirm = await screen.findByRole("button", { name: /record consent/i });
    fireEvent.click(confirm);

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "destructive", description: "backend down" })
    );
  });
});
