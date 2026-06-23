"use client";

import { Phone, PhoneOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ServerMessage } from "../../../types/websocket";
import { useVoiceSession } from "./use-voice-session";

export interface VoiceCallBarProps {
  namespace: string;
  agentName: string;
  sampleRate: number;
  channels: number;
  onServerMessage?: (m: ServerMessage) => void;
}

export function VoiceCallBar(props: VoiceCallBarProps) {
  const { state, call, hangup } = useVoiceSession(props);
  const idle = state === "idle" || state === "error";

  if (idle) {
    return (
      <div className="p-4 border-t bg-muted/30 flex justify-center">
        <Button onClick={call} aria-label="Call" data-testid="voice-call-button">
          <Phone className="h-4 w-4 mr-2" /> Call
        </Button>
        {state === "error" && (
          <span className="ml-3 text-sm text-destructive">
            Voice unavailable — check microphone access.
          </span>
        )}
      </div>
    );
  }

  const label = state === "live" ? "Live" : "Connecting…";
  return (
    <div className="p-4 border-t bg-muted/30 flex items-center justify-center gap-3" data-testid="voice-live">
      <span className="inline-flex items-center gap-2 text-sm">
        <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" /> {label}
      </span>
      <Button variant="destructive" onClick={hangup} aria-label="End call" data-testid="voice-end-button">
        <PhoneOff className="h-4 w-4 mr-2" /> End
      </Button>
    </div>
  );
}
