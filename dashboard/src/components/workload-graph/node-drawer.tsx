"use client";

import { X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { WorkloadNode } from "./types";

export function NodeDrawer({
  node,
  onClose,
}: Readonly<{ node?: WorkloadNode; onClose: () => void }>) {
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
