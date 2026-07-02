/**
 * Tests for ResetPasswordForm — drives the success state and asserts the
 * confirmation chip uses the success design token, not a raw green shade.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ResetPasswordForm } from "./reset-password-form";

const TEST_PASSWORD = "password123";

describe("ResetPasswordForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("shows a token-styled success chip after a successful reset", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    });

    const { container } = render(<ResetPasswordForm token="tok-123" />);
    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: TEST_PASSWORD },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: TEST_PASSWORD },
    });
    fireEvent.click(screen.getByRole("button", { name: /Reset password/i }));

    await waitFor(() =>
      expect(screen.getByText("Password reset!")).toBeInTheDocument(),
    );
    expect(container.innerHTML).toContain("text-success");
    expect(container.innerHTML).not.toMatch(/green-\d/);
  });
});
