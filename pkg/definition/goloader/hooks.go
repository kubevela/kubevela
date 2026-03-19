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

package goloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// DefaultScriptTimeout is the default timeout for script execution
	DefaultScriptTimeout = 30 * time.Second
	// DefaultWaitTimeout is the default timeout for waiting on resources
	DefaultWaitTimeout = 5 * time.Minute
	// DefaultPollInterval is the polling interval when waiting for resources
	DefaultPollInterval = 2 * time.Second
)

// HookExecutor handles the execution of module hooks
type HookExecutor struct {
	// Client is the Kubernetes client for applying manifests
	Client client.Client
	// ModulePath is the base path of the module
	ModulePath string
	// Namespace is the target namespace for resources
	Namespace string
	// DryRun indicates whether to perform a dry run
	DryRun bool
	// Streams provides output streams for logging
	Streams util.IOStreams
}

// HookExecutionStats contains statistics from hook execution
type HookExecutionStats struct {
	// TotalDuration is the total time spent executing hooks
	TotalDuration time.Duration
	// HookDetails contains per-hook timing information
	HookDetails []HookDetail
	// ResourcesCreated is the number of resources created by path hooks
	ResourcesCreated int
	// ResourcesUpdated is the number of resources updated by path hooks
	ResourcesUpdated int
	// OptionalFailed is the number of optional hooks that failed
	OptionalFailed int
}

// HookDetail contains information about a single hook execution
type HookDetail struct {
	// Name is the hook name (path or script)
	Name string
	// Duration is how long the hook took to execute
	Duration time.Duration
	// Wait indicates if this hook waited for resources
	Wait bool
	// ResourcesCreated is resources created by this hook
	ResourcesCreated int
	// ResourcesUpdated is resources updated by this hook
	ResourcesUpdated int
}

// NewHookExecutor creates a new HookExecutor
func NewHookExecutor(c client.Client, modulePath, namespace string, dryRun bool, streams util.IOStreams) *HookExecutor {
	return &HookExecutor{
		Client:     c,
		ModulePath: modulePath,
		Namespace:  namespace,
		DryRun:     dryRun,
		Streams:    streams,
	}
}

// ExecuteHooks runs a list of hooks in order and returns execution statistics
func (e *HookExecutor) ExecuteHooks(ctx context.Context, phase string, hooks []Hook) (*HookExecutionStats, error) {
	stats := &HookExecutionStats{}

	if len(hooks) == 0 {
		return stats, nil
	}

	totalStart := time.Now()
	e.Streams.Infof("\nExecuting %s hooks...\n", phase)

	for i, hook := range hooks {
		hookName := fmt.Sprintf("%s[%d]", phase, i)
		displayName := hook.Path
		if hook.Path != "" {
			hookName = fmt.Sprintf("%s: %s", hookName, hook.Path)
		} else if hook.Script != "" {
			hookName = fmt.Sprintf("%s: %s", hookName, hook.Script)
			displayName = hook.Script
		}

		e.Streams.Infof("  Running %s...\n", hookName)

		hookStart := time.Now()
		var err error
		var created, updated int

		if hook.Path != "" {
			created, updated, err = e.executePathHook(ctx, hook)
		} else if hook.Script != "" {
			err = e.executeScriptHook(ctx, hook)
		}

		hookDuration := time.Since(hookStart)

		// Record hook detail
		detail := HookDetail{
			Name:             displayName,
			Duration:         hookDuration,
			Wait:             hook.Wait,
			ResourcesCreated: created,
			ResourcesUpdated: updated,
		}
		stats.HookDetails = append(stats.HookDetails, detail)
		stats.ResourcesCreated += created
		stats.ResourcesUpdated += updated

		if err != nil {
			if hook.Optional {
				e.Streams.Infof("    Warning: %s failed (optional): %v\n", hookName, err)
				stats.OptionalFailed++
				continue
			}
			stats.TotalDuration = time.Since(totalStart)
			return stats, fmt.Errorf("hook %s failed: %w", hookName, err)
		}

		e.Streams.Infof("    %s completed successfully\n", hookName)
	}

	stats.TotalDuration = time.Since(totalStart)
	e.Streams.Infof("%s hooks completed\n", phase)
	return stats, nil
}

// executePathHook applies YAML manifests from a directory
// Returns the number of resources created, updated, and any error
func (e *HookExecutor) executePathHook(ctx context.Context, hook Hook) (created, updated int, err error) {
	fullPath := filepath.Join(e.ModulePath, hook.Path)

	// Get list of YAML files, sorted alphabetically
	files, err := getYAMLFiles(fullPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list files in %s: %w", hook.Path, err)
	}

	if len(files) == 0 {
		e.Streams.Infof("    No YAML files found in %s\n", hook.Path)
		return 0, 0, nil
	}

	// Apply each file
	var appliedObjects []*unstructured.Unstructured
	for _, file := range files {
		objs, fileCreated, fileUpdated, applyErr := e.applyManifestFile(ctx, file)
		if applyErr != nil {
			return created, updated, fmt.Errorf("failed to apply %s: %w", filepath.Base(file), applyErr)
		}
		appliedObjects = append(appliedObjects, objs...)
		created += fileCreated
		updated += fileUpdated
	}

	// Wait for resources if requested
	if hook.Wait && !e.DryRun && len(appliedObjects) > 0 {
		timeout := parseTimeout(hook.Timeout, DefaultWaitTimeout)
		if waitErr := e.waitForResources(ctx, appliedObjects, timeout, hook.WaitFor); waitErr != nil {
			return created, updated, fmt.Errorf("timed out waiting for resources: %w", waitErr)
		}
	}

	return created, updated, nil
}

// executeScriptHook runs a shell script
func (e *HookExecutor) executeScriptHook(ctx context.Context, hook Hook) error {
	fullPath := filepath.Join(e.ModulePath, hook.Script)

	// Parse timeout
	timeout := parseTimeout(hook.Timeout, DefaultScriptTimeout)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if e.DryRun {
		e.Streams.Infof("    [dry-run] Would execute: %s\n", hook.Script)
		return nil
	}

	// Make script executable
	if err := os.Chmod(fullPath, 0755); err != nil { //nolint:gosec // G302: 0755 required for script execution
		return fmt.Errorf("failed to make script executable: %w", err)
	}

	// Execute the script
	cmd := exec.CommandContext(ctx, fullPath) //nolint:gosec // G204: Script path is from trusted module.yaml hooks config
	cmd.Dir = e.ModulePath
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("MODULE_PATH=%s", e.ModulePath),
		fmt.Sprintf("NAMESPACE=%s", e.Namespace),
	)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include output in error message
		errMsg := fmt.Sprintf("script failed: %v", err)
		if stderr.Len() > 0 {
			errMsg += fmt.Sprintf("\nstderr: %s", stderr.String())
		}
		if stdout.Len() > 0 {
			errMsg += fmt.Sprintf("\nstdout: %s", stdout.String())
		}
		return errors.New(errMsg)
	}

	// Print script output
	if stdout.Len() > 0 {
		e.Streams.Infof("    Output: %s\n", strings.TrimSpace(stdout.String()))
	}

	return nil
}

// applyManifestFile reads and applies all resources from a YAML file
// Returns the applied objects, count of created resources, count of updated resources, and any error
func (e *HookExecutor) applyManifestFile(ctx context.Context, filePath string) ([]*unstructured.Unstructured, int, int, error) {
	content, err := os.ReadFile(filePath) //nolint:gosec // G304: File path is from trusted module hook directory
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML documents
	reader := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 4096)
	var objects []*unstructured.Unstructured
	var created, updated int

	for {
		obj := &unstructured.Unstructured{}
		if err := reader.Decode(obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, created, updated, fmt.Errorf("failed to decode YAML: %w", err)
		}

		// Skip empty documents
		if len(obj.Object) == 0 {
			continue
		}

		// Set namespace if not specified and resource is namespaced
		if obj.GetNamespace() == "" && e.Namespace != "" {
			// Note: We don't know if it's cluster-scoped, so we set namespace
			// and let the API server reject if inappropriate
			obj.SetNamespace(e.Namespace)
		}

		if e.DryRun {
			e.Streams.Infof("    [dry-run] Would apply: %s %s/%s\n",
				obj.GetKind(), obj.GetNamespace(), obj.GetName())
			objects = append(objects, obj)
			created++ // Count as created for dry-run purposes
			continue
		}

		// Try to create, update if already exists
		err = e.Client.Create(ctx, obj)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				// Get existing object and update
				existing := &unstructured.Unstructured{}
				existing.SetGroupVersionKind(obj.GroupVersionKind())
				if getErr := e.Client.Get(ctx, client.ObjectKeyFromObject(obj), existing); getErr != nil {
					return nil, created, updated, fmt.Errorf("failed to get existing %s %s: %w", obj.GetKind(), obj.GetName(), getErr)
				}
				obj.SetResourceVersion(existing.GetResourceVersion())
				if updateErr := e.Client.Update(ctx, obj); updateErr != nil {
					return nil, created, updated, fmt.Errorf("failed to update %s %s: %w", obj.GetKind(), obj.GetName(), updateErr)
				}
				e.Streams.Infof("    Updated %s %s/%s\n", obj.GetKind(), obj.GetNamespace(), obj.GetName())
				updated++
			} else {
				return nil, created, updated, fmt.Errorf("failed to create %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		} else {
			e.Streams.Infof("    Created %s %s/%s\n", obj.GetKind(), obj.GetNamespace(), obj.GetName())
			created++
		}

		objects = append(objects, obj)
	}

	return objects, created, updated, nil
}

// waitForResources waits for all applied resources to be ready
func (e *HookExecutor) waitForResources(ctx context.Context, objects []*unstructured.Unstructured, timeout time.Duration, waitFor string) error {
	if waitFor != "" {
		e.Streams.Infof("    Waiting for resources (condition: %s, timeout: %s)...\n", waitFor, timeout)
	} else {
		e.Streams.Infof("    Waiting for resources to be ready (timeout: %s)...\n", timeout)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, obj := range objects {
		if err := e.waitForResource(ctx, obj, waitFor); err != nil {
			return err
		}
	}

	return nil
}

// waitForResource waits for a single resource to be ready
func (e *HookExecutor) waitForResource(ctx context.Context, obj *unstructured.Unstructured, waitFor string) error {
	key := client.ObjectKeyFromObject(obj)
	gvk := obj.GroupVersionKind()

	return wait.PollUntilContextCancel(ctx, DefaultPollInterval, true, func(ctx context.Context) (bool, error) {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(gvk)
		if err := e.Client.Get(ctx, key, current); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil // Resource doesn't exist yet, keep waiting
			}
			return false, err // Propagate other errors
		}

		// If a custom waitFor expression is provided, evaluate it
		if waitFor != "" {
			return evaluateWaitFor(current, waitFor)
		}

		// Otherwise, check for common readiness conditions
		ready, found := isResourceReady(current)
		if found {
			return ready, nil
		}

		// If no status, assume ready (for CRDs and similar)
		return true, nil
	})
}

// isResourceReady checks if a resource is ready based on its status
func isResourceReady(obj *unstructured.Unstructured) (ready bool, found bool) {
	status, exists, _ := unstructured.NestedMap(obj.Object, "status")
	if !exists {
		return false, false
	}

	// Check for conditions array (common pattern)
	conditions, exists, _ := unstructured.NestedSlice(status, "conditions")
	if exists {
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(cond, "type")
			condStatus, _, _ := unstructured.NestedString(cond, "status")

			// Check for Ready or Available conditions
			if (condType == "Ready" || condType == "Available" || condType == "Established") && condStatus == "True" {
				return true, true
			}
		}
		// Found conditions but none are ready
		return false, true
	}

	// Check for phase field (Pods, PVs, etc.)
	phase, exists, _ := unstructured.NestedString(status, "phase")
	if exists {
		return phase == "Running" || phase == "Bound" || phase == "Active", true
	}

	return false, false
}

// simpleConditionPattern matches simple condition names like "Ready", "Available", "Established"
var simpleConditionPattern = regexp.MustCompile(`^[A-Z][a-zA-Z]*$`)

// evaluateWaitFor evaluates a waitFor expression against a resource.
// It supports two formats:
// 1. Simple condition name (e.g., "Ready", "Established") - checks status.conditions
// 2. CUE expression (e.g., "status.replicas == status.readyReplicas") - evaluated against the resource
func evaluateWaitFor(obj *unstructured.Unstructured, waitFor string) (bool, error) {
	// Check if it's a simple condition name
	if isSimpleConditionName(waitFor) {
		return checkCondition(obj, waitFor)
	}

	// Otherwise, treat it as a CUE expression
	return evaluateCUEExpression(obj, waitFor)
}

// isSimpleConditionName checks if the waitFor string is a simple condition name
func isSimpleConditionName(waitFor string) bool {
	return simpleConditionPattern.MatchString(waitFor)
}

// checkCondition checks if a specific condition is True in the resource's status
func checkCondition(obj *unstructured.Unstructured, conditionName string) (bool, error) {
	status, exists, _ := unstructured.NestedMap(obj.Object, "status")
	if !exists {
		return false, nil // Keep waiting, status not yet present
	}

	conditions, exists, _ := unstructured.NestedSlice(status, "conditions")
	if !exists {
		return false, nil // Keep waiting, conditions not yet present
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")

		if condType == conditionName && condStatus == "True" {
			return true, nil
		}
	}

	return false, nil // Condition not found or not True
}

// evaluateCUEExpression evaluates a CUE expression against a resource
// The resource is available as the root context, so expressions like
// "status.replicas == status.readyReplicas" or "status.phase == \"Running\""
// can be used.
func evaluateCUEExpression(obj *unstructured.Unstructured, expr string) (bool, error) {
	// Convert the unstructured object to JSON
	jsonBytes, err := json.Marshal(obj.Object)
	if err != nil {
		return false, fmt.Errorf("failed to marshal resource to JSON: %w", err)
	}

	// Create CUE context and compile the resource
	ctx := cuecontext.New()
	resourceValue := ctx.CompileBytes(jsonBytes)
	if resourceValue.Err() != nil {
		return false, fmt.Errorf("failed to compile resource as CUE: %w", resourceValue.Err())
	}

	// Compile and evaluate the expression against the resource
	// We create a CUE expression that references fields from the resource
	exprValue := ctx.CompileString(expr, cue.Scope(resourceValue))
	if exprValue.Err() != nil {
		return false, fmt.Errorf("failed to compile waitFor expression %s: %w", expr, exprValue.Err())
	}

	// The expression should evaluate to a boolean
	result, err := exprValue.Bool()
	if err != nil {
		// If it's not a boolean, check if the expression is incomplete (waiting for more data)
		if exprValue.Exists() && !exprValue.IsConcrete() {
			return false, nil // Keep waiting
		}
		return false, fmt.Errorf("waitFor expression must evaluate to boolean %s: %w", expr, err)
	}

	return result, nil
}

// getYAMLFiles returns a sorted list of YAML files in a directory
func getYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			files = append(files, filepath.Join(dir, name))
		}
	}

	// Sort alphabetically for deterministic ordering
	sort.Strings(files)
	return files, nil
}

// parseTimeout parses a timeout string (e.g., "30s", "5m") or returns the default
func parseTimeout(timeout string, defaultTimeout time.Duration) time.Duration {
	if timeout == "" {
		return defaultTimeout
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return defaultTimeout
	}
	return d
}
