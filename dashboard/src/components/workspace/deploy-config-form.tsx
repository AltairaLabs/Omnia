"use client";

/**
 * Configure form for the deploy-profile export (#1519). Lets the user choose
 * which Ready providers/skills to include, which LLM is the `default` primary,
 * and (when a deploy key already exists) whether to regenerate or reuse it.
 */

import { formatDistanceToNow } from "date-fns";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Badge } from "@/components/ui/badge";
import type { DeployProfile } from "@/types/deploy-profile";

export type TokenAction = "regenerate" | "reuse";

interface KeyInfo {
  id: string;
  name: string;
  createdAt: string;
  lastUsedAt: string | null;
}

function lastUsedLabel(lastUsedAt: string | null): string {
  return lastUsedAt
    ? `last used ${formatDistanceToNow(new Date(lastUsedAt), { addSuffix: true })}`
    : "never used";
}

/** Provider include checklist with a default-LLM radio. */
function ProviderSelect({
  profile,
  included,
  defaultProvider,
  onToggle,
  onSetDefault,
}: Readonly<{
  profile: DeployProfile;
  included: Set<string>;
  defaultProvider: string;
  onToggle: (name: string) => void;
  onSetDefault: (name: string) => void;
}>) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">Providers</p>
      <RadioGroup value={defaultProvider} onValueChange={onSetDefault}>
        <div className="space-y-2 rounded-md border p-3">
          {profile.providers.map((p) => {
            const isIncluded = included.has(p.name);
            const isLlm = p.role === "llm";
            return (
              <div key={p.name} className="flex items-center gap-3">
                <Checkbox
                  id={`prov-${p.name}`}
                  checked={isIncluded}
                  onCheckedChange={() => onToggle(p.name)}
                />
                <Label htmlFor={`prov-${p.name}`} className="flex-1 font-normal">
                  {p.name}
                </Label>
                <Badge variant="outline">{p.role}</Badge>
                {isLlm && (
                  <Label
                    className="flex items-center gap-1.5 text-xs font-normal text-muted-foreground"
                    data-disabled={!isIncluded || undefined}
                  >
                    <RadioGroupItem
                      value={p.name}
                      disabled={!isIncluded}
                      aria-label={`Set ${p.name} as default`}
                    />
                    default
                  </Label>
                )}
              </div>
            );
          })}
        </div>
      </RadioGroup>
    </div>
  );
}

/** Skill include checklist. */
function SkillSelect({
  profile,
  included,
  onToggle,
}: Readonly<{
  profile: DeployProfile;
  included: Set<string>;
  onToggle: (name: string) => void;
}>) {
  if (profile.skills.length === 0) return null;
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">Skills</p>
      <div className="space-y-2 rounded-md border p-3">
        {profile.skills.map((s) => (
          <div key={s.name} className="flex items-center gap-3">
            <Checkbox
              id={`skill-${s.name}`}
              checked={included.has(s.name)}
              onCheckedChange={() => onToggle(s.name)}
            />
            <Label htmlFor={`skill-${s.name}`} className="flex-1 font-normal">
              {s.name}
            </Label>
            <Badge variant="outline">{s.type}</Badge>
          </div>
        ))}
      </div>
    </div>
  );
}

/** Token action chooser, shown only when a deploy key already exists. */
function TokenChoice({
  existingKey,
  action,
  onAction,
}: Readonly<{
  existingKey: KeyInfo;
  action: TokenAction;
  onAction: (a: TokenAction) => void;
}>) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">Token</p>
      <p className="text-xs text-muted-foreground">
        A deploy key (<span className="font-mono">{existingKey.name}</span>) already
        exists — created{" "}
        {formatDistanceToNow(new Date(existingKey.createdAt), { addSuffix: true })} (
        {lastUsedLabel(existingKey.lastUsedAt)}).
      </p>
      <RadioGroup value={action} onValueChange={(v) => onAction(v as TokenAction)}>
        <Label className="flex items-center gap-2 font-normal">
          <RadioGroupItem value="regenerate" aria-label="regenerate" /> Regenerate —
          revoke the old key and mint a fresh token
        </Label>
        <Label className="flex items-center gap-2 font-normal">
          <RadioGroupItem value="reuse" aria-label="reuse" /> Reuse — paste the token
          I saved
        </Label>
      </RadioGroup>
    </div>
  );
}

export function DeployConfigForm({
  profile,
  includedProviders,
  includedSkills,
  defaultProvider,
  onToggleProvider,
  onToggleSkill,
  onSetDefault,
  allowCreate,
  existingKey,
  tokenAction,
  onTokenAction,
}: Readonly<{
  profile: DeployProfile;
  includedProviders: Set<string>;
  includedSkills: Set<string>;
  defaultProvider: string;
  onToggleProvider: (name: string) => void;
  onToggleSkill: (name: string) => void;
  onSetDefault: (name: string) => void;
  allowCreate: boolean;
  existingKey: KeyInfo | null;
  tokenAction: TokenAction;
  onTokenAction: (a: TokenAction) => void;
}>) {
  const hasLlm = profile.providers.some((p) => p.role === "llm");
  return (
    <div className="max-h-96 space-y-4 overflow-y-auto py-2">
      <ProviderSelect
        profile={profile}
        included={includedProviders}
        defaultProvider={defaultProvider}
        onToggle={onToggleProvider}
        onSetDefault={onSetDefault}
      />
      {!hasLlm && (
        <p className="text-sm text-destructive">
          No Ready LLM provider in this workspace — a deployment needs one as its
          default. Make a Provider Ready, then export again.
        </p>
      )}
      <SkillSelect
        profile={profile}
        included={includedSkills}
        onToggle={onToggleSkill}
      />
      {allowCreate && existingKey && (
        <TokenChoice
          existingKey={existingKey}
          action={tokenAction}
          onAction={onTokenAction}
        />
      )}
      {!allowCreate && (
        <p className="text-xs text-muted-foreground">
          This deployment uses a read-only key store — the profile will contain a
          placeholder; paste a token you mint manually.
        </p>
      )}
    </div>
  );
}
