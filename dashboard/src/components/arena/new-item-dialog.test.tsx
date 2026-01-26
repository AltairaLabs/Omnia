import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NewItemDialog } from "./new-item-dialog";

describe("NewItemDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    mode: "file" as const,
    parentPath: null,
    onConfirm: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render dialog when open", () => {
    render(<NewItemDialog {...defaultProps} />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("should not render dialog when closed", () => {
    render(<NewItemDialog {...defaultProps} open={false} />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("should show correct title for file mode", () => {
    render(<NewItemDialog {...defaultProps} mode="file" />);

    expect(screen.getByText("New File")).toBeInTheDocument();
  });

  it("should show correct title for folder mode", () => {
    render(<NewItemDialog {...defaultProps} mode="folder" />);

    expect(screen.getByText("New Folder")).toBeInTheDocument();
  });

  it("should show parent path when provided", () => {
    render(<NewItemDialog {...defaultProps} parentPath="prompts" />);

    expect(screen.getByText(/in "prompts"/)).toBeInTheDocument();
  });

  it("should show root indicator when parentPath is null", () => {
    render(<NewItemDialog {...defaultProps} parentPath={null} />);

    expect(screen.getByText(/at the root level/i)).toBeInTheDocument();
  });

  it("should call onConfirm with name when form is submitted", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(<NewItemDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "new-file.yaml");

    const createButton = screen.getByRole("button", { name: /create/i });
    await user.click(createButton);

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith("new-file.yaml");
    });
  });

  it("should close dialog after successful submission", async () => {
    const onOpenChange = vi.fn();
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(
      <NewItemDialog
        {...defaultProps}
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />
    );

    const input = screen.getByRole("textbox");
    await user.type(input, "new-file.yaml");

    const createButton = screen.getByRole("button", { name: /create/i });
    await user.click(createButton);

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("should show error message on submission failure", async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error("Creation failed"));
    const user = userEvent.setup();

    render(<NewItemDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "new-file.yaml");

    const createButton = screen.getByRole("button", { name: /create/i });
    await user.click(createButton);

    await waitFor(() => {
      expect(screen.getByText(/creation failed/i)).toBeInTheDocument();
    });
  });

  it("should disable create button when name is empty", () => {
    render(<NewItemDialog {...defaultProps} />);

    const createButton = screen.getByRole("button", { name: /create/i });
    expect(createButton).toBeDisabled();
  });

  it("should enable create button when name is provided", async () => {
    const user = userEvent.setup();
    render(<NewItemDialog {...defaultProps} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "test");

    const createButton = screen.getByRole("button", { name: /create/i });
    expect(createButton).not.toBeDisabled();
  });

  it("should disable create button while submitting", async () => {
    // Create a promise that we can control
    let resolvePromise!: () => void;
    const onConfirm = vi.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolvePromise = resolve; })
    );
    const user = userEvent.setup();

    render(<NewItemDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "new-file.yaml");

    const createButton = screen.getByRole("button", { name: /create/i });
    await user.click(createButton);

    expect(createButton).toBeDisabled();

    // Clean up
    resolvePromise();
  });

  it("should clear input when dialog is closed", async () => {
    const onOpenChange = vi.fn();

    render(<NewItemDialog {...defaultProps} onOpenChange={onOpenChange} />);

    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "test" } });

    expect(input).toHaveValue("test");

    // Click cancel to close - this should trigger the clear
    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    fireEvent.click(cancelButton);

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should call onOpenChange when cancel button is clicked", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();

    render(<NewItemDialog {...defaultProps} onOpenChange={onOpenChange} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should validate file name for invalid characters", async () => {
    const user = userEvent.setup();
    render(<NewItemDialog {...defaultProps} />);

    const input = screen.getByRole("textbox");
    await user.type(input, "file/with/slashes");

    const createButton = screen.getByRole("button", { name: /create/i });
    await user.click(createButton);

    // Should show validation error
    await waitFor(() => {
      expect(screen.getByText(/cannot contain path separators/i)).toBeInTheDocument();
    });
  });
});
