"use client";

import Link from "next/link";
import { Wrench, Globe, Server, Clock, CheckCircle, XCircle, AlertCircle, Terminal, FileJson } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/agents";
import type { ToolRegistry, DiscoveredTool } from "@/types";

interface ToolRegistryCardProps {
  registry: ToolRegistry;
}

function formatRelativeTime(timestamp?: string): string {
  if (!timestamp) return "-";
  const date = new Date(timestamp);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

function getHandlerTypeIcon(type: string) {
  switch (type) {
    case "http":
      return Globe;
    case "grpc":
      return Server;
    case "mcp":
      return Terminal;
    case "openapi":
      return FileJson;
    default:
      return Wrench;
  }
}

function ToolStatusIcon({ status }: Readonly<{ status: DiscoveredTool["status"] }>) {
  switch (status) {
    case "Available":
      return <CheckCircle className="h-3 w-3 text-green-500" />;
    case "Unavailable":
      return <XCircle className="h-3 w-3 text-red-500" />;
    default:
      return <AlertCircle className="h-3 w-3 text-yellow-500" />;
  }
}

export function ToolRegistryCard({ registry }: Readonly<ToolRegistryCardProps>) {
  const { metadata, spec, status } = registry;
  const tools = status?.discoveredTools || [];
  const availableCount = tools.filter((t) => t.status === "Available").length;
  const totalCount = status?.discoveredToolsCount || 0;

  // Get unique handler types (handle missing handlers gracefully)
  const handlers = spec.handlers || [];
  const handlerTypes = [...new Set(handlers.map((h) => h.type))];

  return (
    <Link
      href={`/tools/${metadata.name}?namespace=${metadata.namespace}`}
    >
      <Card className="hover:border-primary/50 transition-colors cursor-pointer h-full">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <Wrench className="h-4 w-4 text-muted-foreground shrink-0" />
              <CardTitle className="text-base truncate">{metadata.name}</CardTitle>
            </div>
            <StatusBadge phase={status?.phase} />
          </div>
          <p className="text-xs text-muted-foreground">{metadata.namespace}</p>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Tool count summary */}
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Tools:</span>
            <Badge variant="secondary" className="text-xs">
              {availableCount}/{totalCount} available
            </Badge>
          </div>

          {/* Handler types */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Types:</span>
            <div className="flex gap-1.5">
              {handlerTypes.map((type) => {
                const Icon = getHandlerTypeIcon(type);
                return (
                  <Badge key={type} variant="outline" className="text-xs capitalize gap-1">
                    <Icon className="h-3 w-3" />
                    {type}
                  </Badge>
                );
              })}
            </div>
          </div>

          {/* Tool list preview (first 3) */}
          {tools.length > 0 && (
            <div className="space-y-1">
              {tools.slice(0, 3).map((tool) => (
                <div key={tool.name} className="flex items-center gap-2 text-xs">
                  <ToolStatusIcon status={tool.status} />
                  <code className="text-muted-foreground truncate">{tool.name}</code>
                </div>
              ))}
              {tools.length > 3 && (
                <p className="text-xs text-muted-foreground pl-5">
                  +{tools.length - 3} more
                </p>
              )}
            </div>
          )}

          {/* Last discovery time */}
          <div className="flex items-center justify-between text-xs text-muted-foreground pt-1 border-t">
            <span>{handlers.length} handlers</span>
            <div className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              <span>Discovered {formatRelativeTime(status?.lastDiscoveryTime)}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
