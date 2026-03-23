"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  MessageSquare,
  Wrench,
  Clock,
  Search,
  Coins,
  Bot,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { useAgents } from "@/hooks/agents";
import { useSessions, useSessionSearch } from "@/hooks/sessions";
import { useDebounce } from "@/hooks/core";
import type { SessionSummary, SessionListOptions, Session } from "@/types/session";
import { formatDistanceToNow } from "date-fns";

const PAGE_SIZE = 20;

/** Time range presets for filtering */
const TIME_RANGES: { label: string; value: string }[] = [
  { label: "All Time", value: "all" },
  { label: "Last 1h", value: "1h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

function getTimeRangeFrom(value: string): string | undefined {
  if (value === "all") return undefined;
  const now = Date.now();
  const ms: Record<string, number> = {
    "1h": 60 * 60 * 1000,
    "24h": 24 * 60 * 60 * 1000,
    "7d": 7 * 24 * 60 * 60 * 1000,
    "30d": 30 * 24 * 60 * 60 * 1000,
  };
  return new Date(now - (ms[value] || 0)).toISOString();
}

/** Deterministic color for a tag string, Grafana-style. */
const TAG_COLORS = [
  "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
  "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200",
  "bg-pink-100 text-pink-800 dark:bg-pink-900 dark:text-pink-200",
  "bg-cyan-100 text-cyan-800 dark:bg-cyan-900 dark:text-cyan-200",
  "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
  "bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200",
];

function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = ((hash << 5) - hash + tag.charCodeAt(i)) | 0;
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

function formatCost(cost?: number): string {
  if (cost == null || cost === 0) return "-";
  if (cost < 0.01) return `$${cost.toFixed(4)}`;
  return `$${cost.toFixed(2)}`;
}

function getStatusBadge(status: SessionSummary["status"]) {
  const variants: Record<Session["status"], { variant: "default" | "secondary" | "destructive" | "outline"; label: string }> = {
    active: { variant: "default", label: "Active" },
    completed: { variant: "secondary", label: "Completed" },
    error: { variant: "destructive", label: "Error" },
    expired: { variant: "outline", label: "Expired" },
  };
  const { variant, label } = variants[status];
  return <Badge variant={variant}>{label}</Badge>;
}

function StatsCardSkeleton() {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-4" />
          <Skeleton className="h-4 w-24" />
        </div>
        <Skeleton className="h-8 w-16 mt-1" />
      </CardContent>
    </Card>
  );
}

function TableRowSkeleton() {
  return (
    <TableRow>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      <TableCell><Skeleton className="h-4 w-20" /></TableCell>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-8 ml-auto" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-8 ml-auto" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-12 ml-auto" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-10 ml-auto" /></TableCell>
      <TableCell><Skeleton className="h-4 w-32" /></TableCell>
    </TableRow>
  );
}

export default function SessionsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [agentFilter, setAgentFilter] = useState<string>("all");
  const [timeRange, setTimeRange] = useState<string>("all");
  const [tagFilters, setTagFilters] = useState<Set<string>>(new Set());
  const [page, setPage] = useState(0);

  const debouncedSearch = useDebounce(search, 300);

  // Build list options
  const listOptions: SessionListOptions = useMemo(() => {
    const opts: SessionListOptions = {
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
      count: true,
    };
    if (statusFilter !== "all") opts.status = statusFilter as Session["status"];
    if (agentFilter !== "all") opts.agent = agentFilter;
    const from = getTimeRangeFrom(timeRange);
    if (from) opts.from = from;
    return opts;
  }, [statusFilter, agentFilter, timeRange, page]);

  // Use search hook when there's a search query, list hook otherwise
  const isSearching = debouncedSearch.length > 0;

  const listQuery = useSessions(listOptions);
  const searchQuery = useSessionSearch({
    ...listOptions,
    q: debouncedSearch,
  });

  const activeQuery = isSearching ? searchQuery : listQuery;
  const { data, isLoading, error } = activeQuery;

  // Reset to page 0 when filters change
  const resetPage = () => setPage(0);

  // Get unique agent names from the agents hook
  const agentsQuery = useAgents();
  const agentNames = useMemo(() => {
    if (!agentsQuery.data) return [];
    return [...new Set(agentsQuery.data.map((a) => a.metadata?.name).filter(Boolean))] as string[];
  }, [agentsQuery.data]);

  const addTagFilter = (tag: string) => {
    setTagFilters((prev) => new Set([...prev, tag]));
    resetPage();
  };
  const removeTagFilter = (tag: string) => {
    setTagFilters((prev) => {
      const next = new Set(prev);
      next.delete(tag);
      return next;
    });
    resetPage();
  };

  // Stats from current data — filter by tags client-side
  const allSessions = data?.sessions || [];
  const sessions = tagFilters.size > 0
    ? allSessions.filter((s) => {
        const sTags = s.tags || [];
        return [...tagFilters].every((t) => sTags.includes(t));
      })
    : allSessions;
  const stats = {
    total: data?.total || 0,
    active: sessions.filter((s) => s.status === "active").length,
    totalTokens: sessions.reduce((sum, s) => sum + s.totalTokens, 0),
    totalToolCalls: sessions.reduce((sum, s) => sum + s.toolCallCount, 0),
  };

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Sessions"
        description="Browse and replay agent conversations"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Stats */}
        <div className="grid grid-cols-4 gap-4">
          {isLoading ? (
            <>
              <StatsCardSkeleton />
              <StatsCardSkeleton />
              <StatsCardSkeleton />
              <StatsCardSkeleton />
            </>
          ) : (
            <>
              <Card>
                <CardContent className="pt-6">
                  <div className="flex items-center gap-2">
                    <MessageSquare className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">Total Sessions</span>
                  </div>
                  <p className="text-2xl font-bold mt-1">{stats.total}</p>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="pt-6">
                  <div className="flex items-center gap-2">
                    <Clock className="h-4 w-4 text-green-500" />
                    <span className="text-sm text-muted-foreground">Active Now</span>
                  </div>
                  <p className="text-2xl font-bold mt-1">{stats.active}</p>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="pt-6">
                  <div className="flex items-center gap-2">
                    <Coins className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">Total Tokens</span>
                  </div>
                  <p className="text-2xl font-bold mt-1">{stats.totalTokens.toLocaleString()}</p>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="pt-6">
                  <div className="flex items-center gap-2">
                    <Wrench className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">Tool Calls</span>
                  </div>
                  <p className="text-2xl font-bold mt-1">{stats.totalToolCalls}</p>
                </CardContent>
              </Card>
            </>
          )}
        </div>

        {/* Error state */}
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading sessions</AlertTitle>
            <AlertDescription>
              {error instanceof Error ? error.message : "An unexpected error occurred"}
            </AlertDescription>
          </Alert>
        )}

        {/* Filters */}
        <div className="flex items-center gap-4">
          <div className="relative flex-1 max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search sessions..."
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                resetPage();
              }}
              className="pl-9"
            />
          </div>
          <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); resetPage(); }}>
            <SelectTrigger className="w-[150px]">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Status</SelectItem>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="completed">Completed</SelectItem>
              <SelectItem value="error">Error</SelectItem>
              <SelectItem value="expired">Expired</SelectItem>
            </SelectContent>
          </Select>
          <Select value={agentFilter} onValueChange={(v) => { setAgentFilter(v); resetPage(); }}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Agent" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Agents</SelectItem>
              {agentNames.map((agent) => (
                <SelectItem key={agent} value={agent}>
                  {agent}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={timeRange} onValueChange={(v) => { setTimeRange(v); resetPage(); }}>
            <SelectTrigger className="w-[150px]">
              <SelectValue placeholder="Time Range" />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGES.map((range) => (
                <SelectItem key={range.value} value={range.value}>
                  {range.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Active tag filters */}
        {tagFilters.size > 0 && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm text-muted-foreground">Filtering by:</span>
            {[...tagFilters].map((tag) => (
              <span
                key={tag}
                className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${tagColor(tag)}`}
              >
                {tag}
                <button
                  onClick={() => removeTagFilter(tag)}
                  className="ml-0.5 hover:opacity-70"
                  aria-label={`Remove ${tag} filter`}
                >
                  &times;
                </button>
              </span>
            ))}
            <button
              onClick={() => { setTagFilters(new Set()); resetPage(); }}
              className="text-xs text-muted-foreground hover:text-foreground underline"
            >
              Clear all
            </button>
          </div>
        )}

        {/* Sessions Table */}
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Session ID</TableHead>
                <TableHead>Agent</TableHead>
                <TableHead>Tags</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Started</TableHead>
                <TableHead className="text-right">Messages</TableHead>
                <TableHead className="text-right">Tools</TableHead>
                <TableHead className="text-right">Tokens</TableHead>
                <TableHead className="text-right">Cost</TableHead>
                <TableHead>Last Message</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading && (
                <>
                  <TableRowSkeleton />
                  <TableRowSkeleton />
                  <TableRowSkeleton />
                  <TableRowSkeleton />
                  <TableRowSkeleton />
                </>
              )}
              {!isLoading && sessions.length === 0 && (
                <TableRow>
                  <TableCell colSpan={10} className="text-center py-8 text-muted-foreground">
                    No sessions found
                  </TableCell>
                </TableRow>
              )}
              {!isLoading && sessions.map((session) => (
                <TableRow
                  key={session.id}
                  className="cursor-pointer"
                  onClick={() => router.push(`/sessions/${session.id}`)}
                >
                  <TableCell className="font-mono text-sm">{session.id}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Bot className="h-4 w-4 text-muted-foreground" />
                      <span>{session.agentName}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {session.tags?.map((tag) => (
                        <button
                          key={tag}
                          onClick={(e) => {
                            e.stopPropagation();
                            addTagFilter(tag);
                          }}
                          className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium cursor-pointer hover:opacity-80 transition-opacity ${tagColor(tag)}`}
                        >
                          {tag}
                        </button>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{getStatusBadge(session.status)}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDistanceToNow(new Date(session.startedAt), { addSuffix: true })}
                  </TableCell>
                  <TableCell className="text-right">{session.messageCount}</TableCell>
                  <TableCell className="text-right">{session.toolCallCount}</TableCell>
                  <TableCell className="text-right">{session.totalTokens.toLocaleString()}</TableCell>
                  <TableCell className="text-right text-muted-foreground">{formatCost(session.estimatedCost)}</TableCell>
                  <TableCell className="max-w-[200px] truncate text-muted-foreground text-sm">
                    {session.lastMessage}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>

        {/* Pagination */}
        {!isLoading && (data?.total || 0) > 0 && (
          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">
              Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, data?.total || 0)} of {data?.total || 0} sessions
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0}
              >
                <ChevronLeft className="h-4 w-4 mr-1" />
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => p + 1)}
                disabled={!data?.hasMore}
              >
                Next
                <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
