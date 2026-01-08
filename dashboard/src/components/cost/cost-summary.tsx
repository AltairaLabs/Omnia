"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DollarSign, ArrowDownIcon, ArrowUpIcon, Coins } from "lucide-react";
import { calculateCost, formatCost, formatTokens, getModelPricing } from "@/lib/pricing";
import { cn } from "@/lib/utils";

interface CostSummaryProps {
  inputTokens: number;
  outputTokens: number;
  cacheHits?: number;
  model: string;
  period?: string;
  previousPeriodCost?: number;
  className?: string;
}

export function CostSummary({
  inputTokens,
  outputTokens,
  cacheHits = 0,
  model,
  period = "24h",
  previousPeriodCost,
  className,
}: Readonly<CostSummaryProps>) {
  const cost = calculateCost(model, inputTokens, outputTokens, cacheHits);
  const pricing = getModelPricing(model);

  // Calculate change from previous period
  const costChange = previousPeriodCost
    ? ((cost - previousPeriodCost) / previousPeriodCost) * 100
    : null;

  return (
    <div className={cn("grid gap-4 md:grid-cols-2 lg:grid-cols-4", className)}>
      {/* Estimated Cost */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Estimated Cost</CardTitle>
          <DollarSign className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{formatCost(cost)}</div>
          <p className="text-xs text-muted-foreground">
            {period} • {pricing?.displayName || model}
          </p>
          {costChange !== null && (
            <div
              className={cn(
                "flex items-center text-xs mt-1",
                costChange > 0 ? "text-destructive" : "text-green-600"
              )}
            >
              {costChange > 0 ? (
                <ArrowUpIcon className="h-3 w-3 mr-1" />
              ) : (
                <ArrowDownIcon className="h-3 w-3 mr-1" />
              )}
              {Math.abs(costChange).toFixed(1)}% from previous period
            </div>
          )}
        </CardContent>
      </Card>

      {/* Total Tokens */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Total Tokens</CardTitle>
          <Coins className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">
            {formatTokens(inputTokens + outputTokens)}
          </div>
          <p className="text-xs text-muted-foreground">{period}</p>
        </CardContent>
      </Card>

      {/* Input Tokens */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Input Tokens</CardTitle>
          <ArrowDownIcon className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{formatTokens(inputTokens)}</div>
          <p className="text-xs text-muted-foreground">
            {pricing
              ? `$${pricing.inputPer1M.toFixed(2)}/1M tokens`
              : "Unknown pricing"}
          </p>
        </CardContent>
      </Card>

      {/* Output Tokens */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Output Tokens</CardTitle>
          <ArrowUpIcon className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{formatTokens(outputTokens)}</div>
          <p className="text-xs text-muted-foreground">
            {pricing
              ? `$${pricing.outputPer1M.toFixed(2)}/1M tokens`
              : "Unknown pricing"}
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
