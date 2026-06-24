"use client";

/**
 * Export deploy profile action. Fetches the workspace discovery menu (Ready
 * Providers/SkillSources only), lets the user pick which to include and which
 * LLM is the `default` primary, supplies an omnia_sk_ token, assembles the
 * promptarena-deploy-omnia config: block, and shows it once. Issue #1519.
 *
 * Token handling avoids minting a duplicate on every export: keys are
 * show-once, so a deterministic `deploy-<workspace>` key is used. When one
 * already exists the user chooses regenerate (revoke + re-mint) or reuse.
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
import type { DeployProfile } from "@/types/deploy-profile";
import { DeployConfigForm, type TokenAction } from "./deploy-config-form";

const READ_ONLY_NOTE =
  "Token minting is unavailable in this deployment (read-only key store). Paste a token you mint manually.";
const MINTED_NOTE =
  "The token is shown once — store it securely and revoke it from Settings → API keys when done.";
const REUSE_NOTE =
  "Reusing the existing key — keys are shown only once, so paste the token you saved when it was created.";

const service = new WorkspaceApiService();

/** A key as returned by the list endpoint (subset the export needs). */
export interface KeyInfo {
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

/** Mint a fresh omnia_sk_ token scoped to this workspace (#1561 P3). */
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

type TokenSource = "minted" | "reuse" | "readonly";

function noteFor(source: TokenSource): string {
  if (source === "reuse") return REUSE_NOTE;
  if (source === "readonly") return READ_ONLY_NOTE;
  return MINTED_NOTE;
}

/** Resolve the token to embed, applying the chosen token action. */
async function resolveToken(args: {
  workspace: string;
  allowCreate: boolean;
  existingKey: KeyInfo | null;
  action: TokenAction;
}): Promise<{ token: string; source: TokenSource }> {
  const { workspace, allowCreate, existingKey, action } = args;
  if (!allowCreate) return { token: readOnlyPlaceholder(), source: "readonly" };
  if (existingKey && action === "reuse") {
    return { token: reusePlaceholder(workspace), source: "reuse" };
  }
  if (existingKey) await deleteKey(existingKey.id);
  return { token: await mintToken(workspace), source: "minted" };
}

function firstLlm(profile: DeployProfile): string {
  return profile.providers.find((p) => p.role === "llm")?.name ?? "";
}

type Phase = "configure" | "output";

export default function ExportDeployProfile({ workspace }: { workspace: string }) {
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<Phase>("configure");
  const [profile, setProfile] = useState<DeployProfile | null>(null);
  const [includedProviders, setIncludedProviders] = useState<Set<string>>(new Set());
  const [includedSkills, setIncludedSkills] = useState<Set<string>>(new Set());
  const [defaultProvider, setDefaultProvider] = useState("");
  const [existingKey, setExistingKey] = useState<KeyInfo | null>(null);
  const [allowCreate, setAllowCreate] = useState(false);
  const [tokenAction, setTokenAction] = useState<TokenAction>("regenerate");
  const [output, setOutput] = useState("");
  const [tokenSource, setTokenSource] = useState<TokenSource>("minted");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function handleExport() {
    setLoading(true);
    setError(null);
    setCopied(false);
    try {
      const { allowCreate: canCreate, keys } = await fetchKeyListing();
      const p = await service.getDeployProfile(workspace);
      setProfile(p);
      setIncludedProviders(new Set(p.providers.map((x) => x.name)));
      setIncludedSkills(new Set(p.skills.map((x) => x.name)));
      setDefaultProvider(firstLlm(p));
      setAllowCreate(canCreate);
      setExistingKey(keys.find((k) => k.name === deployKeyName(workspace)) ?? null);
      setTokenAction("regenerate");
      setPhase("configure");
      setOpen(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Export failed");
    } finally {
      setLoading(false);
    }
  }

  function toggle(set: Set<string>, name: string): Set<string> {
    const next = new Set(set);
    if (next.has(name)) {
      next.delete(name);
    } else {
      next.add(name);
    }
    return next;
  }

  function toggleProvider(name: string) {
    setIncludedProviders((s) => toggle(s, name));
  }

  function toggleSkill(name: string) {
    setIncludedSkills((s) => toggle(s, name));
  }

  async function handleGenerate() {
    if (!profile) return;
    setLoading(true);
    setError(null);
    try {
      const subset: DeployProfile = {
        ...profile,
        providers: profile.providers.filter((p) => includedProviders.has(p.name)),
        skills: profile.skills.filter((s) => includedSkills.has(s.name)),
      };
      const { token, source } = await resolveToken({
        workspace,
        allowCreate,
        existingKey,
        action: tokenAction,
      });
      setOutput(assembleDeployConfig(subset, token, defaultProvider).yaml);
      setTokenSource(source);
      setPhase("output");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to generate profile");
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

  // A deploy needs exactly one included LLM marked default; gate Generate on it.
  const defaultIsValid =
    !!defaultProvider &&
    includedProviders.has(defaultProvider) &&
    !!profile?.providers.find((p) => p.name === defaultProvider && p.role === "llm");

  return (
    <div className="space-y-3">
      <Button onClick={handleExport} disabled={loading && phase === "configure"}>
        {loading && !open ? "Loading…" : "Export deploy profile"}
      </Button>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          {phase === "configure" && profile ? (
            <>
              <DialogHeader>
                <DialogTitle>Configure deploy profile</DialogTitle>
                <DialogDescription>
                  Pick the providers and skills to include, and which LLM is the
                  default. Only Ready resources are shown.
                </DialogDescription>
              </DialogHeader>
              <DeployConfigForm
                profile={profile}
                includedProviders={includedProviders}
                includedSkills={includedSkills}
                defaultProvider={defaultProvider}
                onToggleProvider={toggleProvider}
                onToggleSkill={toggleSkill}
                onSetDefault={setDefaultProvider}
                allowCreate={allowCreate}
                existingKey={existingKey}
                tokenAction={tokenAction}
                onTokenAction={setTokenAction}
              />
              <DialogFooter>
                <Button onClick={handleGenerate} disabled={loading || !defaultIsValid}>
                  {loading ? "Generating…" : "Generate"}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>Deploy profile</DialogTitle>
                <DialogDescription>
                  Paste this into your arena deploy config. {noteFor(tokenSource)}
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
