"use client";

import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { FilePlus, FolderPlus, Pencil, Trash2, Copy } from "lucide-react";
import type { ReactNode } from "react";

export interface FileContextMenuProps {
  children: ReactNode;
  isDirectory: boolean;
  isRoot?: boolean;
  onNewFile?: () => void;
  onNewFolder?: () => void;
  onRename?: () => void;
  onDelete?: () => void;
  onCopyPath?: () => void;
}

/**
 * Context menu for file tree items.
 * Shows different options based on whether the item is a file or directory.
 */
export function FileContextMenu({
  children,
  isDirectory,
  isRoot = false,
  onNewFile,
  onNewFolder,
  onRename,
  onDelete,
  onCopyPath,
}: FileContextMenuProps) {
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        {/* Create options (only for directories) */}
        {isDirectory && (
          <>
            <ContextMenuItem onClick={onNewFile} className="gap-2">
              <FilePlus className="h-4 w-4" />
              New File
            </ContextMenuItem>
            <ContextMenuItem onClick={onNewFolder} className="gap-2">
              <FolderPlus className="h-4 w-4" />
              New Folder
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        )}

        {/* Copy path */}
        <ContextMenuItem onClick={onCopyPath} className="gap-2">
          <Copy className="h-4 w-4" />
          Copy Path
        </ContextMenuItem>

        {/* Edit/Delete options (not for root or config.arena.yaml) */}
        {!isRoot && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onClick={onRename} className="gap-2">
              <Pencil className="h-4 w-4" />
              Rename
            </ContextMenuItem>
            <ContextMenuItem
              onClick={onDelete}
              className="gap-2 text-destructive focus:text-destructive"
            >
              <Trash2 className="h-4 w-4" />
              Delete
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}
