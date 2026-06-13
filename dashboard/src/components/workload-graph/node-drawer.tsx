"use client";

import Link from "next/link";
import { X, ExternalLink } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useSkillSourceContent } from "@/hooks/use-skill-source-content";
import type { ArenaSourceContentNode } from "@/types/arena";
import type { WorkloadNode } from "./types";

// A skill is any directory (at any depth) that contains a SKILL.md.
function collectSkillDirs(nodes: ArenaSourceContentNode[], out: string[]): void {
  for (const n of nodes) {
    if (!n.isDirectory) continue;
    const kids = n.children ?? [];
    if (kids.some((c) => c.name.toLowerCase() === "skill.md")) out.push(n.name);
    else collectSkillDirs(kids, out);
  }
}

function skillNames(tree: ArenaSourceContentNode[]): string[] {
  const out: string[] = [];
  collectSkillDirs(tree, out);
  return out.sort((a, b) => a.localeCompare(b));
}

function SkillList({ source }: Readonly<{ source: string }>) {
  const { tree, loading } = useSkillSourceContent(source);
  if (loading) return <p className="text-xs text-muted-foreground">Loading skills…</p>;
  const names = skillNames(tree);
  if (names.length === 0) return null;
  return (
    <div>
      <p className="text-sm font-medium mb-1">Skills ({names.length})</p>
      <div className="flex flex-wrap gap-1">
        {names.map((n) => (
          <Badge key={n} variant="outline" className="text-xs font-mono">{n}</Badge>
        ))}
      </div>
    </div>
  );
}

function SkillSourceDetail({
  node,
  namespace,
}: Readonly<{ node: WorkloadNode; namespace?: string }>) {
  const source = node.detail.skillSource;
  if (!source) return null;
  const query = namespace ? `?namespace=${namespace}` : "";
  const href = `/skills/${encodeURIComponent(source)}${query}`;
  return (
    <div className="mb-3 space-y-2 text-sm">
      <div className="space-y-1">
        <div>
          <span className="text-muted-foreground">Skill source: </span>
          <span className="font-mono">{source}</span>
          <span className="text-muted-foreground"> ({node.detail.skillPhase})</span>
        </div>
        {node.detail.mountAs && (
          <div>
            <span className="text-muted-foreground">Mount as: </span>
            <span className="font-mono">{node.detail.mountAs}</span>
          </div>
        )}
        {!!node.detail.include?.length && (
          <div>
            <span className="text-muted-foreground">Include: </span>
            <span className="font-mono">{node.detail.include.join(", ")}</span>
          </div>
        )}
      </div>
      <SkillList source={source} />
      <Link
        href={href}
        className="inline-flex items-center gap-1 text-xs text-violet-600 hover:underline"
      >
        <ExternalLink className="h-3 w-3" />
        Open in Skills explorer
      </Link>
    </div>
  );
}

function VariableDetail({ node }: Readonly<{ node: WorkloadNode }>) {
  if (node.kind !== "variable") return null;
  const d = node.detail;
  return (
    <div className="mb-3 space-y-1 text-sm">
      <div>
        <span className="text-muted-foreground">Type: </span>
        <span className="font-mono">{d.varType}</span>
        {d.required ? <span className="text-muted-foreground"> · required</span> : null}
      </div>
      {d.example && (
        <div><span className="text-muted-foreground">Example: </span><span className="font-mono">{d.example}</span></div>
      )}
      {!!d.values?.length && (
        <div><span className="text-muted-foreground">Values: </span><span className="font-mono">{d.values.join(", ")}</span></div>
      )}
    </div>
  );
}

function ArtifactDetail({ node }: Readonly<{ node: WorkloadNode }>) {
  if (node.kind !== "artifact") return null;
  const d = node.detail;
  return (
    <div className="mb-3 space-y-1 text-sm">
      {d.artifactMode && (
        <div><span className="text-muted-foreground">Mode: </span><span className="font-mono">{d.artifactMode}</span></div>
      )}
      {!!d.producers?.length && (
        <div><span className="text-muted-foreground">Produced by: </span><span className="font-mono">{d.producers.join(", ")}</span></div>
      )}
      {!!d.consumers?.length && (
        <div><span className="text-muted-foreground">Consumed by: </span><span className="font-mono">{d.consumers.join(", ")}</span></div>
      )}
    </div>
  );
}

function ProviderPricingDetail({ node }: Readonly<{ node: WorkloadNode }>) {
  const p = node.detail.pricing;
  if (node.kind !== "provider" || !p) return null;
  return (
    <div className="mb-3 space-y-1 text-sm">
      {p.inputPer1kTokens != null && (
        <div><span className="text-muted-foreground">Input /1k: </span><span className="font-mono">{p.inputPer1kTokens}</span></div>
      )}
      {p.outputPer1kTokens != null && (
        <div><span className="text-muted-foreground">Output /1k: </span><span className="font-mono">{p.outputPer1kTokens}</span></div>
      )}
    </div>
  );
}

function ScenarioListDetail({ node }: Readonly<{ node: WorkloadNode }>) {
  if (node.kind !== "scenario" || !node.detail.scenarios?.length) return null;
  return (
    <div className="mb-3">
      <p className="text-sm font-medium mb-1">Scenarios ({node.detail.scenarios.length})</p>
      <ul className="space-y-1.5">
        {node.detail.scenarios.map((s) => (
          <li key={s.id} className="text-xs">
            <span className="font-mono">{s.id}</span>
            {s.turnCount != null && <span className="text-muted-foreground"> · {s.turnCount} turns</span>}
            {!!s.tags?.length && <span className="text-muted-foreground"> · {s.tags.join(", ")}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function ArenaRoleDetail({ node }: Readonly<{ node: WorkloadNode }>) {
  const d = node.detail;
  if (node.kind === "judge" && d.judgeProvider) {
    return (
      <div className="mb-3 text-sm">
        <span className="text-muted-foreground">Judge provider: </span>
        <span className="font-mono">{d.judgeProvider}</span>
      </div>
    );
  }
  if (node.kind === "persona" && d.persona) {
    return (
      <div className="mb-3 space-y-1 text-sm">
        {d.persona.role && <div><span className="text-muted-foreground">Role: </span><span className="font-mono">{d.persona.role}</span></div>}
        {d.persona.provider && <div><span className="text-muted-foreground">Provider: </span><span className="font-mono">{d.persona.provider}</span></div>}
      </div>
    );
  }
  return null;
}

export function NodeDrawer({
  node,
  onClose,
  namespace,
}: Readonly<{ node?: WorkloadNode; onClose: () => void; namespace?: string }>) {
  if (!node) return null;
  return (
    <aside className="absolute top-0 right-0 h-full w-80 bg-card border-l shadow-lg p-4 overflow-y-auto z-20">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-semibold">{node.label}</h3>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close">
          <X className="h-4 w-4" />
        </Button>
      </div>
      {node.detail.description && (
        <p className="text-sm text-muted-foreground mb-3">{node.detail.description}</p>
      )}
      <SkillSourceDetail node={node} namespace={namespace} />
      <VariableDetail node={node} />
      <ArtifactDetail node={node} />
      <ProviderPricingDetail node={node} />
      <ScenarioListDetail node={node} />
      <ArenaRoleDetail node={node} />
      {node.detail.systemTemplatePreview && (
        <pre className="text-xs bg-muted rounded p-2 whitespace-pre-wrap mb-3">
          {node.detail.systemTemplatePreview}
        </pre>
      )}
      {node.detail.model && (
        <div className="text-sm mb-3">
          <span className="text-muted-foreground">Model: </span>
          <span className="font-mono">{node.detail.model}</span>
        </div>
      )}
      {!!node.detail.tools?.length && (
        <div className="mb-3">
          <p className="text-sm font-medium mb-1">Tools</p>
          <ul className="space-y-1">
            {node.detail.tools.map((t) => (
              <li key={t.name} className="flex items-center justify-between text-xs">
                <span className="font-mono">{t.name}</span>
                {t.status && (
                  <Badge variant={t.status === "resolved" ? "secondary" : "outline"} className="text-xs">
                    {t.status}
                  </Badge>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      {!!node.detail.skills?.length && (
        <div>
          <p className="text-sm font-medium mb-1">Skills</p>
          <div className="flex flex-wrap gap-1">
            {node.detail.skills.map((s) => (
              <Badge key={s} variant="outline" className="text-xs">{s}</Badge>
            ))}
          </div>
        </div>
      )}
    </aside>
  );
}
