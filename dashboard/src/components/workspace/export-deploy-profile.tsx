"use client";

/**
 * Export deploy profile action. Fetches the workspace discovery menu, supplies
 * an omnia_sk_ token, assembles the promptarena-deploy-omnia config: block, and
 * shows it once. Issue #1519.
 *
 * Token handling avoids minting a duplicate on every export: keys are
 * show-once, so a deterministic `deploy-<workspace>` key is used. When one
 * already exists the dialog asks whether to regenerate (revoke + re-mint) or
 * reuse a saved token, instead of silently piling up keys.
 */

import { useState } from "react";
import { Copy, Download } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkspaceApiService } from "@/lib/data/workspace-api-service";
import { assembleDeployConfig } from "@/lib/deploy/assemble-profile";
import type { DeployProfile } from "@/types/deploy-profile";

const READ_ONLY_NOTE =
  "Token minting is unavailable in this deployment (read-only key store). Paste a token you mint manually.";
const MINTED_NOTE =
  "The token is shown once — store it securely and revoke it from Settings → API keys when done.";
const REUSE_NOTE =
  "Reusing the existing key — keys are shown only once, so paste the token you saved when it was created.";

const service = new WorkspaceApiService();

/** A key as returned by the list endpoint (subset the export needs). */
interface KeyInfo {
  id: string;
  name: string;
  createdAt: string;
  lastUsedAt: string | null;
}

function deployKeyName(workspace: string): string {
  return `deploy-${workspace}`;
}

function readOnlyPlaceholder(): string {
  return "<mint a token in Settings → API keys>";
}

function reusePlaceholder(workspace: string): string {
  return `<paste your saved ${deployKeyName(workspace)} token>`;
}

/** Fetch the key store config + the caller's existing keys. */
async function fetchKeyListing(): Promise<{ allowCreate: boolean; keys: KeyInfo[] }> {
  const res = await fetch("/api/settings/api-keys");
  if (!res.ok) return { allowCreate: false, keys: [] };
  const data = await res.json();
  return {
    allowCreate: data?.config?.allowCreate ?? false,
    keys: Array.isArray(data?.keys) ? data.keys : [],
  };
}

/**
 * Mint a fresh named omnia_sk_ token scoped to this workspace (#1561 P3):
 * the `workspaces` allowlist confines the downloadable credential so it can
 * only deploy to the workspace it was exported for — least privilege per #1519.
 */
async function mintToken(workspace: string): Promise<string> {
  const res = await fetch("/api/settings/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: deployKeyName(workspace), workspaces: [workspace] }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Failed to mint token");
  }
  const data = await res.json();
  return data.key.key;
}

/** Revoke a key by id (used to rotate the deploy key on regenerate). */
async function deleteKey(id: string): Promise<void> {
  const res = await fetch(`/api/settings/api-keys/${id}`, { method: "DELETE" });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Failed to revoke the existing key");
  }
}

function lastUsedLabel(lastUsedAt: string | null): string {
  return lastUsedAt
    ? `last used ${formatDistanceToNow(new Date(lastUsedAt), { addSuffix: true })}`
    : "never used";
}

type Phase = "choice" | "output";
type TokenSource = "minted" | "reuse" | "readonly";

function noteFor(source: TokenSource): string {
  if (source === "reuse") return REUSE_NOTE;
  if (source === "readonly") return READ_ONLY_NOTE;
  return MINTED_NOTE;
}

export default function ExportDeployProfile({ workspace }: { workspace: string }) {
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<Phase>("output");
  const [output, setOutput] = useState("");
  const [tokenSource, setTokenSource] = useState<TokenSource>("minted");
  const [existing, setExisting] = useState<KeyInfo | null>(null);
  const [profile, setProfile] = useState<DeployProfile | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  function showOutput(p: DeployProfile, token: string, source: TokenSource) {
    setOutput(assembleDeployConfig(p, token).yaml);
    setTokenSource(source);
    setPhase("output");
    setOpen(true);
  }

  async function handleExport() {
    setLoading(true);
    setError(null);
    setCopied(false);
    try {
      const { allowCreate, keys } = await fetchKeyListing();
      const p = await service.getDeployProfile(workspace);
      setProfile(p);

      if (!allowCreate) {
        showOutput(p, readOnlyPlaceholder(), "readonly");
        return;
      }

      const match = keys.find((k) => k.name === deployKeyName(workspace)) ?? null;
      if (match) {
        // A deploy key already exists — ask rather than mint a duplicate.
        setExisting(match);
        setPhase("choice");
        setOpen(true);
      } else {
        showOutput(p, await mintToken(workspace), "minted");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Export failed");
    } finally {
      setLoading(false);
    }
  }

  async function handleRegenerate() {
    if (!profile || !existing) return;
    setLoading(true);
    setError(null);
    try {
      await deleteKey(existing.id);
      showOutput(profile, await mintToken(workspace), "minted");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to regenerate token");
    } finally {
      setLoading(false);
    }
  }

  function handleReuse() {
    if (!profile) return;
    showOutput(profile, reusePlaceholder(workspace), "reuse");
  }

  function handleCopy() {
    navigator.clipboard.writeText(output);
    setCopied(true);
  }

  function handleDownload() {
    const blob = new Blob([output], { type: "text/yaml" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `deploy-profile-${workspace}.yaml`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  const note = noteFor(tokenSource);

  return (
    <div className="space-y-3">
      <Button onClick={handleExport} disabled={loading}>
        {loading ? "Generating…" : "Export deploy profile"}
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          {phase === "choice" && existing ? (
            <>
              <DialogHeader>
                <DialogTitle>A deploy key already exists</DialogTitle>
                <DialogDescription>
                  <span className="font-mono">{existing.name}</span> was created{" "}
                  {formatDistanceToNow(new Date(existing.createdAt), { addSuffix: true })} (
                  {lastUsedLabel(existing.lastUsedAt)}). Keys are shown only once, so we
                  can&apos;t show its token again — regenerate a fresh one (revokes the old
                  key) or reuse the token you saved.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={handleReuse} disabled={loading}>
                  Use saved token
                </Button>
                <Button onClick={handleRegenerate} disabled={loading}>
                  {loading ? "Regenerating…" : "Regenerate token"}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>Deploy profile</DialogTitle>
                <DialogDescription>
                  Paste this into your arena deploy config. {note}
                </DialogDescription>
              </DialogHeader>
              <pre
                data-testid="deploy-profile-output"
                className="max-h-80 overflow-auto rounded bg-muted p-4 text-xs"
              >
                {output}
              </pre>
              <DialogFooter>
                <Button variant="outline" onClick={handleCopy}>
                  <Copy className="mr-2 h-4 w-4" />
                  {copied ? "Copied" : "Copy"}
                </Button>
                <Button onClick={handleDownload}>
                  <Download className="mr-2 h-4 w-4" />
                  Download
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
