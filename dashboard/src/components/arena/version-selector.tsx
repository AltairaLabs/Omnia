"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { AlertCircle, GitBranch, Clock, FileText, HardDrive, Loader2 } from "lucide-react";
import { useArenaSourceVersions, useArenaSourceVersionMutations } from "@/hooks/use-arena-source-versions";
import { formatBytes, formatDate } from "./source-utils";
import type { ArenaVersion } from "@/types/arena";

interface VersionSelectorProps {
  /** Name of the ArenaSource */
  sourceName: string | undefined;
  /** Whether the selector is disabled (e.g., for viewers) */
  disabled?: boolean;
  /** Callback when version is changed */
  onVersionChange?: (version: string) => void;
}

/**
 * Truncate a version hash for display.
 */
function truncateHash(hash: string, length = 12): string {
  return hash.length > length ? hash.slice(0, length) : hash;
}

/**
 * Format version label for display.
 */
function formatVersionLabel(version: ArenaVersion): string {
  return truncateHash(version.hash);
}

/**
 * Dropdown selector for switching between ArenaSource versions.
 * Shows available versions with metadata tooltips and "latest" badge.
 */
export function VersionSelector({
  sourceName,
  disabled = false,
  onVersionChange,
}: Readonly<VersionSelectorProps>) {
  const queryClient = useQueryClient();
  const { versions, headVersion, loading, error, refetch } = useArenaSourceVersions(sourceName);
  const { switchVersion, switching } = useArenaSourceVersionMutations(sourceName, () => {
    // Refetch versions after successful switch
    refetch();
  });

  const [switchError, setSwitchError] = useState<string | null>(null);

  const handleVersionChange = async (versionHash: string) => {
    if (!versionHash || versionHash === headVersion) return;

    setSwitchError(null);

    try {
      await switchVersion(versionHash);

      // Invalidate content queries to force refetch with new version
      // This clears the cache so the next fetch gets the new version's content
      await queryClient.invalidateQueries({
        predicate: (query) => {
          const key = query.queryKey;
          // Invalidate arena-config-content and arena-config-file queries
          return (
            key[0] === "arena-config-content" ||
            key[0] === "arena-config-file"
          );
        },
      });

      onVersionChange?.(versionHash);
    } catch (err) {
      setSwitchError(err instanceof Error ? err.message : "Failed to switch version");
    }
  };

  // Loading state
  if (loading) {
    return (
      <div className="flex items-center gap-2">
        <GitBranch className="h-4 w-4 text-muted-foreground" />
        <Skeleton className="h-9 w-40" />
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex items-center gap-2 text-destructive text-sm">
        <AlertCircle className="h-4 w-4" />
        <span>Failed to load versions</span>
      </div>
    );
  }

  // No versions available
  if (versions.length === 0) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground text-sm">
        <GitBranch className="h-4 w-4" />
        <span>No versions available</span>
      </div>
    );
  }

  // Get the currently selected version details
  const currentVersion = versions.find((v) => v.hash === headVersion);

  return (
    <TooltipProvider>
      <div className="flex items-center gap-2">
        <GitBranch className="h-4 w-4 text-muted-foreground" />

        <Select
          value={headVersion || undefined}
          onValueChange={handleVersionChange}
          disabled={disabled || switching}
        >
          <SelectTrigger className="w-56">
            {switching ? (
              <div className="flex items-center gap-2">
                <Loader2 className="h-3 w-3 animate-spin" />
                <span>Switching...</span>
              </div>
            ) : (
              <SelectValue placeholder="Select version">
                {headVersion && (
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm">{truncateHash(headVersion)}</span>
                    {currentVersion?.isLatest && (
                      <Badge variant="default" className="text-[10px] px-1.5 py-0 bg-green-500">
                        latest
                      </Badge>
                    )}
                  </div>
                )}
              </SelectValue>
            )}
          </SelectTrigger>

          <SelectContent>
            {versions.map((version) => (
              <Tooltip key={version.hash}>
                <TooltipTrigger asChild>
                  <SelectItem value={version.hash} className="cursor-pointer">
                    <div className="flex items-center gap-2 w-full">
                      <span className="font-mono text-sm">{formatVersionLabel(version)}</span>
                      {version.isLatest && (
                        <Badge variant="default" className="text-[10px] px-1.5 py-0 bg-green-500 ml-auto">
                          latest
                        </Badge>
                      )}
                    </div>
                  </SelectItem>
                </TooltipTrigger>
                <TooltipContent side="right" className="max-w-xs">
                  <div className="space-y-1 text-xs">
                    <div className="font-mono font-medium">{version.hash}</div>
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <Clock className="h-3 w-3" />
                      <span>{formatDate(version.createdAt, true)}</span>
                    </div>
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <HardDrive className="h-3 w-3" />
                      <span>{formatBytes(version.size)}</span>
                    </div>
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <FileText className="h-3 w-3" />
                      <span>{version.fileCount} files</span>
                    </div>
                  </div>
                </TooltipContent>
              </Tooltip>
            ))}
          </SelectContent>
        </Select>

        {disabled && (
          <Tooltip>
            <TooltipTrigger>
              <Badge variant="outline" className="text-xs text-muted-foreground">
                Read-only
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <span>You need editor permissions to switch versions</span>
            </TooltipContent>
          </Tooltip>
        )}

        {switchError && (
          <div className="flex items-center gap-1 text-destructive text-sm">
            <AlertCircle className="h-3 w-3" />
            <span>{switchError}</span>
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}
