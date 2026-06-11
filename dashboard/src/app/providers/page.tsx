"use client";

import { useState, useMemo, useCallback } from "react";
import { LayoutGrid, List, Plus } from "lucide-react";
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
import { ProviderDialog } from "@/components/providers/provider-dialog";
import { SharedBadge } from "@/components/shared";
import { formatCost, formatTokens } from "@/lib/pricing";
import { useProviders, useSharedProviders, useProviderMetrics } from "@/hooks/resources";
import { useWorkspace } from "@/contexts/workspace-context";
import type { Provider } from "@/types";

/** Extended provider type with shared flag */
interface ProviderWithSource extends Provider {
  _isShared?: boolean;
}

type ProviderPhase = "Pending" | "Ready" | "Error" | "Failed";

type ViewMode = "cards" | "table";
type FilterPhase = "all" | ProviderPhase;
type ProviderRole = NonNullable<Provider["spec"]["role"]>;
type FilterRole = "all" | ProviderRole;

const ROLE_FILTERS: { value: FilterRole; label: string }[] = [
  { value: "all", label: "All roles" },
  { value: "llm", label: "LLM" },
  { value: "embedding", label: "Embedding" },
  { value: "tts", label: "TTS" },
  { value: "stt", label: "STT" },
  { value: "image", label: "Image" },
  { value: "inference", label: "Inference" },
];

// Pre-role Providers omit spec.role — treat them as llm for filtering and
// display so the migration is backwards-compatible.
function effectiveRole(provider: Provider): ProviderRole {
  return provider.spec?.role ?? "llm";
}

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
              <p className="text-muted-foreground">Role</p>
              <p className="font-medium capitalize">{effectiveRole(provider)}</p>
            </div>
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
            <TableHead>Role</TableHead>
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
                <Badge variant="outline" className="capitalize">
                  {effectiveRole(provider)}
                </Badge>
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
  const [filterRole, setFilterRole] = useState<FilterRole>("all");
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const { data: workspaceProviders, isLoading: isLoadingWorkspace, refetch } = useProviders();
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

  // Filter by role first, then phase, then sort.
  const roleFilteredProviders = useMemo(() => {
    if (filterRole === "all") return namespaceFilteredProviders;
    return namespaceFilteredProviders.filter((p) => effectiveRole(p) === filterRole);
  }, [namespaceFilteredProviders, filterRole]);

  const filteredProviders = useMemo(() => {
    const filtered = filterPhase === "all"
      ? roleFilteredProviders
      : roleFilteredProviders.filter((p) => p.status?.phase === filterPhase);
    // Sort alphabetically by name for stable ordering
    return [...filtered].sort((a, b) =>
      (a.metadata?.name || "").localeCompare(b.metadata?.name || "")
    );
  }, [roleFilteredProviders, filterPhase]);

  const phaseCounts = roleFilteredProviders.reduce(
    (acc, provider) => {
      const phase = provider.status?.phase;
      if (phase === "Ready") acc.ready++;
      else if (phase === "Error") acc.error++;
      return acc;
    },
    { ready: 0, error: 0 }
  );

  // Count providers per role within the current namespace selection so the
  // chip badges reflect what filtering each role would produce.
  const roleCounts = useMemo(() => {
    const counts: Record<FilterRole, number> = {
      all: namespaceFilteredProviders.length,
      llm: 0,
      embedding: 0,
      tts: 0,
      stt: 0,
      image: 0,
      inference: 0,
    };
    for (const p of namespaceFilteredProviders) {
      counts[effectiveRole(p)]++;
    }
    return counts;
  }, [namespaceFilteredProviders]);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Providers"
        description="Manage your LLM provider configurations"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3 flex-wrap">
            <Tabs
              value={filterPhase}
              onValueChange={(v) => setFilterPhase(v as FilterPhase)}
            >
              <TabsList>
                <TabsTrigger value="all">
                  All ({roleFilteredProviders.length})
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
            <fieldset className="flex items-center gap-1 border-0 p-0 m-0">
              <legend className="sr-only">Filter by role</legend>
              {ROLE_FILTERS.map((opt) => (
                <Button
                  key={opt.value}
                  type="button"
                  size="sm"
                  variant={filterRole === opt.value ? "default" : "outline"}
                  onClick={() => setFilterRole(opt.value)}
                  aria-pressed={filterRole === opt.value}
                >
                  {opt.label} ({roleCounts[opt.value]})
                </Button>
              ))}
            </fieldset>
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
            {canEdit && (
              <Button onClick={() => setCreateDialogOpen(true)}>
                <Plus className="h-4 w-4 mr-2" />
                Create Provider
              </Button>
            )}
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
              {canEdit
                ? "Create a provider to configure LLM access for your workspace."
                : "Create a Provider CRD in your cluster to configure LLM access."}
            </p>
            {canEdit && (
              <Button className="mt-4" onClick={() => setCreateDialogOpen(true)}>
                <Plus className="h-4 w-4 mr-2" />
                Create Provider
              </Button>
            )}
          </div>
        )}
      </div>

      <ProviderDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onSuccess={() => refetch()}
      />
    </div>
  );
}
