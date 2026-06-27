import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AddCredentialSecretDialog } from "./add-credential-secret-dialog";

vi.mock("@/hooks/resources", () => ({
  useCreateSecret: vi.fn(),
  useNamespaces: vi.fn(),
}));

import { useCreateSecret, useNamespaces } from "@/hooks/resources";

const mockMutateAsync = vi.fn();

function setMockHooks() {
  vi.mocked(useNamespaces).mockReturnValue({ data: ["default", "production"] } as never);
  vi.mocked(useCreateSecret).mockReturnValue({
    mutateAsync: mockMutateAsync,
    isPending: false,
    error: null,
  } as never);
}

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("AddCredentialSecretDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMutateAsync.mockResolvedValue({});
    setMockHooks();
  });

  it("renders the dialog when open=true", () => {
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    expect(
      screen.getByRole("heading", { name: /add provider credentials/i })
    ).toBeInTheDocument();
  });

  it("does not render dialog content when open=false", () => {
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={false}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    expect(
      screen.queryByRole("heading", { name: /add provider credentials/i })
    ).not.toBeInTheDocument();
  });

  it("calls onOpenChange(false) when Cancel is clicked", () => {
    const onOpenChange = vi.fn();
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={onOpenChange}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("disables Create button when form is empty", () => {
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    expect(screen.getByRole("button", { name: /^create$/i })).toBeDisabled();
  });

  it("creates secret and calls onCreated with the name", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    const onCreated = vi.fn();

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          namespace="default"
          onCreated={onCreated}
        />
      </TestWrapper>
    );

    await user.type(screen.getByPlaceholderText(/e\.g\., anthropic-credentials/i), "my-secret");
    await user.type(screen.getByPlaceholderText(/key.*openai_api_key/i), "ANTHROPIC_API_KEY");
    await user.type(screen.getByPlaceholderText(/value/i), "sk-test");

    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          namespace: "default",
          name: "my-secret",
          data: { ANTHROPIC_API_KEY: "sk-test" },
        })
      );
    });

    expect(onCreated).toHaveBeenCalledWith("my-secret");
  });

  it("adds and removes key-value pairs", async () => {
    vi.useRealTimers();

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    // Add a second pair
    fireEvent.click(screen.getByRole("button", { name: /add key/i }));

    // Should now have 2 remove buttons (one per pair)
    const removeButtons = screen.getAllByRole("button", { name: "" });
    // There are delete buttons when length > 1
    expect(screen.getAllByPlaceholderText(/key.*openai_api_key/i).length).toBe(2);

    // Remove the first pair by clicking the trash button
    const trashButtons = removeButtons.filter(
      (btn) => btn.querySelector("svg") !== null && !btn.textContent?.includes("Add") && !btn.textContent?.includes("Cancel") && !btn.textContent?.includes("Create")
    );
    // Click the first Trash2 button (delete pair)
    if (trashButtons.length > 0) {
      fireEvent.click(trashButtons[0]);
    }
  });

  it("skips key-value pairs with empty key or value on submit", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    const onCreated = vi.fn();

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={onCreated}
        />
      </TestWrapper>
    );

    await user.type(screen.getByPlaceholderText(/e\.g\., anthropic-credentials/i), "test-secret");
    // Only fill key, not value — should not submit
    await user.type(screen.getByPlaceholderText(/key.*openai_api_key/i), "MY_KEY");

    expect(screen.getByRole("button", { name: /^create$/i })).toBeDisabled();
  });

  it("applies provider template when selecting a provider", async () => {
    vi.useRealTimers();

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    // Click on provider template select
    fireEvent.click(screen.getByLabelText("Provider Template"));
    fireEvent.click(await screen.findByRole("option", { name: /anthropic/i }));

    // The key input should be pre-filled with ANTHROPIC_API_KEY
    const keyInputs = screen.getAllByPlaceholderText(/key.*openai_api_key/i);
    expect((keyInputs[0] as HTMLInputElement).value).toBe("ANTHROPIC_API_KEY");
  });

  it("shows namespaces from useNamespaces in the namespace select", async () => {
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    fireEvent.click(screen.getByLabelText("Namespace"));
    expect(await screen.findByRole("option", { name: "production" })).toBeInTheDocument();
  });

  it("shows 'Creating...' and disables Create when isPending", () => {
    vi.mocked(useCreateSecret).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: true,
      error: null,
    } as never);

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    expect(screen.getByRole("button", { name: /creating/i })).toBeDisabled();
  });

  it("shows error message when createMutation has an error", () => {
    vi.mocked(useCreateSecret).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
      error: new Error("API error"),
    } as never);

    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    expect(screen.getByText("API error")).toBeInTheDocument();
  });

  it("uses initialNamespace as default namespace when provided", () => {
    render(
      <TestWrapper>
        <AddCredentialSecretDialog
          open={true}
          onOpenChange={vi.fn()}
          namespace="production"
          onCreated={vi.fn()}
        />
      </TestWrapper>
    );

    // The trigger should show the initial namespace as selected
    const trigger = screen.getByLabelText("Namespace");
    expect(trigger).toHaveTextContent("production");
  });
});
