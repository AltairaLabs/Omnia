/**
 * Tests for SignupForm — verifies brand-aware signup copy and that the
 * verification success chip uses the success design token, not a raw green.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SignupForm } from "./signup-form";

const TEST_PASSWORD = "password123";

const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, refresh: vi.fn() }),
}));

describe("SignupForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("renders brand-aware signup copy", () => {
    render(<SignupForm />);
    expect(
      screen.getByText("Sign up to get started with Omnia"),
    ).toBeInTheDocument();
  });

  it("shows a token-styled success state when verification is required", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ requiresVerification: true }),
    });

    const { container } = render(<SignupForm />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "jo" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "j@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: TEST_PASSWORD },
    });
    fireEvent.click(screen.getByRole("button", { name: /Create account/i }));

    await waitFor(() =>
      expect(screen.getByText("Check your email")).toBeInTheDocument(),
    );
    expect(container.innerHTML).toContain("text-success");
    expect(container.innerHTML).not.toMatch(/green-\d/);
  });
});
