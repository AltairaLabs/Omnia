"use client";

import { ReactFlow, Background, type Node, type Edge } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { notFound } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { BrandPresetSwitcher } from "@/components/branding/brand-preset-switcher";
import { useDevMode, useDemoMode } from "@/hooks/core";
import { useFlowColorMode } from "@/lib/flow/use-color-mode";
import { getStatusClasses, type StatusKind } from "@/lib/colors/status";

const STATUS_KINDS: StatusKind[] = ["success", "warning", "info", "error", "neutral"];
const CATEGORY_INDICES = [1, 2, 3, 4, 5, 6, 7, 8];
const CHART_INDICES = [1, 2, 3, 4, 5];
const BUTTON_VARIANTS = ["default", "secondary", "outline", "ghost", "destructive"] as const;

const FLOW_NODES: Node[] = [
  { id: "a", position: { x: 0, y: 20 }, data: { label: "Agent" }, style: { background: "var(--category-1)", color: "#fff", border: "none" } },
  { id: "b", position: { x: 180, y: 0 }, data: { label: "Skill" }, style: { background: "var(--category-2)", color: "#fff", border: "none" } },
  { id: "c", position: { x: 180, y: 80 }, data: { label: "Tool" }, style: { background: "var(--category-4)", color: "#fff", border: "none" } },
];
const FLOW_EDGES: Edge[] = [
  { id: "a-b", source: "a", target: "b" },
  { id: "a-c", source: "a", target: "c" },
];

function Swatch({ varName, label }: Readonly<{ varName: string; label: string }>) {
  return (
    <div className="flex flex-col items-center gap-1">
      <div className="h-12 w-12 rounded-md border border-border" style={{ background: `var(${varName})` }} />
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  );
}

function Section({ title, children }: Readonly<{ title: string; children: React.ReactNode }>) {
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold text-foreground">{title}</h2>
      {children}
    </section>
  );
}

/**
 * Dev-only theme kitchen-sink. Renders every design-token-driven primitive so a
 * brand preset switch (header or the switcher below) visibly re-themes the whole
 * page — and so Playwright can screenshot it per preset for visual regression.
 * 404s outside dev/demo so it is never reachable in a real deployment.
 */
export default function ThemePreviewPage() {
  const { isDevMode, loading: devLoading } = useDevMode();
  const { isDemoMode, loading: demoLoading } = useDemoMode();
  const colorMode = useFlowColorMode();

  if (devLoading || demoLoading) return null;
  if (!isDevMode && !isDemoMode) {
    notFound();
  }

  return (
    <div className="space-y-8 p-8" data-testid="theme-preview">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Theme preview</h1>
          <p className="text-sm text-muted-foreground">
            Dev-only. Switch brand presets to see every token re-theme.
          </p>
        </div>
        <BrandPresetSwitcher />
      </div>

      <Section title="Status badges">
        <div className="flex flex-wrap gap-2">
          {STATUS_KINDS.map((kind) => {
            const c = getStatusClasses(kind);
            return (
              <span
                key={kind}
                className={`inline-flex items-center rounded-full px-3 py-1 text-sm font-medium ${c.bg} ${c.text}`}
              >
                {kind}
              </span>
            );
          })}
        </div>
      </Section>

      <Section title="Buttons">
        <div className="flex flex-wrap gap-2">
          {BUTTON_VARIANTS.map((v) => (
            <Button key={v} variant={v}>
              {v}
            </Button>
          ))}
        </div>
      </Section>

      <Section title="Card">
        <Card className="max-w-sm">
          <CardHeader>
            <CardTitle>Themed card</CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            Surface, border, and muted text all derive from tokens.
            <div className="mt-2 flex gap-2">
              <Badge>Default</Badge>
              <Badge variant="secondary">Secondary</Badge>
              <Badge variant="outline">Outline</Badge>
            </div>
          </CardContent>
        </Card>
      </Section>

      <Section title="Categorical palette">
        <div className="flex flex-wrap gap-3">
          {CATEGORY_INDICES.map((i) => (
            <Swatch key={i} varName={`--category-${i}`} label={`category-${i}`} />
          ))}
        </div>
      </Section>

      <Section title="Chart series">
        <div className="flex flex-wrap gap-3">
          {CHART_INDICES.map((i) => (
            <Swatch key={i} varName={`--chart-${i}`} label={`chart-${i}`} />
          ))}
        </div>
      </Section>

      <Section title="React Flow">
        <div className="h-64 rounded-md border border-border" data-testid="theme-preview-flow">
          <ReactFlow
            nodes={FLOW_NODES}
            edges={FLOW_EDGES}
            colorMode={colorMode}
            fitView
            proOptions={{ hideAttribution: true }}
          >
            <Background />
          </ReactFlow>
        </div>
      </Section>
    </div>
  );
}
