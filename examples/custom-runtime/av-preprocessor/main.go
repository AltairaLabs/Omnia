/*
Copyright 2026.

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

// Command av-preprocessor is a minimal, conformant custom Omnia runtime that
// demonstrates an audio/video-preprocessing seam. It serves the
// omnia.runtime.v1.RuntimeService gRPC contract — advertising only the
// capabilities it actually implements, sending a RuntimeHello first, and passing
// the runtime-conformance suite — and shows how to wire PromptKit's real
// video-to-frames stage in front of a model. See the README for how to run it
// and the FFmpeg base-image requirement.
package main

import (
	"context"
	"io"
	"log"
	"net"
	"os"

	"github.com/AltairaLabs/PromptKit/runtime/pipeline/stage"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/pkg/runtime/contract"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

const greeting = "hello from av-preprocessor"

// capabilities is the honest set this runtime implements: it serves Invoke
// (function mode) and nothing else. It does NOT claim duplex_audio, client_tools,
// etc. — advertising a capability it could not back would fail conformance.
func capabilities() []string {
	return []string{contract.CapabilityInvoke}
}

// server implements omnia.runtime.v1.RuntimeService.
type server struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func (server) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{
		Healthy:         true,
		ContractVersion: contract.Version,
		Capabilities:    capabilities(),
	}, nil
}

// Converse sends a RuntimeHello as its first ServerMessage, then answers each
// turn with a canned chunk + done. A real runtime would run inbound video
// through buildVideoPreprocessStage before invoking its model.
func (s server) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if err := stream.Send(&runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_RuntimeHello{
		RuntimeHello: &runtimev1.RuntimeHello{Capabilities: capabilities()},
	}}); err != nil {
		return err
	}
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_Chunk{
			Chunk: &runtimev1.Chunk{Content: greeting},
		}}); err != nil {
			return err
		}
		if err := stream.Send(&runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_Done{
			Done: &runtimev1.Done{FinalContent: greeting},
		}}); err != nil {
			return err
		}
	}
}

func (server) Invoke(context.Context, *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error) {
	return &runtimev1.InvocationResponse{OutputJson: `{"message":"` + greeting + `"}`}, nil
}

func (server) HasConversation(
	context.Context, *runtimev1.HasConversationRequest,
) (*runtimev1.HasConversationResponse, error) {
	// This example keeps no durable context, so nothing is resumable.
	return &runtimev1.HasConversationResponse{State: runtimev1.ResumeState_RESUME_STATE_NOT_FOUND}, nil
}

// buildVideoPreprocessStage constructs PromptKit's real video-to-frames stage —
// the A/V-preprocessing seam. A production runtime feeds inbound video
// StreamElements through stage.Process(...) to extract frames before handing the
// text + frames to its model. Frame extraction shells out to `ffmpeg` on PATH,
// so a runtime that uses it needs a media-capable base image (see the README).
func buildVideoPreprocessStage() *stage.VideoToFramesStage {
	cfg := stage.DefaultVideoToFramesConfig()
	cfg.Mode = stage.FrameExtractionFPS
	cfg.TargetFPS = 1.0
	cfg.MaxFrames = 10
	cfg.OutputFormat = "jpeg"
	return stage.NewVideoToFramesStage(cfg)
}

func grpcPort() string {
	if p := os.Getenv("OMNIA_GRPC_PORT"); p != "" {
		return p
	}
	return "9000"
}

func main() {
	// Construct the A/V-preprocessing stage up front so it is ready to run on
	// inbound video turns.
	log.Printf("av-preprocessor: video preprocessing stage ready (%T)", buildVideoPreprocessStage())

	addr := ":" + grpcPort()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("av-preprocessor: listen %s: %v", addr, err)
	}
	gs := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(gs, server{})
	log.Printf("av-preprocessor: serving omnia.runtime.v1 on %s", addr)
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("av-preprocessor: serve: %v", err)
	}
}
