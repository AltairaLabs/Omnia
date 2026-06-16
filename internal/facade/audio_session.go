/*
Copyright 2025-2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package facade

import (
	"context"
	"sync"
)

// AudioSessionStart holds negotiated audio parameters for a duplex session.
type AudioSessionStart struct {
	Codec      string
	SampleRate int32
	Channels   int32
}

// duplexSink abstracts the runtime duplex stream so the facade stays decoupled
// from internal/agent. The real implementation (next task) wraps the runtime
// gRPC client; tests use a fake.
type duplexSink interface {
	Start(ctx context.Context, s *AudioSessionStart) error
	SendAudio(data []byte, seq uint32, isLast bool) error
	Close() error
}

// audioSession owns one persistent duplex stream for a WebSocket connection.
// It is created lazily on the first inbound media chunk and torn down on
// connection close.
type audioSession struct {
	sessionID string
	sink      duplexSink
	writer    ResponseWriter
	mu        sync.Mutex
	started   bool
}

func newAudioSession(sessionID string, sink duplexSink, writer ResponseWriter) *audioSession {
	return &audioSession{sessionID: sessionID, sink: sink, writer: writer}
}

// start opens the duplex stream on the sink. Idempotent — safe to call
// on every inbound frame; the sink's Start is invoked exactly once.
func (a *audioSession) start(ctx context.Context, s *AudioSessionStart) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return nil
	}
	if err := a.sink.Start(ctx, s); err != nil {
		return err
	}
	a.started = true
	return nil
}

// pushAudio forwards raw audio bytes to the sink.
func (a *audioSession) pushAudio(data []byte, seq uint32, isLast bool) error {
	return a.sink.SendAudio(data, seq, isLast)
}

// handleInboundFrame maps an OMNI media-chunk binary frame to a pushAudio
// call, extracting sequence number and the FlagIsLast flag from the header.
func (a *audioSession) handleInboundFrame(frame *BinaryFrame) error {
	isLast := frame.Header.Flags&FlagIsLast != 0
	return a.pushAudio(frame.Payload, frame.Header.Sequence, isLast)
}

func (a *audioSession) close() error { return a.sink.Close() }
