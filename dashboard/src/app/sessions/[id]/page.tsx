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
import { Markdown } from "@/components/console/markdown";
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

/** Compact expandable badge for inline eval results attached to assistant messages. */
function InlineEvalsBadge({ evals }: Readonly<{ evals: ParsedEval[] }>) {
  const [expanded, setExpanded] = useState(false);
  const passed = evals.filter((e) => e.passed).length;
  const failed = evals.length - passed;
  const allPassed = failed === 0;

  return (
    <div className="mt-1">
      <button
        className="inline-flex items-center gap-1"
        onClick={() => setExpanded(!expanded)}
      >
        <Badge
          variant={allPassed ? "secondary" : "destructive"}
          className={cn(
            "gap-1 text-xs cursor-pointer",
            allPassed && "bg-green-100 text-green-800 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400"
          )}
        >
          <Shield className="h-3 w-3" />
          {allPassed
            ? `${passed} eval${passed === 1 ? "" : "s"} passed`
            : `${failed} of ${evals.length} eval${evals.length === 1 ? "" : "s"} failed`}
        </Badge>
      </button>
      {expanded && (
        <div className="mt-2 space-y-1 max-w-md">
          {evals.map((e) => (
            <div key={e.evalID} className="border rounded p-2 text-xs space-y-0.5">
              <div className="flex items-center gap-2">
                {e.passed
                  ? <CheckCircle2 className="h-3 w-3 text-green-500" />
                  : <XCircle className="h-3 w-3 text-red-500" />
                }
                <span className="font-medium font-mono">{e.evalID}</span>
                <Badge variant="outline" className="text-[10px] px-1 py-0">
                  {evalTypeLabel(e.evalType)}
                </Badge>
                {e.score !== undefined && e.score !== null && (
                  <span className="text-muted-foreground">{(e.score * 100).toFixed(0)}%</span>
                )}
                {e.durationMs !== undefined && e.durationMs > 0 && (
                  <span className="text-muted-foreground">{e.durationMs}ms</span>
                )}
              </div>
              {e.explanation && <p className="text-muted-foreground pl-5">{e.explanation}</p>}
              {e.error && <p className="text-red-500 pl-5">{e.error}</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function MessageBubble({ message, showTimestamp, evalResults, inlineEvals }: Readonly<{ message: Message; showTimestamp?: boolean; evalResults?: EvalResult[]; inlineEvals?: ParsedEval[] }>) {
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
          <Markdown content={message.content} className={cn("text-sm", isUser && "prose-invert")} />
        </div>

        {/* Eval results from eval_results table */}
        {evalResults && evalResults.length > 0 && (
          <EvalResultsBadge results={evalResults} />
        )}

        {/* Inline eval results from message events */}
        {inlineEvals && inlineEvals.length > 0 && (
          <InlineEvalsBadge evals={inlineEvals} />
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
 * Returns true if a message is an eval result event.
 */
function isEvalMessage(m: Message): boolean {
  const t = m.metadata?.type;
  return t === "eval_completed" || t === "eval_failed";
}

/**
 * Returns true if a message belongs in the conversation view.
 *
 * Conversation messages are: user/assistant messages without a metadata.type,
 * plus tool_call messages (rendered as compact indicators).
 * Everything else (pipeline events, eval events, provider calls, etc.) goes to the Debug panel.
 */
function isConversationMessage(m: Message): boolean {
  if (m.role === "tool") return false;
  if (m.metadata?.type === "tool_call") return true;
  if (m.metadata?.type) return false;
  if (m.metadata?.source === "runtime") return false;
  return true;
}

/** Parsed eval content from a message. */
interface ParsedEval {
  evalID: string;
  evalType: string;
  trigger: string;
  passed: boolean;
  score?: number;
  durationMs?: number;
  explanation?: string;
  message?: string;
  error?: string;
}

/** Parse the JSON content of an eval message into structured data. */
function parseEvalContent(content: string): ParsedEval {
  try {
    return JSON.parse(content);
  } catch {
    return { evalID: "unknown", evalType: "unknown", trigger: "", passed: false };
  }
}

/**
 * Collect eval messages that follow each assistant message.
 * Returns a map from assistant message ID to the parsed eval results.
 */
function collectEvalsForAssistantMessages(messages: Message[]): Map<string, ParsedEval[]> {
  const sorted = [...messages].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  );
  const evalMap = new Map<string, ParsedEval[]>();
  let lastAssistantId: string | null = null;

  for (const m of sorted) {
    if (m.role === "assistant" && !m.metadata?.type) {
      lastAssistantId = m.id;
    } else if (isEvalMessage(m) && lastAssistantId) {
      const existing = evalMap.get(lastAssistantId);
      const parsed = parseEvalContent(m.content);
      if (existing) {
        existing.push(parsed);
      } else {
        evalMap.set(lastAssistantId, [parsed]);
      }
    }
  }
  return evalMap;
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
  const inlineEvalsByMessage = collectEvalsForAssistantMessages(messages);

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
          inlineEvals={inlineEvalsByMessage.get(message.id)}
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
        </div>
      </div>

      <div className="flex-1 flex flex-col min-h-0 p-6">
        <Tabs defaultValue={defaultTab} className="flex-1 flex flex-col min-h-0">
          <TabsList>
            <TabsTrigger value="conversation">Conversation</TabsTrigger>
            <TabsTrigger value="evals" className="gap-1">
              <Shield className="h-3.5 w-3.5" />
              Evals
              {(() => {
                const hasTableResults = evalResults && evalResults.length > 0;
                const hasMsgResults = session.messages.some(isEvalMessage);
                if (!hasTableResults && !hasMsgResults) return null;
                const msgFailed = session.messages.filter((m) => m.metadata?.type === "eval_failed").length;
                const tableFailed = evalResults?.filter((r) => !r.passed).length ?? 0;
                const totalFailed = tableFailed + msgFailed;
                return (
                  <Badge
                    variant={totalFailed > 0 ? "destructive" : "secondary"}
                    className="ml-1 px-1.5 py-0 text-[10px] leading-4"
                  >
                    {totalFailed > 0 ? `${totalFailed} failed` : "pass"}
                  </Badge>
                );
              })()}
            </TabsTrigger>
            <TabsTrigger value="metrics">Metrics</TabsTrigger>
            <TabsTrigger value="metadata">Metadata</TabsTrigger>
          </TabsList>

          <TabsContent value="conversation" className="flex-1 min-h-0 mt-4">
            <ConversationWithDebugPanel session={session} evalResults={evalResults || []} />
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

/** Extract and aggregate eval items from both sources. Turn evals are averaged by evalId. */
function aggregateEvals(
  results: EvalResult[],
  messages: Message[],
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

  for (const m of messages.filter(isEvalMessage)) {
    const data = parseEvalContent(m.content);
    rawItems.push({
      evalId: data.evalID, evalType: data.evalType, trigger: data.trigger,
      passed: data.passed, score: data.score, durationMs: data.durationMs,
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
