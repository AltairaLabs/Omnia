/*
Copyright 2025.

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

package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func TestReceiver_Export(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())
	receiver := NewReceiver(transformer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "Hi there!"))
	attrs = append(attrs, tokenAttrs(25, 10)...)
	span := makeSpan("grpc-conv-1", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("prod", "grpc-agent", span)

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{rs},
	}

	resp, err := receiver.Export(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, resp)

	sess := writer.sessions["grpc-conv-1"]
	require.NotNil(t, sess)
	assert.Equal(t, "grpc-agent", sess.AgentName)

	msgs := writer.messages["grpc-conv-1"]
	require.Len(t, msgs, 1)
	assert.Equal(t, "Hi there!", msgs[0].Content)
}

func TestReceiver_Export_EmptyRequest(t *testing.T) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())
	receiver := NewReceiver(transformer, logr.Discard())

	req := &coltracepb.ExportTraceServiceRequest{}

	resp, err := receiver.Export(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, writer.sessions)
}

func TestReceiver_Export_PartialFailure(t *testing.T) {
	writer := newMockWriter()
	writer.appendErr = assert.AnError

	transformer := NewTransformer(writer, logr.Discard())
	receiver := NewReceiver(transformer, logr.Discard())

	attrs := outputMsgAttrs(makeMessageValue("assistant", "Hello"))
	span := makeSpan("grpc-fail", uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("ns", "agent", span)

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{rs},
	}

	resp, err := receiver.Export(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
}
