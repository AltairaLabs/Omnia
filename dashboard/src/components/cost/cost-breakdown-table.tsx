"use client";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatCost, formatTokens } from "@/lib/pricing";
import type { CostAllocationItem } from "@/lib/data/types";

interface CostBreakdownTableProps {
  data: CostAllocationItem[];
  title?: string;
  description?: string;
}

export function CostBreakdownTable({
  data,
  title = "Cost Breakdown by Agent",
  description = "Detailed cost allocation for each agent",
}: Readonly<CostBreakdownTableProps>) {
  // Sort by total cost descending
  const sortedData = [...data].sort((a, b) => b.totalCost - a.totalCost);

  // Calculate totals
  const totals = data.reduce(
    (acc, item) => ({
      inputCost: acc.inputCost + item.inputCost,
      outputCost: acc.outputCost + item.outputCost,
      cacheSavings: acc.cacheSavings + item.cacheSavings,
      totalCost: acc.totalCost + item.totalCost,
      tokens: acc.tokens + item.inputTokens + item.outputTokens,
      requests: acc.requests + item.requests,
    }),
    { inputCost: 0, outputCost: 0, cacheSavings: 0, totalCost: 0, tokens: 0, requests: 0 }
  );

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agent</TableHead>
              <TableHead>Provider</TableHead>
              <TableHead>Model</TableHead>
              <TableHead className="text-right">Tokens</TableHead>
              <TableHead className="text-right">Requests</TableHead>
              <TableHead className="text-right">Input Cost</TableHead>
              <TableHead className="text-right">Output Cost</TableHead>
              <TableHead className="text-right">Cache Savings</TableHead>
              <TableHead className="text-right">Total</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sortedData.map((item) => (
              <TableRow key={`${item.namespace}/${item.agent}`}>
                <TableCell>
                  <div className="flex flex-col">
                    <span className="font-medium">{item.agent}</span>
                    <span className="text-xs text-muted-foreground">{item.namespace}</span>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    variant="outline"
                    className={
                      item.provider === "anthropic"
                        ? "border-orange-500 text-orange-600 dark:text-orange-400"
                        : "border-green-500 text-green-600 dark:text-green-400"
                    }
                  >
                    {item.provider === "anthropic" ? "Anthropic" : "OpenAI"}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {item.model.includes("sonnet") && "Sonnet 4"}
                  {item.model.includes("opus") && "Opus 4"}
                  {item.model.includes("gpt-4-turbo") && "GPT-4 Turbo"}
                  {item.model.includes("gpt-3.5") && "GPT-3.5"}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatTokens(item.inputTokens + item.outputTokens)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {item.requests.toLocaleString()}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatCost(item.inputCost)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatCost(item.outputCost)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm text-green-600 dark:text-green-400">
                  {item.cacheSavings > 0 ? `-${formatCost(item.cacheSavings)}` : "-"}
                </TableCell>
                <TableCell className="text-right font-mono text-sm font-medium">
                  {formatCost(item.totalCost)}
                </TableCell>
              </TableRow>
            ))}
            {/* Totals row */}
            <TableRow className="bg-muted/50 font-medium">
              <TableCell colSpan={3}>Total (24h)</TableCell>
              <TableCell className="text-right font-mono">{formatTokens(totals.tokens)}</TableCell>
              <TableCell className="text-right font-mono">{totals.requests.toLocaleString()}</TableCell>
              <TableCell className="text-right font-mono">{formatCost(totals.inputCost)}</TableCell>
              <TableCell className="text-right font-mono">{formatCost(totals.outputCost)}</TableCell>
              <TableCell className="text-right font-mono text-green-600 dark:text-green-400">
                {totals.cacheSavings > 0 ? `-${formatCost(totals.cacheSavings)}` : "-"}
              </TableCell>
              <TableCell className="text-right font-mono">{formatCost(totals.totalCost)}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
