package runtime_test

import (
	"context"
	"log"
	"net"

	rt "github.com/altairalabs/omnia/pkg/runtime"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

type echoRuntime struct{}

func (echoRuntime) Capabilities() []string { return []string{contract.CapabilityClientTools} }

func (echoRuntime) Converse(_ context.Context, turn rt.Turn, emit rt.Emitter) error {
	if err := emit.Chunk("you said: " + turn.Content); err != nil {
		return err
	}
	return emit.Done(rt.Done{Final: "you said: " + turn.Content})
}

// ExampleServe shows a minimal conformant runtime. It is compiled as
// documentation but not executed (Serve blocks).
func ExampleServe() {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatal(err)
	}
	if err := rt.Serve(lis, echoRuntime{}); err != nil {
		log.Fatal(err)
	}
}
