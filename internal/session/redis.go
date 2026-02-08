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

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes
	sessionKeyPrefix  = "session:"
	messagesKeySuffix = ":messages"
	stateKeySuffix    = ":state"

	// Error format strings
	errMarshalSession   = "failed to marshal session: %w"
	errUnmarshalSession = "failed to unmarshal session: %w"
	errGetSession       = "failed to get session: %w"
	errCheckExistence   = "failed to check session existence: %w"
)

// RedisConfig contains configuration for the Redis session store.
type RedisConfig struct {
	// Addr is the Redis server address (host:port).
	Addr string
	// Password is the Redis password (empty for no auth).
	Password string
	// DB is the Redis database number.
	DB int
	// KeyPrefix is an optional prefix for all keys.
	KeyPrefix string
}

// ParseRedisURL parses a Redis URL and returns a RedisConfig.
// Supported formats:
//   - redis://[:password@]host:port[/db]
//   - redis://[:password@]host:port[/db]?key_prefix=prefix
func ParseRedisURL(url string) (RedisConfig, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return RedisConfig{}, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	return RedisConfig{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	}, nil
}

// RedisStore implements Store using Redis for persistent storage.
type RedisStore struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisStore creates a new Redis session store.
func NewRedisStore(cfg RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStore{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
	}, nil
}

// NewRedisStoreFromClient creates a new Redis session store from an existing client.
func NewRedisStoreFromClient(client *redis.Client, keyPrefix string) *RedisStore {
	return &RedisStore{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

func (r *RedisStore) sessionKey(sessionID string) string {
	return r.keyPrefix + sessionKeyPrefix + sessionID
}

func (r *RedisStore) messagesKey(sessionID string) string {
	return r.sessionKey(sessionID) + messagesKeySuffix
}

func (r *RedisStore) stateKey(sessionID string) string {
	return r.sessionKey(sessionID) + stateKeySuffix
}

// CreateSession creates a new session and returns it.
func (r *RedisStore) CreateSession(ctx context.Context, opts CreateSessionOptions) (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:        uuid.New().String(),
		AgentName: opts.AgentName,
		Namespace: opts.Namespace,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
		State:     make(map[string]string),
	}

	if opts.TTL > 0 {
		session.ExpiresAt = now.Add(opts.TTL)
	}

	if opts.InitialState != nil {
		for k, v := range opts.InitialState {
			session.State[k] = v
		}
	}

	// Serialize session metadata
	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf(errMarshalSession, err)
	}

	pipe := r.client.Pipeline()

	// Set session data
	if opts.TTL > 0 {
		pipe.Set(ctx, r.sessionKey(session.ID), data, opts.TTL)
	} else {
		pipe.Set(ctx, r.sessionKey(session.ID), data, 0)
	}

	// Initialize state hash if we have initial state
	if len(opts.InitialState) > 0 {
		stateArgs := make([]interface{}, 0, len(opts.InitialState)*2)
		for k, v := range opts.InitialState {
			stateArgs = append(stateArgs, k, v)
		}
		pipe.HSet(ctx, r.stateKey(session.ID), stateArgs...)
		if opts.TTL > 0 {
			pipe.Expire(ctx, r.stateKey(session.ID), opts.TTL)
		}
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a session by ID.
func (r *RedisStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	data, err := r.client.Get(ctx, r.sessionKey(sessionID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf(errGetSession, err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf(errUnmarshalSession, err)
	}

	// Load messages
	messages, err := r.GetMessages(ctx, sessionID)
	if err != nil && err != ErrSessionNotFound {
		return nil, err
	}
	session.Messages = messages

	// Load state
	state, err := r.client.HGetAll(ctx, r.stateKey(sessionID)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get session state: %w", err)
	}
	if state != nil {
		session.State = state
	}

	return &session, nil
}

// DeleteSession removes a session.
func (r *RedisStore) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	// Check if session exists first
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf(errCheckExistence, err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	// Delete all related keys
	keys := []string{
		r.sessionKey(sessionID),
		r.messagesKey(sessionID),
		r.stateKey(sessionID),
	}

	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// AppendMessage adds a message to the session's conversation history.
func (r *RedisStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	// Check if session exists
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf(errCheckExistence, err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	// Generate message ID if not provided
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Append to messages list
	if err := r.client.RPush(ctx, r.messagesKey(sessionID), data).Err(); err != nil {
		return fmt.Errorf("failed to append message: %w", err)
	}

	// Update session timestamp
	if err := r.updateSessionTimestamp(ctx, sessionID); err != nil {
		return err
	}

	return nil
}

// GetMessages retrieves all messages for a session.
func (r *RedisStore) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	// Check if session exists
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return nil, fmt.Errorf(errCheckExistence, err)
	}
	if exists == 0 {
		return nil, ErrSessionNotFound
	}

	// Get all messages
	data, err := r.client.LRange(ctx, r.messagesKey(sessionID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	messages := make([]Message, len(data))
	for i, d := range data {
		if err := json.Unmarshal([]byte(d), &messages[i]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
	}

	return messages, nil
}

// SetState sets a state value for the session.
func (r *RedisStore) SetState(ctx context.Context, sessionID string, key, value string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	// Check if session exists
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf(errCheckExistence, err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	// Set state value
	if err := r.client.HSet(ctx, r.stateKey(sessionID), key, value).Err(); err != nil {
		return fmt.Errorf("failed to set state: %w", err)
	}

	// Update session timestamp
	if err := r.updateSessionTimestamp(ctx, sessionID); err != nil {
		return err
	}

	return nil
}

// GetState retrieves a state value from the session.
func (r *RedisStore) GetState(ctx context.Context, sessionID string, key string) (string, error) {
	if sessionID == "" {
		return "", ErrInvalidSessionID
	}

	// Check if session exists
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return "", fmt.Errorf("failed to check session existence: %w", err)
	}
	if exists == 0 {
		return "", ErrSessionNotFound
	}

	// Get state value
	value, err := r.client.HGet(ctx, r.stateKey(sessionID), key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("failed to get state: %w", err)
	}

	return value, nil
}

// RefreshTTL extends the session's expiration time.
func (r *RedisStore) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	// Check if session exists
	exists, err := r.client.Exists(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf(errCheckExistence, err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}

	pipe := r.client.Pipeline()

	keys := []string{
		r.sessionKey(sessionID),
		r.messagesKey(sessionID),
		r.stateKey(sessionID),
	}

	for _, key := range keys {
		if ttl > 0 {
			pipe.Expire(ctx, key, ttl)
		} else {
			pipe.Persist(ctx, key)
		}
	}

	// Update session metadata
	data, err := r.client.Get(ctx, r.sessionKey(sessionID)).Bytes()
	if err != nil {
		return fmt.Errorf(errGetSession, err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf(errUnmarshalSession, err)
	}

	session.UpdatedAt = time.Now()
	if ttl > 0 {
		session.ExpiresAt = time.Now().Add(ttl)
	} else {
		session.ExpiresAt = time.Time{}
	}

	updatedData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf(errMarshalSession, err)
	}

	if ttl > 0 {
		pipe.Set(ctx, r.sessionKey(sessionID), updatedData, ttl)
	} else {
		pipe.Set(ctx, r.sessionKey(sessionID), updatedData, 0)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to refresh TTL: %w", err)
	}

	return nil
}

// UpdateSessionStats atomically increments session-level counters.
func (r *RedisStore) UpdateSessionStats(ctx context.Context, sessionID string, update SessionStatsUpdate) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	data, err := r.client.Get(ctx, r.sessionKey(sessionID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return ErrSessionNotFound
		}
		return fmt.Errorf(errGetSession, err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf(errUnmarshalSession, err)
	}

	if session.IsExpired() {
		return ErrSessionExpired
	}

	session.TotalInputTokens += int64(update.AddInputTokens)
	session.TotalOutputTokens += int64(update.AddOutputTokens)
	session.EstimatedCostUSD += update.AddCostUSD
	session.ToolCallCount += update.AddToolCalls
	session.MessageCount += update.AddMessages

	if update.SetStatus != "" {
		session.Status = update.SetStatus
	}

	session.UpdatedAt = time.Now()

	updatedData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf(errMarshalSession, err)
	}

	// Preserve TTL if set
	ttl, err := r.client.TTL(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf("failed to get TTL: %w", err)
	}

	if ttl > 0 {
		return r.client.Set(ctx, r.sessionKey(sessionID), updatedData, ttl).Err()
	}
	return r.client.Set(ctx, r.sessionKey(sessionID), updatedData, 0).Err()
}

// Close releases resources held by the store.
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// updateSessionTimestamp updates the session's UpdatedAt timestamp.
func (r *RedisStore) updateSessionTimestamp(ctx context.Context, sessionID string) error {
	data, err := r.client.Get(ctx, r.sessionKey(sessionID)).Bytes()
	if err != nil {
		return fmt.Errorf(errGetSession, err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf(errUnmarshalSession, err)
	}

	session.UpdatedAt = time.Now()

	updatedData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf(errMarshalSession, err)
	}

	// Preserve TTL if set
	ttl, err := r.client.TTL(ctx, r.sessionKey(sessionID)).Result()
	if err != nil {
		return fmt.Errorf("failed to get TTL: %w", err)
	}

	if ttl > 0 {
		return r.client.Set(ctx, r.sessionKey(sessionID), updatedData, ttl).Err()
	}
	return r.client.Set(ctx, r.sessionKey(sessionID), updatedData, 0).Err()
}

// Ensure RedisStore implements Store interface.
var _ Store = (*RedisStore)(nil)
