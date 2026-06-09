"use client";

import { useState } from "react";
import { Trash2, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { usePurgeSessions } from "@/hooks/use-session-mutations";

/** "Older than" presets mapped to a day count (0 = no cutoff, purge everything). */
const OLDER_THAN_OPTIONS: { label: string; value: string; days: number }[] = [
  { label: "Everything", value: "all", days: 0 },
  { label: "Older than 7 days", value: "7d", days: 7 },
  { label: "Older than 30 days", value: "30d", days: 30 },
  { label: "Older than 90 days", value: "90d", days: 90 },
];

const DAY_MS = 24 * 60 * 60 * 1000;

/** Resolve an "older than" preset value to an RFC3339 before-cutoff, or undefined. */
function beforeFromPreset(value: string): string | undefined {
  const preset = OLDER_THAN_OPTIONS.find((o) => o.value === value);
  if (!preset || preset.days === 0) return undefined;
  return new Date(Date.now() - preset.days * DAY_MS).toISOString();
}

interface PurgeSessionsDialogProps {
  /** Agent names available in the workspace, for scoping the purge. */
  readonly agentNames: string[];
}

/**
 * Owner-only bulk session purge. Lets an owner delete all sessions in the
 * workspace, optionally narrowed to a single agent and/or sessions older than
 * a cutoff. User-agnostic — removes automated (ArenaJob, function) sessions too.
 */
export function PurgeSessionsDialog({ agentNames }: PurgeSessionsDialogProps) {
  const [open, setOpen] = useState(false);
  const [agent, setAgent] = useState("all");
  const [olderThan, setOlderThan] = useState("all");
  const [deletedCount, setDeletedCount] = useState<number | null>(null);
  const purge = usePurgeSessions();

  const handlePurge = () => {
    setDeletedCount(null);
    purge.mutate(
      {
        agent: agent === "all" ? undefined : agent,
        before: beforeFromPreset(olderThan),
      },
      { onSuccess: (count) => setDeletedCount(count) }
    );
  };

  const scopeLabel = agent === "all" ? "all agents" : agent;
  const timeLabel =
    OLDER_THAN_OPTIONS.find((o) => o.value === olderThan)?.label ?? "Everything";

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="text-destructive hover:text-destructive"
          data-testid="purge-sessions-open"
        >
          <Trash2 className="h-4 w-4 mr-2" />
          Purge
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Purge sessions</DialogTitle>
          <DialogDescription>
            Permanently delete sessions in this workspace, including their
            messages, tool calls, and eval results. This removes automated
            sessions (ArenaJob, function runs) too, and cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <span className="text-sm font-medium">Agent</span>
            <Select value={agent} onValueChange={setAgent}>
              <SelectTrigger data-testid="purge-agent">
                <SelectValue placeholder="Agent" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All agents</SelectItem>
                {agentNames.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <span className="text-sm font-medium">Age</span>
            <Select value={olderThan} onValueChange={setOlderThan}>
              <SelectTrigger data-testid="purge-age">
                <SelectValue placeholder="Age" />
              </SelectTrigger>
              <SelectContent>
                {OLDER_THAN_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <p className="text-sm text-muted-foreground">
            This will delete <span className="font-medium">{timeLabel.toLowerCase()}</span>{" "}
            for <span className="font-medium">{scopeLabel}</span>.
          </p>

          {purge.isError && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Purge failed</AlertTitle>
              <AlertDescription>
                {purge.error instanceof Error ? purge.error.message : "An unexpected error occurred"}
              </AlertDescription>
            </Alert>
          )}

          {deletedCount !== null && (
            <Alert>
              <AlertTitle>Purge complete</AlertTitle>
              <AlertDescription>
                Deleted {deletedCount} session{deletedCount === 1 ? "" : "s"}.
              </AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Close
          </Button>
          <Button
            variant="destructive"
            onClick={handlePurge}
            disabled={purge.isPending}
            data-testid="purge-confirm"
          >
            {purge.isPending ? "Purging…" : "Purge sessions"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
