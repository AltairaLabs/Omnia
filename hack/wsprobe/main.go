// Command wsprobe dials an Omnia agent WebSocket, sends one message, and exits
// zero only when it observes a {"type":"done"} frame. Used by the install smoke test.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// probe dials url, sends a message frame, and returns nil once a "done" frame
// arrives, or an error on dial/read failure or timeout.
func probe(url, message string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", url, err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"type": "message", "content": message}); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read before done frame: %w", err)
		}
		fmt.Printf("recv: %s\n", data)
		var frame struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &frame) == nil && frame.Type == "done" {
			return nil
		}
	}
}

func main() {
	url := flag.String("url", "ws://localhost:8080/ws?agent=my-assistant", "agent WebSocket URL")
	message := flag.String("message", "Hello, who are you?", "message content to send")
	timeout := flag.Duration("timeout", 30*time.Second, "overall read timeout")
	flag.Parse()

	if err := probe(*url, *message, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "wsprobe: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("wsprobe: received done frame — roundtrip OK")
}
