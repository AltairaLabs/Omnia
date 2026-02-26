/**
 * Alert configuration panel for eval metric thresholds.
 *
 * Stores alert configs in localStorage. Shows a form to add/edit/remove
 * threshold alerts per eval metric.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Trash2, Plus, Bell } from "lucide-react";
import type { EvalMetricInfo } from "@/hooks";

const STORAGE_KEY = "omnia-eval-alerts";

export interface EvalAlert {
  metricName: string;
  threshold: number;
  enabled: boolean;
}

/** Load alerts from localStorage. */
export function loadAlerts(): EvalAlert[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    return JSON.parse(raw) as EvalAlert[];
  } catch {
    return [];
  }
}

/** Save alerts to localStorage. */
export function saveAlerts(alerts: EvalAlert[]): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(alerts));
}

/** Build a threshold map from alerts for quick lookup. */
export function buildAlertThresholdMap(alerts: EvalAlert[]): Map<string, number> {
  const map = new Map<string, number>();
  for (const alert of alerts) {
    if (alert.enabled) {
      map.set(alert.metricName, alert.threshold);
    }
  }
  return map;
}

interface AlertConfigPanelProps {
  availableMetrics?: EvalMetricInfo[];
  onAlertsChange?: (alerts: EvalAlert[]) => void;
}

export function AlertConfigPanel({
  availableMetrics,
  onAlertsChange,
}: Readonly<AlertConfigPanelProps>) {
  const [alerts, setAlerts] = useState<EvalAlert[]>(() => loadAlerts());
  const [newMetric, setNewMetric] = useState<string>("");
  const [newThreshold, setNewThreshold] = useState<string>("0.8");

  const updateAlerts = useCallback(
    (updated: EvalAlert[]) => {
      setAlerts(updated);
      saveAlerts(updated);
      onAlertsChange?.(updated);
    },
    [onAlertsChange]
  );

  const handleAdd = useCallback(() => {
    if (!newMetric) return;
    const threshold = Number.parseFloat(newThreshold);
    if (Number.isNaN(threshold) || threshold < 0 || threshold > 1) return;

    const exists = alerts.some((a) => a.metricName === newMetric);
    if (exists) return;

    updateAlerts([...alerts, { metricName: newMetric, threshold, enabled: true }]);
    setNewMetric("");
    setNewThreshold("0.8");
  }, [newMetric, newThreshold, alerts, updateAlerts]);

  const handleRemove = useCallback(
    (metricName: string) => {
      updateAlerts(alerts.filter((a) => a.metricName !== metricName));
    },
    [alerts, updateAlerts]
  );

  const handleToggle = useCallback(
    (metricName: string) => {
      updateAlerts(
        alerts.map((a) =>
          a.metricName === metricName ? { ...a, enabled: !a.enabled } : a
        )
      );
    },
    [alerts, updateAlerts]
  );

  const metricOptions = (availableMetrics ?? []).filter(
    (m) => !alerts.some((a) => a.metricName === m.name)
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <Bell className="h-4 w-4" />
          Alert Thresholds
        </CardTitle>
        <CardDescription>
          Set pass rate thresholds for eval metrics. Alerts are stored locally.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Add new alert */}
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Label htmlFor="alert-metric" className="text-xs">Metric</Label>
            <Select value={newMetric} onValueChange={setNewMetric}>
              <SelectTrigger id="alert-metric">
                <SelectValue placeholder="Select metric" />
              </SelectTrigger>
              <SelectContent>
                {metricOptions.map((m) => (
                  <SelectItem key={m.name} value={m.name}>
                    {m.name.replace(/^omnia_eval_/, "")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="w-24">
            <Label htmlFor="alert-threshold" className="text-xs">Threshold</Label>
            <Input
              id="alert-threshold"
              type="number"
              min="0"
              max="1"
              step="0.05"
              value={newThreshold}
              onChange={(e) => setNewThreshold(e.target.value)}
            />
          </div>
          <Button size="sm" onClick={handleAdd} disabled={!newMetric}>
            <Plus className="h-4 w-4" />
          </Button>
        </div>

        {/* Active alerts list */}
        {alerts.length === 0 && (
          <p className="text-sm text-muted-foreground text-center py-4">
            No alerts configured
          </p>
        )}
        <div className="space-y-2">
          {alerts.map((alert) => (
            <div
              key={alert.metricName}
              className="flex items-center justify-between rounded-md border px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => handleToggle(alert.metricName)}
                  className="text-xs"
                >
                  <Badge variant={alert.enabled ? "default" : "secondary"}>
                    {alert.enabled ? "On" : "Off"}
                  </Badge>
                </button>
                <span className="font-mono text-sm">
                  {alert.metricName.replace(/^omnia_eval_/, "")}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  &lt; {alert.threshold.toFixed(2)}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRemove(alert.metricName)}
                >
                  <Trash2 className="h-3 w-3 text-muted-foreground" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
