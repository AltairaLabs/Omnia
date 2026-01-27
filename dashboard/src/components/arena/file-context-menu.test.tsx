import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FileContextMenu } from "./file-context-menu";

describe("FileContextMenu", () => {
  const defaultProps = {
    isDirectory: false,
    isRoot: false,
    onNewFile: vi.fn(),
    onNewFolder: vi.fn(),
    onNewTypedFile: vi.fn(),
    onRename: vi.fn(),
    onDelete: vi.fn(),
    onCopyPath: vi.fn(),
    children: <div data-testid="trigger">Trigger</div>,
  };

  it("should render children", () => {
    render(<FileContextMenu {...defaultProps} />);

    expect(screen.getByTestId("trigger")).toBeInTheDocument();
  });

  it("should show context menu on right-click", async () => {
    render(<FileContextMenu {...defaultProps} />);

    const trigger = screen.getByTestId("trigger");
    fireEvent.contextMenu(trigger);

    // Context menu items should appear
    expect(await screen.findByText("Copy Path")).toBeInTheDocument();
  });

  it("should show New submenu for directories", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={true} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // The "New" submenu trigger should be visible
    expect(await screen.findByText("New")).toBeInTheDocument();
  });

  it("should not show New submenu for files", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={false} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Wait for menu to open
    expect(await screen.findByText("Copy Path")).toBeInTheDocument();

    // The "New" submenu should not be present for files
    expect(screen.queryByText("New")).not.toBeInTheDocument();
  });

  it("should not show Rename and Delete for root items", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={true} isRoot={true} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    expect(await screen.findByText("Copy Path")).toBeInTheDocument();
    expect(screen.queryByText("Rename")).not.toBeInTheDocument();
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  it("should show Rename and Delete for non-root items", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={false} isRoot={false} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    expect(await screen.findByText("Rename")).toBeInTheDocument();
    expect(screen.getByText("Delete")).toBeInTheDocument();
  });

  it("should call onNewFile when File... is clicked in submenu", async () => {
    const user = userEvent.setup();
    const onNewFile = vi.fn();
    render(<FileContextMenu {...defaultProps} isDirectory={true} onNewFile={onNewFile} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Hover over the "New" submenu to open it
    const newSubmenu = await screen.findByText("New");
    await user.hover(newSubmenu);

    // Wait for submenu to open and find "File..." option
    const fileItem = await screen.findByText("File...");
    fireEvent.click(fileItem);

    expect(onNewFile).toHaveBeenCalledTimes(1);
  });

  it("should call onNewFolder when Folder... is clicked in submenu", async () => {
    const user = userEvent.setup();
    const onNewFolder = vi.fn();
    render(<FileContextMenu {...defaultProps} isDirectory={true} onNewFolder={onNewFolder} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Hover over the "New" submenu to open it
    const newSubmenu = await screen.findByText("New");
    await user.hover(newSubmenu);

    // Wait for submenu to open and find "Folder..." option
    const folderItem = await screen.findByText("Folder...");
    fireEvent.click(folderItem);

    expect(onNewFolder).toHaveBeenCalledTimes(1);
  });

  it("should call onNewTypedFile with correct kind when typed file is clicked", async () => {
    const user = userEvent.setup();
    const onNewTypedFile = vi.fn();
    render(<FileContextMenu {...defaultProps} isDirectory={true} onNewTypedFile={onNewTypedFile} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Hover over the "New" submenu to open it
    const newSubmenu = await screen.findByText("New");
    await user.hover(newSubmenu);

    // Wait for submenu to open and click "Prompt" option
    const promptItem = await screen.findByText("Prompt");
    fireEvent.click(promptItem);

    expect(onNewTypedFile).toHaveBeenCalledWith("prompt");
  });

  it("should show all Arena file types in submenu", async () => {
    const user = userEvent.setup();
    render(<FileContextMenu {...defaultProps} isDirectory={true} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Hover over the "New" submenu to open it
    const newSubmenu = await screen.findByText("New");
    await user.hover(newSubmenu);

    // Wait for submenu to open and check all file types are present
    await waitFor(() => {
      expect(screen.getByText("Prompt")).toBeInTheDocument();
    });
    expect(screen.getByText("Provider")).toBeInTheDocument();
    expect(screen.getByText("Scenario")).toBeInTheDocument();
    expect(screen.getByText("Tool")).toBeInTheDocument();
    expect(screen.getByText("Persona")).toBeInTheDocument();
  });

  it("should call onRename when Rename is clicked", async () => {
    const onRename = vi.fn();
    render(<FileContextMenu {...defaultProps} onRename={onRename} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));
    const renameItem = await screen.findByText("Rename");
    fireEvent.click(renameItem);

    expect(onRename).toHaveBeenCalledTimes(1);
  });

  it("should call onDelete when Delete is clicked", async () => {
    const onDelete = vi.fn();
    render(<FileContextMenu {...defaultProps} onDelete={onDelete} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));
    const deleteItem = await screen.findByText("Delete");
    fireEvent.click(deleteItem);

    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("should call onCopyPath when Copy Path is clicked", async () => {
    const onCopyPath = vi.fn();
    render(<FileContextMenu {...defaultProps} onCopyPath={onCopyPath} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));
    const copyPathItem = await screen.findByText("Copy Path");
    fireEvent.click(copyPathItem);

    expect(onCopyPath).toHaveBeenCalledTimes(1);
  });
});
