import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FileContextMenu } from "./file-context-menu";

describe("FileContextMenu", () => {
  const defaultProps = {
    isDirectory: false,
    isRoot: false,
    onNewFile: vi.fn(),
    onNewFolder: vi.fn(),
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

  it("should show New File and New Folder options for directories", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={true} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    expect(await screen.findByText("New File")).toBeInTheDocument();
    expect(screen.getByText("New Folder")).toBeInTheDocument();
  });

  it("should not show New File and New Folder for files", async () => {
    render(<FileContextMenu {...defaultProps} isDirectory={false} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));

    // Wait for menu to open
    expect(await screen.findByText("Copy Path")).toBeInTheDocument();

    // These should not be present
    expect(screen.queryByText("New File")).not.toBeInTheDocument();
    expect(screen.queryByText("New Folder")).not.toBeInTheDocument();
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

  it("should call onNewFile when New File is clicked", async () => {
    const onNewFile = vi.fn();
    render(<FileContextMenu {...defaultProps} isDirectory={true} onNewFile={onNewFile} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));
    const newFileItem = await screen.findByText("New File");
    fireEvent.click(newFileItem);

    expect(onNewFile).toHaveBeenCalledTimes(1);
  });

  it("should call onNewFolder when New Folder is clicked", async () => {
    const onNewFolder = vi.fn();
    render(<FileContextMenu {...defaultProps} isDirectory={true} onNewFolder={onNewFolder} />);

    fireEvent.contextMenu(screen.getByTestId("trigger"));
    const newFolderItem = await screen.findByText("New Folder");
    fireEvent.click(newFolderItem);

    expect(onNewFolder).toHaveBeenCalledTimes(1);
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
