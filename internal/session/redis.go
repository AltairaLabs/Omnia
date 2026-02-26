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
// This uses a Lua script to atomically check existence, update metadata,
// and set expiry on all related keys, eliminating the TOCTOU race between
// EXISTS and EXPIRE.
func (r *RedisStore) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	now := time.Now()
	var expiresAt string
	var ttlSeconds int64
	if ttl > 0 {
		expiresAt = now.Add(ttl).Format(time.RFC3339Nano)
		ttlSeconds = int64(ttl.Seconds())
		if ttlSeconds < 1 {
			ttlSeconds = 1
		}
	}

	keys := []string{
		r.sessionKey(sessionID),
		r.messagesKey(sessionID),
		r.stateKey(sessionID),
	}

	result, err := refreshTTLScript.Run(ctx, r.client, keys,
		ttlSeconds,
		now.Format(time.RFC3339Nano),
		expiresAt,
	).Int64()
	if err != nil {
		return fmt.Errorf("failed to refresh TTL: %w", err)
	}

	if result == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// Lua script for atomic TTL refresh.
// KEYS[1] = session key, KEYS[2] = messages key, KEYS[3] = state key
// ARGV[1] = ttl in seconds (0 means persist), ARGV[2] = now (RFC3339Nano),
// ARGV[3] = expiresAt (RFC3339Nano, empty to clear)
// Returns: 0 = not found, 1 = success
var refreshTTLScript = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then
	return 0
end

local session = cjson.decode(data)
session['updatedAt'] = ARGV[2]

if ARGV[3] ~= "" then
	session['expiresAt'] = ARGV[3]
else
	session['expiresAt'] = nil
end

local encoded = cjson.encode(session)
local ttl = tonumber(ARGV[1])

for i = 1, 3 do
	if ttl > 0 then
		redis.call('EXPIRE', KEYS[i], ttl)
	else
		redis.call('PERSIST', KEYS[i])
	end
end

if ttl > 0 then
	redis.call('SET', KEYS[1], encoded, 'EX', ttl)
else
	redis.call('SET', KEYS[1], encoded)
end

return 1
`)

// UpdateSessionStats atomically increments session-level counters.
// This uses a Lua script to perform a read-modify-write in a single
// atomic operation, preventing concurrent updates from overwriting each other.
func (r *RedisStore) UpdateSessionStats(ctx context.Context, sessionID string, update SessionStatsUpdate) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	now := time.Now().Format(time.RFC3339Nano)

	result, err := updateStatsScript.Run(ctx, r.client,
		[]string{r.sessionKey(sessionID)},
		update.AddInputTokens,
		update.AddOutputTokens,
		update.AddCostUSD,
		update.AddToolCalls,
		update.AddMessages,
		string(update.SetStatus),
		now,
	).Int64()
	if err != nil {
		return fmt.Errorf("failed to update session stats: %w", err)
	}

	switch result {
	case 0:
		return ErrSessionNotFound
	case 2:
		return ErrSessionExpired
	}

	return nil
}

// Lua script for atomic update of session stats.
// KEYS[1] = session key
// ARGV[1] = addInputTokens, ARGV[2] = addOutputTokens, ARGV[3] = addCostUSD
// ARGV[4] = addToolCalls, ARGV[5] = addMessages, ARGV[6] = setStatus (empty = no change)
// ARGV[7] = now (RFC3339Nano)
// Returns: 0 = not found, 1 = success, 2 = expired
var updateStatsScript = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then
	return 0
end

local session = cjson.decode(data)

-- Check expiry: if expiresAt is set (non-empty, non-zero-time) and in the past, return expired
local expiresAt = session['expiresAt']
if expiresAt and expiresAt ~= "" and string.sub(expiresAt, 1, 4) ~= "0001" then
	local now = ARGV[7]
	if expiresAt < now then
		return 2
	end
end

session['totalInputTokens'] = (session['totalInputTokens'] or 0) + tonumber(ARGV[1])
session['totalOutputTokens'] = (session['totalOutputTokens'] or 0) + tonumber(ARGV[2])
session['estimatedCostUSD'] = (session['estimatedCostUSD'] or 0) + tonumber(ARGV[3])
session['toolCallCount'] = (session['toolCallCount'] or 0) + tonumber(ARGV[4])
session['messageCount'] = (session['messageCount'] or 0) + tonumber(ARGV[5])

if ARGV[6] ~= "" then
	session['status'] = ARGV[6]
end

session['updatedAt'] = ARGV[7]

local encoded = cjson.encode(session)
local ttl = redis.call('TTL', KEYS[1])
if ttl > 0 then
	redis.call('SET', KEYS[1], encoded, 'EX', ttl)
else
	redis.call('SET', KEYS[1], encoded)
end
return 1
`)

// Close releases resources held by the store.
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// updateSessionTimestamp atomically updates the session's UpdatedAt timestamp
// using a Lua script to prevent concurrent read-modify-write races.
func (r *RedisStore) updateSessionTimestamp(ctx context.Context, sessionID string) error {
	now := time.Now().Format(time.RFC3339Nano)

	result, err := updateTimestampScript.Run(ctx, r.client,
		[]string{r.sessionKey(sessionID)},
		now,
	).Int64()
	if err != nil {
		return fmt.Errorf("failed to update session timestamp: %w", err)
	}

	if result == 0 {
		return fmt.Errorf(errGetSession, ErrSessionNotFound)
	}

	return nil
}

// Lua script for atomic update of session timestamp.
// KEYS[1] = session key
// ARGV[1] = now (RFC3339Nano)
// Returns 0 if key does not exist, 1 on success.
var updateTimestampScript = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then
	return 0
end

local session = cjson.decode(data)
session['updatedAt'] = ARGV[1]

local encoded = cjson.encode(session)
local ttl = redis.call('TTL', KEYS[1])
if ttl > 0 then
	redis.call('SET', KEYS[1], encoded, 'EX', ttl)
else
	redis.call('SET', KEYS[1], encoded)
end
return 1
`)

// Ensure RedisStore implements Store interface.
var _ Store = (*RedisStore)(nil)
