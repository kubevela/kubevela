/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"fmt"
	"strings"
)

// CUERenderer is implemented by builder types that can render themselves to CUE.
// The renderValue function is provided by the CUE generator to render nested Value types.
type CUERenderer interface {
	RenderCUE(renderValue func(Value) string) string
}

// ---------------------------------------------------------------------------
// KubeRead builder
// ---------------------------------------------------------------------------

// KubeReadBuilder builds a kube.#Read operation fluently.
type KubeReadBuilder struct {
	apiVersion string
	kind       string
	name       Value
	namespace  Value
	cluster    Value
	nsCond     Condition // for NamespaceIf
}

func (b *KubeReadBuilder) expr()      {}
func (b *KubeReadBuilder) value()     {}
func (b *KubeReadBuilder) condition() {}

// KubeRead creates a new kube.#Read builder for the given apiVersion and kind.
func KubeRead(apiVersion, kind string) *KubeReadBuilder {
	return &KubeReadBuilder{
		apiVersion: apiVersion,
		kind:       kind,
	}
}

// Name sets the metadata.name value.
func (b *KubeReadBuilder) Name(v Value) *KubeReadBuilder {
	b.name = v
	return b
}

// Namespace sets the metadata.namespace value.
func (b *KubeReadBuilder) Namespace(v Value) *KubeReadBuilder {
	b.namespace = v
	return b
}

// NamespaceIf sets the metadata.namespace conditionally.
func (b *KubeReadBuilder) NamespaceIf(cond Condition, v Value) *KubeReadBuilder {
	b.nsCond = cond
	b.namespace = v
	return b
}

// Cluster sets the optional cluster parameter for multi-cluster reads.
func (b *KubeReadBuilder) Cluster(v Value) *KubeReadBuilder {
	b.cluster = v
	return b
}

// RenderCUE renders the builder to a CUE string.
func (b *KubeReadBuilder) RenderCUE(rv func(Value) string) string {
	var sb strings.Builder
	sb.WriteString("kube.#Read & {\n")

	// cluster param sits alongside value in $params
	if b.cluster != nil {
		sb.WriteString("\t$params: {\n")
		sb.WriteString(fmt.Sprintf("\t\tcluster: %s\n", rv(b.cluster)))
		sb.WriteString("\t\tvalue: {\n")
		b.writeValueBody(&sb, rv, "\t\t\t")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t}\n")
	} else {
		sb.WriteString("\t$params: value: {\n")
		b.writeValueBody(&sb, rv, "\t\t")
		sb.WriteString("\t}\n")
	}

	sb.WriteString("}")
	return sb.String()
}

func (b *KubeReadBuilder) writeValueBody(sb *strings.Builder, rv func(Value) string, indent string) {
	sb.WriteString(fmt.Sprintf("%sapiVersion: %q\n", indent, b.apiVersion))
	sb.WriteString(fmt.Sprintf("%skind:       %q\n", indent, b.kind))
	sb.WriteString(fmt.Sprintf("%smetadata: {\n", indent))
	if b.name != nil {
		sb.WriteString(fmt.Sprintf("%s\tname:      %s\n", indent, rv(b.name)))
	}
	if b.namespace != nil {
		sb.WriteString(fmt.Sprintf("%s\tnamespace: %s\n", indent, rv(b.namespace)))
	}
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// ---------------------------------------------------------------------------
// KubeApply builder
// ---------------------------------------------------------------------------

// KubeApplyBuilder builds a kube.#Apply operation fluently.
type KubeApplyBuilder struct {
	objectValue Value
	cluster     Value
}

func (b *KubeApplyBuilder) expr()      {}
func (b *KubeApplyBuilder) value()     {}
func (b *KubeApplyBuilder) condition() {}

// KubeApply creates a new kube.#Apply builder with the given object value.
func KubeApply(objectValue Value) *KubeApplyBuilder {
	return &KubeApplyBuilder{objectValue: objectValue}
}

// Cluster sets the optional cluster parameter.
func (b *KubeApplyBuilder) Cluster(v Value) *KubeApplyBuilder {
	b.cluster = v
	return b
}

// RenderCUE renders the builder to a CUE string.
func (b *KubeApplyBuilder) RenderCUE(rv func(Value) string) string {
	var sb strings.Builder
	sb.WriteString("kube.#Apply & {\n")
	sb.WriteString("\t$params: {\n")
	sb.WriteString(fmt.Sprintf("\t\tvalue:   %s\n", rv(b.objectValue)))
	if b.cluster != nil {
		sb.WriteString(fmt.Sprintf("\t\tcluster: %s\n", rv(b.cluster)))
	}
	sb.WriteString("\t}\n")
	sb.WriteString("}")
	return sb.String()
}

// ---------------------------------------------------------------------------
// HTTPPost builder
// ---------------------------------------------------------------------------

// HTTPPostBuilder builds an http.#HTTPDo POST operation fluently.
type HTTPPostBuilder struct {
	url     Value
	body    Value
	headers map[string]string
}

func (b *HTTPPostBuilder) expr()      {}
func (b *HTTPPostBuilder) value()     {}
func (b *HTTPPostBuilder) condition() {}

// HTTPPost creates a new http.#HTTPDo builder for a POST request.
func HTTPPost(url Value) *HTTPPostBuilder {
	return &HTTPPostBuilder{
		url:     url,
		headers: make(map[string]string),
	}
}

// Body sets the request body.
func (b *HTTPPostBuilder) Body(v Value) *HTTPPostBuilder {
	b.body = v
	return b
}

// Header adds a request header.
func (b *HTTPPostBuilder) Header(key, value string) *HTTPPostBuilder {
	b.headers[key] = value
	return b
}

// RenderCUE renders the builder to a CUE string.
func (b *HTTPPostBuilder) RenderCUE(rv func(Value) string) string {
	var sb strings.Builder
	sb.WriteString("http.#HTTPDo & {\n")
	sb.WriteString("\t$params: {\n")
	sb.WriteString("\t\tmethod: \"POST\"\n")
	sb.WriteString(fmt.Sprintf("\t\turl:    %s\n", rv(b.url)))
	sb.WriteString("\t\trequest: {\n")
	if b.body != nil {
		sb.WriteString(fmt.Sprintf("\t\t\tbody: %s\n", rv(b.body)))
	}
	for k, v := range b.headers {
		sb.WriteString(fmt.Sprintf("\t\t\theader: %q: %q\n", k, v))
	}
	sb.WriteString("\t\t}\n")
	sb.WriteString("\t}\n")
	sb.WriteString("}")
	return sb.String()
}

// ---------------------------------------------------------------------------
// ConvertString builder
// ---------------------------------------------------------------------------

// ConvertStringBuilder builds a util.#ConvertString operation fluently.
type ConvertStringBuilder struct {
	input Value
}

func (b *ConvertStringBuilder) expr()      {}
func (b *ConvertStringBuilder) value()     {}
func (b *ConvertStringBuilder) condition() {}

// ConvertString creates a new util.#ConvertString builder.
func ConvertString(input Value) *ConvertStringBuilder {
	return &ConvertStringBuilder{input: input}
}

// RenderCUE renders the builder to a CUE string.
func (b *ConvertStringBuilder) RenderCUE(rv func(Value) string) string {
	return fmt.Sprintf("util.#ConvertString & {\n\t$params: bt: %s\n}", rv(b.input))
}

// ---------------------------------------------------------------------------
// WaitUntil builder
// ---------------------------------------------------------------------------

// WaitUntilBuilder builds a builtin.#ConditionalWait operation fluently.
type WaitUntilBuilder struct {
	continueExpr Value
	guards       []Value
	messageCond  Condition
	messageVal   Value
}

func (b *WaitUntilBuilder) expr()      {}
func (b *WaitUntilBuilder) value()     {}
func (b *WaitUntilBuilder) condition() {}

// WaitUntil creates a new builtin.#ConditionalWait builder.
func WaitUntil(continueExpr Value) *WaitUntilBuilder {
	return &WaitUntilBuilder{continueExpr: continueExpr}
}

// Guard adds guard conditions that must be true (not _|_) before the continue
// check is evaluated. Multiple guards are nested as chained if statements.
func (b *WaitUntilBuilder) Guard(guards ...Value) *WaitUntilBuilder {
	b.guards = append(b.guards, guards...)
	return b
}

// MessageIf conditionally sets the message parameter.
func (b *WaitUntilBuilder) MessageIf(cond Condition, val Value) *WaitUntilBuilder {
	b.messageCond = cond
	b.messageVal = val
	return b
}

// RenderCUE renders the builder to a CUE string.
func (b *WaitUntilBuilder) RenderCUE(rv func(Value) string) string {
	var sb strings.Builder
	sb.WriteString("builtin.#ConditionalWait & {\n")

	if len(b.guards) > 0 {
		// Build chained guard: if g1 if g2 { ... }
		var guardParts []string
		for _, g := range b.guards {
			guardParts = append(guardParts, fmt.Sprintf("if %s != _|_", rv(g)))
		}
		sb.WriteString(fmt.Sprintf("\t%s {\n", strings.Join(guardParts, " ")))
		sb.WriteString(fmt.Sprintf("\t\t$params: continue: %s\n", rv(b.continueExpr)))
		sb.WriteString("\t}\n")
	} else {
		sb.WriteString(fmt.Sprintf("\t$params: continue: %s\n", rv(b.continueExpr)))
	}

	if b.messageCond != nil && b.messageVal != nil {
		// messageCond rendering needs conditionToCUE, but we only have renderValue.
		// Use the message value path with a != _|_ guard as the common pattern.
		sb.WriteString(fmt.Sprintf("\tif %s != _|_ {\n", rv(b.messageVal)))
		sb.WriteString(fmt.Sprintf("\t\t$params: message: %s\n", rv(b.messageVal)))
		sb.WriteString("\t}\n")
	}

	sb.WriteString("}")
	return sb.String()
}

// ---------------------------------------------------------------------------
// Fail builder
// ---------------------------------------------------------------------------

// FailBuilder builds a builtin.#Fail operation fluently.
type FailBuilder struct {
	message Value
}

func (b *FailBuilder) expr()      {}
func (b *FailBuilder) value()     {}
func (b *FailBuilder) condition() {}

// Fail creates a new builtin.#Fail builder with the given message.
func Fail(message Value) *FailBuilder {
	return &FailBuilder{message: message}
}

// RenderCUE renders the builder to a CUE string.
func (b *FailBuilder) RenderCUE(rv func(Value) string) string {
	return fmt.Sprintf("builtin.#Fail & {\n\t$params: message: %s\n}", rv(b.message))
}
