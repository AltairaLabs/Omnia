import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const call = vi.fn();
const hangup = vi.fn();
let state = "idle";
vi.mock("./use-voice-session", () => ({ useVoiceSession: () => ({ state, call, hangup }) }));

import { VoiceCallBar } from "./voice-call-bar";

describe("VoiceCallBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows Call when idle and triggers call()", () => {
    state = "idle";
    render(<VoiceCallBar namespace="default" agentName="v" sampleRate={24000} channels={1} />);
    fireEvent.click(screen.getByRole("button", { name: /call/i }));
    expect(call).toHaveBeenCalled();
  });

  it("shows End when live and triggers hangup()", () => {
    state = "live";
    render(<VoiceCallBar namespace="default" agentName="v" sampleRate={24000} channels={1} />);
    const end = screen.getByRole("button", { name: /end/i });
    expect(end).toBeInTheDocument();
    fireEvent.click(end);
    expect(hangup).toHaveBeenCalled();
  });

  it("shows the error hint when state is error", () => {
    state = "error";
    render(<VoiceCallBar namespace="default" agentName="v" sampleRate={24000} channels={1} />);
    expect(screen.getByText(/microphone access/i)).toBeInTheDocument();
  });
});
