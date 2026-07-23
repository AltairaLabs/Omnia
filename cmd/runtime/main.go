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

// Command runtime is the Omnia agent runtime container: a gRPC server that
// wraps the LLM provider via the PromptKit SDK. Its entire construction and
// serving lifecycle now lives in the public pkg/runtime/promptkit facade, so
// the first-party runtime and any downstream, separate-repo PromptKit runtime
// ride the same public rails. main() is a thin shell: load operator-injected
// config, serve until signalled, shut down.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/altairalabs/omnia/pkg/runtime/promptkit"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt, err := promptkit.FromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = rt.Close() }()

	if err := rt.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "runtime serve error: %v\n", err)
		os.Exit(1)
	}
}
