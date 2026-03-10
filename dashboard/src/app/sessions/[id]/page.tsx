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
  User,
  Wrench,
  Clock,
  Coins,
  Download,
  Copy,
  MessageSquare,
  AlertCircle,
  ExternalLink,
  Bug,
  CheckCircle2,
  XCircle,
  Shield,
} from "lucide-react";
import { useSessionDetail, useSessionAllMessages, useSessionEvalResults } from "@/hooks";
import type { Message, Session, EvalResult } from "@/types";
import { EvalResultsBadge } from "@/components/sessions/eval-results-badge";
import { ToolCallBadge } from "@/components/sessions/tool-call-badge";
import { DebugPanel } from "@/components/sessions/debug-panel";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { format as formatDate, formatDistanceToNow } from "date-fns";
import { cn } from "@/lib/utils";
import {
  useGrafana,
  buildSessionDashboardUrl,
} from "@/hooks/use-grafana";

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

/**
 * Parse a tool_call message's content to extract name and arguments.
 */
function parseToolCallContent(content: string): { name: string; arguments: Record<string, unknown> } {
  try {
    const parsed = JSON.parse(content);
    return { name: parsed.name || "unknown", arguments: parsed.arguments || {} };
  } catch {
    return { name: "unknown", arguments: {} };
  }
}

function ToolCallMessage({ message }: Readonly<{ message: Message }>) {
  const openToolCall = useDebugPanelStore((s) => s.openToolCall);
  const { name } = parseToolCallContent(message.content);
  const durationStr = message.metadata?.duration_ms;
  const duration = durationStr ? Number.parseInt(durationStr, 10) : undefined;
  const status = message.metadata?.status as "success" | "error" | undefined;

  return (
    <div className="flex gap-3">
      <div className="flex items-center justify-center h-8 w-8 rounded-full shrink-0 bg-orange-500/10">
        <Wrench className="h-4 w-4 text-orange-500" />
      </div>
      <button
        className="flex items-center gap-2 border rounded-lg bg-muted/30 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors"
        onClick={() => message.toolCallId && openToolCall(message.toolCallId)}
      >
        <span className="font-mono text-sm font-medium truncate">{name}</span>
        {status && <ToolCallBadge status={status} />}
        {duration !== undefined && !Number.isNaN(duration) && (
          <span className="text-xs text-muted-foreground">{duration}ms</span>
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

  // Tool call messages get their own compact rendering
  if (message.metadata?.type === "tool_call") {
    return <ToolCallMessage message={message} />;
  }

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
          <div className="whitespace-pre-wrap text-sm">{message.content}</div>
        </div>

        {/* Eval results */}
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
 * Conversation messages are: user/assistant messages without a metadata.type,
 * plus tool_call messages (rendered as compact indicators).
 * Everything else (pipeline events, provider calls, etc.) goes to the Debug panel.
 */
function isConversationMessage(m: Message): boolean {
  if (m.role === "tool") return false;
  if (m.metadata?.type === "tool_call") return true;
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
function ConversationMessages({
  messages,
  evalResults,
  hasMore,
  isFetchingMore,
  onLoadMore,
}: Readonly<{
  messages: Message[];
  evalResults: EvalResult[];
  hasMore?: boolean;
  isFetchingMore?: boolean;
  onLoadMore?: () => void;
}>) {
  const [visibleCount, setVisibleCount] = useState(INITIAL_MESSAGE_WINDOW);
  const evalsByMessage = groupEvalResultsByMessageId(evalResults);

  const sorted = messages
    .filter(isConversationMessage)
    .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());

  const total = sorted.length;
  const startIndex = Math.max(0, total - visibleCount);
  const visible = sorted.slice(startIndex);
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
      {visible.map((message) => (
        <MessageBubble
          key={message.id}
          message={message}
          showTimestamp
          evalResults={evalsByMessage.get(message.id)}
        />
      ))}
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

function DebugToggleButton() {
  const debugPanelOpen = useDebugPanelStore((s) => s.isOpen);
  const toggle = useDebugPanelStore((s) => s.toggle);

  return (
    <Button
      variant={debugPanelOpen ? "secondary" : "outline"}
      size="sm"
      onClick={toggle}
    >
      <Bug className="h-4 w-4 mr-2" />
      Debug
    </Button>
  );
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
  const { data: session, isLoading, error } = useSessionDetail(id);
  const { data: evalResults } = useSessionEvalResults(id);
  const grafana = useGrafana();
  const sessionDashboardUrl = grafana.enabled ? buildSessionDashboardUrl(grafana, id) : null;

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

      session.messages
        .filter(isConversationMessage)
        .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
        .forEach((msg) => {
          if (msg.metadata?.type === "tool_call") {
            const { name, arguments: args } = parseToolCallContent(msg.content);
            lines.push(
              `**Tool Call:** \`${name}\``,
              "```json",
              JSON.stringify(args, null, 2),
              "```",
              ""
            );
          } else {
            const roleLabel = msg.role.charAt(0).toUpperCase() + msg.role.slice(1);
            lines.push(`### ${roleLabel}`, "", msg.content, "");
          }
        });

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
          <div className="flex items-center gap-3">
            <Button variant="ghost" size="icon" asChild>
              <Link href="/sessions">
                <ArrowLeft className="h-4 w-4" />
              </Link>
            </Button>
            <span>Session {session.id}</span>
            <Button variant="ghost" size="icon" onClick={copySessionId}>
              <Copy className="h-4 w-4" />
            </Button>
            {getStatusBadge(session.status)}
          </div>
        }
        description={
          <div className="flex items-center gap-4 text-sm">
            <span className="flex items-center gap-1">
              <Bot className="h-4 w-4" />
              {session.agentName}
            </span>
            <span className="flex items-center gap-1">
              <Clock className="h-4 w-4" />
              {formatDistanceToNow(new Date(session.startedAt), { addSuffix: true })}
            </span>
          </div>
        }
      >
        <div className="flex items-center gap-2">
          {sessionDashboardUrl && (
            <Button variant="outline" size="sm" asChild>
              <a href={sessionDashboardUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-2" />
                Observe
              </a>
            </Button>
          )}
          <DebugToggleButton />
          <Button variant="outline" size="sm" onClick={() => handleExport("markdown")}>
            <Download className="h-4 w-4 mr-2" />
            Export MD
          </Button>
          <Button variant="outline" size="sm" onClick={() => handleExport("json")}>
            <Download className="h-4 w-4 mr-2" />
            Export JSON
          </Button>
        </div>
      </Header>

      <div className="flex-1 flex flex-col min-h-0 p-6">
        <Tabs defaultValue={defaultTab} className="flex-1 flex flex-col min-h-0">
          <TabsList>
            <TabsTrigger value="conversation">Conversation</TabsTrigger>
            <TabsTrigger value="evals" className="gap-1">
              <Shield className="h-3.5 w-3.5" />
              Evals
              {evalResults && evalResults.length > 0 && (
                <Badge
                  variant={evalResults.some((r) => !r.passed) ? "destructive" : "secondary"}
                  className="ml-1 px-1.5 py-0 text-[10px] leading-4"
                >
                  {evalResults.filter((r) => !r.passed).length > 0
                    ? `${evalResults.filter((r) => !r.passed).length} failed`
                    : evalResults.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="metrics">Metrics</TabsTrigger>
            <TabsTrigger value="metadata">Metadata</TabsTrigger>
          </TabsList>

          <TabsContent value="conversation" className="flex-1 min-h-0 mt-4">
            <ConversationWithDebugPanel session={session} evalResults={evalResults || []} />
          </TabsContent>

          <TabsContent value="evals" className="mt-4">
            <EvalResultsPanel results={evalResults || []} />
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

/** Full eval results panel shown in the Evals tab. */
function EvalResultsPanel({ results }: Readonly<{ results: EvalResult[] }>) {
  const passed = results.filter((r) => r.passed);
  const failed = results.filter((r) => !r.passed);

  if (results.length === 0) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-muted-foreground">
          No eval results for this session.
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Shield className="h-4 w-4" />
              Total Evals
            </div>
            <p className="text-2xl font-bold mt-1">{results.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <CheckCircle2 className="h-4 w-4 text-green-500" />
              Passed
            </div>
            <p className="text-2xl font-bold mt-1 text-green-600 dark:text-green-400">{passed.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <XCircle className="h-4 w-4 text-red-500" />
              Failed
            </div>
            <p className="text-2xl font-bold mt-1 text-red-600 dark:text-red-400">{failed.length}</p>
          </CardContent>
        </Card>
      </div>

      {/* Failed evals first */}
      {failed.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base text-red-600 dark:text-red-400 flex items-center gap-2">
              <XCircle className="h-4 w-4" />
              Failed Evals ({failed.length})
            </CardTitle>
          </CardHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Eval ID</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Score</TableHead>
                <TableHead>Details</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {failed.map((r) => (
                <EvalResultRow key={r.id} result={r} />
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Passed evals */}
      {passed.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base text-green-600 dark:text-green-400 flex items-center gap-2">
              <CheckCircle2 className="h-4 w-4" />
              Passed Evals ({passed.length})
            </CardTitle>
          </CardHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Eval ID</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Score</TableHead>
                <TableHead>Details</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {passed.map((r) => (
                <EvalResultRow key={r.id} result={r} />
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}

/** Single eval result row with expandable details. */
function EvalResultRow({ result }: Readonly<{ result: EvalResult }>) {
  const [expanded, setExpanded] = useState(false);
  const hasDetails = result.details && Object.keys(result.details).length > 0;

  return (
    <>
      <TableRow
        className={cn("cursor-pointer", hasDetails && "hover:bg-muted/50")}
        onClick={() => hasDetails && setExpanded(!expanded)}
      >
        <TableCell className="font-mono text-sm">
          <div className="flex items-center gap-2">
            {result.passed
              ? <CheckCircle2 className="h-4 w-4 text-green-500 shrink-0" />
              : <XCircle className="h-4 w-4 text-red-500 shrink-0" />
            }
            {result.evalId}
          </div>
        </TableCell>
        <TableCell>
          <Badge variant="outline">{evalTypeLabel(result.evalType)}</Badge>
        </TableCell>
        <TableCell>
          {result.score === undefined ? "-" : `${(result.score * 100).toFixed(0)}%`}
        </TableCell>
        <TableCell className="text-muted-foreground text-sm">
          {hasDetails
            ? <span className="text-primary text-xs">{expanded ? "Hide details" : "Show details"}</span>
            : <span className="text-xs">-</span>
          }
        </TableCell>
      </TableRow>
      {expanded && hasDetails && (
        <TableRow>
          <TableCell colSpan={4} className="bg-muted/30 p-4">
            <pre className="text-xs font-mono whitespace-pre-wrap overflow-auto max-h-48">
              {JSON.stringify(result.details, null, 2)}
            </pre>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function ConversationWithDebugPanel({
  session,
  evalResults,
}: Readonly<{ session: Session; evalResults: EvalResult[] }>) {
  const debugOpen = useDebugPanelStore((s) => s.isOpen);

  // Use paginated message loading. Falls back to session.messages while loading.
  const {
    messages: paginatedMessages,
    hasMore,
    isFetchingMore,
    fetchMore,
  } = useSessionAllMessages(session.id);

  // Use paginated messages once loaded, otherwise fall back to session.messages
  const messages = paginatedMessages.length > 0 ? paginatedMessages : session.messages;

  const conversationContent = (
    <ConversationMessages
      messages={messages}
      evalResults={evalResults}
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
        <DebugPanel messages={messages} session={session} />
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
        <DebugPanel messages={messages} session={session} />
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}
