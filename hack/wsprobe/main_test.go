package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestProbeReceivesDone(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
		for _, f := range []string{
			`{"type":"connected"}`, `{"type":"chunk","content":"hi"}`, `{"type":"done"}`,
		} {
			_ = c.WriteMessage(websocket.TextMessage, []byte(f))
		}
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	if err := probe(url, "hello", 5*time.Second); err != nil {
		t.Fatalf("probe returned error, want nil: %v", err)
	}
}

func TestProbeTimesOutWithoutDone(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_, _, _ = c.ReadMessage()
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"chunk"}`))
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	if err := probe(url, "hello", 500*time.Millisecond); err == nil {
		t.Fatal("probe returned nil, want timeout error")
	}
}
