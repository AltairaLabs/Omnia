"use client";

/**
 * AgentConditions — compact table of an agent's status conditions.
 *
 * Renders all conditions (healthy and failing) in a low-padding table. Lives
 * at the bottom of the agent Overview tab as reference detail rather than the
 * airy, top-of-page table it replaced. Renders nothing when there are no
 * conditions.
 */

import { cn } from "@/lib/utils";
import type { Condition } from "@/types/common";

interface AgentConditionsProps {
  conditions?: Condition[];
}

export function AgentConditions({ conditions }: Readonly<AgentConditionsProps>) {
  if (!conditions || conditions.length === 0) {
    return null;
  }

  return (
    <div className="rounded-lg border bg-card p-3">
      <p className="text-xs font-medium text-muted-foreground mb-2">Conditions</p>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-muted-foreground border-b">
            <th className="pb-1 font-medium">Type</th>
            <th className="pb-1 font-medium">Reason</th>
            <th className="pb-1 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          {conditions.map((condition) => (
            <tr key={condition.type} className="border-b last:border-0">
              <td className="py-1 pr-4">
                <span
                  className={cn(
                    "px-2 py-0.5 rounded text-xs font-medium",
                    condition.status === "True"
                      ? "bg-success/15 text-success"
                      : "bg-destructive/15 text-destructive",
                  )}
                >
                  {condition.type}
                </span>
              </td>
              <td className="py-1 pr-4 font-medium">{condition.reason}</td>
              <td className="py-1 text-muted-foreground">{condition.message || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
