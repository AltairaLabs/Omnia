/*
Copyright 2026 Altaira Labs.

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

package agent

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/facade"
)

const routeKeyPrefix = "rt:route:"

type redisRouteStore struct{ client redis.UniversalClient }

// NewRedisRouteStore returns a facade.RouteStore backed by Redis.
func NewRedisRouteStore(client redis.UniversalClient) facade.RouteStore {
	return &redisRouteStore{client: client}
}

func (r *redisRouteStore) PutRoute(ctx context.Context, sessionID, addr string, ttl time.Duration) error {
	return r.client.Set(ctx, routeKeyPrefix+sessionID, addr, ttl).Err()
}

func (r *redisRouteStore) DeleteRoute(ctx context.Context, sessionID string) error {
	return r.client.Del(ctx, routeKeyPrefix+sessionID).Err()
}
