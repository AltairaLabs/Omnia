const MEDIA_ID_OFFSET = 20;
const MEDIA_ID_SIZE = 12;
const HEADER_SIZE = MEDIA_ID_OFFSET + MEDIA_ID_SIZE; // 32
const FLAG_IS_LAST = 0x04;
export const OMNI_MEDIA_CHUNK = 1;

export interface EncodeMediaFrameOptions {
  sessionId: string;
  sequence: number;
  isLast: boolean;
  mimeType: string;
  sampleRate: number;
  channels: number;
  codec: string;
  payload: ArrayBuffer;
}

export function encodeOmniMediaFrame(o: EncodeMediaFrameOptions): ArrayBuffer {
  const metaObj = {
    session_id: o.sessionId,
    mime_type: o.mimeType,
    sample_rate: o.sampleRate,
    channels: o.channels,
    codec: o.codec,
  };
  const metadata = new TextEncoder().encode(JSON.stringify(metaObj));
  const payload = new Uint8Array(o.payload);
  const total = HEADER_SIZE + metadata.length + payload.length;
  const buf = new ArrayBuffer(total);
  const view = new DataView(buf);
  const bytes = new Uint8Array(buf);

  bytes[0] = 0x4f; bytes[1] = 0x4d; bytes[2] = 0x4e; bytes[3] = 0x49; // "OMNI"
  view.setUint8(4, 1); // version
  view.setUint8(5, o.isLast ? FLAG_IS_LAST : 0); // flags
  view.setUint16(6, OMNI_MEDIA_CHUNK, false); // messageType, big-endian
  view.setUint32(8, metadata.length, false);
  view.setUint32(12, payload.length, false);
  view.setUint32(16, o.sequence >>> 0, false);
  // mediaID [MEDIA_ID_OFFSET : HEADER_SIZE] left zero-filled by ArrayBuffer default
  bytes.set(metadata, HEADER_SIZE);
  bytes.set(payload, HEADER_SIZE + metadata.length);
  return buf;
}

export interface DecodedFrame {
  messageType: number;
  sequence: number;
  isLast: boolean;
  metadata: Record<string, unknown>;
  payload: ArrayBuffer;
}

export function decodeOmniFrame(buf: ArrayBuffer): DecodedFrame {
  const view = new DataView(buf);
  const bytes = new Uint8Array(buf);
  const magic = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3]);
  if (magic !== "OMNI") {
    throw new Error(`decodeOmniFrame: invalid magic "${magic}", expected "OMNI"`);
  }
  const version = view.getUint8(4);
  if (version !== 1) {
    throw new Error(`decodeOmniFrame: unsupported version ${version}, expected 1`);
  }
  const flags = view.getUint8(5);
  const messageType = view.getUint16(6, false);
  const metadataLen = view.getUint32(8, false);
  const payloadLen = view.getUint32(12, false);
  const sequence = view.getUint32(16, false);
  const metaStart = HEADER_SIZE;
  const metaBytes = new Uint8Array(buf, metaStart, metadataLen);
  const metadata = metadataLen > 0
    ? (JSON.parse(new TextDecoder().decode(metaBytes)) as Record<string, unknown>)
    : {};
  const payload = buf.slice(metaStart + metadataLen, metaStart + metadataLen + payloadLen);
  return { messageType, sequence, isLast: (flags & FLAG_IS_LAST) !== 0, metadata, payload };
}
