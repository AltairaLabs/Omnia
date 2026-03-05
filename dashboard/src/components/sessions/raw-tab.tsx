"use client";

import { useCallback, useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { Copy, Check } from "lucide-react";
import type { Session } from "@/types/session";

interface RawTabProps {
  readonly session: Session;
}

export function RawTab({ session }: RawTabProps) {
  const [copied, setCopied] = useState(false);
  const json = JSON.stringify(session, null, 2);

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
        <pre className="p-4 text-xs font-mono whitespace-pre-wrap" data-testid="raw-json">
          {json}
        </pre>
      </ScrollArea>
    </div>
  );
}
