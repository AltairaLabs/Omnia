"use client";

import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  FilePlus,
  FolderPlus,
  Pencil,
  Trash2,
  Copy,
  Plus,
  FileCode,
  Server,
  Play,
  Wrench,
  User,
  Download,
} from "lucide-react";
import type { ReactNode } from "react";
import { ARENA_FILE_TYPES, type ArenaFileKind } from "@/lib/arena/file-templates";

/**
 * Icons for each Arena file type
 */
const FILE_TYPE_ICONS: Record<ArenaFileKind, typeof FileCode> = {
  prompt: FileCode,
  provider: Server,
  scenario: Play,
  tool: Wrench,
  persona: User,
};

export interface FileContextMenuProps {
  children: ReactNode;
  isDirectory: boolean;
  isRoot?: boolean;
  onNewFile?: () => void;
  onNewFolder?: () => void;
  onNewTypedFile?: (kind: ArenaFileKind) => void;
  onImportProvider?: () => void;
  onImportTool?: () => void;
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
  onNewTypedFile,
  onImportProvider,
  onImportTool,
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
            <ContextMenuSub>
              <ContextMenuSubTrigger className="gap-2">
                <Plus className="h-4 w-4" />
                New
              </ContextMenuSubTrigger>
              <ContextMenuSubContent className="w-44">
                {/* Arena file types */}
                {ARENA_FILE_TYPES.map((fileType) => {
                  const Icon = FILE_TYPE_ICONS[fileType.kind];
                  return (
                    <ContextMenuItem
                      key={fileType.kind}
                      onClick={() => onNewTypedFile?.(fileType.kind)}
                      className="gap-2"
                    >
                      <Icon className="h-4 w-4" />
                      {fileType.label}
                    </ContextMenuItem>
                  );
                })}

                <ContextMenuSeparator />

                {/* Generic file/folder options */}
                <ContextMenuItem onClick={onNewFile} className="gap-2">
                  <FilePlus className="h-4 w-4" />
                  File...
                </ContextMenuItem>
                <ContextMenuItem onClick={onNewFolder} className="gap-2">
                  <FolderPlus className="h-4 w-4" />
                  Folder...
                </ContextMenuItem>
              </ContextMenuSubContent>
            </ContextMenuSub>

            {/* Import submenu */}
            <ContextMenuSub>
              <ContextMenuSubTrigger className="gap-2">
                <Download className="h-4 w-4" />
                Import
              </ContextMenuSubTrigger>
              <ContextMenuSubContent className="w-44">
                <ContextMenuItem onClick={onImportProvider} className="gap-2">
                  <Server className="h-4 w-4" />
                  Provider...
                </ContextMenuItem>
                <ContextMenuItem onClick={onImportTool} className="gap-2">
                  <Wrench className="h-4 w-4" />
                  Tool...
                </ContextMenuItem>
              </ContextMenuSubContent>
            </ContextMenuSub>

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
