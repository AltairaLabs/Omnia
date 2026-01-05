"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import {
  ArrowLeft,
  Bot,
  User,
  Wrench,
  Clock,
  Coins,
  Download,
  CheckCircle2,
  XCircle,
  Loader2,
  ChevronDown,
  ChevronRight,
  Copy,
  MessageSquare,
} from "lucide-react";
import { getMockSession } from "@/lib/mock-data";
import type { Message, ToolCall, Session } from "@/types";
import { format as formatDate, formatDistanceToNow } from "date-fns";
import { cn } from "@/lib/utils";

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

function ToolCallBadge({ status }: { status: ToolCall["status"] }) {
  switch (status) {
    case "success":
      return (
        <Badge variant="secondary" className="gap-1">
          <CheckCircle2 className="h-3 w-3 text-green-500" />
          Success
        </Badge>
      );
    case "error":
      return (
        <Badge variant="destructive" className="gap-1">
          <XCircle className="h-3 w-3" />
          Error
        </Badge>
      );
    case "pending":
      return (
        <Badge variant="outline" className="gap-1">
          <Loader2 className="h-3 w-3 animate-spin" />
          Pending
        </Badge>
      );
  }
}

function ToolCallCard({ toolCall }: { toolCall: ToolCall }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border rounded-lg bg-muted/30 p-3 my-2">
      <button
        className="flex items-center justify-between w-full text-left"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-2">
          <Wrench className="h-4 w-4 text-orange-500" />
          <span className="font-mono text-sm font-medium">{toolCall.name}</span>
          <ToolCallBadge status={toolCall.status} />
          {toolCall.duration && (
            <span className="text-xs text-muted-foreground">
              {toolCall.duration}ms
            </span>
          )}
        </div>
        {expanded ? (
          <ChevronDown className="h-4 w-4" />
        ) : (
          <ChevronRight className="h-4 w-4" />
        )}
      </button>

      {expanded && (
        <div className="mt-3 space-y-2">
          <div>
            <div className="text-xs text-muted-foreground mb-1">Arguments</div>
            <pre className="bg-background p-2 rounded text-xs overflow-x-auto">
              {JSON.stringify(toolCall.arguments, null, 2)}
            </pre>
          </div>
          {toolCall.result !== undefined && (
            <div>
              <div className="text-xs text-muted-foreground mb-1">Result</div>
              <pre className="bg-background p-2 rounded text-xs overflow-x-auto">
                {JSON.stringify(toolCall.result, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function MessageBubble({ message, showTimestamp }: { message: Message; showTimestamp?: boolean }) {
  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";
  const isTool = message.role === "tool";
  const isSystem = message.role === "system";

  if (isTool) {
    return null; // Tool results are shown inline with tool calls
  }

  return (
    <div className={cn("flex gap-3", isUser ? "flex-row-reverse" : "flex-row")}>
      <div
        className={cn(
          "flex items-center justify-center h-8 w-8 rounded-full shrink-0",
          isUser
            ? "bg-primary text-primary-foreground"
            : isAssistant
            ? "bg-blue-500 text-white"
            : "bg-gray-500 text-white"
        )}
      >
        {isUser ? (
          <User className="h-4 w-4" />
        ) : isAssistant ? (
          <Bot className="h-4 w-4" />
        ) : (
          <MessageSquare className="h-4 w-4" />
        )}
      </div>

      <div className={cn("flex flex-col max-w-[80%]", isUser ? "items-end" : "items-start")}>
        <div
          className={cn(
            "rounded-lg px-4 py-2",
            isUser
              ? "bg-primary text-primary-foreground"
              : isSystem
              ? "bg-muted border"
              : "bg-muted"
          )}
        >
          <div className="whitespace-pre-wrap text-sm">{message.content}</div>
        </div>

        {/* Tool calls */}
        {message.toolCalls?.map((tc) => (
          <ToolCallCard key={tc.id} toolCall={tc} />
        ))}

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

export default function SessionDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const router = useRouter();
  const session = getMockSession(id);

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

      session.messages.forEach((msg) => {
        if (msg.role === "tool") return;
        const roleLabel = msg.role.charAt(0).toUpperCase() + msg.role.slice(1);
        lines.push(`### ${roleLabel}`);
        lines.push("");
        lines.push(msg.content);
        lines.push("");

        if (msg.toolCalls) {
          msg.toolCalls.forEach((tc) => {
            lines.push(`**Tool Call:** \`${tc.name}\``);
            lines.push("```json");
            lines.push(JSON.stringify(tc.arguments, null, 2));
            lines.push("```");
            if (tc.result) {
              lines.push("**Result:**");
              lines.push("```json");
              lines.push(JSON.stringify(tc.result, null, 2));
              lines.push("```");
            }
            lines.push("");
          });
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
    document.body.removeChild(a);
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

      <div className="flex-1 p-6">
        <Tabs defaultValue="conversation" className="h-full flex flex-col">
          <TabsList>
            <TabsTrigger value="conversation">Conversation</TabsTrigger>
            <TabsTrigger value="metrics">Metrics</TabsTrigger>
            <TabsTrigger value="metadata">Metadata</TabsTrigger>
          </TabsList>

          <TabsContent value="conversation" className="flex-1 mt-4">
            <Card className="h-[calc(100vh-300px)]">
              <ScrollArea className="h-full p-6">
                <div className="space-y-6">
                  {session.messages
                    .filter((m) => m.role !== "tool")
                    .map((message) => (
                      <MessageBubble
                        key={message.id}
                        message={message}
                        showTimestamp
                      />
                    ))}
                </div>
              </ScrollArea>
            </Card>
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
