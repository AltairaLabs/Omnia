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

// Command runtime-conformance runs the omnia.runtime.v1 protocol conformance
// suite against a runtime's gRPC endpoint and exits non-zero if it is not
// conformant. A runtime author points it at their container:
//
//	runtime-conformance --addr localhost:9000
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/altairalabs/omnia/pkg/runtime/conformance"
)

func main() {
	addr := flag.String("addr", "", "runtime gRPC address (host:port) — required")
	timeout := flag.Duration("timeout", 30*time.Second, "overall probe timeout")
	flag.Parse()
	if *addr == "" {
		fmt.Fprintln(os.Stderr, "error: --addr is required")
		flag.Usage()
		os.Exit(2)
	}
	os.Exit(run(*addr, *timeout))
}

// run dials addr, executes the suite, prints the report, and returns the process
// exit code (0 conformant, 1 non-conformant, 2 dial error).
func run(addr string, timeout time.Duration) int {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: dial %s: %v\n", addr, err)
		return 2
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res := conformance.Run(ctx, conformance.Config{Conn: conn})
	printResult(os.Stdout, res)
	return exitCode(res)
}

// printResult writes a per-check table and a summary line to w.
func printResult(w io.Writer, res conformance.Result) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range res.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Status, c.Detail)
	}
	_ = tw.Flush()
	summary := "\nFAIL: runtime is not conformant"
	if res.Passed {
		summary = "\nPASS: runtime is conformant"
	}
	_, _ = fmt.Fprintln(w, summary)
}

// exitCode maps a Result to a process exit code.
func exitCode(res conformance.Result) int {
	if res.Passed {
		return 0
	}
	return 1
}
