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

	"github.com/go-logr/logr"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Receiver implements the OTLP gRPC TraceService.
type Receiver struct {
	coltracepb.UnimplementedTraceServiceServer
	transformer *Transformer
	log         logr.Logger
}

// NewReceiver creates a new gRPC OTLP trace receiver.
func NewReceiver(transformer *Transformer, log logr.Logger) *Receiver {
	return &Receiver{
		transformer: transformer,
		log:         log.WithName("otlp-receiver"),
	}
}

// Export implements TraceServiceServer.Export by delegating to the transformer.
func (r *Receiver) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	processed, err := r.transformer.ProcessExport(ctx, req.GetResourceSpans())
	if err != nil {
		r.log.Error(err, "partial export failure", "processed", processed)
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}
