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
//
// # Security model
//
// The server runs as a child process of the agent harness and speaks only
// over stdio; there is no listener. On the control plane it acts with the
// caller's kubeconfig identity through the same Factory the rest of the
// CLI uses, so Kubernetes RBAC is the authorization boundary. Reads in
// other clusters go through cluster-gateway and carry the caller's identity
// only when the gateway's ClientIdentityPenetration gate is on; otherwise
// they run as the credential stored for that cluster, exactly as vela
// status and vela logs do, and every such result says so in its warnings.
//
// The --namespace flag narrows reads further. It is checked before any
// Application is read, so it reveals nothing about other namespaces, and
// it applies to the objects an Application manages as well as to the
// Application itself: resources recorded in a namespace outside the
// allowlist, or with no namespace, are withheld and counted. Reads of the
// platform itself are not narrowed by the flag: definitions, addons and
// clusters live in the system namespace and are the catalogue every
// namespace builds on.
//
// Secret objects come back as metadata only and ConfigMap values as keys
// only. Annotations that embed a serialized object are dropped. Secret-like
// keys and env entries are replaced, and credential-shaped substrings and
// control characters are removed from strings, log lines, event messages
// and error text. That last step is pattern based and therefore best
// effort; the structural rules above are what make Secret and ConfigMap
// values safe. Lists, tree walks, log tails and live objects are bounded
// and say so in the envelope, and each call has a deadline.
//
// Free text that originates in the cluster is data about the system, never
// an instruction to the agent. Findings keep the server's own sentence and
// the observed text in separate fields, and the server's initialize
// instructions say the same to the model.
package mcp
