"use client";

/**
 * Export deploy profile action. Fetches the workspace discovery menu, mints a
 * fresh omnia_sk_ token (when the key store allows it), assembles the
 * promptarena-deploy-omnia config: block, and shows it once. Issue #1519.
 */

import { useState } from "react";
import { Copy, Download } from "lucide-react";
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

const TOKEN_PLACEHOLDER = "<mint a token in Settings → API keys>";
const READ_ONLY_NOTE =
  "Token minting is unavailable in this deployment (read-only key store). Paste a token you mint manually.";
const MINTED_NOTE =
  "The token is shown once — store it securely and revoke it from Settings → API keys when done.";

const service = new WorkspaceApiService();

/** Read whether the api-key store supports minting new keys. */
async function fetchMintAvailability(): Promise<boolean> {
  const res = await fetch("/api/settings/api-keys");
  if (!res.ok) return false;
  const data = await res.json();
  return data?.config?.allowCreate ?? false;
}

/** Mint a fresh named omnia_sk_ token for this workspace. */
async function mintToken(workspace: string): Promise<string> {
  const res = await fetch("/api/settings/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: `deploy-${workspace}` }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Failed to mint token");
  }
  const data = await res.json();
  return data.key.key;
}

export default function ExportDeployProfile({ workspace }: { workspace: string }) {
  const [open, setOpen] = useState(false);
  const [output, setOutput] = useState("");
  const [minted, setMinted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function handleExport() {
    setLoading(true);
    setError(null);
    setCopied(false);
    try {
      const allowCreate = await fetchMintAvailability();
      const profile = await service.getDeployProfile(workspace);
      const token = allowCreate ? await mintToken(workspace) : TOKEN_PLACEHOLDER;
      setOutput(assembleDeployConfig(profile, token).yaml);
      setMinted(allowCreate);
      setOpen(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Export failed");
    } finally {
      setLoading(false);
    }
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

  const note = minted ? MINTED_NOTE : READ_ONLY_NOTE;

  return (
    <div className="space-y-3">
      <Button onClick={handleExport} disabled={loading}>
        {loading ? "Generating…" : "Export deploy profile"}
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
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
        </DialogContent>
      </Dialog>
    </div>
  );
}
