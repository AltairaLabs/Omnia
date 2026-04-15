"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Markdown } from "@/components/ui/markdown";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";
import {
  ArrowLeft,
  Bot,
  Brain,
  User,
  Wrench,
  Clock,
  Coins,
  Download,
  Copy,
  MessageSquare,
  AlertCircle,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Shield,
  Play,
} from "lucide-react";
import { useSessionDetail, useSessionAllMessages, useSessionEvalResults, useSessionToolCalls, useSessionProviderCalls, useSessionRuntimeEvents } from "@/hooks/sessions";
import { MemorySidebar } from "@/components/memories/memory-sidebar";
import type { Message, Session, ToolCall, ProviderCall, RuntimeEvent, EvalResult } from "@/types";
import { EvalResultsBadge } from "@/components/sessions/eval-results-badge";
import { ToolCallBadge } from "@/components/sessions/tool-call-badge";
import { ReplayTab } from "@/components/sessions/replay";
import { DebugPanel } from "@/components/sessions/debug-panel";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { collapseToolCalls } from "@/lib/sessions/collapse-tool-calls";
import { format as formatDate, formatDistanceToNow } from "date-fns";
import { cn } from "@/lib/utils";
import {
  useGrafana,
  buildSessionDashboardUrl,
} from "@/hooks/logs";

function getStatusBadge(status: Session["status"]) {
  const variants: Record<Session["status"], { variant: "default" | "secondary" | "destructive" | "outline"; label: string }> = {
    active: { variant: "default", label: "Active" },
    completed: { variant: "secondary", label: "Completed" },
    error: { variant: "destructive", label: "Error" },
    expired: { variant: "outline", label: "Expired" },
  };
  const { variant, label } = variants[status];
  return <Badge variant={variant}>{label}</Badge>;
}

/** Inline tool call indicator rendered from a first-class ToolCall record. */
function ToolCallIndicator({ toolCall }: Readonly<{ toolCall: ToolCall }>) {
  const openToolCall = useDebugPanelStore((s) => s.openToolCall);

  return (
    <div className="flex gap-3">
      <div className="flex items-center justify-center h-8 w-8 rounded-full shrink-0 bg-orange-500/10">
        <Wrench className="h-4 w-4 text-orange-500" />
      </div>
      <button
        className="flex items-center gap-2 border rounded-lg bg-muted/30 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors"
        onClick={() => openToolCall(toolCall.callId || toolCall.id)}
      >
        <span className="font-mono text-sm font-medium truncate">{toolCall.name}</span>
        {toolCall.status && <ToolCallBadge status={toolCall.status} />}
        {toolCall.durationMs !== undefined && (
          <span className="text-xs text-muted-foreground">{toolCall.durationMs}ms</span>
        )}
        <span className="text-xs text-muted-foreground ml-auto shrink-0">View details &gt;</span>
      </button>
    </div>
  );
}

/**
 * Get avatar background class based on message role.
 */
function getAvatarClassName(isUser: boolean, isAssistant: boolean): string {
  if (isUser) return "bg-primary text-primary-foreground";
  if (isAssistant) return "bg-blue-500 text-white";
  return "bg-gray-500 text-white";
}

/**
 * Get icon component based on message role.
 */
function getMessageIcon(isUser: boolean, isAssistant: boolean) {
  if (isUser) return <User className="h-4 w-4" />;
  if (isAssistant) return <Bot className="h-4 w-4" />;
  return <MessageSquare className="h-4 w-4" />;
}

/**
 * Get bubble background class based on message role.
 */
function getBubbleClassName(isUser: boolean, isSystem: boolean): string {
  if (isUser) return "bg-primary text-primary-foreground";
  if (isSystem) return "bg-muted border";
  return "bg-muted";
}

function MessageBubble({ message, showTimestamp, evalResults }: Readonly<{ message: Message; showTimestamp?: boolean; evalResults?: EvalResult[] }>) {
  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";
  const isSystem = message.role === "system";

  return (
    <div className={cn("flex gap-3", isUser ? "flex-row-reverse" : "flex-row")}>
      <div
        className={cn(
          "flex items-center justify-center h-8 w-8 rounded-full shrink-0",
          getAvatarClassName(isUser, isAssistant)
        )}
      >
        {getMessageIcon(isUser, isAssistant)}
      </div>

      <div className={cn("flex flex-col max-w-[80%]", isUser ? "items-end" : "items-start")}>
        <div
          className={cn(
            "rounded-lg px-4 py-2",
            getBubbleClassName(isUser, isSystem)
          )}
        >
          <Markdown content={message.content} className={cn("text-sm", isUser && "prose-invert")} />
        </div>

        {/* Eval results from eval_results table */}
        {evalResults && evalResults.length > 0 && (
          <EvalResultsBadge results={evalResults} />
        )}

        {/* Timestamp and tokens */}
        <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
          {showTimestamp && (
            <span>{formatDate(new Date(message.timestamp), "HH:mm:ss")}</span>
          )}
          {message.tokens && (
            <span className="flex items-center gap-1">
              <Coins className="h-3 w-3" />
              {message.tokens.input || message.tokens.output} tokens
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Returns true if a message belongs in the conversation view.
 *
 * Conversation messages are: user/assistant messages without a metadata.type.
 * Everything else (pipeline events, eval events, provider calls, tool events, etc.) goes to the Debug panel.
 */
function isConversationMessage(m: Message): boolean {
  if (m.role === "tool") return false;
  if (m.metadata?.type) return false;
  if (m.metadata?.source === "runtime") return false;
  return true;
}

const INITIAL_MESSAGE_WINDOW = 50;

/**
 * Renders the conversation message list with eval results grouped by message.
 * Uses windowed rendering for the visible portion, plus server-side pagination
 * via "Load more" to fetch additional pages from the API.
 */
/** A unified item for interleaving messages and tool calls by timestamp. */
type ConversationItem =
  | { kind: "message"; message: Message; id: string; timestamp: string }
  | { kind: "toolCall"; toolCall: ToolCall; id: string; timestamp: string };

function ConversationMessages({
  messages,
  evalResults,
  toolCalls,
  hasMore,
  isFetchingMore,
  onLoadMore,
}: Readonly<{
  messages: Message[];
  evalResults: EvalResult[];
  toolCalls: ToolCall[];
  hasMore?: boolean;
  isFetchingMore?: boolean;
  onLoadMore?: () => void;
}>) {
  const [visibleCount, setVisibleCount] = useState(INITIAL_MESSAGE_WINDOW);
  const evalsByMessage = groupEvalResultsByMessageId(evalResults);

  // Merge messages and tool calls into a single timeline
  const items: ConversationItem[] = [];
  for (const m of messages.filter(isConversationMessage)) {
    items.push({ kind: "message", message: m, id: m.id, timestamp: m.timestamp });
  }
  for (const tc of toolCalls) {
    items.push({ kind: "toolCall", toolCall: tc, id: `tc-${tc.id}`, timestamp: tc.createdAt });
  }
  items.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());

  const total = items.length;
  const startIndex = Math.max(0, total - visibleCount);
  const visible = items.slice(startIndex);
  const remaining = startIndex;

  return (
    <div className="space-y-6">
      {/* Server-side "load more" — fetch older pages from the API */}
      {hasMore && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={onLoadMore}
            disabled={isFetchingMore}
          >
            {isFetchingMore ? "Loading..." : "Load more messages from server"}
          </Button>
        </div>
      )}
      {/* Client-side windowing — show more of the already-loaded messages */}
      {remaining > 0 && (
        <div className="flex justify-center">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setVisibleCount((c) => c + INITIAL_MESSAGE_WINDOW)}
          >
            Show earlier messages ({remaining} remaining)
          </Button>
        </div>
      )}
      {visible.map((item) =>
        item.kind === "message" ? (
          <MessageBubble
            key={item.id}
            message={item.message}
            showTimestamp
            evalResults={evalsByMessage.get(item.message.id)}
          />
        ) : (
          <ToolCallIndicator key={item.id} toolCall={item.toolCall} />
        )
      )}
    </div>
  );
}

/**
 * Group eval results by messageId for efficient lookup.
 */
function groupEvalResultsByMessageId(results: EvalResult[]): Map<string, EvalResult[]> {
  const grouped = new Map<string, EvalResult[]>();
  for (const result of results) {
    if (!result.messageId) continue;
    const existing = grouped.get(result.messageId);
    if (existing) {
      existing.push(result);
    } else {
      grouped.set(result.messageId, [result]);
    }
  }
  return grouped;
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title={
          <div className="flex items-center gap-3">
            <Button variant="ghost" size="icon" asChild>
              <Link href="/sessions">
                <ArrowLeft className="h-4 w-4" />
              </Link>
            </Button>
            <Skeleton className="h-6 w-48" />
          </div>
        }
      />
      <div className="flex-1 p-6 space-y-4">
        <Skeleton className="h-10 w-64" />
        <Card className="h-[calc(100vh-300px)]">
          <div className="p-6 space-y-6">
            <Skeleton className="h-16 w-3/4" />
            <Skeleton className="h-16 w-1/2 ml-auto" />
            <Skeleton className="h-16 w-3/4" />
            <Skeleton className="h-16 w-1/2 ml-auto" />
          </div>
        </Card>
      </div>
    </div>
  );
}

export default function SessionDetailPage({
  params,
}: Readonly<{
  params: Promise<{ id: string }>;
}>) {
  const { id } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const defaultTab = searchParams.get("tab") ?? "conversation";
  const [memorySidebarOpen, setMemorySidebarOpen] = useState(false);
  const { data: session, isLoading, error } = useSessionDetail(id);
  const sessionReady = !!session;
  const { data: evalResults } = useSessionEvalResults(id, sessionReady);
  const { data: rawToolCalls } = useSessionToolCalls(id, sessionReady);
  const toolCalls = rawToolCalls ? collapseToolCalls(rawToolCalls) : undefined;
  const { data: providerCalls } = useSessionProviderCalls(id, sessionReady);
  const { data: runtimeEvents } = useSessionRuntimeEvents(id, sessionReady);
  const { messages: paginatedMessages, hasMore: messagesHasMore, isFetchingMore: messagesIsFetchingMore, fetchMore: messagesFetchMore } = useSessionAllMessages(id, session?.status, sessionReady);
  const allMessages = paginatedMessages.length > 0 ? paginatedMessages : (session?.messages ?? []);
  const grafana = useGrafana();
  const sessionDashboardUrl = grafana.enabled && session
    ? buildSessionDashboardUrl(grafana, id, session.agentName, session.agentNamespace)
    : null;

  if (isLoading) {
    return <DetailSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Session Error" />
        <div className="flex-1 flex items-center justify-center p-6">
          <Alert variant="destructive" className="max-w-md">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Failed to load session</AlertTitle>
            <AlertDescription>
              {error instanceof Error ? error.message : "An unexpected error occurred"}
            </AlertDescription>
            <Button variant="outline" size="sm" className="mt-3" onClick={() => router.push("/sessions")}>
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Sessions
            </Button>
          </Alert>
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Session Not Found" />
        <div className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <p className="text-muted-foreground mb-4">
              Session &quot;{id}&quot; was not found.
            </p>
            <Button onClick={() => router.push("/sessions")}>
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Sessions
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const handleExport = (format: "json" | "markdown") => {
    let content: string;
    let filename: string;
    let mimeType: string;

    if (format === "json") {
      content = JSON.stringify(session, null, 2);
      filename = `session-${session.id}.json`;
      mimeType = "application/json";
    } else {
      // Markdown export
      const lines = [
        `# Session ${session.id}`,
        "",
        `**Agent:** ${session.agentName}`,
        `**Status:** ${session.status}`,
        `**Started:** ${formatDate(new Date(session.startedAt), "PPpp")}`,
        session.endedAt ? `**Ended:** ${formatDate(new Date(session.endedAt), "PPpp")}` : "",
        "",
        "## Conversation",
        "",
      ];

      // Interleave messages and tool calls for export
      const exportItems: { timestamp: string; render: () => void }[] = [];
      for (const msg of session.messages.filter(isConversationMessage)) {
        exportItems.push({
          timestamp: msg.timestamp,
          render: () => {
            const roleLabel = msg.role.charAt(0).toUpperCase() + msg.role.slice(1);
            lines.push(`### ${roleLabel}`, "", msg.content, "");
          },
        });
      }
      for (const tc of toolCalls || []) {
        exportItems.push({
          timestamp: tc.createdAt,
          render: () => {
            lines.push(
              `**Tool Call:** \`${tc.name}\``,
              "```json",
              JSON.stringify(tc.arguments, null, 2),
              "```",
              ""
            );
          },
        });
      }
      exportItems
        .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
        .forEach((item) => item.render());

      content = lines.join("\n");
      filename = `session-${session.id}.md`;
      mimeType = "text/markdown";
    }

    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  };

  const copySessionId = () => {
    navigator.clipboard.writeText(session.id);
  };

  return (
    <div className="flex flex-col h-full">
      <Header
        title={
          <div className="flex items-center gap-3 min-w-0">
            <Button variant="ghost" size="icon" className="shrink-0" asChild>
              <Link href="/sessions">
                <ArrowLeft className="h-4 w-4" />
              </Link>
            </Button>
            <span className="truncate">Session {session.id}</span>
          </div>
        }
      />

      {/* Session info bar */}
      <div className="flex items-center justify-between border-b border-border bg-card px-6 py-3">
        <div className="flex items-center gap-4 min-w-0">
          {getStatusBadge(session.status)}
          <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Bot className="h-4 w-4 shrink-0" />
            {session.agentName}
          </span>
          <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Clock className="h-4 w-4 shrink-0" />
            {formatDistanceToNow(new Date(session.startedAt), { addSuffix: true })}
          </span>
          <Button variant="ghost" size="sm" className="h-7 px-2 text-muted-foreground" onClick={copySessionId}>
            <Copy className="h-3.5 w-3.5 mr-1.5" />
            Copy ID
          </Button>
        </div>
        <div className="flex items-center gap-2">
          {sessionDashboardUrl && (
            <Button variant="outline" size="sm" asChild>
              <a href={sessionDashboardUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-2" />
                Grafana
              </a>
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => handleExport("markdown")}>
            <Download className="h-4 w-4 mr-2" />
            Export MD
          </Button>
          <Button variant="outline" size="sm" onClick={() => handleExport("json")}>
            <Download className="h-4 w-4 mr-2" />
            Export JSON
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setMemorySidebarOpen(true)}
            data-testid="memories-toggle"
          >
            <Brain className="h-4 w-4 mr-2" />
            Memories
          </Button>
        </div>
      </div>

      <div className="flex-1 flex flex-col min-h-0 p-6">
        <Tabs defaultValue={defaultTab} className="flex-1 flex flex-col min-h-0">
          <TabsList>
            <TabsTrigger value="conversation">Conversation</TabsTrigger>
            <TabsTrigger value="replay">
              <Play className="h-4 w-4 mr-2" />
              Replay
            </TabsTrigger>
            <TabsTrigger value="evals" className="gap-1">
              <Shield className="h-3.5 w-3.5" />
              Evals
              {(() => {
                if (!evalResults || evalResults.length === 0) return null;
                const failed = evalResults.filter((r) => !r.passed).length;
                return (
                  <Badge
                    variant={failed > 0 ? "destructive" : "secondary"}
                    className="ml-1 px-1.5 py-0 text-[10px] leading-4"
                  >
                    {failed > 0 ? `${failed} failed` : "pass"}
                  </Badge>
                );
              })()}
            </TabsTrigger>
            <TabsTrigger value="metrics">Metrics</TabsTrigger>
            <TabsTrigger value="metadata">Metadata</TabsTrigger>
          </TabsList>

          <TabsContent value="conversation" className="flex-1 min-h-0 mt-4">
            <ConversationWithDebugPanel session={session} messages={allMessages} hasMore={messagesHasMore} isFetchingMore={messagesIsFetchingMore} fetchMore={messagesFetchMore} evalResults={evalResults || []} toolCalls={toolCalls || []} providerCalls={providerCalls || []} runtimeEvents={runtimeEvents || []} />
          </TabsContent>

          <TabsContent value="replay" className="flex-1 min-h-0">
            <ReplayTab
              session={session}
              messages={allMessages}
              toolCalls={toolCalls ?? []}
              providerCalls={providerCalls ?? []}
              runtimeEvents={runtimeEvents ?? []}
            />
          </TabsContent>

          <TabsContent value="evals" className="mt-4">
            <EvalResultsPanel results={evalResults || []} messages={session.messages} />
          </TabsContent>

          <TabsContent value="metrics" className="mt-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Messages
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{session.metrics.messageCount}</p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Tool Calls
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{session.metrics.toolCallCount}</p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Total Tokens
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">
                    {session.metrics.totalTokens.toLocaleString()}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {session.metrics.inputTokens} in / {session.metrics.outputTokens} out
                  </p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">
                    Estimated Cost
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">
                    ${session.metrics.estimatedCost?.toFixed(4) || "0.0000"}
                  </p>
                </CardContent>
              </Card>
            </div>

            {session.metrics.avgResponseTime && (
              <Card className="mt-4">
                <CardHeader>
                  <CardTitle className="text-sm font-medium">Performance</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-2">
                    <Clock className="h-4 w-4 text-muted-foreground" />
                    <span>Average Response Time:</span>
                    <span className="font-medium">{session.metrics.avgResponseTime}ms</span>
                  </div>
                </CardContent>
              </Card>
            )}
          </TabsContent>

          <TabsContent value="metadata" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">Session Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-muted-foreground">Session ID</p>
                    <p className="font-mono">{session.id}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Agent</p>
                    <p>{session.agentName}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Namespace</p>
                    <p>{session.agentNamespace}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Status</p>
                    <p>{session.status}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Started At</p>
                    <p>{formatDate(new Date(session.startedAt), "PPpp")}</p>
                  </div>
                  {session.endedAt && (
                    <div>
                      <p className="text-sm text-muted-foreground">Ended At</p>
                      <p>{formatDate(new Date(session.endedAt), "PPpp")}</p>
                    </div>
                  )}
                </div>

                {session.metadata?.tags && session.metadata.tags.length > 0 && (
                  <>
                    <Separator />
                    <div>
                      <p className="text-sm text-muted-foreground mb-2">Tags</p>
                      <div className="flex flex-wrap gap-2">
                        {session.metadata.tags.map((tag) => (
                          <Badge key={tag} variant="outline">
                            {tag}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  </>
                )}

                {(session.metadata?.userAgent || session.metadata?.clientIp) && (
                  <>
                    <Separator />
                    <div className="grid grid-cols-2 gap-4">
                      {session.metadata.userAgent && (
                        <div>
                          <p className="text-sm text-muted-foreground">User Agent</p>
                          <p className="text-sm truncate">{session.metadata.userAgent}</p>
                        </div>
                      )}
                      {session.metadata.clientIp && (
                        <div>
                          <p className="text-sm text-muted-foreground">Client IP</p>
                          <p className="font-mono">{session.metadata.clientIp}</p>
                        </div>
                      )}
                    </div>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>

      <MemorySidebar
        agentName={session?.agentName ?? ""}
        open={memorySidebarOpen}
        onClose={() => setMemorySidebarOpen(false)}
      />
    </div>
  );
}

/** Eval type label mapping. */
function evalTypeLabel(evalType: string): string {
  const labels: Record<string, string> = {
    rule: "Rule", llm_judge: "LLM Judge", similarity: "Similarity",
    regex: "Regex", custom: "Custom", contains: "Contains",
  };
  return labels[evalType] || evalType;
}

/** Aggregated eval — turn evals are averaged across executions. */
interface AggregatedEval {
  key: string;
  evalId: string;
  evalType: string;
  trigger: string;
  passRate: number;
  avgScore?: number;
  avgDurationMs?: number;
  executions: number;
  details?: Record<string, unknown>;
}

/** Extract and aggregate eval items from the eval_results table. Turn evals are averaged by evalId. */
function aggregateEvals(
  results: EvalResult[],
  _messages: Message[],
): { turnEvals: AggregatedEval[]; sessionEvals: AggregatedEval[] } {
  // Collect all raw items
  interface RawItem {
    evalId: string;
    evalType: string;
    trigger: string;
    passed?: boolean;
    score?: number;
    durationMs?: number;
    details?: Record<string, unknown>;
  }

  const rawItems: RawItem[] = [];

  for (const r of results) {
    rawItems.push({
      evalId: r.evalId, evalType: r.evalType, trigger: r.trigger,
      passed: r.passed, score: r.score, durationMs: r.durationMs, details: r.details,
    });
  }

  // Group by evalId + trigger
  const groups = new Map<string, RawItem[]>();
  for (const item of rawItems) {
    const key = `${item.evalId}:${item.trigger}`;
    const existing = groups.get(key);
    if (existing) {
      existing.push(item);
    } else {
      groups.set(key, [item]);
    }
  }

  // Aggregate each group
  const turnEvals: AggregatedEval[] = [];
  const sessionEvals: AggregatedEval[] = [];

  for (const [key, items] of groups) {
    const passedCount = items.filter((i) => i.passed).length;
    const scores = items.filter((i) => i.score !== undefined && i.score !== null).map((i) => i.score!);
    const durations = items.filter((i) => i.durationMs !== undefined && i.durationMs > 0).map((i) => i.durationMs!);

    const agg: AggregatedEval = {
      key,
      evalId: items[0].evalId,
      evalType: items[0].evalType,
      trigger: items[0].trigger,
      passRate: passedCount / items.length,
      avgScore: scores.length > 0 ? scores.reduce((a, b) => a + b, 0) / scores.length : undefined,
      avgDurationMs: durations.length > 0 ? Math.round(durations.reduce((a, b) => a + b, 0) / durations.length) : undefined,
      executions: items.length,
      details: items[0].details,
    };

    if (agg.trigger === "on_session_complete") {
      sessionEvals.push(agg);
    } else {
      turnEvals.push(agg);
    }
  }

  return { turnEvals, sessionEvals };
}

/** Full eval results panel shown in the Evals tab. Shows aggregated results from both eval_results table and message-based eval events. */
function EvalResultsPanel({ results, messages }: Readonly<{ results: EvalResult[]; messages: Message[] }>) {
  const { turnEvals, sessionEvals } = aggregateEvals(results, messages);
  const allEvals = [...turnEvals, ...sessionEvals];

  if (allEvals.length === 0) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-muted-foreground">
          No eval results for this session.
        </CardContent>
      </Card>
    );
  }

  const totalExecutions = allEvals.reduce((sum, e) => sum + e.executions, 0);
  const totalPassed = allEvals.reduce((sum, e) => sum + Math.round(e.passRate * e.executions), 0);
  const totalFailed = totalExecutions - totalPassed;

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Shield className="h-4 w-4" />
              Unique Evals
            </div>
            <p className="text-2xl font-bold mt-1">{allEvals.length}</p>
            <p className="text-xs text-muted-foreground">{totalExecutions} total executions</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <CheckCircle2 className="h-4 w-4 text-green-500" />
              Passed
            </div>
            <p className="text-2xl font-bold mt-1 text-green-600 dark:text-green-400">{totalPassed}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <XCircle className="h-4 w-4 text-red-500" />
              Failed
            </div>
            <p className="text-2xl font-bold mt-1 text-red-600 dark:text-red-400">{totalFailed}</p>
          </CardContent>
        </Card>
      </div>

      {/* Turn-level evals (aggregated) */}
      {turnEvals.length > 0 && (
        <AggregatedEvalSection title="Turn-Level Evals" evals={turnEvals} />
      )}

      {/* Session-level evals */}
      {sessionEvals.length > 0 && (
        <AggregatedEvalSection title="Session-Level Evals" evals={sessionEvals} />
      )}
    </div>
  );
}

/** Section that displays aggregated eval results in a table. */
function AggregatedEvalSection({ title, evals }: Readonly<{ title: string; evals: AggregatedEval[] }>) {
  const allPassing = evals.every((e) => e.passRate === 1);
  const someFailing = evals.some((e) => e.passRate < 1);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          {title}
          <Badge variant={someFailing ? "destructive" : "outline"} className="text-xs font-normal">
            {allPassing ? `${evals.length} passing` : `${evals.filter((e) => e.passRate < 1).length} with failures`}
          </Badge>
        </CardTitle>
      </CardHeader>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Status</TableHead>
            <TableHead>Eval ID</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Pass Rate</TableHead>
            <TableHead>Avg Score</TableHead>
            <TableHead>Avg Duration</TableHead>
            <TableHead>Runs</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {evals
            .sort((a, b) => a.passRate - b.passRate)
            .map((e) => (
              <AggregatedEvalRow key={e.key} eval_={e} />
            ))}
        </TableBody>
      </Table>
    </Card>
  );
}

/** Single aggregated eval result row. */
function AggregatedEvalRow({ eval_ }: Readonly<{ eval_: AggregatedEval }>) {
  const allPassed = eval_.passRate === 1;
  const allFailed = eval_.passRate === 0;

  return (
    <TableRow>
      <TableCell>
        {allPassed
          ? <CheckCircle2 className="h-4 w-4 text-green-500" />
          : allFailed
            ? <XCircle className="h-4 w-4 text-red-500" />
            : <AlertCircle className="h-4 w-4 text-yellow-500" />
        }
      </TableCell>
      <TableCell className="font-mono text-sm">{eval_.evalId}</TableCell>
      <TableCell>
        <Badge variant="outline">{evalTypeLabel(eval_.evalType)}</Badge>
      </TableCell>
      <TableCell>
        <span className={cn(
          "font-medium",
          allPassed && "text-green-600 dark:text-green-400",
          allFailed && "text-red-600 dark:text-red-400",
          !allPassed && !allFailed && "text-yellow-600 dark:text-yellow-400",
        )}>
          {(eval_.passRate * 100).toFixed(0)}%
        </span>
      </TableCell>
      <TableCell>
        {eval_.avgScore === undefined ? "-" : `${(eval_.avgScore * 100).toFixed(0)}%`}
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {eval_.avgDurationMs === undefined ? "-" : `${eval_.avgDurationMs}ms`}
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {eval_.executions}
      </TableCell>
    </TableRow>
  );
}

function ConversationWithDebugPanel({
  session,
  messages,
  hasMore,
  isFetchingMore,
  fetchMore,
  evalResults,
  toolCalls,
  providerCalls,
  runtimeEvents,
}: Readonly<{ session: Session; messages: Message[]; hasMore: boolean; isFetchingMore: boolean; fetchMore: () => void; evalResults: EvalResult[]; toolCalls: ToolCall[]; providerCalls: ProviderCall[]; runtimeEvents: RuntimeEvent[] }>) {
  const debugOpen = useDebugPanelStore((s) => s.isOpen);

  const conversationContent = (
    <ConversationMessages
      messages={messages}
      evalResults={evalResults}
      toolCalls={toolCalls}
      hasMore={hasMore}
      isFetchingMore={isFetchingMore}
      onLoadMore={fetchMore}
    />
  );

  if (!debugOpen) {
    return (
      <div className="flex flex-col h-full">
        <Card className="flex-1 min-h-0">
          <ScrollArea className="h-full p-6">
            {conversationContent}
          </ScrollArea>
        </Card>
        <DebugPanel messages={messages} session={session} toolCalls={toolCalls} providerCalls={providerCalls} runtimeEvents={runtimeEvents} evalResults={evalResults} />
      </div>
    );
  }

  return (
    <ResizablePanelGroup orientation="vertical" className="h-full">
      <ResizablePanel defaultSize={70} minSize={30}>
        <Card className="h-full">
          <ScrollArea className="h-full p-6">
            {conversationContent}
          </ScrollArea>
        </Card>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={30} minSize={15}>
        <DebugPanel messages={messages} session={session} toolCalls={toolCalls} providerCalls={providerCalls} runtimeEvents={runtimeEvents} evalResults={evalResults} />
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}
