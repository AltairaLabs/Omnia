/**
 * Tests for ForgotPasswordForm — drives the success state and asserts the
 * confirmation chip uses the success design token, not a raw green shade.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ForgotPasswordForm } from "./forgot-password-form";

describe("ForgotPasswordForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("shows a token-styled success chip after submitting", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    });

    const { container } = render(<ForgotPasswordForm />);
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "j@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Send reset link/i }));

    await waitFor(() =>
      expect(screen.getByText("Check your email")).toBeInTheDocument(),
    );
    expect(container.innerHTML).toContain("text-success");
    expect(container.innerHTML).not.toMatch(/green-\d/);
  });
});
