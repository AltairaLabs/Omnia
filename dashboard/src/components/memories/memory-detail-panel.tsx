/**
 * MemoryDetailPanel — slide-out sheet showing full memory details.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Trash2, ExternalLink } from "lucide-react";
import { CategoryBadge } from "./category-badge";
import { TierBadge } from "./tier-badge";
import type { MemoryEntity } from "@/lib/data/types";

interface MemoryDetailPanelProps {
  memory: MemoryEntity | null;
  onClose: () => void;
  onDelete: (memoryId: string) => void;
}

export function MemoryDetailPanel({ memory, onClose, onDelete }: MemoryDetailPanelProps) {
  const category = memory?.metadata?.consent_category as string | undefined;
  const provenance = memory?.metadata?.provenance as string | undefined;

  return (
    <Sheet open={!!memory} onOpenChange={(open) => { if (!open) onClose(); }}>
      <SheetContent data-testid="memory-detail-panel" className="w-[400px] sm:w-[450px]">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            Memory Detail
            <TierBadge tier={memory?.tier} />
            <CategoryBadge category={category} />
          </SheetTitle>
          <SheetDescription>
            {memory?.type ?? "memory"} — {Math.round((memory?.confidence ?? 0) * 100)}% confidence
          </SheetDescription>
        </SheetHeader>

        {memory && (
          <div className="mt-4 space-y-4 px-4">
            {/* Content */}
            <div>
              <h4 className="text-sm font-medium mb-1">Content</h4>
              <p className="text-sm whitespace-pre-wrap bg-muted/50 rounded-md p-3">
                {memory.content}
              </p>
            </div>

            <Separator />

            {/* Metadata */}
            <div className="space-y-2">
              {provenance && (
                <DetailRow label="Provenance">
                  <Badge variant="secondary" className="text-xs">{provenance}</Badge>
                </DetailRow>
              )}
              <DetailRow label="Type">{memory.type}</DetailRow>
              <DetailRow label="Confidence">{Math.round(memory.confidence * 100)}%</DetailRow>
              <DetailRow label="Created">{formatDate(memory.createdAt)}</DetailRow>
              {memory.accessedAt && (
                <DetailRow label="Last Accessed">{formatDate(memory.accessedAt)}</DetailRow>
              )}
              {memory.expiresAt && (
                <DetailRow label="Expires">{formatDate(memory.expiresAt)}</DetailRow>
              )}
              {memory.sessionId && (
                <DetailRow label="Session">
                  <a
                    href={`/sessions/${memory.sessionId}`}
                    className="text-primary hover:underline inline-flex items-center gap-1"
                    data-testid="session-link"
                  >
                    {memory.sessionId.slice(0, 8)}...
                    <ExternalLink className="h-3 w-3" />
                  </a>
                </DetailRow>
              )}
            </div>

            {/* Custom metadata (excluding known keys) */}
            {hasCustomMetadata(memory.metadata) && (
              <>
                <Separator />
                <div>
                  <h4 className="text-sm font-medium mb-2">Metadata</h4>
                  <dl className="text-xs space-y-1">
                    {Object.entries(memory.metadata ?? {})
                      .filter(([key]) => !isKnownMetadataKey(key))
                      .map(([key, value]) => (
                        <div key={key} className="flex gap-2">
                          <dt className="font-medium text-muted-foreground min-w-[80px]">{key}</dt>
                          <dd>{String(value)}</dd>
                        </div>
                      ))}
                  </dl>
                </div>
              </>
            )}

            <Separator />

            {/* Delete action */}
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  variant="destructive"
                  size="sm"
                  className="w-full"
                  data-testid="delete-memory-button"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete this memory
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete memory?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will permanently remove this memory. The agent will no longer remember this information.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() => onDelete(memory.id)}
                    data-testid="confirm-delete-button"
                  >
                    Delete
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

// --- Helpers ---

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex justify-between items-center text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span>{children}</span>
    </div>
  );
}

const KNOWN_METADATA_KEYS = new Set(["consent_category", "provenance"]);

function isKnownMetadataKey(key: string): boolean {
  return KNOWN_METADATA_KEYS.has(key);
}

function hasCustomMetadata(metadata?: Record<string, unknown>): boolean {
  if (!metadata) return false;
  return Object.keys(metadata).some((key) => !isKnownMetadataKey(key));
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString();
}
