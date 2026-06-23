import { describe, it, expect } from "vitest";
import { encodeOmniMediaFrame, decodeOmniFrame, OMNI_MEDIA_CHUNK } from "./omni-binary";

describe("omni-binary", () => {
  it("round-trips a media frame", () => {
    const payload = new Uint8Array([1, 2, 3, 4]).buffer;
    const frame = encodeOmniMediaFrame({
      sessionId: "s1", sequence: 7, isLast: true, mimeType: "audio/pcm",
      sampleRate: 24000, channels: 1, codec: "pcm", payload,
    });
    const got = decodeOmniFrame(frame);
    expect(got.messageType).toBe(OMNI_MEDIA_CHUNK);
    expect(got.sequence).toBe(7);
    expect(got.isLast).toBe(true);
    expect(got.metadata.session_id).toBe("s1");
    expect(got.metadata.sample_rate).toBe(24000);
    expect(new Uint8Array(got.payload)).toEqual(new Uint8Array([1, 2, 3, 4]));
  });

  it("writes the OMNI magic and version", () => {
    const frame = encodeOmniMediaFrame({
      sessionId: "s", sequence: 0, isLast: false, mimeType: "audio/pcm",
      sampleRate: 24000, channels: 1, codec: "pcm", payload: new ArrayBuffer(0),
    });
    const head = new Uint8Array(frame, 0, 5);
    expect(String.fromCharCode(...head.slice(0, 4))).toBe("OMNI");
    expect(head[4]).toBe(1);
  });

  it("clears the is_last flag when false", () => {
    const frame = encodeOmniMediaFrame({
      sessionId: "s", sequence: 1, isLast: false, mimeType: "audio/pcm",
      sampleRate: 24000, channels: 1, codec: "pcm", payload: new Uint8Array([9]).buffer,
    });
    expect(decodeOmniFrame(frame).isLast).toBe(false);
  });

  it("throws on bad magic bytes", () => {
    const frame = encodeOmniMediaFrame({
      sessionId: "s", sequence: 0, isLast: false, mimeType: "audio/pcm",
      sampleRate: 24000, channels: 1, codec: "pcm", payload: new ArrayBuffer(0),
    });
    // Corrupt the magic bytes
    new Uint8Array(frame).set([0x42, 0x41, 0x44, 0x21]); // "BAD!"
    expect(() => decodeOmniFrame(frame)).toThrow(/invalid magic/);
  });

  it("throws on unsupported version", () => {
    const frame = encodeOmniMediaFrame({
      sessionId: "s", sequence: 0, isLast: false, mimeType: "audio/pcm",
      sampleRate: 24000, channels: 1, codec: "pcm", payload: new ArrayBuffer(0),
    });
    // Patch version byte (offset 4) to 2
    new DataView(frame).setUint8(4, 2);
    expect(() => decodeOmniFrame(frame)).toThrow(/unsupported version/);
  });
});
