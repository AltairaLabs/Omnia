"use client";

import { useCallback, useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { JsonBlock } from "@/components/ui/json-block";
import { Copy, Check } from "lucide-react";
import type { Session, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";
import type { EvalResult } from "@/types/eval";

interface RawTabProps {
  readonly session: Session;
  readonly toolCalls?: ToolCall[];
  readonly providerCalls?: ProviderCall[];
  readonly runtimeEvents?: RuntimeEvent[];
  readonly evalResults?: EvalResult[];
}

export function RawTab({ session, toolCalls, providerCalls, runtimeEvents, evalResults }: RawTabProps) {
  const [copied, setCopied] = useState(false);
  const fullData = {
    ...session,
    toolCalls: toolCalls ?? [],
    providerCalls: providerCalls ?? [],
    runtimeEvents: runtimeEvents ?? [],
    evalResults: evalResults ?? [],
  };
  const json = JSON.stringify(fullData, null, 2);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(json);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [json]);

  return (
    <div className="relative h-full" data-testid="raw-tab">
      <Button
        variant="ghost"
        size="sm"
        className="absolute top-2 right-4 z-10"
        onClick={handleCopy}
        data-testid="raw-copy-button"
      >
        {copied ? (
          <><Check className="h-3.5 w-3.5 mr-1" /> Copied</>
        ) : (
          <><Copy className="h-3.5 w-3.5 mr-1" /> Copy</>
        )}
      </Button>
      <ScrollArea className="h-full">
        <div className="p-4" data-testid="raw-json">
          <JsonBlock data={fullData} defaultExpandDepth={1} defaultCollapsed={["messages", "metadata", "toolCalls", "providerCalls", "runtimeEvents", "evalResults"]} />
        </div>
      </ScrollArea>
    </div>
  );
}
