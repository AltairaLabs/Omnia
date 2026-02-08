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

package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/session"
)

func benchProvider(b *testing.B) *Provider {
	b.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(mr.Close)

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	return NewFromClient(client, DefaultOptions())
}

func benchSession() *session.Session {
	now := time.Now()
	return &session.Session{
		ID:                "bench-sess",
		AgentName:         "bench-agent",
		Namespace:         "bench-ns",
		CreatedAt:         now,
		UpdatedAt:         now,
		Status:            session.SessionStatusActive,
		WorkspaceName:     "bench-ws",
		MessageCount:      42,
		TotalInputTokens:  5000,
		TotalOutputTokens: 8000,
		Tags:              []string{"bench"},
	}
}

func BenchmarkGetSession(b *testing.B) {
	p := benchProvider(b)
	ctx := context.Background()
	s := benchSession()
	_ = p.SetSession(ctx, s, 0)

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.GetSession(ctx, s.ID)
	}
}

func BenchmarkSetSession(b *testing.B) {
	p := benchProvider(b)
	ctx := context.Background()
	s := benchSession()

	b.Run("NoTTL", func(b *testing.B) {
		for b.Loop() {
			_ = p.SetSession(ctx, s, 0)
		}
	})

	b.Run("WithTTL", func(b *testing.B) {
		for b.Loop() {
			_ = p.SetSession(ctx, s, time.Hour)
		}
	})
}

func BenchmarkAppendMessage(b *testing.B) {
	p := benchProvider(b)
	ctx := context.Background()
	s := benchSession()
	_ = p.SetSession(ctx, s, 0)

	msg := &session.Message{
		ID:          "bench-msg",
		Role:        session.RoleUser,
		Content:     "benchmark message content that is reasonably sized for a typical user turn",
		Timestamp:   time.Now(),
		SequenceNum: 1,
	}

	b.ResetTimer()
	for b.Loop() {
		_ = p.AppendMessage(ctx, s.ID, msg)
	}
}

func BenchmarkGetRecentMessages(b *testing.B) {
	p := benchProvider(b)
	ctx := context.Background()
	s := benchSession()
	_ = p.SetSession(ctx, s, 0)

	// Seed 200 messages.
	for i := 1; i <= 200; i++ {
		_ = p.AppendMessage(ctx, s.ID, &session.Message{
			ID:          fmt.Sprintf("msg-%d", i),
			Role:        session.RoleUser,
			Content:     "benchmark message content that is reasonably sized for a typical user turn",
			Timestamp:   time.Now(),
			SequenceNum: int32(i),
		})
	}

	for _, limit := range []int{10, 50, 200} {
		b.Run(fmt.Sprintf("Limit_%d", limit), func(b *testing.B) {
			for b.Loop() {
				_, _ = p.GetRecentMessages(ctx, s.ID, limit)
			}
		})
	}
}
