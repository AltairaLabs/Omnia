/*
Copyright 2026.

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

// Package contract publishes the version of the omnia.runtime.v1 gRPC contract
// that this build implements.
//
// A custom runtime reports the contract version it was built against so the
// control plane can detect a runtime that has fallen behind. The value mirrors
// the `// Contract-Version:` marker in api/proto/runtime/v1/runtime.proto;
// version_test.go asserts the two agree.
//
// Bump the minor version for additive changes (new message, new optional
// field, new oneof variant); bump the major version for any change that would
// break an existing conformant runtime.
package contract

// Version is the omnia.runtime.v1 contract version implemented by this build.
const Version = "1.2.0"
