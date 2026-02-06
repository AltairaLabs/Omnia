"use client";

import { use, useCallback, useMemo } from "react";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Activity, Settings, Zap, DollarSign, FileText, AlertCircle, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Header } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { YamlBlock } from "@/components/ui/yaml-block";
import { MetricSparklineCard } from "@/components/ui/sparkline";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { ProviderStatusBadge } from "@/components/providers/provider-status-badge";
import { ProviderTypeIcon } from "@/components/providers/provider-type-icon";
import { useProvider, useUpdateProviderSecretRef, useSecrets } from "@/hooks";
import { useProviderMetrics } from "@/hooks/use-provider-metrics";

interface ProviderDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

function formatNumber(value: number, decimals = 2): string {
  if (value >= 1000000) {
    return `${(value / 1000000).toFixed(decimals)}M`;
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(decimals)}K`;
  }
  return value.toFixed(decimals);
}

export default function ProviderDetailPage({ params }: Readonly<ProviderDetailPageProps>) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const namespace = searchParams.get("namespace") || "default";
  const currentTab = searchParams.get("tab") || "overview";

  const { data: provider, isLoading } = useProvider(name, namespace);
  const { data: metrics, isLoading: metricsLoading } = useProviderMetrics(name, provider?.spec?.type);
  const { data: secrets, isLoading: secretsLoading } = useSecrets({ namespace });
  const updateSecretRef = useUpdateProviderSecretRef();

  // Determine current secret status
  const currentSecretRef = provider?.spec?.secretRef?.name;
  const secretExists = useMemo(() => {
    if (!currentSecretRef) return true; // No secret configured
    if (!secrets) return undefined; // Still loading
    return secrets.some((s) => s.name === currentSecretRef);
  }, [currentSecretRef, secrets]);

  // Handle secret selection change
  const handleSecretChange = useCallback(
    async (value: string) => {
      if (!provider) return;
      const providerName = provider.metadata?.name;
      const providerNamespace = provider.metadata?.namespace;
      if (!providerName || !providerNamespace) return;

      const newSecretRef = value === "__none__" ? null : value;

      try {
        await updateSecretRef.mutateAsync({
          namespace: providerNamespace,
          name: providerName,
          secretRef: newSecretRef,
        });
      } catch (error) {
        console.error("Failed to update provider:", error);
      }
    },
    [provider, updateSecretRef]
  );

  const handleTabChange = useCallback((tab: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", tab);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }, [searchParams, router, pathname]);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Provider Details" />
        <div className="flex-1 p-6 space-y-6">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-[400px] rounded-lg" />
        </div>
      </div>
    );
  }

  if (!provider) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Provider Not Found" />
        <div className="flex-1 p-6">
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">
              Provider &quot;{name}&quot; not found in namespace &quot;{namespace}&quot;
            </p>
            <Link href="/providers">
              <Button variant="outline" className="mt-4">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Providers
              </Button>
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const { metadata, spec, status } = provider;

  return (
    <div className="flex flex-col h-full">
      <Header
        title={metadata?.name || "Provider"}
        description={`${metadata?.namespace || "default"} namespace`}
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Back link and status */}
        <div className="flex items-center justify-between">
          <Link href="/providers">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Providers
            </Button>
          </Link>
          <ProviderStatusBadge phase={status?.phase} />
        </div>

        {/* Tabs */}
        <Tabs value={currentTab} onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="usage" className="gap-1.5">
              <Activity className="h-4 w-4" />
              Usage
            </TabsTrigger>
            <TabsTrigger value="config" className="gap-1.5">
              <Settings className="h-4 w-4" />
              Configuration
            </TabsTrigger>
          </TabsList>

          {/* Overview Tab */}
          <TabsContent value="overview" className="space-y-4 mt-4">
            <div className="grid md:grid-cols-2 gap-4">
              {/* Provider Info Card */}
              <Card>
                <CardHeader>
                  <div className="flex items-center gap-4">
                    <ProviderTypeIcon type={spec?.type} size="lg" />
                    <div>
                      <CardTitle>{metadata?.name}</CardTitle>
                      <CardDescription>
                        {spec?.type ? `${spec.type.charAt(0).toUpperCase()}${spec.type.slice(1)} Provider` : "LLM Provider"}
                      </CardDescription>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Type</span>
                    <span className="font-medium capitalize">{spec?.type || "-"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Model</span>
                    <span className="font-medium">{spec?.model || "-"}</span>
                  </div>
                  {spec?.baseURL && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Base URL</span>
                      <span className="font-medium text-sm truncate max-w-[200px]" title={spec.baseURL}>
                        {spec.baseURL}
                      </span>
                    </div>
                  )}
                  {spec?.capabilities && spec.capabilities.length > 0 && (
                    <div className="flex justify-between items-start">
                      <span className="text-muted-foreground">Capabilities</span>
                      <div className="flex flex-wrap gap-1 justify-end">
                        {spec.capabilities.map((cap) => (
                          <Badge key={cap} variant="secondary" className="text-xs">{cap}</Badge>
                        ))}
                      </div>
                    </div>
                  )}
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Created</span>
                    <span className="font-medium text-sm">
                      {formatDate(metadata?.creationTimestamp)}
                    </span>
                  </div>
                </CardContent>
              </Card>

              {/* Credentials Card */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <FileText className="h-5 w-5" />
                    Credentials
                  </CardTitle>
                  <CardDescription>
                    API credentials for this provider
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="secret-select" className="text-sm text-muted-foreground">Secret</Label>
                    {secretsLoading ? (
                      <Skeleton className="h-10 w-full" />
                    ) : (
                      <div className="flex items-center gap-2">
                        <Select
                          value={currentSecretRef || "__none__"}
                          onValueChange={handleSecretChange}
                          disabled={updateSecretRef.isPending}
                        >
                          <SelectTrigger id="secret-select" className="w-full">
                            {updateSecretRef.isPending ? (
                              <div className="flex items-center gap-2">
                                <Loader2 className="h-4 w-4 animate-spin" />
                                <span>Updating...</span>
                              </div>
                            ) : (
                              <SelectValue placeholder="Select a secret" />
                            )}
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="__none__">
                              <span className="text-muted-foreground">None (no credentials)</span>
                            </SelectItem>
                            {/* Show missing secret if configured but doesn't exist */}
                            {currentSecretRef && secretExists === false && (
                              <SelectItem value={currentSecretRef} disabled>
                                <div className="flex items-center gap-2 text-destructive">
                                  <AlertCircle className="h-4 w-4" />
                                  <span>{currentSecretRef} (missing)</span>
                                </div>
                              </SelectItem>
                            )}
                            {/* Available secrets */}
                            {secrets?.map((secret) => (
                              <SelectItem key={secret.name} value={secret.name}>
                                <div className="flex items-center gap-2">
                                  <span>{secret.name}</span>
                                  {secret.annotations?.["omnia.altairalabs.ai/provider"] && (
                                    <span className="text-xs text-muted-foreground">
                                      ({secret.annotations["omnia.altairalabs.ai/provider"]})
                                    </span>
                                  )}
                                </div>
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        {/* Warning if secret is missing */}
                        {currentSecretRef && secretExists === false && (
                          <div className="text-destructive" title="Secret not found">
                            <AlertCircle className="h-5 w-5" />
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  {spec?.secretRef?.key && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Key</span>
                      <span className="font-medium">{spec.secretRef.key}</span>
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* Defaults Card */}
              {spec?.defaults && (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Settings className="h-5 w-5" />
                      Defaults
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    {spec.defaults.temperature && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Temperature</span>
                        <span className="font-medium">{spec.defaults.temperature}</span>
                      </div>
                    )}
                    {spec.defaults.topP && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Top P</span>
                        <span className="font-medium">{spec.defaults.topP}</span>
                      </div>
                    )}
                    {spec.defaults.maxTokens && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Max Tokens</span>
                        <span className="font-medium">{spec.defaults.maxTokens}</span>
                      </div>
                    )}
                    {spec.defaults.contextWindow && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Context Window</span>
                        <span className="font-medium">{spec.defaults.contextWindow.toLocaleString()}</span>
                      </div>
                    )}
                    {spec.defaults.truncationStrategy && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Truncation</span>
                        <span className="font-medium capitalize">{spec.defaults.truncationStrategy}</span>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}

              {/* Pricing Card */}
              {spec?.pricing && (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <DollarSign className="h-5 w-5" />
                      Pricing
                    </CardTitle>
                    <CardDescription>Cost per 1K tokens</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    {spec.pricing.inputCostPer1K && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Input</span>
                        <span className="font-medium">${spec.pricing.inputCostPer1K}</span>
                      </div>
                    )}
                    {spec.pricing.outputCostPer1K && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Output</span>
                        <span className="font-medium">${spec.pricing.outputCostPer1K}</span>
                      </div>
                    )}
                    {spec.pricing.cachedCostPer1K && (
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Cached</span>
                        <span className="font-medium">${spec.pricing.cachedCostPer1K}</span>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}
            </div>

            {/* Conditions */}
            {status?.conditions && status.conditions.length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle>Conditions</CardTitle>
                </CardHeader>
                <CardContent>
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-left text-muted-foreground border-b">
                        <th className="pb-2 font-medium">Type</th>
                        <th className="pb-2 font-medium">Status</th>
                        <th className="pb-2 font-medium">Reason</th>
                        <th className="pb-2 font-medium">Message</th>
                      </tr>
                    </thead>
                    <tbody>
                      {status.conditions.map((condition) => (
                        <tr key={condition.type} className="border-b last:border-0">
                          <td className="py-2 pr-4 font-medium">{condition.type}</td>
                          <td className="py-2 pr-4">
                            <span
                              className={`px-2 py-0.5 rounded text-xs font-medium ${
                                condition.status === "True"
                                  ? "bg-green-500/15 text-green-700 dark:text-green-400"
                                  : "bg-red-500/15 text-red-700 dark:text-red-400"
                              }`}
                            >
                              {condition.status}
                            </span>
                          </td>
                          <td className="py-2 pr-4">{condition.reason || "-"}</td>
                          <td className="py-2 text-muted-foreground">{condition.message || "-"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </CardContent>
              </Card>
            )}
          </TabsContent>

          {/* Usage Tab */}
          <TabsContent value="usage" className="space-y-4 mt-4">
            {metricsLoading ? (
              <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
                {[1, 2, 3, 4].map((i) => (
                  <Skeleton key={i} className="h-[100px] rounded-lg" />
                ))}
              </div>
            ) : (
              <>
                {/* Metrics Cards with Sparklines */}
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
                  <MetricSparklineCard
                    title="Request Rate"
                    value={formatNumber(metrics?.currentRequestRate ?? 0, 2)}
                    unit="req/s"
                    data={metrics?.requestRate ?? []}
                    color="blue"
                  />
                  <MetricSparklineCard
                    title="Input Tokens"
                    value={formatNumber(metrics?.currentInputTokenRate ?? 0, 0)}
                    unit="tok/s"
                    data={metrics?.inputTokenRate ?? []}
                    color="green"
                  />
                  <MetricSparklineCard
                    title="Output Tokens"
                    value={formatNumber(metrics?.currentOutputTokenRate ?? 0, 0)}
                    unit="tok/s"
                    data={metrics?.outputTokenRate ?? []}
                    color="orange"
                  />
                  <MetricSparklineCard
                    title="Cost (24h)"
                    value={`$${formatNumber(metrics?.totalCost24h ?? 0, 2)}`}
                    data={metrics?.costRate ?? []}
                    color="default"
                  />
                </div>

                {/* Summary Stats */}
                <Card>
                  <CardHeader>
                    <CardTitle>24 Hour Summary</CardTitle>
                    <CardDescription>Aggregate usage statistics for the last 24 hours</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="grid grid-cols-3 gap-6">
                      <div>
                        <p className="text-sm text-muted-foreground">Total Requests</p>
                        <p className="text-2xl font-bold">
                          {formatNumber(metrics?.totalRequests24h ?? 0, 0)}
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">Total Tokens</p>
                        <p className="text-2xl font-bold">
                          {formatNumber(metrics?.totalTokens24h ?? 0, 0)}
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">Total Cost</p>
                        <p className="text-2xl font-bold">
                          ${formatNumber(metrics?.totalCost24h ?? 0, 2)}
                        </p>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                {(metrics?.requestRate?.length === 0) && (
                  <Card>
                    <CardContent className="py-8">
                      <div className="flex flex-col items-center justify-center text-center">
                        <Zap className="h-8 w-8 text-muted-foreground mb-2" />
                        <p className="text-muted-foreground">
                          No usage data available for this provider yet. Data will appear once agents start using this provider.
                        </p>
                      </div>
                    </CardContent>
                  </Card>
                )}
              </>
            )}
          </TabsContent>

          {/* Configuration Tab */}
          <TabsContent value="config" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Full Configuration</CardTitle>
                <CardDescription>Complete provider specification in YAML format</CardDescription>
              </CardHeader>
              <CardContent>
                <YamlBlock data={provider} className="max-h-[600px]" />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
