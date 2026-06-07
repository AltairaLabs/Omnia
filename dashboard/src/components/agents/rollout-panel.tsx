"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import type { RolloutConfig, RolloutStatus, RolloutStep } from "@/types/agent-runtime";

interface RolloutPanelProps {
  spec?: RolloutConfig;
  status?: RolloutStatus;
}

type PillVariant = "default" | "secondary" | "destructive" | "outline";

interface RolloutPhase {
  label: string;
  variant: PillVariant;
}

/**
 * Derives the headline pill from rollout state. `active` drives the
 * step-progress label; once idle, the terminal message distinguishes a clean
 * promotion from a rollback.
 */
export function deriveRolloutPhase(
  spec: RolloutConfig | undefined,
  status: RolloutStatus | undefined,
): RolloutPhase {
  if (status?.active) {
    const stepCount = spec?.steps?.length ?? 0;
    const stepNum = (status.currentStep ?? 0) + 1;
    const suffix = stepCount > 0 ? ` · step ${Math.min(stepNum, stepCount)}/${stepCount}` : "";
    return { label: `In progress${suffix}`, variant: "default" };
  }
  const message = status?.message?.toLowerCase() ?? "";
  if (message.includes("rollback") || message.includes("rolled back") || message.includes("roll back")) {
    return { label: "Rolled back", variant: "destructive" };
  }
  if (message.includes("promot")) {
    return { label: "Promoted", variant: "secondary" };
  }
  return { label: "Idle", variant: "outline" };
}

/** Human label for a single rollout step (exactly one field is set). */
export function stepLabel(step: RolloutStep): string {
  if (typeof step.setWeight === "number") {
    return `Set ${step.setWeight}%`;
  }
  if (step.analysis) {
    return `Analysis: ${step.analysis.templateName}`;
  }
  if (step.pause) {
    return step.pause.duration ? `Pause ${step.pause.duration}` : "Pause";
  }
  return "Step";
}

/** Visual state of a step relative to the current position. */
function stepClassName(index: number, currentStep: number, active?: boolean): string {
  if (active && index === currentStep) {
    return "border-primary bg-primary/10 text-foreground font-medium";
  }
  if (index < currentStep) {
    return "border-muted bg-muted/40 text-muted-foreground";
  }
  return "border-dashed text-muted-foreground";
}

interface StepItem {
  key: string;
  label: string;
  className: string;
}

/**
 * Precomputes render items with stable, unique keys. Steps carry no ID and
 * labels can repeat (e.g. two pauses), so keys are occurrence-counted rather
 * than derived from the array index.
 */
export function buildStepItems(
  steps: RolloutStep[],
  currentStep: number,
  active?: boolean,
): StepItem[] {
  const seen = new Map<string, number>();
  return steps.map((step, i) => {
    const label = stepLabel(step);
    const n = (seen.get(label) ?? 0) + 1;
    seen.set(label, n);
    return { key: `${label}#${n}`, label, className: stepClassName(i, currentStep, active) };
  });
}

/** Two-segment bar: stable (left) vs candidate (right), weight 0-100. */
function TrafficSplitBar({ weight }: { weight: number }) {
  const canary = Math.max(0, Math.min(100, weight));
  const stable = 100 - canary;
  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span className="text-muted-foreground">
          Stable <span className="font-semibold text-foreground">{stable}%</span>
        </span>
        <span className="text-muted-foreground">
          Candidate <span className="font-semibold text-foreground">{canary}%</span>
        </span>
      </div>
      <div
        className="flex h-3 w-full overflow-hidden rounded-full bg-muted"
        role="img"
        aria-label={`Stable ${stable} percent, candidate ${canary} percent`}
      >
        <div className="bg-primary/70 h-full" style={{ width: `${stable}%` }} />
        <div className="bg-amber-500 h-full" style={{ width: `${canary}%` }} />
      </div>
    </div>
  );
}

/**
 * Agent-detail panel visualizing a progressive-delivery rollout: live traffic
 * split, routing mode + enforcement, version delta, and step progression.
 * Renders nothing when the agent has no rollout configured or reported.
 */
export function RolloutPanel({ spec, status }: RolloutPanelProps) {
  if (!spec && !status) {
    return null;
  }

  const phase = deriveRolloutPhase(spec, status);
  const enforced = status?.trafficWeightEnforced;
  const stepItems = buildStepItems(spec?.steps ?? [], status?.currentStep ?? -1, status?.active);
  const showVersionDelta =
    !!status?.candidateVersion && status.candidateVersion !== status.stableVersion;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-2">
          <div>
            <CardTitle>Rollout</CardTitle>
            <CardDescription>Progressive delivery of the candidate</CardDescription>
          </div>
          <Badge variant={phase.variant}>{phase.label}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <TrafficSplitBar weight={status?.currentWeight ?? 0} />

        <div className="flex flex-wrap items-center gap-2 text-sm">
          {status?.trafficRoutingMode && (
            <Badge variant="outline">{status.trafficRoutingMode}</Badge>
          )}
          {enforced !== undefined && (
            <Badge variant={enforced ? "secondary" : "outline"}>
              {enforced ? "enforced" : "approx"}
            </Badge>
          )}
          {status?.stableVersion && (
            <span className="text-muted-foreground">
              {status.stableVersion}
              {showVersionDelta && (
                <>
                  {" → "}
                  <span className="text-foreground font-medium">{status.candidateVersion}</span>
                </>
              )}
            </span>
          )}
        </div>

        {stepItems.length > 0 && (
          <>
            <Separator />
            <ol className="flex flex-wrap gap-2">
              {stepItems.map((item) => (
                <li key={item.key} className={`rounded-md border px-2 py-1 text-xs ${item.className}`}>
                  {item.label}
                </li>
              ))}
            </ol>
          </>
        )}

        {status?.message && <p className="text-sm text-muted-foreground">{status.message}</p>}
      </CardContent>
    </Card>
  );
}
