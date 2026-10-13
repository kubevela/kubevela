/*
Copyright 2026 The KubeVela Authors.

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

// Package mcp exposes KubeVela to AI agents over the Model Context Protocol.
//
// The server wraps code paths the vela CLI already has as a small set of
// deterministic tools. Tools are grouped into three classes: read tools are
// always registered, plan tools render and diff without writing, and write
// tools exist only when the server is started with the corresponding flag.
// Every response carries an envelope that states the versions the tool ran
// against and when the data was observed, so a model explains the system
// that is actually running rather than the one it remembers.
//
// Handlers never shell out to the vela binary and never call a language
// model. See design/vela-cli/kep-vela-mcp.md for the proposal this
// implements.
package mcp
