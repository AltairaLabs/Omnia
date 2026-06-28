/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// redisKV adapts a go-redis UniversalClient to privacy.KVCache.
type redisKV struct {
	client goredis.UniversalClient
	prefix string
}

// Compile-time interface check.
var _ privacy.KVCache = (*redisKV)(nil)

// Get returns the value for key. Returns (found=false, err=nil) on a cache miss.
func (r *redisKV) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.client.Get(ctx, r.prefix+key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Set stores value at key with the given TTL.
func (r *redisKV) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return r.client.Set(ctx, r.prefix+key, val, ttl).Err()
}

// Del removes key from the cache.
func (r *redisKV) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.prefix+key).Err()
}
