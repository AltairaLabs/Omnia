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
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Compile-time interface check.
var _ providers.HotCacheProvider = (*Provider)(nil)

// Provider implements providers.HotCacheProvider using Redis.
type Provider struct {
	client     goredis.UniversalClient
	keyPrefix  string
	maxMsgs    int
	ownsClient bool
}

// New creates a Provider that owns the underlying Redis client. The client is
// created from cfg and verified with a PING. Close will shut down the client.
func New(cfg Config) (*Provider, error) {
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("redis: at least one address is required")
	}

	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = defaultKeyPrefix
	}

	opts := &goredis.UniversalOptions{
		Addrs:        cfg.Addrs,
		Password:     cfg.Password,
		DB:           cfg.DB,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		TLSConfig:    cfg.TLS,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}

	client := goredis.NewUniversalClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: failed to connect: %w", err)
	}

	return &Provider{
		client:     client,
		keyPrefix:  prefix,
		maxMsgs:    cfg.MaxMessagesPerSession,
		ownsClient: true,
	}, nil
}

// NewFromClient wraps an existing UniversalClient. Close is a no-op because
// the caller retains ownership of the client.
func NewFromClient(client goredis.UniversalClient, opts Options) *Provider {
	prefix := opts.KeyPrefix
	if prefix == "" {
		prefix = defaultKeyPrefix
	}
	return &Provider{
		client:     client,
		keyPrefix:  prefix,
		maxMsgs:    opts.MaxMessagesPerSession,
		ownsClient: false,
	}
}

// --- key helpers -----------------------------------------------------------

func (p *Provider) sessionKey(id string) string {
	return p.keyPrefix + "session:{" + id + "}"
}

func (p *Provider) messagesKey(id string) string {
	return p.keyPrefix + "session:{" + id + "}:msgs"
}

// --- HotCacheProvider implementation ---------------------------------------

func (p *Provider) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	data, err := p.client.Get(ctx, p.sessionKey(sessionID)).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, session.ErrSessionNotFound
		}
		return nil, fmt.Errorf("redis: get session: %w", err)
	}

	var s session.Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("redis: unmarshal session: %w", err)
	}
	return &s, nil
}

func (p *Provider) SetSession(ctx context.Context, s *session.Session, ttl time.Duration) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("redis: marshal session: %w", err)
	}
	if err := p.client.Set(ctx, p.sessionKey(s.ID), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set session: %w", err)
	}
	return nil
}

func (p *Provider) DeleteSession(ctx context.Context, sessionID string) error {
	exists, err := p.client.Exists(ctx, p.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf("redis: check existence: %w", err)
	}
	if exists == 0 {
		return session.ErrSessionNotFound
	}

	pipe := p.client.Pipeline()
	pipe.Del(ctx, p.sessionKey(sessionID))
	pipe.Del(ctx, p.messagesKey(sessionID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis: delete session: %w", err)
	}
	return nil
}

func (p *Provider) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	sessionKey := p.sessionKey(sessionID)
	msgsKey := p.messagesKey(sessionID)

	exists, err := p.client.Exists(ctx, sessionKey).Result()
	if err != nil {
		return fmt.Errorf("redis: check existence: %w", err)
	}
	if exists == 0 {
		return session.ErrSessionNotFound
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("redis: marshal message: %w", err)
	}

	pipe := p.client.Pipeline()
	pipe.RPush(ctx, msgsKey, data)

	if p.maxMsgs > 0 {
		pipe.LTrim(ctx, msgsKey, int64(-p.maxMsgs), -1)
	}

	// Sync messages key TTL with session key TTL.
	ttlCmd := p.client.TTL(ctx, sessionKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis: append message: %w", err)
	}

	// Apply TTL from session key to messages key.
	ttl, err := ttlCmd.Result()
	if err != nil {
		return fmt.Errorf("redis: get session ttl: %w", err)
	}

	switch {
	case ttl > 0:
		if err := p.client.Expire(ctx, msgsKey, ttl).Err(); err != nil {
			return fmt.Errorf("redis: sync messages ttl: %w", err)
		}
	case ttl == -1:
		// Session has no expiry — make sure messages key also has no expiry.
		if err := p.client.Persist(ctx, msgsKey).Err(); err != nil {
			return fmt.Errorf("redis: persist messages key: %w", err)
		}
	}
	// ttl == -2 means key doesn't exist; ignore (should not happen here).

	return nil
}

func (p *Provider) GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]*session.Message, error) {
	exists, err := p.client.Exists(ctx, p.sessionKey(sessionID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: check existence: %w", err)
	}
	if exists == 0 {
		return nil, session.ErrSessionNotFound
	}

	var start int64
	if limit > 0 {
		start = int64(-limit)
	}

	data, err := p.client.LRange(ctx, p.messagesKey(sessionID), start, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: lrange messages: %w", err)
	}

	msgs := make([]*session.Message, 0, len(data))
	for _, d := range data {
		var m session.Message
		if err := json.Unmarshal([]byte(d), &m); err != nil {
			return nil, fmt.Errorf("redis: unmarshal message: %w", err)
		}
		msgs = append(msgs, &m)
	}
	return msgs, nil
}

func (p *Provider) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	sessionKey := p.sessionKey(sessionID)
	msgsKey := p.messagesKey(sessionID)

	exists, err := p.client.Exists(ctx, sessionKey).Result()
	if err != nil {
		return fmt.Errorf("redis: check existence: %w", err)
	}
	if exists == 0 {
		return session.ErrSessionNotFound
	}

	pipe := p.client.Pipeline()
	if ttl > 0 {
		pipe.Expire(ctx, sessionKey, ttl)
		pipe.Expire(ctx, msgsKey, ttl)
	} else {
		pipe.Persist(ctx, sessionKey)
		pipe.Persist(ctx, msgsKey)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis: refresh ttl: %w", err)
	}
	return nil
}

func (p *Provider) Invalidate(ctx context.Context, sessionID string) error {
	pipe := p.client.Pipeline()
	pipe.Del(ctx, p.sessionKey(sessionID))
	pipe.Del(ctx, p.messagesKey(sessionID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis: invalidate: %w", err)
	}
	return nil
}

// RedisClient returns the underlying Redis client. This allows other components
// (e.g. event publishers) to share the same connection without owning it.
func (p *Provider) RedisClient() goredis.UniversalClient {
	return p.client
}

func (p *Provider) Ping(ctx context.Context) error {
	return p.client.Ping(ctx).Err()
}

func (p *Provider) Close() error {
	if p.ownsClient {
		return p.client.Close()
	}
	return nil
}
