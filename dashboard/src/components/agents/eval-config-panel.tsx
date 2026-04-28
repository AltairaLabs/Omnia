"use client";

import { useState, useCallback } from "react";
import { AlertTriangle, ChevronDown, Sparkles } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { useEnterpriseConfig } from "@/hooks/core";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import { useQueryClient } from "@tanstack/react-query";
import { useEvalGroups } from "@/hooks/use-eval-groups";
import { GroupSelector } from "./group-selector";

interface EvalConfigPanelProps {
  agentName: string;
  frameworkType: string;
  evalsEnabled?: boolean;
  sampling?: {
    defaultRate?: number;
    extendedRate?: number;
  };
  // inline / worker group routing (issue #988). When the agent's
  // CRD has no spec.evals.inline.groups (or .worker.groups) the
  // operator applies its built-in default — passing undefined here
  // surfaces "[default]" in the UI as the effective value.
  inlineGroups?: string[];
  workerGroups?: string[];
  // The agent's PromptPack name, used to discover custom group names
  // declared on the pack's eval defs. Optional — when absent the
  // GroupSelector still offers the four built-in groups.
  promptPackName?: string;
}

export function EvalConfigPanel({
  agentName,
  frameworkType,
  evalsEnabled = false,
  sampling,
  inlineGroups,
  workerGroups,
  promptPackName,
}: Readonly<EvalConfigPanelProps>) {
  const { enterpriseEnabled, hideEnterprise } = useEnterpriseConfig();
  const dataService = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name || "demo";
  const queryClient = useQueryClient();

  const [enabled, setEnabled] = useState(evalsEnabled);
  const [lightweightRate, setLightweightRate] = useState(sampling?.defaultRate ?? 100);
  const [extendedRate, setExtendedRate] = useState(sampling?.extendedRate ?? 10);
  const [inline, setInline] = useState<string[]>(inlineGroups ?? []);
  const [worker, setWorker] = useState<string[]>(workerGroups ?? []);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const { groups: groupOptions } = useEvalGroups(workspace, promptPackName);

  const isPromptKit = frameworkType === "promptkit" || frameworkType === "";

  const handleToggle = useCallback(async (checked: boolean) => {
    setEnabled(checked);
    setSaving(true);
    try {
      await dataService.updateAgentEvals(workspace, agentName, {
        enabled: checked,
      });
      await queryClient.invalidateQueries({ queryKey: ["agent", workspace, agentName] });
    } catch {
      setEnabled(!checked);
    } finally {
      setSaving(false);
    }
  }, [workspace, agentName, dataService, queryClient]);

  const handleSamplingChange = useCallback(async (field: "defaultRate" | "extendedRate", value: number) => {
    const prev = field === "defaultRate" ? lightweightRate : extendedRate;
    if (field === "defaultRate") setLightweightRate(value);
    else setExtendedRate(value);

    setSaving(true);
    try {
      await dataService.updateAgentEvals(workspace, agentName, {
        sampling: { [field]: value },
      });
      await queryClient.invalidateQueries({ queryKey: ["agent", workspace, agentName] });
    } catch {
      if (field === "defaultRate") setLightweightRate(prev);
      else setExtendedRate(prev);
    } finally {
      setSaving(false);
    }
  }, [workspace, agentName, lightweightRate, extendedRate, dataService, queryClient]);

  const handleGroupsChange = useCallback(
    async (path: "inline" | "worker", next: string[]) => {
      const prev = path === "inline" ? inline : worker;
      if (path === "inline") setInline(next);
      else setWorker(next);
      setSaving(true);
      try {
        await dataService.updateAgentEvals(workspace, agentName, {
          [path]: { groups: next },
        });
        await queryClient.invalidateQueries({ queryKey: ["agent", workspace, agentName] });
      } catch {
        // Roll back optimistic update on failure so the UI reflects
        // the operator-side truth.
        if (path === "inline") setInline(prev);
        else setWorker(prev);
      } finally {
        setSaving(false);
      }
    },
    [workspace, agentName, inline, worker, dataService, queryClient],
  );

  // Only show for EE mode, hide completely if hideEnterprise
  if (hideEnterprise || !enterpriseEnabled) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Sparkles className="h-4 w-4 text-purple-500" />
          Realtime Evals
        </CardTitle>
        <CardDescription>
          Configure real-time eval execution for this agent
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <Alert variant="default" className="border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-950">
          <AlertTriangle className="h-4 w-4 text-blue-600 dark:text-blue-400" />
          <AlertDescription className="text-blue-700 dark:text-blue-300">
            When enabled, cheap deterministic evals (contains, regex) run inline
            in the agent runtime and expensive ones (LLM judges, external API
            checks) run in the eval-worker. The sampling rates control how
            often each tier fires; advanced routing lets you override which
            groups run on which path.
          </AlertDescription>
        </Alert>

        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <Label htmlFor="eval-toggle" className="text-sm font-medium">
              Enable evals
            </Label>
            <p className="text-xs text-muted-foreground">
              {isPromptKit
                ? "Runs lightweight evals inline and expensive evals in the eval-worker"
                : "Only available for PromptKit agents"}
            </p>
          </div>
          <Switch
            id="eval-toggle"
            checked={enabled}
            onCheckedChange={handleToggle}
            disabled={!isPromptKit || saving}
            aria-label="Toggle eval execution"
          />
        </div>

        {enabled && isPromptKit && (
          <div className="space-y-5 rounded-md border p-4">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-medium">Lightweight eval sampling</Label>
                <span className="text-sm text-muted-foreground">{lightweightRate}%</span>
              </div>
              <Slider
                value={[lightweightRate]}
                onValueChange={([v]) => setLightweightRate(v)}
                onValueCommit={([v]) => handleSamplingChange("defaultRate", v)}
                min={0}
                max={100}
                step={5}
                disabled={saving}
                aria-label="Lightweight eval sampling rate"
              />
              <p className="text-xs text-muted-foreground">
                Inline path — fast-running handlers (contains, regex, deterministic scorers)
              </p>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-medium">Extended eval sampling</Label>
                <span className="text-sm text-muted-foreground">{extendedRate}%</span>
              </div>
              <Slider
                value={[extendedRate]}
                onValueChange={([v]) => setExtendedRate(v)}
                onValueCommit={([v]) => handleSamplingChange("extendedRate", v)}
                min={0}
                max={100}
                step={5}
                disabled={saving}
                aria-label="Extended eval sampling rate"
              />
              <p className="text-xs text-muted-foreground">
                Worker path — long-running and external handlers (LLM judges, REST, A2A)
              </p>
            </div>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="flex w-full items-center justify-between text-left text-sm font-medium hover:text-primary"
                >
                  <span>Advanced routing</span>
                  <ChevronDown
                    className={`h-4 w-4 transition-transform ${advancedOpen ? "rotate-180" : ""}`}
                  />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent className="space-y-5 pt-4">
                <p className="text-xs text-muted-foreground">
                  Override which eval groups run on each path. Empty = use the
                  built-in default for that path. Built-in groups:
                  {" "}
                  <code>fast-running</code>, <code>long-running</code>,
                  {" "}<code>external</code>, <code>default</code>. Custom
                  groups discovered from the agent&apos;s PromptPack are also
                  offered.
                </p>
                <GroupSelector
                  idPrefix="evals-inline"
                  label="Inline groups (run in the runtime)"
                  options={groupOptions}
                  value={inline}
                  disabled={saving}
                  onChange={(next) => handleGroupsChange("inline", next)}
                  emptyHint='Empty list uses the built-in default ["fast-running"].'
                />
                <GroupSelector
                  idPrefix="evals-worker"
                  label="Worker groups (run out-of-band in the eval-worker)"
                  options={groupOptions}
                  value={worker}
                  disabled={saving}
                  onChange={(next) => handleGroupsChange("worker", next)}
                  emptyHint='Empty list uses the built-in default ["long-running", "external"].'
                />
              </CollapsibleContent>
            </Collapsible>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
