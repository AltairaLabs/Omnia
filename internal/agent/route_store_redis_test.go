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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisRouteStore_PutGetDelete(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rs := NewRedisRouteStore(client)

	if err := rs.PutRoute(context.Background(), "sid", "10.0.0.5:8080", time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, _ := mr.Get("rt:route:sid"); got != "10.0.0.5:8080" {
		t.Fatalf("stored value = %q", got)
	}
	if ttl := mr.TTL("rt:route:sid"); ttl <= 0 {
		t.Fatalf("PutRoute must set a TTL (EX); got %v", ttl)
	}
	if err := rs.DeleteRoute(context.Background(), "sid"); err != nil {
		t.Fatal(err)
	}
	if mr.Exists("rt:route:sid") {
		t.Fatalf("key not deleted")
	}
}
