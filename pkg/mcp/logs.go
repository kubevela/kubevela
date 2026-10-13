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

package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

const (
	// defaultTailLines is what get_logs returns per pod when unspecified.
	defaultTailLines int64 = 200
	// maxTailLines caps a request so a single call cannot flood a context
	// window. Larger windows belong in the observability MCP.
	maxTailLines int64 = 2000
	// maxLogBytesPerPod bounds the bytes read per pod regardless of lines.
	maxLogBytesPerPod = 256 << 10
	// maxLogPods bounds how many pods one call tails when none is named.
	maxLogPods = 5
	podKind    = "Pod"
)

// PodLogsFunc opens a log stream for one pod. Options.PodLogs lets tests
// supply one without an API server.
type PodLogsFunc func(ctx context.Context, cfg *rest.Config, cluster, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error)

// GetLogsInput selects pods of an Application and how much to read.
type GetLogsInput struct {
	ApplicationRef
	Component    string `json:"component,omitempty" jsonschema:"Only pods of this component."`
	Cluster      string `json:"cluster,omitempty" jsonschema:"Only pods in this cluster. Use local for the control plane."`
	Pod          string `json:"pod,omitempty" jsonschema:"One pod by name. Empty tails up to five pods."`
	Container    string `json:"container,omitempty" jsonschema:"Container name. Empty uses the first container and lists the others."`
	TailLines    int64  `json:"tailLines,omitempty" jsonschema:"Lines per pod from the end. Default 200, maximum 2000."`
	SinceSeconds int64  `json:"sinceSeconds,omitempty" jsonschema:"Only lines newer than this many seconds."`
	Previous     bool   `json:"previous,omitempty" jsonschema:"Read the previous container instance, for crash loops."`
}

// PodLogs is the tail of one container in one pod.
type PodLogs struct {
	Cluster    string   `json:"cluster"`
	Namespace  string   `json:"namespace"`
	Pod        string   `json:"pod"`
	Component  string   `json:"component,omitempty"`
	Container  string   `json:"container"`
	Containers []string `json:"containers,omitempty"`
	Phase      string   `json:"phase,omitempty"`
	Lines      []string `json:"lines"`
	Truncated  bool     `json:"truncated"`
	Error      string   `json:"error,omitempty"`
}

// GetLogsOutput is the structured result of get_logs.
type GetLogsOutput struct {
	Application string    `json:"application"`
	Namespace   string    `json:"namespace"`
	Pods        []PodLogs `json:"pods"`
	Evidence    []string  `json:"evidence"`
	Meta        Meta      `json:"meta"`
}

// podTarget is a pod found in the resource tree.
type podTarget struct {
	cluster, namespace, name, component, phase string
	containers                                 []string
}

func (d *deps) getLogs(ctx context.Context, _ *sdk.CallToolRequest, in GetLogsInput) (*sdk.CallToolResult, GetLogsOutput, error) {
	out := GetLogsOutput{Pods: []PodLogs{}}
	app, err := d.loadApplication(ctx, in.ApplicationRef)
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	out.Evidence = []string{applicationEvidence(app)}

	applied, err := d.listAppliedResources(ctx, app, in.Component, in.Cluster)
	if err != nil {
		return nil, out, err
	}
	targets := collectPods(applied)
	if in.Pod != "" {
		targets = selectPod(targets, in.Pod)
		if len(targets) == 0 {
			return nil, out, fmt.Errorf("pod %q is not managed by application %s/%s", in.Pod, app.Namespace, app.Name)
		}
	}
	if len(targets) == 0 {
		return nil, out, fmt.Errorf("application %s/%s has no pods matching the filter", app.Namespace, app.Name)
	}
	meta := d.meta(ctx)
	if len(targets) > maxLogPods {
		meta.Warnings = append(meta.Warnings, fmt.Sprintf("%d pods matched; only the first %d were read, name a pod to read another", len(targets), maxLogPods))
		meta.Truncated = true
		targets = targets[:maxLogPods]
	}
	opts := logOptions(in)
	for _, t := range targets {
		pl := d.readPodLogs(ctx, t, in.Container, opts)
		if pl.Truncated {
			meta.Truncated = true
		}
		out.Pods = append(out.Pods, pl)
		out.Evidence = append(out.Evidence, fmt.Sprintf("Pod %s/%s container %s in cluster %s", t.namespace, t.name, pl.Container, t.cluster))
	}
	out.Meta = meta
	return nil, out, nil
}

func logOptions(in GetLogsInput) corev1.PodLogOptions {
	tail := in.TailLines
	if tail <= 0 {
		tail = defaultTailLines
	}
	if tail > maxTailLines {
		tail = maxTailLines
	}
	opts := corev1.PodLogOptions{TailLines: &tail, Previous: in.Previous}
	if in.SinceSeconds > 0 {
		since := in.SinceSeconds
		opts.SinceSeconds = &since
	}
	return opts
}

// readPodLogs streams one container's tail under the byte budget and
// redacts each line.
func (d *deps) readPodLogs(ctx context.Context, t podTarget, container string, opts corev1.PodLogOptions) PodLogs {
	pl := PodLogs{
		Cluster:    t.cluster,
		Namespace:  t.namespace,
		Pod:        t.name,
		Component:  t.component,
		Containers: t.containers,
		Phase:      t.phase,
		Lines:      []string{},
	}
	switch {
	case container != "":
		pl.Container = container
	case len(t.containers) > 0:
		pl.Container = t.containers[0]
	}
	opts.Container = pl.Container
	rc, err := d.opts.PodLogs(ctx, d.f.Config(), t.cluster, t.namespace, t.name, &opts)
	if err != nil {
		pl.Error = redactLine(err.Error())
		return pl
	}
	defer func() { _ = rc.Close() }()
	pl.Lines, pl.Truncated, err = readLines(rc, maxLogBytesPerPod)
	if err != nil {
		pl.Error = redactLine(err.Error())
	}
	return pl
}

// readLines reads up to budget bytes of newline-separated text, redacting
// each line. Truncated is set when the stream had more.
func readLines(r io.Reader, budget int) ([]string, bool, error) {
	lines := []string{}
	lr := &io.LimitedReader{R: r, N: int64(budget) + 1}
	sc := bufio.NewScanner(lr)
	sc.Buffer(make([]byte, 64<<10), budget)
	read := 0
	for sc.Scan() {
		line := sc.Text()
		read += len(line) + 1
		if read > budget {
			return lines, true, nil
		}
		lines = append(lines, redactLine(line))
	}
	return lines, false, sc.Err()
}

// collectPods walks every tree for Pod nodes. The collector already holds
// the live Pod object on each node, so containers and phase cost nothing.
func collectPods(applied []querytypes.AppliedResource) []podTarget {
	var pods []podTarget
	seen := map[string]bool{}
	var walk func(component string, n *querytypes.ResourceTreeNode)
	walk = func(component string, n *querytypes.ResourceTreeNode) {
		if n == nil {
			return
		}
		if n.Kind == podKind {
			key := clusterName(n.Cluster) + "/" + n.Namespace + "/" + n.Name
			if !seen[key] {
				seen[key] = true
				pods = append(pods, podTargetFromNode(component, n))
			}
		}
		for _, leaf := range n.LeafNodes {
			walk(component, leaf)
		}
	}
	for i := range applied {
		walk(applied[i].Component, applied[i].ResourceTree)
	}
	sort.SliceStable(pods, func(i, j int) bool {
		if pods[i].cluster != pods[j].cluster {
			return pods[i].cluster < pods[j].cluster
		}
		return pods[i].name < pods[j].name
	})
	return pods
}

func podTargetFromNode(component string, n *querytypes.ResourceTreeNode) podTarget {
	t := podTarget{cluster: clusterName(n.Cluster), namespace: n.Namespace, name: n.Name, component: component}
	if n.Object != nil {
		t.containers = containerNames(n.Object)
		t.phase, _, _ = unstructured.NestedString(n.Object.Object, "status", "phase")
	}
	return t
}

func containerNames(obj *unstructured.Unstructured) []string {
	var names []string
	for _, field := range []string{"initContainers", "containers"} {
		items, _, _ := unstructured.NestedSlice(obj.Object, "spec", field)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if name, ok := m["name"].(string); ok && name != "" {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func selectPod(targets []podTarget, name string) []podTarget {
	for _, t := range targets {
		if t.name == name {
			return []podTarget{t}
		}
	}
	return nil
}

// streamPodLogs is the default PodLogsFunc: a clientset on the factory's
// config, which already carries the multicluster transport, reading the
// pod's log subresource in its cluster.
func (d *deps) streamPodLogs(ctx context.Context, cfg *rest.Config, cluster, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	if cfg == nil {
		return nil, errNoConfig
	}
	cs, err := d.clientset(cfg)
	if err != nil {
		return nil, err
	}
	if cluster != "" && cluster != localCluster {
		ctx = multicluster.ContextWithClusterName(ctx, cluster)
	}
	return cs.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
}

func (d *deps) clientset(cfg *rest.Config) (kubernetes.Interface, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cs != nil {
		return d.cs, nil
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}
	d.cs = cs
	return cs, nil
}
