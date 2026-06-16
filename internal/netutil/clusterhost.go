/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

// Package netutil holds small, dependency-free networking helpers shared across
// services. It deliberately imports only the standard library so any package
// (core or ee) can use it without creating an import cycle.
package netutil

import (
	"net"
	"strings"
)

// IsClusterInternalHost reports whether host is reachable only inside the
// cluster / local network — the trusted network on which Kubernetes itself
// ships projected ServiceAccount tokens. Callers use it to decide whether
// plaintext transport (http / ws) is acceptable for sending a credential: the
// dashboard and agent facades are served over plaintext on ClusterIP Services
// in every Omnia install, so requiring TLS for those in-cluster endpoints would
// break service-to-service auth everywhere. External/public hosts return false
// so credentials are never sent over plaintext across an untrusted network.
//
// Recognised as internal:
//   - loopback ("localhost", 127.0.0.0/8, ::1) and link-local
//   - RFC1918 / unique-local private IPs
//   - bare single-label hostnames (e.g. "omnia-dashboard" — same-namespace svc)
//   - the ".svc" suffix (e.g. "omnia-dashboard.omnia.svc")
//   - the ".local" suffix, which covers the conventional ".svc.cluster.local"
//     Service FQDN and ".cluster.local" cluster domain
//
// Matching uses suffixes, NOT a ".svc." substring: a substring check would
// treat an attacker-controlled "anything.svc.evil.com" as internal and leak the
// credential. Two-label "<svc>.<ns>" forms are ambiguous against public domains
// and are NOT auto-trusted; non-".local" custom cluster domains must opt in
// explicitly at the call site.
func IsClusterInternalHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// Bare single-label hostname → a same-namespace Service short name.
	if !strings.Contains(h, ".") {
		return true
	}
	if strings.HasSuffix(h, ".local") {
		return true
	}
	return strings.HasSuffix(h, ".svc")
}
