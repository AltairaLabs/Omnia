"use client";

import { useState, useMemo, useCallback } from "react";
import { LayoutGrid, List } from "lucide-react";
import Link from "next/link";
import { Header } from "@/components/layout";
import { NamespaceFilter } from "@/components/filters";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { CostSparkline } from "@/components/cost";
import { ProviderStatusBadge } from "@/components/providers/provider-status-badge";
import { ProviderTypeIcon } from "@/components/providers/provider-type-icon";
import { SharedBadge } from "@/components/shared";
import { formatCost } from "@/lib/pricing";
import { formatTokens } from "@/lib/utils";
import { useProviders, useSharedProviders } from "@/hooks";
import { useProviderMetrics } from "@/hooks/use-provider-metrics";
import type { Provider } from "@/types";

/** Extended provider type with shared flag */
interface ProviderWithSource extends Provider {
  _isShared?: boolean;
}

type ProviderPhase = "Pending" | "Ready" | "Error" | "Failed";

type ViewMode = "cards" | "table";
type FilterPhase = "all" | ProviderPhase;

/** Color mapping for provider types */
const providerColorMap: Record<string, string> = {
  anthropic: "#F97316", // orange
  openai: "#22C55E",    // green
  gemini: "#3B82F6",    // blue
  ollama: "#A855F7",    // purple
  bedrock: "#EAB308",   // yellow
  mock: "#6B7280",      // gray
};

function ProviderCard({ provider }: Readonly<{ provider: ProviderWithSource }>) {
  const { metadata, spec, status, _isShared } = provider;

  // Fetch metrics for this provider
  const { data: metrics } = useProviderMetrics(metadata?.name || "", spec?.type);

  // Get sparkline color based on provider type
  const sparklineColor = spec?.type ? providerColorMap[spec.type] || "#3B82F6" : "#3B82F6";

  // Convert cost rate data for sparkline (uses { value } format)
  const sparklineData = metrics?.costRate?.map(p => ({ value: p.value })) || [];

  return (
    <Link href={`/providers/${metadata?.name}?namespace=${metadata?.namespace || "default"}`}>
      <Card className="cursor-pointer hover:border-primary/50 transition-colors h-full">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-3">
            <ProviderTypeIcon type={spec?.type} />
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <CardTitle className="text-lg truncate">{metadata?.name}</CardTitle>
                {_isShared && <SharedBadge />}
              </div>
              <p className="text-sm text-muted-foreground">{metadata?.namespace}</p>
            </div>
            <ProviderStatusBadge phase={status?.phase} />
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Cost Sparkline */}
          <div className="space-y-1">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Cost (24h)</span>
              <span className="font-medium">{formatCost(metrics?.totalCost24h || 0)}</span>
            </div>
            <CostSparkline data={sparklineData} color={sparklineColor} height={28} />
          </div>

          {/* Stats Grid */}
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <p className="text-muted-foreground">Type</p>
              <p className="font-medium capitalize">{spec?.type || "-"}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Model</p>
              <p className="font-medium truncate" title={spec?.model}>
                {spec?.model || "-"}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground">Requests (24h)</p>
              <p className="font-medium">
                {metrics?.totalRequests24h ? Math.round(metrics.totalRequests24h).toLocaleString() : "-"}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground">Tokens (24h)</p>
              <p className="font-medium">
                {metrics?.totalTokens24h ? formatTokens(metrics.totalTokens24h) : "-"}
              </p>
            </div>
          </div>

          {/* Capabilities */}
          {spec?.capabilities && spec.capabilities.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {spec.capabilities.map((cap) => (
                <Badge key={cap} variant="outline" className="text-xs">{cap}</Badge>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </Link>
  );
}


function ProviderTable({ providers }: Readonly<{ providers: ProviderWithSource[] }>) {
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Namespace</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Model</TableHead>
            <TableHead>Base URL</TableHead>
            <TableHead>Capabilities</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {providers.map((provider) => (
            <TableRow key={provider.metadata?.uid}>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Link
                    href={`/providers/${provider.metadata?.name}?namespace=${provider.metadata?.namespace || "default"}`}
                    className="font-medium hover:underline"
                  >
                    {provider.metadata?.name}
                  </Link>
                  {provider._isShared && <SharedBadge />}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {provider.metadata?.namespace}
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <ProviderTypeIcon type={provider.spec?.type} />
                  <span className="capitalize">{provider.spec?.type || "-"}</span>
                </div>
              </TableCell>
              <TableCell className="max-w-[200px] truncate" title={provider.spec?.model}>
                {provider.spec?.model || "-"}
              </TableCell>
              <TableCell className="max-w-[200px] truncate" title={provider.spec?.baseURL}>
                {provider.spec?.baseURL || "-"}
              </TableCell>
              <TableCell>
                {provider.spec?.capabilities && provider.spec.capabilities.length > 0
                  ? provider.spec.capabilities.join(", ")
                  : "-"}
              </TableCell>
              <TableCell>
                <ProviderStatusBadge phase={provider.status?.phase} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function renderLoadingSkeleton(viewMode: ViewMode) {
  if (viewMode === "cards") {
    return (
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {["sk-1", "sk-2", "sk-3", "sk-4", "sk-5", "sk-6"].map((id) => (
          <Skeleton key={id} className="h-[160px] rounded-lg" />
        ))}
      </div>
    );
  }
  return <Skeleton className="h-[400px] rounded-lg" />;
}

export default function ProvidersPage() {
  const [viewMode, setViewMode] = useState<ViewMode>("cards");
  const [filterPhase, setFilterPhase] = useState<FilterPhase>("all");
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);

  const { data: workspaceProviders, isLoading: isLoadingWorkspace } = useProviders();
  const { data: sharedProviders, isLoading: isLoadingShared } = useSharedProviders();

  const isLoading = isLoadingWorkspace || isLoadingShared;

  // Combine shared and workspace providers, marking shared ones
  const providers = useMemo((): ProviderWithSource[] => {
    const shared: ProviderWithSource[] = (sharedProviders || []).map((p) => ({
      ...p,
      _isShared: true,
    }));
    const workspace: ProviderWithSource[] = (workspaceProviders || []).map((p) => ({
      ...p,
      _isShared: false,
    }));
    return [...shared, ...workspace];
  }, [sharedProviders, workspaceProviders]);

  // Extract unique namespaces
  const allNamespaces = useMemo(() => {
    if (!providers) return [];
    return [...new Set(providers.map((p) => p.metadata?.namespace).filter((ns): ns is string => !!ns))];
  }, [providers]);

  // Initialize selected namespaces when data loads
  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Filter by namespace first, then by phase
  const namespaceFilteredProviders = useMemo(() => {
    if (!providers) return [];
    if (selectedNamespaces.length === 0) return providers;
    return providers.filter((p) => p.metadata?.namespace && selectedNamespaces.includes(p.metadata.namespace));
  }, [providers, selectedNamespaces]);

  // Filter by phase and sort alphabetically by name
  const filteredProviders = useMemo(() => {
    const filtered = filterPhase === "all"
      ? namespaceFilteredProviders
      : namespaceFilteredProviders.filter((p) => p.status?.phase === filterPhase);
    // Sort alphabetically by name for stable ordering
    return [...filtered].sort((a, b) =>
      (a.metadata?.name || "").localeCompare(b.metadata?.name || "")
    );
  }, [namespaceFilteredProviders, filterPhase]);

  const phaseCounts = namespaceFilteredProviders.reduce(
    (acc, provider) => {
      const phase = provider.status?.phase;
      if (phase === "Ready") acc.ready++;
      else if (phase === "Error") acc.error++;
      return acc;
    },
    { ready: 0, error: 0 }
  );

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Providers"
        description="Manage your LLM provider configurations"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Tabs
              value={filterPhase}
              onValueChange={(v) => setFilterPhase(v as FilterPhase)}
            >
              <TabsList>
                <TabsTrigger value="all">
                  All ({namespaceFilteredProviders.length})
                </TabsTrigger>
                <TabsTrigger value="Ready">
                  Ready ({phaseCounts?.ready ?? 0})
                </TabsTrigger>
                <TabsTrigger value="Error">
                  Error ({phaseCounts?.error ?? 0})
                </TabsTrigger>
              </TabsList>
            </Tabs>
            <NamespaceFilter
              namespaces={allNamespaces}
              selectedNamespaces={selectedNamespaces}
              onSelectionChange={handleNamespaceChange}
            />
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center rounded-md border bg-muted p-1">
              <Button
                variant={viewMode === "cards" ? "secondary" : "ghost"}
                size="icon"
                className="h-7 w-7"
                onClick={() => setViewMode("cards")}
              >
                <LayoutGrid className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === "table" ? "secondary" : "ghost"}
                size="icon"
                className="h-7 w-7"
                onClick={() => setViewMode("table")}
              >
                <List className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        {/* Content */}
        {isLoading && renderLoadingSkeleton(viewMode)}
        {!isLoading && viewMode === "cards" && (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {filteredProviders?.map((provider) => (
              <ProviderCard key={provider.metadata?.uid} provider={provider} />
            ))}
          </div>
        )}
        {!isLoading && viewMode !== "cards" && (
          <ProviderTable providers={filteredProviders ?? []} />
        )}

        {!isLoading && filteredProviders?.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">No providers found</p>
            <p className="text-sm text-muted-foreground mt-2">
              Create a Provider CRD in your cluster to configure LLM access
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
