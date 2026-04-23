"use client";

import { useState, useCallback } from "react";
import { AlertTriangle, Sparkles } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useEnterpriseConfig } from "@/hooks/core";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import { useQueryClient } from "@tanstack/react-query";

interface EvalConfigPanelProps {
  agentName: string;
  frameworkType: string;
  evalsEnabled?: boolean;
  sampling?: {
    defaultRate?: number;
    extendedRate?: number;
  };
}

export function EvalConfigPanel({
  agentName,
  frameworkType,
  evalsEnabled = false,
  sampling,
}: Readonly<EvalConfigPanelProps>) {
  const { enterpriseEnabled, hideEnterprise } = useEnterpriseConfig();
  const dataService = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name || "demo";
  const queryClient = useQueryClient();

  const [enabled, setEnabled] = useState(evalsEnabled);
  const [lightweightRate, setLightweightRate] = useState(sampling?.defaultRate ?? 100);
  const [extendedRate, setExtendedRate] = useState(sampling?.extendedRate ?? 10);
  const [saving, setSaving] = useState(false);

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
            checks) run in the eval-worker. The sampling rates below control how
            often each tier fires. For custom group routing, edit the AgentRuntime
            CRD directly.
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
          </div>
        )}
      </CardContent>
    </Card>
  );
}
