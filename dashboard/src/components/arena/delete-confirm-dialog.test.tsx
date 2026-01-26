import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeleteConfirmDialog } from "./delete-confirm-dialog";

describe("DeleteConfirmDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    itemName: "test-file.yaml",
    itemPath: "prompts/test-file.yaml",
    isDirectory: false,
    onConfirm: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render dialog when open", () => {
    render(<DeleteConfirmDialog {...defaultProps} />);

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("should not render dialog when closed", () => {
    render(<DeleteConfirmDialog {...defaultProps} open={false} />);

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("should show correct title for file deletion", () => {
    render(<DeleteConfirmDialog {...defaultProps} isDirectory={false} />);

    expect(screen.getByText("Delete file?")).toBeInTheDocument();
  });

  it("should show correct title for folder deletion", () => {
    render(<DeleteConfirmDialog {...defaultProps} isDirectory={true} />);

    expect(screen.getByText("Delete folder?")).toBeInTheDocument();
  });

  it("should display item name in the dialog", () => {
    render(<DeleteConfirmDialog {...defaultProps} itemName="my-file.yaml" />);

    expect(screen.getByText(/my-file\.yaml/)).toBeInTheDocument();
  });

  it("should display item path in the dialog", () => {
    render(<DeleteConfirmDialog {...defaultProps} itemPath="prompts/my-file.yaml" />);

    expect(screen.getByText(/prompts\/my-file\.yaml/)).toBeInTheDocument();
  });

  it("should warn about recursive deletion for directories", () => {
    render(<DeleteConfirmDialog {...defaultProps} isDirectory={true} />);

    expect(screen.getByText(/all its contents/i)).toBeInTheDocument();
  });

  it("should call onConfirm when delete button is clicked", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(<DeleteConfirmDialog {...defaultProps} onConfirm={onConfirm} />);

    const deleteButton = screen.getByRole("button", { name: /delete/i });
    await user.click(deleteButton);

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledTimes(1);
    });
  });

  it("should close dialog after successful deletion", async () => {
    const onOpenChange = vi.fn();
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(
      <DeleteConfirmDialog
        {...defaultProps}
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />
    );

    const deleteButton = screen.getByRole("button", { name: /delete/i });
    await user.click(deleteButton);

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("should handle deletion failure gracefully", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const onConfirm = vi.fn().mockRejectedValue(new Error("Deletion failed"));
    const user = userEvent.setup();

    render(<DeleteConfirmDialog {...defaultProps} onConfirm={onConfirm} />);

    const deleteButton = screen.getByRole("button", { name: /delete/i });
    await user.click(deleteButton);

    await waitFor(() => {
      expect(consoleSpy).toHaveBeenCalled();
    });

    consoleSpy.mockRestore();
  });

  it("should disable delete button while deleting", async () => {
    let resolvePromise!: () => void;
    const onConfirm = vi.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolvePromise = resolve; })
    );
    const user = userEvent.setup();

    render(<DeleteConfirmDialog {...defaultProps} onConfirm={onConfirm} />);

    const deleteButton = screen.getByRole("button", { name: /delete/i });
    await user.click(deleteButton);

    expect(deleteButton).toBeDisabled();

    // Clean up
    resolvePromise();
  });

  it("should call onOpenChange when cancel button is clicked", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();

    render(<DeleteConfirmDialog {...defaultProps} onOpenChange={onOpenChange} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should have destructive styling on delete button", () => {
    render(<DeleteConfirmDialog {...defaultProps} />);

    const deleteButton = screen.getByRole("button", { name: /delete/i });
    // Check for destructive variant class (bg-destructive)
    expect(deleteButton).toHaveClass("bg-destructive");
  });
});
