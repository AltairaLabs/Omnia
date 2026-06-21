import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RenameDialog } from "./rename-dialog";

describe("RenameDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    currentName: "old.yaml",
    onConfirm: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the dialog pre-filled with the current name", () => {
    render(<RenameDialog {...defaultProps} />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toHaveValue("old.yaml");
    expect(screen.getByText(/rename file/i)).toBeInTheDocument();
  });

  it("does not render when closed", () => {
    render(<RenameDialog {...defaultProps} open={false} />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows the folder wording when renaming a directory", () => {
    render(<RenameDialog {...defaultProps} isDirectory currentName="prompts" />);

    expect(screen.getByText(/rename folder/i)).toBeInTheDocument();
  });

  it("calls onConfirm with the new name", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(<RenameDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "renamed.yaml");
    await user.click(screen.getByRole("button", { name: /^rename$/i }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith("renamed.yaml");
    });
  });

  it("closes without calling onConfirm when the name is unchanged", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const onOpenChange = vi.fn();
    const user = userEvent.setup();

    render(
      <RenameDialog {...defaultProps} onConfirm={onConfirm} onOpenChange={onOpenChange} />
    );

    await user.click(screen.getByRole("button", { name: /^rename$/i }));

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("rejects names containing path separators", async () => {
    const onConfirm = vi.fn();
    const user = userEvent.setup();

    render(<RenameDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "sub/name.yaml");
    await user.click(screen.getByRole("button", { name: /^rename$/i }));

    await waitFor(() => {
      expect(screen.getByText(/cannot contain path separators/i)).toBeInTheDocument();
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("surfaces an error returned by onConfirm", async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error("destination exists"));
    const user = userEvent.setup();

    render(<RenameDialog {...defaultProps} onConfirm={onConfirm} />);

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "taken.yaml");
    await user.click(screen.getByRole("button", { name: /^rename$/i }));

    await waitFor(() => {
      expect(screen.getByText(/destination exists/i)).toBeInTheDocument();
    });
  });

  it("calls onOpenChange when cancel is clicked", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();

    render(<RenameDialog {...defaultProps} onOpenChange={onOpenChange} />);

    await user.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
