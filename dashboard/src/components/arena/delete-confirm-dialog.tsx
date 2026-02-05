"use client";

import { useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Loader2 } from "lucide-react";

export interface DeleteConfirmDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly itemName: string;
  readonly itemPath: string;
  readonly isDirectory: boolean;
  readonly onConfirm: () => Promise<void>;
}

/**
 * Dialog for confirming file or folder deletion.
 */
export function DeleteConfirmDialog({
  open,
  onOpenChange,
  itemName,
  itemPath,
  isDirectory,
  onConfirm,
}: DeleteConfirmDialogProps) {
  const [loading, setLoading] = useState(false);

  const itemType = isDirectory ? "folder" : "file";
  const warningMessage = isDirectory
    ? "This will permanently delete this folder and all its contents."
    : "This will permanently delete this file.";

  const handleConfirm = async () => {
    setLoading(true);
    try {
      await onConfirm();
      onOpenChange(false);
    } catch (err) {
      // Error handling is done by the parent component
      console.error("Failed to delete:", err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {itemType}?</AlertDialogTitle>
          <AlertDialogDescription>
            <span className="block mb-2">
              Are you sure you want to delete <strong>{itemName}</strong>?
            </span>
            <span className="block text-muted-foreground text-xs mb-2">
              Path: {itemPath}
            </span>
            <span className="block text-destructive">
              {warningMessage}
            </span>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            disabled={loading}
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
