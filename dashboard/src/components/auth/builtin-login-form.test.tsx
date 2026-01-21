/**
 * Tests for BuiltinLoginForm component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BuiltinLoginForm } from "./builtin-login-form";

// Test credentials (not real passwords - used for testing only)
const TEST_USER = "testuser";
// eslint-disable-next-line sonarjs/no-hardcoded-passwords -- test-only credential
const TEST_PASSWORD = "testSecret123";
// eslint-disable-next-line sonarjs/no-hardcoded-passwords -- test-only credential
const WRONG_PASSWORD = "wrongInput123";

// Mock next/navigation
const mockPush = vi.fn();
const mockRefresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    refresh: mockRefresh,
  }),
}));

describe("BuiltinLoginForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("should render login form", () => {
    render(<BuiltinLoginForm />);

    expect(screen.getByText("Sign in to Omnia")).toBeInTheDocument();
    expect(screen.getByLabelText("Username or Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sign in/i })).toBeInTheDocument();
  });

  it("should show signup link by default", () => {
    render(<BuiltinLoginForm />);

    expect(screen.getByText(/Don't have an account\?/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Sign up/i })).toBeInTheDocument();
  });

  it("should hide signup link when allowSignup is false", () => {
    render(<BuiltinLoginForm allowSignup={false} />);

    expect(screen.queryByText(/Don't have an account\?/)).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Sign up/i })).not.toBeInTheDocument();
  });

  it("should show initial error message", () => {
    render(<BuiltinLoginForm error="auth_error" errorMessage="Invalid credentials" />);

    expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
  });

  it("should show forgot password link", () => {
    render(<BuiltinLoginForm />);

    expect(screen.getByRole("link", { name: /Forgot password\?/i })).toBeInTheDocument();
  });

  it("should update identity input on change", () => {
    render(<BuiltinLoginForm />);

    const identityInput = screen.getByLabelText("Username or Email");
    fireEvent.change(identityInput, { target: { value: TEST_USER } });

    expect(identityInput).toHaveValue(TEST_USER);
  });

  it("should update password input on change", () => {
    render(<BuiltinLoginForm />);

    const passwordInput = screen.getByLabelText("Password");
    fireEvent.change(passwordInput, { target: { value: TEST_PASSWORD } });

    expect(passwordInput).toHaveValue(TEST_PASSWORD);
  });

  it("should submit form and redirect on success", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ success: true }),
    });

    render(<BuiltinLoginForm />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith("/api/auth/builtin/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ identity: TEST_USER, password: TEST_PASSWORD }),
      });
    });

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/");
      expect(mockRefresh).toHaveBeenCalled();
    });
  });

  it("should redirect to returnTo URL on success", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ success: true }),
    });

    render(<BuiltinLoginForm returnTo="/dashboard" />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("should show error message on login failure", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: "Invalid username or password" }),
    });

    render(<BuiltinLoginForm />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: WRONG_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(screen.getByText("Invalid username or password")).toBeInTheDocument();
    });

    expect(mockPush).not.toHaveBeenCalled();
  });

  it("should show generic error on fetch failure", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Network error"));

    render(<BuiltinLoginForm />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(screen.getByText("An error occurred. Please try again.")).toBeInTheDocument();
    });
  });

  it("should show loading state during submission", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      }), 100))
    );

    render(<BuiltinLoginForm />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    expect(screen.getByText("Signing in...")).toBeInTheDocument();
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("should disable inputs during submission", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      }), 100))
    );

    render(<BuiltinLoginForm />);

    fireEvent.change(screen.getByLabelText("Username or Email"), {
      target: { value: TEST_USER },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: TEST_PASSWORD },
    });

    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    expect(screen.getByLabelText("Username or Email")).toBeDisabled();
    expect(screen.getByLabelText("Password")).toBeDisabled();
  });
});
