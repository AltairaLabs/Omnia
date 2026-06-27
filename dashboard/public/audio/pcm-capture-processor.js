// Converts Float32 mic samples to Int16 PCM and posts them to the main thread.
// Registered name: "pcm-capture-processor". Served from /audio/.
class PcmCaptureProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0] && inputs[0][0];
    if (!channel) return true;
    const pcm = new Int16Array(channel.length);
    for (let i = 0; i < channel.length; i++) {
      const s = Math.max(-1, Math.min(1, channel[i]));
      pcm[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
    }
    this.port.postMessage({ pcm: pcm.buffer }, [pcm.buffer]);
    return true;
  }
}
registerProcessor("pcm-capture-processor", PcmCaptureProcessor);
